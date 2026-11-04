package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func testSFTPConnection(t *testing.T) (*App, Host, string, *atomic.Int32) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, _ := ssh.NewSignerFromKey(key)
	config := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "sftp-password" {
			return nil, ssh.ErrNoAuth
		}
		return nil, nil
	}}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	handlers := sftp.InMemHandler()
	shells := &atomic.Int32{}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					_ = conn.Close()
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for request := range channels {
					channel, requests, err := request.Accept()
					if err != nil {
						return
					}
					go func() {
						defer channel.Close()
						for request := range requests {
							if request.Type == "shell" || request.Type == "pty-req" {
								shells.Add(1)
							}
							var subsystem struct{ Name string }
							if request.Type != "subsystem" || ssh.Unmarshal(request.Payload, &subsystem) != nil || subsystem.Name != "sftp" {
								_ = request.Reply(false, nil)
								continue
							}
							_ = request.Reply(true, nil)
							files := sftp.NewRequestServer(channel, handlers, sftp.WithStartDirectory("/"))
							_ = files.Serve()
							_ = files.Close()
							return
						}
					}()
				}
			}()
		}
	}()
	app, err := NewApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app.passwords = &memoryPasswordStore{values: map[string]string{}}
	app.eventSink = func(string, any) {}
	identity, err := app.SaveIdentity(Identity{Name: "Files", Username: "files-user", AuthMethod: "password"}, "sftp-password", "")
	if err != nil {
		t.Fatal(err)
	}
	host, err := app.SaveHost(Host{Name: "File server", Address: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, IdentityID: identity.ID}, "")
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := app.ConnectSFTP("personal", host.ID, "", "", "")
	if err != nil || challenge.Status != "host_key_unknown" {
		t.Fatalf("SFTP must verify the host key: %+v, %v", challenge, err)
	}
	result, err := app.ConnectSFTP("personal", host.ID, "", "", challenge.Fingerprint)
	if err != nil || result.Status != "connected" {
		t.Fatalf("SFTP could not connect: %+v, %v", result, err)
	}
	t.Cleanup(func() { app.DisconnectSession(result.SessionID) })
	return app, host, result.SessionID, shells
}

func remoteContents(t *testing.T, app *App, id, path string) []byte {
	t.Helper()
	session, err := app.fileSession(id)
	if err != nil {
		t.Fatal(err)
	}
	file, err := session.client.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contents, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return contents
}

func TestSFTPRecursiveUploadDownloadAndReplacement(t *testing.T) {
	app, _, id, shells := testSFTPConnection(t)
	listing, err := app.ListSFTPFiles(id, "")
	if err != nil || listing.Path != "/" {
		t.Fatalf("remote home: %+v, %v", listing, err)
	}
	if err := app.CreateFileFolder(id, "remote", "/", "uploads"); err != nil {
		t.Fatal(err)
	}
	local := t.TempDir()
	folder := filepath.Join(local, "project")
	if err := os.MkdirAll(filepath.Join(folder, "nested", "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	contents := bytes.Repeat([]byte("binary\x00payload"), 18000)
	if err := os.WriteFile(filepath.Join(folder, "nested", "data.bin"), contents, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, ".config"), []byte("configuration"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := app.TransferSFTP(id, "upload", "upload", []string{folder}, "/uploads", false, "")
	if err != nil || result.Status != "completed" {
		t.Fatalf("folder upload: %+v, %v", result, err)
	}
	if !bytes.Equal(remoteContents(t, app, id, "/uploads/project/nested/data.bin"), contents) {
		t.Fatal("uploaded file changed")
	}
	listing, err = app.ListSFTPFiles(id, "/uploads/project/nested")
	if err != nil || len(listing.Entries) != 2 || !listing.Entries[0].Directory {
		t.Fatal("directory listing is incomplete or unsorted")
	}
	target := t.TempDir()
	result, err = app.TransferSFTP(id, "download", "download", []string{"/uploads/project"}, target, false, "")
	if err != nil || result.Status != "completed" {
		t.Fatalf("folder download: %+v, %v", result, err)
	}
	downloaded, err := os.ReadFile(filepath.Join(target, "project", "nested", "data.bin"))
	if err != nil || !bytes.Equal(downloaded, contents) {
		t.Fatal("downloaded file changed")
	}
	if info, err := os.Stat(filepath.Join(target, "project", "nested", "empty")); err != nil || !info.IsDir() {
		t.Fatal("empty folder missing")
	}
	if err := os.WriteFile(filepath.Join(folder, ".config"), []byte("replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err = app.TransferSFTP(id, "conflict", "upload", []string{folder}, "/uploads", false, "")
	if err != nil || result.Status != "conflict" || len(result.Conflicts) != 2 {
		t.Fatalf("replacement was not confirmed: %+v, %v", result, err)
	}
	if string(remoteContents(t, app, id, "/uploads/project/.config")) != "configuration" {
		t.Fatal("conflict modified an existing file")
	}
	result, err = app.TransferSFTP(id, "replace", "upload", []string{folder}, "/uploads", true, "")
	if err != nil || result.Status != "completed" {
		t.Fatalf("replacement failed: %+v, %v", result, err)
	}
	if string(remoteContents(t, app, id, "/uploads/project/.config")) != "replacement" {
		t.Fatal("replacement not uploaded")
	}
	if shells.Load() != 0 {
		t.Fatal("SFTP unexpectedly started a terminal")
	}
}

func TestSFTPCancelPreservesExistingFileAndCleansTemporary(t *testing.T) {
	app, _, id, _ := testSFTPConnection(t)
	local := t.TempDir()
	file := filepath.Join(local, "data.bin")
	if err := os.WriteFile(file, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.TransferSFTP(id, "first", "upload", []string{file}, "/", false, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, bytes.Repeat([]byte("replacement"), 1024*1024), 0644); err != nil {
		t.Fatal(err)
	}
	cancelled := false
	app.eventSink = func(name string, data any) {
		if name != "files:progress" {
			return
		}
		progress := data.(FileTransferProgress)
		if progress.ID == "cancel" && progress.Copied > 0 && !cancelled {
			cancelled = true
			_ = app.CancelSFTPTransfer(id, "cancel")
		}
	}
	result, err := app.TransferSFTP(id, "cancel", "upload", []string{file}, "/", true, "")
	if err != nil || result.Status != "cancelled" || !cancelled {
		t.Fatalf("cancel failed: %+v, %v", result, err)
	}
	if string(remoteContents(t, app, id, "/data.bin")) != "original" {
		t.Fatal("cancel replaced the original destination")
	}
	listing, err := app.ListSFTPFiles(id, "/")
	if err != nil || len(listing.Entries) != 1 {
		t.Fatal("temporary file left after cancel")
	}
}

func TestSFTPClosesReconnectsAndRejectsUnsafePaths(t *testing.T) {
	app, host, id, _ := testSFTPConnection(t)
	if _, err := app.ReconnectSFTP(id, "personal", host.ID, "", "", ""); err == nil {
		t.Fatal("active SFTP session reconnected")
	}
	for _, name := range []string{"../escape", "/escape", "..", "a\\b"} {
		if err := app.CreateFileFolder(id, "remote", "/", name); err == nil {
			t.Fatal("folder name escaped its parent")
		}
	}
	session, _ := app.fileSession(id)
	if err := session.client.Symlink("/", "/link"); err != nil {
		t.Fatal(err)
	}
	result, err := app.TransferSFTP(id, "link", "download", []string{"/link"}, t.TempDir(), false, "")
	if err == nil || result.Status == "completed" {
		t.Fatal("symbolic link was copied recursively")
	}
	app.DisconnectSession(id)
	if _, err := app.ListSFTPFiles(id, "/"); err == nil {
		t.Fatal("closed connection still browses")
	}
	resultConnection, err := app.ReconnectSFTP(id, "personal", host.ID, "", "", "")
	if err != nil || resultConnection.SessionID != id {
		t.Fatalf("SFTP reconnect did not reuse tab: %+v, %v", resultConnection, err)
	}
	if _, err := app.ListSFTPFiles(id, "/"); err != nil {
		t.Fatal(err)
	}
}

func TestSFTPDownloadReplacementAndDuplicateDestinations(t *testing.T) {
	app, _, id, _ := testSFTPConnection(t)
	first, second := t.TempDir(), t.TempDir()
	for _, directory := range []string{first, second} {
		if err := os.WriteFile(filepath.Join(directory, "same.txt"), []byte("remote contents"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	sources := []string{filepath.Join(first, "same.txt"), filepath.Join(second, "same.txt")}
	if _, err := app.TransferSFTP(id, "duplicates", "upload", sources, "/", true, ""); err == nil {
		t.Fatal("two sources were allowed to silently overwrite the same destination")
	}
	listing, err := app.ListSFTPFiles(id, "/")
	if err != nil || len(listing.Entries) != 0 {
		t.Fatal("duplicate destination check wrote files before failing")
	}
	if _, err := app.TransferSFTP(id, "upload", "upload", sources[:1], "/", false, ""); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	file := filepath.Join(target, "same.txt")
	if err := os.WriteFile(file, []byte("local original"), 0644); err != nil {
		t.Fatal(err)
	}
	result, err := app.TransferSFTP(id, "conflict", "download", []string{"/same.txt"}, target, false, "")
	if err != nil || result.Status != "conflict" {
		t.Fatalf("download did not ask before replacement: %+v, %v", result, err)
	}
	contents, _ := os.ReadFile(file)
	if string(contents) != "local original" {
		t.Fatal("download modified existing local file before confirmation")
	}
	result, err = app.TransferSFTP(id, "replace", "download", []string{"/same.txt"}, target, true, "")
	if err != nil || result.Status != "completed" {
		t.Fatalf("download replacement: %+v, %v", result, err)
	}
	contents, _ = os.ReadFile(file)
	if string(contents) != "remote contents" {
		t.Fatal("download replacement contents differ")
	}
}

func testSFTPPeer(t *testing.T, app *App) (Host, string) {
	t.Helper()
	_, remote, _, _ := testSFTPConnection(t)
	host, err := app.SaveHost(Host{
		Name: "Other file server", Address: remote.Address, Port: remote.Port,
		Username: "files-user", AuthMethod: "password",
	}, "sftp-password")
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := app.ConnectSFTP("personal", host.ID, "", "", "")
	if err != nil || challenge.Status != "host_key_unknown" {
		t.Fatalf("peer host-key challenge: %+v, %v", challenge, err)
	}
	result, err := app.ConnectSFTP("personal", host.ID, "", "", challenge.Fingerprint)
	if err != nil || result.Status != "connected" {
		t.Fatalf("peer connection: %+v, %v", result, err)
	}
	t.Cleanup(func() { app.DisconnectSession(result.SessionID) })
	return host, result.SessionID
}

func TestSFTPTransfersBetweenHosts(t *testing.T) {
	app, _, id, _ := testSFTPConnection(t)
	_, peerID := testSFTPPeer(t, app)
	local := t.TempDir()
	folder := filepath.Join(local, "project")
	if err := os.MkdirAll(filepath.Join(folder, "nested", "empty"), 0755); err != nil {
		t.Fatal(err)
	}
	contents := bytes.Repeat([]byte("host-to-host\x00"), 18000)
	if err := os.WriteFile(filepath.Join(folder, "nested", "data.bin"), contents, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.TransferSFTP(id, "seed", "upload", []string{folder}, "/", false, ""); err != nil {
		t.Fatal(err)
	}
	checkedReservation, sawCopyProgress := false, false
	app.eventSink = func(name string, data any) {
		if name != "files:progress" {
			return
		}
		progress := data.(FileTransferProgress)
		if progress.ID != "between" {
			return
		}
		sawCopyProgress = progress.Direction == "copy"
		if !checkedReservation {
			checkedReservation = true
			if _, err := app.TransferSFTP(peerID, "competing", "upload", []string{folder}, "/", false, id); err == nil {
				t.Error("a second transfer reused the active connections")
			}
		}
	}
	result, err := app.TransferSFTP(id, "between", "download", []string{"/project"}, "/", false, peerID)
	if err != nil || result.Status != "completed" || !checkedReservation || !sawCopyProgress {
		t.Fatalf("host-to-host transfer: %+v, %v", result, err)
	}
	if !bytes.Equal(remoteContents(t, app, peerID, "/project/nested/data.bin"), contents) {
		t.Fatal("peer contents differ")
	}
	listing, err := app.ListSFTPFiles(peerID, "/project/nested")
	if err != nil || len(listing.Entries) != 2 || !listing.Entries[0].Directory {
		t.Fatal("host-to-host copy lost an empty folder")
	}
	result, err = app.TransferSFTP(id, "conflict", "download", []string{"/project"}, "/", false, peerID)
	if err != nil || result.Status != "conflict" {
		t.Fatalf("host-to-host replacement did not ask: %+v, %v", result, err)
	}
	if err := app.CreateFileFolder(id, "remote", "/", "return"); err != nil {
		t.Fatal(err)
	}
	result, err = app.TransferSFTP(id, "reverse", "upload", []string{"/project"}, "/return", false, peerID)
	if err != nil || result.Status != "completed" || !bytes.Equal(remoteContents(t, app, id, "/return/project/nested/data.bin"), contents) {
		t.Fatalf("reverse host-to-host copy: %+v, %v", result, err)
	}
	result, err = app.TransferSFTP(id, "replace", "download", []string{"/project"}, "/", true, peerID)
	if err != nil || result.Status != "completed" {
		t.Fatalf("host-to-host replacement: %+v, %v", result, err)
	}
}

func TestSFTPPeerDisconnectCancelsAndPreservesDestination(t *testing.T) {
	app, _, id, _ := testSFTPConnection(t)
	host, peerID := testSFTPPeer(t, app)
	file := filepath.Join(t.TempDir(), "data.bin")
	if err := os.WriteFile(file, []byte("original"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.TransferSFTP(peerID, "original", "upload", []string{file}, "/", false, ""); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, bytes.Repeat([]byte("replacement"), 1024*1024), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := app.TransferSFTP(id, "seed", "upload", []string{file}, "/", false, ""); err != nil {
		t.Fatal(err)
	}
	closed := false
	app.eventSink = func(name string, data any) {
		if name != "files:progress" {
			return
		}
		progress := data.(FileTransferProgress)
		if progress.ID == "between" && progress.Copied > 0 && !closed {
			closed = true
			app.DisconnectSession(peerID)
		}
	}
	result, err := app.TransferSFTP(id, "between", "download", []string{"/data.bin"}, "/", true, peerID)
	if err != nil || result.Status != "cancelled" || !closed {
		t.Fatalf("peer disconnect did not cancel transfer: %+v, %v", result, err)
	}
	if _, err := app.ReconnectSFTP(peerID, "personal", host.ID, "", "", ""); err != nil {
		t.Fatal(err)
	}
	if string(remoteContents(t, app, peerID, "/data.bin")) != "original" {
		t.Fatal("disconnect replaced the existing destination")
	}
}

func TestSFTPSameServerRejectsCopyingFolderInsideItself(t *testing.T) {
	app, host, id, _ := testSFTPConnection(t)
	peer, err := app.ConnectSFTP("personal", host.ID, "", "", "")
	if err != nil || peer.Status != "connected" {
		t.Fatalf("second connection to same host: %+v, %v", peer, err)
	}
	t.Cleanup(func() { app.DisconnectSession(peer.SessionID) })
	if err := app.CreateFileFolder(id, "remote", "/", "folder"); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"/", "/folder"} {
		if _, err := app.TransferSFTP(id, "overlap", "download", []string{"/folder"}, target, true, peer.SessionID); err == nil {
			t.Fatalf("copy onto or inside the source was allowed: %s", target)
		}
	}
}
