# bb-backup

A CLI tool to backup Bitbucket Cloud workspaces, including git repositories and all associated metadata (projects, pull requests, issues, comments).

## Features

- **Full repository backup** - Mirror clones of all git repositories
- **Pure Go implementation** - No external git CLI required (uses go-git)
- **Metadata export** - Pull requests, comments, issues, approvals, activity
- **Project hierarchy** - Preserves Bitbucket's project structure
- **Rate limit aware** - Respects Bitbucket API limits with smart backoff
- **Incremental backups** - Only fetch PRs/issues changed since last backup
- **Parallel processing** - Configurable worker pools for faster backups
- **Repository filtering** - Include/exclude repos by glob patterns
- **Interactive mode** - Progress bar with ETA and real-time status (`-i` flag)
- **Progress reporting** - JSON output for automation (`--json-progress`)
- **Backup verification** - Verify integrity with `git fsck` and JSON validation
- **Graceful shutdown** - CTRL-C safely stops backup without losing progress
- **Automatic retry** - Retry failed repos with exponential backoff
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
- No external dependencies required (uses pure Go git implementation)

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
  backup        Run a backup of the workspace
  list          List repos/projects that would be backed up
  retry-failed  Retry backup for previously failed repos
  verify        Verify backup integrity
  version       Print version info

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
| `--retry N` | Max retry attempts for failed repos (default: 0) |
| `-i, --interactive` | Interactive mode with progress bar and ETA |
| `--json-progress` | Output progress as JSON lines for automation |
| `--include "pattern"` | Only include repos matching glob pattern |
| `--exclude "pattern"` | Exclude repos matching glob pattern |
| `--repo "name"` | Backup only a single repository (optimized) |
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

# Backup a single repository (optimized - skips fetching all repos)
bb-backup backup --repo my-repo-name

# Interactive mode with progress bar
bb-backup backup -i

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

### retry-failed

Retry backup for repositories that failed in a previous run.

```bash
bb-backup retry-failed [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--retry N` | Max retry attempts per repo (default: 2) |
| `--clear` | Clear failed repos list without retrying |

**Examples:**
```bash
# Retry all failed repos
bb-backup retry-failed -c config.yaml

# Retry with more attempts
bb-backup retry-failed --retry 5

# Clear the failed list without retrying
bb-backup retry-failed --clear
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
    ├── latest/                    # Shared git repos (updated incrementally)
    │   ├── projects/
    │   │   └── PROJECT-KEY/
    │   │       └── repositories/
    │   │           └── repo-name/
    │   │               └── repo.git/      # Git mirror clone (fetch updates)
    │   └── personal/
    │       └── repositories/
    │           └── repo-name/
    │               └── repo.git/
    ├── 2024-01-15T10-30-00Z/      # Timestamped backup run
    │   ├── manifest.json          # Backup manifest
    │   ├── workspace.json         # Workspace metadata
    │   ├── projects/
    │   │   └── PROJECT-KEY/
    │   │       ├── project.json   # Project metadata
    │   │       └── repositories/
    │   │           └── repo-name/
    │   │               ├── repository.json    # Repository metadata
    │   │               ├── pull-requests/
    │   │               │   ├── 1.json         # PR metadata
    │   │               │   └── 1/
    │   │               │       ├── comments.json
    │   │               │       └── activity.json
    │   │               └── issues/
    │   │                   ├── 1.json         # Issue metadata
    │   │                   └── 1/
    │   │                       └── comments.json
    │   └── personal/
    │       └── repositories/
    │           └── personal-repo/
    │               └── ...
    └── 2024-01-16T10-30-00Z/      # Next backup run (metadata only)
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
  username: "your-username"        # Bitbucket username (for git operations)
  email: "your-email@example.com"  # Email address (for API calls)
  api_token: "${BITBUCKET_API_TOKEN}"
```

Create an API token at: https://bitbucket.org/account/settings/api-tokens/

**Important:** API tokens require:
- Your **email** for API calls
- Your **username** for git clone/fetch operations

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
  git_timeout_minutes: 30  # Timeout for git clone/fetch (default: 30)

logging:
  level: "info"
  file: ""  # Optional: log to file (timestamped automatically)
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

## Restoring from Backup

Repositories are backed up as bare git mirror clones (`.git` format). This preserves all branches, tags, and history.

### Restore a Single Repository

```bash
# Clone from the backup to create a working directory
git clone /backups/my-workspace/latest/projects/PROJ/repositories/my-repo/repo.git my-repo

# Or clone as bare repository
git clone --bare /backups/my-workspace/latest/projects/PROJ/repositories/my-repo/repo.git my-repo.git
```

### Restore to a New Remote

```bash
# Clone from backup
git clone /backups/my-workspace/latest/projects/PROJ/repositories/my-repo/repo.git my-repo
cd my-repo

# Change the remote to point to a new location
git remote set-url origin https://github.com/org/my-repo.git

# Push all branches and tags
git push --all origin
git push --tags origin
```

### Restore All Repositories

```bash
#!/bin/bash
# Script to restore all repos from a backup

BACKUP_DIR="/backups/my-workspace/latest"
TARGET_DIR="/restored"

# Find all repo.git directories and clone them
find "$BACKUP_DIR" -name "repo.git" -type d | while read repo_path; do
    # Extract repo name from path
    repo_name=$(basename $(dirname "$repo_path"))
    echo "Restoring $repo_name..."
    git clone "$repo_path" "$TARGET_DIR/$repo_name"
done
```

### View Backup Contents Without Cloning

```bash
# List all branches in a backup
git --git-dir=/backups/my-workspace/latest/.../repo.git branch -a

# List all tags
git --git-dir=/backups/my-workspace/latest/.../repo.git tag

# View commit log
git --git-dir=/backups/my-workspace/latest/.../repo.git log --oneline -20

# Show a specific file from a commit
git --git-dir=/backups/my-workspace/latest/.../repo.git show HEAD:path/to/file.txt
```

### Restoring Metadata

Metadata (PRs, issues, comments) is stored as JSON files alongside each repository:

```bash
# View PR metadata
cat /backups/.../repositories/my-repo/pull-requests/123.json | jq .

# View issue with comments
cat /backups/.../repositories/my-repo/issues/45.json | jq .
cat /backups/.../repositories/my-repo/issues/45/comments.json | jq .

# List all PRs for a repo
ls /backups/.../repositories/my-repo/pull-requests/*.json
```

**Note:** There is currently no automated restore command to push metadata back to Bitbucket. The JSON files serve as an archive for reference, compliance, or migration to other platforms.

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
