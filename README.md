# bb-backup

A CLI tool to backup Bitbucket Cloud workspaces, including git repositories and all associated metadata (projects, pull requests, issues, comments).

## Features

- **Full repository backup** - Mirror clones of all git repositories
- **Metadata export** - Pull requests, comments, issues, approvals, activity
- **Project hierarchy** - Preserves Bitbucket's project structure
- **Rate limit aware** - Respects Bitbucket API limits with smart backoff
- **Incremental backups** - Only fetch changes since last backup
- **Parallel processing** - Configurable worker pools for faster backups
- **Repository filtering** - Include/exclude repos by glob patterns
- **Progress reporting** - Real-time progress with JSON output option
- **Backup verification** - Verify integrity with `git fsck` and JSON validation
- **Cross-platform** - Linux and macOS support

## Installation

### From releases

Download the latest binary from the [Releases](https://github.com/andy-wilson/bb-backup/releases) page.

### From source

```bash
go install github.com/andy-wilson/bb-backup/cmd/bb-backup@latest
```

Or build from source:

```bash
git clone https://github.com/andy-wilson/bb-backup.git
cd bb-backup
make build
```

### Prerequisites

- Go 1.21+ (for building from source)
- `git` CLI installed and in PATH

## Quick Start

See [QUICKSTART.md](QUICKSTART.md) for a detailed getting started guide.

```bash
# Set credentials
export BITBUCKET_USERNAME="your-username"
export BITBUCKET_APP_PASSWORD="your-app-password"

# Run backup
bb-backup backup -w your-workspace -o ./backups

# Verify backup
bb-backup verify ./backups/your-workspace
```

## Usage

```
bb-backup - Backup Bitbucket Cloud workspaces

Usage:
  bb-backup [command] [flags]

Commands:
  backup      Run a backup of the workspace
  list        List repos/projects that would be backed up
  verify      Verify backup integrity
  version     Print version info

Global Flags:
  -c, --config string      Config file (default: ./bb-backup.yaml)
  -w, --workspace string   Workspace to backup (overrides config)
  -v, --verbose            Verbose logging
  -q, --quiet              Quiet mode (errors only)
```

### backup

Run a backup of the configured Bitbucket workspace.

```bash
bb-backup backup [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-o, --output` | Output directory (overrides config) |
| `--full` | Force full backup (ignore previous state) |
| `--incremental` | Force incremental (fail if no state exists) |
| `--dry-run` | Show what would be backed up without doing it |
| `--parallel N` | Number of parallel git workers (default: 4) |
| `--json-progress` | Output progress as JSON lines for automation |
| `--include "pattern"` | Only include repos matching glob pattern |
| `--exclude "pattern"` | Exclude repos matching glob pattern |
| `--username` | Bitbucket username |
| `--app-password` | Bitbucket app password |

**Examples:**
```bash
# Basic backup with config file
bb-backup backup -c config.yaml

# Quick backup with CLI args
bb-backup backup -w my-workspace -o /backups \
  --username user --app-password $TOKEN

# Dry run to preview
bb-backup backup --dry-run

# Force full backup
bb-backup backup --full

# Incremental only (fails if no previous backup)
bb-backup backup --incremental

# Filter repositories
bb-backup backup --include "core-*" --exclude "test-*"

# Parallel backup with progress
bb-backup backup --parallel 8 --json-progress
```

### list

List all projects and repositories that would be backed up.

```bash
bb-backup list [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--json` | Output as JSON for automation |
| `--include "pattern"` | Only include repos matching glob pattern |
| `--exclude "pattern"` | Exclude repos matching glob pattern |
| `--username` | Bitbucket username |
| `--app-password` | Bitbucket app password |

**Examples:**
```bash
# List all repos
bb-backup list -w my-workspace

# List with verbose output
bb-backup list -w my-workspace -v

# JSON output for scripting
bb-backup list -w my-workspace --json

# Preview with filters
bb-backup list --exclude "archive-*" --exclude "test-*"
```

### verify

Verify the integrity of a backup.

```bash
bb-backup verify <backup-path> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--json` | Output results as JSON |
| `-v, --verbose` | Show detailed per-file results |

**Checks performed:**
- Manifest file exists and is valid JSON
- All referenced repositories exist
- Git repositories pass `git fsck`
- All metadata JSON files are valid

**Exit codes:**
- `0` - All checks passed
- `1` - One or more checks failed

**Examples:**
```bash
# Basic verification
bb-backup verify /backups/my-workspace

# Verbose output showing all files
bb-backup verify /backups/my-workspace -v

# JSON output for CI/CD pipelines
bb-backup verify /backups/my-workspace --json
```

### version

Print version information.

```bash
bb-backup version
```

## Output Structure

```
/backups/
└── my-workspace/
    ├── .bb-backup-state.json      # State file for incremental backups
    ├── manifest.json              # Backup manifest
    ├── workspace.json             # Workspace metadata
    ├── projects/
    │   └── PROJECT-KEY/
    │       ├── project.json       # Project metadata
    │       └── repositories/
    │           └── repo-name/
    │               ├── repo.git/          # Git mirror clone
    │               ├── repository.json    # Repository metadata
    │               ├── pull-requests/
    │               │   ├── 1.json         # PR metadata
    │               │   └── 1/
    │               │       ├── comments.json
    │               │       └── activity.json
    │               └── issues/
    │                   ├── 1.json         # Issue metadata
    │                   └── 1/
    │                       └── comments.json
    └── personal/
        └── repositories/
            └── personal-repo/
                └── ...
```

## Configuration

### Authentication Methods

bb-backup supports multiple authentication methods:

#### API Token (Recommended)

API tokens are the recommended authentication method as Bitbucket is deprecating app passwords.

```yaml
auth:
  method: "api_token"
  username: "your-username"        # Bitbucket username (for API calls)
  email: "your-email@example.com"  # Email address (for git operations)
  api_token: "${BITBUCKET_API_TOKEN}"
```

Create an API token at: https://bitbucket.org/account/settings/api-tokens/

**Important:** API tokens require:
- Your **username** for API calls
- Your **email** for git clone/fetch operations

#### Access Token (Repository/Project/Workspace)

Access tokens provide scoped access without a user account:

```yaml
auth:
  method: "access_token"
  access_token: "${BITBUCKET_ACCESS_TOKEN}"
```

Create access tokens in repository/project/workspace settings.

#### App Password (Deprecated)

App passwords are deprecated and will stop working on June 9, 2026.

```yaml
auth:
  method: "app_password"
  username: "${BITBUCKET_USERNAME}"
  app_password: "${BITBUCKET_APP_PASSWORD}"
```

### Config File

Create a `bb-backup.yaml` file:

```yaml
workspace: "your-workspace"

auth:
  method: "api_token"
  username: "${BITBUCKET_USERNAME}"
  email: "${BITBUCKET_EMAIL}"
  api_token: "${BITBUCKET_API_TOKEN}"

storage:
  type: "local"
  path: "/backups/bitbucket"

rate_limit:
  requests_per_hour: 900
  burst_size: 10
  max_retries: 5

parallelism:
  git_workers: 4

backup:
  include_prs: true
  include_pr_comments: true
  include_pr_activity: true
  include_issues: true
  include_issue_comments: true
  exclude_repos: []
  include_repos: []

logging:
  level: "info"
```

See [configs/example.yaml](configs/example.yaml) for a fully documented example.

### Environment Variables

Config values can reference environment variables using `${VAR_NAME}` syntax:

```yaml
auth:
  username: "${BITBUCKET_USERNAME}"
  email: "${BITBUCKET_EMAIL}"
  api_token: "${BITBUCKET_API_TOKEN}"
```

You can also use environment variables directly without a config file:

```bash
export BITBUCKET_WORKSPACE="my-workspace"
export BITBUCKET_USERNAME="my-username"
export BITBUCKET_APP_PASSWORD="my-app-password"
export BITBUCKET_BACKUP_PATH="./backups"

bb-backup backup
```

### Configuration Precedence

1. CLI flags (highest priority)
2. Environment variables
3. Config file
4. Defaults (lowest priority)

## Repository Filtering

Use glob patterns to include or exclude repositories:

```bash
# Only backup repos starting with "core-"
bb-backup backup --include "core-*"

# Exclude test and archive repos
bb-backup backup --exclude "test-*" --exclude "archive-*"

# Combine: only core repos, but not core-test
bb-backup backup --include "core-*" --exclude "core-test-*"
```

**Pattern syntax:**
- `*` matches any sequence of characters
- `?` matches any single character
- Exclusions take precedence over inclusions

Patterns can also be set in the config file:

```yaml
backup:
  include_repos:
    - "core-*"
    - "platform-*"
  exclude_repos:
    - "test-*"
    - "archive-*"
```

## Rate Limiting

Bitbucket Cloud limits API requests to ~1000/hour. The default configuration uses 900 req/hour to leave headroom.

**Estimated times for 200 repositories:**
- Full backup: 4-9 hours (depending on PR/issue counts)
- Incremental backup: Minutes to 1 hour

The tool automatically:
- Uses token bucket rate limiting
- Backs off exponentially on 429 responses
- Respects `Retry-After` headers

## Incremental Backups

After the first full backup, subsequent runs are incremental by default:

```bash
# First run: full backup
bb-backup backup

# Subsequent runs: incremental (only fetches changes)
bb-backup backup

# Force a full backup
bb-backup backup --full

# Force incremental (fails if no previous backup exists)
bb-backup backup --incremental
```

The state file (`.bb-backup-state.json`) tracks:
- Last backup timestamps
- Per-repository PR/issue update times
- Project and repo UUIDs

## Development

```bash
# Build
make build

# Run tests
make test

# Run linter
make lint

# Build for all platforms
make build-all

# Clean build artifacts
make clean
```

## License

MIT License - see [LICENSE](LICENSE) for details.
