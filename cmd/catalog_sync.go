package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	catalogCmd.AddCommand(catalogSyncCmd)
	catalogSyncCmd.Flags().StringVar(&catalogSyncRemote, "remote", "", "Remote repository for catalog sync (e.g. user/my-catalog)")
}

var catalogSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync global catalog to/from a remote Git repository",
	Long: `Sync catalog.yaml and local assets (skills/, rules/, commands/) to a private
remote repository for multi-device backup and restore.

First time:
  aikit catalog sync --remote user/my-catalog

Subsequent syncs (remote is remembered):
  aikit catalog sync`,
	RunE: runCatalogSync,
}

var catalogSyncRemote string

const catalogRemoteFile = ".catalog-remote"

func runCatalogSync(cmd *cobra.Command, args []string) error {
	aikitHome, err := config.AikitHome()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(aikitHome, 0755); err != nil {
		return err
	}

	remotePath := filepath.Join(aikitHome, catalogRemoteFile)

	if catalogSyncRemote != "" {
		if err := initCatalogRepo(aikitHome, catalogSyncRemote); err != nil {
			return err
		}
		if err := os.WriteFile(remotePath, []byte(catalogSyncRemote), 0644); err != nil {
			return err
		}
		fmt.Printf("Remote set to: %s\n", catalogSyncRemote)
	}

	remoteData, err := os.ReadFile(remotePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no remote configured; run 'aikit catalog sync --remote <repo>' first")
		}
		return err
	}
	remote := strings.TrimSpace(string(remoteData))
	if remote == "" {
		return fmt.Errorf("no remote configured; run 'aikit catalog sync --remote <repo>' first")
	}
	fmt.Printf("Using remote: %s\n", remote)

	// Stage all local tracked files first
	fmt.Println("Staging local changes...")
	catalogGitQuiet(aikitHome, "add", "-A")

	// Commit local changes (so rebase can work)
	if catalogGitQuiet(aikitHome, "diff", "--cached", "--quiet") != nil {
		fmt.Println("Committing local changes...")
		if err := catalogGit(aikitHome, "commit", "-m", "aikit: sync catalog"); err != nil {
			return fmt.Errorf("git commit failed: %w", err)
		}
	}

	// Fetch + rebase on top of remote
	fmt.Println("Syncing with remote...")
	catalogGitQuiet(aikitHome, "fetch", "origin")
	if catalogGitQuiet(aikitHome, "rev-parse", "origin/main") == nil {
		if err := catalogGit(aikitHome, "rebase", "--allow-empty", "--strategy-option=theirs", "origin/main"); err != nil {
			// Abort failed rebase and try a simpler merge
			catalogGitQuiet(aikitHome, "rebase", "--abort")
			fmt.Println("  Rebase conflict, falling back to merge...")
			if err := catalogGit(aikitHome, "merge", "--allow-unrelated-histories", "-X", "ours", "origin/main", "-m", "aikit: merge remote catalog"); err != nil {
				return fmt.Errorf("merge failed: %w", err)
			}
		}
	} else {
		fmt.Println("  Remote is empty, skipping pull.")
	}

	// Push
	fmt.Println("Pushing to remote...")
	if err := catalogGit(aikitHome, "push", "-u", "origin", "main"); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	fmt.Println("\nCatalog synced successfully.")
	return nil
}

func initCatalogRepo(aikitHome, remote string) error {
	gitDir := filepath.Join(aikitHome, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		fmt.Println("Initializing catalog repository...")
		if err := catalogGit(aikitHome, "init"); err != nil {
			return fmt.Errorf("git init: %w", err)
		}
		// Set default branch to main
		catalogGit(aikitHome, "branch", "-M", "main")
	}

	// Write .gitignore to exclude cache/
	gitignorePath := filepath.Join(aikitHome, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte("cache/\n"), 0644); err != nil {
		return err
	}

	// Set remote (add or update)
	remoteURL := source.ToGitURL(remote)
	if err := catalogGitQuiet(aikitHome, "remote", "add", "origin", remoteURL); err != nil {
		// Already exists, update URL
		catalogGitQuiet(aikitHome, "remote", "set-url", "origin", remoteURL)
	}

	// Disable GPG signing for this repo
	catalogGitQuiet(aikitHome, "config", "commit.gpgSign", "false")

	return nil
}

func catalogGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// catalogGitQuiet runs git without printing output (for setup commands).
func catalogGitQuiet(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	return cmd.Run()
}
