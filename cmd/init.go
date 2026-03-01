package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVar(&initFrom, "from", "", "Initialize from a remote repo (user/repo) or a local .aikit.yaml file path")
	initCmd.Flags().StringVarP(&initProjectDir, "dir", "C", ".", "Project directory")
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create .aikit.yaml interactively or from a shared config",
	Long: `Initialize a new project with .aikit.yaml.

  aikit init                              Create a blank .aikit.yaml
  aikit init --from user/repo             Import from a remote repo (interactive selection)
  aikit init --from /path/to/.aikit.yaml  Import from a local file (interactive selection)`,
	RunE: runInit,
}

var initFrom string
var initProjectDir string

func runInit(cmd *cobra.Command, args []string) error {
	if initFrom != "" {
		return runInitFrom(initFrom, initProjectDir)
	}
	name := filepath.Base(initProjectDir)
	if name == "." {
		wd, _ := os.Getwd()
		name = filepath.Base(wd)
	}
	cfg := &config.ProjectConfig{}
	cfg.Project.Name = name
	if err := config.SaveProject(initProjectDir, cfg); err != nil {
		return err
	}
	fmt.Println("Created .aikit.yaml with project name:", name)
	fmt.Println("\nNext steps:")
	fmt.Println("  aikit add <source>        Add assets from a remote repo")
	fmt.Println("  aikit sync                Sync assets to your IDEs")
	return nil
}

func runInitFrom(from, dir string) error {
	srcCfg, err := loadRemoteConfig(from)
	if err != nil {
		return err
	}

	// Collect all assets from the source config
	var items []tui.CatalogItem
	for _, r := range srcCfg.Assets.Skills {
		items = append(items, tui.CatalogItem{Kind: "skill", Name: r.Name, Source: r.Source})
	}
	for _, r := range srcCfg.Assets.Rules {
		items = append(items, tui.CatalogItem{Kind: "rule", Name: r.Name, Source: r.Source})
	}
	for _, r := range srcCfg.Assets.Mcps {
		items = append(items, tui.CatalogItem{Kind: "mcp", Name: r.Name, Source: r.Source})
	}
	for _, r := range srcCfg.Assets.Commands {
		items = append(items, tui.CatalogItem{Kind: "command", Name: r.Name, Source: r.Source})
	}

	if len(items) == 0 && len(srcCfg.LocalRules) == 0 {
		fmt.Println("No assets found in the source config.")
		return nil
	}

	fmt.Printf("Found %d asset(s) in source config", len(items))
	if len(srcCfg.LocalRules) > 0 {
		fmt.Printf(" + %d local rule(s)", len(srcCfg.LocalRules))
	}
	fmt.Println()

	// Interactive selection
	var selected []tui.CatalogItem
	if len(items) > 0 {
		selected, err = tui.SelectCatalogItems(items)
		if err != nil {
			return err
		}
	}

	// Build the new project config with only selected assets
	name := filepath.Base(dir)
	if name == "." {
		wd, _ := os.Getwd()
		name = filepath.Base(wd)
	}
	newCfg := &config.ProjectConfig{}
	newCfg.Project.Name = name
	if len(srcCfg.Project.Targets) > 0 {
		newCfg.Project.Targets = srcCfg.Project.Targets
	}

	for _, item := range selected {
		ref := asset.AssetRef{Source: item.Source, Name: item.Name}
		switch item.Kind {
		case "skill":
			newCfg.Assets.Skills = append(newCfg.Assets.Skills, ref)
		case "rule":
			newCfg.Assets.Rules = append(newCfg.Assets.Rules, ref)
		case "mcp":
			newCfg.Assets.Mcps = append(newCfg.Assets.Mcps, ref)
		case "command":
			newCfg.Assets.Commands = append(newCfg.Assets.Commands, ref)
		}
	}

	// local_rules are always included (they are project-specific inline rules)
	newCfg.LocalRules = srcCfg.LocalRules

	if err := config.SaveProject(dir, newCfg); err != nil {
		return err
	}

	total := len(selected)
	fmt.Printf("\nCreated .aikit.yaml with %d asset(s) from %s\n", total, from)
	if len(newCfg.LocalRules) > 0 {
		fmt.Printf("Included %d local rule(s)\n", len(newCfg.LocalRules))
	}
	fmt.Println("\nRun 'aikit sync' to install assets to your IDEs.")
	return nil
}

func loadRemoteConfig(from string) (*config.ProjectConfig, error) {
	if isLocalFile(from) {
		return config.LoadProjectFile(from)
	}
	cacheDir, err := config.CacheDir()
	if err != nil {
		return nil, err
	}
	subdir := source.NormalizeSource(from)
	repoDir := filepath.Join(cacheDir, subdir)
	fmt.Printf("Fetching %s ...\n", from)
	if err := source.CloneOrFetch(from, repoDir); err != nil {
		return nil, fmt.Errorf("fetch %s: %w", from, err)
	}
	srcFile := filepath.Join(repoDir, ".aikit.yaml")
	return config.LoadProjectFile(srcFile)
}

func isLocalFile(s string) bool {
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "./") || strings.HasPrefix(s, "../") {
		return true
	}
	if strings.HasSuffix(s, ".yaml") || strings.HasSuffix(s, ".yml") {
		if _, err := os.Stat(s); err == nil {
			return true
		}
	}
	return false
}
