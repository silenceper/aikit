package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func init() {
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print aikit version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("aikit version %s (commit: %s, built: %s)\n", version, commit, date)
	},
}
