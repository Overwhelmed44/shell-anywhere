package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/ssh"
)

func dockerExecCommand(id, shell string) (string, error) {
	if !dockerContainerID.MatchString(id) {
		return "", errors.New("Invalid Docker container ID")
	}
	if shell != "sh" && shell != "bash" {
		return "", errors.New("Choose sh or bash for the container shell")
	}
	return "env LC_ALL=C docker container exec -it -e TERM=xterm-256color " + id + " " + shell, nil
}

// An exec terminal owns only its SSH channel; closing it must never close the
// parent transport. Register it as a child so normal terminal input/resize work.
func (a *App) StartDockerExec(parentID, containerID, shell string) (string, error) {
	command, err := dockerExecCommand(containerID, shell)
	if err != nil {
		return "", err
	}
	a.mu.RLock()
	parent := a.sessions[parentID]
	a.mu.RUnlock()
	if parent == nil || parent.client == nil || parent.parentID != "" {
		return "", errors.New("SSH session is closed")
	}
	if !parent.execMu.TryLock() {
		return "", errors.New("A container shell is already starting")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	type response struct {
		id  string
		err error
	}
	result := make(chan response)
	go func() {
		id, err := a.startDockerExec(ctx, parentID, parent, command)
		parent.execMu.Unlock()
		select {
		case result <- response{id, err}:
		case <-ctx.Done():
			if id != "" {
				a.DisconnectSession(id)
			}
		}
	}()
	select {
	case reply := <-result:
		if ctx.Err() != nil {
			if reply.id != "" {
				a.DisconnectSession(reply.id)
			}
			return "", errors.New("Starting the container shell timed out")
		}
		return reply.id, reply.err
	case <-ctx.Done():
		return "", errors.New("Starting the container shell timed out")
	}
}

func (a *App) startDockerExec(ctx context.Context, parentID string, parent *activeSession, command string) (string, error) {
	channel, err := parent.client.NewSession()
	if err != nil {
		return "", err
	}
	started := false
	defer func() {
		if !started {
			_ = channel.Close()
		}
	}()
	setupDone := make(chan struct{})
	defer close(setupDone)
	go func() {
		select {
		case <-ctx.Done():
			select {
			case <-setupDone:
				return
			default:
				_ = channel.Close()
			}
		case <-setupDone:
		}
	}()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	stdin, err := channel.StdinPipe()
	if err != nil {
		return "", err
	}
	id := newID()
	writer := &terminalWriter{app: a, sessionID: id}
	channel.Stdout, channel.Stderr = writer, writer
	if err := channel.RequestPty("xterm-256color", 24, 100, ssh.TerminalModes{}); err != nil {
		return "", fmt.Errorf("Request container terminal: %w", err)
	}
	child := &activeSession{client: parent.client, ssh: channel, stdin: stdin, parentID: parentID}
	a.mu.Lock()
	if a.sessions[parentID] != parent || ctx.Err() != nil {
		a.mu.Unlock()
		return "", errors.New("SSH connection closed while starting the container shell")
	}
	a.sessions[id] = child
	a.mu.Unlock()
	if err := channel.Start(command); err != nil {
		a.DisconnectSession(id)
		return "", fmt.Errorf("Start container shell: %w", err)
	}
	started = true
	go func() {
		err := channel.Wait()
		_ = channel.Close()
		a.mu.Lock()
		emitClosed := a.sessions[id] == child
		if emitClosed {
			delete(a.sessions, id)
		}
		a.mu.Unlock()
		if emitClosed {
			reason := "Container shell exited"
			if err != nil {
				reason = fmt.Sprintf("Container shell exited: %v", err)
			}
			a.emit("terminal:closed", sessionEvent{SessionID: id, Reason: reason})
		}
	}()
	return id, nil
}
