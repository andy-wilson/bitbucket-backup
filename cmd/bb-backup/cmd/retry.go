package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/andy-wilson/bb-backup/internal/backup"
	"github.com/andy-wilson/bb-backup/internal/logging"
	"github.com/spf13/cobra"
)

var (
	retryMaxRetry int
	retryClear    bool
)

var retryCmd = &cobra.Command{
	Use:   "retry-failed",
	Short: "Retry backup for previously failed repositories",
	Long: `Retry backup for repositories that failed in a previous run.

This command reads the state file to find repositories that failed
during the last backup and attempts to back them up again.

Examples:
  bb-backup retry-failed -c config.yaml
  bb-backup retry-failed --retry 3
  bb-backup retry-failed --clear  # Clear failed list without retrying`,
	RunE: runRetryFailed,
}

func init() {
	rootCmd.AddCommand(retryCmd)

	retryCmd.Flags().IntVar(&retryMaxRetry, "retry", 2, "max retry attempts per repo")
	retryCmd.Flags().BoolVar(&retryClear, "clear", false, "clear failed repos list without retrying")
}

func runRetryFailed(_ *cobra.Command, _ []string) error {
	// Load configuration
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Apply CLI overrides
	applyOverrides(cfg)

	// Load state file
	statePath := backup.GetStatePath(cfg.Storage.Path, cfg.Workspace)
	state, err := backup.LoadState(statePath)
	if err != nil {
		return fmt.Errorf("loading state file: %w", err)
	}
	if state == nil {
		return fmt.Errorf("no state file found at %s", statePath)
	}

	// Check for failed repos
	failedRepos := state.GetFailedRepos()
	if len(failedRepos) == 0 {
		fmt.Println("No failed repositories found in state file.")
		return nil
	}

	fmt.Printf("Found %d failed repositories:\n", len(failedRepos))
	for _, repo := range failedRepos {
		fmt.Printf("  - %s (failed at %s): %s\n", repo.Slug, repo.FailedAt, repo.Error)
	}

	// If --clear flag, just clear the list
	if retryClear {
		state.ClearFailedRepos()
		if err := state.Save(statePath); err != nil {
			return fmt.Errorf("saving state file: %w", err)
		}
		fmt.Println("\nCleared failed repositories list.")
		return nil
	}

	fmt.Println("\nRetrying failed repositories...")

	// Set up context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived interrupt, shutting down gracefully...")
		cancel()
	}()

	// Build include list from failed repos
	var includeRepos []string
	for _, repo := range failedRepos {
		includeRepos = append(includeRepos, repo.Slug)
	}

	// Override config to only include failed repos
	cfg.Backup.IncludeRepos = includeRepos
	cfg.Backup.ExcludeRepos = nil

	// Determine effective log level
	effectiveLevel := cfg.Logging.Level
	if verbose {
		effectiveLevel = "debug"
	} else if quiet {
		effectiveLevel = "error"
	}

	// Create logger
	logFile := cfg.Logging.File
	if logFile == "" {
		logFile = filepath.Join(cfg.Storage.Path, "bb-backup-retry.log")
	}
	log, err := logging.New(logging.Config{
		Level:   effectiveLevel,
		Format:  cfg.Logging.Format,
		File:    logFile,
		Console: !quiet,
	})
	if err != nil {
		return fmt.Errorf("initializing logger: %w", err)
	}
	defer func() { _ = log.Close() }()

	// Create and run backup
	opts := backup.Options{
		DryRun:   dryRun,
		Verbose:  log.IsDebug(),
		Quiet:    log.IsQuiet(),
		MaxRetry: retryMaxRetry,
		Logger:   log,
	}

	b, err := backup.New(cfg, opts)
	if err != nil {
		return fmt.Errorf("initializing backup: %w", err)
	}

	if err := b.Run(ctx); err != nil {
		return fmt.Errorf("running retry backup: %w", err)
	}

	return nil
}
