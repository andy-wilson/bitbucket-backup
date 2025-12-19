# Quick Start Guide

This guide will help you get started with `bb-backup` in under 5 minutes.

## Prerequisites

- `git` CLI installed and in your PATH
- A Bitbucket Cloud account with workspace access

## Step 1: Install bb-backup

### Option A: Download binary

Download the latest release for your platform from the [Releases](https://github.com/andy-wilson/bb-backup/releases) page.

```bash
# Linux (amd64)
curl -LO https://github.com/andy-wilson/bb-backup/releases/latest/download/bb-backup-linux-amd64
chmod +x bb-backup-linux-amd64
sudo mv bb-backup-linux-amd64 /usr/local/bin/bb-backup

# macOS (Apple Silicon)
curl -LO https://github.com/andy-wilson/bb-backup/releases/latest/download/bb-backup-darwin-arm64
chmod +x bb-backup-darwin-arm64
sudo mv bb-backup-darwin-arm64 /usr/local/bin/bb-backup
```

### Option B: Build from source

```bash
git clone https://github.com/andy-wilson/bb-backup.git
cd bb-backup
make build
sudo mv bin/bb-backup /usr/local/bin/
```

Verify installation:

```bash
bb-backup version
```

## Step 2: Create a Bitbucket App Password

1. Go to [Bitbucket App Passwords](https://bitbucket.org/account/settings/app-passwords/)
2. Click **Create app password**
3. Give it a name like "bb-backup"
4. Select these permissions:
   - **Account**: Read
   - **Workspace membership**: Read
   - **Projects**: Read
   - **Repositories**: Read
   - **Pull requests**: Read
   - **Issues**: Read
5. Click **Create**
6. **Copy the password** - you won't be able to see it again!

## Step 3: Set Up Credentials

Set your credentials as environment variables:

```bash
export BITBUCKET_USERNAME="your-bitbucket-username"
export BITBUCKET_APP_PASSWORD="your-app-password"
```

To make these permanent, add them to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.):

```bash
echo 'export BITBUCKET_USERNAME="your-username"' >> ~/.bashrc
echo 'export BITBUCKET_APP_PASSWORD="your-app-password"' >> ~/.bashrc
source ~/.bashrc
```

## Step 4: List Your Repositories

Before backing up, preview what will be included:

```bash
bb-backup list -w your-workspace
```

This shows all projects and repositories in your workspace.

For detailed output:

```bash
bb-backup list -w your-workspace -v
```

## Step 5: Run Your First Backup

### Quick backup (no config file)

```bash
bb-backup backup -w your-workspace -o ./backups
```

### With a config file (recommended)

Create `bb-backup.yaml`:

```yaml
workspace: "your-workspace"

auth:
  method: "app_password"
  username: "${BITBUCKET_USERNAME}"
  app_password: "${BITBUCKET_APP_PASSWORD}"

storage:
  type: "local"
  path: "./backups"

parallelism:
  git_workers: 4
```

Run backup:

```bash
bb-backup backup -c bb-backup.yaml
```

## Step 6: Verify Your Backup

After the backup completes, verify its integrity:

```bash
bb-backup verify ./backups/your-workspace
```

This checks:
- All git repositories are valid (`git fsck`)
- All JSON metadata files are valid
- The manifest is complete

## What Gets Backed Up?

| Data | Location |
|------|----------|
| Git repositories | `repo.git/` (mirror clone) |
| Repository metadata | `repository.json` |
| Pull requests | `pull-requests/*.json` |
| PR comments | `pull-requests/*/comments.json` |
| PR activity | `pull-requests/*/activity.json` |
| Issues | `issues/*.json` |
| Issue comments | `issues/*/comments.json` |
| Project metadata | `project.json` |
| Workspace metadata | `workspace.json` |

## Common Use Cases

### Dry run (preview without backup)

```bash
bb-backup backup --dry-run
```

### Backup specific repositories

```bash
# Only repos starting with "core-"
bb-backup backup --include "core-*"

# Exclude test repos
bb-backup backup --exclude "test-*" --exclude "archive-*"
```

### Incremental backup (after first run)

```bash
# First backup (full)
bb-backup backup

# Subsequent backups (incremental - only changes)
bb-backup backup
```

### Force full backup

```bash
bb-backup backup --full
```

### JSON progress output (for automation)

```bash
bb-backup backup --json-progress
```

### Parallel backup (faster)

```bash
bb-backup backup --parallel 8
```

## Setting Up Scheduled Backups

### Using cron (Linux/macOS)

Edit your crontab:

```bash
crontab -e
```

Add a daily backup at 2 AM:

```cron
0 2 * * * BITBUCKET_USERNAME=user BITBUCKET_APP_PASSWORD=pass /usr/local/bin/bb-backup backup -c /path/to/bb-backup.yaml >> /var/log/bb-backup.log 2>&1
```

### Using systemd timer (Linux)

Create `/etc/systemd/system/bb-backup.service`:

```ini
[Unit]
Description=Bitbucket Backup
After=network.target

[Service]
Type=oneshot
User=backup
Environment="BITBUCKET_USERNAME=user"
Environment="BITBUCKET_APP_PASSWORD=pass"
ExecStart=/usr/local/bin/bb-backup backup -c /etc/bb-backup/config.yaml
```

Create `/etc/systemd/system/bb-backup.timer`:

```ini
[Unit]
Description=Daily Bitbucket Backup

[Timer]
OnCalendar=*-*-* 02:00:00
Persistent=true

[Install]
WantedBy=timers.target
```

Enable and start:

```bash
sudo systemctl enable bb-backup.timer
sudo systemctl start bb-backup.timer
```

## Troubleshooting

### "rate limited" errors

Bitbucket limits API requests to ~1000/hour. For large workspaces:
- Reduce `requests_per_hour` in config (default: 900)
- Use `--parallel 2` to reduce concurrent requests
- Run backups during off-peak hours

### "authentication failed"

- Verify your app password has the required scopes
- Check that `BITBUCKET_USERNAME` is your Bitbucket username (not email)
- Ensure the app password hasn't expired

### "git clone failed"

- Ensure `git` is installed: `git --version`
- Check you have read access to the repository
- For large repos, increase timeout in config

### Checking backup integrity

```bash
# Quick check
bb-backup verify ./backups/your-workspace

# Detailed check
bb-backup verify ./backups/your-workspace -v

# JSON output for scripts
bb-backup verify ./backups/your-workspace --json
```

## Next Steps

- Read the full [README](README.md) for advanced configuration
- Check [CHANGELOG](CHANGELOG.md) for latest features
- See [configs/example.yaml](configs/example.yaml) for all config options
