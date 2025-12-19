# bb-backup: Bitbucket Cloud Backup Tool

## Project Overview

A CLI tool written in Go to backup Bitbucket Cloud workspaces, including git repositories and all associated metadata (projects, pull requests, issues, comments, etc.).

## Problem Statement

Bitbucket Cloud lacks a built-in backup feature for organisations. Existing open source tools only backup git data, not metadata (PRs, comments, issues). This tool fills that gap.

## Goals

- Provide a reliable, automated backup solution for Bitbucket Cloud workspaces
- Backup both git repository data AND associated metadata
- Support full and incremental backups
- Be runnable on a schedule (cron/systemd timer friendly)
- Respect API rate limits to avoid overloading Bitbucket
- Storage-agnostic (local filesystem initially, extensible to S3/GCS)

## Target Platforms

| Platform | Support Level |
|----------|---------------|
| Linux (amd64) | Primary |
| Linux (arm64) | Primary |
| macOS (amd64) | Secondary |
| macOS (arm64) | Secondary |

Single static binary, no runtime dependencies except `git` CLI.

## Bitbucket Hierarchy

```
Workspace (organisation)
├── Project A (KEY: PROJA)
│   ├── repo-1
│   ├── repo-2
│   └── repo-3
├── Project B (KEY: PROJB)
│   └── repo-4
└── (Personal repos - no project)
    └── repo-5
```

## Scope

### In Scope (MVP)

| Entity | Backup | API Endpoint |
|--------|--------|--------------|
| Workspace metadata | ✅ | `GET /2.0/workspaces/{workspace}` |
| Projects | ✅ | `GET /2.0/workspaces/{workspace}/projects` |
| Project metadata | ✅ | `GET /2.0/workspaces/{workspace}/projects/{key}` |
| Repositories (git) | ✅ | `git clone --mirror` |
| Repository metadata | ✅ | `GET /2.0/repositories/{workspace}/{repo}` |
| Pull Requests (all states) | ✅ | `GET /2.0/repositories/{workspace}/{repo}/pullrequests` |
| PR Comments (inline + general) | ✅ | `GET /2.0/repositories/{workspace}/{repo}/pullrequests/{id}/comments` |
| PR Activity/Approvals | ✅ | `GET /2.0/repositories/{workspace}/{repo}/pullrequests/{id}/activity` |
| Issues (if enabled) | ✅ | `GET /2.0/repositories/{workspace}/{repo}/issues` |
| Issue Comments | ✅ | `GET /2.0/repositories/{workspace}/{repo}/issues/{id}/comments` |

### Out of Scope (MVP)

| Entity | Reason |
|--------|--------|
| Pipelines configuration | Lives in `bitbucket-pipelines.yml` in repo |
| Pipeline build history | High volume, debatable value |
| Webhooks | Contains secrets |
| Deploy keys | Security concern |
| Downloads | Lower priority, add later |
| Wiki | Add later if needed |
| LFS objects | Complex, v2 feature |
| Branch permissions | Admin settings, add later |
| Restore to Bitbucket | Complex, Phase 4 feature |

## Output Structure

```
/backups/bitbucket/
└── my-workspace/
    └── 2025-12-15T10-30-00Z/
        ├── manifest.json
        ├── workspace.json
        ├── projects/
        │   ├── PROJA/
        │   │   ├── project.json
        │   │   └── repositories/
        │   │       ├── repo-1/
        │   │       │   ├── repo.git/
        │   │       │   ├── repository.json
        │   │       │   ├── pull-requests/
        │   │       │   │   ├── 1.json
        │   │       │   │   └── 1/
        │   │       │   │       ├── comments.json
        │   │       │   │       ├── activity.json
        │   │       │   │       └── approvals.json
        │   │       │   └── issues/
        │   │       │       ├── 1.json
        │   │       │       └── 1/
        │   │       │           └── comments.json
        │   │       └── repo-2/
        │   │           └── ...
        │   └── PROJB/
        │       ├── project.json
        │       └── repositories/
        │           └── ...
        └── personal/
            └── repositories/
                └── repo-5/
                    └── ...
```

## Configuration

### Config File Format (YAML)

```yaml
# bb-backup.yaml
workspace: "your-workspace-slug"

auth:
  method: "app_password"  # or "oauth"
  username: "${BITBUCKET_USERNAME}"
  app_password: "${BITBUCKET_APP_PASSWORD}"

storage:
  type: "local"
  path: "/backups/bitbucket"

rate_limit:
  requests_per_hour: 900
  burst_size: 10
  max_retries: 5
  retry_backoff_seconds: 5
  retry_backoff_multiplier: 2.0
  max_backoff_seconds: 300

parallelism:
  git_workers: 4
  api_workers: 2

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
  format: "text"
```

### Environment Variable Substitution

Config values like `${BITBUCKET_USERNAME}` are substituted from environment variables.

### Authentication

**App Password (recommended for automation):**
Required scopes:
- `repository` (read)
- `pullrequest` (read)
- `issue` (read)
- `project` (read)
- `account` (read)

**OAuth 2.0 (for interactive use):**
Token cached in `~/.config/bb-backup/token.json`

**Precedence:**
1. CLI flags
2. Environment variables
3. Config file
4. OAuth token cache

## Rate Limiting

**Bitbucket Cloud limits:** ~1,000 requests/hour

**Strategy:**
- Token bucket for smooth distribution
- Read `X-RateLimit-*` headers from responses
- Proactive slowdown when approaching limit
- Exponential backoff with jitter on 429
- Configurable via config file

**Capacity estimate for 200 repos:**
- ~4,000-8,000 API requests for full backup
- At 900 req/hour = 4.5-9 hours for full metadata backup
- Git clones separate, ~100 mins with parallelism of 4

## Incremental Backups

**State file:** `.bb-backup-state.json`

```json
{
  "workspace": "my-workspace",
  "last_full_backup": "2025-12-15T10:30:00Z",
  "last_incremental": "2025-12-16T10:30:00Z",
  "projects": {
    "PROJA": {
      "uuid": "{uuid-here}",
      "last_backed_up": "2025-12-15T10:30:00Z"
    }
  },
  "repositories": {
    "PROJA/repo-1": {
      "uuid": "{uuid-here}",
      "project_key": "PROJA",
      "last_commit": "abc123def456",
      "last_pr_updated": "2025-12-15T09:00:00Z",
      "last_issue_updated": "2025-12-14T15:00:00Z"
    }
  }
}
```

**Incremental logic:**
1. Git: `git fetch` into existing mirror
2. PRs: Query with `updated_on > last_backup` filter
3. Issues: Similar filter by updated timestamp
4. New repos: Full backup
5. Deleted repos: Flag/archive (don't delete local)

## CLI Interface

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

Auth Flags:
      --username string      Bitbucket username
      --app-password string  Bitbucket app password

Examples:
  bb-backup backup -c config.yaml
  bb-backup backup -w my-workspace -o /backups --username user --app-password $TOKEN
  bb-backup backup --dry-run
  bb-backup list -w my-workspace
```

## Project Structure

```
bb-backup/
├── .github/
│   └── workflows/
│       ├── ci.yml
│       └── release.yml
├── cmd/
│   └── bb-backup/
│       └── main.go
├── internal/
│   ├── api/
│   │   ├── client.go
│   │   ├── client_test.go
│   │   ├── ratelimit.go
│   │   ├── ratelimit_test.go
│   │   ├── repositories.go
│   │   ├── pullrequests.go
│   │   ├── issues.go
│   │   └── projects.go
│   ├── backup/
│   │   ├── backup.go
│   │   ├── backup_test.go
│   │   ├── incremental.go
│   │   ├── state.go
│   │   └── manifest.go
│   ├── git/
│   │   ├── git.go
│   │   └── git_test.go
│   ├── storage/
│   │   ├── storage.go
│   │   ├── local.go
│   │   └── local_test.go
│   └── config/
│       ├── config.go
│       └── config_test.go
├── pkg/
│   └── models/
│       ├── repository.go
│       ├── pullrequest.go
│       ├── issue.go
│       └── project.go
├── testdata/
│   └── fixtures/
├── configs/
│   └── example.yaml
├── Makefile
├── go.mod
├── go.sum
├── README.md
├── CHANGELOG.md
└── LICENSE
```

## Implementation Phases

### Phase 1: Foundation (MVP)

- [ ] Project scaffolding (Go modules, Makefile, CI)
- [ ] Config parsing (YAML with env var substitution)
- [ ] Bitbucket API client with rate limiting built-in
- [ ] App Password authentication
- [ ] List workspace metadata
- [ ] List workspace projects with metadata
- [ ] List repositories per project
- [ ] Handle personal repos (no project)
- [ ] Git mirror clone
- [ ] Local storage backend with hierarchical structure
- [ ] Basic CLI (`backup` command, `--full` only)
- [ ] Manifest generation

### Phase 2: Metadata Export

- [ ] Repository metadata export
- [ ] PR list and detail export
- [ ] PR comments export (including inline)
- [ ] PR activity/approvals export
- [ ] Issue export (if tracker enabled)
- [ ] Issue comments export
- [ ] Handle pagination for all endpoints

### Phase 3: Robustness & Incremental

- [ ] Retry logic with exponential backoff
- [ ] Parallel execution (configurable workers)
- [ ] Progress reporting (stdout, optional JSON)
- [ ] Exclusion/inclusion patterns (glob on repo slug)
- [ ] State file generation
- [ ] Incremental backup mode
- [ ] `--dry-run` flag

### Phase 4: Extended Features

- [ ] OAuth authentication
- [ ] S3 storage backend
- [ ] `verify` command
- [ ] `list` command
- [ ] `restore` command (rate-limited)
- [ ] Notifications (Slack, email)

## Acceptance Criteria

### Phase 1

| ID | Criterion | Test Method |
|----|-----------|-------------|
| P1-01 | Config loads from YAML file | Unit test |
| P1-02 | Config substitutes environment variables | Unit test |
| P1-03 | App Password auth succeeds | Integration test |
| P1-04 | API client respects rate limit config | Unit test with mock |
| P1-05 | API client handles 429 with backoff | Unit test with mock |
| P1-06 | Lists all projects in workspace | Integration test |
| P1-07 | Exports project.json for each project | Verify fields |
| P1-08 | Lists all repos per project | Integration test |
| P1-09 | Handles repos without project (personal) | Test |
| P1-10 | Git mirror clone succeeds | `git fsck` |
| P1-11 | Git clone handles auth (HTTPS) | Integration test |
| P1-12 | Backup written to configured path | Verify structure |
| P1-13 | Directory structure uses project key | Verify |
| P1-14 | Repos nested under correct project | Cross-reference |
| P1-15 | Manifest.json created | Schema validation |
| P1-16 | CLI --help shows usage | Smoke test |
| P1-17 | CLI exits 0 success, non-zero failure | Script test |
| P1-18 | Credentials never in logs | Audit logs |
| P1-19 | Partial failure doesn't corrupt backups | Kill test |

### Phase 2

| ID | Criterion | Test Method |
|----|-----------|-------------|
| P2-01 | repository.json contains full metadata | Compare with API |
| P2-02 | All PRs exported (all states) | Count matches |
| P2-03 | PR JSON has all fields | Spot check |
| P2-04 | PR comments include inline with diff context | Verify inline field |
| P2-05 | PR approvals exported | Spot check |
| P2-06 | Issues exported if enabled | Count matches |
| P2-07 | Issue comments ordered correctly | Verify ordering |
| P2-08 | Handles repos with 0 PRs/issues | Test empty repo |
| P2-09 | Handles pagination (>100 items) | Test large repo |

## Testing Strategy

Since no test workspace is available:

1. **Unit tests** - All API methods tested with recorded/mocked responses
2. **Recorded responses** - Capture sanitised API responses as fixtures
3. **Integration test mode** - `--integration-test` limits to 1-2 repos
4. **Dry run** - `--dry-run` exercises code without writes
5. **Manual validation** - Checklist for release testing

## Dependencies

**Required:**
- Go 1.21+
- `git` CLI installed and in PATH

**Go dependencies (minimal):**
- `gopkg.in/yaml.v3` - Config parsing
- `github.com/spf13/cobra` - CLI framework
- Standard library for HTTP, JSON, etc.

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| API rate limits | Slow/failed backup | Backoff, configurable parallelism |
| API changes | Backup fails | Pin to API v2, monitor deprecations |
| Large repos timeout | Incomplete backup | Configurable timeout, resume |
| Permissions unclear | Missing data | Document required scopes |
