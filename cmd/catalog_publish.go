package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	catalogCmd.AddCommand(catalogPublishCmd)
	catalogPublishCmd.Flags().StringVar(&catalogPublishRemote, "remote", "", "Target repository (e.g. user/repo or full URL)")
	catalogPublishCmd.MarkFlagRequired("remote")
	catalogPublishCmd.Flags().StringVar(&catalogPublishSkill, "skill", "", "Push only this skill")
	catalogPublishCmd.Flags().StringVar(&catalogPublishMcp, "mcp", "", "Push only this MCP")
}

var catalogPublishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Push local (_local) assets to a remote Git repository",
	Long:  "Use --remote to specify the target repo (e.g. user/repo). Optionally --skill/--mcp to push only specific assets.",
	RunE:  runCatalogPublish,
}

var catalogPublishRemote, catalogPublishSkill, catalogPublishMcp string

func runCatalogPublish(cmd *cobra.Command, args []string) error {
	if catalogPublishRemote == "" {
		return fmt.Errorf("--remote is required (e.g. aikit catalog publish --remote user/repo)")
	}

	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}

	// Clone or fetch the remote repo
	subdir := source.NormalizeSource(catalogPublishRemote)
	repoDir := filepath.Join(cacheDir, subdir)
	fmt.Printf("Preparing remote repo %s ...\n", catalogPublishRemote)
	if err := source.CloneOrFetch(catalogPublishRemote, repoDir); err != nil {
		return fmt.Errorf("fetch remote: %w", err)
	}

	localBase := filepath.Join(cacheDir, "_local")
	cat, err := config.LoadCatalog()
	if err != nil {
		return err
	}

	published := 0

	// Copy skill files from _local to remote repo
	for i, entry := range cat.Skills {
		if entry.Source != config.LocalSourceID() {
			continue
		}
		if catalogPublishSkill != "" && entry.Name != catalogPublishSkill {
			continue
		}
		srcDir := filepath.Join(localBase, "skill", entry.Name)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			fmt.Printf("  Skip skill %s: local files not found\n", entry.Name)
			continue
		}
		destDir := filepath.Join(repoDir, entry.Name)
		if err := agent.ExportCopyDir(srcDir, destDir); err != nil {
			return fmt.Errorf("copy skill %s: %w", entry.Name, err)
		}
		cat.Skills[i].Source = catalogPublishRemote
		published++
		fmt.Printf("  Published skill: %s\n", entry.Name)
	}

	for i, entry := range cat.Mcps {
		if entry.Source != config.LocalSourceID() {
			continue
		}
		if catalogPublishMcp != "" && entry.Name != catalogPublishMcp {
			continue
		}
		srcDir := filepath.Join(localBase, "mcp", entry.Name)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			fmt.Printf("  Skip MCP %s: local files not found\n", entry.Name)
			continue
		}
		destDir := filepath.Join(repoDir, entry.Name)
		if err := agent.ExportCopyDir(srcDir, destDir); err != nil {
			return fmt.Errorf("copy MCP %s: %w", entry.Name, err)
		}
		cat.Mcps[i].Source = catalogPublishRemote
		published++
		fmt.Printf("  Published MCP: %s\n", entry.Name)
	}

	if published == 0 {
		fmt.Println("No local assets found to publish.")
		return nil
	}

	// Git add + commit + push
	if err := gitCommitAndPush(repoDir, fmt.Sprintf("aikit: publish %d asset(s)", published)); err != nil {
		return fmt.Errorf("git push: %w", err)
	}

	// Update catalog to replace _local source with remote
	if err := config.SaveCatalog(cat); err != nil {
		return fmt.Errorf("update catalog: %w", err)
	}

	fmt.Printf("\n%d asset(s) published to %s. Catalog updated.\n", published, catalogPublishRemote)
	return nil
}

func gitCommitAndPush(repoDir, message string) error {
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
	if err := run("commit", "-m", message); err != nil {
		return err
	}
	return run("push")
}
