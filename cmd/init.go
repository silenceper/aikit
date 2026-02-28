package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVar(&initFrom, "from", "", "Initialize from remote repo (e.g. user/repo) or URL to .aikit.yaml")
	initCmd.Flags().StringVarP(&initProjectDir, "dir", "C", ".", "Project directory")
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create .aikit.yaml interactively or from a remote config",
	Long:  "Run the init wizard to pick IDEs, skills, and MCPs from your catalog, or use --from to clone a remote .aikit.yaml.",
	RunE:  runInit,
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
	fmt.Println("  aikit add <source>        Add assets from a remote repo (interactive selection)")
	fmt.Println("  aikit sync                Sync assets to your IDEs")
	return nil
}

func runInitFrom(from, dir string) error {
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

	// Look for .aikit.yaml in the fetched repo
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
