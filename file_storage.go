package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/pkg/sftp"
)

// FileStorage reports space available to the current user on a filesystem.
// Some SFTP servers do not implement the OpenSSH statvfs extension.
type FileStorage struct {
	Available bool   `json:"available"`
	Total     uint64 `json:"total"`
	Free      uint64 `json:"free"`
}

func (a *App) GetFileStorage(sessionID, directory string) (FileStorage, error) {
	if sessionID != "" {
		session, err := a.fileSession(sessionID)
		if err != nil {
			return FileStorage{}, err
		}
		if _, supported := session.client.HasExtension("statvfs@openssh.com"); !supported {
			return FileStorage{}, nil
		}
		stats, err := session.client.StatVFS(directory)
		if err != nil {
			var status *sftp.StatusError
			if errors.As(err, &status) && status.FxCode() == sftp.ErrSSHFxOpUnsupported {
				return FileStorage{}, nil
			}
			return FileStorage{}, err
		}
		blockSize := stats.Frsize
		if blockSize == 0 {
			blockSize = stats.Bsize
		}
		return FileStorage{Available: true, Total: stats.Blocks * blockSize, Free: stats.Bavail * blockSize}, nil
	}
	if directory == "" {
		var err error
		directory, err = os.UserHomeDir()
		if err != nil {
			return FileStorage{}, err
		}
	}
	directory, err := filepath.Abs(directory)
	if err != nil {
		return FileStorage{}, err
	}
	return localFileStorage(directory)
}
