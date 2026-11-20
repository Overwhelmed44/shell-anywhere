package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSettingsPasswordsResolveCredentialSource(t *testing.T) {
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.passwords = &memoryPasswordStore{values: map[string]string{
		"own": " own password ", "identity": "shared password",
	}}
	app.hosts = []Host{
		{ID: "own", AuthMethod: "password", SavedPassword: true},
		{ID: "direct", IdentityID: "identity"},
		{ID: "inherited", Group: "group", UseGroupIdentity: true},
		{ID: "key", AuthMethod: "key"},
		{ID: "missing", AuthMethod: "password", SavedPassword: true},
	}
	app.groups = []Group{{ID: "group", DefaultIdentityID: "identity"}}
	app.identities = []Identity{{ID: "identity", AuthMethod: "password", SavedPassword: true}}
	for id, want := range map[string]string{
		"own": " own password ", "direct": "shared password", "inherited": "shared password", "key": "",
	} {
		got, err := app.GetHostPassword(id)
		if err != nil || got != want {
			t.Errorf("settings password source for %s is incorrect: %v", id, err)
		}
	}
	if got, err := app.GetIdentityPassword("identity"); err != nil || got != "shared password" {
		t.Errorf("identity settings password is incorrect: %v", err)
	}
	if _, err := app.GetHostPassword("missing"); !errors.Is(err, keyring.ErrNotFound) {
		t.Errorf("credential store errors must be returned: %v", err)
	}
	if _, err := app.GetHostPassword("unknown"); err == nil {
		t.Error("unknown host must be rejected")
	}
	if _, err := app.GetIdentityPassword("unknown"); err == nil {
		t.Error("unknown identity must be rejected")
	}
	data, err := json.Marshal(app.GetWorkspace())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "own password") || strings.Contains(string(data), "shared password") {
		t.Error("workspace metadata must not include password values")
	}
	app.hosts = nil
	app.identities = nil
	if _, err := app.GetHostPassword("own"); err == nil {
		t.Error("removed host must no longer expose its password")
	}
	if _, err := app.GetIdentityPassword("identity"); err == nil {
		t.Error("removed identity must no longer expose its password")
	}
}
