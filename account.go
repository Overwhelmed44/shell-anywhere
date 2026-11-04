package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	msar "github.com/Overwhelmed44/msar/clients/go"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zalando/go-keyring"
)

const accountService = "shell-everywhere-oxee"

// Release builds set this with -ldflags; the environment override still takes precedence
var defaultOxeeURL = "https://localhost"

type systemAccountStore struct{}

func (systemAccountStore) Set(id, value string) error {
	return keyring.Set(accountService, id, value)
}

func (systemAccountStore) Get(id string) (string, error) {
	return keyring.Get(accountService, id)
}

func (systemAccountStore) Delete(id string) error {
	return keyring.Delete(accountService, id)
}

type Account struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
}

type AccountStatus struct {
	Account
	Pending bool `json:"pending"`
}

type savedAccount struct {
	User         Account `json:"user"`
	RefreshToken string  `json:"refresh_token"`
}

type accountLogin struct {
	rid    string
	server *http.Server
	timer  *time.Timer
}

func (a *App) accountOrigin() (string, error) {
	origin := a.accountURL

	if origin == "" {
		origin = os.Getenv("SHELLANYWHERE_OXEE_URL")
	}

	if origin == "" {
		origin = defaultOxeeURL
	}

	u, err := url.Parse(origin)

	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", errors.New("invalid Oxee address")
	}

	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return "", errors.New("Oxee requires HTTPS or a local development address")
	}

	return strings.TrimRight(origin, "/"), nil
}

func (a *App) accountClient(origin string) *http.Client {
	if a.accountHTTP != nil {
		return a.accountHTTP
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	address, err := url.Parse(origin)

	if err == nil {
		host := strings.TrimSuffix(strings.ToLower(address.Hostname()), ".")
		ip := net.ParseIP(host)

		if host == "localhost" || (ip != nil && ip.IsLoopback()) {
			// Only loopback development servers may use untrusted certificates.
			transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
	}

	return &http.Client{
		Timeout:   20 * time.Second,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func (a *App) authenticatedAccountClient(origin, refresh string) (*msar.Client, http.CookieJar, error) {
	baseURL, err := url.Parse(origin)
	if err != nil {
		return nil, nil, err
	}

	cookies, err := cookiejar.New(nil)
	if err != nil {
		return nil, nil, err
	}

	cookies.SetCookies(baseURL, []*http.Cookie{{
		Name:     "refresh_token",
		Value:    refresh,
		Path:     "/",
		Secure:   baseURL.Scheme == "https",
		HttpOnly: true,
	}})

	httpClient := *a.accountClient(origin)
	httpClient.Jar = cookies
	if transport, ok := httpClient.Transport.(*http.Transport); ok && transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		verified := transport.Clone()
		verified.TLSClientConfig = transport.TLSClientConfig.Clone()
		verified.TLSClientConfig.InsecureSkipVerify = false
		httpClient.Transport = &accountTransport{local: transport, verified: verified}
	}

	client, err := msar.New(
		origin,
		msar.WithHTTPClient(&httpClient),
		msar.WithPlatform("Shell Anywhere;"+runtime.GOOS),
	)

	return client, cookies, err
}

type accountTransport struct {
	local    *http.Transport
	verified *http.Transport
}

func (t *accountTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	host := strings.TrimSuffix(strings.ToLower(request.URL.Hostname()), ".")
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return t.local.RoundTrip(request)
	}

	return t.verified.RoundTrip(request)
}

func accountResponse(response *http.Response, target any) error {
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Oxee returned %s", response.Status)
	}

	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(target)
}

func (a *App) accountStatusLocked() AccountStatus {
	return AccountStatus{Account: a.account.User, Pending: a.accountLogin != nil}
}

// GetAccount restores only the optional Oxee account; local hosts never depend on it.
func (a *App) GetAccount() (AccountStatus, error) {
	a.accountMu.Lock()
	defer a.accountMu.Unlock()

	if a.accountLoaded {
		return a.accountStatusLocked(), nil
	}

	origin, err := a.accountOrigin()
	if err != nil {
		return AccountStatus{}, err
	}

	stored, err := a.accounts.Get(origin)
	if errors.Is(err, keyring.ErrNotFound) {
		a.accountLoaded = true
		return a.accountStatusLocked(), nil
	}

	if err != nil {
		return AccountStatus{}, errors.New("could not read your saved Oxee login")
	}

	var saved savedAccount
	if err := json.Unmarshal([]byte(stored), &saved); err != nil || saved.RefreshToken == "" || saved.User.UserID == "" {
		return AccountStatus{}, errors.New("saved Oxee login is invalid; sign in again")
	}

	a.account = saved
	a.accountLoaded = true

	client, cookies, err := a.authenticatedAccountClient(origin, saved.RefreshToken)
	if err != nil {
		return a.accountStatusLocked(), err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Keep the cached identity available offline. A rejected session is cleared.
	response, err := client.Get(ctx, "/api/users/self")
	if err != nil {
		return a.accountStatusLocked(), nil
	}

	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusNotFound {
		response.Body.Close()

		if err := a.accounts.Delete(origin); err != nil {
			return AccountStatus{}, err
		}

		a.account = savedAccount{}
		return a.accountStatusLocked(), nil
	}

	updated := saved
	refreshFromCookies(origin, cookies, &updated)

	var profile Account
	profileOK := accountResponse(response, &profile) == nil && profile.UserID != "" && profile.Username != ""

	if profileOK {
		updated.User = profile
	}

	// Rotation may succeed even when the profile handler fails afterward.
	if profileOK || updated.RefreshToken != saved.RefreshToken {
		if err := a.saveAccount(origin, updated); err != nil {
			return AccountStatus{}, err
		}
	}

	return a.accountStatusLocked(), nil
}

func refreshFromCookies(origin string, cookies http.CookieJar, saved *savedAccount) {
	baseURL, err := url.Parse(origin + "/api/users/self")
	if err != nil {
		return
	}

	for _, cookie := range cookies.Cookies(baseURL) {
		if cookie.Name == "refresh_token" {
			saved.RefreshToken = cookie.Value
			return
		}
	}
}

func (a *App) saveAccount(origin string, saved savedAccount) error {
	data, err := json.Marshal(saved)
	if err != nil {
		return err
	}

	if err := a.accounts.Set(origin, string(data)); err != nil {
		return errors.New("could not save your Oxee login in the system credential store")
	}

	a.account = saved
	a.accountLoaded = true

	return nil
}

func (a *App) BeginLogin() error {
	a.accountMu.Lock()

	if a.accountLogin != nil {
		a.accountMu.Unlock()

		return errors.New("a login is already open in your browser")
	}

	origin, err := a.accountOrigin()
	if err != nil {
		a.accountMu.Unlock()

		return err
	}

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		a.accountMu.Unlock()

		return errors.New("could not open the local login listener")
	}

	form := url.Values{
		"client":     {"shell-anywhere"},
		"local_port": {fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)},
	}

	request, err := http.NewRequest(
		http.MethodPost,
		origin+"/api/users/auth/oauth",
		strings.NewReader(form.Encode()),
	)

	if err != nil {
		listener.Close()
		a.accountMu.Unlock()

		return err
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := a.accountClient(origin).Do(request)

	var created struct {
		RID string `json:"rid"`
	}

	if err == nil {
		err = accountResponse(response, &created)
	}

	if err != nil || created.RID == "" {
		listener.Close()
		a.accountMu.Unlock()

		if err != nil {
			return fmt.Errorf("could not start Oxee login: %w", err)
		}

		return errors.New("Oxee returned an invalid login request")
	}

	attempt := &accountLogin{
		rid: created.RID,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/"+attempt.rid, func(w http.ResponseWriter, r *http.Request) {
		a.receiveLogin(w, r, attempt, origin, listener.Addr().String())
	})

	attempt.server = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       5 * time.Second,
	}

	a.accountLogin = attempt
	attempt.timer = time.AfterFunc(10*time.Minute, func() { a.expireLogin(attempt) })
	status := a.accountStatusLocked()
	a.accountMu.Unlock()

	go func() { _ = attempt.server.Serve(listener) }()
	a.emit("account:changed", status)

	loginURL := origin + "/auth?" + url.Values{"oauth": {created.RID}}.Encode()

	if a.openAccountURL != nil {
		a.openAccountURL(loginURL)
	} else {
		wailsruntime.BrowserOpenURL(a.ctx, loginURL)
	}

	return nil
}

func (a *App) receiveLogin(w http.ResponseWriter, r *http.Request, attempt *accountLogin, origin, expectedHost string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	requestOrigin := r.Header.Get("Origin")

	if requestOrigin != "" && requestOrigin != origin {
		http.Error(w, "Invalid login origin", http.StatusForbidden)

		return
	}

	if requestOrigin == origin {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}

	query := r.URL.Query()
	requestMatches := r.URL.Path == "/oauth/"+attempt.rid

	if r.Host != expectedHost || !requestMatches {
		http.Error(w, "Invalid login callback", http.StatusBadRequest)

		return
	}

	if r.Method == http.MethodOptions {
		if requestOrigin != origin || r.Header.Get("Access-Control-Request-Method") != http.MethodGet {
			http.Error(w, "Invalid login preflight", http.StatusForbidden)

			return
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.WriteHeader(http.StatusNoContent)

		return
	}

	if r.Method != http.MethodGet || query.Get("code") == "" {
		http.Error(w, "Invalid login callback", http.StatusBadRequest)

		return
	}

	a.accountMu.Lock()

	if a.accountLogin != attempt {
		a.accountMu.Unlock()
		http.Error(w, "This login is no longer active", http.StatusGone)

		return
	}

	body, _ := json.Marshal(map[string]string{
		"code": query.Get("code"),
	})

	request, err := http.NewRequest(
		http.MethodPost,
		origin+"/api/users/auth/oauth/"+url.PathEscape(attempt.rid),
		strings.NewReader(string(body)),
	)

	var saved savedAccount

	if err == nil {
		request.Header.Set("Content-Type", "application/json")

		var response *http.Response
		response, err = a.accountClient(origin).Do(request)

		if err == nil {
			err = accountResponse(response, &saved)
		}
	}

	if err == nil && (saved.RefreshToken == "" || saved.User.UserID == "" || saved.User.Username == "") {
		err = errors.New("Oxee returned an incomplete login")
	}

	if err == nil {
		err = a.saveAccount(origin, saved)
	}

	if err != nil {
		a.accountMu.Unlock()

		// A forged or failed exchange must not cancel the pending login.
		a.emit("account:error", "Could not complete Oxee login: "+err.Error())
		http.Error(w, "Could not complete login. Retry this page or return to Shell Anywhere", http.StatusBadGateway)

		return
	}

	attempt.timer.Stop()
	a.accountLogin = nil
	status := a.accountStatusLocked()
	a.accountMu.Unlock()

	defer stopAccountServer(attempt.server)

	a.emit("account:changed", status)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		User Account `json:"user"`
	}{User: saved.User})
}

func stopAccountServer(server *http.Server) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = server.Shutdown(ctx)
	}()
}

func (a *App) expireLogin(attempt *accountLogin) {
	a.accountMu.Lock()

	if a.accountLogin != attempt {
		a.accountMu.Unlock()

		return
	}

	a.accountLogin = nil
	status := a.accountStatusLocked()
	a.accountMu.Unlock()

	stopAccountServer(attempt.server)
	a.emit("account:changed", status)
	a.emit("account:error", "Login expired; start again when you are ready")
}

func (a *App) closeAccountLogin() {
	a.accountMu.Lock()
	attempt := a.accountLogin
	a.accountLogin = nil
	a.accountMu.Unlock()

	if attempt != nil {
		attempt.timer.Stop()
		stopAccountServer(attempt.server)
	}
}

func (a *App) CancelLogin() {
	a.closeAccountLogin()

	a.accountMu.Lock()
	status := a.accountStatusLocked()
	a.accountMu.Unlock()

	a.emit("account:changed", status)
}

func (a *App) Logout() error {
	a.vault.mu.Lock()
	defer a.vault.mu.Unlock()

	a.closeAccountLogin()

	a.accountMu.Lock()

	origin, err := a.accountOrigin()
	if err != nil {
		a.accountMu.Unlock()

		return err
	}

	if err := a.accounts.Delete(origin); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		a.accountMu.Unlock()

		return errors.New("could not remove your saved Oxee login")
	}

	saved := a.account
	a.account = savedAccount{}
	a.accountLoaded = true
	a.accountMu.Unlock()

	a.vault.owner = ""
	a.vault.key = vaultKey{}
	a.vault.client = nil
	a.vault.cookies = nil
	a.vault.status = VaultStatus{}

	a.emit("account:changed", AccountStatus{})
	a.emit("vault:changed", VaultStatus{})

	if saved.RefreshToken != "" {
		go a.revokeAccount(origin, saved)
	}

	return nil
}

func (a *App) revokeAccount(origin string, saved savedAccount) {
	client, cookies, err := a.authenticatedAccountClient(origin, saved.RefreshToken)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	response, err := client.Get(ctx, "/api/users/self")
	if err != nil {
		return
	}

	response.Body.Close()

	if response.StatusCode != http.StatusOK || client.AccessToken() == "" {
		return
	}

	refreshFromCookies(origin, cookies, &saved)
	parts := strings.Split(saved.RefreshToken, ".")

	if len(parts) != 3 {
		return
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return
	}

	var claims struct {
		JTI string `json:"jti"`
	}

	if json.Unmarshal(payload, &claims) != nil || claims.JTI == "" {
		return
	}

	response, err = client.Delete(
		ctx,
		"/api/users/"+url.PathEscape(saved.User.UserID)+"/sessions",
		map[string][]string{"ids": {claims.JTI}},
	)

	if err == nil {
		response.Body.Close()
	}
}
