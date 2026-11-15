package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

// Exercise msar and the provider protocol without opening local sockets
type vaultTransport func(*http.Request) (*http.Response, error)

func (f vaultTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func (s *vaultServer) testApp(t *testing.T) *App {
	t.Helper()
	a := syncTestApp(t, "http://127.0.0.1")
	a.accountHTTP = &http.Client{Transport: vaultTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		s.serve(t, w, r)
		return w.Result(), nil
	})}
	return a
}

func TestPasswordlessVaultSyncAndCredentials(t *testing.T) {
	cloud := &vaultServer{}
	a := cloud.testApp(t)
	group, err := a.CreateGroup("Servers")
	if err != nil {
		t.Fatal(err)
	}
	host, err := a.SaveHost(Host{Address: "private.example", Port: 22, AuthMethod: "password", Group: group.ID, Favorite: true}, "ssh-secret")
	if err != nil {
		t.Fatal(err)
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "sync test", []byte("key-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	keyBytes := pem.EncodeToMemory(block)
	source := filepath.Join(t.TempDir(), "original-key")
	if err := os.WriteFile(source, keyBytes, 0600); err != nil {
		t.Fatal(err)
	}
	keyHost, err := a.SaveHost(Host{Address: "key.example", Port: 22, AuthMethod: "key", KeyPath: source}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.passwords.Set(passphraseID(keyHost.ID), "key-passphrase"); err != nil {
		t.Fatal(err)
	}
	status, err := a.ConfigureVaultSync("", "")
	if err != nil || !status.Enabled || status.Protected || status.Locked || cloud.protected {
		t.Fatalf("could not enable sync without a vault password: %+v, %v", status, err)
	}
	if len(a.vault.keys.(*memoryPasswordStore).values) != 0 {
		t.Fatal("passwordless syncing unexpectedly needs a saved vault key")
	}
	if _, key, protected, err := decodeVault(cloud.blob, "", vaultKey{}); err != nil || protected || len(key.Key) != 0 {
		t.Fatalf("uploaded vault requires an encryption key: %v", err)
	}

	b := cloud.testApp(t)
	status, err = b.ConfigureVaultSync("", "")
	if err != nil || !status.Enabled || status.Protected || status.Locked || len(b.ListHosts()) != 2 || len(b.ListGroups()) != 1 {
		t.Fatalf("fresh device could not sync without a password: %+v, %v", status, err)
	}
	if password, err := b.passwords.Get(host.ID); err != nil || password != "ssh-secret" {
		t.Fatal("SSH password was not restored")
	}
	if password, err := b.passwords.Get(passphraseID(keyHost.ID)); err != nil || password != "key-passphrase" {
		t.Fatal("key passphrase was not restored")
	}
	for _, restored := range b.ListHosts() {
		if restored.ID == keyHost.ID {
			data, err := os.ReadFile(restored.KeyPath)
			if err != nil || !bytes.Equal(data, keyBytes) || restored.KeyPath == source {
				t.Fatal("private key was not restored to a device-local path")
			}
		}
	}
	host.Name = "Updated host"
	if _, err := a.SaveHost(host, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err != nil {
		t.Fatalf("passwordless upload failed: %v", err)
	}
	if _, err := b.SyncVault(); err != nil {
		t.Fatalf("passwordless download failed: %v", err)
	}
	for _, restored := range b.ListHosts() {
		if restored.ID == host.ID && (restored.Name != host.Name || !restored.Favorite || restored.Group != group.ID) {
			t.Fatal("host updates or group membership were lost")
		}
	}
	before := cloud.uploads
	if _, err := a.PauseVaultSync(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err != nil || cloud.uploads != before {
		t.Fatal("paused sync uploaded")
	}
	if status, err := a.ConfigureVaultSync("", ""); err != nil || !status.Enabled || status.Protected {
		t.Fatalf("could not resume without a vault password: %v", err)
	}
	restored, err := NewApp(a.configDir)
	if err != nil {
		t.Fatal(err)
	}
	restored.accountURL, restored.accountLoaded, restored.account = a.accountURL, true, a.account
	restored.accounts, restored.passwords, restored.vault.keys = a.accounts, a.passwords, a.vault.keys
	restored.accountHTTP = a.accountHTTP
	restored.eventSink = func(string, any) {}
	if status, err := restored.GetVaultStatus(); err != nil || !status.Enabled || status.Protected || status.Locked {
		t.Fatalf("restarting lost passwordless syncing: %+v, %v", status, err)
	}
	if _, err := restored.SyncVault(); err != nil {
		t.Fatalf("could not sync after restarting: %v", err)
	}
	for _, file := range []string{"hosts.json", "vault.json"} {
		data, err := os.ReadFile(filepath.Join(b.configDir, file))
		if err != nil || bytes.Contains(data, []byte("ssh-secret")) || bytes.Contains(data, []byte("key-passphrase")) {
			t.Fatalf("local metadata contains an SSH credential: %s", file)
		}
	}
}

func TestUnprotectedVaultValidation(t *testing.T) {
	for _, data := range []json.RawMessage{
		nil, json.RawMessage(`null`), json.RawMessage(`{"version":2}`), json.RawMessage(`{"version":1}`),
	} {
		blob, err := json.Marshal(vaultEnvelope{Version: 1, Data: data})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := decodeVault(blob, "", vaultKey{}); err == nil {
			t.Fatal("invalid unprotected vault was accepted")
		}
	}
}

func TestPasswordCanBeAddedToExistingUnprotectedVault(t *testing.T) {
	cloud := &vaultServer{}
	a := cloud.testApp(t)
	_, err := a.SaveHost(Host{Address: "private.example", Port: 22, AuthMethod: "password"}, "ssh-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ConfigureVaultSync("", ""); err != nil {
		t.Fatal(err)
	}
	if status, err := a.ConfigureVaultSync("vault-password", ""); err != nil || !status.Protected || !cloud.protected {
		t.Fatalf("optional encryption could not be enabled: %+v, %v", status, err)
	}
	if bytes.Contains(cloud.blob, []byte("ssh-secret")) {
		t.Fatal("adding a password did not encrypt the cloud vault")
	}
	b := cloud.testApp(t)
	if status, err := b.ConfigureVaultSync("", ""); err == nil || !status.Locked || !status.Protected {
		t.Fatal("password-protected vault unlocked without its password")
	}
	if _, err := b.ConfigureVaultSync("vault-password", ""); err != nil {
		t.Fatalf("optional password could not unlock a fresh device: %v", err)
	}
}
