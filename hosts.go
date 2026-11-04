package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func (a *App) ListHosts() []Host {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.listHostsLocked()
}

func (a *App) SaveHost(host Host, password string) (Host, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if host.WorkspaceID != "" && host.WorkspaceID != a.workspaceID {
		return Host{}, errors.New("switch to this host's workspace before editing it")
	}
	host.WorkspaceID = ""
	host.Name, host.Address, host.Username = strings.TrimSpace(host.Name), strings.TrimSpace(host.Address), strings.TrimSpace(host.Username)
	host.KeyPath = strings.TrimSpace(host.KeyPath)
	if host.Address == "" {
		return Host{}, errors.New("hostname or IP is required")
	}
	if host.Name == "" {
		host.Name = host.Address
	}
	if host.Port < 1 || host.Port > 65535 {
		return Host{}, errors.New("port must be between 1 and 65535")
	}
	if host.Group != "" {
		found := false
		for _, group := range a.groups {
			if group.ID == host.Group {
				found = true
				if host.ID == "" && host.IdentityID == "" && !host.UseGroupIdentity && password == "" && host.KeyPath == "" && group.DefaultIdentityID != "" {
					host.UseGroupIdentity = true
				}
			}
		}
		if !found {
			return Host{}, errors.New("choose an existing group")
		}
	}
	if host.IdentityID != "" && !hasIdentity(a.identities, host.IdentityID) {
		return Host{}, errors.New("identity not found")
	}
	if host.UseGroupIdentity && host.Group == "" {
		return Host{}, errors.New("choose a group to use its default identity")
	}
	if host.UseGroupIdentity && host.IdentityID != "" {
		return Host{}, errors.New("choose an identity or the group default")
	}
	hosts := append([]Host{}, a.hosts...)
	var old Host
	for _, item := range hosts {
		if item.ID == host.ID {
			old = item
		}
	}
	if host.ID != "" && old.ID == "" {
		return Host{}, errors.New("host not found")
	}
	if host.ID == "" {
		host.ID = newID()
	}
	if host.IdentityID != "" || host.UseGroupIdentity {
		host.Username, host.AuthMethod, host.KeyPath = "", "", ""
		host.SavedPassword = false
		password = ""
	} else {
		if host.Username == "" {
			host.Username = "root"
		}
		if host.AuthMethod != "password" && host.AuthMethod != "key" {
			return Host{}, errors.New("choose password or SSH key authentication")
		}
		if host.AuthMethod == "key" && host.KeyPath == "" {
			return Host{}, errors.New("choose a private key file")
		}
		host.SavedPassword = host.AuthMethod == "password" && (password != "" || old.SavedPassword)
		if host.AuthMethod == "password" {
			host.KeyPath = ""
			if old.ID == "" && password == "" {
				return Host{}, errors.New("enter a password to save this host")
			}
		}
	}
	if old.ID == "" {
		hosts = append(hosts, host)
	} else {
		for i := range hosts {
			if hosts[i].ID == host.ID {
				hosts[i] = host
			}
		}
	}
	restore := &vaultRestore{a: a}
	fail := func(err error) (Host, error) { return Host{}, errors.Join(err, restore.rollback()) }
	if host.SavedPassword && password != "" {
		if err := restore.secret(host.ID, &password); err != nil {
			return fail(err)
		}
	} else if old.SavedPassword && !host.SavedPassword {
		if err := restore.secret(host.ID, nil); err != nil {
			return fail(err)
		}
	}
	if old.AuthMethod == "key" && (host.AuthMethod != "key" || old.KeyPath != host.KeyPath) {
		if err := restore.secret(passphraseID(host.ID), nil); err != nil {
			return fail(err)
		}
	}
	if err := a.commitWorkspace(localWorkspace{hosts, a.groups, a.identities}, restore); err != nil {
		return Host{}, err
	}
	return resolvedHost(host, a.workspaceID, localWorkspace{Groups: a.groups, Identities: a.identities}), nil
}

func (a *App) DeleteHost(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	hosts := []Host{}
	found := false
	for _, host := range a.hosts {
		if host.ID == id {
			found = true
		} else {
			hosts = append(hosts, host)
		}
	}
	if !found {
		return errors.New("host not found")
	}
	restore := &vaultRestore{a: a}
	if err := restore.secret(id, nil); err != nil {
		return errors.Join(err, restore.rollback())
	}
	if err := restore.secret(passphraseID(id), nil); err != nil {
		return errors.Join(err, restore.rollback())
	}
	return a.commitWorkspace(localWorkspace{hosts, a.groups, a.identities}, restore)
}

func (a *App) persistHosts() error {
	data, err := json.MarshalIndent(a.hosts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(a.workspaceDir(a.workspaceID), "hosts.json"), data, 0600)
}

func (a *App) persistGroups() error {
	data, err := json.MarshalIndent(a.groups, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(a.workspaceDir(a.workspaceID), "groups.json"), data, 0600)
}
