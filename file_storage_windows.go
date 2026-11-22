//go:build windows

package main

import "golang.org/x/sys/windows"

func localFileStorage(directory string) (FileStorage, error) {
	name, err := windows.UTF16PtrFromString(directory)
	if err != nil {
		return FileStorage{}, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(name, &free, &total, &totalFree); err != nil {
		return FileStorage{}, err
	}
	return FileStorage{Available: true, Total: total, Free: free}, nil
}
