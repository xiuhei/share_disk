//go:build !linux

package localapi

import "net"

// verifyPeerCredential is a no-op on platforms without SO_PEERCRED. Socket
// permissions (0600, 0700 parent directory) still restrict access to the
// agent's user. Windows named-pipe support is tracked separately.
func verifyPeerCredential(conn net.Conn) error {
	return nil
}
