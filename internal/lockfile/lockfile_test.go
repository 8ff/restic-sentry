package lockfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAcquireAndRelease(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")
	lock := NewWithPath(path, time.Hour)

	// Acquire should succeed
	if err := lock.Acquire(); err != nil {
		t.Fatalf("first acquire failed: %v", err)
	}

	// Lock file should exist with our PID
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ld lockData
	json.Unmarshal(data, &ld)
	if ld.PID != os.Getpid() {
		t.Errorf("lock PID = %d, want %d", ld.PID, os.Getpid())
	}

	// Release should clean up
	if err := lock.Release(); err != nil {
		t.Fatalf("release failed: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("lock file should be removed after release")
	}
}

func TestAcquireBlockedByLiveProcess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	// Write a lock with our own PID (simulates another running instance)
	ld := lockData{
		PID:       os.Getpid(),
		CreatedAt: time.Now(),
	}
	data, _ := json.Marshal(ld)
	os.WriteFile(path, data, 0600)

	// A different lock instance should fail to acquire
	lock := NewWithPath(path, time.Hour)
	err := lock.Acquire()
	if err == nil {
		t.Fatal("expected error when lock is held by live process")
	}
}

func TestAcquireClearsDeadPIDLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	// Write a lock with a PID that almost certainly doesn't exist
	ld := lockData{
		PID:       999999999,
		CreatedAt: time.Now(),
	}
	data, _ := json.Marshal(ld)
	os.WriteFile(path, data, 0600)

	lock := NewWithPath(path, time.Hour)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("should clear dead PID lock and acquire: %v", err)
	}
	lock.Release()
}

func TestAcquireClearsExpiredLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	// Write a lock with our own PID but very old timestamp
	ld := lockData{
		PID:       os.Getpid(),
		CreatedAt: time.Now().Add(-24 * time.Hour),
	}
	data, _ := json.Marshal(ld)
	os.WriteFile(path, data, 0600)

	// maxAge of 1 hour means the 24h-old lock is stale
	lock := NewWithPath(path, time.Hour)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("should clear expired lock and acquire: %v", err)
	}
	lock.Release()
}

func TestAcquireCorruptLockFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	// Write garbage to lock file
	os.WriteFile(path, []byte("not json"), 0600)

	lock := NewWithPath(path, time.Hour)
	if err := lock.Acquire(); err != nil {
		t.Fatalf("should handle corrupt lock file: %v", err)
	}
	lock.Release()
}

func TestReleaseIgnoresOtherPID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.lock")

	// Write a lock owned by a different PID
	ld := lockData{
		PID:       999999999,
		CreatedAt: time.Now(),
	}
	data, _ := json.Marshal(ld)
	os.WriteFile(path, data, 0600)

	lock := NewWithPath(path, time.Hour)
	lock.Release() // Should not remove someone else's lock

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("release should not remove lock owned by another PID")
	}
}
