package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func releaseResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestReleaseComparisonAndUnavailableStates(t *testing.T) {
	for _, test := range []struct {
		name, current, body, state string
		code                       int
	}{
		{"newer", "1.0.0", `{"tag_name":"v1.1.0"}`, "update_available", 200},
		{"patch update before 1.0", "0.1.0", `{"tag_name":"v0.1.1"}`, "update_available", 200},
		{"current patch before 1.0", "0.1.1", `{"tag_name":"v0.1.1"}`, "up_to_date", 200},
		{"numeric ordering", "1.9.0", `{"tag_name":"v1.10.0"}`, "update_available", 200},
		{"same", "1.0.0", `{"tag_name":"v1.0.0"}`, "up_to_date", 200},
		{"installed newer", "2.0.0", `{"tag_name":"v1.0.0"}`, "up_to_date", 200},
		{"prerelease to stable", "1.0.0-beta.1", `{"tag_name":"v1.0.0"}`, "update_available", 200},
		{"no prefix", "1.0.0", `{"tag_name":"1.1.0"}`, "update_available", 200},
		{"build metadata", "1.0.0+local", `{"tag_name":"v1.0.0+remote"}`, "up_to_date", 200},
		{"no releases", "1.0.0", `{}`, "no_release", 404},
		{"rate limited", "1.0.0", `{}`, "unavailable", 403},
		{"server failure", "1.0.0", `{}`, "unavailable", 503},
		{"invalid JSON", "1.0.0", `not JSON`, "unavailable", 200},
		{"invalid release version", "1.0.0", `{"tag_name":"not-a-version"}`, "unavailable", 200},
		{"draft", "1.0.0", `{"tag_name":"v2.0.0","draft":true}`, "unavailable", 200},
		{"prerelease", "1.0.0", `{"tag_name":"v2.0.0-beta.1","prerelease":true}`, "unavailable", 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			client := &http.Client{Transport: releaseTransport(func(request *http.Request) (*http.Response, error) {
				if request.URL.String() != latestReleaseURL || request.Method != http.MethodGet {
					t.Error("wrong release endpoint")
				}
				if request.Header.Get("User-Agent") == "" || request.Header.Get("Accept") != "application/vnd.github+json" {
					t.Error("missing GitHub headers")
				}
				if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" {
					t.Error("release check included account credentials")
				}
				return releaseResponse(test.code, test.body), nil
			})}
			result := checkLatestRelease(context.Background(), client, test.current)
			if result.State != test.state || result.CurrentVersion != test.current {
				t.Fatalf("unexpected release status: %+v", result)
			}
		})
	}
}

func TestReleaseCheckOncePerAppSessionIncludingFailures(t *testing.T) {
	for _, fail := range []bool{false, true} {
		var calls atomic.Int32
		client := &http.Client{Transport: releaseTransport(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			if fail {
				return nil, errors.New("offline")
			}
			return releaseResponse(200, `{"tag_name":"v65535.0.0"}`), nil
		})}
		app := &App{updates: updateCheck{client: client}}
		var group sync.WaitGroup
		for range 20 {
			group.Add(1)
			go func() { defer group.Done(); app.GetUpdateStatus() }()
		}
		group.Wait()
		first, second := app.GetUpdateStatus(), app.GetUpdateStatus()
		if calls.Load() != 1 || first != second {
			t.Fatal("startup check was repeated instead of cached")
		}
		if fail && first.State != "unavailable" {
			t.Fatal("failed check claimed app was current")
		}
		other := &App{updates: updateCheck{client: client}}
		other.GetUpdateStatus()
		if calls.Load() != 2 {
			t.Fatal("a new app session did not check again")
		}
	}
}

func TestReleaseCheckUsesBoundedStartupContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	app := &App{ctx: ctx, updates: updateCheck{client: &http.Client{Transport: releaseTransport(func(request *http.Request) (*http.Response, error) {
		if _, ok := request.Context().Deadline(); !ok {
			t.Error("release check has no timeout")
		}
		cancel()
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}}}
	if status := app.GetUpdateStatus(); status.State != "unavailable" {
		t.Fatalf("canceled startup request: %+v", status)
	}
}

func TestAppVersionMatchesEmbeddedConfigurationOrReleaseTag(t *testing.T) {
	version := (&App{}).GetAppVersion()
	if version == "" || version == "unknown" || strings.HasPrefix(version, "v") {
		t.Fatalf("invalid displayed build version: %q", version)
	}
	if appVersion != "" && version != strings.TrimPrefix(appVersion, "v") {
		t.Fatal("full release tag was lost")
	}
	if appVersion == "" {
		var config struct {
			Info struct {
				ProductVersion string `json:"productVersion"`
			} `json:"info"`
		}
		if err := json.Unmarshal(versionConfig, &config); err != nil {
			t.Fatal(err)
		}
		if version != config.Info.ProductVersion {
			t.Fatalf("local version %q differs from Wails product version %q", version, config.Info.ProductVersion)
		}
	}
}

func TestUpdatePackagesMatchReleaseWorkflow(t *testing.T) {
	if latestReleaseURL != "https://api.github.com/repos/Overwhelmed44/shell-anywhere/releases/latest" {
		t.Fatal("updater points to a different repository than the release workflow")
	}
	for _, test := range []struct{ platform, arch, name string }{
		{"windows", "amd64", "ShellAnywhere-amd64-setup.exe"},
		{"windows", "arm64", "ShellAnywhere-arm64-setup.exe"},
		{"darwin", "amd64", "ShellAnywhere-macos-amd64.dmg"},
		{"darwin", "arm64", "ShellAnywhere-macos-arm64.dmg"},
		{"linux", "amd64", "ShellAnywhere-linux-amd64.deb"},
		{"linux", "arm64", "ShellAnywhere-linux-arm64.deb"},
		{"linux", "386", ""},
		{"freebsd", "amd64", ""},
	} {
		if got := updatePackageName(test.platform, test.arch); got != test.name {
			t.Errorf("%s/%s: got %s, want %s", test.platform, test.arch, got, test.name)
		}
	}
}

func TestReleaseBuildVersionTakesPriorityOverLocalConfiguration(t *testing.T) {
	original := appVersion
	t.Cleanup(func() { appVersion = original })
	for _, tag := range []string{"v0.1.0", "v0.1.1", "v0.1.2-beta.1"} {
		appVersion = tag
		if got := (&App{}).GetAppVersion(); got != strings.TrimPrefix(tag, "v") {
			t.Fatalf("release build %s used the local fallback instead: %s", tag, got)
		}
	}
}

func testUpdateAsset() releaseAsset {
	hash := sha256.Sum256([]byte("installer"))
	name := updatePackageName(runtime.GOOS, runtime.GOARCH)
	return releaseAsset{
		Name: name, URL: "https://github.com/" + releaseRepository + "/releases/download/v65535.0.0/" + name,
		Digest: "sha256:" + hex.EncodeToString(hash[:]), Size: 9, State: "uploaded",
	}
}

func TestUpdateRejectsUntrustedOrIncompleteAssets(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*releaseAsset)
	}{
		{"wrong repository", func(a *releaseAsset) { a.URL = strings.Replace(a.URL, releaseRepository, "attacker/repo", 1) }},
		{"wrong tag", func(a *releaseAsset) { a.URL = strings.Replace(a.URL, "v65535.0.0", "v1.0.0", 1) }},
		{"http", func(a *releaseAsset) { a.URL = strings.Replace(a.URL, "https:", "http:", 1) }},
		{"credentials", func(a *releaseAsset) { a.URL = strings.Replace(a.URL, "github.com", "user@github.com", 1) }},
		{"no digest", func(a *releaseAsset) { a.Digest = "" }},
		{"invalid digest", func(a *releaseAsset) { a.Digest = "sha256:invalid" }},
		{"not uploaded", func(a *releaseAsset) { a.State = "new" }},
		{"empty", func(a *releaseAsset) { a.Size = 0 }},
		{"too large", func(a *releaseAsset) { a.Size = maxUpdateSize + 1 }},
		{"path traversal", func(a *releaseAsset) {
			a.Name = "../installer.exe"
			a.URL = "https://github.com/" + releaseRepository + "/releases/download/v65535.0.0/" + a.Name
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			asset := testUpdateAsset()
			test.change(&asset)
			if validUpdateAsset(asset, "v65535.0.0") {
				t.Fatal("accepted untrusted or incomplete update")
			}
		})
	}
}

func TestInstallUpdateVerifiesDownloadBeforeLaunchingAndQuitting(t *testing.T) {
	for _, scenario := range []string{"success", "wrong hash", "truncated", "oversized", "http error", "missing asset", "wrong architecture", "launch error", "offline"} {
		t.Run(scenario, func(t *testing.T) {
			asset := testUpdateAsset()
			var downloaded, launched, quit bool
			var launchPath string
			client := &http.Client{Transport: releaseTransport(func(request *http.Request) (*http.Response, error) {
				if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" {
					t.Fatal("updater sent account credentials")
				}
				if _, ok := request.Context().Deadline(); !ok {
					t.Fatal("update request has no timeout")
				}
				if scenario == "offline" {
					return nil, errors.New("offline")
				}
				if request.URL.String() == latestReleaseURL {
					assets := []releaseAsset{asset}
					if scenario == "missing asset" {
						assets = nil
					}
					if scenario == "wrong architecture" {
						assets[0].Name = "other-platform.exe"
					}
					body, _ := json.Marshal(map[string]any{"tag_name": "v65535.0.0", "assets": assets})
					return releaseResponse(200, string(body)), nil
				}
				if request.URL.String() != asset.URL {
					t.Fatal("unexpected download URL")
				}
				downloaded = true
				switch scenario {
				case "wrong hash":
					return releaseResponse(200, "corrupted"), nil
				case "truncated":
					return releaseResponse(200, "short"), nil
				case "oversized":
					return releaseResponse(200, "installer-extra"), nil
				case "http error":
					return releaseResponse(503, "unavailable"), nil
				}
				return releaseResponse(200, "installer"), nil
			})}
			app := &App{updates: updateCheck{
				client: client,
				launch: func(path string) error {
					launched, launchPath = true, path
					body, err := os.ReadFile(path)
					if err != nil || string(body) != "installer" {
						t.Fatal("launched unverified download")
					}
					if scenario == "launch error" {
						return errors.New("no package handler")
					}
					return nil
				},
				quit: func() { quit = true },
			}}
			// Startup may have cached a check before the workflow uploaded the installer.
			app.updates.once.Do(func() { app.updates.status = UpdateStatus{State: "update_available"} })
			err := app.InstallUpdate()
			if scenario == "success" {
				defer removeUpdateDownload(launchPath)
				if err != nil || !downloaded || !launched || !quit {
					t.Fatalf("update failed: %v", err)
				}
			} else {
				if err == nil || quit {
					t.Fatalf("failed update closed app: %v", err)
				}
				if launched && scenario != "launch error" {
					t.Fatal("invalid installer was launched")
				}
				if scenario == "launch error" {
					if _, err := os.Stat(filepath.Dir(launchPath)); !os.IsNotExist(err) {
						t.Fatal("failed installer was not removed")
					}
				}
			}
		})
	}
}

func TestDownloadFailureRemovesPartialPackage(t *testing.T) {
	client := &http.Client{Transport: releaseTransport(func(*http.Request) (*http.Response, error) {
		return releaseResponse(200, "corrupted"), nil
	})}
	path, err := downloadUpdate(context.Background(), client, testUpdateAsset())
	if err == nil {
		t.Fatal("corrupted package was accepted")
	}
	if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
		t.Fatal("partial download was not removed")
	}
}

func TestInstallUpdateRejectsConcurrentAttempt(t *testing.T) {
	app := &App{}
	app.updates.installMu.Lock()
	defer app.updates.installMu.Unlock()
	if err := app.InstallUpdate(); err == nil {
		t.Fatal("concurrent update was accepted")
	}
}

func TestUpdateRedirectsStayOnGitHubHTTPS(t *testing.T) {
	for _, test := range []struct {
		url     string
		allowed bool
	}{
		{"https://release-assets.githubusercontent.com/asset?signature=example", true},
		{"https://github.com/asset", true},
		{"http://release-assets.githubusercontent.com/asset", false},
		{"https://github.com.attacker.example/asset", false},
		{"https://attacker.example/asset", false},
		{"https://user@github.com/asset", false},
	} {
		request, _ := http.NewRequest(http.MethodGet, test.url, nil)
		if err := updateRedirect(request, nil); (err == nil) != test.allowed {
			t.Errorf("unexpected redirect decision for %s", test.url)
		}
	}
}

func TestUpdateMetadataRedirectAndCachedPackageRecovery(t *testing.T) {
	for _, scenario := range []string{"metadata redirect", "rate limited with cached package", "rate limited without cached package", "already current"} {
		t.Run(scenario, func(t *testing.T) {
			asset := testUpdateAsset()
			var launchPath string
			var quit bool
			app := &App{}
			app.updates.client = &http.Client{
				CheckRedirect: updateRedirect,
				Transport: releaseTransport(func(request *http.Request) (*http.Response, error) {
					if request.URL.String() == asset.URL {
						return releaseResponse(200, "installer"), nil
					}
					if strings.HasPrefix(scenario, "rate limited") {
						response := releaseResponse(403, `{}`)
						response.Header.Set("X-RateLimit-Remaining", "0")
						return response, nil
					}
					if scenario == "metadata redirect" && request.URL.String() == latestReleaseURL {
						response := releaseResponse(301, "")
						response.Header.Set("Location", "https://api.github.com/repositories/123/releases/latest")
						return response, nil
					}
					tag := "v65535.0.0"
					if scenario == "already current" {
						tag = "v" + app.GetAppVersion()
					}
					body, _ := json.Marshal(map[string]any{"tag_name": tag, "assets": []releaseAsset{asset}})
					return releaseResponse(200, string(body)), nil
				}),
			}
			app.updates.once.Do(func() {
				app.updates.status = UpdateStatus{State: "unavailable"}
				if scenario == "rate limited with cached package" {
					app.updates.status = UpdateStatus{State: "update_available", CurrentVersion: app.GetAppVersion(), LatestVersion: "65535.0.0", asset: asset}
				}
			})
			app.updates.launch = func(path string) error { launchPath = path; return nil }
			app.updates.quit = func() { quit = true }
			err := app.InstallUpdate()
			if launchPath != "" {
				defer removeUpdateDownload(launchPath)
			}
			if scenario == "rate limited without cached package" || scenario == "already current" {
				if err == nil || quit || launchPath != "" {
					t.Fatal("failed check must not install or quit")
				}
				want := "limit"
				if scenario == "already current" {
					want = "already have"
				}
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("error does not explain the actual failure: %v", err)
				}
			} else if err != nil || launchPath == "" || !quit {
				t.Fatalf("update could not recover: %v", err)
			}
		})
	}
	// A package download cannot redirect into arbitrary API endpoints.
	download, _ := http.NewRequest(http.MethodGet, testUpdateAsset().URL, nil)
	apiRequest, _ := http.NewRequest(http.MethodGet, latestReleaseURL, nil)
	if updateRedirect(apiRequest, []*http.Request{download}) == nil {
		t.Fatal("download was allowed to redirect into the API")
	}
}
