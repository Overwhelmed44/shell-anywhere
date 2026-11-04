package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/crypto/ssh/knownhosts"
)

type vaultGroup struct {
	Name              string `json:"name"`
	DefaultIdentityID string `json:"defaultIdentityId"`
}

type vaultIdentity struct {
	Name       string  `json:"name"`
	Username   string  `json:"username"`
	AuthMethod string  `json:"authMethod"`
	Password   *string `json:"password,omitempty"`
	PrivateKey []byte  `json:"privateKey,omitempty"`
	Passphrase *string `json:"passphrase,omitempty"`
}

type vaultWorkspace struct {
	Name       string                   `json:"name,omitempty"`
	Hosts      map[string]vaultHost     `json:"hosts"`
	Groups     map[string]vaultGroup    `json:"groups"`
	Identities map[string]vaultIdentity `json:"identities"`
	KnownHosts string                   `json:"knownHosts"`
}

type vaultWorkspaces struct {
	Personal vaultWorkspace            `json:"personal"`
	Others   map[string]vaultWorkspace `json:"others"`
}

func validRecordID(id string) bool {
	if len(id) != 24 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (a *App) snapshotWorkspace(id string) (vaultWorkspace, error) {
	local, err := a.workspaceLocked(id)
	if err != nil {
		return vaultWorkspace{}, err
	}
	known, err := os.ReadFile(filepath.Join(a.workspaceDir(id), "known_hosts"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return vaultWorkspace{}, err
	}
	data := vaultWorkspace{
		Hosts: map[string]vaultHost{}, Groups: map[string]vaultGroup{},
		Identities: map[string]vaultIdentity{}, KnownHosts: string(known),
	}
	if id != "personal" {
		data.Name = a.workspaces[id]
	}
	for _, group := range local.Groups {
		data.Groups[group.ID] = vaultGroup{Name: group.Name, DefaultIdentityID: group.DefaultIdentityID}
	}
	for _, host := range local.Hosts {
		item := vaultHost{ID: host.ID, Name: host.Name, Address: host.Address, Port: host.Port, Group: host.Group, Favorite: host.Favorite, IdentityID: host.IdentityID, UseGroupIdentity: host.UseGroupIdentity}
		if host.IdentityID == "" && !host.UseGroupIdentity {
			item.Username, item.AuthMethod = host.Username, host.AuthMethod
			if err := a.snapshotCredentials(host, &item); err != nil {
				return data, err
			}
		}
		data.Hosts[host.ID] = item
	}
	for _, identity := range local.Identities {
		item := vaultHost{ID: identity.ID, Name: identity.Name, Username: identity.Username, AuthMethod: identity.AuthMethod}
		host := Host{ID: identity.ID, Name: identity.Name, AuthMethod: identity.AuthMethod, KeyPath: identity.KeyPath, SavedPassword: identity.SavedPassword}
		if err := a.snapshotCredentials(host, &item); err != nil {
			return data, err
		}
		data.Identities[identity.ID] = vaultIdentity{Name: identity.Name, Username: identity.Username, AuthMethod: identity.AuthMethod, Password: item.Password, PrivateKey: item.PrivateKey, Passphrase: item.Passphrase}
	}
	return data, nil
}

func (a *App) vaultSnapshot() ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.vaultSnapshotLocked()
}

func (a *App) vaultSnapshotLocked() ([]byte, error) {
	personal, err := a.snapshotWorkspace("personal")
	if err != nil {
		return nil, err
	}
	data := vaultData{Version: 1, Workspaces: vaultWorkspaces{Personal: personal, Others: map[string]vaultWorkspace{}}}
	for id := range a.workspaces {
		workspace, err := a.snapshotWorkspace(id)
		if err != nil {
			return nil, err
		}
		data.Workspaces.Others[id] = workspace
	}
	return json.Marshal(data)
}

func validateVaultData(data vaultData) error {
	if data.Version != 1 || data.Workspaces.Others == nil {
		return errors.New("unsupported cloud vault format")
	}
	ids := map[string]bool{}
	validate := func(workspace vaultWorkspace) error {
		if workspace.Hosts == nil || workspace.Groups == nil || workspace.Identities == nil {
			return errors.New("invalid workspace data")
		}
		claim := func(id string) bool {
			if !validRecordID(id) || ids[id] {
				return false
			}
			ids[id] = true
			return true
		}
		for id, identity := range workspace.Identities {
			if !claim(id) || strings.TrimSpace(identity.Name) == "" || strings.TrimSpace(identity.Username) == "" || (identity.AuthMethod != "password" && identity.AuthMethod != "key") {
				return errors.New("invalid cloud identity")
			}
			if err := validateVaultCredentials(vaultHost{AuthMethod: identity.AuthMethod, Password: identity.Password, PrivateKey: identity.PrivateKey, Passphrase: identity.Passphrase}); err != nil {
				return err
			}
		}
		names := map[string]bool{}
		for id, group := range workspace.Groups {
			name := strings.ToLower(strings.TrimSpace(group.Name))
			if !validRecordID(id) || name == "" || name == "all hosts" || name == "favorites" || names[name] {
				return errors.New("invalid cloud group")
			}
			names[name] = true
			if group.DefaultIdentityID != "" {
				if _, ok := workspace.Identities[group.DefaultIdentityID]; !ok {
					return errors.New("group identity not found")
				}
			}
		}
		for id, host := range workspace.Hosts {
			if !claim(id) || strings.TrimSpace(host.Address) == "" || host.Port < 1 || host.Port > 65535 {
				return errors.New("invalid cloud host")
			}
			if host.Group != "" {
				if _, ok := workspace.Groups[host.Group]; !ok {
					return errors.New("host group not found")
				}
			}
			if host.IdentityID != "" || host.UseGroupIdentity {
				if host.IdentityID != "" {
					if _, ok := workspace.Identities[host.IdentityID]; !ok {
						return errors.New("host identity not found")
					}
				}
				if host.UseGroupIdentity && (host.Group == "" || host.IdentityID != "") {
					return errors.New("invalid group identity reference")
				}
				if host.Password != nil || len(host.PrivateKey) != 0 || host.Passphrase != nil || host.Username != "" || host.AuthMethod != "" {
					return errors.New("host credentials duplicate an identity")
				}
			} else {
				if host.AuthMethod != "password" && host.AuthMethod != "key" {
					return errors.New("invalid host authentication")
				}
				if err := validateVaultCredentials(host); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := validate(data.Workspaces.Personal); err != nil {
		return err
	}
	for id, workspace := range data.Workspaces.Others {
		if !validRecordID(id) || strings.TrimSpace(workspace.Name) == "" {
			return errors.New("invalid cloud workspace")
		}
		if err := validate(workspace); err != nil {
			return fmt.Errorf("workspace %s: %w", workspace.Name, err)
		}
	}
	return nil
}

func (a *App) restoreWorkspace(id string, data vaultWorkspace, restore *vaultRestore) (localWorkspace, error) {
	local := emptyWorkspace()
	for id, group := range data.Groups {
		local.Groups = append(local.Groups, Group{ID: id, Name: group.Name, DefaultIdentityID: group.DefaultIdentityID})
	}
	for id, identity := range data.Identities {
		host := Host{ID: id}
		credentials := vaultHost{ID: id, AuthMethod: identity.AuthMethod, Password: identity.Password, PrivateKey: identity.PrivateKey, Passphrase: identity.Passphrase}
		if err := restore.credentials(credentials, &host); err != nil {
			return local, err
		}
		local.Identities = append(local.Identities, Identity{ID: id, Name: identity.Name, Username: identity.Username, AuthMethod: identity.AuthMethod, KeyPath: host.KeyPath, SavedPassword: host.SavedPassword})
	}
	for id, host := range data.Hosts {
		host.ID = id
		item := Host{ID: id, Name: host.Name, Address: host.Address, Port: host.Port, Username: host.Username, AuthMethod: host.AuthMethod, Group: host.Group, Favorite: host.Favorite, IdentityID: host.IdentityID, UseGroupIdentity: host.UseGroupIdentity}
		if err := restore.credentials(host, &item); err != nil {
			return local, err
		}
		local.Hosts = append(local.Hosts, item)
	}
	sort.Slice(local.Groups, func(i, j int) bool { return local.Groups[i].Name < local.Groups[j].Name })
	sort.Slice(local.Identities, func(i, j int) bool { return local.Identities[i].Name < local.Identities[j].Name })
	sort.Slice(local.Hosts, func(i, j int) bool { return local.Hosts[i].ID < local.Hosts[j].ID })
	if err := a.writeWorkspace(id, local, restore); err != nil {
		return local, err
	}
	path := filepath.Join(a.workspaceDir(id), "known_hosts")
	if err := restore.file(path, []byte(data.KnownHosts)); err != nil {
		return local, err
	}
	if _, err := knownhosts.New(path); err != nil {
		return local, errors.New("invalid cloud trusted server keys")
	}
	return local, nil
}

func (a *App) applyVault(blob []byte, expected string) (applied bool, err error) {
	var data vaultData
	if json.Unmarshal(blob, &data) != nil {
		return false, errors.New("invalid cloud data")
	}
	if err := validateVaultData(data); err != nil {
		return false, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	current, err := a.vaultSnapshotLocked()
	if err != nil || vaultDigest(current) != expected {
		return false, err
	}
	restore := &vaultRestore{a: a}
	defer func() {
		if err != nil {
			err = errors.Join(err, restore.rollback())
		}
	}()
	// This backup contains local metadata only, without passwords or private key contents
	backup := map[string]localWorkspace{}
	for _, id := range append([]string{"personal"}, workspaceIDs(a.workspaces)...) {
		local, readErr := a.workspaceLocked(id)
		if readErr != nil {
			return false, readErr
		}
		backup[id] = local
	}
	backupBlob, _ := json.Marshal(backup)
	if err := restore.file(filepath.Join(a.configDir, "hosts-before-sync.json"), backupBlob); err != nil {
		return false, err
	}
	personal, err := a.restoreWorkspace("personal", data.Workspaces.Personal, restore)
	if err != nil {
		return false, err
	}
	names := map[string]string{}
	active := personal
	activeID := "personal"
	for id, workspace := range data.Workspaces.Others {
		local, restoreErr := a.restoreWorkspace(id, workspace, restore)
		if restoreErr != nil {
			return false, restoreErr
		}
		names[id] = workspace.Name
		if a.workspaceID == id {
			active, activeID = local, id
		}
	}
	retained := map[string]bool{}
	collect := func(workspace vaultWorkspace) {
		for id := range workspace.Hosts {
			retained[id] = true
		}
		for id := range workspace.Identities {
			retained[id] = true
		}
	}
	collect(data.Workspaces.Personal)
	for _, workspace := range data.Workspaces.Others {
		collect(workspace)
	}
	for _, workspace := range backup {
		oldIDs := []string{}
		for _, host := range workspace.Hosts {
			oldIDs = append(oldIDs, host.ID)
		}
		for _, identity := range workspace.Identities {
			oldIDs = append(oldIDs, identity.ID)
		}
		for _, id := range oldIDs {
			if !retained[id] {
				if err := restore.secret(id, nil); err != nil {
					return false, err
				}
				if err := restore.secret(passphraseID(id), nil); err != nil {
					return false, err
				}
			}
		}
	}
	namesBlob, _ := json.MarshalIndent(names, "", "  ")
	if err := restore.file(filepath.Join(a.configDir, "workspaces.json"), namesBlob); err != nil {
		return false, err
	}
	a.workspaceID, a.workspaces = activeID, names
	a.hosts, a.groups, a.identities = active.Hosts, active.Groups, active.Identities
	return true, nil
}

func workspaceIDs(names map[string]string) []string {
	ids := []string{}
	for id := range names {
		ids = append(ids, id)
	}
	return ids
}

func (a *App) ConnectWorkspaceHost(workspaceID, hostID, password, passphrase, fingerprint string) (ConnectResult, error) {
	return a.connectHost("", workspaceID, hostID, password, passphrase, fingerprint, "ssh")
}

func (a *App) ReconnectWorkspaceHost(sessionID, workspaceID, hostID, password, passphrase, fingerprint string) (ConnectResult, error) {
	if sessionID == "" {
		return ConnectResult{}, errors.New("session ID is required")
	}
	return a.connectHost(sessionID, workspaceID, hostID, password, passphrase, fingerprint, "ssh")
}
