package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "aikit",
	Short: "Manage AI dev assets. Sync once, align everyone.",
	Long: `aikit lets you share and sync AI dev assets (Skills, Rules, MCP, Commands)
so your team stays aligned. One .aikit.yaml, one aikit sync—everyone gets the same setup.
See https://github.com/silenceper/aikit`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
