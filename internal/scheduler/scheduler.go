package scheduler

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/8ff/restic-sentry/internal/logger"
)

const taskName = "ResticSentry"

type Scheduler struct {
	log *logger.Logger
}

func New(log *logger.Logger) *Scheduler {
	return &Scheduler{log: log}
}

// Install registers a scheduled task to run "restic-guard backup" every intervalHours.
func (s *Scheduler) Install(configPath string, intervalHours int) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("scheduler install is only supported on Windows (current OS: %s)", runtime.GOOS)
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	// Remove existing task first (ignore errors if it doesn't exist)
	s.Uninstall()

	// schtasks /create
	// /sc HOURLY /mo <interval>
	// /tn <task name>
	// /tr "<exe> backup --config <config>"
	// /rl HIGHEST  (run with highest privileges — needed for VSS)
	// /f (force overwrite)
	taskCmd := fmt.Sprintf(`"%s" backup --config "%s"`, exe, configPath)

	args := []string{
		"/create",
		"/sc", "HOURLY",
		"/mo", strconv.Itoa(intervalHours),
		"/tn", taskName,
		"/tr", taskCmd,
		"/rl", "HIGHEST",
		"/f",
	}

	s.log.Info("creating scheduled task", map[string]any{
		"task_name": taskName,
		"interval":  fmt.Sprintf("every %d hours", intervalHours),
		"command":   taskCmd,
	})

	cmd := exec.Command("schtasks.exe", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("schtasks /create failed: %w", err)
	}

	s.log.Info("scheduled task created successfully")
	return nil
}

// Uninstall removes the scheduled task.
func (s *Scheduler) Uninstall() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("scheduler uninstall is only supported on Windows (current OS: %s)", runtime.GOOS)
	}

	s.log.Info("removing scheduled task", map[string]any{"task_name": taskName})

	cmd := exec.Command("schtasks.exe", "/delete", "/tn", taskName, "/f")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		s.log.Warn("schtasks /delete failed (task may not exist)", map[string]any{
			"error": err.Error(),
		})
		return err
	}

	s.log.Info("scheduled task removed successfully")
	return nil
}

// Status checks if the scheduled task exists and prints its status.
func (s *Scheduler) Status() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("scheduler status is only supported on Windows (current OS: %s)", runtime.GOOS)
	}

	cmd := exec.Command("schtasks.exe", "/query", "/tn", taskName, "/v", "/fo", "LIST")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("task %q not found or query failed: %w", taskName, err)
	}
	return nil
}
