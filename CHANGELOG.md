# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Pre-scan Repository Stats
- Shows "Processing N repos (X updates, Y new)" before starting backup
- Progress display shows "updating: repo-name" or "cloning: repo-name"
- Helps users understand backup scope at a glance

#### Interactive Progress Bar Mode
- New `--interactive` / `-i` flag for user-friendly progress display
- Visual progress bar with Unicode block characters
- Real-time elapsed time display
- ETA countdown and expected completion timestamp
- Cleaner output with debug logs going to file only

#### Two-Line Progress Display
- Current repository shown on dedicated line above progress bar
- Animated spinner (Braille dots) indicates active processing
- Shows "updating: repo-name" or "cloning: repo-name" with spinner
- Displays "Complete" when finished, "Waiting..." when idle
- Clear visual separation between status and progress metrics

#### Automatic Retry for Failed Repos
- New `--retry N` flag to automatically retry failed repos (default: 0)
- Configurable retry attempts with exponential backoff
- Panic recovery prevents go-git crashes from stopping entire backup
- Failed repos are tracked in state file for later retry

#### Retry-Failed Command
- New `retry-failed` command to retry previously failed repositories
- Reads failed repos from state file
- `--clear` flag to clear failed list without retrying
- Integrates with existing backup infrastructure

#### Pure Go Git Implementation
- Replaced shell exec git operations with go-git library (pure Go)
- No longer requires git CLI to be installed
- ~1.7x faster clone operations compared to shell exec
- Full rate limiting control over git HTTP requests
- Integrated with API rate limiter for unified throttling

#### Activity Spinner
- Animated spinner shows during long operations (e.g., `bb-backup list`)
- Updates dynamically with pagination progress ("Fetching repositories... page N, M items")
- Only shown in interactive terminals (not in piped output or JSON mode)
- Terminal detection via `IsTerminal()` helper

#### Single Repository Backup
- New `--repo` flag to backup a single repository by name
- Optimized to fetch repository directly via API (1 call vs paginated list)
- Useful for testing and targeted backups

#### Separate Git and Metadata Backup Modes
- New `--git-only` flag to backup only git repositories (skip PRs, issues)
- New `--metadata-only` flag to backup only PRs, issues, metadata (skip git)
- Enables fast git-only backups without API rate limiting bottleneck
- Run metadata backups less frequently or during off-hours
- Useful for large workspaces where full backups take hours

#### Git Operation Timeout
- New `git_timeout_minutes` config option (default: 30 minutes)
- Prevents indefinite hangs on large repository clones
- Context-based timeout with proper cancellation

#### Enhanced Logging
- Git auth debug logging shows credentials being used (password masked)
- Extensive debug logging throughout backup process
- Log flushing after each write ensures logs hit disk immediately
- Timestamped log filenames to preserve history across runs
- Per-repo job trace IDs (`[abc12345]`) for tracing all operations for a specific repository
- Uses UUIDv7 (time-ordered) for unique job IDs across backup runs
- Worker lifecycle logs still use `[worker-N]` prefix for pool management

#### API Token Authentication
- Support for Bitbucket API tokens (`api_token` auth method)
- Support for Repository/Project/Workspace access tokens (`access_token` auth method)
- Backward compatibility with deprecated app passwords

### Fixed

#### API Token Credentials
- Fixed API token auth: email for API calls, username for git operations
- Previously had these reversed, causing authentication failures

#### Incremental Backup for PRs and Issues
- PRs and issues now use `UpdatedSince` API for truly incremental backups
- Only fetches items modified since last backup (previously fetched all)
- Significantly reduces API calls for incremental runs

#### API Pagination Optimization
- Added `pagelen=50` to paginated API requests (5x fewer requests)
- Using 50 instead of 100 for compatibility with all endpoints (some have lower max)

#### Graceful Shutdown
- CTRL-C now properly exits within 5 seconds
- Previously could hang indefinitely if workers were stuck in long operations
- Added timeout-based force shutdown after graceful shutdown period

#### Clean Shutdown Output
- Suppresses noisy "context canceled" error flood on CTRL-C
- Progress bar stops immediately when shutdown starts
- Silently counts interrupted repos instead of logging each one
- Only shows summary at end with interrupted count
- Previously logged every cancelled operation as an error

#### Fast Shutdown on CTRL-C
- Workers now exit immediately when context is cancelled
- Previously workers would drain entire job queue (could take minutes)
- Only in-progress jobs complete; pending jobs are abandoned
- Shutdown now completes in seconds instead of minutes

### Performance Optimizations

#### Adaptive Worker Scaling
- Git workers now auto-scale based on CPU cores (2x cores, clamped 4-16)
- Previously fixed at 4 workers regardless of machine capabilities
- Improves throughput on multi-core systems

#### Parallel PR State Fetching
- Fetches all 4 PR states (OPEN, MERGED, DECLINED, SUPERSEDED) concurrently
- Previously fetched sequentially, now 4x faster for PR metadata

#### Buffer Pool for JSON Marshaling
- Reuses buffers via sync.Pool to reduce GC pressure
- Streaming JSON encoder avoids intermediate byte slice allocations
- Reduces memory allocations when writing many metadata files

#### Streaming JSON Decoder for API
- Uses json.Decoder to parse API responses directly from HTTP body
- Avoids buffering entire response in memory before parsing
- Reduces memory usage for paginated API requests

#### Skip Directory Size Calculation
- Directory size calculation disabled during backup operations
- Previously calculated size after each clone/fetch (expensive I/O)
- Reduces backup time, especially for many repositories

#### Periodic State Checkpointing
- State file saved every 50 repositories for crash recovery
- Previously only saved at end of backup
- Reduces data loss if backup is interrupted

#### Lock-Free Progress Counters
- Progress counters use atomic.Int64 for lock-free updates
- Reduces contention when multiple workers complete simultaneously

#### State File Thread Safety
- State struct now protected by sync.RWMutex
- Safe for concurrent access from multiple workers

#### Latest Directory (Complete Archive)
- `latest/` directory contains complete, aggregated archive of everything
- Git repos updated incrementally (fetch) instead of re-cloned each run
- PRs, issues, comments all saved to both `latest/` and timestamped directories
- `latest/` = always-current complete archive
- Timestamped directories = audit trail of what was fetched each run
- Dramatically faster backups for subsequent runs
- Structure: `latest/projects/<project>/repositories/<repo>/`

#### Accurate Parallel Progress Display
- Progress bar now shows "N repos in progress" when multiple workers active
- Previously showed only the most recently started repo (misleading)
- Correctly tracks active worker count with atomic operations

#### Interrupted Repos Not Counted as Failed
- Repos interrupted by CTRL-C are now tracked separately from failures
- Interrupted repos are NOT added to the failed list in state file
- Summary shows "X interrupted" separately from "Y failed"
- Previously all interrupted repos were marked as failed

#### Panic Stack Traces
- Added full stack traces to panic logs for easier debugging
- Helps identify root cause of go-git crashes

#### Shell Git CLI Fallback
- Automatic fallback to `git` CLI when go-git fails with known issues
- Detects git CLI availability at startup
- Retries with shell git for: packfile errors, nil pointer, unexpected EOF
- Logs fallback attempts for debugging
- Works transparently - no configuration needed

#### go-git Packfile Fix
- Uses forked go-git ([andy-wilson/go-git](https://github.com/andy-wilson/go-git)) with nil packfile fix
- Fixes upstream go-git panic in `decodeObjectAt` and `decodeDeltaObjectAt`
- Prevents "nil pointer dereference" crashes during clone when processing tags
- Converts panic into a graceful error that triggers shell git fallback

## [0.4.0] - 2025-12-19

### Added

#### Phase 4: Extended Features (Partial)

##### Enhanced List Command (`cmd/bb-backup/cmd/list.go`)
- JSON output format with `--json` flag for automation
- Repository filtering with `--include` and `--exclude` patterns
- Shows filtered repository count when patterns applied
- Detailed project/repository output structure

##### Verify Command (`cmd/bb-backup/cmd/verify.go`)
- Verify backup integrity with `bb-backup verify <path>`
- Manifest validation (existence, valid JSON, contents)
- Git repository verification using `git fsck`
- JSON metadata file validation
- Support for verifying PRs, issues, comments, and activity files
- Text and JSON output formats (`--json` flag)
- Verbose mode (`-v`) for detailed per-file results
- Exit code 0 for pass, 1 for failure
- Summary statistics (repos, git, JSON files)

#### Tests
- Verify command tests for manifest, git, and JSON validation
- Directory scanning tests
- Complete repository verification tests

## [0.3.0] - 2025-12-19

### Added

#### Phase 3: Robustness & Incremental Backups

##### Parallel Execution (`internal/backup/worker.go`)
- Worker pool for concurrent repository processing
- Configurable number of parallel workers via `--parallel` flag
- Thread-safe result collection and statistics aggregation

##### Progress Reporting (`internal/backup/progress.go`)
- Real-time progress tracking with completion percentage
- JSON progress output for automation (`--json-progress` flag)
- Rate-limited console updates to avoid spam
- Summary statistics on backup completion

##### Repository Filtering (`internal/backup/filter.go`)
- Include/exclude repositories by glob patterns
- CLI flags: `--include "pattern"` and `--exclude "pattern"`
- Support for `*` and `?` wildcards (e.g., `core-*`, `test-?-*`)
- Exclusion takes precedence over inclusion
- Merge CLI patterns with config file patterns

##### State File & Incremental Backups (`internal/backup/state.go`)
- State file (`.bb-backup-state.json`) tracks backup history
- Per-repository tracking of PR and issue update timestamps
- `--incremental` flag for incremental-only mode (fails if no state)
- `--full` flag to force full backup ignoring previous state
- Auto-detect mode: incremental if state exists, full otherwise
- Incremental mode only fetches PRs/issues updated since last backup

#### Tests
- Filter tests for include/exclude pattern matching
- State file tests for save/load and timestamp tracking

## [0.2.0] - 2025-12-19

### Added

#### Phase 2: Metadata Export

##### Pull Request API (`internal/api/pullrequests.go`)
- `GetPullRequests`: Fetch PRs with optional state filter
- `GetAllPullRequests`: Fetch PRs in all states (OPEN, MERGED, DECLINED, SUPERSEDED)
- `GetPullRequest`: Fetch single PR by ID
- `GetPullRequestComments`: Fetch all comments including inline comments
- `GetPullRequestActivity`: Fetch approvals, updates, and changes requested
- `GetPullRequestsUpdatedSince`: Fetch PRs updated after timestamp (for incremental)

##### Issue API (`internal/api/issues.go`)
- `GetIssues`: Fetch all issues (gracefully handles disabled tracker)
- `GetIssue`: Fetch single issue by ID
- `GetIssueComments`: Fetch all comments on an issue
- `GetIssueChanges`: Fetch issue change history
- `GetIssuesUpdatedSince`: Fetch issues updated after timestamp (for incremental)

##### Data Models
- `PullRequest` with full metadata (source, destination, reviewers, participants)
- `PRComment` with inline comment support (path, line numbers)
- `PRActivity` with approval, update, and changes_requested events
- `Issue` with milestone, version, component support
- `IssueComment` and `IssueChange` for full history

##### Backup Integration
- Export all pull requests for each repository (all states)
- Export PR comments including inline comments with diff context
- Export PR activity (approvals, updates, changes requested)
- Export issues when issue tracker is enabled
- Export issue comments
- Manifest tracks PR and issue counts

#### Tests
- Unit tests for all PR API methods with mocked responses
- Unit tests for all Issue API methods with mocked responses
- Tests for disabled issue tracker handling

## [0.1.0] - 2025-12-19

### Added

#### Phase 1: Foundation

##### Configuration (`internal/config`)
- YAML configuration file parsing with `gopkg.in/yaml.v3`
- Environment variable substitution using `${VAR_NAME}` syntax
- Comprehensive validation for all configuration fields
- Support for app_password and oauth authentication methods
- Sensible default values for all optional settings

##### API Client (`internal/api`)
- Bitbucket Cloud API v2 client with built-in rate limiting
- Token bucket rate limiter with configurable burst size
- Exponential backoff with jitter for 429 (rate limited) responses
- Support for Retry-After header from API responses
- Paginated response handling for list endpoints
- Workspace, projects, and repositories API methods
- Basic authentication with app passwords

##### CLI (`cmd/bb-backup`)
- Cobra-based CLI with subcommands
- `backup` command with full/incremental/dry-run modes
- `list` command to preview repositories
- `version` command with build information
- Global flags: --config, --workspace, --verbose, --quiet
- Support for configuration via file, CLI flags, or environment variables

##### Backup Orchestration (`internal/backup`)
- Full backup of workspace metadata, projects, and repositories
- Git mirror clone for repository backup
- JSON metadata export for workspace, projects, and repos
- Manifest generation with backup statistics
- Graceful shutdown on interrupt signals
- Dry-run mode for safe testing

##### Storage (`internal/storage`)
- Storage interface for pluggable backends
- Local filesystem storage implementation
- Automatic directory creation for nested paths

##### Git Operations (`internal/git`)
- Mirror clone for complete repository backup
- Fetch for incremental updates
- Authenticated URL generation for HTTPS clones
- Git installation detection and version check

#### Tests
- Unit tests for config package (14 tests)
- Unit tests for API client with mocked HTTP responses
- Unit tests for rate limiter
- Unit tests for local storage backend
- Unit tests for git URL authentication

[Unreleased]: https://github.com/andy-wilson/bb-backup/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/andy-wilson/bb-backup/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/andy-wilson/bb-backup/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/andy-wilson/bb-backup/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/andy-wilson/bb-backup/releases/tag/v0.1.0
