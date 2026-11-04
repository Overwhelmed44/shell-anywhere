package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zalando/go-keyring"
)

type Group struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	DefaultIdentityID string `json:"defaultIdentityId"`
}

type Identity struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Username      string `json:"username"`
	AuthMethod    string `json:"authMethod"`
	KeyPath       string `json:"keyPath"`
	SavedPassword bool   `json:"savedPassword"`
}

type Workspace struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type WorkspaceView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Hosts      []Host     `json:"hosts"`
	Groups     []Group    `json:"groups"`
	Identities []Identity `json:"identities"`
}

type localWorkspace struct {
	Hosts      []Host
	Groups     []Group
	Identities []Identity
}

func emptyWorkspace() localWorkspace {
	return localWorkspace{Hosts: []Host{}, Groups: []Group{}, Identities: []Identity{}}
}

func (a *App) workspaceDir(id string) string {
	if id == "personal" {
		return a.configDir
	}
	return filepath.Join(a.configDir, "workspaces", id)
}

func (a *App) readWorkspace(id string) (localWorkspace, error) {
	data := emptyWorkspace()
	for file, target := range map[string]any{
		"hosts.json": &data.Hosts, "groups.json": &data.Groups, "identities.json": &data.Identities,
	} {
		blob, err := os.ReadFile(filepath.Join(a.workspaceDir(id), file))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return data, err
		}
		if err := json.Unmarshal(blob, target); err != nil {
			return data, fmt.Errorf("read %s: %w", file, err)
		}
	}
	if data.Hosts == nil {
		data.Hosts = []Host{}
	}
	if data.Groups == nil {
		data.Groups = []Group{}
	}
	if data.Identities == nil {
		data.Identities = []Identity{}
	}
	return data, nil
}

func (a *App) workspaceLocked(id string) (localWorkspace, error) {
	if id == a.workspaceID {
		return localWorkspace{Hosts: a.hosts, Groups: a.groups, Identities: a.identities}, nil
	}
	if id != "personal" && a.workspaces[id] == "" {
		return localWorkspace{}, errors.New("workspace not found")
	}
	return a.readWorkspace(id)
}

func (a *App) writeWorkspace(id string, data localWorkspace, restore *vaultRestore) error {
	if err := os.MkdirAll(a.workspaceDir(id), 0700); err != nil {
		return err
	}
	for file, value := range map[string]any{
		"hosts.json": data.Hosts, "groups.json": data.Groups, "identities.json": data.Identities,
	} {
		blob, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		if err := restore.file(filepath.Join(a.workspaceDir(id), file), blob); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) commitWorkspace(data localWorkspace, restore *vaultRestore) error {
	if err := a.writeWorkspace(a.workspaceID, data, restore); err != nil {
		return errors.Join(err, restore.rollback())
	}
	a.hosts, a.groups, a.identities = data.Hosts, data.Groups, data.Identities
	a.signalVaultSync()
	return nil
}

func (a *App) ListWorkspaces() []Workspace {
	a.mu.RLock()
	defer a.mu.RUnlock()
	items := []Workspace{{ID: "personal", Name: "Personal"}}
	for id, name := range a.workspaces {
		items = append(items, Workspace{ID: id, Name: name})
	}
	sort.Slice(items[1:], func(i, j int) bool { return items[i+1].Name < items[j+1].Name })
	return items
}

func (a *App) SelectWorkspace(id string) (WorkspaceView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	data, err := a.workspaceLocked(id)
	if err != nil {
		return WorkspaceView{}, err
	}
	a.workspaceID = id
	a.hosts, a.groups, a.identities = data.Hosts, data.Groups, data.Identities
	return a.workspaceViewLocked(), nil
}

func (a *App) GetWorkspace() WorkspaceView {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workspaceViewLocked()
}

func (a *App) workspaceViewLocked() WorkspaceView {
	name := "Personal"
	if a.workspaceID != "personal" {
		name = a.workspaces[a.workspaceID]
	}
	return WorkspaceView{ID: a.workspaceID, Name: name, Hosts: a.listHostsLocked(), Groups: append([]Group{}, a.groups...), Identities: append([]Identity{}, a.identities...)}
}

func identityForHost(host Host, groups []Group, identities []Identity) (Identity, bool) {
	id := host.IdentityID
	if host.UseGroupIdentity {
		for _, group := range groups {
			if group.ID == host.Group {
				id = group.DefaultIdentityID
				break
			}
		}
	}
	for _, identity := range identities {
		if identity.ID == id {
			return identity, true
		}
	}
	return Identity{}, false
}

func resolvedHost(host Host, workspaceID string, data localWorkspace) Host {
	host.WorkspaceID = workspaceID
	if identity, ok := identityForHost(host, data.Groups, data.Identities); ok {
		host.Username, host.AuthMethod, host.KeyPath, host.SavedPassword = identity.Username, identity.AuthMethod, identity.KeyPath, identity.SavedPassword
		host.credentialID = identity.ID
	} else {
		host.credentialID = host.ID
		if host.Username == "" {
			host.Username = "root"
		}
		if host.AuthMethod == "" {
			host.AuthMethod = "password"
		}
	}
	return host
}

func (a *App) listHostsLocked() []Host {
	items := []Host{}
	data := localWorkspace{Groups: a.groups, Identities: a.identities}
	for _, host := range a.hosts {
		items = append(items, resolvedHost(host, a.workspaceID, data))
	}
	return items
}

func (a *App) ListGroups() []Group {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]Group{}, a.groups...)
}

func (a *App) ListIdentities() []Identity {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]Identity{}, a.identities...)
}

func (a *App) rememberIdentityPassword(workspaceID string, host Host, password string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	data, err := a.workspaceLocked(workspaceID)
	if err != nil {
		return err
	}
	data.Identities = append([]Identity{}, data.Identities...)
	found := false
	for i := range data.Identities {
		if data.Identities[i].ID == host.credentialID && data.Identities[i].AuthMethod == "password" && data.Identities[i].Username == host.Username {
			data.Identities[i].SavedPassword = true
			found = true
		}
	}
	if !found {
		return errors.New("identity changed while connecting")
	}
	restore := &vaultRestore{a: a}
	if err := restore.secret(host.credentialID, &password); err != nil {
		return errors.Join(err, restore.rollback())
	}
	if err := a.writeWorkspace(workspaceID, data, restore); err != nil {
		return errors.Join(err, restore.rollback())
	}
	if a.workspaceID == workspaceID {
		a.identities = data.Identities
	}
	a.signalVaultSync()
	return nil
}

func (a *App) CreateGroup(name string) (Group, error) {
	return a.SaveGroup(Group{Name: name})
}

func (a *App) SaveGroup(group Group) (Group, error) {
	group.Name = strings.TrimSpace(group.Name)
	if group.Name == "" {
		return Group{}, errors.New("enter a group name")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.EqualFold(group.Name, "All hosts") || strings.EqualFold(group.Name, "Favorites") {
		return Group{}, errors.New("this name is reserved")
	}
	if group.DefaultIdentityID != "" && !hasIdentity(a.identities, group.DefaultIdentityID) {
		return Group{}, errors.New("identity not found")
	}
	groups := append([]Group{}, a.groups...)
	found := group.ID == ""
	for i, old := range groups {
		if old.ID != group.ID && strings.EqualFold(old.Name, group.Name) {
			return Group{}, errors.New("group already exists")
		}
		if old.ID == group.ID {
			groups[i] = group
			found = true
		}
	}
	if !found {
		return Group{}, errors.New("group not found")
	}
	if group.ID == "" {
		group.ID = newID()
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Name < groups[j].Name })
	if err := a.commitWorkspace(localWorkspace{a.hosts, groups, a.identities}, &vaultRestore{a: a}); err != nil {
		return Group{}, err
	}
	return group, nil
}

func (a *App) DeleteGroup(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	groups := []Group{}
	found := false
	for _, group := range a.groups {
		if group.ID == id {
			found = true
		} else {
			groups = append(groups, group)
		}
	}
	if !found {
		return errors.New("group not found")
	}
	hosts := append([]Host{}, a.hosts...)
	for i := range hosts {
		if hosts[i].Group == id {
			// Keep the same identity when a host loses its group
			if hosts[i].UseGroupIdentity {
				if identity, ok := identityForHost(hosts[i], a.groups, a.identities); ok {
					hosts[i].IdentityID = identity.ID
				}
				hosts[i].UseGroupIdentity = false
				if hosts[i].IdentityID == "" {
					hosts[i].Username = "root"
					hosts[i].AuthMethod = "password"
				}
			}
			hosts[i].Group = ""
		}
	}
	return a.commitWorkspace(localWorkspace{hosts, groups, a.identities}, &vaultRestore{a: a})
}

func hasIdentity(identities []Identity, id string) bool {
	for _, identity := range identities {
		if identity.ID == id {
			return true
		}
	}
	return false
}

func (a *App) SaveIdentity(identity Identity, password, passphrase string) (Identity, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	identity.Name, identity.Username, identity.KeyPath = strings.TrimSpace(identity.Name), strings.TrimSpace(identity.Username), strings.TrimSpace(identity.KeyPath)
	if identity.Name == "" || identity.Username == "" {
		return Identity{}, errors.New("enter an identity name and username")
	}
	if identity.AuthMethod != "password" && identity.AuthMethod != "key" {
		return Identity{}, errors.New("choose password or SSH key authentication")
	}
	if identity.AuthMethod == "key" && identity.KeyPath == "" {
		return Identity{}, errors.New("choose a private key file")
	}
	items := append([]Identity{}, a.identities...)
	var old Identity
	for _, item := range items {
		if item.ID == identity.ID {
			old = item
		}
	}
	if identity.ID != "" && old.ID == "" {
		return Identity{}, errors.New("identity not found")
	}
	if identity.ID == "" {
		identity.ID = newID()
	}
	identity.SavedPassword = identity.AuthMethod == "password" && (password != "" || old.SavedPassword)
	if identity.AuthMethod == "password" && !identity.SavedPassword {
		return Identity{}, errors.New("enter the identity password")
	}
	if identity.AuthMethod == "password" {
		identity.KeyPath = ""
	}
	restore := &vaultRestore{a: a}
	fail := func(err error) (Identity, error) { return Identity{}, errors.Join(err, restore.rollback()) }
	if password != "" && identity.AuthMethod == "password" {
		if err := restore.secret(identity.ID, &password); err != nil {
			return fail(err)
		}
	} else if old.SavedPassword && !identity.SavedPassword {
		if err := restore.secret(identity.ID, nil); err != nil {
			return fail(err)
		}
	}
	if old.KeyPath != identity.KeyPath || identity.AuthMethod != "key" {
		if err := restore.secret(passphraseID(identity.ID), nil); err != nil {
			return fail(err)
		}
	}
	if passphrase != "" && identity.AuthMethod == "key" {
		if err := restore.secret(passphraseID(identity.ID), &passphrase); err != nil {
			return fail(err)
		}
	}
	if old.ID == "" {
		items = append(items, identity)
	} else {
		for i := range items {
			if items[i].ID == identity.ID {
				items[i] = identity
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	if err := a.commitWorkspace(localWorkspace{a.hosts, a.groups, items}, restore); err != nil {
		return Identity{}, err
	}
	return identity, nil
}

func (a *App) DeleteIdentity(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	items := []Identity{}
	var removed Identity
	for _, item := range a.identities {
		if item.ID != id {
			items = append(items, item)
		} else {
			removed = item
		}
	}
	if len(items) == len(a.identities) {
		return errors.New("identity not found")
	}
	restore := &vaultRestore{a: a}
	fail := func(err error) error { return errors.Join(err, restore.rollback()) }
	hosts := append([]Host{}, a.hosts...)
	for i, host := range hosts {
		identity, used := identityForHost(host, a.groups, a.identities)
		if !used || identity.ID != id {
			continue
		}
		host.IdentityID, host.UseGroupIdentity = "", false
		host.Username, host.AuthMethod, host.KeyPath, host.SavedPassword = removed.Username, removed.AuthMethod, removed.KeyPath, removed.SavedPassword
		if removed.SavedPassword {
			password, err := a.passwords.Get(id)
			if err != nil {
				return fail(err)
			}
			if err := restore.secret(host.ID, &password); err != nil {
				return fail(err)
			}
		}
		if removed.AuthMethod == "key" {
			passphrase, err := a.passwords.Get(passphraseID(id))
			if err == nil {
				if err := restore.secret(passphraseID(host.ID), &passphrase); err != nil {
					return fail(err)
				}
			} else if !errors.Is(err, keyring.ErrNotFound) {
				return fail(err)
			}
		}
		hosts[i] = host
	}
	groups := append([]Group{}, a.groups...)
	for i := range groups {
		if groups[i].DefaultIdentityID == id {
			groups[i].DefaultIdentityID = ""
		}
	}
	if err := restore.secret(id, nil); err != nil {
		return fail(err)
	}
	if err := restore.secret(passphraseID(id), nil); err != nil {
		return fail(err)
	}
	return a.commitWorkspace(localWorkspace{hosts, groups, items}, restore)
}
