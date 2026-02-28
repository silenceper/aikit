package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/internal/catalog"
	"github.com/silenceper/aikit/internal/skill"
	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	catalogCmd.AddCommand(catalogAddCmd)
}

var catalogAddCmd = &cobra.Command{
	Use:   "add [source]",
	Short: "Register assets from a remote repo or local path into the global catalog",
	Long:  "Source can be GitHub shorthand (user/repo), a Git URL, or a local path (e.g. .). Use --skill/--mcp/--group to register specific assets.",
	RunE:  runCatalogAdd,
}

var catalogAddSkill, catalogAddMcp, catalogAddGroup string

func init() {
	catalogAddCmd.Flags().StringVar(&catalogAddSkill, "skill", "", "Skill name to register")
	catalogAddCmd.Flags().StringVar(&catalogAddMcp, "mcp", "", "MCP name to register")
	catalogAddCmd.Flags().StringVar(&catalogAddGroup, "group", "Ungrouped", "Group name for the catalog entry")
}

func runCatalogAdd(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("provide a source: remote (user/repo or URL) or local path (e.g. .)")
	}
	src := args[0]
	group := catalogAddGroup
	if group == "" {
		group = "Ungrouped"
	}

	// Local path: source becomes _local
	if isLocalPath(src) {
		abs, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		if catalogAddSkill != "" {
			// Single skill from local dir
			skills, err := skill.Discover(abs)
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
				return fmt.Errorf("skill %q not found under %s", catalogAddSkill, abs)
			}
			entry := asset.CatalogEntry{
				Name:        found.Name,
				Source:      config.LocalSourceID(),
				Description: found.Desc,
				Group:       group,
			}
			if err := catalog.AddSkill(entry); err != nil {
				return err
			}
			fmt.Println("Registered skill", catalogAddSkill, "from local path (source: _local)")
			return copyToLocalCache(found.Dir, "skill", found.Name)
		}
		if catalogAddMcp != "" {
			// TODO: discover MCP from asset.yaml
			return fmt.Errorf("catalog add . --mcp is not implemented yet")
		}
		// No specific asset: interactive selection from local
		skills, err := skill.Discover(abs)
		if err != nil {
			return err
		}
		if len(skills) == 0 {
			fmt.Println("No assets found in", abs)
			return nil
		}
		fmt.Printf("Found %d skill(s) in local path\n", len(skills))
		selected, err := tui.SelectSkills(skills)
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			fmt.Println("Nothing selected.")
			return nil
		}
		for _, name := range selected {
			var dir string
			for _, s := range skills {
				if s.Name == name {
					dir = s.Dir
					break
				}
			}
			desc := ""
			for _, s := range skills {
				if s.Name == name {
					desc = s.Desc
					break
				}
			}
			entry := asset.CatalogEntry{
				Name:        name,
				Source:      config.LocalSourceID(),
				Description: desc,
				Group:       group,
			}
			if err := catalog.AddSkill(entry); err != nil {
				return err
			}
			if dir != "" {
				_ = copyToLocalCache(dir, "skill", name)
			}
			fmt.Printf("  Registered skill %s (source: _local)\n", name)
		}
		fmt.Printf("Registered %d skill(s) to global catalog\n", len(selected))
		return nil
	}

	// Remote: clone/fetch, then discover and add
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}
	subdir := source.NormalizeSource(src)
	if subdir == "" {
		return fmt.Errorf("could not normalize source %q", src)
	}
	dest := filepath.Join(cacheDir, subdir)
	if err := source.CloneOrFetch(src, dest); err != nil {
		return err
	}
	skills, err := skill.Discover(dest)
	if err != nil {
		return err
	}
	if catalogAddSkill != "" {
		var found *skill.Info
		for i := range skills {
			if skills[i].Name == catalogAddSkill {
				found = &skills[i]
				break
			}
		}
		if found == nil {
			return fmt.Errorf("skill %q not found in %s", catalogAddSkill, src)
		}
		entry := asset.CatalogEntry{
			Name:        found.Name,
			Source:      src,
			Description: found.Desc,
			Group:       group,
		}
		if err := catalog.AddSkill(entry); err != nil {
			return err
		}
		fmt.Println("Registered skill", catalogAddSkill, "from", src)
		return nil
	}
	if catalogAddMcp != "" {
		// TODO: discover MCP in dest
		return fmt.Errorf("catalog add <remote> --mcp not implemented yet")
	}
	// No specific asset: interactive multi-select
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
	groupName, err := tui.InputGroup(group)
	if err != nil {
		return err
	}
	for _, name := range selected {
		desc := ""
		for _, s := range skills {
			if s.Name == name {
				desc = s.Desc
				break
			}
		}
		entry := asset.CatalogEntry{
			Name:        name,
			Source:      src,
			Description: desc,
			Group:       groupName,
		}
		if err := catalog.AddSkill(entry); err != nil {
			return err
		}
		fmt.Printf("  Registered skill %s\n", name)
	}
	fmt.Printf("Registered %d skill(s) to global catalog\n", len(selected))
	return nil
}

func isLocalPath(s string) bool {
	return s == "." || strings.HasPrefix(s, "./") || (len(s) > 0 && s[0] == '/')
}

func copyToLocalCache(dir, kind, name string) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}
	localRoot := filepath.Join(cacheDir, "_local", kind, name)
	if err := os.MkdirAll(localRoot, 0755); err != nil {
		return err
	}
	// Copy dir contents into localRoot (e.g. SKILL.md)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(localRoot, e.Name()), data, 0644); err != nil {
			return err
		}
	}
	return nil
}
