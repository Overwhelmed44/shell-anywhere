package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// The caller holds a.mu to serialize changes with syncing and other connections.
// Replacement requires the explicit "replace:" confirmation for the presented fingerprint.
func persistTrustedHostKey(path, address string, remote net.Addr, key ssh.PublicKey, replace bool) error {
	verify, err := knownhosts.New(path)
	if err != nil {
		return err
	}
	err = verify(address, remote, key)
	if err == nil {
		return nil
	}
	var mismatch *knownhosts.KeyError
	if !errors.As(err, &mismatch) || (len(mismatch.Want) != 0 && !replace) {
		return fmt.Errorf("host key changed while connecting; connect again to verify it: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	remove := map[int]bool{}
	for _, old := range mismatch.Want {
		remove[old.Line] = true
	}
	normalized := knownhosts.Normalize(address)
	lines := strings.SplitAfter(string(data), "\n")
	var output strings.Builder
	for index, line := range lines {
		if remove[index+1] {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			hosts := fields[0]
			if strings.HasPrefix(hosts, "@") && len(fields) > 1 {
				hosts = fields[1]
			}
			if hosts == normalized || strings.HasPrefix(hosts, "|") {
				continue
			}
			// Shared aliases and wildcard entries keep their old key for other hosts.
			line = strings.Replace(line, hosts, "!"+normalized+","+hosts, 1)
		}
		output.WriteString(line)
	}
	if output.Len() > 0 && !strings.HasSuffix(output.String(), "\n") {
		output.WriteByte('\n')
	}
	output.WriteString(knownhosts.Line([]string{normalized}, key) + "\n")
	file, err := os.CreateTemp(filepath.Dir(path), ".known-hosts-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.WriteString(output.String()); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
