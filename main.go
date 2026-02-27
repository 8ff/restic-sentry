package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/8ff/restic-sentry/internal/backup"
	"github.com/8ff/restic-sentry/internal/config"
	"github.com/8ff/restic-sentry/internal/install"
	"github.com/8ff/restic-sentry/internal/lockfile"
	"github.com/8ff/restic-sentry/internal/logger"
	"github.com/8ff/restic-sentry/internal/restic"
	"github.com/8ff/restic-sentry/internal/scheduler"
)

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	cmd := os.Args[1]

	// Handle version and help before anything else
	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("restic-sentry %s\n", version)
		return
	case "help", "--help", "-h":
		printUsage()
		return
	case "init-config":
		path := config.DefaultConfigPath()
		if len(os.Args) > 2 {
			path = os.Args[2]
		}
		if err := config.WriteExample(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing example config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Example config written to %s\n", path)
		fmt.Println("Edit this file with your S3 credentials, restic password, backup paths, and Slack webhook URL.")
		return
	case "install-restic":
		if _, err := install.InstallRestic(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	case "update":
		if err := install.SelfUpdate(version); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// All other commands need a config file
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	configPath := fs.String("config", config.DefaultConfigPath(), "path to config file")
	debug := fs.Bool("debug", false, "print full commands with credentials (for manual testing)")
	fs.Parse(os.Args[2:])

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config from %s: %v\n", *configPath, err)
		os.Exit(1)
	}

	log := logger.New()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	switch cmd {
	case "backup":
		runBackup(ctx, cfg, log, *debug)
	case "check":
		runCheck(ctx, cfg, log, *debug)
	case "status":
		runStatus(ctx, cfg, log, *debug)
	case "install":
		runInstall(cfg, log, *configPath)
	case "uninstall":
		runUninstall(log)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		printUsage()
		os.Exit(1)
	}
}

func newRunner(cfg *config.Config, log *logger.Logger, debug bool) *restic.Runner {
	r := restic.NewRunner(cfg, log)
	r.Debug = debug
	return r
}

func runBackup(ctx context.Context, cfg *config.Config, log *logger.Logger, debug bool) {
	// Acquire process lock — auto-clears stale locks from dead processes
	lock, err := lockfile.New(0)
	if err != nil {
		log.Error("failed to create lock", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	if err := lock.Acquire(); err != nil {
		log.Error("cannot start backup", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	defer lock.Release()

	orch := backup.NewOrchestrator(cfg, log)
	orch.SetDebug(debug)
	if err := orch.Run(ctx); err != nil {
		log.Error("backup failed", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
}

func runCheck(ctx context.Context, cfg *config.Config, log *logger.Logger, debug bool) {
	runner := newRunner(cfg, log, debug)

	log.Info("running full integrity check with data read")
	result, err := runner.Check(ctx, 100) // full read
	if err != nil {
		log.Error("check failed to execute", map[string]any{"error": err.Error()})
		os.Exit(1)
	}

	fmt.Println(result.Stdout)
	if result.Stderr != "" {
		fmt.Fprintln(os.Stderr, result.Stderr)
	}

	if result.ExitCode != 0 {
		log.Error("integrity check found issues", map[string]any{"exit_code": result.ExitCode})
		os.Exit(1)
	}

	log.Info("integrity check passed — all data verified")
}

func runStatus(ctx context.Context, cfg *config.Config, log *logger.Logger, debug bool) {
	runner := newRunner(cfg, log, debug)

	fmt.Println("=== Repository Snapshots ===")
	result, err := runner.Snapshots(ctx)
	if err != nil {
		log.Error("failed to list snapshots", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	fmt.Println(result.Stdout)

	fmt.Println("\n=== Repository Stats ===")
	result, err = runner.Stats(ctx)
	if err != nil {
		log.Error("failed to get stats", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	fmt.Println(result.Stdout)

	// Show scheduler status on Windows
	sched := scheduler.New(log)
	fmt.Println("\n=== Scheduled Task ===")
	if err := sched.Status(); err != nil {
		fmt.Println("No scheduled task found (run 'restic-sentry install' to set up)")
	}
}

func runInstall(cfg *config.Config, log *logger.Logger, configPath string) {
	sched := scheduler.New(log)
	if err := sched.Install(configPath, cfg.ScheduleIntervalHours); err != nil {
		log.Error("failed to install scheduled task", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	fmt.Printf("Scheduled task installed — backup will run every %d hours with highest privileges (required for VSS).\n", cfg.ScheduleIntervalHours)
}

func runUninstall(log *logger.Logger) {
	sched := scheduler.New(log)
	if err := sched.Uninstall(); err != nil {
		log.Error("failed to uninstall scheduled task", map[string]any{"error": err.Error()})
		os.Exit(1)
	}
	fmt.Println("Scheduled task removed.")
}

func printUsage() {
	fmt.Printf(`restic-sentry %s — Reliable Windows backup manager using restic

Usage:
  restic-sentry <command> [flags]

Commands:
  backup          Run a full backup (preflight, backup, verify, prune, notify)
  check           Run a full integrity check with data verification
  status          Show snapshots, repo stats, and scheduler status
  install         Register in Windows Task Scheduler (run as admin)
  uninstall       Remove from Windows Task Scheduler
  init-config     Generate an example config file
  install-restic  Download and install latest restic to C:\restic
  update          Self-update to the latest restic-sentry release
  version         Print version

Flags:
  --config     Path to config JSON file (default: restic-sentry.json next to binary)
  --debug      Print full restic commands with credentials visible (for manual testing)

Examples:
  restic-sentry install-restic                     # download restic
  restic-sentry init-config                        # create example config
  restic-sentry backup                             # run backup (uses default config path)
  restic-sentry backup --config C:\mybackup.json   # run backup with specific config
  restic-sentry install                            # schedule automatic backups
  restic-sentry update                             # self-update to latest version
`, version)
}
