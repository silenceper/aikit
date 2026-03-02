package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/internal/catalog"
	"github.com/silenceper/aikit/internal/discovery"
	"github.com/silenceper/aikit/internal/skill"
	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVar(&addSkill, "skill", "", "Skill name to add")
	addCmd.Flags().StringVar(&addRule, "rule", "", "Rule name to add")
	addCmd.Flags().StringVar(&addMcp, "mcp", "", "MCP name to add")
	addCmd.Flags().StringVar(&addCommand, "command", "", "Command name to add")
	addCmd.Flags().StringVarP(&addProjectDir, "dir", "C", ".", "Project directory")
}

var addCmd = &cobra.Command{
	Use:   "add [source]",
	Short: "Add an asset to the current project (.aikit.yaml)",
	Long: `Add from a remote source:
  aikit add user/repo --skill name

From global catalog by name:
  aikit add --skill name

Interactive selection from catalog (no args):
  aikit add`,
	RunE: runAdd,
}

var addSkill, addRule, addMcp, addCommand, addProjectDir string

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
		return addFromRemote(cfg, remoteSource)
	}
	if hasAssetFlag {
		return addFromCatalogByName(cfg)
	}
	return addFromCatalogInteractive(cfg)
}

func addFromRemote(cfg *config.ProjectConfig, src string) error {
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
		var found bool
		for _, s := range skills {
			if s.Name == addSkill {
				found = true
				break
			}
		}
		if !found {
			fmt.Printf("Skill %q not found in %s. Available:\n", addSkill, src)
			for _, s := range skills {
				fmt.Printf("  - %s\n", s.Name)
			}
			return fmt.Errorf("skill %q not found", addSkill)
		}
		return addRefToProject(cfg, "skill", src, addSkill)
	}
	if addRule != "" {
		if err := validateAssetExists(dest, "rule", addRule); err != nil {
			return err
		}
		return addRefToProject(cfg, "rule", src, addRule)
	}
	if addMcp != "" {
		if err := validateAssetExists(dest, "mcp", addMcp); err != nil {
			return err
		}
		return addRefToProject(cfg, "mcp", src, addMcp)
	}
	if addCommand != "" {
		if err := validateAssetExists(dest, "command", addCommand); err != nil {
			return err
		}
		return addRefToProject(cfg, "command", src, addCommand)
	}

	// No flag: interactive — discover all asset types
	var items []tui.CatalogItem
	skills, _ := skill.Discover(dest)
	for _, s := range skills {
		items = append(items, tui.CatalogItem{Kind: "skill", Name: s.Name, Source: src, Desc: s.Desc})
	}
	assetItems, _ := discovery.DiscoverAll(dest)
	for _, a := range assetItems {
		items = append(items, tui.CatalogItem{Kind: a.Kind, Name: a.Name, Source: src, Desc: a.Desc})
	}
	if len(items) == 0 {
		fmt.Println("No assets found in", src)
		return nil
	}
	fmt.Printf("Found %d asset(s) in %s\n", len(items), src)
	selected, err := tui.SelectCatalogItems(items)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}
	for _, item := range selected {
		if err := addRefToProject(cfg, item.Kind, item.Source, item.Name); err != nil {
			return err
		}
	}
	fmt.Printf("Added %d asset(s) from %s\n", len(selected), src)
	return nil
}

func addFromCatalogByName(cfg *config.ProjectConfig) error {
	type lookup struct {
		kind   string
		name   string
		findFn func(string) (*asset.CatalogEntry, error)
	}
	lookups := []lookup{
		{"skill", addSkill, catalog.FindSkill},
		{"rule", addRule, catalog.FindRule},
		{"mcp", addMcp, catalog.FindMCP},
		{"command", addCommand, catalog.FindCommand},
	}
	for _, l := range lookups {
		if l.name == "" {
			continue
		}
		entry, err := l.findFn(l.name)
		if err != nil {
			return err
		}
		if entry == nil {
			return fmt.Errorf("%s %q not in catalog; use 'aikit add <source> --%s %s' or 'aikit catalog add <source>' first", l.kind, l.name, l.kind, l.name)
		}
		return addRefToProject(cfg, l.kind, entry.Source, l.name)
	}
	return nil
}

func addFromCatalogInteractive(cfg *config.ProjectConfig) error {
	cat, err := config.LoadCatalog()
	if err != nil {
		return err
	}
	var items []tui.CatalogItem
	for _, e := range cat.Skills {
		items = append(items, tui.CatalogItem{Kind: "skill", Name: e.Name, Source: e.Source, Desc: e.Description, Group: e.Group})
	}
	for _, e := range cat.Rules {
		items = append(items, tui.CatalogItem{Kind: "rule", Name: e.Name, Source: e.Source, Desc: e.Description, Group: e.Group})
	}
	for _, e := range cat.Mcps {
		items = append(items, tui.CatalogItem{Kind: "mcp", Name: e.Name, Source: e.Source, Desc: e.Description, Group: e.Group})
	}
	for _, e := range cat.Commands {
		items = append(items, tui.CatalogItem{Kind: "command", Name: e.Name, Source: e.Source, Desc: e.Description, Group: e.Group})
	}
	if len(items) == 0 {
		fmt.Println("Catalog is empty. Use 'aikit catalog add <source>' to register assets first.")
		return nil
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
		if err := addRefToProject(cfg, item.Kind, item.Source, item.Name); err != nil {
			return err
		}
	}
	fmt.Printf("\n%d asset(s) added to project.\n", len(selected))
	return nil
}

// addRefToProject adds an asset reference to the project config with deduplication.
func addRefToProject(cfg *config.ProjectConfig, kind, src, name string) error {
	ref := asset.AssetRef{Source: src, Name: name}
	var list *[]asset.AssetRef
	switch kind {
	case "skill":
		list = &cfg.Assets.Skills
	case "rule":
		list = &cfg.Assets.Rules
	case "mcp":
		list = &cfg.Assets.Mcps
	case "command":
		list = &cfg.Assets.Commands
	default:
		return fmt.Errorf("unknown asset kind: %s", kind)
	}
	for _, existing := range *list {
		if existing.Name == name && existing.Source == src {
			fmt.Printf("  %s %s already in project (source: %s), skipped\n", kind, name, src)
			return nil
		}
	}
	*list = append(*list, ref)
	if err := config.SaveProject(addProjectDir, cfg); err != nil {
		return err
	}
	fmt.Printf("  Added %s %s (source: %s)\n", kind, name, src)
	return nil
}

func validateAssetExists(repoDir, kind, name string) error {
	assets, err := discovery.DiscoverByKind(repoDir, kind)
	if err != nil {
		return nil
	}
	for _, a := range assets {
		if a.Name == name {
			return nil
		}
	}
	if len(assets) > 0 {
		fmt.Printf("%s %q not found in source. Available %ss:\n", kind, name, kind)
		for _, a := range assets {
			fmt.Printf("  - %s\n", a.Name)
		}
	}
	return nil
}
