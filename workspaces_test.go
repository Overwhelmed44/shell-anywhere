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

func TestIdentityReferencesSyncAndConnect(t *testing.T) {
	cloud := &vaultServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cloud.serve(t, w, r) }))
	defer server.Close()
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	sshPublic, _ := ssh.NewPublicKey(public)
	port, fingerprint := vaultSSHServer(t, sshPublic)
	a := syncTestApp(t, server.URL)
	identity, err := a.SaveIdentity(Identity{Name: "Deploy", Username: "deploy", AuthMethod: "password"}, "restored-ssh-password", "")
	if err != nil {
		t.Fatal(err)
	}
	group, err := a.SaveGroup(Group{Name: "Production", DefaultIdentityID: identity.ID})
	if err != nil {
		t.Fatal(err)
	}
	direct, err := a.SaveHost(Host{Name: "Direct", Address: "127.0.0.1", Port: port, IdentityID: identity.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	inherited, err := a.SaveHost(Host{Name: "Inherited", Address: "127.0.0.1", Port: port, Group: group.ID, UseGroupIdentity: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	if direct.Username != "deploy" || inherited.Username != "deploy" || !inherited.SavedPassword {
		t.Fatal("identity credentials not resolved")
	}
	group.Name = "Renamed production"
	if _, err := a.SaveGroup(group); err != nil {
		t.Fatal(err)
	}
	identity.Name, identity.Username = "Renamed deploy", "new-user"
	if _, err := a.SaveIdentity(identity, "", ""); err != nil {
		t.Fatal(err)
	}
	for _, host := range a.ListHosts() {
		if host.Username != "new-user" {
			t.Fatal("identity edit did not update a dependent host")
		}
	}
	connected, err := a.ConnectHost(direct.ID, "", "", fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	a.DisconnectSession(connected.SessionID)
	if _, err := a.ConfigureVaultSync("vault-password", ""); err != nil {
		t.Fatal(err)
	}
	plain, _, _, err := decodeVault(cloud.blob, "vault-password", vaultKey{})
	if err != nil {
		t.Fatal(err)
	}
	var data vaultData
	if err := json.Unmarshal(plain, &data); err != nil {
		t.Fatal(err)
	}
	personal := data.Workspaces.Personal
	if len(personal.Identities) != 1 || len(personal.Hosts) != 2 || len(data.Workspaces.Others) != 0 {
		t.Fatal("incorrect workspace maps")
	}
	for _, host := range personal.Hosts {
		if host.Password != nil || len(host.PrivateKey) != 0 || host.Username != "" || host.AuthMethod != "" {
			t.Fatal("identity credentials duplicated in a host")
		}
	}
	if bytes.Count(plain, []byte("restored-ssh-password")) != 1 || bytes.Contains(plain, []byte(`"id":`)) {
		t.Fatal("credentials or record IDs duplicated in JSON")
	}
	b := syncTestApp(t, server.URL)
	if _, err := b.ConfigureVaultSync("vault-password", ""); err != nil {
		t.Fatal(err)
	}
	for _, host := range b.ListHosts() {
		result, err := b.ConnectHost(host.ID, "", "", fingerprint)
		if err != nil || result.Status != "connected" {
			t.Fatalf("synced identity could not connect: %+v, %v", result, err)
		}
		b.DisconnectSession(result.SessionID)
	}
	before := cloud.uploads
	if _, err := b.SyncVault(); err != nil || cloud.uploads != before {
		t.Fatalf("identity restore caused another upload: %v", err)
	}
	if err := b.DeleteGroup(group.ID); err != nil {
		t.Fatal(err)
	}
	for _, host := range b.ListHosts() {
		if host.ID == inherited.ID && (host.Group != "" || host.UseGroupIdentity || host.IdentityID != identity.ID || host.Username != "new-user") {
			t.Fatal("deleting a group lost its inherited identity")
		}
	}
	for _, file := range []string{"hosts.json", "groups.json", "identities.json", "hosts-before-sync.json"} {
		blob, _ := os.ReadFile(filepath.Join(b.configDir, file))
		if bytes.Contains(blob, []byte("restored-ssh-password")) {
			t.Fatalf("password leaked into %s", file)
		}
	}
}

func TestOtherWorkspaceStructureRoundTripAndReferenceValidation(t *testing.T) {
	a := syncTestApp(t, "http://127.0.0.1")
	before, err := a.vaultSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	var data vaultData
	_ = json.Unmarshal(before, &data)
	id := "0123456789abcdef01234567"
	identityID := "abcdef0123456789abcdef01"
	password := "other-workspace-password"
	data.Workspaces.Others[id] = vaultWorkspace{
		Name: "Future team", Hosts: map[string]vaultHost{}, Groups: map[string]vaultGroup{}, KnownHosts: "",
		Identities: map[string]vaultIdentity{identityID: {Name: "Team identity", Username: "team", AuthMethod: "password", Password: &password}},
	}
	blob, _ := json.Marshal(data)
	if applied, err := a.applyVault(blob, vaultDigest(before)); err != nil || !applied {
		t.Fatalf("workspace restore failed: %v", err)
	}
	after, err := a.vaultSnapshot()
	if err != nil || !bytes.Equal(blob, after) {
		t.Fatal("other workspace structure was not preserved")
	}
	if len(a.ListIdentities()) != 0 || a.GetWorkspace().ID != "personal" {
		t.Fatal("other workspace identities leaked into Personal")
	}
	if _, err := a.SaveHost(Host{Address: "example.test", Port: 22, IdentityID: identityID}, ""); err == nil {
		t.Fatal("host referenced an identity from another workspace")
	}
	if _, err := a.SaveGroup(Group{Name: "Broken", DefaultIdentityID: identityID}); err == nil {
		t.Fatal("group referenced an identity from another workspace")
	}
	data.Workspaces.Personal.Hosts["111111111111111111111111"] = vaultHost{Address: "example.test", Port: 22, IdentityID: identityID}
	invalid, _ := json.Marshal(data)
	if _, err := a.applyVault(invalid, vaultDigest(after)); err == nil {
		t.Fatal("cloud reference crossed workspace boundaries")
	}
	unchanged, _ := a.vaultSnapshot()
	if !bytes.Equal(after, unchanged) {
		t.Fatal("invalid reference changed local data")
	}
}

func TestIdentitySaveRollsBackCredentialsOnStorageFailure(t *testing.T) {
	a := syncTestApp(t, "http://127.0.0.1")
	identity, err := a.SaveIdentity(Identity{Name: "Deploy", Username: "deploy", AuthMethod: "password"}, "original", "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.configDir, "groups.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	identity.Username = "changed"
	if _, err := a.SaveIdentity(identity, "changed-password", ""); err == nil {
		t.Fatal("storage failure ignored")
	}
	if value, _ := a.passwords.Get(identity.ID); value != "original" || a.ListIdentities()[0].Username != "deploy" {
		t.Fatal("failed identity save changed credentials or metadata")
	}
}

func TestKeyIdentitySyncRemembersPassphraseForAllItsHosts(t *testing.T) {
	cloud := &vaultServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { cloud.serve(t, w, r) }))
	defer server.Close()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(private)
	port, fingerprint := vaultSSHServer(t, signer.PublicKey())
	block, err := ssh.MarshalPrivateKeyWithPassphrase(private, "identity test", []byte("key-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "identity.key")
	key := pem.EncodeToMemory(block)
	if err := os.WriteFile(path, key, 0600); err != nil {
		t.Fatal(err)
	}
	a := syncTestApp(t, server.URL)
	identity, err := a.SaveIdentity(Identity{Name: "Deploy key", Username: "deploy", AuthMethod: "key", KeyPath: path}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	host, err := a.SaveHost(Host{Address: "127.0.0.1", Port: port, IdentityID: identity.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := a.ConnectHost(host.ID, "", "", fingerprint); err != nil || result.Status != "passphrase_required" {
		t.Fatalf("missing identity passphrase prompt: %+v, %v", result, err)
	}
	result, err := a.ConnectHost(host.ID, "", "key-passphrase", fingerprint)
	if err != nil || result.Status != "connected" {
		t.Fatalf("key identity could not connect: %+v, %v", result, err)
	}
	a.DisconnectSession(result.SessionID)
	if value, _ := a.passwords.Get(passphraseID(identity.ID)); value != "key-passphrase" {
		t.Fatal("passphrase was not stored against the identity")
	}
	if _, err := a.ConfigureVaultSync("vault-password", ""); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	b := syncTestApp(t, server.URL)
	if _, err := b.ConfigureVaultSync("vault-password", ""); err != nil {
		t.Fatal(err)
	}
	if len(b.ListIdentities()) != 1 {
		t.Fatal("key identity was not restored")
	}
	restored := b.ListIdentities()[0]
	contents, err := os.ReadFile(restored.KeyPath)
	if err != nil || !bytes.Equal(key, contents) {
		t.Fatal("identity private key was not restored")
	}
	result, err = b.ConnectHost(host.ID, "", "", "")
	if err != nil || result.Status != "connected" {
		t.Fatalf("restored identity needs credentials again: %+v, %v", result, err)
	}
	b.DisconnectSession(result.SessionID)
	if err := b.DeleteIdentity(identity.ID); err != nil {
		t.Fatal(err)
	}
	if value, _ := b.passwords.Get(passphraseID(host.ID)); value != "key-passphrase" {
		t.Fatal("deleting the key identity lost its host's passphrase")
	}
	if _, err := b.passwords.Get(passphraseID(identity.ID)); err == nil {
		t.Fatal("deleted key identity left its saved passphrase")
	}
	result, err = b.ConnectHost(host.ID, "", "", "")
	if err != nil || result.Status != "connected" {
		t.Fatalf("host stopped connecting after key identity deletion: %+v, %v", result, err)
	}
	b.DisconnectSession(result.SessionID)
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	// Restore the original device's key so it has no unrelated local edit
	if err := os.WriteFile(path, key, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if len(a.ListIdentities()) != 0 || a.ListHosts()[0].IdentityID != "" {
		t.Fatal("identity deletion did not sync to the original device")
	}
	result, err = a.ConnectHost(host.ID, "", "", "")
	if err != nil || result.Status != "connected" {
		t.Fatalf("synced identity deletion lost host credentials: %+v, %v", result, err)
	}
	a.DisconnectSession(result.SessionID)
}

func TestVaultExampleMatchesCurrentFormat(t *testing.T) {
	blob, err := os.ReadFile("vault.example.json")
	if err != nil {
		t.Fatal(err)
	}
	var data vaultData
	if err := json.Unmarshal(blob, &data); err != nil {
		t.Fatal(err)
	}
	if err := validateVaultData(data); err != nil {
		t.Fatalf("documented JSON is invalid: %v", err)
	}
}

func TestGroupWithoutIdentityUsesProvidedPassword(t *testing.T) {
	public, _, _ := ed25519.GenerateKey(rand.Reader)
	sshPublic, _ := ssh.NewPublicKey(public)
	port, fingerprint := vaultSSHServer(t, sshPublic)
	a := syncTestApp(t, "http://127.0.0.1")
	group, err := a.CreateGroup("No default yet")
	if err != nil {
		t.Fatal(err)
	}
	host, err := a.SaveHost(Host{Address: "127.0.0.1", Port: port, Group: group.ID, UseGroupIdentity: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.ConnectHost(host.ID, "", "", fingerprint); err == nil {
		t.Fatal("missing password did not require input")
	}
	result, err := a.ConnectHost(host.ID, "restored-ssh-password", "", fingerprint)
	if err != nil || result.Status != "connected" {
		t.Fatalf("provided fallback password failed: %+v, %v", result, err)
	}
	a.DisconnectSession(result.SessionID)
	if err := a.DeleteGroup(group.ID); err != nil {
		t.Fatal(err)
	}
	blob, err := a.vaultSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encodeVault(blob, newVaultKey("vault-password")); err != nil {
		t.Fatalf("deleting an empty-default group left invalid host credentials: %v", err)
	}
}

func TestGroupDefaultIsAssignedToNewHostsAndIdentityCanBeDeleted(t *testing.T) {
	a := syncTestApp(t, "http://127.0.0.1")
	identity, err := a.SaveIdentity(Identity{Name: "Deploy", Username: "deploy", AuthMethod: "password"}, "identity-password", "")
	if err != nil {
		t.Fatal(err)
	}
	group, err := a.SaveGroup(Group{Name: "Production", DefaultIdentityID: identity.ID})
	if err != nil {
		t.Fatal(err)
	}
	inherited, err := a.SaveHost(Host{Address: "inherited.example", Port: 22, Group: group.ID}, "")
	if err != nil || !inherited.UseGroupIdentity || inherited.Username != "deploy" {
		t.Fatalf("new host did not inherit the group identity: %+v, %v", inherited, err)
	}
	direct, err := a.SaveHost(Host{Address: "direct.example", Port: 22, IdentityID: identity.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	separate, err := a.SaveHost(Host{Address: "separate.example", Port: 22, Group: group.ID, AuthMethod: "password", Username: "other"}, "separate-password")
	if err != nil || separate.UseGroupIdentity {
		t.Fatal("group default replaced explicit credentials")
	}
	if err := a.DeleteIdentity(identity.ID); err != nil {
		t.Fatal(err)
	}
	if len(a.ListIdentities()) != 0 || a.ListGroups()[0].DefaultIdentityID != "" {
		t.Fatal("identity or group default was not removed")
	}
	for _, host := range a.ListHosts() {
		if host.ID == inherited.ID || host.ID == direct.ID {
			password, _ := a.passwords.Get(host.ID)
			if host.IdentityID != "" || host.UseGroupIdentity || host.Username != "deploy" || !host.SavedPassword || password != "identity-password" {
				t.Fatal("deleting the identity lost host credentials")
			}
		}
	}
	if _, err := a.passwords.Get(identity.ID); err == nil {
		t.Fatal("deleted identity left its saved password")
	}
	if password, _ := a.passwords.Get(separate.ID); password != "separate-password" {
		t.Fatal("unrelated credentials changed")
	}
	data, err := a.vaultSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := encodeVault(data, newVaultKey("vault-password")); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewApp(a.configDir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded.passwords = a.passwords
	if len(reloaded.ListHosts()) != 3 || len(reloaded.ListIdentities()) != 0 {
		t.Fatal("identity deletion did not persist")
	}
}

func TestIdentityDeletionRollsBackOnStorageFailure(t *testing.T) {
	a := syncTestApp(t, "http://127.0.0.1")
	identity, err := a.SaveIdentity(Identity{Name: "Deploy", Username: "deploy", AuthMethod: "password"}, "identity-password", "")
	if err != nil {
		t.Fatal(err)
	}
	host, err := a.SaveHost(Host{Address: "example.test", Port: 22, IdentityID: identity.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(a.configDir, "groups.json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteIdentity(identity.ID); err == nil {
		t.Fatal("storage failure ignored")
	}
	if len(a.ListIdentities()) != 1 || a.ListHosts()[0].IdentityID != identity.ID {
		t.Fatal("failed deletion changed metadata")
	}
	if value, _ := a.passwords.Get(identity.ID); value != "identity-password" {
		t.Fatal("failed deletion removed the identity password")
	}
	if _, err := a.passwords.Get(host.ID); err == nil {
		t.Fatal("failed deletion left a copied password")
	}
}
