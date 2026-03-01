package cmd

import (
	"fmt"
	"os"

	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/internal/tui"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)
	removeCmd.Flags().StringVar(&removeSkill, "skill", "", "Skill name to remove")
	removeCmd.Flags().StringVar(&removeRule, "rule", "", "Rule name to remove")
	removeCmd.Flags().StringVar(&removeMcp, "mcp", "", "MCP name to remove")
	removeCmd.Flags().StringVar(&removeCommand, "command", "", "Command name to remove")
	removeCmd.Flags().StringVarP(&removeDir, "dir", "C", ".", "Project directory")
}

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove an asset from the current project (.aikit.yaml)",
	RunE:  runRemove,
}

var removeSkill, removeRule, removeMcp, removeCommand, removeDir string

func runRemove(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadProject(removeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no .aikit.yaml in %s", removeDir)
		}
		return err
	}

	hasFlag := removeSkill != "" || removeRule != "" || removeMcp != "" || removeCommand != ""
	if hasFlag {
		return removeByFlags(cfg)
	}
	return removeInteractive(cfg)
}

func removeByFlags(cfg *config.ProjectConfig) error {
	type removeOp struct {
		kind string
		name string
		list *[]asset.AssetRef
	}
	ops := []removeOp{
		{"skill", removeSkill, &cfg.Assets.Skills},
		{"rule", removeRule, &cfg.Assets.Rules},
		{"mcp", removeMcp, &cfg.Assets.Mcps},
		{"command", removeCommand, &cfg.Assets.Commands},
	}

	changed := false
	for _, op := range ops {
		if op.name == "" {
			continue
		}
		before := len(*op.list)
		*op.list = filterByName(*op.list, op.name)
		if len(*op.list) < before {
			fmt.Printf("Removed %s %q from project\n", op.kind, op.name)
			changed = true
		} else {
			fmt.Printf("%s %q not found in project\n", op.kind, op.name)
		}
	}
	if changed {
		return config.SaveProject(removeDir, cfg)
	}
	return nil
}

func removeInteractive(cfg *config.ProjectConfig) error {
	var items []tui.CatalogItem
	for _, r := range cfg.Assets.Skills {
		items = append(items, tui.CatalogItem{Kind: "skill", Name: r.Name, Source: r.Source})
	}
	for _, r := range cfg.Assets.Rules {
		items = append(items, tui.CatalogItem{Kind: "rule", Name: r.Name, Source: r.Source})
	}
	for _, r := range cfg.Assets.Mcps {
		items = append(items, tui.CatalogItem{Kind: "mcp", Name: r.Name, Source: r.Source})
	}
	for _, r := range cfg.Assets.Commands {
		items = append(items, tui.CatalogItem{Kind: "command", Name: r.Name, Source: r.Source})
	}

	if len(items) == 0 {
		fmt.Println("No assets in project, nothing to remove.")
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

	for _, item := range selected {
		switch item.Kind {
		case "skill":
			cfg.Assets.Skills = filterByName(cfg.Assets.Skills, item.Name)
		case "rule":
			cfg.Assets.Rules = filterByName(cfg.Assets.Rules, item.Name)
		case "mcp":
			cfg.Assets.Mcps = filterByName(cfg.Assets.Mcps, item.Name)
		case "command":
			cfg.Assets.Commands = filterByName(cfg.Assets.Commands, item.Name)
		}
		fmt.Printf("Removed %s %q from project\n", item.Kind, item.Name)
	}

	if err := config.SaveProject(removeDir, cfg); err != nil {
		return err
	}
	fmt.Printf("\n%d asset(s) removed from project.\n", len(selected))
	return nil
}

func filterByName(refs []asset.AssetRef, name string) []asset.AssetRef {
	var out []asset.AssetRef
	for _, r := range refs {
		if r.Name != name {
			out = append(out, r)
		}
	}
	return out
}
