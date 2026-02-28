package cmd

import (
	"fmt"
	"os"

	"github.com/silenceper/aikit/pkg/config"
	"github.com/spf13/cobra"
)


var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List assets in the current project (.aikit.yaml)",
	RunE:  runList,
}

var listProjectDir string

func init() {
	rootCmd.AddCommand(listCmd)
	listCmd.Flags().StringVarP(&listProjectDir, "dir", "C", ".", "Project directory")
}

func runList(cmd *cobra.Command, args []string) error {
	cfg, err := config.LoadProject(listProjectDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "No .aikit.yaml found in the current directory. Run 'aikit init' first.")
			os.Exit(1)
		}
		return err
	}
	fmt.Println("Project:", cfg.Project.Name)
	if len(cfg.Project.Targets) > 0 {
		fmt.Println("Targets:", cfg.Project.Targets)
	}
	fmt.Println("\nAssets:")
	for _, s := range cfg.Assets.Skills {
		fmt.Printf("  skill   %s (source: %s)\n", s.Name, s.Source)
	}
	for _, r := range cfg.Assets.Rules {
		fmt.Printf("  rule   %s (source: %s)\n", r.Name, r.Source)
	}
	for _, m := range cfg.Assets.Mcps {
		fmt.Printf("  mcp    %s (source: %s)\n", m.Name, m.Source)
	}
	for _, c := range cfg.Assets.Commands {
		fmt.Printf("  command %s (source: %s)\n", c.Name, c.Source)
	}
	return nil
}
