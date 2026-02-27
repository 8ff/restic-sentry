package install

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const selfAPI = "https://api.github.com/repos/8ff/restic-sentry/releases/latest"

// SelfUpdate downloads the latest restic-sentry release and replaces the
// running binary. On Windows, the running exe can't be overwritten directly,
// so we rename the current binary to .old, write the new one, and the old
// one can be cleaned up on next run.
func SelfUpdate(currentVersion string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("self-update is only supported on Windows (current OS: %s)", runtime.GOOS)
	}

	fmt.Println("Checking for updates...")
	release, err := fetchRelease(selfAPI)
	if err != nil {
		return fmt.Errorf("fetching release info: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	cleanCurrent := strings.TrimPrefix(currentVersion, "v")

	if latestVersion == cleanCurrent {
		fmt.Printf("Already running the latest version (%s).\n", currentVersion)
		return nil
	}

	fmt.Printf("Current: %s → Latest: %s\n", currentVersion, release.TagName)

	// Find the .exe asset
	var asset *githubAsset
	for _, a := range release.Assets {
		if strings.HasSuffix(strings.ToLower(a.Name), ".exe") {
			asset = &a
			break
		}
	}
	if asset == nil {
		return fmt.Errorf("no .exe asset found in release %s", release.TagName)
	}

	fmt.Printf("Downloading %s...\n", asset.Name)
	data, err := downloadAsset(asset.BrowserDownloadURL)
	if err != nil {
		return fmt.Errorf("downloading: %w", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}

	// On Windows: rename current → .old, then write new binary
	oldPath := exePath + ".old"
	os.Remove(oldPath) // clean up any previous .old file

	if err := os.Rename(exePath, oldPath); err != nil {
		return fmt.Errorf("renaming current binary: %w", err)
	}

	if err := os.WriteFile(exePath, data, 0755); err != nil {
		// Try to restore the old binary
		os.Rename(oldPath, exePath)
		return fmt.Errorf("writing new binary: %w", err)
	}

	fmt.Printf("Updated to %s. Old binary saved as %s\n", release.TagName, oldPath)
	return nil
}

func fetchRelease(apiURL string) (*githubRelease, error) {
	client := newGitHubClient()
	req, err := newGitHubRequest(apiURL)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	return decodeRelease(resp.Body)
}
