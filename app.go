package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zalando/go-keyring"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

type Host struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Address          string `json:"address"`
	Port             int    `json:"port"`
	Username         string `json:"username"`
	AuthMethod       string `json:"authMethod"`
	KeyPath          string `json:"keyPath"`
	Group            string `json:"groupId"`
	IdentityID       string `json:"identityId"`
	UseGroupIdentity bool   `json:"useGroupIdentity"`
	WorkspaceID      string `json:"workspaceId,omitempty"`
	credentialID     string
	Favorite         bool `json:"favorite"`
	SavedPassword    bool `json:"savedPassword"`
}

const passwordService = "shell-everywhere-ssh"

type passwordStore interface {
	Set(string, string) error
	Get(string) (string, error)
	Delete(string) error
}

type systemPasswordStore struct{}

func (systemPasswordStore) Set(id, password string) error {
	return keyring.Set(passwordService, id, password)
}
func (systemPasswordStore) Get(id string) (string, error) { return keyring.Get(passwordService, id) }
func (systemPasswordStore) Delete(id string) error        { return keyring.Delete(passwordService, id) }

type ConnectResult struct {
	Status      string `json:"status"`
	SessionID   string `json:"sessionId,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	KeyType     string `json:"keyType,omitempty"`
}

type terminalEvent struct {
	SessionID string `json:"sessionId"`
	Data      string `json:"data"`
}

type sessionEvent struct {
	SessionID string `json:"sessionId"`
	Reason    string `json:"reason"`
}

type activeSession struct {
	client  *ssh.Client
	ssh     *ssh.Session
	stdin   io.WriteCloser
	hostID  string
	writeMu sync.Mutex
}

type localTerminal interface {
	io.ReadWriteCloser
	Resize(columns, rows int) error
	Wait(context.Context) (uint32, error)
}

type localSession struct {
	terminal  localTerminal
	writeMu   sync.Mutex
	closeOnce sync.Once
}

func (s *localSession) close() { s.closeOnce.Do(func() { _ = s.terminal.Close() }) }

type App struct {
	ctx            context.Context
	configDir      string
	mu             sync.RWMutex
	hosts          []Host
	groups         []Group
	identities     []Identity
	workspaceID    string
	workspaces     map[string]string
	sessions       map[string]*activeSession
	locals         map[string]*localSession
	files          map[string]*fileSession
	eventSink      func(string, any)
	passwords      passwordStore
	accountMu      sync.Mutex
	accountLoaded  bool
	account        savedAccount
	accountLogin   *accountLogin
	accounts       passwordStore
	accountHTTP    *http.Client
	accountURL     string
	openAccountURL func(string)
	vault          vaultSync
	updates        updateCheck
}

func NewApp(configDir string) (*App, error) {
	if configDir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		configDir = filepath.Join(base, "shell-everywhere")
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return nil, err
	}
	a := &App{
		configDir: configDir,
		sessions:  make(map[string]*activeSession),
		locals:    make(map[string]*localSession),
		files:     make(map[string]*fileSession),
		passwords: systemPasswordStore{},
		accounts:  systemAccountStore{},
		vault: vaultSync{
			keys: systemVaultStore{}, requests: make(chan struct{}, 1), stop: make(chan struct{}),
		},
	}

	a.workspaceID = "personal"
	a.workspaces = make(map[string]string)
	data, err := os.ReadFile(filepath.Join(configDir, "workspaces.json"))
	if err == nil {
		if err := json.Unmarshal(data, &a.workspaces); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for id, name := range a.workspaces {
		if !validRecordID(id) || strings.TrimSpace(name) == "" {
			return nil, errors.New("invalid workspace data")
		}
	}
	workspace, err := a.readWorkspace("personal")
	if err != nil {
		return nil, err
	}
	a.hosts, a.groups, a.identities = workspace.Hosts, workspace.Groups, workspace.Identities

	return a, nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	go a.runVaultSync(ctx)
}

func (a *App) shutdown(ctx context.Context) {
	if a.vault.stop != nil {
		a.vault.once.Do(func() { close(a.vault.stop) })
	}
	a.closeAccountLogin()
	a.mu.Lock()
	sessions := a.sessions
	a.sessions = make(map[string]*activeSession)
	files := a.files
	a.files = make(map[string]*fileSession)
	locals := a.locals
	a.locals = make(map[string]*localSession)
	a.mu.Unlock()
	for _, session := range sessions {
		_ = session.ssh.Close()
		_ = session.client.Close()
	}
	for _, session := range files {
		session.close()
	}
	for _, session := range locals {
		session.close()
	}
}

func (a *App) ChooseKey() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose SSH private key",
	})
}

type unknownHostKey struct {
	fingerprint string
	keyType     string
}

func (e *unknownHostKey) Error() string { return "host key has not been trusted" }

func (a *App) ConnectHost(hostID, password, passphrase, trustedFingerprint string) (ConnectResult, error) {
	return a.connectHost("", "", hostID, password, passphrase, trustedFingerprint, "ssh")
}

func (a *App) ReconnectHost(sessionID, hostID, password, passphrase, trustedFingerprint string) (ConnectResult, error) {
	if sessionID == "" {
		return ConnectResult{}, errors.New("session ID is required")
	}
	return a.connectHost(sessionID, "", hostID, password, passphrase, trustedFingerprint, "ssh")
}

func (a *App) connectHost(sessionID, workspaceID, hostID, password, passphrase, trustedFingerprint, protocol string) (ConnectResult, error) {
	a.mu.RLock()
	if sessionID != "" && (a.sessions[sessionID] != nil || a.locals[sessionID] != nil || a.files[sessionID] != nil) {
		a.mu.RUnlock()
		return ConnectResult{}, errors.New("session is already active")
	}
	if workspaceID == "" {
		workspaceID = a.workspaceID
	}
	workspace, workspaceErr := a.workspaceLocked(workspaceID)
	if workspaceErr != nil {
		a.mu.RUnlock()
		return ConnectResult{}, workspaceErr
	}
	var host Host
	for _, candidate := range workspace.Hosts {
		if candidate.ID == hostID {
			host = resolvedHost(candidate, workspaceID, workspace)
			break
		}
	}
	a.mu.RUnlock()
	if host.ID == "" {
		return ConnectResult{}, errors.New("host not found")
	}
	address := net.JoinHostPort(host.Address, fmt.Sprint(host.Port))
	knownHostsPath := filepath.Join(a.workspaceDir(workspaceID), "known_hosts")
	if _, err := os.Stat(knownHostsPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(knownHostsPath, nil, 0600); err != nil {
			return ConnectResult{}, err
		}
	}
	verifyKnown, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return ConnectResult{}, fmt.Errorf("read known hosts: %w", err)
	}
	var acceptedKey ssh.PublicKey
	config := &ssh.ClientConfig{
		User:    host.Username,
		Timeout: 12 * time.Second,
		HostKeyCallback: func(hostname string, remote net.Addr, key ssh.PublicKey) error {
			err := verifyKnown(hostname, remote, key)
			if err == nil {
				return nil
			}
			var keyError *knownhosts.KeyError
			if !errors.As(err, &keyError) || len(keyError.Want) != 0 {
				return fmt.Errorf("host key verification failed; the saved key may have changed: %w", err)
			}
			fingerprint := ssh.FingerprintSHA256(key)
			if trustedFingerprint != fingerprint {
				return &unknownHostKey{fingerprint: fingerprint, keyType: key.Type()}
			}
			acceptedKey = key
			return nil
		},
	}
	if host.AuthMethod == "password" {
		if password == "" && host.SavedPassword {
			password, err = a.passwords.Get(host.credentialID)
			if err != nil {
				return ConnectResult{}, fmt.Errorf("read saved password: %w", err)
			}
		}
		if password == "" {
			return ConnectResult{}, errors.New("enter the SSH password")
		}
		config.Auth = []ssh.AuthMethod{ssh.Password(password)}
	} else {
		keyPath, err := privateKeyPath(host.KeyPath)
		if err != nil {
			return ConnectResult{}, err
		}
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			return ConnectResult{}, fmt.Errorf("read private key: %w", err)
		}
		var signer ssh.Signer
		if passphrase == "" {
			passphrase, err = a.passwords.Get(passphraseID(host.credentialID))
			if err != nil && !errors.Is(err, keyring.ErrNotFound) {
				return ConnectResult{}, fmt.Errorf("read saved key passphrase: %w", err)
			}
		}
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyData)
		}
		if err != nil {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return ConnectResult{Status: "passphrase_required"}, nil
			}
			return ConnectResult{}, fmt.Errorf("parse private key: %w", err)
		}
		config.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	}
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		var challenge *unknownHostKey
		if errors.As(err, &challenge) {
			return ConnectResult{Status: "host_key_unknown", Fingerprint: challenge.fingerprint, KeyType: challenge.keyType}, nil
		}
		return ConnectResult{}, fmt.Errorf("connect to %s: %w", address, err)
	}
	if acceptedKey != nil {
		a.mu.Lock()
		line := knownhosts.Line([]string{knownhosts.Normalize(address)}, acceptedKey)
		file, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			a.mu.Unlock()
			_ = client.Close()
			return ConnectResult{}, err
		}
		_, err = fmt.Fprintln(file, line)
		closeErr := file.Close()
		a.mu.Unlock()
		if err != nil || closeErr != nil {
			_ = client.Close()
			return ConnectResult{}, errors.Join(err, closeErr)
		}
		a.signalVaultSync()
	}
	if host.AuthMethod == "password" && password != "" && host.credentialID != host.ID {
		if err := a.rememberIdentityPassword(workspaceID, host, password); err != nil {
			_ = client.Close()
			return ConnectResult{}, fmt.Errorf("save identity password: %w", err)
		}
	}
	if host.AuthMethod == "key" && passphrase != "" {
		a.mu.Lock()
		currentWorkspace, readErr := a.workspaceLocked(workspaceID)
		if readErr != nil {
			err = readErr
		}
		for _, current := range currentWorkspace.Hosts {
			current = resolvedHost(current, workspaceID, currentWorkspace)
			if current.ID == host.ID && current.AuthMethod == "key" && current.KeyPath == host.KeyPath && current.credentialID == host.credentialID {
				err = a.passwords.Set(passphraseID(host.credentialID), passphrase)
				break
			}
		}
		a.mu.Unlock()
		if err != nil {
			_ = client.Close()
			return ConnectResult{}, fmt.Errorf("save key passphrase: %w", err)
		}
		a.signalVaultSync()
	}
	if protocol == "sftp" {
		return a.startSFTP(client, sessionID)
	}
	remoteSession, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return ConnectResult{}, err
	}
	stdin, err := remoteSession.StdinPipe()
	if err != nil {
		_ = remoteSession.Close()
		_ = client.Close()
		return ConnectResult{}, err
	}
	id := sessionID
	if id == "" {
		id = newID()
	}
	w := &terminalWriter{app: a, sessionID: id}
	remoteSession.Stdout = w
	remoteSession.Stderr = w
	if err := remoteSession.RequestPty("xterm-256color", 32, 110, ssh.TerminalModes{}); err != nil {
		_ = remoteSession.Close()
		_ = client.Close()
		return ConnectResult{}, fmt.Errorf("request terminal: %w", err)
	}
	s := &activeSession{client: client, ssh: remoteSession, stdin: stdin, hostID: hostID}
	a.mu.Lock()
	if a.sessions[id] != nil || a.locals[id] != nil || a.files[id] != nil {
		a.mu.Unlock()
		_ = remoteSession.Close()
		_ = client.Close()
		return ConnectResult{}, errors.New("session is already active")
	}
	a.sessions[id] = s
	a.mu.Unlock()
	if err := remoteSession.Shell(); err != nil {
		a.DisconnectSession(id)
		return ConnectResult{}, fmt.Errorf("start shell: %w", err)
	}
	go func() {
		err := remoteSession.Wait()
		a.mu.Lock()
		emitClosed := a.sessions[id] == s
		if a.sessions[id] == s {
			delete(a.sessions, id)
		}
		a.mu.Unlock()
		_ = client.Close()
		if !emitClosed {
			return
		}
		reason := "Connection closed"
		if err != nil {
			reason = err.Error()
		}
		a.emit("terminal:closed", sessionEvent{SessionID: id, Reason: reason})
	}()
	return ConnectResult{Status: "connected", SessionID: id}, nil
}

type terminalWriter struct {
	app       *App
	sessionID string
}

func (w *terminalWriter) Write(p []byte) (int, error) {
	w.app.emit("terminal:output", terminalEvent{SessionID: w.sessionID, Data: base64.StdEncoding.EncodeToString(p)})
	return len(p), nil
}

func (a *App) emit(name string, data any) {
	if a.eventSink != nil {
		a.eventSink(name, data)
		return
	}
	runtime.EventsEmit(a.ctx, name, data)
}

func (a *App) StartLocalShell(shell string) (string, error) {
	available := false
	for _, option := range localShells() {
		if option.ID == shell {
			available = true
			break
		}
	}
	if !available {
		return "", errors.New("choose a local shell available on this computer")
	}
	terminal, err := startLocalPTY(shell)
	if err != nil {
		return "", fmt.Errorf("start local %s: %w", shell, err)
	}
	id := newID()
	session := &localSession{terminal: terminal}
	a.mu.Lock()
	a.locals[id] = session
	a.mu.Unlock()
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		_, _ = io.Copy(&terminalWriter{app: a, sessionID: id}, terminal)
	}()
	go func() {
		code, err := terminal.Wait(context.Background())
		session.close()
		<-outputDone
		a.mu.Lock()
		delete(a.locals, id)
		a.mu.Unlock()
		reason := fmt.Sprintf("Local shell exited (%d)", code)
		if err != nil {
			reason = "Local shell closed"
		}
		a.emit("terminal:closed", sessionEvent{SessionID: id, Reason: reason})
	}()
	return id, nil
}

type LocalShellOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (a *App) GetLocalShells() []LocalShellOption {
	return localShells()
}

func (a *App) WriteSession(id, data string) error {
	a.mu.RLock()
	s := a.sessions[id]
	local := a.locals[id]
	a.mu.RUnlock()
	if local != nil {
		local.writeMu.Lock()
		defer local.writeMu.Unlock()
		_, err := io.WriteString(local.terminal, data)
		return err
	}
	if s == nil {
		return errors.New("session is closed")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := io.WriteString(s.stdin, data)
	return err
}

func (a *App) ResizeSession(id string, rows, columns int) error {
	if rows < 1 || columns < 1 {
		return errors.New("invalid terminal size")
	}
	a.mu.RLock()
	s := a.sessions[id]
	local := a.locals[id]
	a.mu.RUnlock()
	if local != nil {
		return local.terminal.Resize(columns, rows)
	}
	if s == nil {
		return errors.New("session is closed")
	}
	return s.ssh.WindowChange(rows, columns)
}

func (a *App) DisconnectSession(id string) {
	a.mu.Lock()
	s := a.sessions[id]
	local := a.locals[id]
	files := a.files[id]
	delete(a.sessions, id)
	delete(a.locals, id)
	delete(a.files, id)
	a.mu.Unlock()
	if files != nil {
		files.close()
	}
	if local != nil {
		local.close()
	}
	if s != nil {
		_ = s.ssh.Close()
		_ = s.client.Close()
	}
}

func newID() string {
	var bytes [12]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(bytes[:])
}
