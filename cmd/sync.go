package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/internal/skill"
	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(syncCmd)
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync project assets to target IDEs (Cursor, Claude Code, etc.)",
	Long:  "Reads .aikit.yaml, fetches missing assets from remote if needed, and installs them to detected agents.",
	RunE:  runSync,
}

var syncTargets []string
var syncDir string
var syncDryRun bool

func init() {
	syncCmd.Flags().StringSliceVar(&syncTargets, "target", nil, "Limit to these agents (e.g. cursor, claude-code)")
	syncCmd.Flags().StringVarP(&syncDir, "dir", "C", ".", "Project directory")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "Show what would be done without writing")
}

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadProject(syncDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no .aikit.yaml in %s; run 'aikit init' first", syncDir)
		}
		return err
	}

	projectDir, _ := filepath.Abs(syncDir)

	var agents []agent.Agent
	if len(syncTargets) > 0 {
		agents = agent.ByName(syncTargets)
	} else {
		var options []tui.AgentOption
		for _, a := range agent.All() {
			options = append(options, tui.AgentOption{
				Name:     a.Name(),
				Detected: a.Detect(projectDir),
			})
		}
		selectedNames, err := tui.SelectAgents(options)
		if err != nil {
			return err
		}
		if len(selectedNames) == 0 {
			fmt.Println("No agents selected.")
			return nil
		}
		agents = agent.ByName(selectedNames)
	}
	if len(agents) == 0 {
		fmt.Println("No matching agents found.")
		return nil
	}

	fmt.Printf("Syncing project %s to: ", cfg.Project.Name)
	for i, a := range agents {
		if i > 0 {
			fmt.Print(", ")
		}
		fmt.Print(a.Name())
	}
	fmt.Println()

	if syncDryRun {
		fmt.Println("\nDry run — would sync:")
		fmt.Printf("  Skills:   %d\n", len(cfg.Assets.Skills))
		fmt.Printf("  Rules:    %d\n", len(cfg.Assets.Rules))
		fmt.Printf("  MCPs:     %d\n", len(cfg.Assets.Mcps))
		fmt.Printf("  Commands: %d\n", len(cfg.Assets.Commands))
		return nil
	}

	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}

	skillCount := 0
	for _, ref := range cfg.Assets.Skills {
		dir, err := resolveSkillDir(ref, cacheDir)
		if err != nil {
			fmt.Printf("  Warning: could not resolve skill %s: %v\n", ref.Name, err)
			continue
		}
		for _, a := range agents {
			if err := a.InstallSkill(dir, ref.Name); err != nil {
				fmt.Printf("  Warning: failed to install skill %s to %s: %v\n", ref.Name, a.Name(), err)
			}
		}
		skillCount++
	}

	fmt.Printf("\nDone: %d skill(s) synced to %d agent(s)\n", skillCount, len(agents))
	if len(cfg.Assets.Rules) > 0 {
		fmt.Printf("  Rules sync: planned for Phase 2\n")
	}
	if len(cfg.Assets.Mcps) > 0 {
		fmt.Printf("  MCP sync: planned for Phase 2\n")
	}
	if len(cfg.Assets.Commands) > 0 {
		fmt.Printf("  Command sync: planned for Phase 2\n")
	}
	return nil
}

// resolveSkillDir ensures the skill is cached locally and returns the directory containing SKILL.md.
func resolveSkillDir(ref asset.AssetRef, cacheDir string) (string, error) {
	if ref.Source == "" || ref.Source == config.LocalSourceID() {
		localDir := filepath.Join(cacheDir, "_local", "skill", ref.Name)
		if _, err := os.Stat(localDir); err == nil {
			return localDir, nil
		}
		return "", fmt.Errorf("local skill %s not found in cache", ref.Name)
	}

	subdir := source.NormalizeSource(ref.Source)
	repoDir := filepath.Join(cacheDir, subdir)

	// Auto-fetch if cache is missing
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		fmt.Printf("  Fetching %s ...\n", ref.Source)
		if err := source.CloneOrFetch(ref.Source, repoDir); err != nil {
			return "", fmt.Errorf("fetch %s: %w", ref.Source, err)
		}
	}

	// Discover skills in the cached repo to find the exact directory
	skills, err := skill.Discover(repoDir)
	if err != nil {
		return "", err
	}
	for _, s := range skills {
		if s.Name == ref.Name {
			return s.Dir, nil
		}
	}
	return "", fmt.Errorf("skill %s not found in %s", ref.Name, ref.Source)
}
