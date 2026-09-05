//go:build windows

package platform

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// GetDiskSpace returns the available disk space in bytes for the given path.
func GetDiskSpace(path string) (int64, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("failed to convert path: %w", err)
	}

	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return 0, fmt.Errorf("failed to get disk space: %w", err)
	}

	return int64(freeBytesAvailable), nil
}
