package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

type FileTransferProgress struct {
	SessionID string `json:"sessionId"`
	ID        string `json:"id"`
	Direction string `json:"direction"`
	Phase     string `json:"phase"`
	Current   string `json:"current"`
	Copied    int64  `json:"copied"`
	Total     int64  `json:"total"`
	Files     int    `json:"files"`
	Completed int    `json:"completed"`
}

type FileTransferResult struct {
	Status    string   `json:"status"`
	Conflicts []string `json:"conflicts"`
}

type transferFS struct{ remote *sftp.Client }

func (f transferFS) join(parts ...string) string {
	if f.remote != nil {
		return path.Join(parts...)
	}
	return filepath.Join(parts...)
}

func (f transferFS) base(name string) string {
	if f.remote != nil {
		return path.Base(name)
	}
	return filepath.Base(name)
}

func (f transferFS) parent(name string) string {
	if f.remote != nil {
		return path.Dir(name)
	}
	return filepath.Dir(name)
}

func (f transferFS) stat(name string) (os.FileInfo, error) {
	if f.remote != nil {
		return f.remote.Lstat(name)
	}
	return os.Lstat(name)
}

func (f transferFS) readDir(name string) ([]os.FileInfo, error) {
	if f.remote != nil {
		return f.remote.ReadDir(name)
	}
	entries, err := os.ReadDir(name)
	if err != nil {
		return nil, err
	}
	items := []os.FileInfo{}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		items = append(items, info)
	}
	return items, nil
}

func (f transferFS) open(name string) (io.ReadCloser, error) {
	if f.remote != nil {
		return f.remote.Open(name)
	}
	return os.Open(name)
}

func (f transferFS) temporary(directory string) (io.WriteCloser, string, error) {
	if f.remote != nil {
		name := path.Join(directory, ".shell-anywhere-"+newID()+".part")
		file, err := f.remote.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		return file, name, err
	}
	file, err := os.CreateTemp(directory, ".shell-anywhere-*.part")
	if err != nil {
		return nil, "", err
	}
	return file, file.Name(), nil
}

func (f transferFS) remove(name string) error {
	if f.remote != nil {
		return f.remote.Remove(name)
	}
	return os.Remove(name)
}

func (f transferFS) mkdir(name string) error {
	if f.remote != nil {
		return f.remote.Mkdir(name)
	}
	return os.Mkdir(name, 0755)
}

func (f transferFS) chmod(name string, mode os.FileMode) error {
	if f.remote != nil {
		return f.remote.Chmod(name, mode.Perm())
	}
	return os.Chmod(name, mode.Perm())
}

func (f transferFS) commit(temporary, target string, overwrite bool) error {
	info, err := f.stat(target)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
			return errors.New("the destination is not a regular file")
		}
		if !overwrite {
			return errors.New("the destination appeared during transfer; retry to replace it")
		}
	}
	if f.remote == nil {
		return os.Rename(temporary, target)
	}
	if _, supported := f.remote.HasExtension("posix-rename@openssh.com"); supported {
		return f.remote.PosixRename(temporary, target)
	}
	if os.IsNotExist(err) {
		return f.remote.Rename(temporary, target)
	}
	backup := target + ".shell-anywhere-" + newID() + ".bak"
	if err := f.remote.Rename(target, backup); err != nil {
		return err
	}
	if err := f.remote.Rename(temporary, target); err != nil {
		return errors.Join(err, f.remote.Rename(backup, target))
	}
	return f.remote.Remove(backup)
}

func validFileName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
		return errors.New("choose a file or folder name without path separators")
	}
	return nil
}

func validLocalName(name string) error {
	if err := validFileName(name); err != nil {
		return err
	}
	if runtime.GOOS != "windows" {
		return nil
	}
	stem := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	reserved := stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '1' && stem[3] <= '9')
	if reserved || strings.ContainsAny(name, ":*?\"<>|") || strings.TrimRight(name, ". ") != name {
		return fmt.Errorf("%q is not a valid filename on this computer", name)
	}
	return nil
}

type transferItem struct {
	source string
	target string
	info   os.FileInfo
}

func planTransfer(ctx context.Context, source, destination transferFS, sources []string, target string, overwrite bool) ([]transferItem, []string, error) {
	root, err := destination.stat(target)
	if err != nil {
		return nil, nil, err
	}
	if !root.IsDir() || root.Mode()&os.ModeSymlink != 0 {
		return nil, nil, errors.New("choose a destination folder without symbolic links")
	}
	items := []transferItem{}
	conflicts := []string{}
	targets := map[string]bool{}
	var walk func(string, string, int) error
	walk = func(from, to string, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if depth > 128 {
			return errors.New("the folder tree is too deep")
		}
		key := to
		if destination.remote == nil && runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if targets[key] {
			return fmt.Errorf("multiple selected items have the same destination %s", to)
		}
		targets[key] = true
		info, err := source.stat(from)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return fmt.Errorf("%s is a symbolic link or special file and cannot be copied", from)
		}
		existing, err := destination.stat(to)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if err == nil {
			if existing.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("the destination %s is a symbolic link", to)
			}
			if existing.IsDir() != info.IsDir() {
				return fmt.Errorf("a different file type already exists at %s", to)
			}
			if !info.IsDir() && !overwrite {
				conflicts = append(conflicts, to)
			}
		}
		items = append(items, transferItem{source: from, target: to, info: info})
		if !info.IsDir() {
			return nil
		}
		entries, err := source.readDir(from)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := validFileName(entry.Name()); err != nil {
				return err
			}
			if destination.remote == nil {
				if err := validLocalName(entry.Name()); err != nil {
					return err
				}
			}
			if err := walk(source.join(from, entry.Name()), destination.join(to, entry.Name()), depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	seen := map[string]bool{}
	for _, from := range sources {
		if from == "" || seen[from] {
			continue
		}
		seen[from] = true
		name := source.base(from)
		if err := validFileName(name); err != nil {
			return nil, nil, err
		}
		if destination.remote == nil {
			if err := validLocalName(name); err != nil {
				return nil, nil, err
			}
		}
		if err := walk(from, destination.join(target, name), 0); err != nil {
			return nil, nil, err
		}
	}
	return items, conflicts, nil
}

func copyTransferFile(ctx context.Context, source, destination transferFS, item transferItem, overwrite bool, progress func(int64)) (err error) {
	input, err := source.open(item.source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, temporary, err := destination.temporary(destination.parent(item.target))
	if err != nil {
		return err
	}
	defer func() {
		_ = output.Close()
		if err != nil {
			_ = destination.remove(temporary)
		}
	}()
	buffer := make([]byte, 64*1024)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, readErr := input.Read(buffer)
		if n > 0 {
			written, writeErr := output.Write(buffer[:n])
			progress(int64(written))
			if writeErr != nil {
				return writeErr
			}
			if written != n {
				return io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := destination.chmod(temporary, item.info.Mode()); err != nil {
		return err
	}
	return destination.commit(temporary, item.target, overwrite)
}

func (a *App) TransferSFTP(sessionID, id, direction string, sources []string, target string, overwrite bool, peerSessionID string) (result FileTransferResult, err error) {
	session, err := a.fileSession(sessionID)
	if err != nil {
		return result, err
	}
	if id == "" || len(sources) == 0 || target == "" {
		return result, errors.New("choose files and a destination folder")
	}
	if direction != "upload" && direction != "download" {
		return result, errors.New("choose upload or download")
	}
	participants := map[string]*fileSession{sessionID: session}
	var peer *fileSession
	if peerSessionID != "" {
		if peerSessionID == sessionID {
			return result, errors.New("choose a separate SFTP connection for the other pane")
		}
		peer, err = a.fileSession(peerSessionID)
		if err != nil {
			return result, err
		}
		participants[peerSessionID] = peer
	}
	ctx, cancel := context.WithCancel(session.ctx)
	defer cancel()
	if peer != nil {
		stop := context.AfterFunc(peer.ctx, cancel)
		defer stop()
	}
	ids := []string{sessionID}
	if peer != nil {
		ids = append(ids, peerSessionID)
	}
	sort.Strings(ids)
	for _, key := range ids {
		participants[key].mu.Lock()
	}
	for _, key := range ids {
		if participants[key].transferCancel != nil {
			for i := len(ids) - 1; i >= 0; i-- {
				participants[ids[i]].mu.Unlock()
			}
			return result, errors.New("another transfer is already running in one of these connections")
		}
	}
	for _, key := range ids {
		participants[key].transferID, participants[key].transferCancel = id, cancel
	}
	for i := len(ids) - 1; i >= 0; i-- {
		participants[ids[i]].mu.Unlock()
	}
	defer func() {
		for _, participant := range participants {
			participant.mu.Lock()
			participant.transferID, participant.transferCancel = "", nil
			participant.mu.Unlock()
		}
	}()
	state := FileTransferProgress{SessionID: sessionID, ID: id, Direction: direction, Phase: "preparing"}
	if peer != nil {
		state.Direction = "copy"
	}
	a.emit("files:progress", state)
	defer func() {
		if ctx.Err() != nil || (peer != nil && peer.ctx.Err() != nil) {
			state.Phase = "cancelled"
			result.Status = "cancelled"
			err = nil
		} else if err != nil {
			state.Phase = "failed"
		} else {
			state.Phase = result.Status
		}
		a.emit("files:progress", state)
	}()
	source, destination := transferFS{}, transferFS{remote: session.client}
	if peer != nil {
		source.remote = peer.client
	}
	if direction == "download" {
		source, destination = destination, source
	}
	if peer != nil && session.ssh.RemoteAddr().String() == peer.ssh.RemoteAddr().String() && session.ssh.User() == peer.ssh.User() {
		for _, from := range sources {
			to := path.Join(target, source.base(from))
			if to == path.Clean(from) || strings.HasPrefix(to, strings.TrimRight(path.Clean(from), "/")+"/") {
				return result, errors.New("a file or folder cannot be copied onto itself or inside itself")
			}
		}
	}
	items, conflicts, err := planTransfer(ctx, source, destination, sources, target, overwrite)
	if err != nil {
		return result, err
	}
	if len(conflicts) != 0 {
		return FileTransferResult{Status: "conflict", Conflicts: conflicts}, nil
	}
	for _, item := range items {
		if !item.info.IsDir() {
			state.Total += item.info.Size()
			state.Files++
		}
	}
	state.Phase = "copying"
	last := time.Time{}
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		state.Current = item.target
		if item.info.IsDir() {
			if err := destination.mkdir(item.target); err != nil {
				info, statErr := destination.stat(item.target)
				if statErr != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
					return result, err
				}
			}
			continue
		}
		err := copyTransferFile(ctx, source, destination, item, overwrite, func(bytes int64) {
			state.Copied += bytes
			if time.Since(last) >= 80*time.Millisecond {
				a.emit("files:progress", state)
				last = time.Now()
			}
		})
		if err != nil {
			return result, fmt.Errorf("copy %s: %w", item.source, err)
		}
		state.Completed++
		a.emit("files:progress", state)
	}
	return FileTransferResult{Status: "completed", Conflicts: []string{}}, nil
}

func (a *App) CancelSFTPTransfer(sessionID, id string) error {
	session, err := a.fileSession(sessionID)
	if err != nil {
		return err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.transferID == id && session.transferCancel != nil {
		session.transferCancel()
	}
	return nil
}
