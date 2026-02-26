package backup

import (
	"strings"
	"testing"
	"time"

	"github.com/8ff/restic-sentry/internal/restic"
)

func TestFormatSummary(t *testing.T) {
	result := &restic.Result{
		ExitCode: 0,
		Stdout:   "",
		Stderr: `open repository
lock repository
load index files
using parent snapshot abc123
start scan on [C:\Users\Me\Documents]
start backup on [C:\Users\Me\Documents]

Files:           5 new,     0 changed,    10 unmodified
Added to the repository: 1.234 MiB

processed 15 files, 5.678 MiB in 0:02
snapshot def456 saved
`,
		Duration: 2 * time.Second,
	}

	summary := formatSummary(result, 5*time.Second, "")

	if summary == "" {
		t.Fatal("summary should not be empty")
	}

	for _, want := range []string{"Duration:", "Files:", "Added to the repository:", "processed", "snapshot"} {
		if !strings.Contains(summary, want) {
			t.Errorf("summary missing %q:\n%s", want, summary)
		}
	}
}

func TestFormatSummaryWithExtra(t *testing.T) {
	result := &restic.Result{
		Stderr: "snapshot abc saved\n",
	}

	summary := formatSummary(result, time.Second, "Warning: some files skipped")

	if !strings.Contains(summary, "Warning: some files skipped") {
		t.Errorf("summary missing extra text:\n%s", summary)
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if got := truncate(short, 10); got != short {
		t.Errorf("truncate(%q, 10) = %q", short, got)
	}

	long := "hello world this is a long string"
	got := truncate(long, 10)
	if len(got) > 30 {
		t.Errorf("truncate should be short, got len=%d: %q", len(got), got)
	}
	if !strings.Contains(got, "truncated") {
		t.Errorf("truncated string should contain 'truncated': %q", got)
	}
}
