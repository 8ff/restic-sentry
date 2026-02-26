//go:build windows

package lockfile

import (
	"golang.org/x/sys/windows"
)

// processExists checks if a process with the given PID is still running.
// On Windows, we open a handle with limited query rights and check the exit code.
// STILL_ACTIVE (259) means the process is running.
func processExists(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)

	var exitCode uint32
	err = windows.GetExitCodeProcess(handle, &exitCode)
	if err != nil {
		return false
	}

	// STILL_ACTIVE = 259
	return exitCode == 259
}
