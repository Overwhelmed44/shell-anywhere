//go:build linux || darwin

package main

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestUnixLocalShellSession(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	output := make(chan terminalEvent, 64)
	closed := make(chan sessionEvent, 1)
	app.eventSink = func(name string, data any) {
		switch name {
		case "terminal:output":
			output <- data.(terminalEvent)
		case "terminal:closed":
			closed <- data.(sessionEvent)
		}
	}
	options := app.GetLocalShells()
	if len(options) != 1 || options[0].ID != "shell" {
		t.Fatalf("unexpected Unix shells: %+v", options)
	}
	if _, err := app.StartLocalShell("powershell"); err == nil {
		t.Fatal("Windows shell accepted on Unix")
	}
	id, err := app.StartLocalShell("shell")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.DisconnectSession(id) })
	if err := app.ResizeSession(id, 24, 100); err != nil {
		t.Fatal(err)
	}
	if err := app.WriteSession(id, "stty size; printf '%s%s\\n' 'unix_' 'shell_ready'\r"); err != nil {
		t.Fatal(err)
	}
	deadline := time.After(8 * time.Second)
	var text strings.Builder
	for !strings.Contains(text.String(), "unix_shell_ready") || !strings.Contains(text.String(), "24 100") {
		select {
		case event := <-output:
			contents, err := base64.StdEncoding.DecodeString(event.Data)
			if err != nil {
				t.Fatal(err)
			}
			text.Write(contents)
		case <-deadline:
			t.Fatalf("shell command or PTY resize failed: %q", text.String())
		}
	}
	app.DisconnectSession(id)
	select {
	case event := <-closed:
		if event.SessionID != id {
			t.Fatal("wrong local session closed")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("closing the terminal did not stop the shell")
	}
}

func TestUnixTerminalWaitCancellation(t *testing.T) {
	t.Setenv("SHELL", "/bin/sh")
	terminal, err := startLocalPTY("shell")
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := terminal.Wait(ctx); err != context.Canceled {
		t.Fatalf("wait ignored cancellation: %v", err)
	}
}
