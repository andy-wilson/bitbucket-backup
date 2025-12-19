// Package cmd implements the CLI commands for bb-backup.
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// Build information, set via ldflags.
var (
	version   = "dev"
	commit    = "unknown"
	buildTime = "unknown"
)

// SetVersionInfo sets the version information from ldflags.
func SetVersionInfo(v, c, b string) {
	version = v
	commit = c
	buildTime = b
}

// Global flags
var (
	cfgFile   string
	workspace string
	verbose   bool
	quiet     bool
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "bb-backup",
	Short: "Backup Bitbucket Cloud workspaces",
	Long: `bb-backup is a CLI tool to backup Bitbucket Cloud workspaces.

It backs up git repositories and metadata including projects, pull requests,
issues, and comments. Supports both full and incremental backups.

Examples:
  bb-backup backup -c config.yaml
  bb-backup backup -w my-workspace -o /backups --username user --app-password $TOKEN
  bb-backup backup --dry-run
  bb-backup list -w my-workspace`,
	SilenceUsage: true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ./bb-backup.yaml)")
	rootCmd.PersistentFlags().StringVarP(&workspace, "workspace", "w", "", "workspace to backup (overrides config)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose logging")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "quiet mode (errors only)")
}

// getConfigPath returns the config file path, using default if not specified.
func getConfigPath() string {
	if cfgFile != "" {
		return cfgFile
	}

	// Check for default config file
	defaultPaths := []string{
		"bb-backup.yaml",
		"bb-backup.yml",
		".bb-backup.yaml",
		".bb-backup.yml",
	}

	for _, p := range defaultPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	return ""
}
