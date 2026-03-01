package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/silenceper/aikit/internal/source"
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
	Short: "Create .aikit.yaml interactively or from a remote/local config",
	Long: `Initialize a new project with .aikit.yaml.

  aikit init                              Create a blank .aikit.yaml
  aikit init --from user/repo             Import .aikit.yaml from a remote repo
  aikit init --from /path/to/.aikit.yaml  Copy from a local file`,
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
	// Local file: copy directly
	if isLocalFile(from) {
		data, err := os.ReadFile(from)
		if err != nil {
			return fmt.Errorf("read %s: %w", from, err)
		}
		dest := config.ProjectPath(dir)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			return err
		}
		fmt.Printf("Imported .aikit.yaml from %s\n", from)
		fmt.Println("\nRun 'aikit sync' to install assets to your IDEs.")
		return nil
	}

	// Remote repo
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}
	subdir := source.NormalizeSource(from)
	repoDir := filepath.Join(cacheDir, subdir)

	fmt.Printf("Fetching %s ...\n", from)
	if err := source.CloneOrFetch(from, repoDir); err != nil {
		return fmt.Errorf("fetch %s: %w", from, err)
	}

	srcFile := filepath.Join(repoDir, ".aikit.yaml")
	if _, err := os.Stat(srcFile); os.IsNotExist(err) {
		return fmt.Errorf("no .aikit.yaml found in %s", from)
	}

	data, err := os.ReadFile(srcFile)
	if err != nil {
		return err
	}

	dest := config.ProjectPath(dir)
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return err
	}

	fmt.Printf("Imported .aikit.yaml from %s\n", from)
	fmt.Println("\nRun 'aikit sync' to install assets to your IDEs.")
	return nil
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
