# CLAUDE.md

This file provides context for Claude Code when working on this project.

## Project Summary

`bb-backup` is a Go CLI tool to backup Bitbucket Cloud workspaces including git repositories and metadata (projects, PRs, issues, comments).

## Project Status

**Completed Phases:**
- Phase 1: Foundation (config, API client, CLI, git operations, storage)
- Phase 2: Metadata Export (PRs, issues, comments, activity)
- Phase 3: Robustness & Incremental (parallel execution, progress, filtering, state file)
- Phase 4: Extended Features (partial - verify command, enhanced list command)

**Remaining from Phase 4:**
- OAuth authentication
- S3 storage backend
- `restore` command
- Notifications (Slack, email)

## Key Files

| File | Purpose |
|------|---------|
| `SPEC.md` | Full project specification with requirements and acceptance criteria |
| `QUICKSTART.md` | Getting started guide |
| `cmd/bb-backup/main.go` | CLI entrypoint |
| `cmd/bb-backup/cmd/` | CLI commands (backup, list, verify, retry-failed, version) |
| `internal/api/` | Bitbucket API client with rate limiting |
| `internal/api/client.go` | HTTP client with retry logic |
| `internal/api/ratelimit.go` | Token bucket rate limiter |
| `internal/api/pullrequests.go` | PR API methods |
| `internal/api/issues.go` | Issue API methods |
| `internal/backup/` | Backup orchestration |
| `internal/backup/backup.go` | Main backup logic |
| `internal/backup/worker.go` | Parallel worker pool |
| `internal/backup/progress.go` | Progress reporting |
| `internal/backup/filter.go` | Repository filtering |
| `internal/backup/state.go` | State file for incremental backups |
| `internal/config/` | Configuration handling |
| `internal/git/` | Git operations (clone, fetch) |
| `internal/git/gogit.go` | Pure Go git implementation using go-git |
| `internal/git/shell.go` | Shell git CLI fallback implementation |
| `internal/storage/` | Storage backends (local filesystem) |
| `internal/ui/` | Terminal UI components (spinner, progress bar) |
| `internal/ui/progressbar.go` | Interactive progress bar with ETA |

## Build Commands

```bash
# Build
make build

# Run tests
make test

# Run linter
make lint

# Build for all platforms
make build-all

# Clean
make clean
```

## Development Guidelines

1. **Language**: Go 1.21+
2. **Dependencies**: Keep minimal - key deps: go-git (git), cobra (CLI), yaml.v3 (config)
3. **Testing**: Unit tests with mocked API responses (no test workspace available)
4. **Error handling**: Wrap errors with context using `fmt.Errorf("context: %w", err)`
5. **Logging**: Use structured logging, never log credentials
6. **Rate limiting**: Critical - Bitbucket limits to ~1000 req/hour
7. **Git CLI fallback**: Uses go-git (pure Go) with optional shell git fallback for edge cases

## Architecture Notes

- API client has built-in rate limiting (token bucket + backoff on 429)
- Config supports environment variable substitution `${VAR_NAME}`
- Output structure mirrors Bitbucket hierarchy: workspace/project/repo
- Personal repos (no project) go under `personal/` directory
- **Latest directory**: Git repos stored in `<workspace>/latest/` for incremental updates
  - Repos are fetched (updated) instead of re-cloned on subsequent runs
  - Timestamped directories (`<workspace>/<timestamp>/`) contain metadata only
  - Structure: `latest/projects/<project>/repositories/<repo>/repo.git`
- State file (`.bb-backup-state.json`) tracks last backup for incremental support
- Worker pool enables parallel git operations with worker ID tracking in logs
- Filter supports glob patterns for include/exclude
- Single-repo mode (`--repo`) fetches directly via API (optimized)
- Git operations have configurable timeout (`git_timeout_minutes`)
- API tokens: email for API calls, username for git operations
- Pure Go git via go-git library with shell git CLI fallback
- Automatic fallback to `git` CLI for packfile errors, nil pointer panics
- Git HTTP transport integrated with API rate limiter
- Activity spinner for long operations (terminal-only, auto-detected)
- Interactive progress bar mode (`-i`) with ETA and visual progress
- Incremental backup uses `UpdatedSince` API for PRs/issues (only fetches changes)
- Graceful shutdown on CTRL-C with 5-second timeout
- Interrupted repos tracked separately from failures (not added to retry list)
- Panic recovery in workers with stack trace logging

## Common Tasks

### Adding a new API endpoint

1. Add method to appropriate file in `internal/api/`
2. Add model structs if new entity type
3. Add unit test with mocked HTTP response

### Adding a new CLI command

1. Create command file in `cmd/bb-backup/cmd/`
2. Register in `init()` with `rootCmd.AddCommand()`
3. Update help text and README.md

### Running a dry-run backup

```bash
bb-backup backup --dry-run -w your-workspace
```

### Testing single-repo backup

```bash
bb-backup backup --repo my-repo-name -w your-workspace
```

### Verifying a backup

```bash
bb-backup verify /path/to/backup
```

## Testing Without a Workspace

Since no test workspace is available:
- Use mocked API responses for unit tests
- `--dry-run` flag for safe testing against production
- Verify command checks backup integrity without API access

## Test Coverage

Current test coverage by package:
- `internal/config`: ~80%
- `internal/api`: ~57%
- `internal/storage`: ~84%
- `internal/git`: ~45%
- `internal/backup`: ~12%
- `cmd/bb-backup/cmd`: ~31%
