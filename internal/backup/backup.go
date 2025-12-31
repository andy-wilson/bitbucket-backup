// Package backup orchestrates the backup process for Bitbucket workspaces.
package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andy-wilson/bb-backup/internal/api"
	"github.com/andy-wilson/bb-backup/internal/config"
	"github.com/andy-wilson/bb-backup/internal/git"
	"github.com/andy-wilson/bb-backup/internal/storage"
)

// bufferPool is a sync.Pool for reusing bytes.Buffer in JSON marshaling.
// This reduces GC pressure when marshaling many JSON files.
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// Options configures the backup behavior.
type Options struct {
	DryRun       bool
	Full         bool
	Incremental  bool
	Verbose      bool
	Quiet        bool
	JSONProgress bool
	Interactive  bool   // Interactive mode with progress bar
	MaxRetry     int    // Maximum retry attempts for failed repos
	Logger       Logger // Optional external logger
	GitOnly      bool   // Only backup git repositories (skip PRs, issues)
	MetadataOnly bool   // Only backup PRs, issues (skip git operations)
}

// Backup orchestrates the backup process.
type Backup struct {
	cfg            *config.Config
	opts           Options
	client         *api.Client
	storage        storage.Storage
	log            Logger
	state          *State
	filter         *RepoFilter
	progress       *Progress
	gitClient      *git.GoGitClient
	shellGitClient *git.ShellGitClient // Fallback for when go-git fails
	shuttingDown   atomic.Bool         // Set when graceful shutdown starts
}

// Logger interface for backup logging.
type Logger interface {
	Info(msg string, args ...interface{})
	Debug(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// defaultLogger is a simple console logger.
type defaultLogger struct {
	verbose bool
	quiet   bool
}

func (l *defaultLogger) Info(msg string, args ...interface{}) {
	if !l.quiet {
		fmt.Printf("[INFO] "+msg+"\n", args...)
	}
}

func (l *defaultLogger) Debug(msg string, args ...interface{}) {
	if l.verbose && !l.quiet {
		fmt.Printf("[DEBUG] "+msg+"\n", args...)
	}
}

func (l *defaultLogger) Error(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[ERROR] "+msg+"\n", args...)
}

// New creates a new Backup instance.
func New(cfg *config.Config, opts Options) (*Backup, error) {
	// Use provided logger or create default (needed before API client)
	var log Logger
	if opts.Logger != nil {
		log = opts.Logger
	} else {
		log = &defaultLogger{
			verbose: opts.Verbose,
			quiet:   opts.Quiet,
		}
	}

	// Log authentication method being used
	log.Debug("Using authentication method: %s", cfg.Auth.Method)

	// Create API client with logging
	clientOpts := []api.ClientOption{
		api.WithLogFunc(log.Debug),
	}
	client := api.NewClient(cfg, clientOpts...)

	store, err := storage.NewLocal(cfg.Storage.Path)
	if err != nil {
		return nil, fmt.Errorf("initializing storage: %w", err)
	}

	// Load existing state for incremental backups
	var state *State
	if !opts.Full {
		statePath := GetStatePath(cfg.Storage.Path, cfg.Workspace)
		state, err = LoadState(statePath)
		if err != nil {
			return nil, fmt.Errorf("loading state: %w", err)
		}
	}

	// If incremental requested but no state, fail
	if opts.Incremental && (state == nil || !state.HasPreviousBackup()) {
		return nil, fmt.Errorf("incremental backup requested but no previous backup state found")
	}

	// Create new state if none exists
	if state == nil {
		state = NewState(cfg.Workspace)
	}

	// Create repo filter with logging
	filter := NewRepoFilterWithLog(cfg.Backup.IncludeRepos, cfg.Backup.ExcludeRepos, log.Debug)

	// Create go-git client with credentials and rate limiting
	gitUser, gitPass := cfg.GetGitCredentials()
	gitClient := git.NewGoGitClient(
		git.WithCredentials(gitUser, gitPass),
		git.WithLogger(log.Debug),
		git.WithRateLimit(client.RateLimiter().Wait),
		git.WithSkipSizeCalc(), // Skip expensive directory size calculation during backup
	)

	// Create shell git client as fallback (may be nil if git CLI not available)
	var shellGitClient *git.ShellGitClient
	if git.IsGitCLIAvailable() {
		shellGitClient = git.NewShellGitClient(
			git.WithShellCredentials(gitUser, gitPass),
			git.WithShellLogger(log.Debug),
		)
		log.Debug("Git CLI available, will use as fallback for go-git failures")
	} else {
		log.Debug("Git CLI not available, no fallback for go-git failures")
	}

	return &Backup{
		cfg:            cfg,
		opts:           opts,
		client:         client,
		storage:        store,
		log:            log,
		state:          state,
		filter:         filter,
		gitClient:      gitClient,
		shellGitClient: shellGitClient,
	}, nil
}

// Run executes the backup process.
func (b *Backup) Run(ctx context.Context) error {
	startTime := time.Now()
	b.log.Info("Starting backup for workspace: %s", b.cfg.Workspace)

	// In interactive mode, print status to console since logs go to file only
	if b.opts.Interactive {
		fmt.Fprintf(os.Stderr, "Starting backup for workspace: %s\n", b.cfg.Workspace)
	}

	if b.opts.DryRun {
		b.log.Info("DRY RUN - no changes will be made")
	}

	if b.opts.Incremental && b.state.HasPreviousBackup() {
		// Use whichever timestamp is more recent
		lastBackup := b.state.LastIncremental
		if b.state.LastFullBackup > lastBackup {
			lastBackup = b.state.LastFullBackup
		}
		b.log.Info("Incremental backup (last: %s)", lastBackup)
		if b.opts.Interactive {
			fmt.Fprintf(os.Stderr, "Mode: incremental (last backup: %s)\n", lastBackup)
		}
	} else {
		b.log.Info("Full backup")
	}

	// Log backup scope
	if b.opts.GitOnly {
		b.log.Info("Git-only mode: skipping PRs, issues, and metadata")
		if b.opts.Interactive {
			fmt.Fprintln(os.Stderr, "Mode: git-only (skipping PRs, issues, metadata)")
		}
	} else if b.opts.MetadataOnly {
		b.log.Info("Metadata-only mode: skipping git operations")
		if b.opts.Interactive {
			fmt.Fprintln(os.Stderr, "Mode: metadata-only (skipping git clone/fetch)")
		}
	}

	// Create backup directory with timestamp
	backupDir := filepath.Join(b.cfg.Workspace, startTime.Format("2006-01-02T15-04-05Z"))

	// Fetch workspace metadata
	b.log.Info("Fetching workspace metadata...")
	if b.opts.Interactive {
		fmt.Fprint(os.Stderr, "Fetching workspace metadata... ")
	}
	workspace, err := b.client.GetWorkspace(ctx, b.cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching workspace: %w", err)
	}
	if b.opts.Interactive {
		fmt.Fprintln(os.Stderr, "done")
	}

	if !b.opts.DryRun {
		if err := b.saveJSON(backupDir, "workspace.json", workspace); err != nil {
			return fmt.Errorf("saving workspace metadata: %w", err)
		}
	}
	b.log.Debug("Workspace: %s (%s)", workspace.Name, workspace.UUID)

	// Fetch projects
	b.log.Info("Fetching projects...")
	if b.opts.Interactive {
		fmt.Fprint(os.Stderr, "Fetching projects... ")
	}
	projects, err := b.client.GetProjects(ctx, b.cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching projects: %w", err)
	}
	if b.opts.Interactive {
		fmt.Fprintf(os.Stderr, "found %d\n", len(projects))
	}
	b.log.Info("Found %d projects", len(projects))

	// Fetch repositories
	var repos []api.Repository

	// Check if we're backing up a single specific repository
	if singleRepoSlug := b.filter.SingleRepoSlug(); singleRepoSlug != "" {
		b.log.Info("Fetching single repository: %s", singleRepoSlug)
		if b.opts.Interactive {
			fmt.Fprintf(os.Stderr, "Fetching repository %s... ", singleRepoSlug)
		}
		repo, err := b.client.GetRepository(ctx, b.cfg.Workspace, singleRepoSlug)
		if err != nil {
			return fmt.Errorf("fetching repository %s: %w", singleRepoSlug, err)
		}
		repos = []api.Repository{*repo}
		if b.opts.Interactive {
			fmt.Fprintln(os.Stderr, "done")
		}
		b.log.Info("Found repository: %s", repo.Slug)
	} else {
		b.log.Info("Fetching repositories...")
		if b.opts.Interactive {
			fmt.Fprint(os.Stderr, "Fetching repositories... ")
		}
		allRepos, err := b.client.GetRepositories(ctx, b.cfg.Workspace)
		if err != nil {
			return fmt.Errorf("fetching repositories: %w", err)
		}

		// Apply filters
		repos = b.filter.Filter(allRepos)
		included, excluded := b.filter.FilteredCount(allRepos)
		if excluded > 0 {
			if b.opts.Interactive {
				fmt.Fprintf(os.Stderr, "found %d (%d excluded)\n", included, excluded)
			}
			b.log.Info("Found %d repositories (%d excluded by filters)", included, excluded)
		} else {
			if b.opts.Interactive {
				fmt.Fprintf(os.Stderr, "found %d\n", len(repos))
			}
			b.log.Info("Found %d repositories", len(repos))
		}
	}

	// Pre-scan to count existing vs new repos
	existingCount, newCount := b.countExistingRepos(backupDir, repos, projects)

	// Initialize progress tracker
	if b.opts.Interactive {
		if existingCount > 0 {
			fmt.Fprintf(os.Stderr, "\nProcessing %d repositories (%d updates, %d new)...\n", len(repos), existingCount, newCount)
		} else {
			fmt.Fprintf(os.Stderr, "\nProcessing %d repositories...\n", len(repos))
		}
	}
	b.progress = NewProgress(len(repos), b.opts.JSONProgress, b.opts.Quiet, b.opts.Interactive)

	// Track stats
	stats := &backupStats{}

	// Process projects
	for _, project := range projects {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("backup cancelled: %w", err)
		}

		b.log.Info("Processing project: %s (%s)", project.Name, project.Key)

		projectDir := filepath.Join(backupDir, "projects", project.Key)

		if !b.opts.DryRun {
			if err := b.saveJSON(projectDir, "project.json", project); err != nil {
				return fmt.Errorf("saving project %s metadata: %w", project.Key, err)
			}
			b.state.UpdateProject(project.Key, project.UUID)
		}
		stats.Projects++
	}

	// Process repositories with parallel workers
	if err := b.processRepositories(ctx, backupDir, repos, projects, stats); err != nil {
		return err
	}

	// Save state file
	if !b.opts.DryRun {
		if b.opts.Full || !b.state.HasPreviousBackup() {
			b.state.MarkFullBackup()
			b.log.Debug("State: marked full backup complete")
		} else {
			b.state.MarkIncrementalBackup()
			b.log.Debug("State: marked incremental backup complete")
		}

		statePath := GetStatePath(b.cfg.Storage.Path, b.cfg.Workspace)
		b.log.Debug("State: saving to %s (%d projects, %d repos)",
			statePath, len(b.state.Projects), len(b.state.Repositories))
		if err := b.state.Save(statePath); err != nil {
			b.log.Error("Failed to save state file: %v", err)
		}
	}

	// Generate manifest
	if !b.opts.DryRun {
		manifest := b.createManifest(startTime, stats)
		if err := b.saveJSON(backupDir, "manifest.json", manifest); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}
	}

	// Print summary
	elapsed := time.Since(startTime)
	b.log.Info("Backup completed in %s", elapsed.Round(time.Second))
	if stats.Interrupted > 0 {
		b.log.Info("Stats: %d projects, %d repos, %d PRs, %d issues, %d failed, %d interrupted",
			stats.Projects, stats.Repos, stats.PullRequests, stats.Issues, stats.Failed, stats.Interrupted)
	} else {
		b.log.Info("Stats: %d projects, %d repos, %d PRs, %d issues, %d failed",
			stats.Projects, stats.Repos, stats.PullRequests, stats.Issues, stats.Failed)
	}

	if b.progress != nil {
		b.progress.Summary()
	}

	// List failed repos if any
	if stats.Failed > 0 {
		failedRepos := b.state.GetFailedRepos()
		if len(failedRepos) > 0 {
			var names []string
			for _, fr := range failedRepos {
				names = append(names, fr.Slug)
			}
			b.log.Info("Failed repos: %s", strings.Join(names, ", "))
			if b.opts.Interactive {
				fmt.Fprintf(os.Stderr, "Failed repos: %s\n", strings.Join(names, ", "))
			}
		}
	}

	return nil
}

// processRepositories processes all repositories with parallel workers.
func (b *Backup) processRepositories(ctx context.Context, backupDir string, repos []api.Repository, projects []api.Project, stats *backupStats) error {
	b.log.Debug("processRepositories: starting with %d repos", len(repos))

	// Group repos by project
	reposByProject := make(map[string][]api.Repository)
	var personalRepos []api.Repository

	for _, repo := range repos {
		if repo.Project != nil {
			reposByProject[repo.Project.Key] = append(reposByProject[repo.Project.Key], repo)
		} else {
			personalRepos = append(personalRepos, repo)
		}
	}
	b.log.Debug("processRepositories: %d project repos, %d personal repos", len(repos)-len(personalRepos), len(personalRepos))

	// Create worker pool
	workers := b.cfg.Parallelism.GitWorkers
	if workers < 1 {
		workers = 1
	}
	totalJobs := len(repos)
	b.log.Debug("processRepositories: starting worker pool with %d workers for %d jobs (max retry: %d)", workers, totalJobs, b.opts.MaxRetry)
	pool := newWorkerPool(workers, totalJobs, b.opts.MaxRetry, b.log.Debug)
	pool.start(ctx, b)

	// Submit jobs for project repos
	jobCount := 0
	for _, project := range projects {
		projectDir := filepath.Join(backupDir, "projects", project.Key)
		for _, repo := range reposByProject[project.Key] {
			jobID := generateJobID()
			b.log.Debug("[%s] Submitting job for %s (project: %s)", jobID, repo.Slug, project.Key)
			pool.submit(repoJob{
				baseDir:  projectDir,
				repo:     &repo,
				maxRetry: b.opts.MaxRetry,
				jobID:    jobID,
			})
			jobCount++
		}
	}

	// Submit jobs for personal repos
	personalDir := filepath.Join(backupDir, "personal")
	for _, repo := range personalRepos {
		jobID := generateJobID()
		b.log.Debug("[%s] Submitting job for %s (personal)", jobID, repo.Slug)
		pool.submit(repoJob{
			baseDir:  personalDir,
			repo:     &repo,
			maxRetry: b.opts.MaxRetry,
			jobID:    jobID,
		})
		jobCount++
	}

	b.log.Debug("processRepositories: submitted %d jobs, closing job channel", jobCount)
	// Close jobs channel and collect results
	pool.close()

	// Start periodic stats logging
	statsCtx, statsCancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-statsCtx.Done():
				return
			case <-ticker.C:
				b.log.Debug("processRepositories: pool stats - %s", pool.stats())
			}
		}
	}()

	// Collect results in a separate goroutine
	b.log.Debug("processRepositories: starting result collector")
	done := make(chan struct{})
	resultCount := 0
	statePath := GetStatePath(b.cfg.Storage.Path, b.cfg.Workspace)
	go func() {
		for result := range pool.results {
			pool.markResultRead()
			resultCount++
			b.log.Debug("processRepositories: received result %d/%d for %s", resultCount, jobCount, result.repo.Slug)
			if result.err != nil {
				// Check if this was just an interrupt/cancellation (not a real failure)
				if isContextCanceled(result.err) {
					stats.Interrupted++
					// Don't log each interrupted repo - just count them silently
					// Don't update progress bar during shutdown (already stopped)
					continue
				}

				// Only log real errors if not shutting down
				if !b.shuttingDown.Load() {
					b.log.Error("Failed to backup repo %s: %v", result.repo.Slug, result.err)
				}
				stats.Failed++

				// Track failed repo in state
				projectKey := ""
				if result.repo.Project != nil {
					projectKey = result.repo.Project.Key
				}
				b.state.AddFailedRepo(result.repo.Slug, projectKey, result.err.Error(), b.opts.MaxRetry+1)

				if !b.shuttingDown.Load() && b.progress != nil {
					b.progress.Fail(result.repo.Slug, result.err)
				}
			} else {
				stats.Repos++
				stats.PullRequests += result.stats.PullRequests
				stats.Issues += result.stats.Issues

				// Update state and remove from failed list if previously failed
				projectKey := ""
				if result.repo.Project != nil {
					projectKey = result.repo.Project.Key
				}
				b.state.UpdateRepository(result.repo.Slug, result.repo.UUID, projectKey)
				b.state.RemoveFailedRepo(result.repo.Slug) // Clear from failed list on success

				if !b.shuttingDown.Load() && b.progress != nil {
					b.progress.Complete(result.repo.Slug)
				}
			}

			// Periodic state checkpoint for crash recovery
			if !b.opts.DryRun && resultCount%CheckpointInterval == 0 {
				if err := b.state.Save(statePath); err != nil {
					b.log.Debug("State checkpoint failed: %v", err)
				} else {
					b.log.Debug("State checkpoint saved (%d repos processed)", resultCount)
				}
			}
		}
		b.log.Debug("processRepositories: result collector finished, received %d results", resultCount)
		close(done)
	}()

	// Wait for workers to finish (with timeout if context cancelled)
	b.log.Debug("processRepositories: waiting for workers to finish...")

	waitDone := make(chan struct{})
	go func() {
		pool.wait()
		close(waitDone)
	}()

	// If context is cancelled, wait max 5 seconds for graceful shutdown
	select {
	case <-waitDone:
		b.log.Debug("processRepositories: workers finished normally")
	case <-ctx.Done():
		// Signal shutdown mode - suppresses noisy error logging
		b.shuttingDown.Store(true)

		// Stop the progress bar immediately to avoid noise
		if b.progress != nil && b.progress.progressBar != nil {
			b.progress.progressBar.Stop()
		}

		b.log.Debug("processRepositories: context cancelled, waiting up to 5s for workers...")
		select {
		case <-waitDone:
			b.log.Debug("processRepositories: workers finished after cancellation")
		case <-time.After(5 * time.Second):
			b.log.Debug("processRepositories: timeout waiting for workers, forcing shutdown")
			// Force close results channel so result collector can exit
			pool.closeResults()
		}
	}

	b.log.Debug("processRepositories: waiting for result collector...")
	// Give result collector a moment to finish
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		b.log.Debug("processRepositories: timeout waiting for result collector")
	}

	// Stop stats logging
	statsCancel()

	// Log final stats
	b.log.Debug("processRepositories: complete - final stats: %s", pool.stats())

	return nil
}

func (b *Backup) saveJSON(dir, filename string, data interface{}) error {
	// Get buffer from pool
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)

	// Use json.Encoder for streaming marshaling
	encoder := json.NewEncoder(buf)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	fullPath := filepath.Join(dir, filename)
	b.log.Debug("Writing %s (%s)", fullPath, formatBytes(int64(buf.Len())))

	return b.storage.Write(fullPath, buf.Bytes())
}

// formatBytes formats a byte count as a human-readable string.
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMG"[exp])
}

func (b *Backup) createManifest(startTime time.Time, stats *backupStats) *Manifest {
	return &Manifest{
		Version:     "1.0",
		Workspace:   b.cfg.Workspace,
		StartedAt:   startTime.UTC().Format(time.RFC3339),
		CompletedAt: time.Now().UTC().Format(time.RFC3339),
		Stats: ManifestStats{
			Projects:     stats.Projects,
			Repositories: stats.Repos,
			PullRequests: stats.PullRequests,
			Issues:       stats.Issues,
			Failed:       stats.Failed,
		},
		Options: ManifestOptions{
			Full:        b.opts.Full,
			Incremental: b.opts.Incremental,
			DryRun:      b.opts.DryRun,
		},
	}
}

type backupStats struct {
	Projects     int
	Repos        int
	PullRequests int
	Issues       int
	Failed       int
	Interrupted  int
}

// isContextCanceled checks if an error is due to context cancellation.
func isContextCanceled(err error) bool {
	if err == nil {
		return false
	}
	// Check for direct context errors
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Check error message for wrapped context errors
	errStr := err.Error()
	return strings.Contains(errStr, "context canceled") ||
		strings.Contains(errStr, "context deadline exceeded")
}

// countExistingRepos counts how many repos already have a backup (update) vs new.
// Checks the latest directory for a valid git repo.
func (b *Backup) countExistingRepos(backupDir string, repos []api.Repository, projects []api.Project) (existing, newRepos int) {
	basePath := b.storage.BasePath()

	for _, repo := range repos {
		// Check the latest directory for existing git repos
		var gitPath string
		if repo.Project != nil && repo.Project.Key != "" {
			gitPath = filepath.Join(basePath, b.cfg.Workspace, "latest", "projects", repo.Project.Key, "repositories", repo.Slug, "repo.git")
		} else {
			gitPath = filepath.Join(basePath, b.cfg.Workspace, "latest", "personal", "repositories", repo.Slug, "repo.git")
		}

		if isValidGitRepo(gitPath) {
			existing++
		} else {
			newRepos++
		}
	}
	return existing, newRepos
}

// isValidGitRepo checks if a path contains a valid git repository.
// Checks for HEAD file in both bare repo format (path/HEAD) and
// go-git nested format (path/.git/HEAD).
func isValidGitRepo(path string) bool {
	// Check for bare repo format: path/HEAD
	if _, err := os.Stat(path + "/HEAD"); err == nil {
		return true
	}
	// Check for go-git nested format: path/.git/HEAD
	if _, err := os.Stat(path + "/.git/HEAD"); err == nil {
		return true
	}
	return false
}

// Manifest describes a backup.
type Manifest struct {
	Version     string          `json:"version"`
	Workspace   string          `json:"workspace"`
	StartedAt   string          `json:"started_at"`
	CompletedAt string          `json:"completed_at"`
	Stats       ManifestStats   `json:"stats"`
	Options     ManifestOptions `json:"options"`
}

// ManifestStats contains backup statistics.
type ManifestStats struct {
	Projects     int `json:"projects"`
	Repositories int `json:"repositories"`
	PullRequests int `json:"pull_requests"`
	Issues       int `json:"issues"`
	Failed       int `json:"failed"`
}

// ManifestOptions records the backup options used.
type ManifestOptions struct {
	Full        bool `json:"full"`
	Incremental bool `json:"incremental"`
	DryRun      bool `json:"dry_run"`
}
