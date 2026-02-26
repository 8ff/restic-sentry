# restic-sentry

A single Go binary that wraps [restic](https://restic.net/) for reliable, automated Windows backups to S3 with Slack notifications and Windows Task Scheduler integration.

Built for backing up financial documents and other critical files where reliability matters more than anything else.

## Features

### Backup Pipeline

Every `backup` run executes a full 6-step pipeline:

1. **Preflight checks** — verifies restic binary exists, prints version
2. **Stale lock removal** — runs `restic unlock` to clear locks left by crashed runs
3. **Auto repo init** — initializes the S3 repository on first run, no manual `restic init` needed
4. **Backup with VSS** — runs `restic backup --use-fs-snapshot` so locked files (Excel, QuickBooks, etc.) are backed up correctly
5. **Integrity check** — runs `restic check --read-data-subset=5%` to verify a portion of stored data after every backup
6. **Retention policy** — runs `restic forget --prune` with configurable keep-daily/weekly/monthly rules

### Reliability

- **VSS snapshots** (`--use-fs-snapshot`) — backs up files even when they're open/locked by another process. Requires running as Administrator
- **Retry with exponential backoff** — transient failures (network issues, S3 timeouts) are retried up to 3 times with increasing delays (30s, 60s, 120s). Partial successes (restic exit code 3) are *not* retried since the snapshot was created
- **Restic exit code parsing** — differentiates between success (0), fatal error (1), and partial completion with warnings (3). Each gets different handling and Slack notification color
- **Process lock file** — prevents overlapping runs when the scheduler fires while a backup is still in progress. The lock is PID-aware: if the holding process is dead, the lock is automatically cleared. A 6-hour max age acts as a safety net so backups never get permanently stuck
- **Stale restic lock cleanup** — runs `restic unlock` before every backup to clear repository locks left by crashed/killed restic processes

### Notifications

Slack webhook notifications with color-coded severity:

- **Green** — backup successful, shows duration, files processed, data added
- **Orange** — backup completed with warnings (some files were skipped due to permissions/locks)
- **Red** — backup failed, preflight failed, or integrity check failed

### Observability

Every restic command is logged to stderr with the full copy-pasteable command, including env vars (credentials masked with `***`):

```
RESTIC_REPOSITORY=s3:s3.us-east-1.amazonaws.com/my-backups RESTIC_PASSWORD=*** AWS_ACCESS_KEY_ID=*** AWS_SECRET_ACCESS_KEY=*** AWS_DEFAULT_REGION=us-east-1 restic backup --verbose --use-fs-snapshot --exclude *.tmp "C:\Users\Me\Documents\Financial"
```

You can grab any logged command, fill in your real credentials, and run it manually.

### Scheduling

Self-registers in Windows Task Scheduler via `schtasks.exe`. Runs with `HIGHEST` privileges (required for VSS). Configurable interval (default: every 6 hours).

## Commands

| Command | Description |
|---------|-------------|
| `backup` | Run the full 6-step backup pipeline |
| `check` | Run a full integrity check (`--read-data-subset=100%`) |
| `status` | Show repository snapshots, stats, and scheduler status |
| `install` | Register in Windows Task Scheduler |
| `uninstall` | Remove from Windows Task Scheduler |
| `init-config` | Generate an example config file |
| `version` | Print version |

All commands except `init-config` and `version` accept `--config <path>` (defaults to `restic-sentry.json` next to the binary).

## Quick Start

### 1. Build

```bash
# From source (cross-compile for Windows from any OS)
make build-windows    # produces restic-sentry.exe

# Or just for your current platform
make build
```

### 2. Configure

```powershell
# Generate example config
restic-sentry.exe init-config

# Edit restic-sentry.json with your values
```

### 3. Test

```powershell
# Run a backup manually to make sure everything works
restic-sentry.exe backup --config restic-sentry.json
```

### 4. Schedule

```powershell
# Run as Administrator — registers in Task Scheduler
restic-sentry.exe install --config restic-sentry.json
```

## Configuration

```jsonc
{
  // S3 backend
  "s3": {
    "endpoint": "",                    // custom S3 endpoint (leave empty for AWS)
    "bucket": "my-backups",
    "access_key": "AKIA...",
    "secret_key": "...",
    "region": "us-east-1"
  },

  // Restic repo encryption password
  "restic_password": "...",

  // Path to restic binary (default: "restic", i.e. on PATH)
  "restic_binary": "",

  // Directories to back up
  "paths": [
    "C:\\Users\\Me\\Documents\\Financial"
  ],

  // Exclude patterns (passed as --exclude to restic)
  "excludes": ["*.tmp", "~$*"],

  // Slack webhook URL
  "slack_webhook_url": "https://hooks.slack.com/services/T.../B.../...",

  // How often to run (hours) — used by 'install' command
  "schedule_interval_hours": 6,

  // Retry on transient failures
  "retry": {
    "max_attempts": 3,        // number of attempts
    "base_delay_sec": 30      // delay doubles each retry: 30s, 60s, 120s
  },

  // Retention policy
  "keep_last": 0,             // 0 = disabled
  "keep_daily": 7,
  "keep_weekly": 4,
  "keep_monthly": 12,

  // What percentage of data to verify after each backup
  "check_subset_percent": 5
}
```

## Project Structure

```
main.go                              CLI entry point, subcommand routing
internal/
  config/config.go                   JSON config loading, validation, defaults
  restic/restic.go                   Restic command runner (backup, check, forget, init, unlock)
  backup/backup.go                   Orchestrator: full pipeline with retry logic
  lockfile/lockfile.go               PID-based lock file with stale detection
  lockfile/process_unix.go           Unix process liveness check (kill -0)
  lockfile/process_windows.go        Windows process liveness check (OpenProcess)
  scheduler/scheduler.go             Windows Task Scheduler via schtasks.exe
  slack/slack.go                     Webhook notifications (green/orange/red)
  logger/logger.go                   Structured JSON logging to stderr
Makefile                             Build targets
```

## Prerequisites

- [restic](https://restic.net/) installed and on PATH (or provide full path in config)
- An S3 bucket (AWS, MinIO, Backblaze B2 with S3 compatibility, etc.)
- A Slack incoming webhook URL
- Windows (for VSS and Task Scheduler features; backup/check work on any OS)

## S3 Recommendations

- **Enable bucket versioning** — belt-and-suspenders against accidental deletion or repo corruption
- **Enable S3 Object Lock** — prevents even someone with your AWS keys from deleting backups (ransomware protection)
- **Use a dedicated IAM user** with permissions scoped to the backup bucket only

## License

MIT
