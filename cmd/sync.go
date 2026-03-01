package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/internal/discovery"
	"github.com/silenceper/aikit/internal/skill"
	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().StringSliceVar(&syncTargets, "target", nil, "Limit to these agents (e.g. cursor, claude-code)")
	syncCmd.Flags().StringVarP(&syncDir, "dir", "C", ".", "Project directory")
	syncCmd.Flags().BoolVar(&syncDryRun, "dry-run", false, "Show what would be done without writing")
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

func runSync(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadProject(syncDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no .aikit.yaml in %s; run 'aikit init' first", syncDir)
		}
		return err
	}

	projectDir, _ := filepath.Abs(syncDir)

	agents, err := resolveAgents(projectDir)
	if err != nil {
		return err
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
		fmt.Printf("  Skills:      %d\n", len(cfg.Assets.Skills))
		fmt.Printf("  Rules:       %d\n", len(cfg.Assets.Rules))
		fmt.Printf("  MCPs:        %d\n", len(cfg.Assets.Mcps))
		fmt.Printf("  Commands:    %d\n", len(cfg.Assets.Commands))
		fmt.Printf("  Local Rules: %d\n", len(cfg.LocalRules))
		return nil
	}

	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}

	counts := struct{ skill, rule, mcp, command int }{}

	// --- Skills ---
	for _, ref := range cfg.Assets.Skills {
		dir, err := resolveSkillDir(ref, cacheDir)
		if err != nil {
			fmt.Printf("  Warning: could not resolve skill %s: %v\n", ref.Name, err)
			continue
		}
		for _, a := range agents {
			if err := a.InstallSkill(projectDir, dir, ref.Name); err != nil {
				fmt.Printf("  Warning: failed to install skill %s to %s: %v\n", ref.Name, a.Name(), err)
			}
		}
		counts.skill++
	}

	// --- Rules ---
	for _, ref := range cfg.Assets.Rules {
		ruleData, err := resolveRule(ref, cacheDir)
		if err != nil {
			fmt.Printf("  Warning: could not resolve rule %s: %v\n", ref.Name, err)
			continue
		}
		for _, a := range agents {
			if err := a.InstallRule(projectDir, *ruleData); err != nil {
				fmt.Printf("  Warning: failed to install rule %s to %s: %v\n", ref.Name, a.Name(), err)
			}
		}
		counts.rule++
	}

	// --- Local Rules ---
	for _, lr := range cfg.LocalRules {
		name := lr.Name
		if name == "" {
			name = "local-rule"
		}
		ruleData := asset.RuleData{
			Name:        name,
			Content:     lr.Content,
			Globs:       lr.Globs,
			AlwaysApply: lr.AlwaysApply,
		}
		for _, a := range agents {
			if err := a.InstallRule(projectDir, ruleData); err != nil {
				fmt.Printf("  Warning: failed to install local rule %s to %s: %v\n", name, a.Name(), err)
			}
		}
		counts.rule++
	}

	// --- MCPs ---
	for _, ref := range cfg.Assets.Mcps {
		mcpData, err := resolveMCP(ref, cacheDir, agents)
		if err != nil {
			fmt.Printf("  Warning: could not resolve MCP %s: %v\n", ref.Name, err)
			continue
		}
		for _, a := range agents {
			resolved := *mcpData
			// Apply platform overrides if available — handled by resolveMCP per-agent
			if err := a.InstallMCP(projectDir, resolved); err != nil {
				fmt.Printf("  Warning: failed to install MCP %s to %s: %v\n", ref.Name, a.Name(), err)
			}
		}
		counts.mcp++
	}

	// --- Commands ---
	for _, ref := range cfg.Assets.Commands {
		cmdData, err := resolveCommand(ref, cacheDir)
		if err != nil {
			fmt.Printf("  Warning: could not resolve command %s: %v\n", ref.Name, err)
			continue
		}
		for _, a := range agents {
			if !a.SupportsCommand() {
				continue
			}
			if err := a.InstallCommand(projectDir, *cmdData); err != nil {
				fmt.Printf("  Warning: failed to install command %s to %s: %v\n", ref.Name, a.Name(), err)
			}
		}
		counts.command++
	}

	fmt.Printf("\nDone: %d skill(s), %d rule(s), %d MCP(s), %d command(s) synced to %d agent(s)\n",
		counts.skill, counts.rule, counts.mcp, counts.command, len(agents))
	return nil
}

func resolveAgents(projectDir string) ([]agent.Agent, error) {
	if len(syncTargets) > 0 {
		return agent.ByName(syncTargets), nil
	}
	var options []tui.AgentOption
	for _, a := range agent.All() {
		options = append(options, tui.AgentOption{
			Name:     a.Name(),
			Detected: a.Detect(projectDir),
		})
	}
	selectedNames, err := tui.SelectAgents(options)
	if err != nil {
		return nil, err
	}
	return agent.ByName(selectedNames), nil
}

func resolveSkillDir(ref asset.AssetRef, cacheDir string) (string, error) {
	if ref.Source == "" || ref.Source == config.LocalSourceID() {
		skillDir, err := config.LocalSkillDir()
		if err != nil {
			return "", err
		}
		localDir := filepath.Join(skillDir, ref.Name)
		if _, err := os.Stat(localDir); err == nil {
			return localDir, nil
		}
		return "", fmt.Errorf("local skill %s not found in ~/.aikit/skills/", ref.Name)
	}
	repoDir, err := ensureCache(ref.Source, cacheDir)
	if err != nil {
		return "", err
	}
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

func resolveRule(ref asset.AssetRef, cacheDir string) (*asset.RuleData, error) {
	if ref.Source == "" || ref.Source == config.LocalSourceID() {
		ruleDir, err := config.LocalRuleDir()
		if err != nil {
			return nil, err
		}
		return loadRuleFromDir(filepath.Join(ruleDir, ref.Name), ref.Name)
	}
	repoDir, err := ensureCache(ref.Source, cacheDir)
	if err != nil {
		return nil, err
	}
	assets, err := discovery.DiscoverByKind(repoDir, "rule")
	if err != nil {
		return nil, err
	}
	for _, a := range assets {
		if a.Name == ref.Name {
			return loadRuleFromAssetInfo(a)
		}
	}
	return nil, fmt.Errorf("rule %s not found in %s", ref.Name, ref.Source)
}

func loadRuleFromDir(dir, name string) (*asset.RuleData, error) {
	assets, err := discovery.DiscoverByKind(dir, "rule")
	if err != nil || len(assets) == 0 {
		// Fallback: try content.md directly
		contentFile := filepath.Join(dir, "content.md")
		if data, err := os.ReadFile(contentFile); err == nil {
			return &asset.RuleData{Name: name, Content: string(data)}, nil
		}
		return nil, fmt.Errorf("rule %s not found in %s", name, dir)
	}
	return loadRuleFromAssetInfo(assets[0])
}

func loadRuleFromAssetInfo(info discovery.AssetInfo) (*asset.RuleData, error) {
	content, err := discovery.LoadContent(info)
	if err != nil {
		return nil, err
	}
	return &asset.RuleData{
		Name:        info.Name,
		Content:     content,
		Globs:       info.Spec.Globs,
		AlwaysApply: info.Spec.AlwaysApply,
	}, nil
}

func resolveMCP(ref asset.AssetRef, cacheDir string, _ []agent.Agent) (*asset.MCPData, error) {
	if ref.Source == "" || ref.Source == config.LocalSourceID() {
		return nil, fmt.Errorf("local MCP %s: MCP assets require a remote source with asset.yaml", ref.Name)
	}
	repoDir, err := ensureCache(ref.Source, cacheDir)
	if err != nil {
		return nil, err
	}
	assets, err := discovery.DiscoverByKind(repoDir, "mcp")
	if err != nil {
		return nil, err
	}
	for _, a := range assets {
		if a.Name == ref.Name {
			return &asset.MCPData{
				Name:      a.Name,
				Transport: a.Spec.Transport,
				Command:   a.Spec.Command,
				Args:      a.Spec.Args,
				Env:       a.Spec.Env,
			}, nil
		}
	}
	return nil, fmt.Errorf("MCP %s not found in %s", ref.Name, ref.Source)
}

func resolveCommand(ref asset.AssetRef, cacheDir string) (*asset.CommandData, error) {
	if ref.Source == "" || ref.Source == config.LocalSourceID() {
		cmdDir, err := config.LocalCommandDir()
		if err != nil {
			return nil, err
		}
		return loadCommandFromDir(filepath.Join(cmdDir, ref.Name), ref.Name)
	}
	repoDir, err := ensureCache(ref.Source, cacheDir)
	if err != nil {
		return nil, err
	}
	assets, err := discovery.DiscoverByKind(repoDir, "command")
	if err != nil {
		return nil, err
	}
	for _, a := range assets {
		if a.Name == ref.Name {
			content, err := discovery.LoadContent(a)
			if err != nil {
				return nil, err
			}
			return &asset.CommandData{Name: a.Name, Content: content}, nil
		}
	}
	return nil, fmt.Errorf("command %s not found in %s", ref.Name, ref.Source)
}

func loadCommandFromDir(dir, name string) (*asset.CommandData, error) {
	contentFile := filepath.Join(dir, "content.md")
	if data, err := os.ReadFile(contentFile); err == nil {
		return &asset.CommandData{Name: name, Content: string(data)}, nil
	}
	assets, err := discovery.DiscoverByKind(dir, "command")
	if err != nil || len(assets) == 0 {
		return nil, fmt.Errorf("command %s not found in %s", name, dir)
	}
	content, err := discovery.LoadContent(assets[0])
	if err != nil {
		return nil, err
	}
	return &asset.CommandData{Name: assets[0].Name, Content: content}, nil
}

func ensureCache(src, cacheDir string) (string, error) {
	subdir := source.NormalizeSource(src)
	repoDir := filepath.Join(cacheDir, subdir)
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		fmt.Printf("  Fetching %s ...\n", src)
		if err := source.CloneOrFetch(src, repoDir); err != nil {
			return "", fmt.Errorf("fetch %s: %w", src, err)
		}
	}
	return repoDir, nil
}
