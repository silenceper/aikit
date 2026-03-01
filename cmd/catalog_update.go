package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/silenceper/aikit/internal/asset"
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
	cfg, err := config.LoadCatalog()
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	allEntries := make([]asset.CatalogEntry, 0, len(cfg.Skills)+len(cfg.Rules)+len(cfg.Mcps)+len(cfg.Commands))
	allEntries = append(allEntries, cfg.Skills...)
	allEntries = append(allEntries, cfg.Rules...)
	allEntries = append(allEntries, cfg.Mcps...)
	allEntries = append(allEntries, cfg.Commands...)
	for _, e := range allEntries {
		if e.Source == "" || e.Source == config.LocalSourceID() || seen[e.Source] {
			continue
		}
		seen[e.Source] = true
		subdir := source.NormalizeSource(e.Source)
		dest := filepath.Join(cacheDir, subdir)
		if err := source.CloneOrFetch(e.Source, dest); err != nil {
			fmt.Printf("  Warning: failed to update %s: %v\n", e.Source, err)
			continue
		}
		fmt.Println("Updated", e.Source)
	}
	if len(seen) == 0 {
		fmt.Println("No remote sources to update.")
	}
	return nil
}
