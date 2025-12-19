# CLAUDE.md

This file provides context for Claude Code when working on this project.

## Project Summary

`bb-backup` is a Go CLI tool to backup Bitbucket Cloud workspaces including git repositories and metadata (projects, PRs, issues, comments).

## Key Files

- `SPEC.md` - Full project specification with requirements, acceptance criteria, and phases
- `cmd/bb-backup/main.go` - CLI entrypoint
- `internal/api/` - Bitbucket API client with rate limiting
- `internal/backup/` - Backup orchestration
- `internal/config/` - Configuration handling
- `internal/git/` - Git operations
- `internal/storage/` - Storage backends

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
2. **Dependencies**: Keep minimal - prefer standard library
3. **Testing**: Unit tests with mocked API responses (no test workspace available)
4. **Error handling**: Wrap errors with context using `fmt.Errorf("context: %w", err)`
5. **Logging**: Use structured logging, never log credentials
6. **Rate limiting**: Critical - Bitbucket limits to ~1000 req/hour

## Architecture Notes

- API client has built-in rate limiting (token bucket + backoff on 429)
- Config supports environment variable substitution `${VAR_NAME}`
- Output structure mirrors Bitbucket hierarchy: workspace/project/repo
- Personal repos (no project) go under `personal/` directory
- State file tracks last backup for incremental support

## Common Tasks

### Adding a new API endpoint

1. Add method to appropriate file in `internal/api/`
2. Add model in `pkg/models/` if new entity
3. Add unit test with mocked response in `testdata/fixtures/`

### Adding a new CLI command

1. Add command in `cmd/bb-backup/`
2. Wire up in `main.go`
3. Update help text

## Current Phase

**Phase 1: Foundation** - Building core infrastructure:
- Config parsing
- API client with rate limiting
- Project/repo listing
- Git mirror clone
- Local storage
- Basic CLI

## Testing Without a Workspace

Since no test workspace is available:
- Use mocked API responses for unit tests
- `--dry-run` flag for safe testing against production
- `--integration-test` flag limits to 1-2 repos for quick validation
