package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/silenceper/aikit/internal/agent"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configAgentsCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configuration and supported agents",
}

var configAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List supported IDE/agent adapters and detect local project usage",
	RunE:  runConfigAgents,
}

var configAgentsDir string

func init() {
	configAgentsCmd.Flags().StringVarP(&configAgentsDir, "dir", "C", ".", "Project directory to check")
}

func runConfigAgents(cmd *cobra.Command, args []string) error {
	projectDir, _ := filepath.Abs(configAgentsDir)

	fmt.Println("Supported agents:")
	detected := 0
	for _, a := range agent.All() {
		status := ""
		if a.Detect(projectDir) {
			status = " (detected)"
			detected++
		}
		fmt.Printf("  - %s%s\n", a.Name(), status)
	}
	if detected > 0 {
		fmt.Printf("\n%d agent(s) detected in %s\n", detected, projectDir)
	}
	fmt.Println("\nUse 'aikit sync --target <agent>' or run 'aikit sync' for interactive selection.")
	return nil
}
