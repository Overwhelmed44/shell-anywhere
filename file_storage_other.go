//go:build !windows && !linux && !darwin

package main

func localFileStorage(string) (FileStorage, error) {
	return FileStorage{}, nil
}
