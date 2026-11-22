package main

import (
	"context"
	"net"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/pkg/sftp"
)

func TestLocalFileStorage(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("storage statistics are not implemented on this OS")
	}
	app := &App{}
	directory := t.TempDir()
	stats, err := app.GetFileStorage("", directory)
	if err != nil || !stats.Available || stats.Total == 0 || stats.Free > stats.Total {
		t.Fatalf("invalid local storage statistics: %+v, %v", stats, err)
	}
	if _, err := app.GetFileStorage("", filepath.Join(directory, "missing")); err == nil {
		t.Fatal("a missing directory must not report storage statistics")
	}
}

type storageFileCmder struct {
	sftp.FileCmder
	stats sftp.StatVFS
}

// Expose only the base command interface to simulate a server without statvfs.
type basicFileCmder struct{ sftp.FileCmder }

func (c storageFileCmder) StatVFS(*sftp.Request) (*sftp.StatVFS, error) {
	stats := c.stats
	return &stats, nil
}

func TestRemoteFileStorage(t *testing.T) {
	for _, test := range []struct {
		name  string
		stats *sftp.StatVFS
		want  FileStorage
	}{
		{"unsupported", nil, FileStorage{}},
		{"available blocks", &sftp.StatVFS{Bsize: 8192, Frsize: 4096, Blocks: 1000, Bfree: 500, Bavail: 400}, FileStorage{Available: true, Total: 4096000, Free: 1638400}},
		{"block size fallback", &sftp.StatVFS{Bsize: 4096, Blocks: 1000, Bavail: 400}, FileStorage{Available: true, Total: 4096000, Free: 1638400}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// A protocol-level pipe exercises SFTP without opening a network socket.
			clientPipe, serverPipe := net.Pipe()
			t.Cleanup(func() { clientPipe.Close(); serverPipe.Close() })
			handlers := sftp.InMemHandler()
			handlers.FileCmd = basicFileCmder{handlers.FileCmd}
			if test.stats != nil {
				handlers.FileCmd = storageFileCmder{FileCmder: handlers.FileCmd, stats: *test.stats}
			}
			server := sftp.NewRequestServer(serverPipe, handlers)
			go func() { _ = server.Serve() }()
			t.Cleanup(func() { server.Close() })
			client, err := sftp.NewClientPipe(clientPipe, clientPipe)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { client.Close() })
			app := &App{files: map[string]*fileSession{"files": {client: client, ctx: context.Background()}}}
			got, err := app.GetFileStorage("files", "/")
			if err != nil || got != test.want {
				t.Fatalf("remote storage: got %+v, want %+v, error %v", got, test.want, err)
			}
			if _, err := app.ListSFTPFiles("files", "/"); err != nil {
				t.Fatalf("storage lookup disrupted browsing: %v", err)
			}
			delete(app.files, "files")
			if _, err := app.GetFileStorage("files", "/"); err == nil {
				t.Fatal("closed sessions must not report local storage")
			}
		})
	}
}
