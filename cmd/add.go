package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/internal/catalog"
	"github.com/silenceper/aikit/internal/skill"
	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add [source]",
	Short: "Add an asset to the current project (.aikit.yaml)",
	Long: `Add from a remote source:
  aikit add user/repo --skill name
  aikit add https://github.com/org/repo --skill name

From global catalog by name:
  aikit add --skill name

Interactive selection from catalog (no args):
  aikit add`,
	RunE: runAdd,
}

var (
	addSkill      string
	addRule       string
	addMcp        string
	addCommand    string
	addProjectDir string
)

func init() {
	addCmd.Flags().StringVar(&addSkill, "skill", "", "Skill name to add")
	addCmd.Flags().StringVar(&addRule, "rule", "", "Rule name to add")
	addCmd.Flags().StringVar(&addMcp, "mcp", "", "MCP name to add")
	addCmd.Flags().StringVar(&addCommand, "command", "", "Command name to add")
	addCmd.Flags().StringVarP(&addProjectDir, "dir", "C", ".", "Project directory")
}

func runAdd(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadProject(addProjectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no .aikit.yaml in %s; run 'aikit init' first", addProjectDir)
		}
		return err
	}

	remoteSource := ""
	if len(args) > 0 {
		remoteSource = args[0]
	}

	hasAssetFlag := addSkill != "" || addRule != "" || addMcp != "" || addCommand != ""

	if remoteSource != "" {
		return addFromRemote(cfg, remoteSource, hasAssetFlag)
	}
	if hasAssetFlag {
		return addFromCatalogByName(cfg)
	}
	// No args, no flags: interactive catalog selection
	return addFromCatalogInteractive(cfg)
}

// addFromRemote fetches the remote repo, discovers the requested asset, and adds it to the project.
func addFromRemote(cfg *config.ProjectConfig, src string, hasAssetFlag bool) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}
	subdir := source.NormalizeSource(src)
	if subdir == "" {
		return fmt.Errorf("could not normalize source %q", src)
	}
	dest := filepath.Join(cacheDir, subdir)

	fmt.Printf("Fetching %s ...\n", src)
	if err := source.CloneOrFetch(src, dest); err != nil {
		return fmt.Errorf("failed to fetch %s: %w", src, err)
	}

	if addSkill != "" {
		skills, err := skill.Discover(dest)
		if err != nil {
			return err
		}
		var found *skill.Info
		for i := range skills {
			if skills[i].Name == addSkill {
				found = &skills[i]
				break
			}
		}
		if found == nil {
			fmt.Printf("Skill %q not found in %s. Available skills:\n", addSkill, src)
			for _, s := range skills {
				fmt.Printf("  - %s\n", s.Name)
			}
			return fmt.Errorf("skill %q not found", addSkill)
		}
		cfg.Assets.Skills = append(cfg.Assets.Skills, asset.AssetRef{Source: src, Name: addSkill})
		if err := config.SaveProject(addProjectDir, cfg); err != nil {
			return err
		}
		fmt.Printf("Added skill %s from %s\n", addSkill, src)
		return nil
	}

	if addRule != "" {
		cfg.Assets.Rules = append(cfg.Assets.Rules, asset.AssetRef{Source: src, Name: addRule})
		if err := config.SaveProject(addProjectDir, cfg); err != nil {
			return err
		}
		fmt.Printf("Added rule %s from %s\n", addRule, src)
		return nil
	}
	if addMcp != "" {
		cfg.Assets.Mcps = append(cfg.Assets.Mcps, asset.AssetRef{Source: src, Name: addMcp})
		if err := config.SaveProject(addProjectDir, cfg); err != nil {
			return err
		}
		fmt.Printf("Added MCP %s from %s\n", addMcp, src)
		return nil
	}
	if addCommand != "" {
		cfg.Assets.Commands = append(cfg.Assets.Commands, asset.AssetRef{Source: src, Name: addCommand})
		if err := config.SaveProject(addProjectDir, cfg); err != nil {
			return err
		}
		fmt.Printf("Added command %s from %s\n", addCommand, src)
		return nil
	}

	// No specific asset flag: interactive selection
	skills, err := skill.Discover(dest)
	if err != nil {
		return err
	}
	if len(skills) == 0 {
		fmt.Println("No assets found in", src)
		return nil
	}
	fmt.Printf("Found %d skill(s) in %s\n", len(skills), src)
	selected, err := tui.SelectSkills(skills)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}
	for _, name := range selected {
		cfg.Assets.Skills = append(cfg.Assets.Skills, asset.AssetRef{Source: src, Name: name})
		fmt.Printf("  Added skill %s\n", name)
	}
	if err := config.SaveProject(addProjectDir, cfg); err != nil {
		return err
	}
	fmt.Printf("Added %d skill(s) from %s\n", len(selected), src)
	return nil
}

// addFromCatalogByName looks up a specific asset by name in the global catalog.
func addFromCatalogByName(cfg *config.ProjectConfig) error {
	if addSkill != "" {
		entry, err := catalog.FindSkill(addSkill)
		if err != nil {
			return err
		}
		if entry == nil {
			return fmt.Errorf("skill %q not in catalog; use 'aikit add <source> --skill %s' to add from remote, or 'aikit catalog add <source>' to register first", addSkill, addSkill)
		}
		cfg.Assets.Skills = append(cfg.Assets.Skills, asset.AssetRef{Source: entry.Source, Name: addSkill})
		if err := config.SaveProject(addProjectDir, cfg); err != nil {
			return err
		}
		fmt.Printf("Added skill %s from catalog (source: %s)\n", addSkill, entry.Source)
		return nil
	}
	if addMcp != "" {
		entry, err := catalog.FindMCP(addMcp)
		if err != nil {
			return err
		}
		if entry == nil {
			return fmt.Errorf("mcp %q not in catalog; use 'aikit add <source> --mcp %s' to add from remote, or 'aikit catalog add <source>' to register first", addMcp, addMcp)
		}
		cfg.Assets.Mcps = append(cfg.Assets.Mcps, asset.AssetRef{Source: entry.Source, Name: addMcp})
		if err := config.SaveProject(addProjectDir, cfg); err != nil {
			return err
		}
		fmt.Printf("Added MCP %s from catalog (source: %s)\n", addMcp, entry.Source)
		return nil
	}
	return fmt.Errorf("catalog lookup only supports --skill or --mcp; for --rule/--command, provide a source: aikit add <source> --rule <name>")
}

// addFromCatalogInteractive loads the global catalog and presents an interactive multi-select.
func addFromCatalogInteractive(cfg *config.ProjectConfig) error {
	cat, err := config.LoadCatalog()
	if err != nil {
		return err
	}

	var items []tui.CatalogItem
	for _, e := range cat.Skills {
		items = append(items, tui.CatalogItem{Kind: "skill", Name: e.Name, Source: e.Source, Desc: e.Description})
	}
	for _, e := range cat.Rules {
		items = append(items, tui.CatalogItem{Kind: "rule", Name: e.Name, Source: e.Source, Desc: e.Description})
	}
	for _, e := range cat.Mcps {
		items = append(items, tui.CatalogItem{Kind: "mcp", Name: e.Name, Source: e.Source, Desc: e.Description})
	}
	for _, e := range cat.Commands {
		items = append(items, tui.CatalogItem{Kind: "command", Name: e.Name, Source: e.Source, Desc: e.Description})
	}

	selected, err := tui.SelectCatalogItems(items)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}

	for _, item := range selected {
		ref := asset.AssetRef{Source: item.Source, Name: item.Name}
		switch item.Kind {
		case "skill":
			cfg.Assets.Skills = append(cfg.Assets.Skills, ref)
		case "rule":
			cfg.Assets.Rules = append(cfg.Assets.Rules, ref)
		case "mcp":
			cfg.Assets.Mcps = append(cfg.Assets.Mcps, ref)
		case "command":
			cfg.Assets.Commands = append(cfg.Assets.Commands, ref)
		}
		fmt.Printf("  Added %s: %s (source: %s)\n", item.Kind, item.Name, item.Source)
	}

	if err := config.SaveProject(addProjectDir, cfg); err != nil {
		return err
	}
	fmt.Printf("\n%d asset(s) added to project.\n", len(selected))
	return nil
}
