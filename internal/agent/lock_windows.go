//go:build windows

package agent

import (
	"os"
)

// acquireLock on Windows is a no-op; Windows named-pipe lock support is not
// yet implemented. The agent will start without a process lock.
func acquireLock(storageRoot string) (*os.File, error) {
	return nil, nil
}
