package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/crypto/ssh"
)

// AppSettings sync independently of the encrypted connection vault.
type AppSettings struct {
	Theme                    string `json:"theme,omitempty"`
	ConnectionTimeoutSeconds int    `json:"connectionTimeoutSeconds"`
	TerminalFontSize         int    `json:"terminalFontSize"`
	TerminalScrollback       int    `json:"terminalScrollback"`
	TerminalCursorBlink      bool   `json:"terminalCursorBlink"`
}

func defaultAppSettings() AppSettings {
	return AppSettings{Theme: "dark", ConnectionTimeoutSeconds: 12, TerminalFontSize: 13, TerminalScrollback: 5000, TerminalCursorBlink: true}
}

// Persist and sync only custom values; missing keys inherit this app's defaults.
func (s AppSettings) overrides() map[string]any {
	defaults := defaultAppSettings()
	custom := make(map[string]any)
	if s.Theme != defaults.Theme {
		custom["theme"] = s.Theme
	}
	if s.ConnectionTimeoutSeconds != defaults.ConnectionTimeoutSeconds {
		custom["connectionTimeoutSeconds"] = s.ConnectionTimeoutSeconds
	}
	if s.TerminalFontSize != defaults.TerminalFontSize {
		custom["terminalFontSize"] = s.TerminalFontSize
	}
	if s.TerminalScrollback != defaults.TerminalScrollback {
		custom["terminalScrollback"] = s.TerminalScrollback
	}
	if s.TerminalCursorBlink != defaults.TerminalCursorBlink {
		custom["terminalCursorBlink"] = s.TerminalCursorBlink
	}
	return custom
}

func (s AppSettings) validate() error {
	switch s.Theme {
	case "dark", "light", "midnight", "forest", "rose", "arctic", "sand", "paper":
	default:
		return errors.New("choose a supported theme")
	}
	if s.ConnectionTimeoutSeconds < 1 || s.ConnectionTimeoutSeconds > 300 {
		return errors.New("connection timeout must be between 1 and 300 seconds")
	}
	if s.TerminalFontSize < 10 || s.TerminalFontSize > 24 {
		return errors.New("terminal font size must be between 10 and 24")
	}
	if s.TerminalScrollback < 100 || s.TerminalScrollback > 100000 {
		return errors.New("scrollback must be between 100 and 100000 lines")
	}
	return nil
}

func (a *App) loadSettings() error {
	settings := defaultAppSettings()
	data, err := os.ReadFile(filepath.Join(a.configDir, "settings.json"))
	if errors.Is(err, os.ErrNotExist) {
		a.settings = settings
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	if err := settings.validate(); err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	a.settings = settings
	return nil
}

func (a *App) GetSettings() AppSettings {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.settings
}

func (a *App) SaveSettings(settings AppSettings) (AppSettings, error) {
	if err := settings.validate(); err != nil {
		return AppSettings{}, err
	}
	a.mu.Lock()
	err := a.saveSettingsLocked(settings)
	a.mu.Unlock()
	if err != nil {
		return AppSettings{}, err
	}
	a.emitSettingsChanged(settings)
	a.signalVaultSync()
	return settings, nil
}

// Caller holds a.mu so a cloud download cannot replace a simultaneous local save.
func (a *App) saveSettingsLocked(settings AppSettings) error {
	data, err := json.MarshalIndent(settings.overrides(), "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(a.configDir, "settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), filepath.Join(a.configDir, "settings.json")); err != nil {
		return err
	}
	a.settings = settings
	return nil
}

func (a *App) syncAppSettings(ctx context.Context, checked syncAction, snapshot AppSettings) (string, string, error) {
	data, err := json.Marshal(snapshot.overrides())
	if err != nil {
		return "", "", err
	}
	localDigest := vaultDigest(data)
	dirty := a.vault.settings.LocalSettingsDigest != "" && localDigest != a.vault.settings.LocalSettingsDigest
	if dirty || checked.Action == "init" {
		err = a.vaultRequest(ctx, http.MethodPut, syncPath, syncUpload{
			Settings: &syncEntity{Body: base64.StdEncoding.EncodeToString(data)},
			Groups:   map[string]syncEntity{},
		}, nil)
		return localDigest, localDigest, err
	}
	if checked.Status == "synced" {
		return a.vault.settings.SettingsDigest, localDigest, nil
	}
	cloud, err := base64.StdEncoding.DecodeString(checked.Action)
	if err != nil {
		return "", "", errors.New("sync server returned invalid settings")
	}
	settings := defaultAppSettings()
	if err := json.Unmarshal(cloud, &settings); err != nil {
		return "", "", fmt.Errorf("read cloud settings: %w", err)
	}
	if err := settings.validate(); err != nil {
		return "", "", fmt.Errorf("read cloud settings: %w", err)
	}
	a.mu.Lock()
	if a.settings != snapshot {
		a.mu.Unlock()
		a.signalVaultSync()
		// Keep the snapshot as baseline so a save during download remains dirty.
		return a.vault.settings.SettingsDigest, localDigest, nil
	}
	err = a.saveSettingsLocked(settings)
	a.mu.Unlock()
	if err != nil {
		return "", "", err
	}
	a.emitSettingsChanged(settings)
	canonical, _ := json.Marshal(settings.overrides())
	return vaultDigest(cloud), vaultDigest(canonical), nil
}

func (a *App) emitSettingsChanged(settings AppSettings) {
	if a.ctx != nil || a.eventSink != nil {
		a.emit("settings:changed", settings)
	}
}

// Bound both the TCP connection and SSH handshake, then remove the deadline so
// an established terminal or file transfer can stay open without an idle limit.
func dialSSH(address string, config *ssh.ClientConfig) (*ssh.Client, error) {
	deadline := time.Now().Add(config.Timeout)
	connection, err := (&net.Dialer{Deadline: deadline}).Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	return newSSHClient(connection, address, config, deadline)
}

func newSSHClient(connection net.Conn, address string, config *ssh.ClientConfig, deadline time.Time) (*ssh.Client, error) {
	if err := connection.SetDeadline(deadline); err != nil {
		_ = connection.Close()
		return nil, err
	}
	client, channels, requests, err := ssh.NewClientConn(connection, address, config)
	if err != nil {
		_ = connection.Close()
		return nil, err
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		_ = client.Close()
		return nil, err
	}
	return ssh.NewClient(client, channels, requests), nil
}
