package backup

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andy-wilson/bb-backup/internal/api"
)

// repoJob represents a repository backup job.
type repoJob struct {
	baseDir  string
	repo     *api.Repository
	attempt  int // Current attempt number (0-based)
	maxRetry int // Maximum retry attempts
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
	workers    int
	jobs       chan repoJob
	results    chan repoResult
	wg         sync.WaitGroup
	closeOnce  sync.Once
	jobBuffer  int
	resBuffer  int
	maxRetry   int
	// Instrumentation
	jobsSubmitted  atomic.Int64
	jobsProcessed  atomic.Int64
	jobsRetried    atomic.Int64
	resultsQueued  atomic.Int64
	resultsRead    atomic.Int64
	activeWorkers  atomic.Int64
	lastActivity   atomic.Int64 // Unix timestamp of last activity
	logFunc        func(msg string, args ...interface{})
}

// newWorkerPool creates a new worker pool with the specified number of workers.
func newWorkerPool(workers, totalJobs, maxRetry int, logFunc func(string, ...interface{})) *workerPool {
	// Use larger buffers to prevent deadlock:
	// - jobs buffer: enough for all jobs + potential retries
	// - results buffer: enough for all results to be sent without blocking
	jobBuffer := totalJobs + (totalJobs * maxRetry) // Account for potential retries
	if jobBuffer < workers*2 {
		jobBuffer = workers * 2
	}
	resultBuffer := totalJobs
	if resultBuffer < workers*2 {
		resultBuffer = workers * 2
	}

	p := &workerPool{
		workers:   workers,
		jobs:      make(chan repoJob, jobBuffer),
		results:   make(chan repoResult, resultBuffer),
		jobBuffer: jobBuffer,
		resBuffer: resultBuffer,
		maxRetry:  maxRetry,
		logFunc:   logFunc,
	}
	p.lastActivity.Store(time.Now().Unix())
	return p
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
	defer func() {
		p.activeWorkers.Add(-1)
		p.wg.Done()
		b.log.Debug("[worker-%d] Shutdown (active workers: %d)", workerID, p.activeWorkers.Load())
	}()

	p.activeWorkers.Add(1)
	b.log.Debug("[worker-%d] Started (active workers: %d)", workerID, p.activeWorkers.Load())

	for job := range p.jobs {
		p.processJob(ctx, b, workerID, job)
	}
}

// processJob handles a single backup job with panic recovery and retry support.
func (p *workerPool) processJob(ctx context.Context, b *Backup, workerID int, job repoJob) {
	p.jobsProcessed.Add(1)
	p.lastActivity.Store(time.Now().Unix())

	var jobErr error
	var stats repoStats

	// Recover from panics (e.g., go-git bugs) to prevent crashing the entire backup
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			jobErr = fmt.Errorf("panic recovered in worker: %v", r)
			b.log.Error("[worker-%d] PANIC while processing %s (attempt %d): %v", workerID, job.repo.Slug, job.attempt+1, r)
			b.log.Error("[worker-%d] Stack trace:\n%s", workerID, stack)
		}

		// Handle retry or send result
		if jobErr != nil {
			if p.shouldRetry(job, jobErr) {
				p.requeueJob(b, workerID, job, jobErr)
			} else {
				p.sendResult(workerID, repoResult{repo: job.repo, err: jobErr})
			}
		}
	}()

	select {
	case <-ctx.Done():
		// Context cancelled - don't retry
		p.sendResult(workerID, repoResult{
			repo: job.repo,
			err:  ctx.Err(),
		})
		return
	default:
	}

	attemptStr := ""
	if job.attempt > 0 {
		attemptStr = fmt.Sprintf(" (retry %d/%d)", job.attempt, job.maxRetry)
	}
	b.log.Debug("[worker-%d] Processing: %s%s (jobs: %d/%d processed)",
		workerID, job.repo.Slug, attemptStr, p.jobsProcessed.Load(), p.jobsSubmitted.Load())

	// Update progress with whether this is an update (fetch) or new clone
	if b.progress != nil {
		repoDir := job.baseDir + "/repositories/" + job.repo.Slug
		gitDir := b.storage.BasePath() + "/" + repoDir + "/repo.git"
		if _, err := os.Stat(gitDir); err == nil {
			b.progress.StartWithType(job.repo.Slug, "updating")
		} else {
			b.progress.StartWithType(job.repo.Slug, "cloning")
		}
	}

	stats, jobErr = b.backupRepositoryWorker(ctx, job.baseDir, job.repo, workerID)

	if jobErr == nil {
		b.log.Debug("[worker-%d] Completed: %s%s", workerID, job.repo.Slug, attemptStr)
		p.sendResult(workerID, repoResult{
			repo:  job.repo,
			stats: stats,
			err:   nil,
		})
	} else {
		b.log.Debug("[worker-%d] Failed: %s%s - %v", workerID, job.repo.Slug, attemptStr, jobErr)
		// Defer will handle retry or final result
	}
}

// shouldRetry returns true if the job should be retried.
func (p *workerPool) shouldRetry(job repoJob, err error) bool {
	// Don't retry context cancellation
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}
	return job.attempt < job.maxRetry
}

// requeueJob requeues a failed job for retry.
func (p *workerPool) requeueJob(b *Backup, workerID int, job repoJob, err error) {
	job.attempt++
	p.jobsRetried.Add(1)
	p.jobsSubmitted.Add(1) // Count retry as new submission

	b.log.Info("[worker-%d] Retrying %s (attempt %d/%d) after error: %v",
		workerID, job.repo.Slug, job.attempt+1, job.maxRetry+1, err)

	// Brief delay before retry to avoid hammering on transient errors
	time.Sleep(time.Duration(job.attempt) * 2 * time.Second)

	// Requeue the job (non-blocking since buffer should have space)
	select {
	case p.jobs <- job:
		p.lastActivity.Store(time.Now().Unix())
	default:
		// Buffer full - shouldn't happen with our sizing, but handle gracefully
		b.log.Error("[worker-%d] Failed to requeue %s - job buffer full", workerID, job.repo.Slug)
		p.sendResult(workerID, repoResult{repo: job.repo, err: err})
	}
}

// sendResult sends a result to the results channel with instrumentation.
func (p *workerPool) sendResult(workerID int, result repoResult) {
	startWait := time.Now()

	// Try non-blocking send first
	select {
	case p.results <- result:
		p.resultsQueued.Add(1)
		p.lastActivity.Store(time.Now().Unix())
		return
	default:
		// Channel might be full, log and do blocking send
		if p.logFunc != nil {
			p.logFunc("[worker-%d] Results channel full (%d/%d), waiting...",
				workerID, len(p.results), p.resBuffer)
		}
	}

	// Blocking send with periodic logging
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case p.results <- result:
			p.resultsQueued.Add(1)
			p.lastActivity.Store(time.Now().Unix())
			waited := time.Since(startWait)
			if waited > time.Second {
				if p.logFunc != nil {
					p.logFunc("[worker-%d] Results channel unblocked after %s", workerID, waited.Round(time.Millisecond))
				}
			}
			return
		case <-ticker.C:
			if p.logFunc != nil {
				p.logFunc("[worker-%d] STALL: Waiting to send result for %s (results: %d/%d, read: %d)",
					workerID, time.Since(startWait).Round(time.Second), len(p.results), p.resBuffer, p.resultsRead.Load())
			}
		}
	}
}

// submit adds a job to the worker pool.
func (p *workerPool) submit(job repoJob) {
	p.jobsSubmitted.Add(1)
	p.lastActivity.Store(time.Now().Unix())
	p.jobs <- job
}

// markResultRead should be called when a result is read from the results channel.
func (p *workerPool) markResultRead() {
	p.resultsRead.Add(1)
	p.lastActivity.Store(time.Now().Unix())
}

// stats returns current worker pool statistics.
func (p *workerPool) stats() string {
	return fmt.Sprintf("workers=%d/%d active, jobs=%d/%d processed, retries=%d, results=%d queued/%d read, channels: jobs=%d/%d results=%d/%d",
		p.activeWorkers.Load(), p.workers,
		p.jobsProcessed.Load(), p.jobsSubmitted.Load(),
		p.jobsRetried.Load(),
		p.resultsQueued.Load(), p.resultsRead.Load(),
		len(p.jobs), p.jobBuffer,
		len(p.results), p.resBuffer)
}

// close signals no more jobs will be submitted.
func (p *workerPool) close() {
	close(p.jobs)
}

// wait waits for all workers to finish.
func (p *workerPool) wait() {
	p.wg.Wait()
	p.closeResults()
}

// closeResults closes the results channel (safe to call multiple times).
func (p *workerPool) closeResults() {
	p.closeOnce.Do(func() {
		close(p.results)
	})
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
	var prs []api.PullRequest
	var err error
	var isIncremental bool

	// Check if we can do incremental backup
	lastPRUpdated := b.state.GetLastPRUpdated(repo.Slug)
	if !b.opts.Full && lastPRUpdated != "" {
		// Incremental: only fetch PRs updated since last backup
		prs, err = b.client.GetPullRequestsUpdatedSince(ctx, b.cfg.Workspace, repo.Slug, lastPRUpdated)
		isIncremental = true
		if err != nil {
			return 0, err
		}
		if len(prs) > 0 {
			b.log.Debug("[worker-%d] Found %d updated pull requests for %s (since %s)", workerID, len(prs), repo.Slug, lastPRUpdated)
		}
	} else {
		// Full backup: fetch all PRs
		prs, err = b.client.GetAllPullRequests(ctx, b.cfg.Workspace, repo.Slug)
		if err != nil {
			return 0, err
		}
		if len(prs) > 0 {
			b.log.Debug("[worker-%d] Found %d pull requests for %s", workerID, len(prs), repo.Slug)
		}
	}

	if len(prs) == 0 {
		return 0, nil
	}

	prDir := repoDir + "/pull-requests"
	count := 0
	var latestUpdated string

	for _, pr := range prs {
		if err := ctx.Err(); err != nil {
			return count, err
		}

		// Track the latest updated_on timestamp
		if pr.UpdatedOn > latestUpdated {
			latestUpdated = pr.UpdatedOn
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

	// Update state with latest timestamp for next incremental backup
	if latestUpdated != "" && !b.opts.DryRun {
		b.state.SetRepoLastPRUpdated(repo.Slug, latestUpdated)
	} else if !isIncremental && !b.opts.DryRun && len(prs) == 0 {
		// First backup with no PRs - set timestamp to now
		b.state.SetRepoLastPRUpdated(repo.Slug, time.Now().UTC().Format(time.RFC3339))
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
	var issues []api.Issue
	var err error
	var isIncremental bool

	// Check if we can do incremental backup
	lastIssueUpdated := b.state.GetLastIssueUpdated(repo.Slug)
	if !b.opts.Full && lastIssueUpdated != "" {
		// Incremental: only fetch issues updated since last backup
		issues, err = b.client.GetIssuesUpdatedSince(ctx, b.cfg.Workspace, repo.Slug, lastIssueUpdated)
		isIncremental = true
		if err != nil {
			return 0, err
		}
		if len(issues) > 0 {
			b.log.Debug("[worker-%d] Found %d updated issues for %s (since %s)", workerID, len(issues), repo.Slug, lastIssueUpdated)
		}
	} else {
		// Full backup: fetch all issues
		issues, err = b.client.GetIssues(ctx, b.cfg.Workspace, repo.Slug)
		if err != nil {
			return 0, err
		}
		if len(issues) > 0 {
			b.log.Debug("[worker-%d] Found %d issues for %s", workerID, len(issues), repo.Slug)
		}
	}

	if len(issues) == 0 {
		// If full backup with no issues, set timestamp to now for future incrementals
		if !isIncremental && !b.opts.DryRun {
			b.state.SetRepoLastIssueUpdated(repo.Slug, time.Now().UTC().Format(time.RFC3339))
		}
		return 0, nil
	}

	issueDir := repoDir + "/issues"
	count := 0
	var latestUpdated string

	for _, issue := range issues {
		if err := ctx.Err(); err != nil {
			return count, err
		}

		// Track the latest updated_on timestamp
		if issue.UpdatedOn > latestUpdated {
			latestUpdated = issue.UpdatedOn
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

	// Update state with latest timestamp for next incremental backup
	if latestUpdated != "" && !b.opts.DryRun {
		b.state.SetRepoLastIssueUpdated(repo.Slug, latestUpdated)
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
