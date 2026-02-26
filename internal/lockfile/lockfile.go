package lockfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const defaultMaxAge = 6 * time.Hour

type lockData struct {
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
	Hostname  string    `json:"hostname"`
}

type Lock struct {
	path   string
	maxAge time.Duration
}

// New creates a lock file manager. The lock file is placed next to the executable.
// maxAge is the maximum time a lock can be held before it's considered stale.
// If maxAge is 0, defaults to 6 hours.
func New(maxAge time.Duration) (*Lock, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("getting executable path: %w", err)
	}
	if maxAge == 0 {
		maxAge = defaultMaxAge
	}
	return &Lock{
		path:   filepath.Join(filepath.Dir(exe), "restic-sentry.lock"),
		maxAge: maxAge,
	}, nil
}

// NewWithPath creates a lock at a specific path (useful for testing).
func NewWithPath(path string, maxAge time.Duration) *Lock {
	if maxAge == 0 {
		maxAge = defaultMaxAge
	}
	return &Lock{path: path, maxAge: maxAge}
}

// Acquire tries to take the lock. If a stale lock exists (dead PID or too old),
// it's automatically removed. Returns an error only if another live process holds it.
func (l *Lock) Acquire() error {
	existing, err := l.read()
	if err == nil {
		// Lock file exists — check if it's stale
		if l.isStale(existing) {
			// Stale lock — remove it and proceed
			os.Remove(l.path)
		} else {
			return fmt.Errorf("another backup is already running (pid %d, started %s)",
				existing.PID, existing.CreatedAt.Format(time.RFC3339))
		}
	}

	// Write our lock
	hostname, _ := os.Hostname()
	data := lockData{
		PID:       os.Getpid(),
		CreatedAt: time.Now(),
		Hostname:  hostname,
	}

	content, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshaling lock data: %w", err)
	}

	if err := os.WriteFile(l.path, content, 0600); err != nil {
		return fmt.Errorf("writing lock file: %w", err)
	}

	return nil
}

// Release removes the lock file. Only removes if we own it (same PID).
func (l *Lock) Release() error {
	existing, err := l.read()
	if err != nil {
		// Lock file doesn't exist or is unreadable — nothing to do
		return nil
	}

	if existing.PID != os.Getpid() {
		// Not our lock — don't touch it
		return nil
	}

	return os.Remove(l.path)
}

func (l *Lock) read() (*lockData, error) {
	content, err := os.ReadFile(l.path)
	if err != nil {
		return nil, err
	}

	var data lockData
	if err := json.Unmarshal(content, &data); err != nil {
		// Corrupt lock file — treat as stale
		return nil, err
	}

	return &data, nil
}

func (l *Lock) isStale(data *lockData) bool {
	// Too old — definitely stale regardless of process status
	if time.Since(data.CreatedAt) > l.maxAge {
		return true
	}

	// Check if the process is still alive
	return !processExists(data.PID)
}
