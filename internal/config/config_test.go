package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	content := `{
		"s3": {
			"bucket": "my-backups",
			"access_key": "AKIA1234",
			"secret_key": "secret1234",
			"region": "us-east-1"
		},
		"restic_password": "testpass",
		"paths": ["C:\\Users\\Me\\Documents"],
		"slack_webhook_url": "https://hooks.slack.com/services/T/B/X",
		"schedule_interval_hours": 4
	}`

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.S3.Bucket != "my-backups" {
		t.Errorf("bucket = %q, want %q", cfg.S3.Bucket, "my-backups")
	}
	if cfg.ScheduleIntervalHours != 4 {
		t.Errorf("schedule = %d, want 4", cfg.ScheduleIntervalHours)
	}
	if cfg.Retry.MaxAttempts != 3 {
		t.Errorf("retry max_attempts = %d, want default 3", cfg.Retry.MaxAttempts)
	}
	if cfg.KeepDaily != 7 {
		t.Errorf("keep_daily = %d, want default 7", cfg.KeepDaily)
	}
	if cfg.CheckSubsetPercent != 5 {
		t.Errorf("check_subset = %d, want default 5", cfg.CheckSubsetPercent)
	}
}

func TestLoadMissingFields(t *testing.T) {
	tests := []struct {
		name    string
		json    string
		wantErr string
	}{
		{
			name:    "missing bucket",
			json:    `{"s3":{"access_key":"a","secret_key":"s"},"restic_password":"p","paths":["x"],"slack_webhook_url":"u"}`,
			wantErr: "s3.bucket is required",
		},
		{
			name:    "missing password",
			json:    `{"s3":{"bucket":"b","access_key":"a","secret_key":"s"},"paths":["x"],"slack_webhook_url":"u"}`,
			wantErr: "restic_password is required",
		},
		{
			name:    "missing paths",
			json:    `{"s3":{"bucket":"b","access_key":"a","secret_key":"s"},"restic_password":"p","slack_webhook_url":"u"}`,
			wantErr: "at least one path is required",
		},
		{
			name:    "missing slack",
			json:    `{"s3":{"bucket":"b","access_key":"a","secret_key":"s"},"restic_password":"p","paths":["x"]}`,
			wantErr: "slack_webhook_url is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			os.WriteFile(path, []byte(tt.json), 0600)

			_, err := Load(path)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if got := err.Error(); !strings.Contains(got, tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", got, tt.wantErr)
			}
		})
	}
}

func TestRepoURL(t *testing.T) {
	tests := []struct {
		name     string
		s3       S3Config
		expected string
	}{
		{
			name:     "with custom endpoint",
			s3:       S3Config{Endpoint: "https://s3.custom.com", Bucket: "bkt"},
			expected: "s3:https://s3.custom.com/bkt",
		},
		{
			name:     "with region",
			s3:       S3Config{Bucket: "bkt", Region: "eu-west-1"},
			expected: "s3:s3.eu-west-1.amazonaws.com/bkt",
		},
		{
			name:     "default",
			s3:       S3Config{Bucket: "bkt"},
			expected: "s3:s3.amazonaws.com/bkt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{S3: tt.s3}
			if got := cfg.RepoURL(); got != tt.expected {
				t.Errorf("RepoURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestWriteExample(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "example.json")

	if err := WriteExample(path); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("example config should be loadable: %v", err)
	}

	if cfg.S3.Bucket == "" {
		t.Error("example config should have a non-empty bucket")
	}
}
