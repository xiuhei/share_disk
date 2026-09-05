package localapi

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/protobuf/proto"

	sharediskv1 "github.com/share-disk/share-disk/proto/sharedisk/v1"
)

// maxFrameSize bounds a single IPC frame. Import/stat/status payloads are small;
// 4 MiB leaves headroom for a chunk manifest on very large objects.
const maxFrameSize = 4 * 1024 * 1024

// Handler processes a single LocalRequest and returns a LocalResponse.
type Handler interface {
	Handle(ctx context.Context, req *sharediskv1.LocalRequest) *sharediskv1.LocalResponse
}

// Server serves the local IPC protocol over a Unix domain socket. Only the
// agent process may create it; peer credentials are verified per connection.
type Server struct {
	socketPath string
	handler    Handler

	mu       sync.Mutex
	listener net.Listener
}

// NewServer creates a Server bound to socketPath.
func NewServer(socketPath string, h Handler) *Server {
	return &Server{socketPath: socketPath, handler: h}
}

// Listen creates the socket directory (0700), removes any stale socket, and
// binds a Unix listener restricted to 0600.
func (s *Server) Listen() error {
	dir := filepath.Dir(s.socketPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on socket: %w", err)
	}
	if err := os.Chmod(s.socketPath, 0600); err != nil {
		_ = ln.Close()
		return fmt.Errorf("failed to restrict socket permissions: %w", err)
	}

	s.mu.Lock()
	s.listener = ln
	s.mu.Unlock()
	return nil
}

// Serve accepts connections until ctx is cancelled, then closes the listener.
func (s *Server) Serve(ctx context.Context) error {
	s.mu.Lock()
	ln := s.listener
	s.mu.Unlock()
	if ln == nil {
		return fmt.Errorf("server is not listening")
	}

	go func() {
		<-ctx.Done()
		_ = s.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return fmt.Errorf("accept failed: %w", err)
			}
		}
		go s.handleConn(conn)
	}
}

// Close closes the listener and removes the socket file. It is idempotent and
// safe for concurrent use.
func (s *Server) Close() error {
	s.mu.Lock()
	ln := s.listener
	s.listener = nil
	s.mu.Unlock()

	if ln != nil {
		err := ln.Close()
		_ = os.Remove(s.socketPath)
		return err
	}
	return nil
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()

	if err := verifyPeerCredential(conn); err != nil {
		return
	}

	for {
		var req sharediskv1.LocalRequest
		if err := readMessage(conn, &req); err != nil {
			return
		}

		resp := s.handler.Handle(context.Background(), &req)
		if resp == nil {
			resp = &sharediskv1.LocalResponse{
				Payload: &sharediskv1.LocalResponse_Error{
					Error: &sharediskv1.LocalError{Code: "INTERNAL", Message: "empty response"},
				},
			}
		}
		if err := writeMessage(conn, resp); err != nil {
			return
		}
	}
}

// Client is a client for the local IPC protocol.
type Client struct {
	socketPath string
}

// NewClient creates a Client for the given socket path.
func NewClient(socketPath string) *Client {
	return &Client{socketPath: socketPath}
}

// Call performs a single request/response round trip over the socket.
func (c *Client) Call(ctx context.Context, req *sharediskv1.LocalRequest) (*sharediskv1.LocalResponse, error) {
	d := net.Dialer{}
	conn, err := d.DialContext(ctx, "unix", c.socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to agent: %w", err)
	}
	defer conn.Close()

	if err := writeMessage(conn, req); err != nil {
		return nil, err
	}

	var resp sharediskv1.LocalResponse
	if err := readMessage(conn, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// writeMessage writes a length-prefixed protobuf message, enforcing maxFrameSize.
func writeMessage(w io.Writer, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	if len(data) > maxFrameSize {
		return fmt.Errorf("message exceeds frame size limit: %d", len(data))
	}

	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(data)))
	if _, err := w.Write(prefix[:]); err != nil {
		return fmt.Errorf("failed to write frame prefix: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write frame body: %w", err)
	}
	return nil
}

// readMessage reads a length-prefixed protobuf message, rejecting oversized or
// truncated frames.
func readMessage(r io.Reader, msg proto.Message) error {
	var prefix [4]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return fmt.Errorf("failed to read frame prefix: %w", err)
	}

	size := binary.BigEndian.Uint32(prefix[:])
	if size > maxFrameSize {
		return fmt.Errorf("frame size %d exceeds limit %d", size, maxFrameSize)
	}

	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return fmt.Errorf("failed to read frame body: %w", err)
	}
	if err := proto.Unmarshal(data, msg); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}
	return nil
}
