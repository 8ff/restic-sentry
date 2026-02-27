package backup

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/8ff/restic-sentry/internal/config"
	"github.com/8ff/restic-sentry/internal/logger"
	"github.com/8ff/restic-sentry/internal/restic"
	"github.com/8ff/restic-sentry/internal/slack"
)

type Orchestrator struct {
	cfg    *config.Config
	runner *restic.Runner
	slack  *slack.Client
	log    *logger.Logger
}

func NewOrchestrator(cfg *config.Config, log *logger.Logger) *Orchestrator {
	return &Orchestrator{
		cfg:    cfg,
		runner: restic.NewRunner(cfg, log),
		slack:  slack.NewClient(cfg.SlackWebhookURL),
		log:    log,
	}
}

func (o *Orchestrator) SetDebug(debug bool) {
	o.runner.Debug = debug
}

// Run executes the full backup pipeline:
// preflight -> unlock -> init -> backup (with retry) -> check -> forget -> notify
func (o *Orchestrator) Run(ctx context.Context) error {
	started := time.Now()
	o.log.Info("=== backup run starting ===")

	// 1. Preflight checks
	o.log.Info("step 1/6: preflight checks")
	if err := o.runner.Preflight(ctx); err != nil {
		o.notifyError("Preflight Failed", err.Error())
		return fmt.Errorf("preflight: %w", err)
	}

	// 2. Remove stale locks
	o.log.Info("step 2/6: removing stale locks")
	if err := o.runner.Unlock(ctx); err != nil {
		o.log.Warn("unlock failed, continuing anyway", map[string]any{"error": err.Error()})
	}

	// 3. Init repo if needed
	o.log.Info("step 3/6: ensuring repository exists")
	if err := o.runner.InitRepo(ctx); err != nil {
		o.notifyError("Repository Init Failed", err.Error())
		return fmt.Errorf("init repo: %w", err)
	}

	// 4. Run backup with retry
	o.log.Info("step 4/6: running backup")
	backupResult, err := o.runWithRetry(ctx)
	if err != nil {
		o.notifyError("Backup Failed", err.Error())
		return fmt.Errorf("backup: %w", err)
	}

	// Parse exit code
	switch backupResult.ExitCode {
	case restic.ExitSuccess:
		o.log.Info("backup completed successfully")
	case restic.ExitPartial:
		o.log.Warn("backup completed with warnings — some files may have been skipped", map[string]any{
			"stderr": truncate(backupResult.Stderr, 500),
		})
	case restic.ExitFatal:
		o.notifyError("Backup Fatal Error",
			fmt.Sprintf("Restic exited with fatal error.\n```\n%s\n```", truncate(backupResult.Stderr, 1000)))
		return fmt.Errorf("restic fatal error (exit 1)")
	}

	// 5. Post-backup integrity check (subset)
	o.log.Info("step 5/6: integrity check")
	checkResult, err := o.runner.Check(ctx, o.cfg.CheckSubsetPercent)
	if err != nil {
		o.log.Warn("check command failed to run", map[string]any{"error": err.Error()})
	} else if checkResult.ExitCode != 0 {
		o.notifyError("Integrity Check Failed",
			fmt.Sprintf("restic check failed (exit %d).\n```\n%s\n```", checkResult.ExitCode, truncate(checkResult.Stderr, 1000)))
		return fmt.Errorf("integrity check failed")
	} else {
		o.log.Info("integrity check passed")
	}

	// 6. Apply retention policy
	o.log.Info("step 6/6: applying retention policy")
	forgetResult, err := o.runner.Forget(ctx)
	if err != nil {
		o.log.Warn("forget/prune failed", map[string]any{"error": err.Error()})
	} else if forgetResult.ExitCode != 0 {
		o.log.Warn("forget/prune returned non-zero", map[string]any{
			"exit_code": forgetResult.ExitCode,
			"stderr":    truncate(forgetResult.Stderr, 500),
		})
	}

	duration := time.Since(started)

	// Notify
	if backupResult.ExitCode == restic.ExitPartial {
		skipped := extractErrors(backupResult.Stderr, 20)
		extra := fmt.Sprintf("*%d files skipped:*\n```\n%s\n```", countErrors(backupResult.Stderr), skipped)
		o.notifyWarning("Backup Completed with Warnings",
			formatSummary(backupResult, duration, extra))
	} else {
		o.notifySuccess("Backup Successful",
			formatSummary(backupResult, duration, ""))
	}

	o.log.Info("=== backup run completed ===", map[string]any{
		"duration":  duration.String(),
		"exit_code": backupResult.ExitCode,
	})

	return nil
}

// runWithRetry retries the backup on transient (fatal) failures with exponential backoff.
// Partial successes (exit code 3) are NOT retried — the snapshot was created.
func (o *Orchestrator) runWithRetry(ctx context.Context) (*restic.Result, error) {
	var lastResult *restic.Result
	var lastErr error

	for attempt := 1; attempt <= o.cfg.Retry.MaxAttempts; attempt++ {
		o.log.Info("backup attempt", map[string]any{"attempt": attempt, "max": o.cfg.Retry.MaxAttempts})

		lastResult, lastErr = o.runner.Backup(ctx)
		if lastErr != nil {
			o.log.Error("backup execution error", map[string]any{
				"attempt": attempt,
				"error":   lastErr.Error(),
			})
			if attempt < o.cfg.Retry.MaxAttempts {
				delay := o.backoffDelay(attempt)
				o.log.Info("retrying after delay", map[string]any{"delay": delay.String()})
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			return nil, fmt.Errorf("all %d attempts failed: %w", o.cfg.Retry.MaxAttempts, lastErr)
		}

		// Exit code 0 or 3 — snapshot was created, don't retry
		if lastResult.ExitCode == restic.ExitSuccess || lastResult.ExitCode == restic.ExitPartial {
			return lastResult, nil
		}

		// Exit code 1 (fatal) — retry
		o.log.Warn("backup returned fatal exit code", map[string]any{
			"attempt":   attempt,
			"exit_code": lastResult.ExitCode,
			"stderr":    truncate(lastResult.Stderr, 300),
		})

		if attempt < o.cfg.Retry.MaxAttempts {
			// Unlock before retry in case a stale lock was left
			o.runner.Unlock(ctx)
			delay := o.backoffDelay(attempt)
			o.log.Info("retrying after delay", map[string]any{"delay": delay.String()})
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return lastResult, fmt.Errorf("all %d attempts returned fatal errors", o.cfg.Retry.MaxAttempts)
}

func (o *Orchestrator) backoffDelay(attempt int) time.Duration {
	secs := float64(o.cfg.Retry.BaseDelaySec) * math.Pow(2, float64(attempt-1))
	return time.Duration(secs) * time.Second
}

func (o *Orchestrator) notifySuccess(title, details string) {
	if err := o.slack.NotifySuccess(title, details); err != nil {
		o.log.Error("failed to send slack success notification", map[string]any{"error": err.Error()})
	}
}

func (o *Orchestrator) notifyWarning(title, details string) {
	if err := o.slack.NotifyWarning(title, details); err != nil {
		o.log.Error("failed to send slack warning notification", map[string]any{"error": err.Error()})
	}
}

func (o *Orchestrator) notifyError(title, details string) {
	if err := o.slack.NotifyError(title, details); err != nil {
		o.log.Error("failed to send slack error notification", map[string]any{"error": err.Error()})
	}
}

func formatSummary(result *restic.Result, duration time.Duration, extra string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("*Duration:* %s\n", duration.Round(time.Second)))

	// Extract useful lines from restic output
	lines := strings.Split(result.Stderr, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Added to the repository:") ||
			strings.Contains(line, "processed") ||
			strings.Contains(line, "snapshot") ||
			strings.Contains(line, "Files:") {
			sb.WriteString(line + "\n")
		}
	}

	if extra != "" {
		sb.WriteString("\n" + extra)
	}

	return sb.String()
}

// extractErrors pulls error/warning lines from restic stderr output.
// Each line includes the file path and reason (e.g. "Access is denied", "locked").
// Returns up to maxLines lines to keep Slack messages reasonable.
func extractErrors(stderr string, maxLines int) string {
	var errors []string
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "warning:") {
			errors = append(errors, line)
		}
	}
	total := len(errors)
	if total > maxLines {
		errors = errors[:maxLines]
		errors = append(errors, fmt.Sprintf("... and %d more", total-maxLines))
	}
	return strings.Join(errors, "\n")
}

// countErrors returns the total number of error/warning lines in restic stderr.
func countErrors(stderr string) int {
	count := 0
	for _, line := range strings.Split(stderr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "error:") || strings.HasPrefix(line, "warning:") {
			count++
		}
	}
	return count
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... (truncated)"
}
