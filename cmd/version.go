package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "dev"
var commit = "none"
var date = "unknown"

func versionCommand() *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print aikit version", RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "aikit version %s (commit: %s, built: %s)\n", version, commit, date)
		return err
	}}
}
