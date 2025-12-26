package backup

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/andy-wilson/bb-backup/internal/api"
)

// repoJob represents a repository backup job.
type repoJob struct {
	baseDir string
	repo    *api.Repository
}

// repoResult represents the result of a repository backup.
type repoResult struct {
	repo  *api.Repository
	stats repoStats
	err   error
}

// repoStats tracks stats for a single repository backup.
type repoStats struct {
	PullRequests int
	Issues       int
}

// workerPool manages concurrent repository backup operations.
type workerPool struct {
	workers int
	jobs    chan repoJob
	results chan repoResult
	wg      sync.WaitGroup
}

// newWorkerPool creates a new worker pool with the specified number of workers.
func newWorkerPool(workers int) *workerPool {
	return &workerPool{
		workers: workers,
		jobs:    make(chan repoJob, workers*2),
		results: make(chan repoResult, workers*2),
	}
}

// start launches the worker goroutines.
func (p *workerPool) start(ctx context.Context, b *Backup) {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		workerID := i + 1
		go p.worker(ctx, b, workerID)
	}
}

// worker processes repository backup jobs.
func (p *workerPool) worker(ctx context.Context, b *Backup, workerID int) {
	defer p.wg.Done()

	for job := range p.jobs {
		select {
		case <-ctx.Done():
			p.results <- repoResult{
				repo: job.repo,
				err:  ctx.Err(),
			}
			continue
		default:
		}

		b.log.Debug("[worker-%d] Processing: %s", workerID, job.repo.Slug)
		stats, err := b.backupRepositoryWorker(ctx, job.baseDir, job.repo, workerID)
		if err == nil {
			b.log.Debug("[worker-%d] Completed: %s", workerID, job.repo.Slug)
		}
		p.results <- repoResult{
			repo:  job.repo,
			stats: stats,
			err:   err,
		}
	}
}

// submit adds a job to the worker pool.
func (p *workerPool) submit(job repoJob) {
	p.jobs <- job
}

// close signals no more jobs will be submitted.
func (p *workerPool) close() {
	close(p.jobs)
}

// wait waits for all workers to finish.
func (p *workerPool) wait() {
	p.wg.Wait()
	close(p.results)
}

// backupRepositoryWorker is a worker-friendly version of backupRepository.
func (b *Backup) backupRepositoryWorker(ctx context.Context, baseDir string, repo *api.Repository, workerID int) (repoStats, error) {
	var stats repoStats

	repoDir := baseDir + "/repositories/" + repo.Slug

	// Save repository metadata
	if !b.opts.DryRun {
		if err := b.saveJSON(repoDir, "repository.json", repo); err != nil {
			return stats, err
		}
	}

	// Backup pull requests if enabled
	if b.cfg.Backup.IncludePRs {
		prCount, err := b.backupPullRequestsWorker(ctx, repoDir, repo, workerID)
		if err != nil {
			b.log.Error("[worker-%d] Failed to backup PRs for %s: %v", workerID, repo.Slug, err)
		}
		stats.PullRequests = prCount
	}

	// Backup issues if enabled
	if b.cfg.Backup.IncludeIssues && repo.HasIssues {
		issueCount, err := b.backupIssuesWorker(ctx, repoDir, repo, workerID)
		if err != nil {
			b.log.Error("[worker-%d] Failed to backup issues for %s: %v", workerID, repo.Slug, err)
		}
		stats.Issues = issueCount
	}

	// Clone/fetch the git repository
	if err := b.backupGitRepo(ctx, repoDir, repo, workerID); err != nil {
		return stats, err
	}

	return stats, nil
}

// backupPullRequestsWorker is a worker-friendly version that returns count.
func (b *Backup) backupPullRequestsWorker(ctx context.Context, repoDir string, repo *api.Repository, workerID int) (int, error) {
	prs, err := b.client.GetAllPullRequests(ctx, b.cfg.Workspace, repo.Slug)
	if err != nil {
		return 0, err
	}

	if len(prs) == 0 {
		return 0, nil
	}

	b.log.Debug("[worker-%d] Found %d pull requests for %s", workerID, len(prs), repo.Slug)
	prDir := repoDir + "/pull-requests"
	count := 0

	for _, pr := range prs {
		if err := ctx.Err(); err != nil {
			return count, err
		}

		if b.opts.DryRun {
			count++
			continue
		}

		if err := b.savePR(ctx, prDir, repo.Slug, &pr); err != nil {
			b.log.Error("[worker-%d] Failed to save PR #%d: %v", workerID, pr.ID, err)
			continue
		}
		count++
	}

	return count, nil
}

// savePR saves a single PR and its related data.
func (b *Backup) savePR(ctx context.Context, prDir, repoSlug string, pr *api.PullRequest) error {
	prFile := fmt.Sprintf("%d.json", pr.ID)
	if err := b.saveJSON(prDir, prFile, pr); err != nil {
		return err
	}

	prSubDir := fmt.Sprintf("%s/%d", prDir, pr.ID)

	if b.cfg.Backup.IncludePRComments {
		comments, err := b.client.GetPullRequestComments(ctx, b.cfg.Workspace, repoSlug, pr.ID)
		if err != nil {
			b.log.Error("  Failed to fetch comments for PR #%d: %v", pr.ID, err)
		} else if len(comments) > 0 {
			if err := b.saveJSON(prSubDir, "comments.json", comments); err != nil {
				b.log.Error("  Failed to save comments for PR #%d: %v", pr.ID, err)
			}
		}
	}

	if b.cfg.Backup.IncludePRActivity {
		activity, err := b.client.GetPullRequestActivity(ctx, b.cfg.Workspace, repoSlug, pr.ID)
		if err != nil {
			b.log.Error("  Failed to fetch activity for PR #%d: %v", pr.ID, err)
		} else if len(activity) > 0 {
			if err := b.saveJSON(prSubDir, "activity.json", activity); err != nil {
				b.log.Error("  Failed to save activity for PR #%d: %v", pr.ID, err)
			}
		}
	}

	return nil
}

// backupIssuesWorker is a worker-friendly version that returns count.
func (b *Backup) backupIssuesWorker(ctx context.Context, repoDir string, repo *api.Repository, workerID int) (int, error) {
	issues, err := b.client.GetIssues(ctx, b.cfg.Workspace, repo.Slug)
	if err != nil {
		return 0, err
	}

	if len(issues) == 0 {
		return 0, nil
	}

	b.log.Debug("[worker-%d] Found %d issues for %s", workerID, len(issues), repo.Slug)
	issueDir := repoDir + "/issues"
	count := 0

	for _, issue := range issues {
		if err := ctx.Err(); err != nil {
			return count, err
		}

		if b.opts.DryRun {
			count++
			continue
		}

		if err := b.saveIssue(ctx, issueDir, repo.Slug, &issue); err != nil {
			b.log.Error("[worker-%d] Failed to save issue #%d: %v", workerID, issue.ID, err)
			continue
		}
		count++
	}

	return count, nil
}

// saveIssue saves a single issue and its related data.
func (b *Backup) saveIssue(ctx context.Context, issueDir, repoSlug string, issue *api.Issue) error {
	issueFile := fmt.Sprintf("%d.json", issue.ID)
	if err := b.saveJSON(issueDir, issueFile, issue); err != nil {
		return err
	}

	if b.cfg.Backup.IncludeIssueComments {
		issueSubDir := fmt.Sprintf("%s/%d", issueDir, issue.ID)

		comments, err := b.client.GetIssueComments(ctx, b.cfg.Workspace, repoSlug, issue.ID)
		if err != nil {
			b.log.Error("  Failed to fetch comments for issue #%d: %v", issue.ID, err)
		} else if len(comments) > 0 {
			if err := b.saveJSON(issueSubDir, "comments.json", comments); err != nil {
				b.log.Error("  Failed to save comments for issue #%d: %v", issue.ID, err)
			}
		}
	}

	return nil
}

// backupGitRepo clones or fetches the git repository using go-git.
func (b *Backup) backupGitRepo(ctx context.Context, repoDir string, repo *api.Repository, workerID int) error {
	cloneURL := repo.CloneURL()
	if cloneURL == "" {
		b.log.Debug("[worker-%d] No HTTPS clone URL found for %s, skipping git clone", workerID, repo.Slug)
		return nil
	}

	gitDir := repoDir + "/repo.git"

	if b.opts.DryRun {
		b.log.Info("[worker-%d] [DRY RUN] Would clone %s", workerID, repo.Slug)
		return nil
	}

	// Log git credentials being used (mask password)
	gitUser, gitPass := b.cfg.GetGitCredentials()
	maskedPass := "***"
	if len(gitPass) > 4 {
		maskedPass = gitPass[:4] + "***"
	}
	b.log.Debug("[worker-%d] Git auth: user=%q, pass=%s, method=%s", workerID, gitUser, maskedPass, b.cfg.Auth.Method)

	fullGitPath := b.storage.BasePath() + "/" + gitDir

	// Create a context with timeout for git operations
	timeout := time.Duration(b.cfg.Backup.GitTimeoutMinutes) * time.Minute
	if timeout <= 0 {
		timeout = 30 * time.Minute // Default to 30 minutes
	}
	gitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Use go-git for clone/fetch operations
	if _, err := os.Stat(fullGitPath); os.IsNotExist(err) {
		b.log.Debug("[worker-%d] Cloning %s (mirror, go-git)", workerID, repo.Slug)
		if err := b.gitClient.CloneMirror(gitCtx, cloneURL, fullGitPath); err != nil {
			if gitCtx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("git clone timed out after %d minutes", b.cfg.Backup.GitTimeoutMinutes)
			}
			return err
		}
	} else {
		b.log.Debug("[worker-%d] Fetching updates for %s (go-git)", workerID, repo.Slug)
		if err := b.gitClient.Fetch(gitCtx, fullGitPath); err != nil {
			if gitCtx.Err() == context.DeadlineExceeded {
				return fmt.Errorf("git fetch timed out after %d minutes", b.cfg.Backup.GitTimeoutMinutes)
			}
			return err
		}
	}

	return nil
}
