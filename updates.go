package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/semver"
)

// GitHub release builds set this to the full release tag, including prerelease suffixes
var appVersion string

//go:embed wails.json
var versionConfig []byte

const latestReleaseURL = "https://api.github.com/repos/Overwhelmed44/shell-anywhere/releases/latest"

type UpdateStatus struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	State          string `json:"state"`
}

type updateCheck struct {
	once   sync.Once
	status UpdateStatus
	client *http.Client
}

func (a *App) GetAppVersion() string {
	if appVersion != "" {
		return strings.TrimPrefix(appVersion, "v")
	}
	var config struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if json.Unmarshal(versionConfig, &config) == nil && config.Info.ProductVersion != "" {
		return strings.TrimPrefix(config.Info.ProductVersion, "v")
	}
	return "unknown"
}

// The startup UI calls this once; repeated calls, including React Strict Mode, reuse the result
func (a *App) GetUpdateStatus() UpdateStatus {
	a.updates.once.Do(func() {
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
		defer cancel()
		client := a.updates.client
		if client == nil {
			client = &http.Client{Timeout: 8 * time.Second}
		}
		a.updates.status = checkLatestRelease(ctx, client, a.GetAppVersion())
	})
	return a.updates.status
}

func checkLatestRelease(ctx context.Context, client *http.Client, current string) UpdateStatus {
	status := UpdateStatus{CurrentVersion: current, State: "unavailable"}
	currentTag := "v" + strings.TrimPrefix(current, "v")
	if !semver.IsValid(currentTag) {
		return status
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return status
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	request.Header.Set("User-Agent", "ShellAnywhere/"+current)
	response, err := client.Do(request)
	if err != nil {
		return status
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		status.State = "no_release"
		return status
	}
	if response.StatusCode != http.StatusOK {
		return status
	}
	var release struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release) != nil || release.Draft || release.Prerelease {
		return status
	}
	latestTag := "v" + strings.TrimPrefix(release.TagName, "v")
	if !semver.IsValid(latestTag) {
		return status
	}
	status.LatestVersion = strings.TrimPrefix(release.TagName, "v")
	status.State = "up_to_date"
	if semver.Compare(latestTag, currentTag) > 0 {
		status.State = "update_available"
	}
	return status
}
