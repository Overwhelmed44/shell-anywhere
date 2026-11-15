package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

// Old names are used only to migrate existing installations
const (
	configDirectory       = "shell-anywhere"
	legacyConfigDirectory = "shell-everywhere"
	legacyAccountService  = "shell-everywhere-oxee"
	legacyPasswordService = "shell-everywhere-ssh"
)

func setCredential(service, legacyService, id, value string) error {
	if err := keyring.Set(service, id, value); err != nil {
		return err
	}
	err := keyring.Delete(legacyService, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

func getCredential(service, legacyService, id string) (string, error) {
	value, err := keyring.Get(service, id)
	if !errors.Is(err, keyring.ErrNotFound) {
		return value, err
	}
	value, err = keyring.Get(legacyService, id)
	if err != nil {
		return "", err
	}
	if err := setCredential(service, legacyService, id, value); err != nil {
		return "", err
	}
	return value, nil
}

func deleteCredential(service, legacyService, id string) error {
	err := keyring.Delete(service, id)
	legacyErr := keyring.Delete(legacyService, id)
	if errors.Is(err, keyring.ErrNotFound) && errors.Is(legacyErr, keyring.ErrNotFound) {
		return keyring.ErrNotFound
	}
	if errors.Is(err, keyring.ErrNotFound) {
		err = nil
	}
	if errors.Is(legacyErr, keyring.ErrNotFound) {
		legacyErr = nil
	}
	return errors.Join(err, legacyErr)
}

func defaultConfigDir(base string) (string, error) {
	dir := filepath.Join(base, configDirectory)
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		return dir, err
	}
	legacy := filepath.Join(base, legacyConfigDirectory)
	if _, err := os.Stat(legacy); errors.Is(err, os.ErrNotExist) {
		return dir, nil
	} else if err != nil {
		return "", err
	}
	if err := os.Rename(legacy, dir); err != nil {
		return "", fmt.Errorf("migrate Shell Anywhere settings: %w", err)
	}
	return dir, nil
}

// Restored keys have absolute paths; resolve them after the config directory moves
func (a *App) migratedKeyPath(path string) string {
	if path == "" || filepath.Base(a.configDir) != configDirectory {
		return path
	}
	legacyKeys := filepath.Join(filepath.Dir(a.configDir), legacyConfigDirectory, "keys")
	relative, err := filepath.Rel(legacyKeys, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		return path
	}
	migrated := filepath.Join(a.configDir, "keys", relative)
	if _, err := os.Stat(migrated); err == nil {
		return migrated
	}
	return path
}
