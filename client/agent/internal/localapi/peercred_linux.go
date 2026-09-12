//go:build linux

package localapi

import (
	"fmt"
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// verifyPeerCredential rejects connections from processes owned by a different
// user, so a non-agent process on the same host cannot drive the local IPC. On
// Linux this uses SO_PEERCRED.
func verifyPeerCredential(conn net.Conn) error {
	uc, ok := conn.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("unexpected connection type: %T", conn)
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return fmt.Errorf("failed to access raw connection: %w", err)
	}

	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return fmt.Errorf("failed to inspect socket: %w", err)
	}
	if credErr != nil {
		return fmt.Errorf("failed to read peer credentials: %w", credErr)
	}
	if int(cred.Uid) != os.Getuid() {
		return fmt.Errorf("peer uid %d is not the agent uid %d", cred.Uid, os.Getuid())
	}
	return nil
}
