package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/pkg/sftp"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/crypto/ssh"
)

type fileSession struct {
	client         *sftp.Client
	ssh            *ssh.Client
	ctx            context.Context
	cancel         context.CancelFunc
	mu             sync.Mutex
	transferID     string
	transferCancel context.CancelFunc
	closeOnce      sync.Once
}

func (s *fileSession) close() {
	s.closeOnce.Do(func() {
		s.cancel()
		_ = s.ssh.Close()
		_ = s.client.Close()
	})
}

type FileEntry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
	Symlink   bool   `json:"symlink"`
	Size      int64  `json:"size"`
	Modified  int64  `json:"modified"`
}

type FileListing struct {
	Path    string      `json:"path"`
	Parent  string      `json:"parent"`
	Entries []FileEntry `json:"entries"`
}

func (a *App) ConnectSFTP(workspaceID, hostID, password, passphrase, fingerprint string) (ConnectResult, error) {
	return a.connectHost("", workspaceID, hostID, password, passphrase, fingerprint, "sftp")
}

func (a *App) ReconnectSFTP(sessionID, workspaceID, hostID, password, passphrase, fingerprint string) (ConnectResult, error) {
	if sessionID == "" {
		return ConnectResult{}, errors.New("session ID is required")
	}
	return a.connectHost(sessionID, workspaceID, hostID, password, passphrase, fingerprint, "sftp")
}

func (a *App) startSFTP(client *ssh.Client, id string) (ConnectResult, error) {
	files, err := sftp.NewClient(client)
	if err != nil {
		_ = client.Close()
		return ConnectResult{}, fmt.Errorf("start SFTP: %w", err)
	}
	if id == "" {
		id = newID()
	}
	ctx, cancel := context.WithCancel(context.Background())
	session := &fileSession{client: files, ssh: client, ctx: ctx, cancel: cancel}
	a.mu.Lock()
	if a.sessions[id] != nil || a.locals[id] != nil || a.files[id] != nil {
		a.mu.Unlock()
		session.close()
		return ConnectResult{}, errors.New("session is already active")
	}
	a.files[id] = session
	a.mu.Unlock()
	go func() {
		err := files.Wait()
		a.mu.Lock()
		current := a.files[id] == session
		if current {
			delete(a.files, id)
		}
		a.mu.Unlock()
		session.close()
		if current {
			reason := "SFTP connection closed"
			if err != nil {
				reason = err.Error()
			}
			a.emit("terminal:closed", sessionEvent{SessionID: id, Reason: reason})
		}
	}()
	return ConnectResult{Status: "connected", SessionID: id}, nil
}

func (a *App) fileSession(id string) (*fileSession, error) {
	a.mu.RLock()
	session := a.files[id]
	a.mu.RUnlock()
	if session == nil || session.ctx.Err() != nil {
		return nil, errors.New("SFTP connection is closed")
	}
	return session, nil
}

func makeFileListing(directory, parent string, items []os.FileInfo, join func(...string) string) FileListing {
	listing := FileListing{Path: directory, Parent: parent, Entries: []FileEntry{}}
	for _, item := range items {
		if item.Name() == "." || item.Name() == ".." {
			continue
		}
		listing.Entries = append(listing.Entries, FileEntry{Name: item.Name(), Path: join(directory, item.Name()), Directory: item.IsDir(), Symlink: item.Mode()&os.ModeSymlink != 0, Size: item.Size(), Modified: item.ModTime().Unix()})
	}
	sort.Slice(listing.Entries, func(i, j int) bool {
		left, right := listing.Entries[i], listing.Entries[j]
		if left.Directory != right.Directory {
			return left.Directory
		}
		return strings.ToLower(left.Name) < strings.ToLower(right.Name)
	})
	return listing
}

func (a *App) ListLocalFiles(directory string) (FileListing, error) {
	if directory == "" {
		var err error
		directory, err = os.UserHomeDir()
		if err != nil {
			return FileListing{}, err
		}
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return FileListing{}, err
	}
	directory, err = filepath.EvalSymlinks(directory)
	if err != nil {
		return FileListing{}, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return FileListing{}, err
	}
	items := []os.FileInfo{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return FileListing{}, err
		}
		items = append(items, info)
	}
	return makeFileListing(directory, filepath.Dir(directory), items, filepath.Join), nil
}

func (a *App) ListSFTPFiles(id, directory string) (FileListing, error) {
	session, err := a.fileSession(id)
	if err != nil {
		return FileListing{}, err
	}
	if directory == "" {
		directory, err = session.client.Getwd()
		if err != nil {
			return FileListing{}, err
		}
	}
	directory, err = session.client.RealPath(directory)
	if err != nil {
		return FileListing{}, err
	}
	items, err := session.client.ReadDir(directory)
	if err != nil {
		return FileListing{}, err
	}
	return makeFileListing(directory, path.Dir(directory), items, path.Join), nil
}

func (a *App) ChooseLocalFolder() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Choose local folder"})
}

func (a *App) ChooseUploadFiles() ([]string, error) {
	files, err := wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "Upload files"})
	if files == nil {
		files = []string{}
	}
	return files, err
}

func (a *App) CreateFileFolder(id, side, parent, name string) error {
	if err := validFileName(name); err != nil {
		return err
	}
	if side == "local" {
		if err := validLocalName(name); err != nil {
			return err
		}
		return os.Mkdir(filepath.Join(parent, name), 0755)
	}
	if side != "remote" {
		return errors.New("choose a local or remote folder")
	}
	session, err := a.fileSession(id)
	if err != nil {
		return err
	}
	return session.client.Mkdir(path.Join(parent, name))
}
