package cmd

import (
	"fmt"

	"github.com/silenceper/aikit/internal/catalog"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	catalogCmd.AddCommand(catalogRemoveCmd)
	catalogRemoveCmd.Flags().StringVar(&catalogRemoveSkill, "skill", "", "Skill name to remove")
	catalogRemoveCmd.Flags().StringVar(&catalogRemoveRule, "rule", "", "Rule name to remove")
	catalogRemoveCmd.Flags().StringVar(&catalogRemoveMcp, "mcp", "", "MCP name to remove")
	catalogRemoveCmd.Flags().StringVar(&catalogRemoveCommand, "command", "", "Command name to remove")
}

var catalogRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove an asset from the global catalog",
	RunE:  runCatalogRemove,
}

var catalogRemoveSkill, catalogRemoveRule, catalogRemoveMcp, catalogRemoveCommand string

func runCatalogRemove(cmd *cobra.Command, args []string) error {
	hasFlag := catalogRemoveSkill != "" || catalogRemoveRule != "" || catalogRemoveMcp != "" || catalogRemoveCommand != ""

	if hasFlag {
		return catalogRemoveByFlags()
	}
	return catalogRemoveInteractive()
}

func catalogRemoveByFlags() error {
	type removeFunc struct {
		kind string
		name string
		fn   func(string) (bool, error)
	}
	ops := []removeFunc{
		{"skill", catalogRemoveSkill, catalog.RemoveSkill},
		{"rule", catalogRemoveRule, catalog.RemoveRule},
		{"mcp", catalogRemoveMcp, catalog.RemoveMCP},
		{"command", catalogRemoveCommand, catalog.RemoveCommand},
	}
	for _, op := range ops {
		if op.name == "" {
			continue
		}
		found, err := op.fn(op.name)
		if err != nil {
			return err
		}
		if found {
			fmt.Printf("Removed %s %q from catalog\n", op.kind, op.name)
		} else {
			fmt.Printf("%s %q not found in catalog\n", op.kind, op.name)
		}
	}
	return nil
}

func catalogRemoveInteractive() error {
	cat, err := config.LoadCatalog()
	if err != nil {
		return err
	}

	var items []tui.CatalogItem
	for _, e := range cat.Skills {
		items = append(items, tui.CatalogItem{Kind: "skill", Name: e.Name, Source: e.Source, Desc: e.Description, Group: e.Group})
	}
	for _, e := range cat.Rules {
		items = append(items, tui.CatalogItem{Kind: "rule", Name: e.Name, Source: e.Source, Desc: e.Description, Group: e.Group})
	}
	for _, e := range cat.Mcps {
		items = append(items, tui.CatalogItem{Kind: "mcp", Name: e.Name, Source: e.Source, Desc: e.Description, Group: e.Group})
	}
	for _, e := range cat.Commands {
		items = append(items, tui.CatalogItem{Kind: "command", Name: e.Name, Source: e.Source, Desc: e.Description, Group: e.Group})
	}

	if len(items) == 0 {
		fmt.Println("Catalog is empty, nothing to remove.")
		return nil
	}

	selected, err := tui.SelectCatalogItemsToRemove(items)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		fmt.Println("Nothing selected.")
		return nil
	}

	removeFns := map[string]func(string) (bool, error){
		"skill":   catalog.RemoveSkill,
		"rule":    catalog.RemoveRule,
		"mcp":     catalog.RemoveMCP,
		"command": catalog.RemoveCommand,
	}

	for _, item := range selected {
		fn := removeFns[item.Kind]
		if fn == nil {
			continue
		}
		found, err := fn(item.Name)
		if err != nil {
			return err
		}
		if found {
			fmt.Printf("Removed %s %q from catalog\n", item.Kind, item.Name)
		}
	}
	fmt.Printf("\n%d asset(s) removed from catalog.\n", len(selected))
	return nil
}
