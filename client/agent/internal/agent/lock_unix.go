//go:build !windows

package agent

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// acquireLock acquires an exclusive non-blocking flock rooted in the storage
// directory, preventing concurrent agent instances from corrupting the shared
// SQLite state. The caller must close the returned file to release the lock.
func acquireLock(storageRoot string) (*os.File, error) {
	if err := os.MkdirAll(storageRoot, 0700); err != nil {
		return nil, fmt.Errorf("failed to create storage root for lock: %w", err)
	}
	lockPath := filepath.Join(storageRoot, ".agent.lock")

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another agent is already running (lock file: %s)", lockPath)
	}
	return f, nil
}
