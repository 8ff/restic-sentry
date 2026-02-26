//go:build !windows

package lockfile

import (
	"os"
	"syscall"
)

// processExists checks if a process with the given PID is still running.
// On Unix, we send signal 0 which doesn't actually send a signal but checks
// if the process exists and we have permission to signal it.
func processExists(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
