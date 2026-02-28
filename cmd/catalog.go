package cmd

import (
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(catalogCmd)
}

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Manage global asset catalog (~/.aikit/catalog.yaml)",
}
