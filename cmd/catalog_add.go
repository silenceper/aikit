package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	catalogCmd.AddCommand(catalogAddCmd)
	catalogAddCmd.Flags().StringVar(&catalogAddSkill, "skill", "", "Skill name to register")
	catalogAddCmd.Flags().StringVar(&catalogAddRule, "rule", "", "Rule name to register")
	catalogAddCmd.Flags().StringVar(&catalogAddMcp, "mcp", "", "MCP name to register")
	catalogAddCmd.Flags().StringVar(&catalogAddCommand, "command", "", "Command name to register")
	catalogAddCmd.Flags().StringVar(&catalogAddGroup, "group", "Ungrouped", "Group name for the catalog entry")
}

var catalogAddCmd = &cobra.Command{
	Use:   "add [source]",
	Short: "Register assets from a remote repo or local path into the global catalog",
	Long:  "Source can be GitHub shorthand (user/repo), a Git URL, or a local path (e.g. .).",
	RunE:  runCatalogAdd,
}

var catalogAddSkill, catalogAddRule, catalogAddMcp, catalogAddCommand, catalogAddGroup string

func runCatalogAdd(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("provide a source: remote (user/repo or URL) or local path (e.g. .)")
	}
	src := args[0]
	group := catalogAddGroup
	if group == "" {
		group = "Ungrouped"
	}

	repoDir, resolvedSrc, err := resolveSourceDir(src)
	if err != nil {
		return err
	}

	hasFlag := catalogAddSkill != "" || catalogAddRule != "" || catalogAddMcp != "" || catalogAddCommand != ""

	if catalogAddSkill != "" {
		return catalogAddSkillByName(repoDir, resolvedSrc, group)
	}
	if catalogAddRule != "" {
		return catalogAddAssetByName(repoDir, resolvedSrc, group, "rule", catalogAddRule)
	}
	if catalogAddMcp != "" {
		return catalogAddAssetByName(repoDir, resolvedSrc, group, "mcp", catalogAddMcp)
	}
	if catalogAddCommand != "" {
		return catalogAddAssetByName(repoDir, resolvedSrc, group, "command", catalogAddCommand)
	}

	if hasFlag {
		return nil
	}

	// No specific flag — discover all assets and show interactive selection
	return catalogAddInteractive(repoDir, resolvedSrc, group)
}

func resolveSourceDir(src string) (repoDir, resolvedSrc string, err error) {
	if isLocalPath(src) {
		abs, err := filepath.Abs(src)
		if err != nil {
			return "", "", err
		}
		return abs, config.LocalSourceID(), nil
	}
	cacheDir, err := config.CacheDir()
	if err != nil {
		return "", "", err
	}
	subdir := source.NormalizeSource(src)
	if subdir == "" {
		return "", "", fmt.Errorf("could not normalize source %q", src)
	}
	dest := filepath.Join(cacheDir, subdir)
	if err := source.CloneOrFetch(src, dest); err != nil {
		return "", "", err
	}
	return dest, src, nil
}

func catalogAddSkillByName(repoDir, src, group string) error {
	skills, err := skill.Discover(repoDir)
	if err != nil {
		return err
	}
	var found *skill.Info
	for i := range skills {
		if skills[i].Name == catalogAddSkill {
			found = &skills[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("skill %q not found in %s", catalogAddSkill, repoDir)
	}
	entry := asset.CatalogEntry{Name: found.Name, Source: src, Description: found.Desc, Group: group}
	if err := catalog.AddSkill(entry); err != nil {
		return err
	}
	if src == config.LocalSourceID() {
		_ = copyToLocalAssetDir(found.Dir, "skill", found.Name)
	}
	fmt.Printf("Registered skill %s (source: %s)\n", found.Name, src)
	return nil
}

func catalogAddAssetByName(repoDir, src, group, kind, name string) error {
	assets, err := discovery.DiscoverByKind(repoDir, kind)
	if err != nil {
		return err
	}
	var found *discovery.AssetInfo
	for i := range assets {
		if assets[i].Name == name {
			found = &assets[i]
			break
		}
	}
	if found == nil {
		return fmt.Errorf("%s %q not found in %s", kind, name, repoDir)
	}
	entry := asset.CatalogEntry{Name: found.Name, Source: src, Description: found.Desc, Group: group}
	var addFn func(asset.CatalogEntry) error
	switch kind {
	case "rule":
		addFn = catalog.AddRule
	case "mcp":
		addFn = catalog.AddMCP
	case "command":
		addFn = catalog.AddCommand
	}
	if err := addFn(entry); err != nil {
		return err
	}
	if src == config.LocalSourceID() {
		_ = copyToLocalAssetDir(found.Dir, kind, found.Name)
	}
	fmt.Printf("Registered %s %s (source: %s)\n", kind, found.Name, src)
	return nil
}

func catalogAddInteractive(repoDir, src, group string) error {
	// Discover skills
	skills, _ := skill.Discover(repoDir)
	// Discover rules, mcps, commands
	assetItems, _ := discovery.DiscoverAll(repoDir)

	var items []tui.CatalogItem
	for _, s := range skills {
		items = append(items, tui.CatalogItem{Kind: "skill", Name: s.Name, Source: src, Desc: s.Desc})
	}
	for _, a := range assetItems {
		items = append(items, tui.CatalogItem{Kind: a.Kind, Name: a.Name, Source: src, Desc: a.Desc})
	}
	if len(items) == 0 {
		fmt.Println("No assets found in", repoDir)
		return nil
	}

	fmt.Printf("Found %d asset(s)\n", len(items))
	selected, err := tui.SelectCatalogItems(items)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}

	groupName, err := tui.InputGroup(group)
	if err != nil {
		return err
	}

	for _, item := range selected {
		entry := asset.CatalogEntry{Name: item.Name, Source: src, Description: item.Desc, Group: groupName}
		var addFn func(asset.CatalogEntry) error
		switch item.Kind {
		case "skill":
			addFn = catalog.AddSkill
		case "rule":
			addFn = catalog.AddRule
		case "mcp":
			addFn = catalog.AddMCP
		case "command":
			addFn = catalog.AddCommand
		default:
			continue
		}
		if err := addFn(entry); err != nil {
			return err
		}
		fmt.Printf("  Registered %s %s\n", item.Kind, item.Name)
	}
	fmt.Printf("Registered %d asset(s) to global catalog\n", len(selected))
	return nil
}

func isLocalPath(s string) bool {
	return s == "." || strings.HasPrefix(s, "./") || (len(s) > 0 && s[0] == '/')
}

func copyToLocalAssetDir(dir, kind, name string) error {
	var base string
	var err error
	switch kind {
	case "skill":
		base, err = config.LocalSkillDir()
	case "rule":
		base, err = config.LocalRuleDir()
	case "command":
		base, err = config.LocalCommandDir()
	default:
		return fmt.Errorf("unknown asset kind: %s", kind)
	}
	if err != nil {
		return err
	}
	localRoot := filepath.Join(base, name)
	os.RemoveAll(localRoot)
	return copyDir(localRoot, dir)
}

func copyDir(dst, src string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
}
