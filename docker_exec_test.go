package main

import (
	"bufio"
	"encoding/base64"
	"io"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestDockerExecValidation(t *testing.T) {
	for _, value := range []struct{ id, shell string }{
		{"bad;id", "sh"}, {dockerTestID, "sh; reboot"}, {dockerTestID, "/bin/sh"}, {dockerTestID, ""},
	} {
		if _, err := dockerExecCommand(value.id, value.shell); err == nil {
			t.Fatalf("accepted invalid exec arguments: %+v", value)
		}
	}
	command, err := dockerExecCommand(dockerTestID, "bash")
	if err != nil || command != "env LC_ALL=C docker container exec -it -e TERM=xterm-256color "+dockerTestID+" bash" {
		t.Fatalf("exec command: %q, %v", command, err)
	}
	app := &App{}
	if _, err := app.StartDockerExec("closed", dockerTestID, "sh"); err == nil {
		t.Fatal("exec without a parent connection was accepted")
	}
}

func TestDockerExecLifecycleKeepsParentTerminal(t *testing.T) {
	outputs := make(chan terminalEvent, 20)
	closed := make(chan sessionEvent, 20)
	commands := make(chan string, 5)
	app, terminal := dockerSSHApp(t, func(command string, channel ssh.Channel) uint32 {
		commands <- command
		_, _ = io.WriteString(channel, "container-ready\n")
		reader := bufio.NewReader(channel)
		for {
			line, err := reader.ReadString('\n')
			if err != nil || line == "exit\n" {
				return 0
			}
			_, _ = io.WriteString(channel, line)
		}
	}, true)
	app.eventSink = func(name string, data any) {
		switch name {
		case "terminal:output":
			outputs <- data.(terminalEvent)
		case "terminal:closed":
			closed <- data.(sessionEvent)
		}
	}
	readOutput := func(id, want string) {
		t.Helper()
		select {
		case event := <-outputs:
			data, err := base64.StdEncoding.DecodeString(event.Data)
			if err != nil || event.SessionID != id || string(data) != want {
				t.Fatalf("exec output: %+v, %q, %v", event, data, err)
			}
		case <-time.After(time.Second):
			t.Fatal("container terminal output timed out")
		}
	}
	start := func() string {
		t.Helper()
		id, err := app.StartDockerExec("ssh", dockerTestID, "sh")
		if err != nil || id == "" || id == "ssh" {
			t.Fatalf("start exec: %q, %v", id, err)
		}
		if command := <-commands; !strings.HasSuffix(command, dockerTestID+" sh") {
			t.Fatalf("unexpected command: %s", command)
		}
		readOutput(id, "container-ready\n")
		return id
	}
	assertParent := func() {
		t.Helper()
		if err := app.WriteSession("ssh", "parent-is-live\n"); err != nil {
			t.Fatal(err)
		}
		if line, err := terminal.ReadString('\n'); err != nil || line != "parent-is-live\n" {
			t.Fatalf("exec interrupted the parent: %q, %v", line, err)
		}
	}
	id := start()
	if err := app.ResizeSession(id, 30, 120); err != nil {
		t.Fatal(err)
	}
	if err := app.WriteSession(id, "container-input\n"); err != nil {
		t.Fatal(err)
	}
	readOutput(id, "container-input\n")
	app.DisconnectSession(id)
	assertParent()

	id = start()
	if err := app.WriteSession(id, "exit\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-closed:
		if event.SessionID != id || event.Reason != "Container shell exited" {
			t.Fatalf("exit event: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("container exit was not reported")
	}
	assertParent()

	id = start()
	app.DisconnectSession("ssh")
	select {
	case event := <-closed:
		if event.SessionID != id {
			t.Fatalf("parent closure did not close its child: %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("closing the parent leaked its exec terminal")
	}
	app.mu.RLock()
	remaining := len(app.sessions)
	app.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("%d SSH sessions leaked", remaining)
	}
}

func TestDockerExecPTYFailureKeepsParent(t *testing.T) {
	app, terminal := dockerSSHApp(t, func(_ string, _ ssh.Channel) uint32 { return 0 })
	if _, err := app.StartDockerExec("ssh", dockerTestID, "sh"); err == nil || !strings.Contains(err.Error(), "Request container terminal") {
		t.Fatalf("expected a PTY error: %v", err)
	}
	if err := app.WriteSession("ssh", "still-live\n"); err != nil {
		t.Fatal(err)
	}
	if line, err := terminal.ReadString('\n'); err != nil || line != "still-live\n" {
		t.Fatalf("PTY failure closed the parent: %q, %v", line, err)
	}
}
