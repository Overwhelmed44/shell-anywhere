package main

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSettingsOnlyPersistOverridesAndReset(t *testing.T) {
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(app.configDir, "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("loading defaults unexpectedly wrote settings")
	}
	settings := app.GetSettings()
	settings.Theme = "forest"
	settings.TerminalCursorBlink = false
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(app.configDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored, map[string]any{"theme": "forest", "terminalCursorBlink": false}) {
		t.Fatalf("persisted defaults or lost false override: %s", data)
	}
	reloaded, err := NewApp(app.configDir)
	if err != nil || reloaded.GetSettings() != settings {
		t.Fatalf("overrides were not applied over defaults: %v", err)
	}
	if _, err := app.SaveSettings(defaultAppSettings()); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(app.configDir, "settings.json"))
	if err != nil || string(data) != "{}" {
		t.Fatalf("reset did not clear overrides: %s, %v", data, err)
	}
	reloaded, err = NewApp(app.configDir)
	if err != nil || reloaded.GetSettings() != defaultAppSettings() {
		t.Fatalf("empty overrides did not restore defaults: %v", err)
	}
}

func TestSettingsLoadOverridesOverDefaults(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "settings.json"), []byte(`{"terminalCursorBlink":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	app, err := NewApp(directory)
	if err != nil {
		t.Fatal(err)
	}
	expected := defaultAppSettings()
	expected.TerminalCursorBlink = false
	if app.GetSettings() != expected {
		t.Fatalf("missing fields did not inherit defaults: %+v", app.GetSettings())
	}
}

func TestSettingsPersistAndValidate(t *testing.T) {
	directory := t.TempDir()
	app, err := NewApp(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := app.GetSettings(); got != defaultAppSettings() {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	settings := AppSettings{Theme: "light", ConnectionTimeoutSeconds: 45, TerminalFontSize: 16, TerminalScrollback: 12000, TerminalCursorBlink: false}
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewApp(directory)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.GetSettings(); got != settings {
		t.Fatalf("settings did not survive restart: %+v", got)
	}
	// A second save must replace the file, including on Windows.
	settings.ConnectionTimeoutSeconds = 60
	if _, err := app.SaveSettings(settings); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"theme", "timeout", "font", "scrollback"} {
		t.Run(field, func(t *testing.T) {
			invalid := settings
			switch field {
			case "theme":
				invalid.Theme = "unknown"
			case "timeout":
				invalid.ConnectionTimeoutSeconds = 0
			case "font":
				invalid.TerminalFontSize = 25
			case "scrollback":
				invalid.TerminalScrollback = 100001
			}
			if _, err := app.SaveSettings(invalid); err == nil {
				t.Fatal("invalid settings accepted")
			}
			if got := app.GetSettings(); got != settings {
				t.Fatalf("failed save changed active settings: %+v", got)
			}
		})
	}
	reloaded, err = NewApp(directory)
	if err != nil || reloaded.GetSettings() != settings {
		t.Fatalf("failed save changed persisted settings: %v", err)
	}
}

func TestThemePreferencesSurviveRestart(t *testing.T) {
	for _, theme := range []string{"dark", "light", "midnight", "forest", "rose", "arctic", "sand", "paper"} {
		t.Run(theme, func(t *testing.T) {
			directory := t.TempDir()
			app, err := NewApp(directory)
			if err != nil {
				t.Fatal(err)
			}
			settings := app.GetSettings()
			settings.Theme = theme
			if _, err := app.SaveSettings(settings); err != nil {
				t.Fatal(err)
			}
			restarted, err := NewApp(directory)
			if err != nil {
				t.Fatal(err)
			}
			if restarted.GetSettings() != settings {
				t.Fatal("theme or other preferences changed after restarting")
			}
		})
	}
}

func TestSettingsWriteFailureKeepsCurrentPreferences(t *testing.T) {
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(app.configDir, "settings.json"), 0700); err != nil {
		t.Fatal(err)
	}
	next := app.GetSettings()
	next.ConnectionTimeoutSeconds = 60
	if _, err := app.SaveSettings(next); err == nil {
		t.Fatal("save unexpectedly replaced a directory")
	}
	if app.GetSettings() != defaultAppSettings() {
		t.Fatal("failed write changed active preferences")
	}
}

func TestSSHTimeoutIncludesHandshake(t *testing.T) {
	connection, server := net.Pipe()
	defer server.Close()
	finished := make(chan error, 1)
	go func() {
		_, err := newSSHClient(connection, "server:22", &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.InsecureIgnoreHostKey()}, time.Now().Add(100*time.Millisecond))
		finished <- err
	}()
	select {
	case err := <-finished:
		var timeout net.Error
		if !errors.As(err, &timeout) || !timeout.Timeout() {
			t.Fatalf("stalled handshake did not time out: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SSH handshake exceeded the connection timeout")
	}
}
