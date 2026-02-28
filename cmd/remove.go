package cmd

import (
	"fmt"
	"os"

	"github.com/silenceper/aikit/internal/asset"
	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove an asset from the current project (.aikit.yaml)",
	RunE:  runRemove,
}

var (
	removeSkill   string
	removeRule    string
	removeMcp     string
	removeCommand string
	removeDir     string
)

func init() {
	removeCmd.Flags().StringVar(&removeSkill, "skill", "", "Skill name to remove")
	removeCmd.Flags().StringVar(&removeRule, "rule", "", "Rule name to remove")
	removeCmd.Flags().StringVar(&removeMcp, "mcp", "", "MCP name to remove")
	removeCmd.Flags().StringVar(&removeCommand, "command", "", "Command name to remove")
	removeCmd.Flags().StringVarP(&removeDir, "dir", "C", ".", "Project directory")
}

func runRemove(cmd *cobra.Command, args []string) error {
	if removeSkill == "" && removeRule == "" && removeMcp == "" && removeCommand == "" {
		return fmt.Errorf("specify one of --skill, --rule, --mcp, --command")
	}
	cfg, err := config.LoadProject(removeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no .aikit.yaml in %s", removeDir)
		}
		return err
	}
	removed := false
	if removeSkill != "" {
		cfg.Assets.Skills = filterByName(cfg.Assets.Skills, removeSkill)
		removed = true
	}
	if removeRule != "" {
		cfg.Assets.Rules = filterByName(cfg.Assets.Rules, removeRule)
		removed = true
	}
	if removeMcp != "" {
		cfg.Assets.Mcps = filterByName(cfg.Assets.Mcps, removeMcp)
		removed = true
	}
	if removeCommand != "" {
		cfg.Assets.Commands = filterByName(cfg.Assets.Commands, removeCommand)
		removed = true
	}
	if !removed {
		return nil
	}
	return config.SaveProject(removeDir, cfg)
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
