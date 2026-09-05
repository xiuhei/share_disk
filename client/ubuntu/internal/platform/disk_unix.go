//go:build !windows

package platform

import (
	"fmt"
	"syscall"
)

// GetDiskSpace returns the available disk space in bytes for the given path.
func GetDiskSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, fmt.Errorf("failed to get disk space: %w", err)
	}

	// Bavail is the number of free blocks available to non-super user.
	// Bsize is the fundamental file system block size.
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}
