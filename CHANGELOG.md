# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

#### Single Repository Backup
- New `--repo` flag to backup a single repository by name
- Optimized to fetch repository directly via API (1 call vs paginated list)
- Useful for testing and targeted backups

#### Git Operation Timeout
- New `git_timeout_minutes` config option (default: 30 minutes)
- Prevents indefinite hangs on large repository clones
- Context-based timeout with proper cancellation

#### Enhanced Logging
- Git auth debug logging shows credentials being used (password masked)
- Extensive debug logging throughout backup process
- Log flushing after each write ensures logs hit disk immediately
- Timestamped log filenames to preserve history across runs

#### API Token Authentication
- Support for Bitbucket API tokens (`api_token` auth method)
- Support for Repository/Project/Workspace access tokens (`access_token` auth method)
- Backward compatibility with deprecated app passwords

### Fixed

#### API Token Credentials
- Fixed API token auth: email for API calls, username for git operations
- Previously had these reversed, causing authentication failures

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
