package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestOptionalAccountLoginAndRestore(t *testing.T) {
	var port string
	var formMu sync.Mutex
	var exchanges atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/users/auth/oauth":
			if r.Method != http.MethodPost {
				t.Error("request creation must use POST")
			}
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			formMu.Lock()
			port = r.Form.Get("local_port")
			if r.Form.Get("client") != "shell-anywhere" || port == "" {
				t.Error("incomplete request")
			}

			if len(r.Form) != 2 {
				t.Error("request should include only client and local_port")
			}

			formMu.Unlock()
			io.WriteString(w, `{"rid":"request-id"}`)
		case "/api/users/auth/oauth/request-id":
			if r.Method != http.MethodPost {
				t.Error("code exchange must use POST")
			}

			exchanges.Add(1)
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}

			if body["code"] != "callback-code" {
				http.Error(w, "invalid code", http.StatusBadRequest)
				return
			}

			if len(body) != 1 {
				t.Error("exchange body should include only the code")
			}
			io.WriteString(w, `{"refresh_token":"refresh-secret","user":{"user_id":"user-id","username":"tester"}}`)
		case "/api/users/self":
			cookie, err := r.Cookie("refresh_token")
			if err != nil || cookie.Value != "refresh-secret" {
				t.Error("missing saved refresh cookie")
			}
			if !strings.HasPrefix(r.Header.Get("X-User-Platform"), "Shell Anywhere;") {
				t.Error("missing platform header")
			}
			http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "rotated-secret"})
			io.WriteString(w, `{"user_id":"user-id","username":"tester-renamed"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryPasswordStore{values: map[string]string{}}
	app.accounts = store
	app.accountURL = server.URL
	app.eventSink = func(string, any) {}
	var browserURL string
	app.openAccountURL = func(value string) { browserURL = value }
	t.Cleanup(app.closeAccountLogin)
	account, err := app.GetAccount()
	if err != nil || account.UserID != "" || account.Pending {
		t.Fatalf("guest must be available: %+v %v", account, err)
	}
	if _, err := app.CreateGroup("Without an account"); err != nil {
		t.Fatal(err)
	}
	if err := app.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	formMu.Lock()
	callbackPort := port
	formMu.Unlock()
	opened, err := url.Parse(browserURL)
	if err != nil || opened.Path != "/auth" || opened.Query().Get("oauth") != "request-id" || len(opened.Query()) != 1 {
		t.Fatalf("unexpected browser URL: %s", browserURL)
	}
	if err := app.BeginLogin(); err == nil {
		t.Fatal("parallel login should be rejected")
	}
	callbackOrigin := "http://127.0.0.1:" + callbackPort
	callback := callbackOrigin + "/oauth/request-id?"

	request, _ := http.NewRequest(http.MethodOptions, callback, nil)
	request.Header.Set("Origin", server.URL)
	request.Header.Set("Access-Control-Request-Method", http.MethodGet)
	response, err := http.DefaultClient.Do(request)

	if err != nil {
		t.Fatal(err)
	}

	response.Body.Close()

	if response.StatusCode != http.StatusNoContent || response.Header.Get("Access-Control-Allow-Origin") != server.URL || exchanges.Load() != 0 {
		t.Fatal("preflight must allow the configured Oxee origin without exchanging the code")
	}

	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		request, _ = http.NewRequest(method, callback+"code=callback-code", nil)
		request.Header.Set("Origin", "https://attacker.example")
		request.Header.Set("Access-Control-Request-Method", http.MethodGet)
		response, err = http.DefaultClient.Do(request)

		if err != nil {
			t.Fatal(err)
		}

		response.Body.Close()

		if response.StatusCode != http.StatusForbidden || response.Header.Get("Access-Control-Allow-Origin") != "" || exchanges.Load() != 0 {
			t.Fatal("an unrelated website must not reach the code exchange")
		}
	}

	response, err = http.Get(callbackOrigin + "/oauth/wrong-request?code=callback-code")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound || exchanges.Load() != 0 {
		t.Fatal("a different request path must not reach token exchange")
	}

	request, _ = http.NewRequest(http.MethodGet, callback+url.Values{"code": {"callback-code"}}.Encode(), nil)
	request.Host = "attacker.example"
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusBadRequest || exchanges.Load() != 0 {
		t.Fatal("unexpected Host must not reach token exchange")
	}
	request, _ = http.NewRequest(http.MethodGet, callback+"code=forged-code", nil)
	request.Header.Set("Origin", server.URL)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}

	response.Body.Close()
	account, err = app.GetAccount()

	if response.StatusCode != http.StatusBadGateway || response.Header.Get("Access-Control-Allow-Origin") != server.URL || err != nil || !account.Pending {
		t.Fatal("a forged code must leave the pending login available")
	}

	request, _ = http.NewRequest(http.MethodGet, callback+url.Values{"code": {"callback-code"}}.Encode(), nil)
	request.Header.Set("Origin", server.URL)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || strings.Contains(string(page), "refresh-secret") {
		t.Fatalf("unsafe callback response: %s", page)
	}

	var confirmed struct {
		User Account `json:"user"`
	}

	if err := json.Unmarshal(page, &confirmed); err != nil || confirmed.User.Username != "tester" || confirmed.User.UserID != "user-id" {
		t.Fatalf("callback did not acknowledge the saved account: %s", page)
	}

	if response.Header.Get("Access-Control-Allow-Origin") != server.URL || response.Header.Get("Content-Type") != "application/json" {
		t.Fatal("callback acknowledgement is not readable by the Oxee browser page")
	}
	account, err = app.GetAccount()
	if err != nil || account.Username != "tester" || account.Pending {
		t.Fatalf("login failed: %+v %v", account, err)
	}
	if exchanges.Load() != 2 {
		t.Fatal("expected a rejected exchange followed by a successful exchange")
	}
	for _, name := range []string{"hosts.json", "groups.json"} {
		data, _ := os.ReadFile(filepath.Join(app.configDir, name))
		if strings.Contains(string(data), "refresh-secret") {
			t.Fatal("token leaked into workspace files")
		}
	}
	restored, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	restored.accounts, restored.accountURL, restored.eventSink = store, server.URL, func(string, any) {}
	account, err = restored.GetAccount()
	if err != nil || account.Username != "tester-renamed" {
		t.Fatalf("restore failed: %+v %v", account, err)
	}
	if !strings.Contains(store.values[server.URL], "rotated-secret") {
		t.Fatal("rotated refresh token was not persisted")
	}
}

func TestCancelAndExpireLoginKeepLocalUseAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, `{"rid":"pending"}`) }))
	defer server.Close()
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.accounts = &memoryPasswordStore{values: map[string]string{}}
	app.accountURL, app.eventSink, app.openAccountURL = server.URL, func(string, any) {}, func(string) {}
	t.Cleanup(app.closeAccountLogin)
	if err := app.BeginLogin(); err != nil {
		t.Fatal(err)
	}
	app.accountMu.Lock()
	attempt := app.accountLogin
	app.accountMu.Unlock()
	app.CancelLogin()
	status, err := app.GetAccount()
	if err != nil || status.Pending || status.UserID != "" {
		t.Fatalf("cancelled login is not a guest: %+v %v", status, err)
	}
	if err := app.BeginLogin(); err != nil {
		t.Fatal("cancel must allow retry:", err)
	}
	// An old timer cannot cancel the new attempt.
	app.expireLogin(attempt)
	status, _ = app.GetAccount()
	if !status.Pending {
		t.Fatal("old expiry cancelled new login")
	}
	app.accountMu.Lock()
	current := app.accountLogin
	app.accountMu.Unlock()
	app.expireLogin(current)
	status, _ = app.GetAccount()
	if status.Pending {
		t.Fatal("expired listener still active")
	}
	if _, err := app.CreateGroup("Still available"); err != nil {
		t.Fatal(err)
	}
}

func TestFailedLoginAndLogoutDoNotRequireAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryPasswordStore{values: map[string]string{}}
	app.accounts, app.accountURL, app.eventSink = store, server.URL, func(string, any) {}
	if err := app.BeginLogin(); err == nil {
		t.Fatal("server failure should be reported")
	}
	status, err := app.GetAccount()
	if err != nil || status.Pending || status.UserID != "" {
		t.Fatal("failed login must leave a guest")
	}
	if err := app.Logout(); err != nil {
		t.Fatal("guest logout:", err)
	}
	app.accountMu.Lock()
	err = app.saveAccount(server.URL, savedAccount{User: Account{UserID: "user-id", Username: "tester"}, RefreshToken: "secret"})
	app.accountMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Logout(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(server.URL); err != keyring.ErrNotFound {
		t.Fatal("logout retained credentials")
	}
	status, _ = app.GetAccount()
	if status.UserID != "" {
		t.Fatal("logout retained identity")
	}
}

func TestAccountOriginRejectsRemoteHTTP(t *testing.T) {
	for _, address := range []string{"http://example.com", "https://name:password@example.com", "https://example.com/?token=x", "https://example.com/api"} {
		app := &App{accountURL: address}
		if _, err := app.accountOrigin(); err == nil {
			t.Errorf("accepted unsafe origin %q", address)
		}
	}
}

func TestAccountOriginBuildDefaultAndEnvironmentOverride(t *testing.T) {
	t.Setenv("SHELLANYWHERE_OXEE_URL", "")
	app := &App{}
	if origin, err := app.accountOrigin(); err != nil || origin != defaultOxeeURL {
		t.Fatalf("build default not used: %q, %v", origin, err)
	}
	t.Setenv("SHELLANYWHERE_OXEE_URL", "https://custom.example")
	if origin, err := app.accountOrigin(); err != nil || origin != "https://custom.example" {
		t.Fatalf("environment did not override build default: %q, %v", origin, err)
	}
}

func TestAccountTLSAcceptsDevCertificatesOnlyForLoopback(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	for _, test := range []struct {
		origin string
		local  bool
	}{
		{"https://localhost", true},
		{"https://LOCALHOST.", true},
		{"https://127.0.0.1", true},
		{"https://127.1.2.3", true},
		{"https://[::1]", true},
		{"https://oxee.net", false},
		{"https://localhost.example.com", false},
		{"https://192.168.1.10", false},
	} {
		t.Run(test.origin, func(t *testing.T) {
			client := (&App{}).accountClient(test.origin)
			transport := client.Transport.(*http.Transport)
			defer transport.CloseIdleConnections()

			// Route every hostname to the same untrusted local TLS server.
			transport.Proxy = nil
			transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}

			response, err := client.Get(test.origin)

			if response != nil {
				response.Body.Close()
			}

			if test.local {
				if err != nil {
					t.Fatalf("local development certificate was rejected: %v", err)
				}

				return
			}

			var certificateError *tls.CertificateVerificationError

			if !errors.As(err, &certificateError) {
				t.Fatalf("expected certificate rejection for remote origin, got %v", err)
			}
		})
	}
}

func TestMSARAcceptsLocalDevCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("refresh_token")

		if err != nil || cookie.Value != "local-refresh" {
			t.Error("msar request is missing its refresh cookie")
		}

		io.WriteString(w, `{"user_id":"user-id","username":"tester"}`)
	}))
	defer server.Close()

	client, _, err := (&App{}).authenticatedAccountClient(server.URL, "local-refresh")

	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Get(context.Background(), "/api/users/self")

	if err != nil {
		t.Fatal(err)
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected successful local msar request, got %s", response.Status)
	}
}

func TestMSARValidatesRemoteCertificateWithLocalAccount(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	defer transport.CloseIdleConnections()
	a := &App{accountHTTP: &http.Client{Transport: transport}}
	client, _, err := a.authenticatedAccountClient("https://localhost", "local-refresh")
	if err != nil {
		t.Fatal(err)
	}

	response, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("local development certificate rejected: %v", err)
	}
	response.Body.Close()

	response, err = client.Get(context.Background(), "https://example.com/api/private/vaults")
	if response != nil {
		response.Body.Close()
	}
	var certificateError *tls.CertificateVerificationError
	if !errors.As(err, &certificateError) {
		t.Fatalf("remote certificate must be validated even with a local account: %v", err)
	}
}

func TestMSARAuthorizesSessionRevocation(t *testing.T) {
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"jti":"rotated-session-id"}`))
	rotatedRefresh := "header." + claims + ".signature"
	var requests atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)

		if !strings.HasPrefix(r.Header.Get("X-User-Platform"), "Shell Anywhere;") {
			t.Error("msar did not supply the platform header")
		}

		switch r.URL.Path {
		case "/api/users/self":
			cookie, err := r.Cookie("refresh_token")
			if err != nil || cookie.Value != "saved-refresh" {
				t.Error("saved refresh token was not restored into the cookie jar")
			}

			w.Header().Set("X-Refreshed-Access-Token", "msar-access")
			w.Header().Set("X-Issued-Access-Token", "lower-priority-access")
			http.SetCookie(w, &http.Cookie{
				Name:  "refresh_token",
				Value: rotatedRefresh,
				Path:  "/",
			})

			io.WriteString(w, `{"user_id":"user-id","username":"tester"}`)

		case "/api/users/user-id/sessions":
			if r.Method != http.MethodDelete {
				t.Errorf("expected DELETE, received %s", r.Method)
			}

			if r.Header.Get("Authorization") != "Bearer msar-access" {
				t.Error("msar did not attach the captured access token")
			}

			cookie, err := r.Cookie("refresh_token")
			if err != nil || cookie.Value != rotatedRefresh {
				t.Error("msar did not carry the rotated refresh cookie")
			}

			var body struct {
				IDs []string `json:"ids"`
			}

			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}

			if len(body.IDs) != 1 || body.IDs[0] != "rotated-session-id" {
				t.Errorf("revocation did not use the rotated session: %+v", body.IDs)
			}

			w.WriteHeader(http.StatusOK)

		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	app := &App{}
	app.revokeAccount(server.URL, savedAccount{
		User:         Account{UserID: "user-id", Username: "tester"},
		RefreshToken: "saved-refresh",
	})

	if requests.Load() != 2 {
		t.Fatalf("expected profile and revocation requests, received %d", requests.Load())
	}
}
