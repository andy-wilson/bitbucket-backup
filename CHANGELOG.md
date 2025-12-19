# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2025-12-19

### Added

#### Configuration (`internal/config`)
- YAML configuration file parsing with `gopkg.in/yaml.v3`
- Environment variable substitution using `${VAR_NAME}` syntax
- Comprehensive validation for all configuration fields
- Support for app_password and oauth authentication methods
- Sensible default values for all optional settings

#### API Client (`internal/api`)
- Bitbucket Cloud API v2 client with built-in rate limiting
- Token bucket rate limiter with configurable burst size
- Exponential backoff with jitter for 429 (rate limited) responses
- Support for Retry-After header from API responses
- Paginated response handling for list endpoints
- Workspace, projects, and repositories API methods
- Basic authentication with app passwords

#### CLI (`cmd/bb-backup`)
- Cobra-based CLI with subcommands
- `backup` command with full/incremental/dry-run modes
- `list` command to preview repositories
- `version` command with build information
- Global flags: --config, --workspace, --verbose, --quiet
- Support for configuration via file, CLI flags, or environment variables

#### Backup Orchestration (`internal/backup`)
- Full backup of workspace metadata, projects, and repositories
- Git mirror clone for repository backup
- JSON metadata export for workspace, projects, and repos
- Manifest generation with backup statistics
- Graceful shutdown on interrupt signals
- Dry-run mode for safe testing

#### Storage (`internal/storage`)
- Storage interface for pluggable backends
- Local filesystem storage implementation
- Automatic directory creation for nested paths

#### Git Operations (`internal/git`)
- Mirror clone for complete repository backup
- Fetch for incremental updates
- Authenticated URL generation for HTTPS clones
- Git installation detection and version check

### Testing
- Unit tests for config package (14 tests)
- Unit tests for API client with mocked HTTP responses
- Unit tests for rate limiter
- Unit tests for local storage backend
- Unit tests for git URL authentication

[Unreleased]: https://github.com/andy-wilson/bb-backup/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/andy-wilson/bb-backup/releases/tag/v0.1.0
