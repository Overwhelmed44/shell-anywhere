//go:build linux || darwin

package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

type unixTerminal struct {
	*os.File
	command *exec.Cmd
	done    chan struct{}
	code    uint32
	err     error
	once    sync.Once
}

func defaultLocalShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		if resolved, err := exec.LookPath(shell); err == nil {
			return resolved
		}
	}
	fallback := "/bin/bash"
	if runtime.GOOS == "darwin" {
		fallback = "/bin/zsh"
	}
	if resolved, err := exec.LookPath(fallback); err == nil {
		return resolved
	}
	return "/bin/sh"
}

func localShells() []LocalShellOption {
	name := filepath.Base(defaultLocalShell())
	switch name {
	case "bash":
		name = "Bash"
	case "zsh":
		name = "Zsh"
	case "fish":
		name = "Fish"
	}
	return []LocalShellOption{{ID: "shell", Name: name}}
}

func startLocalPTY(string) (localTerminal, error) {
	command := exec.Command(defaultLocalShell(), "-l")
	command.Dir, _ = os.UserHomeDir()
	command.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	file, err := pty.StartWithSize(command, &pty.Winsize{Cols: 110, Rows: 32})
	if err != nil {
		return nil, err
	}
	terminal := &unixTerminal{File: file, command: command, done: make(chan struct{})}
	go func() {
		terminal.err = command.Wait()
		if command.ProcessState != nil && command.ProcessState.ExitCode() >= 0 {
			terminal.code = uint32(command.ProcessState.ExitCode())
		}
		close(terminal.done)
	}()
	return terminal, nil
}

func (t *unixTerminal) Resize(columns, rows int) error {
	if columns < 1 || columns > 65535 || rows < 1 || rows > 65535 {
		return errors.New("invalid terminal size")
	}
	return pty.Setsize(t.File, &pty.Winsize{Cols: uint16(columns), Rows: uint16(rows)})
}

func (t *unixTerminal) Wait(ctx context.Context) (uint32, error) {
	select {
	case <-t.done:
		return t.code, t.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (t *unixTerminal) Close() error {
	var err error
	t.once.Do(func() {
		select {
		case <-t.done:
		default:
			_ = syscall.Kill(-t.command.Process.Pid, syscall.SIGHUP)
			_ = t.command.Process.Kill()
		}
		err = t.File.Close()
	})
	return err
}
