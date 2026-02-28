package cmd

import (
	"fmt"

	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	catalogCmd.AddCommand(catalogListCmd)
}

var catalogListCmd = &cobra.Command{
	Use:   "list",
	Short: "List catalog entries by group",
	RunE:  runCatalogList,
}

func runCatalogList(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadCatalog()
	if err != nil {
		return err
	}
	printEntries("Skills", cfg.Skills)
	printEntries("Rules", cfg.Rules)
	printEntries("MCPs", cfg.Mcps)
	printEntries("Commands", cfg.Commands)
	return nil
}

func printEntries(kind string, entries []asset.CatalogEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Printf("\n%s:\n", kind)
	grouped := make(map[string][]asset.CatalogEntry)
	for _, e := range entries {
		g := e.Group
		if g == "" {
			g = "Ungrouped"
		}
		grouped[g] = append(grouped[g], e)
	}
	for g, list := range grouped {
		fmt.Printf("  %s\n", g)
		for _, e := range list {
			desc := truncate(e.Description, 60)
			if desc != "" {
				fmt.Printf("    - %s — %s (source: %s)\n", e.Name, desc, e.Source)
			} else {
				fmt.Printf("    - %s (source: %s)\n", e.Name, e.Source)
			}
		}
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
