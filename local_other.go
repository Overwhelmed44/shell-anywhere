//go:build !windows && !linux && !darwin

package main

import "errors"

func startLocalPTY(string) (localTerminal, error) {
	return nil, errors.New("local terminals are not available on this platform")
}

func localShells() []LocalShellOption { return []LocalShellOption{} }
