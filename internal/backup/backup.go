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
	"github.com/andy-wilson/bb-backup/internal/git"
	"github.com/andy-wilson/bb-backup/internal/storage"
)

// Options configures the backup behavior.
type Options struct {
	DryRun      bool
	Full        bool
	Incremental bool
	Verbose     bool
	Quiet       bool
}

// Backup orchestrates the backup process.
type Backup struct {
	cfg     *config.Config
	opts    Options
	client  *api.Client
	storage storage.Storage
	log     Logger
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

	return &Backup{
		cfg:     cfg,
		opts:    opts,
		client:  client,
		storage: store,
		log: &defaultLogger{
			verbose: opts.Verbose,
			quiet:   opts.Quiet,
		},
	}, nil
}

// Run executes the backup process.
func (b *Backup) Run(ctx context.Context) error {
	startTime := time.Now()
	b.log.Info("Starting backup for workspace: %s", b.cfg.Workspace)

	if b.opts.DryRun {
		b.log.Info("DRY RUN - no changes will be made")
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
	repos, err := b.client.GetRepositories(ctx, b.cfg.Workspace)
	if err != nil {
		return fmt.Errorf("fetching repositories: %w", err)
	}
	b.log.Info("Found %d repositories", len(repos))

	// Track stats
	stats := &backupStats{}

	// Process projects and their repositories
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
		}

		// Find repositories for this project
		for _, repo := range repos {
			if repo.Project != nil && repo.Project.Key == project.Key {
				if err := b.backupRepository(ctx, projectDir, &repo, stats); err != nil {
					b.log.Error("Failed to backup repo %s: %v", repo.Slug, err)
					stats.Failed++
					continue
				}
				stats.Repos++
			}
		}
		stats.Projects++
	}

	// Process personal repositories (no project)
	personalRepos := filterPersonalRepos(repos)
	if len(personalRepos) > 0 {
		b.log.Info("Processing %d personal repositories", len(personalRepos))
		personalDir := filepath.Join(backupDir, "personal", "repositories")

		for _, repo := range personalRepos {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("backup cancelled: %w", err)
			}

			if err := b.backupRepository(ctx, personalDir, &repo, stats); err != nil {
				b.log.Error("Failed to backup repo %s: %v", repo.Slug, err)
				stats.Failed++
				continue
			}
			stats.Repos++
		}
	}

	// Generate manifest
	if !b.opts.DryRun {
		manifest := b.createManifest(startTime, stats)
		if err := b.saveJSON(backupDir, "manifest.json", manifest); err != nil {
			return fmt.Errorf("saving manifest: %w", err)
		}
	}

	elapsed := time.Since(startTime)
	b.log.Info("Backup completed in %s", elapsed.Round(time.Second))
	b.log.Info("Stats: %d projects, %d repos, %d PRs, %d issues, %d failed",
		stats.Projects, stats.Repos, stats.PullRequests, stats.Issues, stats.Failed)

	return nil
}

func (b *Backup) backupRepository(ctx context.Context, baseDir string, repo *api.Repository, stats *backupStats) error {
	b.log.Debug("  Backing up repository: %s", repo.Slug)

	repoDir := filepath.Join(baseDir, "repositories", repo.Slug)

	// Save repository metadata
	if !b.opts.DryRun {
		if err := b.saveJSON(repoDir, "repository.json", repo); err != nil {
			return fmt.Errorf("saving repository metadata: %w", err)
		}
	}

	// Backup pull requests if enabled
	if b.cfg.Backup.IncludePRs {
		if err := b.backupPullRequests(ctx, repoDir, repo, stats); err != nil {
			b.log.Error("  Failed to backup PRs for %s: %v", repo.Slug, err)
			// Continue with other backups
		}
	}

	// Backup issues if enabled
	if b.cfg.Backup.IncludeIssues && repo.HasIssues {
		if err := b.backupIssues(ctx, repoDir, repo, stats); err != nil {
			b.log.Error("  Failed to backup issues for %s: %v", repo.Slug, err)
			// Continue with other backups
		}
	}

	// Clone/fetch the git repository
	cloneURL := repo.CloneURL()
	if cloneURL == "" {
		b.log.Debug("  No HTTPS clone URL found for %s, skipping git clone", repo.Slug)
		return nil
	}

	gitDir := filepath.Join(repoDir, "repo.git")

	if b.opts.DryRun {
		b.log.Info("  [DRY RUN] Would clone %s", repo.Slug)
		return nil
	}

	// Create authenticated URL
	authURL := git.AuthenticatedURL(cloneURL, b.cfg.Auth.Username, b.cfg.Auth.AppPassword)

	// Check if this is an update or fresh clone
	fullGitPath := filepath.Join(b.storage.BasePath(), gitDir)
	if _, err := os.Stat(fullGitPath); os.IsNotExist(err) {
		b.log.Debug("  Cloning %s (mirror)", repo.Slug)
		if err := git.CloneMirror(ctx, authURL, fullGitPath); err != nil {
			return fmt.Errorf("cloning repository: %w", err)
		}
	} else {
		b.log.Debug("  Fetching updates for %s", repo.Slug)
		if err := git.Fetch(ctx, fullGitPath); err != nil {
			return fmt.Errorf("fetching repository updates: %w", err)
		}
	}

	return nil
}

func (b *Backup) backupPullRequests(ctx context.Context, repoDir string, repo *api.Repository, stats *backupStats) error {
	b.log.Debug("  Fetching pull requests for %s", repo.Slug)

	// Fetch all PRs in all states
	prs, err := b.client.GetAllPullRequests(ctx, b.cfg.Workspace, repo.Slug)
	if err != nil {
		return fmt.Errorf("fetching pull requests: %w", err)
	}

	if len(prs) == 0 {
		b.log.Debug("  No pull requests found")
		return nil
	}

	b.log.Debug("  Found %d pull requests", len(prs))
	prDir := filepath.Join(repoDir, "pull-requests")

	for _, pr := range prs {
		if err := ctx.Err(); err != nil {
			return err
		}

		if b.opts.DryRun {
			b.log.Debug("  [DRY RUN] Would backup PR #%d: %s", pr.ID, pr.Title)
			stats.PullRequests++
			continue
		}

		// Save PR metadata
		prFile := fmt.Sprintf("%d.json", pr.ID)
		if err := b.saveJSON(prDir, prFile, pr); err != nil {
			b.log.Error("  Failed to save PR #%d: %v", pr.ID, err)
			continue
		}

		prSubDir := filepath.Join(prDir, fmt.Sprintf("%d", pr.ID))

		// Backup PR comments if enabled
		if b.cfg.Backup.IncludePRComments {
			comments, err := b.client.GetPullRequestComments(ctx, b.cfg.Workspace, repo.Slug, pr.ID)
			if err != nil {
				b.log.Error("  Failed to fetch comments for PR #%d: %v", pr.ID, err)
			} else if len(comments) > 0 {
				if err := b.saveJSON(prSubDir, "comments.json", comments); err != nil {
					b.log.Error("  Failed to save comments for PR #%d: %v", pr.ID, err)
				}
			}
		}

		// Backup PR activity if enabled
		if b.cfg.Backup.IncludePRActivity {
			activity, err := b.client.GetPullRequestActivity(ctx, b.cfg.Workspace, repo.Slug, pr.ID)
			if err != nil {
				b.log.Error("  Failed to fetch activity for PR #%d: %v", pr.ID, err)
			} else if len(activity) > 0 {
				if err := b.saveJSON(prSubDir, "activity.json", activity); err != nil {
					b.log.Error("  Failed to save activity for PR #%d: %v", pr.ID, err)
				}
			}
		}

		stats.PullRequests++
	}

	return nil
}

func (b *Backup) backupIssues(ctx context.Context, repoDir string, repo *api.Repository, stats *backupStats) error {
	b.log.Debug("  Fetching issues for %s", repo.Slug)

	issues, err := b.client.GetIssues(ctx, b.cfg.Workspace, repo.Slug)
	if err != nil {
		return fmt.Errorf("fetching issues: %w", err)
	}

	if len(issues) == 0 {
		b.log.Debug("  No issues found (tracker may be disabled)")
		return nil
	}

	b.log.Debug("  Found %d issues", len(issues))
	issueDir := filepath.Join(repoDir, "issues")

	for _, issue := range issues {
		if err := ctx.Err(); err != nil {
			return err
		}

		if b.opts.DryRun {
			b.log.Debug("  [DRY RUN] Would backup issue #%d: %s", issue.ID, issue.Title)
			stats.Issues++
			continue
		}

		// Save issue metadata
		issueFile := fmt.Sprintf("%d.json", issue.ID)
		if err := b.saveJSON(issueDir, issueFile, issue); err != nil {
			b.log.Error("  Failed to save issue #%d: %v", issue.ID, err)
			continue
		}

		// Backup issue comments if enabled
		if b.cfg.Backup.IncludeIssueComments {
			issueSubDir := filepath.Join(issueDir, fmt.Sprintf("%d", issue.ID))

			comments, err := b.client.GetIssueComments(ctx, b.cfg.Workspace, repo.Slug, issue.ID)
			if err != nil {
				b.log.Error("  Failed to fetch comments for issue #%d: %v", issue.ID, err)
			} else if len(comments) > 0 {
				if err := b.saveJSON(issueSubDir, "comments.json", comments); err != nil {
					b.log.Error("  Failed to save comments for issue #%d: %v", issue.ID, err)
				}
			}
		}

		stats.Issues++
	}

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

func filterPersonalRepos(repos []api.Repository) []api.Repository {
	var personal []api.Repository
	for _, r := range repos {
		if r.Project == nil {
			personal = append(personal, r)
		}
	}
	return personal
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
