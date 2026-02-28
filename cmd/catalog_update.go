package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/silenceper/aikit/internal/source"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	catalogCmd.AddCommand(catalogUpdateCmd)
}

var catalogUpdateCmd = &cobra.Command{
	Use:   "update [source]",
	Short: "Update cached assets (git pull) for a remote source or all",
	RunE:  runCatalogUpdate,
}

func runCatalogUpdate(cmd *cobra.Command, args []string) error {
	cacheDir, err := config.CacheDir()
	if err != nil {
		return err
	}
	if len(args) > 0 {
		subdir := source.NormalizeSource(args[0])
		if subdir == "" || subdir == "_local" {
			return fmt.Errorf("invalid source for update: %s", args[0])
		}
		dest := filepath.Join(cacheDir, subdir)
		if err := source.CloneOrFetch(args[0], dest); err != nil {
			return err
		}
		fmt.Println("Updated cache for", args[0])
		return nil
	}
	// Update all: load catalog, for each remote source run pull
	cfg, err := config.LoadCatalog()
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, e := range cfg.Skills {
		if e.Source != "" && e.Source != config.LocalSourceID() && !seen[e.Source] {
			seen[e.Source] = true
			subdir := source.NormalizeSource(e.Source)
			dest := filepath.Join(cacheDir, subdir)
			_ = source.CloneOrFetch(e.Source, dest)
			fmt.Println("Updated", e.Source)
		}
	}
	for _, e := range cfg.Mcps {
		if e.Source != "" && e.Source != config.LocalSourceID() && !seen[e.Source] {
			seen[e.Source] = true
			subdir := source.NormalizeSource(e.Source)
			dest := filepath.Join(cacheDir, subdir)
			_ = source.CloneOrFetch(e.Source, dest)
			fmt.Println("Updated", e.Source)
		}
	}
	return nil
}
