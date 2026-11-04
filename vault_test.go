package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultEncryption(t *testing.T) {
	data := []byte(`{"version":1,"workspaces":{"personal":{"hosts":{},"groups":{"0123456789abcdef01234567":{"name":"Private servers","defaultIdentityId":""}},"identities":{},"knownHosts":""},"others":{}}}`)
	key := newVaultKey("my vault password")
	blob, err := encodeVault(data, key)
	if err != nil || bytes.Contains(blob, []byte("Private servers")) {
		t.Fatalf("encrypted host data exposed: %s, %v", blob, err)
	}
	var envelope vaultEnvelope
	if err := json.Unmarshal(blob, &envelope); err != nil || envelope.Version != 1 {
		t.Fatalf("encrypted envelope must use version 1: %v", err)
	}

	decoded, _, protected, err := decodeVault(blob, "my vault password", vaultKey{})
	if err != nil || !protected || !bytes.Equal(decoded, data) {
		t.Fatalf("could not unlock with password: %v", err)
	}

	if _, _, _, err := decodeVault(blob, "wrong password", vaultKey{}); err == nil {
		t.Fatal("wrong password accepted")
	}

	if _, _, _, err := decodeVault(blob, "", vaultKey{}); err == nil {
		t.Fatal("encrypted hosts unlocked without a password or saved key")
	}

	if _, _, _, err := decodeVault(blob, "", key); err != nil {
		t.Fatalf("saved key cannot unlock: %v", err)
	}

	_ = json.Unmarshal(blob, &envelope)
	envelope.Encrypted[12] ^= 1
	tampered, _ := json.Marshal(envelope)
	if _, _, _, err := decodeVault(tampered, "", key); err == nil {
		t.Fatal("modified ciphertext accepted")
	}
}

type vaultServer struct {
	blob      []byte
	digest    string
	protected bool
	fail      bool
	uploads   int
}

func (s *vaultServer) serve(t *testing.T, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.URL.Path == "/api/users/self" {
		cookie, err := r.Cookie("refresh_token")
		if err != nil || cookie.Value == "" {
			t.Error("missing refresh cookie")
		}
		w.Header().Set("X-Issued-Access-Token", "test-access")
		http.SetCookie(w, &http.Cookie{Name: "refresh_token", Value: "rotated-refresh", Path: "/"})
		io.WriteString(w, `{"user_id":"test-user","username":"Gleb"}`)
		return
	}

	if r.Header.Get("Authorization") != "Bearer test-access" || !strings.HasPrefix(r.Header.Get("X-User-Platform"), "Shell Anywhere;") {
		t.Error("provider request did not use authenticated msar client")
	}
	if s.fail {
		http.Error(w, "unavailable", 503)
		return
	}

	if r.URL.Path == "/api/private/vaults" {
		var body struct{ Protected bool }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if s.blob == nil {
			s.protected = body.Protected
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"suid": "0123456789abcdef01234567", "protected": s.protected})
		return
	}
	if r.URL.Path != "/api/private/vaults/0123456789abcdef01234567" {
		http.NotFound(w, r)
		return
	}

	if r.Method == http.MethodPut {
		var body struct {
			Vault, Digest string
			Protected     bool
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		blob, err := base64.StdEncoding.DecodeString(body.Vault)
		if err != nil || body.Digest != vaultDigest(blob) {
			t.Error("incorrect upload digest")
		}
		s.blob, s.digest, s.protected = blob, body.Digest, body.Protected
		s.uploads++
		return
	}

	var body struct {
		Digest       string
		LocalChanges bool `json:"local_changes"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	status, action := "outdated", "init"
	if body.Digest == s.digest {
		status = "synced"
	}
	if body.LocalChanges {
		action = "share"
	} else if s.blob != nil {
		action = base64.StdEncoding.EncodeToString(s.blob)
	}
	_ = json.NewEncoder(w).Encode(map[string]string{"status": status, "action": action})
}

func syncTestApp(t *testing.T, origin string) *App {
	t.Helper()
	t.Setenv("SHELLANYWHERE_PROVIDER_URL", origin+"/api")
	a, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a.accountURL = origin
	a.accountLoaded = true
	a.account = savedAccount{User: Account{UserID: "test-user", Username: "Gleb"}, RefreshToken: "refresh-secret"}
	a.accounts = &memoryPasswordStore{values: map[string]string{}}
	a.passwords = &memoryPasswordStore{values: map[string]string{}}
	a.vault.keys = &memoryPasswordStore{values: map[string]string{}}
	a.eventSink = func(string, any) {}
	return a
}

func TestProviderDefaultAndOverride(t *testing.T) {
	a := syncTestApp(t, "https://oxee.net")
	t.Setenv("SHELLANYWHERE_PROVIDER_URL", "")
	if _, err := a.GetVaultStatus(); err != nil || a.vault.base != "https://shell-aw.oxee.net/api" {
		t.Fatalf("incorrect provider default: %s, %v", a.vault.base, err)
	}

	t.Setenv("SHELLANYWHERE_PROVIDER_URL", "https://shell-aw.test.oxee.net/api/")
	if _, err := a.GetVaultStatus(); err != nil || a.vault.base != "https://shell-aw.test.oxee.net/api" {
		t.Fatalf("provider override ignored: %s, %v", a.vault.base, err)
	}
}

func TestProviderRefreshOnSeparateHost(t *testing.T) {
	cloud := &vaultServer{}
	profiles, requests := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host == "127.0.0.1" && r.URL.Path == "/api/users/self" {
			profiles++
			cloud.serve(t, w, r)
			return
		}
		if r.Host != "localhost" {
			t.Errorf("unexpected provider host: %s", r.Host)
		}
		if _, err := r.Cookie("refresh_token"); err == nil {
			t.Error("account's host-only refresh cookie reached the provider")
		}
		requests++
		if requests == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		cloud.serve(t, w, r)
	}))
	defer server.Close()

	a := syncTestApp(t, "http://127.0.0.1")
	t.Setenv("SHELLANYWHERE_PROVIDER_URL", "http://localhost/api")
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	defer transport.CloseIdleConnections()
	a.accountHTTP = &http.Client{Transport: transport}

	status, err := a.ConfigureVaultSync("vault-password", "")
	if err != nil || !status.Enabled || profiles != 2 || cloud.uploads != 1 {
		t.Fatalf("cross-host authentication failed: %+v, profiles=%d, %v", status, profiles, err)
	}
	if a.account.RefreshToken != "rotated-refresh" {
		t.Fatal("rotated refresh token was not saved")
	}
}

func TestEncryptedVaultSyncAndCredentials(t *testing.T) {
	t.Setenv("SHELLANYWHERE_PROVIDER_URL", "")
	cloud := &vaultServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cloud.serve(t, w, r) }))
	defer server.Close()
	a := syncTestApp(t, server.URL)
	host, err := a.SaveHost(Host{Address: "private.example", Port: 22, AuthMethod: "password"}, "ssh-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ConfigureVaultSync("", ""); err == nil || cloud.uploads != 0 {
		t.Fatal("credentials must require a vault password before any upload")
	}
	status, err := a.ConfigureVaultSync("vault-secret", "")
	if err != nil || !status.Enabled || !status.Protected || !cloud.protected {
		t.Fatalf("encrypted sync: %+v, %v", status, err)
	}
	for _, secret := range []string{"private.example", "ssh-secret", "vault-secret"} {
		if bytes.Contains(cloud.blob, []byte(secret)) {
			t.Fatal("plaintext exposed in cloud")
		}
	}
	settings, _ := os.ReadFile(filepath.Join(a.configDir, "vault.json"))
	if bytes.Contains(settings, []byte("vault-secret")) || bytes.Contains(settings, []byte("ssh-secret")) {
		t.Fatal("secret stored in sync metadata")
	}
	restored, err := NewApp(a.configDir)
	if err != nil {
		t.Fatal(err)
	}
	restored.accountURL, restored.accountLoaded, restored.account = server.URL, true, a.account
	restored.accounts, restored.vault.keys, restored.passwords = a.accounts, a.vault.keys, a.passwords
	restored.eventSink = func(string, any) {}
	status, err = restored.GetVaultStatus()
	if err != nil || !status.Enabled || !status.Protected || status.Locked {
		t.Fatalf("saved key restore: %+v, %v", status, err)
	}
	b := syncTestApp(t, server.URL)
	if _, err := b.ConfigureVaultSync("wrong", ""); err == nil {
		t.Fatal("wrong password accepted")
	}
	if _, err := b.ConfigureVaultSync("vault-secret", ""); err != nil {
		t.Fatal(err)
	}
	if len(b.hosts) != 1 || !b.hosts[0].SavedPassword {
		t.Fatal("download did not restore password availability")
	}
	if password, _ := b.passwords.Get(host.ID); password != "ssh-secret" {
		t.Fatal("download did not restore SSH password")
	}
	a.hosts[0].Name = "Updated host"
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if b.hosts[0].Name != "Updated host" {
		t.Fatal("remote edit not downloaded")
	}
	b.hosts[0].Name = "Edited on other device"
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if !a.hosts[0].SavedPassword {
		t.Fatal("remote update removed local credentials")
	}
	if secret, _ := a.passwords.Get(host.ID); secret != "ssh-secret" {
		t.Fatal("local SSH secret changed")
	}
	if a.account.RefreshToken != "rotated-refresh" {
		t.Fatal("rotated refresh session not saved")
	}

	before := cloud.uploads
	cloud.fail = true
	a.hosts[0].Name = "Offline edit"
	if _, err := a.SyncVault(); err == nil {
		t.Fatal("server failure ignored")
	}
	if a.hosts[0].Name != "Offline edit" {
		t.Fatal("network failure lost local edit")
	}
	cloud.fail = false
	if _, err := a.SyncVault(); err != nil || cloud.uploads != before+1 {
		t.Fatalf("dirty edit not retried: %v", err)
	}
	if _, err := a.PauseVaultSync(); err != nil {
		t.Fatal(err)
	}
	a.hosts[0].Name = "Local only"
	if _, err := a.SyncVault(); err != nil || cloud.uploads != before+1 {
		t.Fatal("paused sync uploaded")
	}
}

func TestFirstSyncChoiceAndLocalEditProtection(t *testing.T) {
	t.Setenv("SHELLANYWHERE_PROVIDER_URL", "")
	cloud := &vaultServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cloud.serve(t, w, r) }))
	defer server.Close()
	a, b := syncTestApp(t, server.URL), syncTestApp(t, server.URL)
	_, _ = a.SaveHost(Host{Address: "cloud.example", Port: 22, AuthMethod: "password"}, "secret")
	_, _ = b.SaveHost(Host{Address: "local.example", Port: 22, AuthMethod: "password"}, "other-secret")
	if _, err := a.ConfigureVaultSync("vault-secret", ""); err != nil {
		t.Fatal(err)
	}
	status, err := b.ConfigureVaultSync("vault-secret", "")
	if err != nil || !status.NeedsChoice || cloud.uploads != 1 || b.hosts[0].Address != "local.example" {
		t.Fatal("first sync overwrote data before choosing")
	}
	if _, err := b.ConfigureVaultSync("vault-secret", "download"); err != nil {
		t.Fatal(err)
	}
	if b.hosts[0].Address != "cloud.example" {
		t.Fatal("chosen cloud list not applied")
	}
	backup, _ := os.ReadFile(filepath.Join(b.configDir, "hosts-before-sync.json"))
	if !bytes.Contains(backup, []byte("local.example")) {
		t.Fatal("local backup missing")
	}
	previous, _ := b.vaultSnapshot()
	b.hosts[0].Name = "Edit during download"
	if applied, err := b.applyVault(previous, vaultDigest(previous)); err != nil || applied {
		t.Fatal("in-flight download overwrote a newer local edit")
	}
	current, _ := b.vaultSnapshot()
	if _, err := b.applyVault([]byte("null"), vaultDigest(current)); err == nil {
		t.Fatal("invalid cloud data accepted")
	}

	b.account.User.UserID = "different-user"
	status, err = b.GetVaultStatus()
	if err != nil || status.Enabled {
		t.Fatal("different account inherited enabled sync")
	}
}
