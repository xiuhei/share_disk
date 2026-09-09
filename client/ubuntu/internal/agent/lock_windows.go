//go:build windows

package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// acquireLock uses a non-shared Windows file handle so only one Agent can own
// a storage root at a time. Closing the returned file releases the lock.
func acquireLock(storageRoot string) (*os.File, error) {
	if err := os.MkdirAll(storageRoot, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(storageRoot, ".agent.lock")
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(pathPtr, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_HIDDEN, 0)
	if err != nil {
		return nil, fmt.Errorf("another Share Disk client is already using this storage: %w", err)
	}
	return os.NewFile(uintptr(handle), path), nil
}
