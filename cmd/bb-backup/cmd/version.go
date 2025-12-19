package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Print the version, commit hash, and build time of bb-backup.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("bb-backup %s\n", version)
		fmt.Printf("  commit:  %s\n", commit)
		fmt.Printf("  built:   %s\n", buildTime)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
