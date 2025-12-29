package backup

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/andy-wilson/bb-backup/internal/api"
	"github.com/google/uuid"
)

// repoJob represents a repository backup job.
type repoJob struct {
	baseDir  string
	repo     *api.Repository
	attempt  int    // Current attempt number (0-based)
	maxRetry int    // Maximum retry attempts
	jobID    string // Unique trace ID for this job (UUIDv7 short form)
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

// generateJobID creates a short unique job ID using UUIDv7.
// Returns last 8 characters (random portion) of a UUIDv7 for brevity in logs.
// We use the last 8 chars because UUIDv7's first chars are timestamp-based
// and would be identical for jobs submitted within the same millisecond.
func generateJobID() string {
	id, err := uuid.NewV7()
	if err != nil {
		// Fallback to random UUID if UUIDv7 fails
		id = uuid.New()
	}
	// Use last 8 chars (random portion) for uniqueness within same millisecond
	s := id.String()
	return s[len(s)-8:]
}

// workerPool manages concurrent repository backup operations.
type workerPool struct {
	workers   int
	jobs      chan repoJob
	results   chan repoResult
	wg        sync.WaitGroup
	closeOnce sync.Once
	jobBuffer int
	resBuffer int
	maxRetry  int
	// Instrumentation
	jobsSubmitted atomic.Int64
	jobsProcessed atomic.Int64
	jobsRetried   atomic.Int64
	resultsQueued atomic.Int64
	resultsRead   atomic.Int64
	activeWorkers atomic.Int64
	lastActivity  atomic.Int64 // Unix timestamp of last activity
	logFunc       func(msg string, args ...interface{})
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

	for {
		select {
		case <-ctx.Done():
			// Context cancelled - exit immediately without draining queue
			b.log.Debug("[worker-%d] Context cancelled, exiting", workerID)
			return
		case job, ok := <-p.jobs:
			if !ok {
				// Channel closed, no more jobs
				return
			}
			p.processJob(ctx, b, workerID, job)
		}
	}
}

// processJob handles a single backup job with panic recovery and retry support.
func (p *workerPool) processJob(ctx context.Context, b *Backup, workerID int, job repoJob) {
	p.jobsProcessed.Add(1)
	p.lastActivity.Store(time.Now().Unix())

	// Add worker ID and job ID to context for logging
	ctx = api.WithWorkerID(ctx, workerID)
	ctx = api.WithJobID(ctx, job.jobID)

	// Log prefix for this job
	prefix := fmt.Sprintf("[%s]", job.jobID)

	var jobErr error
	var stats repoStats

	// Recover from panics (e.g., go-git bugs) to prevent crashing the entire backup
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			jobErr = fmt.Errorf("panic recovered in worker: %v", r)
			// Only log panics if not shutting down
			if !b.shuttingDown.Load() {
				b.log.Error("%s PANIC while processing %s (attempt %d): %v", prefix, job.repo.Slug, job.attempt+1, r)
				b.log.Error("%s Stack trace:\n%s", prefix, stack)
			}
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
	b.log.Debug("%s Processing: %s%s (worker-%d, jobs: %d/%d)",
		prefix, job.repo.Slug, attemptStr, workerID, p.jobsProcessed.Load(), p.jobsSubmitted.Load())

	// Update progress with operation type
	if b.progress != nil && !b.shuttingDown.Load() {
		if b.opts.MetadataOnly {
			// Metadata-only mode: fetching PRs/issues
			b.progress.StartWithType(job.repo.Slug, "fetching metadata")
		} else if b.opts.GitOnly {
			// Git-only mode: check if update or clone
			latestGitPath := b.storage.BasePath() + "/" + b.getLatestGitPath(job.repo)
			if isValidGitRepo(latestGitPath) {
				b.progress.StartWithType(job.repo.Slug, "fetching")
			} else {
				b.progress.StartWithType(job.repo.Slug, "cloning")
			}
		} else {
			// Normal mode: check if update or clone
			latestGitPath := b.storage.BasePath() + "/" + b.getLatestGitPath(job.repo)
			if isValidGitRepo(latestGitPath) {
				b.progress.StartWithType(job.repo.Slug, "updating")
			} else {
				b.progress.StartWithType(job.repo.Slug, "cloning")
			}
		}
	}

	stats, jobErr = b.backupRepositoryWorker(ctx, job.baseDir, job.repo)

	if jobErr == nil {
		b.log.Debug("%s Completed: %s%s", prefix, job.repo.Slug, attemptStr)
		p.sendResult(workerID, repoResult{
			repo:  job.repo,
			stats: stats,
			err:   nil,
		})
	} else {
		b.log.Debug("%s Failed: %s%s - %v", prefix, job.repo.Slug, attemptStr, jobErr)
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

	b.log.Info("[%s] Retrying %s (attempt %d/%d) after error: %v",
		job.jobID, job.repo.Slug, job.attempt+1, job.maxRetry+1, err)

	// Brief delay before retry to avoid hammering on transient errors
	time.Sleep(time.Duration(job.attempt) * 2 * time.Second)

	// Requeue the job (non-blocking since buffer should have space)
	select {
	case p.jobs <- job:
		p.lastActivity.Store(time.Now().Unix())
	default:
		// Buffer full - shouldn't happen with our sizing, but handle gracefully
		b.log.Error("[%s] Failed to requeue %s - job buffer full", job.jobID, job.repo.Slug)
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
func (b *Backup) backupRepositoryWorker(ctx context.Context, baseDir string, repo *api.Repository) (repoStats, error) {
	var stats repoStats
	prefix := api.LogPrefix(ctx)

	// Timestamped directory for this run's data
	repoDir := baseDir + "/repositories/" + repo.Slug
	// Latest directory for aggregated data
	latestRepoDir := b.getLatestRepoDir(repo)

	// Save repository metadata to both latest and timestamped directories
	// Skip if git-only mode (metadata-only and normal mode both save metadata)
	if !b.opts.DryRun && !b.opts.GitOnly {
		// Save to latest (aggregated)
		if err := b.saveJSON(latestRepoDir, "repository.json", repo); err != nil {
			return stats, err
		}
		// Save to timestamped directory (this run)
		if err := b.saveJSON(repoDir, "repository.json", repo); err != nil {
			return stats, err
		}
	}

	// Backup pull requests if enabled (skip in git-only mode)
	if b.cfg.Backup.IncludePRs && !b.opts.GitOnly {
		prCount, err := b.backupPullRequestsWorker(ctx, repoDir, latestRepoDir, repo)
		if err != nil && !b.shuttingDown.Load() && !isContextCanceled(err) {
			b.log.Error("%sFailed to backup PRs for %s: %v", prefix, repo.Slug, err)
		}
		stats.PullRequests = prCount
	}

	// Backup issues if enabled (skip in git-only mode)
	if b.cfg.Backup.IncludeIssues && repo.HasIssues && !b.opts.GitOnly {
		issueCount, err := b.backupIssuesWorker(ctx, repoDir, latestRepoDir, repo)
		if err != nil && !b.shuttingDown.Load() && !isContextCanceled(err) {
			b.log.Error("%sFailed to backup issues for %s: %v", prefix, repo.Slug, err)
		}
		stats.Issues = issueCount
	}

	// Clone/fetch the git repository (skip in metadata-only mode)
	if !b.opts.MetadataOnly {
		if err := b.backupGitRepo(ctx, repoDir, repo); err != nil {
			return stats, err
		}
	}

	return stats, nil
}

// backupPullRequestsWorker is a worker-friendly version that returns count.
// Saves PRs to both timestamped (repoDir) and latest (latestRepoDir) directories.
func (b *Backup) backupPullRequestsWorker(ctx context.Context, repoDir, latestRepoDir string, repo *api.Repository) (int, error) {
	prefix := api.LogPrefix(ctx)
	var prs []api.PullRequest
	var err error
	var isIncremental bool

	// Update progress to show we're fetching PRs
	if b.progress != nil && !b.shuttingDown.Load() {
		b.progress.UpdateStatus(fmt.Sprintf("fetching PRs: %s", repo.Slug))
	}

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
			b.log.Debug("%sFound %d updated pull requests for %s (since %s)", prefix, len(prs), repo.Slug, lastPRUpdated)
		}
	} else {
		// Full backup: fetch all PRs
		prs, err = b.client.GetAllPullRequests(ctx, b.cfg.Workspace, repo.Slug)
		if err != nil {
			return 0, err
		}
		if len(prs) > 0 {
			b.log.Debug("%sFound %d pull requests for %s", prefix, len(prs), repo.Slug)
		}
	}

	if len(prs) == 0 {
		return 0, nil
	}

	prDir := repoDir + "/pull-requests"
	latestPRDir := latestRepoDir + "/pull-requests"
	count := 0
	var latestUpdated string

	totalPRs := len(prs)
	for i, pr := range prs {
		if err := ctx.Err(); err != nil {
			return count, err
		}

		// Update progress to show PR processing progress
		if b.progress != nil && !b.shuttingDown.Load() {
			b.progress.UpdateStatus(fmt.Sprintf("saving PRs: %s (%d/%d)", repo.Slug, i+1, totalPRs))
		}

		// Track the latest updated_on timestamp
		if pr.UpdatedOn > latestUpdated {
			latestUpdated = pr.UpdatedOn
		}

		if b.opts.DryRun {
			count++
			continue
		}

		// Save to timestamped directory
		if err := b.savePR(ctx, prDir, repo.Slug, &pr); err != nil {
			b.log.Error("%sFailed to save PR #%d: %v", prefix, pr.ID, err)
			continue
		}
		// Save to latest directory (aggregated)
		if err := b.savePR(ctx, latestPRDir, repo.Slug, &pr); err != nil {
			b.log.Error("%sFailed to save PR #%d to latest: %v", prefix, pr.ID, err)
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
	prefix := api.LogPrefix(ctx)
	prFile := fmt.Sprintf("%d.json", pr.ID)
	if err := b.saveJSON(prDir, prFile, pr); err != nil {
		return err
	}

	prSubDir := fmt.Sprintf("%s/%d", prDir, pr.ID)

	if b.cfg.Backup.IncludePRComments {
		// Update progress to show we're fetching PR comments
		if b.progress != nil && !b.shuttingDown.Load() {
			b.progress.UpdateStatus(fmt.Sprintf("PR #%d comments: %s", pr.ID, repoSlug))
		}
		comments, err := b.client.GetPullRequestComments(ctx, b.cfg.Workspace, repoSlug, pr.ID)
		if err != nil {
			if !b.shuttingDown.Load() && !isContextCanceled(err) {
				b.log.Error("%sFailed to fetch comments for PR #%d: %v", prefix, pr.ID, err)
			}
		} else if len(comments) > 0 {
			if err := b.saveJSON(prSubDir, "comments.json", comments); err != nil {
				b.log.Error("%sFailed to save comments for PR #%d: %v", prefix, pr.ID, err)
			}
		}
	}

	if b.cfg.Backup.IncludePRActivity {
		// Update progress to show we're fetching PR activity
		if b.progress != nil && !b.shuttingDown.Load() {
			b.progress.UpdateStatus(fmt.Sprintf("PR #%d activity: %s", pr.ID, repoSlug))
		}
		activity, err := b.client.GetPullRequestActivity(ctx, b.cfg.Workspace, repoSlug, pr.ID)
		if err != nil {
			if !b.shuttingDown.Load() && !isContextCanceled(err) {
				b.log.Error("%sFailed to fetch activity for PR #%d: %v", prefix, pr.ID, err)
			}
		} else if len(activity) > 0 {
			if err := b.saveJSON(prSubDir, "activity.json", activity); err != nil {
				b.log.Error("%sFailed to save activity for PR #%d: %v", prefix, pr.ID, err)
			}
		}
	}

	return nil
}

// backupIssuesWorker is a worker-friendly version that returns count.
// Saves issues to both timestamped (repoDir) and latest (latestRepoDir) directories.
func (b *Backup) backupIssuesWorker(ctx context.Context, repoDir, latestRepoDir string, repo *api.Repository) (int, error) {
	prefix := api.LogPrefix(ctx)
	var issues []api.Issue
	var err error
	var isIncremental bool

	// Update progress to show we're fetching issues
	if b.progress != nil && !b.shuttingDown.Load() {
		b.progress.UpdateStatus(fmt.Sprintf("fetching issues: %s", repo.Slug))
	}

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
			b.log.Debug("%sFound %d updated issues for %s (since %s)", prefix, len(issues), repo.Slug, lastIssueUpdated)
		}
	} else {
		// Full backup: fetch all issues
		issues, err = b.client.GetIssues(ctx, b.cfg.Workspace, repo.Slug)
		if err != nil {
			return 0, err
		}
		if len(issues) > 0 {
			b.log.Debug("%sFound %d issues for %s", prefix, len(issues), repo.Slug)
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
	latestIssueDir := latestRepoDir + "/issues"
	count := 0
	var latestUpdated string

	totalIssues := len(issues)
	for i, issue := range issues {
		if err := ctx.Err(); err != nil {
			return count, err
		}

		// Update progress to show issue processing progress
		if b.progress != nil && !b.shuttingDown.Load() {
			b.progress.UpdateStatus(fmt.Sprintf("saving issues: %s (%d/%d)", repo.Slug, i+1, totalIssues))
		}

		// Track the latest updated_on timestamp
		if issue.UpdatedOn > latestUpdated {
			latestUpdated = issue.UpdatedOn
		}

		if b.opts.DryRun {
			count++
			continue
		}

		// Save to timestamped directory
		if err := b.saveIssue(ctx, issueDir, repo.Slug, &issue); err != nil {
			b.log.Error("%sFailed to save issue #%d: %v", prefix, issue.ID, err)
			continue
		}
		// Save to latest directory (aggregated)
		if err := b.saveIssue(ctx, latestIssueDir, repo.Slug, &issue); err != nil {
			b.log.Error("%sFailed to save issue #%d to latest: %v", prefix, issue.ID, err)
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
	prefix := api.LogPrefix(ctx)
	issueFile := fmt.Sprintf("%d.json", issue.ID)
	if err := b.saveJSON(issueDir, issueFile, issue); err != nil {
		return err
	}

	if b.cfg.Backup.IncludeIssueComments {
		// Update progress to show we're fetching issue comments
		if b.progress != nil && !b.shuttingDown.Load() {
			b.progress.UpdateStatus(fmt.Sprintf("issue #%d comments: %s", issue.ID, repoSlug))
		}
		issueSubDir := fmt.Sprintf("%s/%d", issueDir, issue.ID)

		comments, err := b.client.GetIssueComments(ctx, b.cfg.Workspace, repoSlug, issue.ID)
		if err != nil {
			if !b.shuttingDown.Load() && !isContextCanceled(err) {
				b.log.Error("%sFailed to fetch comments for issue #%d: %v", prefix, issue.ID, err)
			}
		} else if len(comments) > 0 {
			if err := b.saveJSON(issueSubDir, "comments.json", comments); err != nil {
				b.log.Error("%sFailed to save comments for issue #%d: %v", prefix, issue.ID, err)
			}
		}
	}

	return nil
}

// getLatestRepoDir returns the path to the latest copy of a repository.
// The latest directory contains the aggregated/current state of all backups.
// Structure: <workspace>/latest/projects/<project_key>/repositories/<repo_slug>/
func (b *Backup) getLatestRepoDir(repo *api.Repository) string {
	if repo.Project != nil && repo.Project.Key != "" {
		return b.cfg.Workspace + "/latest/projects/" + repo.Project.Key + "/repositories/" + repo.Slug
	}
	return b.cfg.Workspace + "/latest/personal/repositories/" + repo.Slug
}

// getLatestGitPath returns the shared git repo path in the latest directory.
func (b *Backup) getLatestGitPath(repo *api.Repository) string {
	return b.getLatestRepoDir(repo) + "/repo.git"
}

func (b *Backup) backupGitRepo(ctx context.Context, repoDir string, repo *api.Repository) error {
	prefix := api.LogPrefix(ctx)
	cloneURL := repo.CloneURL()
	if cloneURL == "" {
		b.log.Debug("%sNo HTTPS clone URL found for %s, skipping git clone", prefix, repo.Slug)
		return nil
	}

	// Use latest directory for git repos (shared across all backup runs)
	// This allows repos to be updated incrementally instead of re-cloned
	latestGitDir := b.getLatestGitPath(repo)

	if b.opts.DryRun {
		b.log.Info("%s[DRY RUN] Would clone %s", prefix, repo.Slug)
		return nil
	}

	// Log git credentials being used (mask password)
	gitUser, gitPass := b.cfg.GetGitCredentials()
	maskedPass := "***"
	if len(gitPass) > 4 {
		maskedPass = gitPass[:4] + "***"
	}
	b.log.Debug("%sGit auth: user=%q, pass=%s, method=%s", prefix, gitUser, maskedPass, b.cfg.Auth.Method)

	fullGitPath := b.storage.BasePath() + "/" + latestGitDir

	// Create a context with timeout for git operations
	timeout := time.Duration(b.cfg.Backup.GitTimeoutMinutes) * time.Minute
	if timeout <= 0 {
		timeout = 30 * time.Minute // Default to 30 minutes
	}
	gitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Try go-git first, fall back to shell git if it fails
	// Check for HEAD file to verify it's a valid git repo (not just an empty directory)
	isClone := !isValidGitRepo(fullGitPath)

	// Wrap go-git calls in panic recovery so we can fall back to shell git
	var goGitErr error
	func() {
		defer func() {
			if r := recover(); r != nil {
				goGitErr = fmt.Errorf("go-git panic: %v", r)
				b.log.Debug("%sgo-git panicked: %v", prefix, r)
			}
		}()
		if isClone {
			b.log.Debug("%sCloning %s (mirror, go-git)", prefix, repo.Slug)
			goGitErr = b.gitClient.CloneMirror(gitCtx, cloneURL, fullGitPath)
		} else {
			b.log.Debug("%sFetching updates for %s (go-git)", prefix, repo.Slug)
			goGitErr = b.gitClient.Fetch(gitCtx, fullGitPath)
		}
	}()

	// If go-git succeeded, we're done
	if goGitErr == nil {
		return nil
	}

	// Check for timeout
	if gitCtx.Err() == context.DeadlineExceeded {
		if isClone {
			return fmt.Errorf("git clone timed out after %d minutes", b.cfg.Backup.GitTimeoutMinutes)
		}
		return fmt.Errorf("git fetch timed out after %d minutes", b.cfg.Backup.GitTimeoutMinutes)
	}

	// If shell git is not available, return the go-git error
	if b.shellGitClient == nil {
		return goGitErr
	}

	// Check if this is a go-git specific error that shell git might handle better
	if !isGoGitRetryableError(goGitErr) {
		return goGitErr
	}

	// Try shell git as fallback
	b.log.Debug("%sgo-git failed (%v), retrying with git CLI", prefix, goGitErr)

	// Reset context timeout for retry
	gitCtx2, cancel2 := context.WithTimeout(ctx, timeout)
	defer cancel2()

	if isClone {
		// Clean up failed go-git attempt
		_ = os.RemoveAll(fullGitPath)
		b.log.Debug("%sCloning %s (mirror, git CLI fallback)", prefix, repo.Slug)
		if err := b.shellGitClient.CloneMirror(gitCtx2, cloneURL, fullGitPath); err != nil {
			if gitCtx2.Err() == context.DeadlineExceeded {
				return fmt.Errorf("git clone timed out after %d minutes (CLI fallback)", b.cfg.Backup.GitTimeoutMinutes)
			}
			return fmt.Errorf("git CLI fallback also failed: %w (original go-git error: %v)", err, goGitErr)
		}
	} else {
		b.log.Debug("%sFetching updates for %s (git CLI fallback)", prefix, repo.Slug)
		if err := b.shellGitClient.Fetch(gitCtx2, fullGitPath); err != nil {
			if gitCtx2.Err() == context.DeadlineExceeded {
				return fmt.Errorf("git fetch timed out after %d minutes (CLI fallback)", b.cfg.Backup.GitTimeoutMinutes)
			}
			return fmt.Errorf("git CLI fallback also failed: %w (original go-git error: %v)", err, goGitErr)
		}
	}

	b.log.Debug("%sgit CLI fallback succeeded for %s", prefix, repo.Slug)
	return nil
}

// isGoGitRetryableError checks if an error from go-git is likely to be fixed by using shell git.
func isGoGitRetryableError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Known go-git issues that shell git handles better
	retryablePatterns := []string{
		"go-git panic",
		"packfile is nil",
		"nil pointer",
		"invalid memory address",
		"unexpected EOF",
		"reference delta not found",
	}
	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}
	return false
}

