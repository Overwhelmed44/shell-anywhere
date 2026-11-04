package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	msar "github.com/Overwhelmed44/msar/clients/go"
	"github.com/zalando/go-keyring"
)

type VaultStatus struct {
	Enabled     bool   `json:"enabled"`
	Protected   bool   `json:"protected"`
	Locked      bool   `json:"locked"`
	Busy        bool   `json:"busy"`
	NeedsChoice bool   `json:"needsChoice"`
	LocalHosts  int    `json:"localHosts"`
	CloudHosts  int    `json:"cloudHosts"`
	LastSynced  string `json:"lastSynced"`
	Error       string `json:"error"`
}

type vaultSettings struct {
	Owner       string `json:"owner"`
	Enabled     bool   `json:"enabled"`
	SUID        string `json:"suid"`
	Digest      string `json:"digest"`
	LocalDigest string `json:"localDigest"`
	Protected   bool   `json:"protected"`
	LastSynced  string `json:"lastSynced"`
}

type vaultSync struct {
	mu       sync.Mutex
	settings vaultSettings
	status   VaultStatus
	owner    string
	base     string
	key      vaultKey
	keys     passwordStore
	client   *msar.Client
	cookies  http.CookieJar
	requests chan struct{}
	stop     chan struct{}
	once     sync.Once
}

type systemVaultStore struct{}

func (systemVaultStore) Set(id, value string) error {
	return keyring.Set("shell-anywhere-vault", id, value)
}

func (systemVaultStore) Get(id string) (string, error) {
	return keyring.Get("shell-anywhere-vault", id)
}

func (systemVaultStore) Delete(id string) error {
	return keyring.Delete("shell-anywhere-vault", id)
}

func (a *App) loadVault() (string, savedAccount, error) {
	account, err := a.GetAccount()
	if err != nil {
		return "", savedAccount{}, err
	}

	if account.UserID == "" {
		return "", savedAccount{}, errors.New("sign in with Oxee to sync your hosts")
	}

	a.accountMu.Lock()
	origin, err := a.accountOrigin()
	saved := a.account
	a.accountMu.Unlock()

	if err != nil {
		return "", savedAccount{}, err
	}

	base := strings.TrimRight(os.Getenv("SHELLANYWHERE_PROVIDER_URL"), "/")
	if base == "" {
		base = "https://shell-aw.oxee.net/api"
	}

	address, err := url.Parse(base)
	if err != nil || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" {
		return "", savedAccount{}, errors.New("invalid sync server address")
	}

	if address.Scheme != "https" && !(address.Scheme == "http" && (address.Hostname() == "127.0.0.1" || address.Hostname() == "localhost" || address.Hostname() == "::1")) {
		return "", savedAccount{}, errors.New("sync requires HTTPS or a local development address")
	}

	owner := sha256.Sum256([]byte(origin + "|" + base + "|" + account.UserID))
	id := hex.EncodeToString(owner[:])

	if a.vault.owner == id {
		return origin, saved, nil
	}

	a.vault.owner = ""
	a.vault.base = base
	a.vault.settings = vaultSettings{Owner: id}
	a.vault.status = VaultStatus{}
	a.vault.key = vaultKey{}
	a.vault.client = nil
	a.vault.cookies = nil

	if a.vault.keys == nil {
		a.vault.keys = systemVaultStore{}
	}

	data, err := os.ReadFile(filepath.Join(a.configDir, "vault.json"))
	if err == nil {
		var settings vaultSettings

		if json.Unmarshal(data, &settings) != nil {
			return "", savedAccount{}, errors.New("could not read sync settings")
		}

		if settings.Owner == id {
			a.vault.settings = settings
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", savedAccount{}, err
	}

	if a.vault.settings.Protected {
		if data, err := a.vault.keys.Get(id); err == nil {
			_ = json.Unmarshal([]byte(data), &a.vault.key)
		} else if !errors.Is(err, keyring.ErrNotFound) {
			return "", savedAccount{}, errors.New("could not read your saved vault key")
		}
	}

	a.vault.status = VaultStatus{
		Enabled:    a.vault.settings.Enabled,
		Protected:  a.vault.settings.Protected,
		Locked:     a.vault.settings.Protected && len(a.vault.key.Key) != 32,
		LastSynced: a.vault.settings.LastSynced,
	}
	a.vault.owner = id

	return origin, saved, nil
}

func (a *App) vaultClient(ctx context.Context, origin string, saved savedAccount) error {
	if a.vault.client != nil {
		return nil
	}

	client, cookies, err := a.authenticatedAccountClient(origin, saved.RefreshToken)
	if err != nil {
		return err
	}

	response, err := client.Get(ctx, "/api/users/self")
	if err != nil {
		return err
	}

	response.Body.Close()
	if response.StatusCode != http.StatusOK || client.AccessToken() == "" {
		return errors.New("sign in with Oxee again to sync hosts")
	}

	a.vault.client = client
	a.vault.cookies = cookies

	return nil
}

func (a *App) saveVaultSession(origin string) error {
	a.accountMu.Lock()
	defer a.accountMu.Unlock()

	saved := a.account
	refreshFromCookies(origin, a.vault.cookies, &saved)

	if saved.RefreshToken != a.account.RefreshToken {
		return a.saveAccount(origin, saved)
	}

	return nil
}

func (a *App) vaultRequest(ctx context.Context, method, path string, body any, target any) error {
	response, err := a.vault.client.Request(ctx, method, a.vault.base+path, body)
	if err == nil && response.StatusCode == http.StatusUnauthorized {
		response.Body.Close()

		// The provider can be on a different host from the refresh cookie
		refreshed, refreshErr := a.vault.client.Get(ctx, "/api/users/self")
		if refreshErr != nil {
			return errors.New("could not refresh your Oxee session")
		}
		refreshed.Body.Close()
		if refreshed.StatusCode != http.StatusOK {
			return errors.New("sign in with Oxee again to sync hosts")
		}

		response, err = a.vault.client.Request(ctx, method, a.vault.base+path, body)
	}
	if err != nil {
		return errors.New("could not reach the sync server")
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("sync server returned %s", response.Status)
	}

	if target != nil {
		if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(target); err != nil {
			return errors.New("sync server returned an invalid response")
		}
	}

	return nil
}

func vaultDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(digest[:])
}

func (a *App) persistVault() error {
	data, err := json.MarshalIndent(a.vault.settings, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(a.configDir, "vault.json"), data, 0600)
}

func (a *App) GetVaultStatus() (VaultStatus, error) {
	a.vault.mu.Lock()
	defer a.vault.mu.Unlock()

	_, _, err := a.loadVault()
	return a.vault.status, err
}

func (a *App) finishVault(err error) {
	a.vault.status.Busy = false
	a.vault.status.Error = ""

	if err != nil {
		a.vault.status.Error = err.Error()
	}

	a.emit("vault:changed", a.vault.status)
}

func (a *App) ConfigureVaultSync(password, direction string) (status VaultStatus, err error) {
	a.vault.mu.Lock()
	defer a.vault.mu.Unlock()

	origin, saved, err := a.loadVault()
	if err != nil {
		return a.vault.status, err
	}
	if password == "" && len(a.vault.key.Key) != 32 {
		return a.vault.status, errors.New("enter a vault password to encrypt your hosts and SSH credentials")
	}

	a.vault.status.Busy = true
	a.vault.status.NeedsChoice = false
	a.emit("vault:changed", a.vault.status)
	defer func() { a.finishVault(err); status = a.vault.status }()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	if err = a.vaultClient(ctx, origin, saved); err != nil {
		return a.vault.status, err
	}
	defer func() { err = errors.Join(err, a.saveVaultSession(origin)) }()

	var created struct {
		SUID      string `json:"suid"`
		Protected bool   `json:"protected"`
	}

	err = a.vaultRequest(ctx, http.MethodPost, "/private/vaults", map[string]bool{"protected": password != ""}, &created)
	if err != nil {
		return a.vault.status, err
	}
	if len(created.SUID) != 24 {
		return a.vault.status, errors.New("sync server returned an invalid vault id")
	}

	var checked struct {
		Status string `json:"status"`
		Action string `json:"action"`
	}
	path := "/private/vaults/" + url.PathEscape(created.SUID)
	err = a.vaultRequest(ctx, http.MethodPost, path, map[string]any{"digest": "", "local_changes": false}, &checked)
	if err != nil {
		return a.vault.status, err
	}

	local, err := a.vaultSnapshot()
	if err != nil {
		return a.vault.status, err
	}

	key := a.vault.key
	var cloud []byte
	var blob []byte
	protected := created.Protected
	if checked.Action == "init" && password == "" && (!protected || len(key.Key) == 0) {
		protected = false
		key = vaultKey{}
	}
	if checked.Action != "init" {
		blob, err = base64.StdEncoding.DecodeString(checked.Action)
		if err == nil {
			cloud, key, protected, err = decodeVault(blob, password, key)
		}
		if err != nil {
			a.vault.status.Locked = protected
			a.vault.status.Protected = protected
			return a.vault.status, err
		}
	}

	if password != "" && !protected {
		key = newVaultKey(password)
		protected = true
	} else if checked.Action == "init" && password != "" {
		key = newVaultKey(password)
	}

	var localData, cloudData vaultData
	_ = json.Unmarshal(local, &localData)
	_ = json.Unmarshal(cloud, &cloudData)

	if direction == "" && cloud != nil && len(localData.Workspaces.Personal.Hosts)+len(localData.Workspaces.Personal.Groups)+len(localData.Workspaces.Personal.Identities) > 0 && vaultDigest(local) != vaultDigest(cloud) {
		a.vault.status.NeedsChoice = true
		a.vault.status.LocalHosts = len(localData.Workspaces.Personal.Hosts)
		a.vault.status.CloudHosts = len(cloudData.Workspaces.Personal.Hosts)
		return a.vault.status, nil
	}
	if direction != "" && direction != "upload" && direction != "download" {
		return a.vault.status, errors.New("choose cloud hosts or this device's hosts")
	}

	if len(key.Key) != 0 {
		data, _ := json.Marshal(key)
		if err = a.vault.keys.Set(a.vault.owner, string(data)); err != nil {
			return a.vault.status, errors.New("could not save your vault encryption key")
		}
	}

	a.vault.key = key
	if cloud != nil && direction != "upload" {
		applied, applyErr := a.applyVault(cloud, vaultDigest(local))
		if applyErr != nil {
			return a.vault.status, applyErr
		}
		if !applied {
			return a.vault.status, errors.New("hosts changed while syncing; try again")
		}
		local, err = a.vaultSnapshot()
		if err != nil {
			return a.vault.status, err
		}
		a.emit("hosts:changed", nil)
	}

	if cloud == nil || direction == "upload" || protected != created.Protected {
		blob, err = encodeVault(local, key)
		if err != nil {
			return a.vault.status, err
		}
		err = a.vaultRequest(ctx, http.MethodPut, path, map[string]any{
			"vault": base64.StdEncoding.EncodeToString(blob), "digest": vaultDigest(blob), "protected": protected,
		}, nil)
		if err != nil {
			return a.vault.status, err
		}
	}

	a.vault.settings = vaultSettings{
		Owner: a.vault.owner, Enabled: true, SUID: created.SUID, Digest: vaultDigest(blob),
		LocalDigest: vaultDigest(local), Protected: protected, LastSynced: time.Now().UTC().Format(time.RFC3339),
	}
	if err = a.persistVault(); err != nil {
		return a.vault.status, err
	}
	a.vault.status = VaultStatus{Enabled: true, Protected: protected, LastSynced: a.vault.settings.LastSynced}

	return a.vault.status, nil
}

func (a *App) SyncVault() (status VaultStatus, err error) {
	a.vault.mu.Lock()
	defer a.vault.mu.Unlock()

	origin, saved, err := a.loadVault()
	if err != nil {
		return a.vault.status, err
	}
	if !a.vault.settings.Enabled || a.vault.status.Locked {
		return a.vault.status, nil
	}
	if !a.vault.settings.Protected || len(a.vault.key.Key) != 32 {
		return a.vault.status, errors.New("set a vault password in Host syncing to sync SSH credentials")
	}

	a.vault.status.Busy = true
	a.emit("vault:changed", a.vault.status)
	defer func() { a.finishVault(err); status = a.vault.status }()

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	if err = a.vaultClient(ctx, origin, saved); err != nil {
		return a.vault.status, err
	}
	defer func() { err = errors.Join(err, a.saveVaultSession(origin)) }()

	local, err := a.vaultSnapshot()
	if err != nil {
		return a.vault.status, err
	}
	dirty := vaultDigest(local) != a.vault.settings.LocalDigest
	path := "/private/vaults/" + url.PathEscape(a.vault.settings.SUID)
	var checked struct {
		Status string `json:"status"`
		Action string `json:"action"`
	}

	err = a.vaultRequest(ctx, http.MethodPost, path, map[string]any{"digest": a.vault.settings.Digest, "local_changes": dirty}, &checked)
	if err != nil {
		return a.vault.status, err
	}

	if dirty || checked.Action == "init" {
		blob, encodeErr := encodeVault(local, a.vault.key)
		if encodeErr != nil {
			return a.vault.status, encodeErr
		}
		err = a.vaultRequest(ctx, http.MethodPut, path, map[string]any{
			"vault": base64.StdEncoding.EncodeToString(blob), "digest": vaultDigest(blob), "protected": a.vault.settings.Protected,
		}, nil)
		if err != nil {
			return a.vault.status, err
		}
		a.vault.settings.Digest = vaultDigest(blob)
	} else if checked.Status != "synced" {
		blob, decodeErr := base64.StdEncoding.DecodeString(checked.Action)
		if decodeErr != nil {
			return a.vault.status, errors.New("sync server returned invalid vault data")
		}
		cloud, key, protected, decodeErr := decodeVault(blob, "", a.vault.key)
		if decodeErr != nil {
			a.vault.status.Locked = protected
			a.vault.status.Protected = protected
			return a.vault.status, decodeErr
		}
		if !protected {
			return a.vault.status, errors.New("cloud vault is no longer encrypted; open Host syncing to restore encrypted syncing")
		}
		applied, applyErr := a.applyVault(cloud, vaultDigest(local))
		if applyErr != nil {
			return a.vault.status, applyErr
		}
		if !applied {
			a.signalVaultSync()
			return a.vault.status, nil
		}
		local, err = a.vaultSnapshot()
		if err != nil {
			return a.vault.status, err
		}
		a.vault.key = key
		a.vault.settings.Protected = protected
		a.vault.settings.Digest = vaultDigest(blob)
		a.emit("hosts:changed", nil)
	}

	a.vault.settings.LocalDigest = vaultDigest(local)
	a.vault.settings.LastSynced = time.Now().UTC().Format(time.RFC3339)
	if err = a.persistVault(); err != nil {
		return a.vault.status, err
	}
	a.vault.status.Protected = a.vault.settings.Protected
	a.vault.status.LastSynced = a.vault.settings.LastSynced

	return a.vault.status, nil
}

func (a *App) PauseVaultSync() (VaultStatus, error) {
	a.vault.mu.Lock()
	defer a.vault.mu.Unlock()

	if _, _, err := a.loadVault(); err != nil {
		return a.vault.status, err
	}
	a.vault.settings.Enabled = false
	if err := a.persistVault(); err != nil {
		return a.vault.status, err
	}
	a.vault.status.Enabled = false
	a.vault.status.Error = ""
	a.vault.status.NeedsChoice = false
	a.emit("vault:changed", a.vault.status)

	return a.vault.status, nil
}

func (a *App) signalVaultSync() {
	select {
	case a.vault.requests <- struct{}{}:
	default:
	}
}

func (a *App) runVaultSync(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.vault.stop:
			return
		case <-a.vault.requests:
		case <-ticker.C:
		}

		_, _ = a.SyncVault()
	}
}
