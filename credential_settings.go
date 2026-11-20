package main

import (
	"errors"
	"fmt"
)

func (a *App) GetHostPassword(id string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, host := range a.hosts {
		if host.ID == id {
			host = resolvedHost(host, a.workspaceID, localWorkspace{Groups: a.groups, Identities: a.identities})
			return a.settingsPassword(host.credentialID, host.AuthMethod, host.SavedPassword)
		}
	}
	return "", errors.New("host not found")
}

func (a *App) GetIdentityPassword(id string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, identity := range a.identities {
		if identity.ID == id {
			return a.settingsPassword(identity.ID, identity.AuthMethod, identity.SavedPassword)
		}
	}
	return "", errors.New("identity not found")
}

func (a *App) settingsPassword(id, authMethod string, saved bool) (string, error) {
	if authMethod != "password" || !saved {
		return "", nil
	}
	password, err := a.passwords.Get(id)
	if err != nil {
		return "", fmt.Errorf("read saved password: %w", err)
	}
	return password, nil
}
