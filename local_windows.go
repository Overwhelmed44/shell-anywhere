//go:build windows

package main

import (
	"os"

	"github.com/UserExistsError/conpty"
)

func localShells() []LocalShellOption {
	return []LocalShellOption{{ID: "powershell", Name: "PowerShell"}, {ID: "cmd", Name: "Command Prompt"}}
}

func startLocalPTY(shell string) (localTerminal, error) {
	command := "cmd.exe"
	if shell == "powershell" {
		command = "powershell.exe -NoLogo"
	}
	home, _ := os.UserHomeDir()
	return conpty.Start(command, conpty.ConPtyDimensions(110, 32), conpty.ConPtyWorkDir(home))
}
