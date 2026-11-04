package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/ssh"
)

func passphraseID(id string) string { return "key-passphrase:" + id }

func privateKeyPath(path string) (string, error) {
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, path[2:]), nil
	}
	return path, nil
}

func (a *App) snapshotCredentials(host Host, target *vaultHost) error {
	if host.AuthMethod == "password" && host.SavedPassword {
		password, err := a.passwords.Get(host.ID)
		if err != nil {
			return fmt.Errorf("read saved password for %s: %w", host.Name, err)
		}
		target.Password = &password
	}
	if host.AuthMethod == "key" {
		path, err := privateKeyPath(host.KeyPath)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read private key for %s: %w", host.Name, err)
		}
		if len(data) == 0 || len(data) > 1<<20 {
			return errors.New("private key must be between 1 byte and 1 MB")
		}
		target.PrivateKey = data
		passphrase, err := a.passwords.Get(passphraseID(host.ID))
		if err == nil {
			target.Passphrase = &passphrase
		} else if !errors.Is(err, keyring.ErrNotFound) {
			return err
		}
	}
	return nil
}

// A failed restore rolls back credential-store and file writes together
type vaultRestore struct {
	a    *App
	undo []func() error
}

func (r *vaultRestore) rollback() error {
	var err error
	for i := len(r.undo) - 1; i >= 0; i-- {
		err = errors.Join(err, r.undo[i]())
	}
	return err
}

func (r *vaultRestore) file(path string, data []byte) error {
	previous, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	r.undo = append(r.undo, func() error {
		if exists {
			return os.WriteFile(path, previous, 0600)
		}
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	})
	return os.WriteFile(path, data, 0600)
}

func (r *vaultRestore) secret(id string, value *string) error {
	previous, err := r.a.passwords.Get(id)
	exists := err == nil
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	if value == nil && !exists {
		return nil
	}
	r.undo = append(r.undo, func() error {
		if exists {
			return r.a.passwords.Set(id, previous)
		}
		err := r.a.passwords.Delete(id)
		if errors.Is(err, keyring.ErrNotFound) {
			return nil
		}
		return err
	})
	if value == nil {
		return r.a.passwords.Delete(id)
	}
	return r.a.passwords.Set(id, *value)
}

func (r *vaultRestore) credentials(host vaultHost, target *Host) error {
	if err := r.secret(host.ID, host.Password); err != nil {
		return err
	}
	if err := r.secret(passphraseID(host.ID), host.Passphrase); err != nil {
		return err
	}
	target.SavedPassword = host.Password != nil && *host.Password != ""
	if host.AuthMethod == "key" && len(host.PrivateKey) > 0 {
		dir := filepath.Join(r.a.configDir, "keys")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		digest := sha256.Sum256(host.PrivateKey)
		target.KeyPath = filepath.Join(dir, hex.EncodeToString(digest[:])+".key")
		return r.file(target.KeyPath, host.PrivateKey)
	}
	return nil
}

func validateVaultCredentials(host vaultHost) error {
	if host.AuthMethod == "password" {
		if host.Password != nil && *host.Password == "" {
			return errors.New("invalid empty cloud password")
		}
		if len(host.PrivateKey) != 0 || host.Passphrase != nil {
			return errors.New("invalid cloud password credentials")
		}
		return nil
	}
	if host.Password != nil || len(host.PrivateKey) == 0 || len(host.PrivateKey) > 1<<20 {
		return errors.New("invalid cloud private key")
	}
	var err error
	if host.Passphrase != nil && *host.Passphrase != "" {
		_, err = ssh.ParsePrivateKeyWithPassphrase(host.PrivateKey, []byte(*host.Passphrase))
	} else {
		_, err = ssh.ParsePrivateKey(host.PrivateKey)
		var missing *ssh.PassphraseMissingError
		if errors.As(err, &missing) {
			return nil
		}
	}
	if err != nil {
		return errors.New("invalid cloud private key or passphrase")
	}
	return nil
}
