package restic

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/8ff/restic-sentry/internal/config"
	"github.com/8ff/restic-sentry/internal/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	return logger.New()
}

func TestBackupArgsContainVSSOnWindows(t *testing.T) {
	// We can't fully test VSS on macOS, but we can verify the arg logic
	// by checking what the runner would build
	cfg := &config.Config{
		ResticBinary: "echo",
		Paths:        []string{"/tmp/test"},
		Excludes:     []string{"*.tmp", "~$*"},
		S3:           config.S3Config{Bucket: "b", AccessKey: "a", SecretKey: "s"},
	}

	log := newTestLogger(t)
	runner := NewRunner(cfg, log)

	result, err := runner.Backup(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// echo prints its args to stdout — verify what was passed
	output := result.Stdout
	if !strings.Contains(output, "backup") {
		t.Errorf("expected 'backup' in args, got: %s", output)
	}
	if !strings.Contains(output, "--verbose") {
		t.Errorf("expected '--verbose' in args, got: %s", output)
	}
	if !strings.Contains(output, "--exclude") {
		t.Errorf("expected '--exclude' in args, got: %s", output)
	}

	if runtime.GOOS == "windows" {
		if !strings.Contains(output, "--use-fs-snapshot") {
			t.Errorf("expected '--use-fs-snapshot' on Windows, got: %s", output)
		}
	} else {
		if strings.Contains(output, "--use-fs-snapshot") {
			t.Errorf("should not include '--use-fs-snapshot' on %s", runtime.GOOS)
		}
	}
}

func TestCheckArgsSubset(t *testing.T) {
	cfg := &config.Config{
		ResticBinary: "echo",
		S3:           config.S3Config{Bucket: "b", AccessKey: "a", SecretKey: "s"},
	}

	log := newTestLogger(t)
	runner := NewRunner(cfg, log)

	result, _ := runner.Check(context.Background(), 5)
	if !strings.Contains(result.Stdout, "--read-data-subset=5%") {
		t.Errorf("expected subset arg, got: %s", result.Stdout)
	}

	result, _ = runner.Check(context.Background(), 0)
	if strings.Contains(result.Stdout, "--read-data-subset") {
		t.Errorf("should not include subset arg when 0, got: %s", result.Stdout)
	}
}

func TestForgetArgsRetention(t *testing.T) {
	cfg := &config.Config{
		ResticBinary: "echo",
		S3:           config.S3Config{Bucket: "b", AccessKey: "a", SecretKey: "s"},
		KeepLast:     5,
		KeepDaily:    7,
		KeepWeekly:   4,
		KeepMonthly:  12,
	}

	log := newTestLogger(t)
	runner := NewRunner(cfg, log)

	result, _ := runner.Forget(context.Background())
	output := result.Stdout

	for _, want := range []string{"--prune", "--keep-last=5", "--keep-daily=7", "--keep-weekly=4", "--keep-monthly=12"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in args, got: %s", want, output)
		}
	}
}

func TestPreflightBinaryNotFound(t *testing.T) {
	cfg := &config.Config{
		ResticBinary: "nonexistent-binary-that-does-not-exist-12345",
		S3:           config.S3Config{Bucket: "b", AccessKey: "a", SecretKey: "s"},
		Paths:        []string{"/tmp"},
	}

	log := newTestLogger(t)
	runner := NewRunner(cfg, log)

	err := runner.Preflight(context.Background())
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestResticEnvVars(t *testing.T) {
	cfg := &config.Config{
		S3: config.S3Config{
			Bucket:    "my-bucket",
			AccessKey: "AKIA1234",
			SecretKey: "secretXYZ",
			Region:    "eu-west-1",
		},
		ResticPassword: "hunter2",
	}

	env := cfg.ResticEnv()

	expected := map[string]string{
		"RESTIC_REPOSITORY":      "s3:s3.eu-west-1.amazonaws.com/my-bucket",
		"RESTIC_PASSWORD":        "hunter2",
		"AWS_ACCESS_KEY_ID":      "AKIA1234",
		"AWS_SECRET_ACCESS_KEY":  "secretXYZ",
		"AWS_DEFAULT_REGION":     "eu-west-1",
	}

	envMap := make(map[string]string)
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	for k, want := range expected {
		got, ok := envMap[k]
		if !ok {
			t.Errorf("missing env var %s", k)
			continue
		}
		if got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestInitRepoAlreadyExists(t *testing.T) {
	// Use a script that returns exit 0 for "snapshots" — simulates existing repo
	if runtime.GOOS == "windows" {
		t.Skip("test uses unix shell script")
	}

	dir := t.TempDir()
	script := filepath.Join(dir, "fake-restic")
	os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0755)

	cfg := &config.Config{
		ResticBinary: script,
		S3:           config.S3Config{Bucket: "b", AccessKey: "a", SecretKey: "s"},
	}

	log := newTestLogger(t)
	runner := NewRunner(cfg, log)

	err := runner.InitRepo(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
