package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func vaultSSHServer(t *testing.T, public ssh.PublicKey) (int, string) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) != "restored-ssh-password" {
				return nil, errors.New("wrong password")
			}
			return nil, nil
		},
		PublicKeyCallback: func(_ ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if !bytes.Equal(key.Marshal(), public.Marshal()) {
				return nil, errors.New("wrong private key")
			}
			return nil, nil
		},
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					conn.Close()
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for request := range channels {
					channel, requests, err := request.Accept()
					if err != nil {
						return
					}
					go func() {
						defer channel.Close()
						for request := range requests {
							request.Reply(request.Type == "pty-req" || request.Type == "shell", nil)
						}
					}()
				}
			}()
		}
	}()
	return listener.Addr().(*net.TCPAddr).Port, ssh.FingerprintSHA256(signer.PublicKey())
}

func TestFreshDeviceConnectsWithSyncedPasswordAndEncryptedPrivateKey(t *testing.T) {
	t.Setenv("SHELLANYWHERE_PROVIDER_URL", "")
	cloud := &vaultServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cloud.serve(t, w, r) }))
	defer server.Close()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
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
	port, fingerprint := vaultSSHServer(t, signer.PublicKey())
	a := syncTestApp(t, server.URL)
	group, err := a.CreateGroup("Production")
	if err != nil {
		t.Fatal(err)
	}
	passwordHost, err := a.SaveHost(Host{Address: "127.0.0.1", Port: port, Username: "password-user", AuthMethod: "password", Group: group.ID, Favorite: true}, "restored-ssh-password")
	if err != nil {
		t.Fatal(err)
	}
	keyHost, err := a.SaveHost(Host{Address: "127.0.0.1", Port: port, Username: "key-user", AuthMethod: "key", KeyPath: source}, "")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := a.ConnectHost(keyHost.ID, "", "", ""); err != nil || result.Status != "passphrase_required" {
		t.Fatalf("missing passphrase prompt: %+v, %v", result, err)
	}
	connected, err := a.ConnectHost(keyHost.ID, "", "key-passphrase", fingerprint)
	if err != nil || connected.Status != "connected" {
		t.Fatalf("source connection: %+v, %v", connected, err)
	}
	a.DisconnectSession(connected.SessionID)
	if saved, _ := a.passwords.Get(passphraseID(keyHost.ID)); saved != "key-passphrase" {
		t.Fatal("successful key authentication did not remember passphrase")
	}
	if _, err := a.ConfigureVaultSync("vault-password", ""); err != nil {
		t.Fatal(err)
	}
	for _, secret := range [][]byte{keyBytes, []byte("restored-ssh-password"), []byte("key-passphrase"), []byte(source)} {
		if bytes.Contains(cloud.blob, secret) {
			t.Fatal("cloud payload exposes a credential or local file path")
		}
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	b := syncTestApp(t, server.URL)
	if _, err := b.ConfigureVaultSync("vault-password", ""); err != nil {
		t.Fatal(err)
	}
	for _, host := range b.ListHosts() {
		if host.AuthMethod == "key" {
			data, err := os.ReadFile(host.KeyPath)
			if err != nil || !bytes.Equal(data, keyBytes) || host.KeyPath == source {
				t.Fatal("key not restored with a device-local path")
			}
		}
		result, err := b.ConnectHost(host.ID, "", "", "")
		if err != nil || result.Status != "connected" {
			t.Fatalf("fresh device could not connect immediately: %+v, %v", result, err)
		}
		b.DisconnectSession(result.SessionID)
	}
	if password, _ := b.passwords.Get(passwordHost.ID); password != "restored-ssh-password" {
		t.Fatal("SSH password missing")
	}
	if len(b.ListGroups()) != 1 {
		t.Fatal("groups missing")
	}
	before := cloud.uploads
	if _, err := b.SyncVault(); err != nil || cloud.uploads != before {
		t.Fatalf("restoring credentials caused spurious uploads: %v", err)
	}
	for _, file := range []string{"hosts.json", "vault.json", "hosts-before-sync.json"} {
		data, _ := os.ReadFile(filepath.Join(b.configDir, file))
		if bytes.Contains(data, []byte("restored-ssh-password")) || bytes.Contains(data, keyBytes) || bytes.Contains(data, []byte("key-passphrase")) {
			t.Fatalf("credential exposed in %s", file)
		}
	}
}

func TestCredentialVaultRejectsPlaintextAndMissingKeys(t *testing.T) {
	a := syncTestApp(t, "http://localhost")
	_, err := a.SaveHost(Host{Address: "example.test", Port: 22, AuthMethod: "key", KeyPath: filepath.Join(t.TempDir(), "missing-key")}, "")
	if err != nil {
		t.Fatal(err)
	}
	data, err := a.vaultSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encodeVault(data, vaultKey{}); err == nil {
		t.Fatal("credential vault accepted without encryption")
	}
	if _, err := encodeVault(data, newVaultKey("password")); err == nil {
		t.Fatal("missing key silently uploaded")
	}
	plain, _ := json.Marshal(map[string]any{"version": 1, "protected": false, "data": json.RawMessage(data)})
	if _, _, _, err := decodeVault(plain, "", vaultKey{}); err == nil {
		t.Fatal("plaintext credentials accepted")
	}
}

func TestCredentialRestoreRollsBackInvalidTrustedKeys(t *testing.T) {
	a := syncTestApp(t, "http://localhost")
	host, err := a.SaveHost(Host{Address: "old.example", Port: 22, AuthMethod: "password"}, "old-secret")
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.vaultSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var contents vaultData
	json.Unmarshal(before, &contents)
	password := "replacement-secret"
	item := contents.Workspaces.Personal.Hosts[host.ID]
	item.Password = &password
	item.Address = "new.example"
	contents.Workspaces.Personal.Hosts[host.ID] = item
	invalid := "this is not a trusted host key\n"
	contents.Workspaces.Personal.KnownHosts = invalid
	data, _ := json.Marshal(contents)
	if _, err := a.applyVault(data, vaultDigest(before)); err == nil {
		t.Fatal("invalid trusted keys accepted")
	}
	after, err := a.vaultSnapshot()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed restore changed local data")
	}
	if secret, _ := a.passwords.Get(host.ID); secret != "old-secret" {
		t.Fatal("failed restore changed password")
	}
	if _, err := knownhosts.New(filepath.Join(a.configDir, "known_hosts")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed restore left trusted-key file behind")
	}
}

type failingVaultStore struct {
	memoryPasswordStore
	failID string
}

func (s *failingVaultStore) Set(id, value string) error {
	if s.failID == id {
		s.failID = ""
		return errors.New("credential store unavailable")
	}
	return s.memoryPasswordStore.Set(id, value)
}

func TestCredentialStoreFailureRollsBackEarlierPasswords(t *testing.T) {
	a := syncTestApp(t, "http://localhost")
	for _, name := range []string{"first.example", "second.example"} {
		if _, err := a.SaveHost(Host{Address: name, Port: 22, AuthMethod: "password"}, "original-password"); err != nil {
			t.Fatal(err)
		}
	}
	before, err := a.vaultSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var cloud vaultData
	json.Unmarshal(before, &cloud)
	changed := "changed-password"
	for id, host := range cloud.Workspaces.Personal.Hosts {
		host.Password = &changed
		cloud.Workspaces.Personal.Hosts[id] = host
	}
	ids := []string{}
	for id := range cloud.Workspaces.Personal.Hosts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	a.passwords = &failingVaultStore{memoryPasswordStore: *a.passwords.(*memoryPasswordStore), failID: ids[1]}
	data, _ := json.Marshal(cloud)
	if _, err := a.applyVault(data, vaultDigest(before)); err == nil {
		t.Fatal("credential-store error ignored")
	}
	after, err := a.vaultSnapshot()
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("partial credential restore was not rolled back")
	}
}

func TestPasswordUpdatesAndDeletionSyncToOtherDevice(t *testing.T) {
	t.Setenv("SHELLANYWHERE_PROVIDER_URL", "")
	cloud := &vaultServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cloud.serve(t, w, r) }))
	defer server.Close()
	a, b := syncTestApp(t, server.URL), syncTestApp(t, server.URL)
	host, err := a.SaveHost(Host{Address: "example.test", Port: 22, AuthMethod: "password"}, "original-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ConfigureVaultSync("vault-password", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := b.ConfigureVaultSync("vault-password", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveHost(host, "changed-password"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if value, _ := b.passwords.Get(host.ID); value != "changed-password" {
		t.Fatal("password-only edit was not synced")
	}
	if err := a.DeleteHost(host.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if len(b.ListHosts()) != 0 {
		t.Fatal("host deletion was not synced")
	}
	if _, err := b.passwords.Get(host.ID); err == nil {
		t.Fatal("deleted host left credentials on the other device")
	}
}
