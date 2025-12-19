# bb-backup

A CLI tool to backup Bitbucket Cloud workspaces, including git repositories and all associated metadata (projects, pull requests, issues, comments).

## Features

- **Full repository backup** - Mirror clones of all git repositories
- **Metadata export** - Pull requests, comments, issues, approvals
- **Project hierarchy** - Preserves Bitbucket's project structure
- **Rate limit aware** - Respects Bitbucket API limits with smart backoff
- **Incremental backups** - Only fetch changes since last backup
- **Cross-platform** - Linux and macOS support

## Installation

### From releases

Download the latest binary from the [Releases](https://github.com/andy-wilson/bb-backup/releases) page.

### From source

```bash
go install github.com/andy-wilson/bb-backup/cmd/bb-backup@latest
```

### Prerequisites

- `git` CLI installed and in PATH

## Quick Start

1. Create a Bitbucket App Password at https://bitbucket.org/account/settings/app-passwords/
   
   Required scopes:
   - `repository` (read)
   - `pullrequest` (read)
   - `issue` (read)
   - `project` (read)
   - `account` (read)

2. Create a config file:

```yaml
# bb-backup.yaml
workspace: "your-workspace"

auth:
  method: "app_password"
  username: "${BITBUCKET_USERNAME}"
  app_password: "${BITBUCKET_APP_PASSWORD}"

storage:
  type: "local"
  path: "/backups/bitbucket"
```

3. Set environment variables:

```bash
export BITBUCKET_USERNAME="your-username"
export BITBUCKET_APP_PASSWORD="your-app-password"
```

4. Run a backup:

```bash
bb-backup backup -c bb-backup.yaml
```

## Usage

```
bb-backup - Backup Bitbucket Cloud workspaces

Usage:
  bb-backup [command] [flags]

Commands:
  backup      Run a backup
  list        List repos/projects that would be backed up
  verify      Verify backup integrity
  version     Print version info

Backup Flags:
  -c, --config string       Config file (default: ./bb-backup.yaml)
  -w, --workspace string    Workspace to backup (overrides config)
  -o, --output string       Output directory (overrides config)
      --full                Force full backup
      --incremental         Force incremental (fail if no state)
      --dry-run             Show what would be backed up
      --parallel int        Parallel repo operations (default: 4)
  -v, --verbose             Verbose logging
  -q, --quiet               Quiet mode (errors only)
```

### Examples

```bash
# Backup using config file
bb-backup backup -c config.yaml

# Quick backup with CLI args
bb-backup backup -w my-workspace -o /backups \
  --username user --app-password $TOKEN

# Dry run to see what would be backed up
bb-backup backup --dry-run

# List all repos in workspace
bb-backup list -w my-workspace

# Force full backup (ignore incremental state)
bb-backup backup --full

# Incremental backup (only changes since last run)
bb-backup backup --incremental
```

## Output Structure

```
/backups/bitbucket/
└── my-workspace/
    └── 2025-12-15T10-30-00Z/
        ├── manifest.json
        ├── workspace.json
        └── projects/
            └── PROJECT-KEY/
                ├── project.json
                └── repositories/
                    └── repo-name/
                        ├── repo.git/
                        ├── repository.json
                        ├── pull-requests/
                        │   ├── 1.json
                        │   └── 1/
                        │       ├── comments.json
                        │       └── activity.json
                        └── issues/
                            └── ...
```

## Configuration

See [configs/example.yaml](configs/example.yaml) for a fully documented example.

### Environment Variable Substitution

Config values can reference environment variables:

```yaml
auth:
  username: "${BITBUCKET_USERNAME}"
  app_password: "${BITBUCKET_APP_PASSWORD}"
```

### Rate Limiting

Bitbucket Cloud limits API requests to ~1000/hour. The default configuration uses 900 req/hour to leave headroom. For a workspace with 200 repositories, a full backup may take 4-9 hours.

Incremental backups are much faster as they only fetch changed data.

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
```

## License

MIT License - see [LICENSE](LICENSE) for details.
