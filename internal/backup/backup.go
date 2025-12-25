// Package backup orchestrates the backup process for Bitbucket workspaces.
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/andy-wilson/bb-backup/internal/api"
	"github.com/andy-wilson/bb-backup/internal/config"
	"github.com/andy-wilson/bb-backup/internal/storage"
)

// Options configures the backup behavior.
type Options struct {
	DryRun       bool
	Full         bool
	Incremental  bool
	Verbose      bool
	Quiet        bool
	JSONProgress bool
	Logger       Logger // Optional external logger
}

// Backup orchestrates the backup process.
type Backup struct {
	cfg      *config.Config
	opts     Options
	client   *api.Client
	storage  storage.Storage
	log      Logger
	state    *State
	filter   *RepoFilter
	progress *Progress
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
	client := api.NewClient(cfg)

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

	// Create repo filter
	filter := NewRepoFilter(cfg.Backup.IncludeRepos, cfg.Backup.ExcludeRepos)

	// Use provided logger or create default
	var log Logger
	if opts.Logger != nil {
		log = opts.Logger
	} else {
		log = &defaultLogger{
			verbose: opts.Verbose,
			quiet:   opts.Quiet,
		}
	}

	return &Backup{
		cfg:     cfg,
		opts:    opts,
		client:  client,
		storage: store,
		log:     log,
		state:   state,
		filter:  filter,
	}, nil
}

// Run executes the backup process.
func (b *Backup) Run(ctx context.Context) error {
	startTime := time.Now()
	b.log.Info("Starting backup for workspace: %s", b.cfg.Workspace)

	if b.opts.DryRun {
		b.log.Info("DRY RUN - no changes will be made")
	}

	if b.opts.Incremental && b.state.HasPreviousBackup() {
		b.log.Info("Incremental backup (last: %s)", b.state.LastIncremental)
	} else {
		b.log.Info("Full backup")
	}

	// Create backup directory with timestamp
	backupDir := filepath.Join(b.cfg.Workspace, startTime.Format("2006-01-02T15-04-05Z"))

	// Fetch workspace metadata
	b.log.Info("Fetching workspace metadata...")
	workspace, err := b.client.GetWorkspace(ctx, b.cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching workspace: %w", err)
	}

	if !b.opts.DryRun {
		if err := b.saveJSON(backupDir, "workspace.json", workspace); err != nil {
			return fmt.Errorf("saving workspace metadata: %w", err)
		}
	}
	b.log.Debug("Workspace: %s (%s)", workspace.Name, workspace.UUID)

	// Fetch projects
	b.log.Info("Fetching projects...")
	projects, err := b.client.GetProjects(ctx, b.cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching projects: %w", err)
	}
	b.log.Info("Found %d projects", len(projects))

	// Fetch all repositories
	b.log.Info("Fetching repositories...")
	allRepos, err := b.client.GetRepositories(ctx, b.cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching repositories: %w", err)
	}

	// Apply filters
	repos := b.filter.Filter(allRepos)
	included, excluded := b.filter.FilteredCount(allRepos)
	if excluded > 0 {
		b.log.Info("Found %d repositories (%d excluded by filters)", included, excluded)
	} else {
		b.log.Info("Found %d repositories", len(repos))
	}

	// Initialize progress tracker
	b.progress = NewProgress(len(repos), b.opts.JSONProgress, b.opts.Quiet)

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
		} else {
			b.state.MarkIncrementalBackup()
		}

		statePath := GetStatePath(b.cfg.Storage.Path, b.cfg.Workspace)
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
	b.log.Info("Stats: %d projects, %d repos, %d PRs, %d issues, %d failed",
		stats.Projects, stats.Repos, stats.PullRequests, stats.Issues, stats.Failed)

	if b.progress != nil {
		b.progress.Summary()
	}

	return nil
}

// processRepositories processes all repositories with parallel workers.
func (b *Backup) processRepositories(ctx context.Context, backupDir string, repos []api.Repository, projects []api.Project, stats *backupStats) error {
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

	// Create worker pool
	workers := b.cfg.Parallelism.GitWorkers
	if workers < 1 {
		workers = 1
	}
	pool := newWorkerPool(workers)
	pool.start(ctx, b)

	// Submit jobs for project repos
	for _, project := range projects {
		projectDir := filepath.Join(backupDir, "projects", project.Key)
		for _, repo := range reposByProject[project.Key] {
			pool.submit(repoJob{
				baseDir: projectDir,
				repo:    &repo,
			})
		}
	}

	// Submit jobs for personal repos
	personalDir := filepath.Join(backupDir, "personal")
	for _, repo := range personalRepos {
		pool.submit(repoJob{
			baseDir: personalDir,
			repo:    &repo,
		})
	}

	// Close jobs channel and collect results
	pool.close()

	// Collect results in a separate goroutine
	done := make(chan struct{})
	go func() {
		for result := range pool.results {
			if result.err != nil {
				b.log.Error("Failed to backup repo %s: %v", result.repo.Slug, result.err)
				stats.Failed++
				if b.progress != nil {
					b.progress.Fail(result.repo.Slug, result.err)
				}
			} else {
				stats.Repos++
				stats.PullRequests += result.stats.PullRequests
				stats.Issues += result.stats.Issues

				// Update state
				projectKey := ""
				if result.repo.Project != nil {
					projectKey = result.repo.Project.Key
				}
				b.state.UpdateRepository(result.repo.Slug, result.repo.UUID, projectKey)

				if b.progress != nil {
					b.progress.Complete(result.repo.Slug)
				}
			}
		}
		close(done)
	}()

	// Wait for workers to finish
	pool.wait()
	<-done

	return nil
}

func (b *Backup) saveJSON(dir, filename string, data interface{}) error {
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}

	return b.storage.Write(filepath.Join(dir, filename), content)
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
