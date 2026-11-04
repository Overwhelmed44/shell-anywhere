package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type memoryPasswordStore struct{ values map[string]string }

func (s *memoryPasswordStore) Set(id, password string) error {
	s.values[id] = password
	return nil
}

func (s *memoryPasswordStore) Get(id string) (string, error) {
	password, ok := s.values[id]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return password, nil
}

func (s *memoryPasswordStore) Delete(id string) error {
	if _, ok := s.values[id]; !ok {
		return keyring.ErrNotFound
	}
	delete(s.values, id)
	return nil
}

func TestSavedPasswordAndHostDefaults(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryPasswordStore{values: make(map[string]string)}
	app.passwords = store

	if _, err := app.SaveHost(Host{Address: "example.test", Port: 22, AuthMethod: "password"}, ""); err == nil {
		t.Fatal("a new saved password must have a value")
	}
	host, err := app.SaveHost(Host{Address: " example.test ", Port: 22, AuthMethod: "password"}, "top-secret")
	if err != nil {
		t.Fatal(err)
	}
	if host.Name != "example.test" || host.Username != "root" || !host.SavedPassword {
		t.Fatalf("unexpected defaults: %+v", host)
	}
	if password, err := store.Get(host.ID); err != nil || password != "top-secret" {
		t.Fatalf("saved password: %q, %v", password, err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "hosts.json"))
	if err != nil || strings.Contains(string(data), "top-secret") {
		t.Fatalf("password leaked into hosts.json: %v", err)
	}

	reloaded, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.passwords = store
	if got := reloaded.ListHosts(); len(got) != 1 || got[0] != host {
		t.Fatalf("reloaded hosts: %+v", got)
	}
	host.Favorite = true
	host, err = reloaded.SaveHost(host, "")
	if err != nil {
		t.Fatal(err)
	}
	if password, err := store.Get(host.ID); err != nil || password != "top-secret" {
		t.Fatalf("editing host lost password: %q, %v", password, err)
	}
	if _, err := reloaded.SaveHost(host, "new-secret"); err != nil {
		t.Fatal(err)
	}
	if password, err := store.Get(host.ID); err != nil || password != "new-secret" {
		t.Fatalf("password was not replaced: %q, %v", password, err)
	}
	host.SavedPassword = false
	host, err = reloaded.SaveHost(host, "")
	if err != nil {
		t.Fatal(err)
	}
	if !host.SavedPassword {
		t.Fatal("editing a password host must keep its saved password")
	}
	if password, err := store.Get(host.ID); err != nil || password != "new-secret" {
		t.Fatalf("editing host lost password: %q, %v", password, err)
	}
	host.AuthMethod = "key"
	host.KeyPath = "test-key"
	host, err = reloaded.SaveHost(host, "")
	if err != nil {
		t.Fatal(err)
	}
	if host.SavedPassword {
		t.Fatal("key host must not retain a saved password")
	}
	if _, err := store.Get(host.ID); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("switching to a key left the password behind: %v", err)
	}
	if err := reloaded.DeleteHost(host.ID); err != nil {
		t.Fatal(err)
	}
	other, err := reloaded.SaveHost(Host{Address: "other.test", Port: 22, AuthMethod: "password"}, "delete-me")
	if err != nil {
		t.Fatal(err)
	}
	if err := reloaded.DeleteHost(other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(other.ID); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("deleting host left saved password behind: %v", err)
	}
}

func TestGroupsPersistAndHostsUseExistingGroups(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	app.passwords = &memoryPasswordStore{values: make(map[string]string)}
	if _, err := app.SaveHost(Host{Address: "example.test", Port: 22, AuthMethod: "password", Group: "Work"}, "secret"); err == nil {
		t.Fatal("host accepted a group that does not exist")
	}
	group, err := app.CreateGroup("  Work  ")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.CreateGroup("work"); err == nil {
		t.Fatal("duplicate group accepted")
	}
	if _, err := app.CreateGroup("Favorites"); err == nil {
		t.Fatal("reserved group name accepted")
	}
	host, err := app.SaveHost(Host{Address: "example.test", Port: 22, AuthMethod: "password", Group: group.ID}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteHost(host.ID); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	if groups := reloaded.ListGroups(); len(groups) != 1 || groups[0].Name != "Work" {
		t.Fatalf("empty group was not retained: %v", groups)
	}
}

func TestDeleteGroupKeepsHostsAndCredentials(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryPasswordStore{values: make(map[string]string)}
	app.passwords = store
	group, err := app.CreateGroup("Work")
	if err != nil {
		t.Fatal(err)
	}
	host, err := app.SaveHost(Host{Address: "example.test", Port: 22, AuthMethod: "password", Group: group.ID}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := app.DeleteGroup("Missing"); err == nil {
		t.Fatal("missing group was deleted")
	}
	if err := app.DeleteGroup(group.ID); err != nil {
		t.Fatal(err)
	}
	if groups := app.ListGroups(); len(groups) != 0 {
		t.Fatalf("deleted group remains: %v", groups)
	}
	if password, err := store.Get(host.ID); err != nil || password != "secret" {
		t.Fatalf("host password changed: %q, %v", password, err)
	}
	reloaded, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	if groups := reloaded.ListGroups(); len(groups) != 0 {
		t.Fatalf("deleted group returned after reload: %v", groups)
	}
	got := reloaded.ListHosts()
	if len(got) != 1 || got[0].ID != host.ID || got[0].Group != "" || !got[0].SavedPassword {
		t.Fatalf("host was not kept without a group: %+v", got)
	}
}

func TestHostsPersistAndValidate(t *testing.T) {
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	store := &memoryPasswordStore{values: make(map[string]string)}
	app.passwords = store
	if _, err := app.SaveHost(Host{Name: "Incomplete", Port: 22, Username: "root", AuthMethod: "password"}, ""); err == nil {
		t.Fatal("expected an incomplete host to be rejected")
	}
	host, err := app.SaveHost(Host{Name: " Staging ", Address: "example.test", Port: 22, Username: "deploy", AuthMethod: "password", Favorite: true}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if host.ID == "" || host.Name != "Staging" {
		t.Fatalf("unexpected saved host: %+v", host)
	}
	reloaded, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.passwords = store
	got := reloaded.ListHosts()
	if len(got) != 1 || got[0] != host {
		t.Fatalf("persisted hosts: %+v", got)
	}
	if err := reloaded.DeleteHost(host.ID); err != nil {
		t.Fatal(err)
	}
	if len(reloaded.ListHosts()) != 0 {
		t.Fatal("host was not deleted")
	}
}

func TestInteractiveSession(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	server := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "secret" {
			return nil, ssh.ErrNoAuth
		}
		return nil, nil
	}}
	server.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				serverConn, channels, requests, err := ssh.NewServerConn(conn, server)
				if err != nil {
					_ = conn.Close()
					return
				}
				defer serverConn.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					if channel.ChannelType() != "session" {
						_ = channel.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					remote, reqs, err := channel.Accept()
					if err != nil {
						return
					}
					go func() {
						defer remote.Close()
						for req := range reqs {
							switch req.Type {
							case "pty-req", "window-change":
								_ = req.Reply(true, nil)
							case "shell":
								_ = req.Reply(true, nil)
								_, _ = remote.Write([]byte("ready\r\n"))
								buffer := make([]byte, 256)
								for {
									n, err := remote.Read(buffer)
									if err != nil {
										return
									}
									_, _ = remote.Write(buffer[:n])
								}
							default:
								_ = req.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.passwords = &memoryPasswordStore{values: make(map[string]string)}
	events := make(chan terminalEvent, 16)
	app.eventSink = func(name string, data any) {
		if name == "terminal:output" {
			events <- data.(terminalEvent)
		}
	}
	port := listener.Addr().(*net.TCPAddr).Port
	host, err := app.SaveHost(Host{Name: "Local", Address: "127.0.0.1", Port: port, Username: "test", AuthMethod: "password", SavedPassword: true}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := app.ConnectHost(host.ID, "", "", "")
	if err != nil || challenge.Status != "host_key_unknown" {
		t.Fatalf("initial challenge: %+v, %v", challenge, err)
	}
	result, err := app.ConnectHost(host.ID, "", "", challenge.Fingerprint)
	if err != nil || result.Status != "connected" {
		t.Fatalf("connection: %+v, %v", result, err)
	}
	defer app.DisconnectSession(result.SessionID)
	read := func(want string) {
		t.Helper()
		select {
		case event := <-events:
			if event.SessionID != result.SessionID {
				t.Fatalf("wrong session: %s", event.SessionID)
			}
			decoded, err := base64.StdEncoding.DecodeString(event.Data)
			if err != nil || !strings.Contains(string(decoded), want) {
				t.Fatalf("output %q, error %v; want %q", decoded, err, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %q", want)
		}
	}
	read("ready")
	if err := app.ResizeSession(result.SessionID, 40, 120); err != nil {
		t.Fatal(err)
	}
	if err := app.WriteSession(result.SessionID, "ping\n"); err != nil {
		t.Fatal(err)
	}
	read("ping")
	if _, err := app.ReconnectHost(result.SessionID, host.ID, "", "", ""); err == nil {
		t.Fatal("reconnecting an active session must fail")
	}
	app.DisconnectSession(result.SessionID)
	reconnected, err := app.ReconnectHost(result.SessionID, host.ID, "", "", "")
	if err != nil || reconnected.Status != "connected" || reconnected.SessionID != result.SessionID {
		t.Fatalf("reconnection did not reuse the session: %+v, %v", reconnected, err)
	}
	read("ready")
	if err := app.WriteSession(result.SessionID, "again\n"); err != nil {
		t.Fatal(err)
	}
	read("again")
	knownHosts, err := os.ReadFile(filepath.Join(app.configDir, "known_hosts"))
	if err != nil || !strings.Contains(string(knownHosts), "ssh-ed25519") {
		t.Fatalf("trusted host key not saved: %v", err)
	}
}

func TestUnknownAndChangedHostKeys(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	server := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) { return nil, nil }}
	server.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() { _, _, _, _ = ssh.NewServerConn(conn, server); _ = conn.Close() }()
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	dir := t.TempDir()
	app, err := NewApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	app.passwords = &memoryPasswordStore{values: make(map[string]string)}
	host, err := app.SaveHost(Host{Name: "Local test", Address: "127.0.0.1", Port: port, Username: "test", AuthMethod: "password"}, "secret")
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.ConnectHost(host.ID, "secret", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "host_key_unknown" || result.Fingerprint != ssh.FingerprintSHA256(signer.PublicKey()) {
		t.Fatalf("unexpected first-connection result: %+v", result)
	}
	_, differentKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	differentSigner, err := ssh.NewSignerFromKey(differentKey)
	if err != nil {
		t.Fatal(err)
	}
	address := net.JoinHostPort(host.Address, strconv.Itoa(port))
	line := knownhosts.Line([]string{knownhosts.Normalize(address)}, differentSigner.PublicKey())
	if err := os.WriteFile(filepath.Join(dir, "known_hosts"), []byte(line+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = app.ConnectHost(host.ID, "secret", "", result.Fingerprint)
	if err == nil || !strings.Contains(err.Error(), "host key verification failed") {
		t.Fatalf("changed host key must be rejected, got %v", err)
	}
}
