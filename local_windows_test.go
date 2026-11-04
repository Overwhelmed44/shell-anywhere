//go:build windows

package main

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestLocalShellSessions(t *testing.T) {
	for _, shell := range []string{"cmd", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			app, err := NewApp(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			output := make(chan terminalEvent, 64)
			app.eventSink = func(name string, data any) {
				if name == "terminal:output" {
					output <- data.(terminalEvent)
				}
			}
			if _, err := app.StartLocalShell("unknown"); err == nil {
				t.Fatal("unsupported local shell was accepted")
			}
			id, err := app.StartLocalShell(shell)
			if err != nil {
				t.Fatal(err)
			}
			defer app.DisconnectSession(id)
			if err := app.ResizeSession(id, 24, 100); err != nil {
				t.Fatal(err)
			}
			marker := "shell_everywhere_local_test"
			if err := app.WriteSession(id, "echo "+marker+"\r"); err != nil {
				t.Fatal(err)
			}
			deadline := time.After(8 * time.Second)
			var text strings.Builder
			for !strings.Contains(text.String(), marker) {
				select {
				case event := <-output:
					if event.SessionID != id {
						continue
					}
					data, err := base64.StdEncoding.DecodeString(event.Data)
					if err != nil {
						t.Fatal(err)
					}
					text.Write(data)
				case <-deadline:
					t.Fatalf("local terminal produced no command output: %q", text.String())
				}
			}
		})
	}
}
