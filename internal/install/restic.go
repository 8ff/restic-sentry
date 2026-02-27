package install

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	resticAPI  = "https://api.github.com/repos/restic/restic/releases/latest"
	installDir = `C:\restic`
	binaryName = "restic.exe"
)

// InstallRestic downloads the latest restic release for Windows amd64
// and installs it to C:\restic\restic.exe.
func InstallRestic() (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("install-restic is only supported on Windows (current OS: %s)", runtime.GOOS)
	}

	fmt.Println("Fetching latest restic release info...")
	release, err := fetchRelease(resticAPI)
	if err != nil {
		return "", fmt.Errorf("fetching release info: %w", err)
	}
	fmt.Printf("Latest version: %s\n", release.TagName)

	asset, err := findWindowsZipAsset(release)
	if err != nil {
		return "", err
	}
	fmt.Printf("Downloading %s...\n", asset.Name)

	zipData, err := downloadAsset(asset.BrowserDownloadURL)
	if err != nil {
		return "", fmt.Errorf("downloading: %w", err)
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return "", fmt.Errorf("creating %s: %w", installDir, err)
	}

	destPath := filepath.Join(installDir, binaryName)
	if err := extractResticFromZip(zipData, destPath); err != nil {
		return "", fmt.Errorf("extracting: %w", err)
	}

	fmt.Printf("Installed restic %s to %s\n", release.TagName, destPath)
	fmt.Println("\nSet \"restic_binary\" in your config to this path, or add C:\\restic to your PATH.")

	return destPath, nil
}

func findWindowsZipAsset(release *githubRelease) (*githubAsset, error) {
	for _, asset := range release.Assets {
		name := strings.ToLower(asset.Name)
		if strings.Contains(name, "windows") && strings.Contains(name, "amd64") && strings.HasSuffix(name, ".zip") {
			return &asset, nil
		}
	}
	return nil, fmt.Errorf("no windows_amd64.zip asset found in release %s", release.TagName)
}

func extractResticFromZip(zipData []byte, destPath string) error {
	reader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}

	for _, f := range reader.File {
		if strings.HasSuffix(strings.ToLower(f.Name), "restic.exe") {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("opening %s in zip: %w", f.Name, err)
			}
			defer rc.Close()

			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
			if err != nil {
				return fmt.Errorf("creating %s: %w", destPath, err)
			}
			defer out.Close()

			if _, err := io.Copy(out, rc); err != nil {
				return fmt.Errorf("writing %s: %w", destPath, err)
			}
			return nil
		}
	}

	return fmt.Errorf("restic.exe not found in zip archive")
}
