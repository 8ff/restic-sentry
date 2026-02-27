package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type S3Config struct {
	Endpoint  string `json:"endpoint"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	Region    string `json:"region"`
}

type RetryConfig struct {
	MaxAttempts int `json:"max_attempts"` // default 3
	BaseDelaySec int `json:"base_delay_sec"` // default 30
}

type Config struct {
	// S3 backend
	S3 S3Config `json:"s3"`

	// Restic password for repo encryption
	ResticPassword string `json:"restic_password"`

	// Path to restic binary (default: "restic" i.e. on PATH)
	ResticBinary string `json:"restic_binary"`

	// Paths to back up
	Paths []string `json:"paths"`

	// Exclude patterns (passed as --exclude to restic)
	Excludes []string `json:"excludes"`

	// Slack webhook URL for notifications
	SlackWebhookURL string `json:"slack_webhook_url"`

	// Schedule interval in hours (for Task Scheduler)
	ScheduleIntervalHours int `json:"schedule_interval_hours"`

	// Retry settings for transient failures
	Retry RetryConfig `json:"retry"`

	// Retention policy — how many snapshots to keep
	KeepLast    int `json:"keep_last"`     // default 0 = disabled
	KeepDaily   int `json:"keep_daily"`    // default 7
	KeepWeekly  int `json:"keep_weekly"`   // default 4
	KeepMonthly int `json:"keep_monthly"`  // default 12

	// Integrity check subset percentage (default 5)
	CheckSubsetPercent int `json:"check_subset_percent"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	cfg := &Config{}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	cfg.applyDefaults()

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.ResticBinary == "" {
		c.ResticBinary = `C:\restic\restic.exe`
	}
	if c.Retry.MaxAttempts == 0 {
		c.Retry.MaxAttempts = 3
	}
	if c.Retry.BaseDelaySec == 0 {
		c.Retry.BaseDelaySec = 30
	}
	if c.ScheduleIntervalHours == 0 {
		c.ScheduleIntervalHours = 6
	}
	if c.KeepDaily == 0 {
		c.KeepDaily = 7
	}
	if c.KeepWeekly == 0 {
		c.KeepWeekly = 4
	}
	if c.KeepMonthly == 0 {
		c.KeepMonthly = 12
	}
	if c.CheckSubsetPercent == 0 {
		c.CheckSubsetPercent = 5
	}
}

func (c *Config) validate() error {
	if c.S3.Bucket == "" {
		return fmt.Errorf("s3.bucket is required")
	}
	if c.S3.AccessKey == "" {
		return fmt.Errorf("s3.access_key is required")
	}
	if c.S3.SecretKey == "" {
		return fmt.Errorf("s3.secret_key is required")
	}
	if c.ResticPassword == "" {
		return fmt.Errorf("restic_password is required")
	}
	if len(c.Paths) == 0 {
		return fmt.Errorf("at least one path is required")
	}
	for _, p := range c.Paths {
		if p == "" {
			return fmt.Errorf("empty path in paths array")
		}
	}
	if c.SlackWebhookURL == "" {
		return fmt.Errorf("slack_webhook_url is required")
	}
	return nil
}

// RepoURL returns the restic repository URL for S3.
func (c *Config) RepoURL() string {
	if c.S3.Endpoint != "" {
		return fmt.Sprintf("s3:%s/%s", c.S3.Endpoint, c.S3.Bucket)
	}
	if c.S3.Region != "" {
		return fmt.Sprintf("s3:s3.%s.amazonaws.com/%s", c.S3.Region, c.S3.Bucket)
	}
	return fmt.Sprintf("s3:s3.amazonaws.com/%s", c.S3.Bucket)
}

// ResticEnv returns the environment variables restic needs.
func (c *Config) ResticEnv() []string {
	env := os.Environ()
	env = append(env,
		"RESTIC_REPOSITORY="+c.RepoURL(),
		"RESTIC_PASSWORD="+c.ResticPassword,
		"AWS_ACCESS_KEY_ID="+c.S3.AccessKey,
		"AWS_SECRET_ACCESS_KEY="+c.S3.SecretKey,
	)
	if c.S3.Region != "" {
		env = append(env, "AWS_DEFAULT_REGION="+c.S3.Region)
	}
	return env
}

func DefaultConfigPath() string {
	exe, err := os.Executable()
	if err != nil {
		return "restic-sentry.json"
	}
	return filepath.Join(filepath.Dir(exe), "restic-sentry.json")
}

func WriteExample(path string) error {
	example := &Config{
		S3: S3Config{
			Bucket:    "my-backups",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
			SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			Region:    "us-east-1",
		},
		ResticBinary:          `C:\restic\restic.exe`,
		ResticPassword:        "change-me-to-a-strong-password",
		Paths:                 []string{`C:\Users\Me\Documents\Financial`},
		Excludes:              []string{"*.tmp", "~$*"},
		SlackWebhookURL:       "https://hooks.slack.com/services/YOUR/SLACK/WEBHOOK",
		ScheduleIntervalHours: 6,
		Retry: RetryConfig{
			MaxAttempts:  3,
			BaseDelaySec: 30,
		},
		KeepDaily:          7,
		KeepWeekly:         4,
		KeepMonthly:        12,
		CheckSubsetPercent: 5,
	}

	data, err := json.MarshalIndent(example, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
