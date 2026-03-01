package cmd

import (
	"fmt"
	"os"

	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().StringVarP(&listProjectDir, "dir", "C", ".", "Project directory")
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List assets in the current project (.aikit.yaml)",
	RunE:  runList,
}

var listProjectDir string

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadProject(listProjectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no .aikit.yaml found; run 'aikit init' first")
		}
		return err
	}
	fmt.Println("Project:", cfg.Project.Name)
	if len(cfg.Project.Targets) > 0 {
		fmt.Println("Targets:", cfg.Project.Targets)
	}
	fmt.Println("\nAssets:")
	for _, s := range cfg.Assets.Skills {
		fmt.Printf("  skill    %s (source: %s)\n", s.Name, s.Source)
	}
	for _, r := range cfg.Assets.Rules {
		fmt.Printf("  rule     %s (source: %s)\n", r.Name, r.Source)
	}
	for _, m := range cfg.Assets.Mcps {
		fmt.Printf("  mcp      %s (source: %s)\n", m.Name, m.Source)
	}
	for _, c := range cfg.Assets.Commands {
		fmt.Printf("  command  %s (source: %s)\n", c.Name, c.Source)
	}
	if len(cfg.LocalRules) > 0 {
		fmt.Printf("\nLocal Rules: %d\n", len(cfg.LocalRules))
		for _, lr := range cfg.LocalRules {
			name := lr.Name
			if name == "" {
				name = "(unnamed)"
			}
			fmt.Printf("  rule     %s (inline)\n", name)
		}
	}
	total := len(cfg.Assets.Skills) + len(cfg.Assets.Rules) + len(cfg.Assets.Mcps) + len(cfg.Assets.Commands)
	if total == 0 && len(cfg.LocalRules) == 0 {
		fmt.Println("  (no assets configured)")
	}
	return nil
}
