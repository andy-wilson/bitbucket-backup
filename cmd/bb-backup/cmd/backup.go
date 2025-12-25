package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/andy-wilson/bb-backup/internal/backup"
	"github.com/andy-wilson/bb-backup/internal/config"
	"github.com/spf13/cobra"
)

var (
	outputDir       string
	fullBackup      bool
	incrementalOnly bool
	dryRun          bool
	parallel        int
	username        string
	appPassword     string
	jsonProgress    bool
	excludeRepos    []string
	includeRepos    []string
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Run a backup of the workspace",
	Long: `Run a backup of the configured Bitbucket workspace.

This will backup:
  - Workspace metadata
  - All projects and their metadata
  - All repositories (git mirror clone)
  - Pull requests, comments, and activity
  - Issues and comments

Backup modes:
  --full         Force a full backup (ignore previous state)
  --incremental  Force incremental (fail if no previous state)
  (default)      Auto-detect: incremental if state exists, full otherwise

Progress output:
  --json-progress  Output progress as JSON lines (for automation)
  --quiet          Suppress progress output
  --verbose        Show detailed debug output

Repository filtering:
  --include "pattern"  Only include repos matching glob pattern
  --exclude "pattern"  Exclude repos matching glob pattern
  Patterns support * and ? wildcards (e.g., "core-*", "test-?-*")

Examples:
  bb-backup backup -c config.yaml
  bb-backup backup -w my-workspace -o /backups
  bb-backup backup --dry-run
  bb-backup backup --full
  bb-backup backup --incremental
  bb-backup backup --exclude "test-*" --exclude "archive-*"
  bb-backup backup --include "core-*" --include "platform-*"`,
	RunE: runBackup,
}

func init() {
	rootCmd.AddCommand(backupCmd)

	backupCmd.Flags().StringVarP(&outputDir, "output", "o", "", "output directory (overrides config)")
	backupCmd.Flags().BoolVar(&fullBackup, "full", false, "force full backup")
	backupCmd.Flags().BoolVar(&incrementalOnly, "incremental", false, "force incremental (fail if no state)")
	backupCmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would be backed up")
	backupCmd.Flags().IntVar(&parallel, "parallel", 0, "parallel repo operations (overrides config)")
	backupCmd.Flags().StringVar(&username, "username", "", "Bitbucket username")
	backupCmd.Flags().StringVar(&appPassword, "app-password", "", "Bitbucket app password")
	backupCmd.Flags().BoolVar(&jsonProgress, "json-progress", false, "output progress as JSON lines")
	backupCmd.Flags().StringArrayVar(&excludeRepos, "exclude", nil, "exclude repos matching glob pattern")
	backupCmd.Flags().StringArrayVar(&includeRepos, "include", nil, "only include repos matching glob pattern")
}

func runBackup(_ *cobra.Command, _ []string) error {
	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Apply CLI overrides
	applyOverrides(cfg)

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		if !jsonProgress {
			fmt.Println("\nReceived interrupt, shutting down gracefully...")
		}
		cancel()
	}()

	// Determine verbosity from CLI flags or config
	effectiveVerbose := verbose || cfg.Logging.Level == "debug"
	effectiveQuiet := quiet || cfg.Logging.Level == "error"

	// Create and run backup
	opts := backup.Options{
		DryRun:       dryRun,
		Full:         fullBackup,
		Incremental:  incrementalOnly,
		Verbose:      effectiveVerbose,
		Quiet:        effectiveQuiet,
		JSONProgress: jsonProgress,
	}

	b, err := backup.New(cfg, opts)
	if err != nil {
		return fmt.Errorf("initializing backup: %w", err)
	}

	if err := b.Run(ctx); err != nil {
		return fmt.Errorf("running backup: %w", err)
	}

	return nil
}

func loadConfig() (*config.Config, error) {
	cfgPath := getConfigPath()

	// If we have a config file, load it
	if cfgPath != "" {
		cfg, err := config.Load(cfgPath)
		if err != nil {
			return nil, fmt.Errorf("loading config from %s: %w", cfgPath, err)
		}
		return cfg, nil
	}

	// No config file - try to build config from CLI flags and env vars
	if workspace == "" {
		workspace = os.Getenv("BITBUCKET_WORKSPACE")
	}
	if workspace == "" {
		return nil, fmt.Errorf("no config file found and --workspace not specified")
	}

	// Build minimal config from flags
	cfg := config.Default()
	cfg.Workspace = workspace

	// Auth from flags or env
	if username == "" {
		username = os.Getenv("BITBUCKET_USERNAME")
	}
	if appPassword == "" {
		appPassword = os.Getenv("BITBUCKET_APP_PASSWORD")
	}

	cfg.Auth.Username = username
	cfg.Auth.AppPassword = appPassword

	if outputDir == "" {
		outputDir = os.Getenv("BITBUCKET_BACKUP_PATH")
		if outputDir == "" {
			outputDir = "./backups"
		}
	}
	cfg.Storage.Path = outputDir

	// Validate the assembled config
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyOverrides(cfg *config.Config) {
	if workspace != "" {
		cfg.Workspace = workspace
	}
	if outputDir != "" {
		cfg.Storage.Path = outputDir
	}
	if username != "" {
		cfg.Auth.Username = username
	}
	if appPassword != "" {
		cfg.Auth.AppPassword = appPassword
	}
	if parallel > 0 {
		cfg.Parallelism.GitWorkers = parallel
	}

	// Apply filter overrides
	if len(excludeRepos) > 0 {
		cfg.Backup.ExcludeRepos = mergePatterns(cfg.Backup.ExcludeRepos, excludeRepos)
	}
	if len(includeRepos) > 0 {
		cfg.Backup.IncludeRepos = mergePatterns(cfg.Backup.IncludeRepos, includeRepos)
	}
}

// mergePatterns merges patterns from config and CLI, avoiding duplicates.
func mergePatterns(existing, additional []string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, p := range existing {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}

	for _, p := range additional {
		p = strings.TrimSpace(p)
		if p != "" && !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}

	return result
}
