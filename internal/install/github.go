package install

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type githubRelease struct {
	TagName string        `json:"tag_name"`
	Assets  []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func newGitHubClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

func newGitHubRequest(url string) (*http.Request, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	return req, nil
}

func decodeRelease(r io.Reader) (*githubRelease, error) {
	var release githubRelease
	if err := json.NewDecoder(r).Decode(&release); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return &release, nil
}

func downloadAsset(url string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
