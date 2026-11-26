package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAppSettingsSyncRoundTripAndRestart(t *testing.T) {
	cloud := &vaultServer{}
	a := cloud.testApp(t)
	settings := a.GetSettings()
	settings.Theme = "forest"
	settings.TerminalFontSize = 17
	if _, err := a.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := a.ConfigureVaultSync("", ""); err != nil {
		t.Fatal(err)
	}
	uploaded := defaultAppSettings()
	if err := json.Unmarshal(cloud.settings, &uploaded); err != nil || uploaded != settings {
		t.Fatalf("settings were not uploaded: %+v, %v", uploaded, err)
	}
	var overrides map[string]any
	if err := json.Unmarshal(cloud.settings, &overrides); err != nil || len(overrides) != 2 || overrides["theme"] != "forest" || overrides["terminalFontSize"] != float64(17) {
		t.Fatalf("cloud contains defaults instead of only overrides: %s", cloud.settings)
	}
	b := cloud.testApp(t)
	events := 0
	b.eventSink = func(name string, data any) {
		if name == "settings:changed" {
			events++
		}
	}
	if _, err := b.ConfigureVaultSync("", ""); err != nil {
		t.Fatal(err)
	}
	if b.GetSettings() != settings || events != 1 {
		t.Fatal("cloud settings did not apply or notify frontend")
	}
	uploads := cloud.settingsUploads
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if cloud.settingsUploads != uploads {
		t.Fatal("unchanged settings were uploaded again")
	}

	settings.Theme = "paper"
	settings.TerminalCursorBlink = false
	if _, err := a.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if b.GetSettings() != settings {
		t.Fatal("settings changes did not reach second device")
	}
	restarted, err := NewApp(b.configDir)
	if err != nil || restarted.GetSettings() != settings {
		t.Fatalf("cloud settings did not survive restart: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.configDir, "vault.json")); err != nil {
		t.Fatal(err)
	}

	cloud.fail = true
	settings.Theme = "rose"
	if _, err := a.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err == nil {
		t.Fatal("unavailable server accepted sync")
	}
	if a.GetSettings() != settings {
		t.Fatal("failed sync lost offline changes")
	}
	cloud.fail = false
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if b.GetSettings() != settings {
		t.Fatal("offline changes were not retried")
	}
}

func TestSettingsSaveDuringDownloadIsNotOverwritten(t *testing.T) {
	cloud := &vaultServer{}
	a := cloud.testApp(t)
	if _, err := a.ConfigureVaultSync("", ""); err != nil {
		t.Fatal(err)
	}
	cloudSettings := a.GetSettings()
	cloudSettings.Theme = "light"
	cloud.settings, _ = json.Marshal(cloudSettings.overrides())
	local := a.GetSettings()
	local.Theme = "midnight"
	saved := false
	a.vault.client = nil
	a.accountHTTP = &http.Client{Transport: vaultTransport(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		cloud.serve(t, w, r)
		if r.Method == http.MethodPost && r.URL.Path == "/api/v1/sync" && !saved {
			saved = true
			if _, err := a.SaveSettings(local); err != nil {
				t.Fatal(err)
			}
		}
		return w.Result(), nil
	})}
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if a.GetSettings() != local {
		t.Fatal("cloud download overwrote concurrent local save")
	}
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	uploaded := defaultAppSettings()
	_ = json.Unmarshal(cloud.settings, &uploaded)
	if uploaded != local {
		t.Fatal("concurrent save was not uploaded on retry")
	}
}

func TestSettingsSyncEmptyOverridesAndReset(t *testing.T) {
	cloud := &vaultServer{}
	a := cloud.testApp(t)
	if _, err := a.ConfigureVaultSync("", ""); err != nil {
		t.Fatal(err)
	}
	if string(cloud.settings) != "{}" {
		t.Fatalf("default settings were expanded in cloud: %s", cloud.settings)
	}
	custom := a.GetSettings()
	custom.Theme = "forest"
	custom.TerminalCursorBlink = false
	if _, err := a.SaveSettings(custom); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	b := cloud.testApp(t)
	if _, err := b.ConfigureVaultSync("", ""); err != nil {
		t.Fatal(err)
	}
	if b.GetSettings() != custom {
		t.Fatal("partial cloud settings did not apply over defaults")
	}
	if _, err := a.SaveSettings(defaultAppSettings()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if string(cloud.settings) != "{}" {
		t.Fatalf("reset did not clear cloud overrides: %s", cloud.settings)
	}
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if b.GetSettings() != defaultAppSettings() {
		t.Fatal("cleared overrides did not reset the other device")
	}
	data, err := os.ReadFile(filepath.Join(b.configDir, "settings.json"))
	if err != nil || string(data) != "{}" {
		t.Fatalf("download wrote effective defaults: %s, %v", data, err)
	}
	uploads := cloud.settingsUploads
	if _, err := b.SyncVault(); err != nil {
		t.Fatal(err)
	}
	if cloud.settingsUploads != uploads {
		t.Fatal("unchanged defaults caused a redundant upload")
	}
	if b.vault.settings.LocalSettingsDigest != vaultDigest([]byte("{}")) {
		t.Fatal("digest includes expanded defaults")
	}
}
