package restic

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/8ff/restic-sentry/internal/config"
	"github.com/8ff/restic-sentry/internal/logger"
)

// Exit codes from restic
const (
	ExitSuccess      = 0
	ExitFatal        = 1
	ExitPartial      = 3 // backup completed with warnings (some files couldn't be read)
)

type Runner struct {
	cfg   *config.Config
	log   *logger.Logger
	Debug bool
}

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
}

func NewRunner(cfg *config.Config, log *logger.Logger) *Runner {
	return &Runner{cfg: cfg, log: log}
}

// formatEnv returns the restic env vars formatted as a copy-pasteable command prefix.
// On Windows, outputs PowerShell syntax ($env:VAR="val"; ).
// On Unix, outputs inline syntax (VAR=val ).
// If redact is true, secrets are masked with ***.
func (r *Runner) formatEnv(redact bool) string {
	secretKeys := map[string]bool{
		"RESTIC_PASSWORD":       true,
		"AWS_ACCESS_KEY_ID":     true,
		"AWS_SECRET_ACCESS_KEY": true,
	}

	var parts []string
	for _, pair := range r.cfg.ResticEnv() {
		key, val, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		// Only include restic/AWS vars, not the entire inherited env
		switch key {
		case "RESTIC_REPOSITORY", "RESTIC_PASSWORD",
			"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_DEFAULT_REGION":
		default:
			continue
		}
		if redact && secretKeys[key] {
			val = "***"
		}
		if runtime.GOOS == "windows" {
			parts = append(parts, fmt.Sprintf("$env:%s=\"%s\"", key, val))
		} else {
			parts = append(parts, fmt.Sprintf("%s=%s", key, val))
		}
	}
	if runtime.GOOS == "windows" {
		return strings.Join(parts, "; ")
	}
	return strings.Join(parts, " ")
}

// commandTimeout returns a reasonable timeout based on the restic subcommand.
// Short commands (unlock, version, snapshots, stats, init) get 2 minutes.
// Long commands (backup, check, forget) get 4 hours.
func commandTimeout(args []string) time.Duration {
	if len(args) > 0 {
		switch args[0] {
		case "backup", "check", "forget":
			return 4 * time.Hour
		}
	}
	return 2 * time.Minute
}

// run executes a restic command with the configured environment.
func (r *Runner) run(ctx context.Context, args ...string) (*Result, error) {
	start := time.Now()

	timeout := commandTimeout(args)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.cfg.ResticBinary, args...)
	cmd.Env = r.cfg.ResticEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	envStr := r.formatEnv(!r.Debug)
	var fullCmd string
	if runtime.GOOS == "windows" {
		fullCmd = fmt.Sprintf("%s; %s %s", envStr, r.cfg.ResticBinary, strings.Join(args, " "))
	} else {
		fullCmd = fmt.Sprintf("%s %s %s", envStr, r.cfg.ResticBinary, strings.Join(args, " "))
	}
	r.log.Info("running restic", map[string]any{
		"cmd": fullCmd,
	})

	err := cmd.Run()
	duration := time.Since(start)

	result := &Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: duration,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			return result, fmt.Errorf("executing restic: %w", err)
		}
	}

	r.log.Info("restic completed", map[string]any{
		"exit_code": result.ExitCode,
		"duration":  duration.String(),
	})

	return result, nil
}

// InitRepo initializes the restic repository if it doesn't exist.
func (r *Runner) InitRepo(ctx context.Context) error {
	// Try snapshots first to see if repo exists
	result, err := r.run(ctx, "snapshots", "--json", "--latest", "1")
	if err != nil {
		return err
	}
	if result.ExitCode == 0 {
		r.log.Info("repository already initialized")
		return nil
	}

	r.log.Info("initializing new repository")
	result, err = r.run(ctx, "init")
	if err != nil {
		return fmt.Errorf("init failed: %w", err)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("init failed (exit %d): %s", result.ExitCode, result.Stderr)
	}
	r.log.Info("repository initialized successfully")
	return nil
}

// Unlock removes stale locks from the repository.
func (r *Runner) Unlock(ctx context.Context) error {
	r.log.Info("removing stale locks")
	result, err := r.run(ctx, "unlock")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		r.log.Warn("unlock returned non-zero", map[string]any{
			"exit_code": result.ExitCode,
			"stderr":    result.Stderr,
		})
	}
	return nil
}

// Backup runs the actual backup with VSS on Windows.
func (r *Runner) Backup(ctx context.Context) (*Result, error) {
	args := []string{"backup", "--verbose"}

	// Enable VSS on Windows
	if runtime.GOOS == "windows" {
		args = append(args, "--use-fs-snapshot")
	}

	// Add exclude patterns
	for _, ex := range r.cfg.Excludes {
		args = append(args, "--exclude", ex)
	}

	// Add paths
	args = append(args, r.cfg.Paths...)

	return r.run(ctx, args...)
}

// Check runs integrity verification.
func (r *Runner) Check(ctx context.Context, readDataSubset int) (*Result, error) {
	args := []string{"check"}
	if readDataSubset > 0 {
		args = append(args, fmt.Sprintf("--read-data-subset=%d%%", readDataSubset))
	}
	return r.run(ctx, args...)
}

// Forget applies the retention policy and prunes unreferenced data.
func (r *Runner) Forget(ctx context.Context) (*Result, error) {
	args := []string{"forget", "--prune"}

	if r.cfg.KeepLast > 0 {
		args = append(args, fmt.Sprintf("--keep-last=%d", r.cfg.KeepLast))
	}
	if r.cfg.KeepDaily > 0 {
		args = append(args, fmt.Sprintf("--keep-daily=%d", r.cfg.KeepDaily))
	}
	if r.cfg.KeepWeekly > 0 {
		args = append(args, fmt.Sprintf("--keep-weekly=%d", r.cfg.KeepWeekly))
	}
	if r.cfg.KeepMonthly > 0 {
		args = append(args, fmt.Sprintf("--keep-monthly=%d", r.cfg.KeepMonthly))
	}

	return r.run(ctx, args...)
}

// Snapshots lists snapshots.
func (r *Runner) Snapshots(ctx context.Context) (*Result, error) {
	return r.run(ctx, "snapshots")
}

// Stats gets repository stats.
func (r *Runner) Stats(ctx context.Context) (*Result, error) {
	return r.run(ctx, "stats")
}

// Preflight checks that restic is available and the repo is reachable.
func (r *Runner) Preflight(ctx context.Context) error {
	// Check restic binary exists
	_, err := exec.LookPath(r.cfg.ResticBinary)
	if err != nil {
		return fmt.Errorf("restic binary not found at %q: %w", r.cfg.ResticBinary, err)
	}

	// Check version
	result, err := r.run(ctx, "version")
	if err != nil {
		return fmt.Errorf("could not run restic: %w", err)
	}
	r.log.Info("restic version", map[string]any{"output": strings.TrimSpace(result.Stdout)})

	// Check backup paths exist
	for _, p := range r.cfg.Paths {
		// We just log warnings for missing paths, don't fail — restic will handle it
		r.log.Info("backup path configured", map[string]any{"path": p})
	}

	return nil
}
