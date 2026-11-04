package main

import (
	"context"
	"errors"
	"io"
	"net/http"
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
}
