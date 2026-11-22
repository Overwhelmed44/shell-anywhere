//go:build linux || darwin

package main

import "golang.org/x/sys/unix"

func localFileStorage(directory string) (FileStorage, error) {
	var stats unix.Statfs_t
	if err := unix.Statfs(directory, &stats); err != nil {
		return FileStorage{}, err
	}
	blockSize := uint64(stats.Bsize)
	return FileStorage{Available: true, Total: stats.Blocks * blockSize, Free: stats.Bavail * blockSize}, nil
}
