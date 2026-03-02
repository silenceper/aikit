package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/skill"
	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(publishCmd)
	publishCmd.Flags().StringVar(&publishRemote, "remote", "", "Target repository (e.g. user/repo or full URL)")
	publishCmd.MarkFlagRequired("remote")
	publishCmd.Flags().StringVar(&publishSkill, "skill", "", "Publish only this skill")
	publishCmd.Flags().StringVar(&publishRule, "rule", "", "Publish only this rule")
	publishCmd.Flags().StringVar(&publishMcp, "mcp", "", "Publish only this MCP")
	publishCmd.Flags().StringVar(&publishCommand, "command", "", "Publish only this command")
	publishCmd.Flags().StringVarP(&publishDir, "dir", "C", ".", "Project directory")
	publishCmd.Flags().StringVar(&publishBranch, "branch", "", "Target branch in remote repository")
}

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish local project assets to a remote Git repository",
	Long: `Discover assets from project IDE directories and push selected ones
to a remote Git repository for sharing.

Skills are discovered from IDE skill dirs (.cursor/skills/, .claude/skills/, etc.).
Rules are discovered from IDE rule dirs (.cursor/rules/, .windsurf/rules/).
Commands are discovered from IDE command dirs (.cursor/commands/, .claude/commands/).`,
	RunE: runPublish,
}

var publishRemote, publishSkill, publishRule, publishMcp, publishCommand, publishDir, publishBranch string

type publishItem struct {
	Kind string
	Name string
	Dir  string // skill directory (contains SKILL.md)
	File string // rule/command file path (single file)
}

func runPublish(cmd *cobra.Command, args []string) error {
	projectDir, err := filepath.Abs(publishDir)
	if err != nil {
		return err
	}

	allItems := discoverPublishableAssets(projectDir)
	if len(allItems) == 0 {
		fmt.Println("No local assets found to publish.")
		return nil
	}

	var toPublish []publishItem
	hasFlag := publishSkill != "" || publishRule != "" || publishMcp != "" || publishCommand != ""
	if hasFlag {
		if publishSkill != "" {
			toPublish = filterPublishItems(allItems, "skill", publishSkill)
		} else if publishRule != "" {
			toPublish = filterPublishItems(allItems, "rule", publishRule)
		} else if publishMcp != "" {
			toPublish = filterPublishItems(allItems, "mcp", publishMcp)
		} else if publishCommand != "" {
			toPublish = filterPublishItems(allItems, "command", publishCommand)
		}
	} else {
		var catalogItems []tui.CatalogItem
		for _, item := range allItems {
			catalogItems = append(catalogItems, tui.CatalogItem{Kind: item.Kind, Name: item.Name, Source: "local"})
		}
		selected, err := tui.SelectCatalogItems(catalogItems)
		if err != nil {
			return err
		}
		if len(selected) == 0 {
			fmt.Println("Nothing selected.")
			return nil
		}
		nameSet := make(map[string]bool)
		for _, s := range selected {
			nameSet[s.Kind+":"+s.Name] = true
		}
		for _, item := range allItems {
			if nameSet[item.Kind+":"+item.Name] {
				toPublish = append(toPublish, item)
			}
		}
	}

	if len(toPublish) == 0 {
		fmt.Println("No matching assets found to publish.")
		return nil
	}

	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}
	subdir := source.NormalizeSource(publishRemote)
	if publishBranch != "" {
		subdir = subdir + "@" + publishBranch
	}
	repoDir := filepath.Join(cacheDir, subdir)
	fmt.Printf("Preparing remote repo %s ...\n", publishRemote)
	if err := source.CloneOrFetch(publishRemote, repoDir, publishBranch); err != nil {
		return fmt.Errorf("fetch remote: %w", err)
	}

	for _, item := range toPublish {
		destDir := filepath.Join(repoDir, item.Kind+"s", item.Name)
		if item.Dir != "" {
			if err := agent.ExportCopyDir(item.Dir, destDir); err != nil {
				return fmt.Errorf("copy %s %s: %w", item.Kind, item.Name, err)
			}
		} else if item.File != "" {
			if err := exportFileAsAsset(item, destDir); err != nil {
				return fmt.Errorf("export %s %s: %w", item.Kind, item.Name, err)
			}
		}
		fmt.Printf("  Prepared %s: %s\n", item.Kind, item.Name)
	}

	msg := fmt.Sprintf("aikit: publish %d asset(s)", len(toPublish))
	if err := publishGitPush(repoDir, msg, publishBranch); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	updatePublishRefs(toPublish)

	fmt.Printf("\n%d asset(s) published to %s\n", len(toPublish), publishRemote)
	return nil
}

// exportFileAsAsset converts a single IDE file (rule/command) into the standard
// remote format: <name>/asset.yaml + <name>/content.md
func exportFileAsAsset(item publishItem, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	content, err := os.ReadFile(item.File)
	if err != nil {
		return err
	}

	// Strip .mdc YAML frontmatter (Cursor rules have --- delimited headers)
	body := string(content)
	if item.Kind == "rule" && strings.HasPrefix(body, "---\n") {
		if idx := strings.Index(body[4:], "---\n"); idx >= 0 {
			body = strings.TrimSpace(body[4+idx+4:])
		}
	}

	if err := os.WriteFile(filepath.Join(destDir, "content.md"), []byte(body), 0644); err != nil {
		return err
	}

	assetYAML := fmt.Sprintf(`kind: %s
metadata:
  name: %s
spec:
  content_file: content.md
`, item.Kind, item.Name)

	return os.WriteFile(filepath.Join(destDir, "asset.yaml"), []byte(assetYAML), 0644)
}

// discoverPublishableAssets discovers all assets from project IDE directories.
func discoverPublishableAssets(projectDir string) []publishItem {
	seen := make(map[string]bool)
	var items []publishItem

	for _, s := range discoverLocalSkills(projectDir) {
		key := "skill:" + s.Name
		if !seen[key] {
			items = append(items, publishItem{Kind: "skill", Name: s.Name, Dir: s.Dir})
			seen[key] = true
		}
	}

	ruleDirs := []string{".cursor/rules", ".windsurf/rules"}
	for _, rel := range ruleDirs {
		dir := filepath.Join(projectDir, rel)
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			relPath, _ := filepath.Rel(dir, path)
			name := stripRuleExt(relPath)
			key := "rule:" + name
			if seen[key] {
				return nil
			}
			items = append(items, publishItem{Kind: "rule", Name: name, File: path})
			seen[key] = true
			return nil
		})
	}

	ruleFiles := []string{"AGENTS.md", "CLAUDE.md", ".github/copilot-instructions.md"}
	for _, rel := range ruleFiles {
		fullPath := filepath.Join(projectDir, rel)
		if _, err := os.Stat(fullPath); err != nil {
			continue
		}
		name := stripRuleExt(filepath.Base(rel))
		key := "rule:" + name
		if seen[key] {
			continue
		}
		items = append(items, publishItem{Kind: "rule", Name: name, File: fullPath})
		seen[key] = true
	}

	cmdDirs := []string{".cursor/commands", ".claude/commands"}
	for _, rel := range cmdDirs {
		dir := filepath.Join(projectDir, rel)
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			key := "command:" + name
			if seen[key] {
				continue
			}
			items = append(items, publishItem{Kind: "command", Name: name, File: filepath.Join(dir, e.Name())})
			seen[key] = true
		}
	}

	return items
}

func stripRuleExt(name string) string {
	name = strings.TrimSuffix(name, ".mdc")
	name = strings.TrimSuffix(name, ".md")
	return name
}

func filterPublishItems(items []publishItem, kind, name string) []publishItem {
	for _, item := range items {
		if item.Kind == kind && item.Name == name {
			return []publishItem{item}
		}
	}
	fmt.Printf("%s %q not found. Available local %ss:\n", kind, name, kind)
	for _, item := range items {
		if item.Kind == kind {
			fmt.Printf("  - %s\n", item.Name)
		}
	}
	return nil
}

func updatePublishRefs(published []publishItem) {
	projCfg, _ := config.LoadProject(publishDir)
	if projCfg != nil {
		updated := false
		for _, item := range published {
			switch item.Kind {
			case "skill":
				for i, ref := range projCfg.Assets.Skills {
					if ref.Name == item.Name && (ref.Source == "" || ref.Source == config.LocalSourceID()) {
						projCfg.Assets.Skills[i].Source = publishRemote
						updated = true
					}
				}
			case "rule":
				for i, ref := range projCfg.Assets.Rules {
					if ref.Name == item.Name && (ref.Source == "" || ref.Source == config.LocalSourceID()) {
						projCfg.Assets.Rules[i].Source = publishRemote
						updated = true
					}
				}
			case "mcp":
				for i, ref := range projCfg.Assets.Mcps {
					if ref.Name == item.Name && (ref.Source == "" || ref.Source == config.LocalSourceID()) {
						projCfg.Assets.Mcps[i].Source = publishRemote
						updated = true
					}
				}
			case "command":
				for i, ref := range projCfg.Assets.Commands {
					if ref.Name == item.Name && (ref.Source == "" || ref.Source == config.LocalSourceID()) {
						projCfg.Assets.Commands[i].Source = publishRemote
						updated = true
					}
				}
			}
		}
		if updated {
			_ = config.SaveProject(publishDir, projCfg)
			fmt.Println("Updated .aikit.yaml source references.")
		}
	}

	cat, err := config.LoadCatalog()
	if err != nil {
		return
	}
	catUpdated := false
	for _, item := range published {
		switch item.Kind {
		case "skill":
			for i, e := range cat.Skills {
				if e.Name == item.Name && e.Source == config.LocalSourceID() {
					cat.Skills[i].Source = publishRemote
					catUpdated = true
				}
			}
		case "rule":
			for i, e := range cat.Rules {
				if e.Name == item.Name && e.Source == config.LocalSourceID() {
					cat.Rules[i].Source = publishRemote
					catUpdated = true
				}
			}
		case "mcp":
			for i, e := range cat.Mcps {
				if e.Name == item.Name && e.Source == config.LocalSourceID() {
					cat.Mcps[i].Source = publishRemote
					catUpdated = true
				}
			}
		case "command":
			for i, e := range cat.Commands {
				if e.Name == item.Name && e.Source == config.LocalSourceID() {
					cat.Commands[i].Source = publishRemote
					catUpdated = true
				}
			}
		}
	}
	if catUpdated {
		_ = config.SaveCatalog(cat)
		fmt.Println("Updated catalog source references.")
	}
}

func discoverLocalSkills(projectDir string) []skill.Info {
	seen := make(map[string]bool)
	var results []skill.Info

	for _, a := range agent.All() {
		skillDir := filepath.Join(projectDir, a.ProjectSkillDir())
		entries, err := os.ReadDir(skillDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || seen[e.Name()] {
				continue
			}
			fullPath := filepath.Join(skillDir, e.Name())
			skillMD := filepath.Join(fullPath, "SKILL.md")
			if _, err := os.Stat(skillMD); os.IsNotExist(err) {
				continue
			}
			skills, err := skill.Discover(fullPath)
			if err != nil || len(skills) == 0 {
				results = append(results, skill.Info{Name: e.Name(), Dir: fullPath})
			} else {
				results = append(results, skills[0])
			}
			seen[e.Name()] = true
		}
	}
	return results
}

func publishGitPush(repoDir, message, branch string) error {
	run := func(args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	if err := run("add", "."); err != nil {
		return err
	}
	check := exec.Command("git", "diff", "--cached", "--quiet")
	check.Dir = repoDir
	if check.Run() == nil {
		fmt.Println("No changes to push (remote already up to date).")
		return nil
	}
	if err := run("commit", "--no-gpg-sign", "-m", message); err != nil {
		return err
	}
	if branch != "" {
		return run("push", "-u", "origin", branch)
	}
	return run("push", "-u", "origin", "HEAD")
}
