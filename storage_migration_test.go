package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestCredentialsMigrateWithoutLosingOrRevivingSecrets(t *testing.T) {
	for _, test := range []struct {
		name, service, legacy, id string
		store                     passwordStore
	}{
		{"account", accountService, legacyAccountService, "https://oxee.net", systemAccountStore{}},
		{"SSH password", passwordService, legacyPasswordService, "host-id", systemPasswordStore{}},
		{"key passphrase", passwordService, legacyPasswordService, passphraseID("host-id"), systemPasswordStore{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			keyring.MockInit()
			if err := keyring.Set(test.legacy, test.id, "saved-secret"); err != nil {
				t.Fatal(err)
			}
			if value, err := test.store.Get(test.id); err != nil || value != "saved-secret" {
				t.Fatalf("existing credential was lost: %v", err)
			}
			if _, err := keyring.Get(test.service, test.id); err != nil {
				t.Fatal("credential was not copied into the new service")
			}
			if _, err := keyring.Get(test.legacy, test.id); !errors.Is(err, keyring.ErrNotFound) {
				t.Fatal("migrated credential remains in the old service")
			}

			if err := keyring.Set(test.legacy, test.id, "outdated-secret"); err != nil {
				t.Fatal(err)
			}
			if value, err := test.store.Get(test.id); err != nil || value != "saved-secret" {
				t.Fatal("old credential replaced the current credential")
			}
			if err := test.store.Set(test.id, "rotated-secret"); err != nil {
				t.Fatal(err)
			}
			if value, err := keyring.Get(test.service, test.id); err != nil || value != "rotated-secret" {
				t.Fatal("rotated credential was not saved in the new service")
			}
			if _, err := keyring.Get(test.legacy, test.id); !errors.Is(err, keyring.ErrNotFound) {
				t.Fatal("rotating a credential left its old value behind")
			}

			for _, legacyOnly := range []bool{false, true} {
				if err := keyring.Set(test.legacy, test.id, "outdated-secret"); err != nil {
					t.Fatal(err)
				}
				if legacyOnly {
					_ = keyring.Delete(test.service, test.id)
				}
				if err := test.store.Delete(test.id); err != nil {
					t.Fatal(err)
				}
				if _, err := test.store.Get(test.id); !errors.Is(err, keyring.ErrNotFound) {
					t.Fatal("deleted credential was restored from the old service")
				}
			}
		})
	}
}

func TestCredentialStoreErrorsArePreserved(t *testing.T) {
	want := errors.New("credential store is locked")
	keyring.MockInitWithError(want)
	t.Cleanup(keyring.MockInit)
	store := systemAccountStore{}
	if _, err := store.Get("account"); !errors.Is(err, want) {
		t.Fatal("credential lookup hid a storage error")
	}
	if err := store.Set("account", "value"); !errors.Is(err, want) {
		t.Fatal("credential write hid a storage error")
	}
	if err := store.Delete("account"); !errors.Is(err, want) {
		t.Fatal("credential deletion hid a storage error")
	}
}

func TestConfigDirectoryMigrationPreservesHostsAndPrivateKeys(t *testing.T) {
	base := t.TempDir()
	legacy := filepath.Join(base, legacyConfigDirectory)
	keyPath := filepath.Join(legacy, "keys", "restored.key")
	if err := os.MkdirAll(filepath.Dir(keyPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("private-key"), 0600); err != nil {
		t.Fatal(err)
	}
	workspaceID := "1234567890abcdef12345678"
	for file, value := range map[string]any{
		"hosts.json":      []Host{{ID: "host", AuthMethod: "key", KeyPath: keyPath}},
		"identities.json": []Identity{{ID: "identity", AuthMethod: "key", KeyPath: keyPath}},
		"workspaces.json": map[string]string{workspaceID: "Team"},
		filepath.Join("workspaces", workspaceID, "hosts.json"): []Host{{ID: "team-host", KeyPath: keyPath}},
	} {
		path := filepath.Join(legacy, file)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		blob, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, blob, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"known_hosts", "vault.json", "hosts-before-sync.json"} {
		if err := os.WriteFile(filepath.Join(legacy, file), []byte(file), 0600); err != nil {
			t.Fatal(err)
		}
	}
	dir, err := defaultConfigDir(base)
	if err != nil || dir != filepath.Join(base, configDirectory) {
		t.Fatalf("config directory migration failed: %v", err)
	}
	for _, file := range []string{"known_hosts", "vault.json", "hosts-before-sync.json", filepath.Join("keys", "restored.key")} {
		if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
			t.Fatalf("migration lost %s: %v", file, err)
		}
	}
	wantKey := filepath.Join(dir, "keys", "restored.key")
	for range 2 {
		app, err := NewApp(dir)
		if err != nil {
			t.Fatal(err)
		}
		if app.ListHosts()[0].KeyPath != wantKey || app.ListIdentities()[0].KeyPath != wantKey {
			t.Fatal("migration broke a restored key path")
		}
		team, err := app.SelectWorkspace(workspaceID)
		if err != nil || len(team.Hosts) != 1 || team.Hosts[0].KeyPath != wantKey {
			t.Fatalf("migration broke another workspace: %v", err)
		}
		for _, path := range []string{filepath.Join(base, "external.key"), filepath.Join(base, legacyConfigDirectory+"-other", "keys", "restored.key")} {
			if app.migratedKeyPath(path) != path {
				t.Fatal("migration changed an external key path")
			}
		}
	}
}

func TestConfigDirectoryPrefersCurrentSettings(t *testing.T) {
	base := t.TempDir()
	want := filepath.Join(base, configDirectory)
	if dir, err := defaultConfigDir(base); err != nil || dir != want {
		t.Fatalf("new installation uses the wrong directory: %v", err)
	}
	for _, name := range []string{configDirectory, legacyConfigDirectory} {
		if err := os.MkdirAll(filepath.Join(base, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if dir, err := defaultConfigDir(base); err != nil || dir != want {
		t.Fatalf("existing settings were replaced: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, legacyConfigDirectory)); err != nil {
		t.Fatal("migration overwrote an existing configuration")
	}
}
