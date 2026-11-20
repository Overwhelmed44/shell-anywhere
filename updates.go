package main

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/mod/semver"
)

// GitHub release builds set this to the full release tag, including prerelease suffixes
var appVersion string

//go:embed wails.json
var versionConfig []byte

const releaseRepository = "Overwhelmed44/shell-anywhere"
const latestReleaseURL = "https://api.github.com/repos/" + releaseRepository + "/releases/latest"

const maxUpdateSize = 512 << 20

type releaseAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
	State  string `json:"state"`
}

type UpdateStatus struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	State          string `json:"state"`
	Error          string `json:"error,omitempty"`
	asset          releaseAsset
}

type updateCheck struct {
	once      sync.Once
	status    UpdateStatus
	client    *http.Client
	installMu sync.Mutex
	launch    func(string) error
	quit      func()
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
		status.Error = "The installed app version is invalid"
		return status
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		status.Error = fmt.Sprintf("Could not prepare the update check: %v", err)
		return status
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	request.Header.Set("User-Agent", "ShellAnywhere/"+current)
	response, err := client.Do(request)
	if err != nil {
		status.Error = fmt.Sprintf("Could not check for updates: %v", err)
		return status
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		status.State = "no_release"
		return status
	}
	if response.StatusCode != http.StatusOK {
		status.Error = fmt.Sprintf("GitHub update check failed (HTTP %d). Try again later", response.StatusCode)
		if response.StatusCode == http.StatusForbidden && response.Header.Get("X-RateLimit-Remaining") == "0" || response.StatusCode == http.StatusTooManyRequests {
			status.Error = "GitHub's update check limit has been reached. Wait for the limit to reset, then try again"
		}
		return status
	}
	var release struct {
		TagName    string         `json:"tag_name"`
		Draft      bool           `json:"draft"`
		Prerelease bool           `json:"prerelease"`
		Assets     []releaseAsset `json:"assets"`
	}
	if json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&release) != nil || release.Draft || release.Prerelease {
		status.Error = "GitHub returned invalid stable-release information. Try again later"
		return status
	}
	latestTag := "v" + strings.TrimPrefix(release.TagName, "v")
	if !semver.IsValid(latestTag) {
		status.Error = "The latest release has an invalid version tag"
		return status
	}
	status.LatestVersion = strings.TrimPrefix(release.TagName, "v")
	status.State = "up_to_date"
	if semver.Compare(latestTag, currentTag) > 0 {
		status.State = "update_available"
		name := updatePackageName(runtime.GOOS, runtime.GOARCH)
		for _, asset := range release.Assets {
			if name != "" && asset.Name == name && validUpdateAsset(asset, release.TagName) {
				status.asset = asset
				break
			}
		}
	}
	return status
}

func updatePackageName(platform, arch string) string {
	if arch != "amd64" && arch != "arm64" {
		return ""
	}
	switch platform {
	case "windows":
		return "ShellAnywhere-" + arch + "-setup.exe"
	case "darwin":
		return "ShellAnywhere-macos-" + arch + ".dmg"
	case "linux":
		return "ShellAnywhere-linux-" + arch + ".deb"
	}
	return ""
}

func validUpdateAsset(asset releaseAsset, tag string) bool {
	u, err := url.Parse(asset.URL)
	digest, digestErr := hex.DecodeString(strings.TrimPrefix(asset.Digest, "sha256:"))
	return err == nil && u.Scheme == "https" && u.Host == "github.com" && u.User == nil &&
		u.RawQuery == "" && u.Fragment == "" &&
		u.Path == "/"+releaseRepository+"/releases/download/"+tag+"/"+asset.Name &&
		filepath.Base(asset.Name) == asset.Name && !strings.ContainsAny(asset.Name, `/\\`) &&
		asset.State == "uploaded" && asset.Size > 0 && asset.Size <= maxUpdateSize &&
		strings.HasPrefix(asset.Digest, "sha256:") && digestErr == nil && len(digest) == sha256.Size
}

// InstallUpdate only accepts a package supplied by the configured public release repository.
// Refresh metadata on each attempt: release assets may still be building during the startup check.
func (a *App) InstallUpdate() error {
	if !a.updates.installMu.TryLock() {
		return errors.New("An update is already downloading")
	}
	defer a.updates.installMu.Unlock()
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	client := a.updates.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute, CheckRedirect: updateRedirect}
	}
	checkCtx, checkCancel := context.WithTimeout(ctx, 8*time.Second)
	status := checkLatestRelease(checkCtx, client, a.GetAppVersion())
	checkCancel()
	if status.State == "unavailable" {
		// A transient API failure must not discard a verified package found at startup.
		cached := a.GetUpdateStatus()
		if cached.State == "update_available" && cached.asset.Name != "" && cached.CurrentVersion == a.GetAppVersion() {
			status = cached
		}
	}
	if status.State != "update_available" {
		if status.Error != "" {
			return errors.New(status.Error)
		}
		if status.State == "up_to_date" {
			return fmt.Errorf("You already have the latest available release (v%s)", status.LatestVersion)
		}
		return errors.New("No published release is available yet. Try again later")
	}
	if status.asset.Name == "" {
		return errors.New("The update package for this device is not ready yet. Try again later")
	}
	path, err := downloadUpdate(ctx, client, status.asset)
	if err != nil {
		return err
	}
	launch := a.updates.launch
	if launch == nil {
		launch = launchUpdatePackage
	}
	if err := launch(path); err != nil {
		removeUpdateDownload(path)
		return fmt.Errorf("Could not open the update installer: %w", err)
	}
	// Keep the package until the OS has finished installing it.
	if a.updates.quit != nil {
		a.updates.quit()
	} else {
		wailsruntime.Quit(a.ctx)
	}
	return nil
}

func updateRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("Too many download redirects")
	}
	if request.URL.Scheme == "https" && request.URL.User == nil {
		if request.URL.Host == "api.github.com" && len(via) > 0 && via[0].URL.Host == "api.github.com" &&
			strings.HasSuffix(request.URL.Path, "/releases/latest") &&
			(strings.HasPrefix(request.URL.Path, "/repositories/") || strings.HasPrefix(request.URL.Path, "/repos/")) {
			return nil
		}
		switch request.URL.Host {
		case "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
			return nil
		}
	}
	return errors.New("Update download redirected outside GitHub")
}

func downloadUpdate(ctx context.Context, client *http.Client, asset releaseAsset) (path string, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.URL, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "ShellAnywhere-Updater")
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("Could not download the update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Update download failed (HTTP %d)", response.StatusCode)
	}
	dir, err := os.MkdirTemp("", "shell-anywhere-update-")
	if err != nil {
		return "", err
	}
	path = filepath.Join(dir, asset.Name)
	defer func() {
		if err != nil {
			removeUpdateDownload(path)
		}
	}()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return path, err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, asset.Size+1))
	closeErr := file.Close()
	if copyErr != nil {
		return path, fmt.Errorf("Update download interrupted: %w", copyErr)
	}
	if closeErr != nil {
		return path, closeErr
	}
	if n != asset.Size || "sha256:"+hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(asset.Digest) {
		return path, errors.New("Update verification failed. Please retry the download")
	}
	return path, nil
}

func removeUpdateDownload(path string) {
	_ = os.Remove(path)
	_ = os.Remove(filepath.Dir(path))
}

func launchUpdatePackage(path string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command(path)
		if err := command.Start(); err != nil {
			return err
		}
		return command.Process.Release()
	case "darwin":
		command = exec.Command("open", path)
	case "linux":
		command = exec.Command("xdg-open", path)
	default:
		return errors.New("Updates are not supported on this operating system")
	}
	return command.Run()
}
