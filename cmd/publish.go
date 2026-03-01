package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/discovery"
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
	publishCmd.Flags().StringVar(&publishCommand, "command", "", "Publish only this command")
	publishCmd.Flags().StringVarP(&publishDir, "dir", "C", ".", "Project directory")
}

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish local project assets to a remote Git repository",
	Long: `Scan IDE directories for user-created assets (non-symlink skills), 
and ~/.aikit/ for local rules/commands, then push selected ones to a remote repository.`,
	RunE: runPublish,
}

var publishRemote, publishSkill, publishRule, publishCommand, publishDir string

type publishItem struct {
	Kind string
	Name string
	Dir  string
}

func runPublish(cmd *cobra.Command, args []string) error {
	projectDir, err := filepath.Abs(publishDir)
	if err != nil {
		return err
	}

	// Discover all publishable assets
	var allItems []publishItem

	// Skills from IDE dirs
	for _, s := range discoverLocalSkills(projectDir) {
		allItems = append(allItems, publishItem{Kind: "skill", Name: s.Name, Dir: s.Dir})
	}
	// Rules from ~/.aikit/rules/
	if ruleDir, err := config.LocalRuleDir(); err == nil {
		if assets, err := discovery.DiscoverByKind(ruleDir, "rule"); err == nil {
			for _, a := range assets {
				allItems = append(allItems, publishItem{Kind: "rule", Name: a.Name, Dir: a.Dir})
			}
		}
	}
	// Commands from ~/.aikit/commands/
	if cmdDir, err := config.LocalCommandDir(); err == nil {
		if assets, err := discovery.DiscoverByKind(cmdDir, "command"); err == nil {
			for _, a := range assets {
				allItems = append(allItems, publishItem{Kind: "command", Name: a.Name, Dir: a.Dir})
			}
		}
	}

	if len(allItems) == 0 {
		fmt.Println("No local (user-created) assets found to publish.")
		return nil
	}

	// Filter by flag
	var toPublish []publishItem
	if publishSkill != "" {
		toPublish = filterPublishItems(allItems, "skill", publishSkill)
	} else if publishRule != "" {
		toPublish = filterPublishItems(allItems, "rule", publishRule)
	} else if publishCommand != "" {
		toPublish = filterPublishItems(allItems, "command", publishCommand)
	} else {
		// Interactive selection
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

	// Clone/fetch the target remote repo
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}
	subdir := source.NormalizeSource(publishRemote)
	repoDir := filepath.Join(cacheDir, subdir)
	fmt.Printf("Preparing remote repo %s ...\n", publishRemote)
	if err := source.CloneOrFetch(publishRemote, repoDir); err != nil {
		return fmt.Errorf("fetch remote: %w", err)
	}

	// Copy selected assets to the remote repo
	for _, item := range toPublish {
		destDir := filepath.Join(repoDir, item.Name)
		if err := agent.ExportCopyDir(item.Dir, destDir); err != nil {
			return fmt.Errorf("copy %s %s: %w", item.Kind, item.Name, err)
		}
		fmt.Printf("  Prepared %s: %s\n", item.Kind, item.Name)
	}

	msg := fmt.Sprintf("aikit: publish %d asset(s)", len(toPublish))
	if err := publishGitPush(repoDir, msg); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	updatePublishRefs(toPublish)

	fmt.Printf("\n%d asset(s) published to %s\n", len(toPublish), publishRemote)
	return nil
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
			fi, err := os.Lstat(fullPath)
			if err != nil {
				continue
			}
			if fi.Mode()&os.ModeSymlink != 0 {
				continue
			}
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

func publishGitPush(repoDir, message string) error {
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
	return run("push")
}
