package p2p

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"

	sharediskv1 "github.com/share-disk/share-disk/contracts/proto/sharedisk/v1"
)

// TransferProtocolID is the protocol ID for object transfers.
const TransferProtocolID = protocol.ID("/sharedisk/object-transfer/1.0.0")

// ProtocolVersion is the supported transfer protocol version.
const ProtocolVersion = 1

// maxFrameSize bounds a single protocol frame. The largest frame is a ChunkData
// message carrying one 4 MiB chunk plus framing overhead.
const maxFrameSize = 4*1024*1024 + 64*1024

const streamIdleTimeout = 30 * time.Second

// TransferHandler authorizes incoming transfer requests. The Node delegates all
// business authorization (ticket validation, account/object/source/target
// binding) to this interface; the default is to reject.
type TransferHandler interface {
	// HandleOpen validates a TransferOpen and returns either an accept or a
	// reject. It is called with the authenticated remote peer ID.
	HandleOpen(ctx context.Context, remote peer.ID, open *sharediskv1.TransferOpen) (*sharediskv1.TransferAccept, *sharediskv1.TransferReject)
}

// ChunkProvider supplies verified object chunks after HandleOpen authorizes a
// transfer. Keeping this separate preserves the authorization boundary while
// allowing handlers that only implement the handshake to fail closed for data.
type ChunkProvider interface {
	ReadChunk(ctx context.Context, remote peer.ID, open *sharediskv1.TransferOpen, index uint32) ([]byte, error)
}

// ChunkSink is the target-side durable progress boundary. HasChunk is checked
// before requesting data, and WriteChunk must return only after the chunk is
// verified and durable enough to survive a reconnect.
type ChunkSink interface {
	HasChunk(index uint32) bool
	WriteChunk(ctx context.Context, index uint32, offset uint64, data []byte) error
}

// Node represents a P2P node.
type Node struct {
	host     host.Host
	userID   string
	deviceID string
	handler  TransferHandler
	mu       sync.RWMutex
	peers    map[peer.ID]*PeerInfo
}

// PeerInfo represents information about a peer.
type PeerInfo struct {
	ID       peer.ID `json:"id"`
	UserID   string  `json:"user_id"`
	DeviceID string  `json:"device_id"`
}

// NewNode creates a new P2P node.
func NewNode(userID, deviceID string, listenAddr string) (*Node, error) {
	return NewNodeWithHandler(userID, deviceID, listenAddr, nil)
}

// NewNodeWithHandler creates a new P2P node with an explicit transfer handler.
func NewNodeWithHandler(userID, deviceID string, listenAddr string, handler TransferHandler) (*Node, error) {
	return newNode(userID, deviceID, listenAddr, handler, nil)
}

// NewNodeWithIdentity creates a node with a persistent private key. Production
// agents must use this constructor so a restart cannot silently change PeerID.
func NewNodeWithIdentity(userID, deviceID, listenAddr string, handler TransferHandler, privateKey crypto.PrivKey) (*Node, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("private key is required")
	}
	return newNode(userID, deviceID, listenAddr, handler, privateKey)
}

func newNode(userID, deviceID, listenAddr string, handler TransferHandler, privateKey crypto.PrivKey) (*Node, error) {
	addr, err := multiaddr.NewMultiaddr(listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse listen address: %w", err)
	}

	options := []libp2p.Option{libp2p.ListenAddrs(addr)}
	if privateKey != nil {
		options = append(options, libp2p.Identity(privateKey))
	}
	h, err := libp2p.New(options...)
	if err != nil {
		return nil, fmt.Errorf("failed to create host: %w", err)
	}

	node := &Node{
		host:     h,
		userID:   userID,
		deviceID: deviceID,
		handler:  handler,
		peers:    make(map[peer.ID]*PeerInfo),
	}

	h.SetStreamHandler(TransferProtocolID, node.handleStream)

	return node, nil
}

// LoadOrCreateIdentity returns the private key stored at path, creating it with
// owner-only permissions when absent. Platform launchers may replace this file
// adapter with an OS credential-store implementation without changing Node.
func LoadOrCreateIdentity(path string) (crypto.PrivKey, error) {
	if path == "" {
		return nil, fmt.Errorf("identity path is required")
	}
	encoded, err := os.ReadFile(path)
	if err == nil {
		key, unmarshalErr := crypto.UnmarshalPrivateKey(encoded)
		if unmarshalErr != nil {
			return nil, fmt.Errorf("decode peer identity: %w", unmarshalErr)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read peer identity: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create peer identity directory: %w", err)
	}
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate peer identity: %w", err)
	}
	encoded, err = crypto.MarshalPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("encode peer identity: %w", err)
	}
	identityFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if os.IsExist(err) {
			return LoadOrCreateIdentity(path)
		}
		return nil, fmt.Errorf("create peer identity: %w", err)
	}
	if _, err := identityFile.Write(encoded); err != nil {
		_ = identityFile.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write peer identity: %w", err)
	}
	if err := identityFile.Sync(); err != nil {
		_ = identityFile.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("sync peer identity: %w", err)
	}
	if err := identityFile.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close peer identity: %w", err)
	}
	return key, nil
}

// Start starts the P2P node.
func (n *Node) Start(ctx context.Context) error {
	log.Printf("P2P node started with ID: %s", n.host.ID())
	log.Printf("Listening on: %s", n.host.Addrs())

	<-ctx.Done()
	return n.host.Close()
}

// Connect connects to a peer.
func (n *Node) Connect(ctx context.Context, peerAddr string) error {
	addr, err := multiaddr.NewMultiaddr(peerAddr)
	if err != nil {
		return fmt.Errorf("failed to parse peer address: %w", err)
	}

	peerInfo, err := peer.AddrInfoFromP2pAddr(addr)
	if err != nil {
		return fmt.Errorf("failed to extract peer info: %w", err)
	}

	if err := n.host.Connect(ctx, *peerInfo); err != nil {
		return fmt.Errorf("failed to connect to peer: %w", err)
	}

	n.mu.Lock()
	n.peers[peerInfo.ID] = &PeerInfo{ID: peerInfo.ID}
	n.mu.Unlock()

	return nil
}

// SendOpen opens a transfer handshake with a peer and returns the source's
// accept or reject decision.
func (n *Node) SendOpen(ctx context.Context, peerID peer.ID, open *sharediskv1.TransferOpen) (*sharediskv1.TransferAccept, *sharediskv1.TransferReject, error) {
	stream, err := n.host.NewStream(ctx, peerID, TransferProtocolID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	if err := writeMessage(stream, &sharediskv1.TransferMessage{
		Message: &sharediskv1.TransferMessage_Open{Open: open},
	}); err != nil {
		return nil, nil, fmt.Errorf("failed to send request: %w", err)
	}

	var resp sharediskv1.TransferMessage
	if err := readMessage(stream, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to read response: %w", err)
	}

	switch m := resp.Message.(type) {
	case *sharediskv1.TransferMessage_Accept:
		return m.Accept, nil, nil
	case *sharediskv1.TransferMessage_Reject:
		return nil, m.Reject, nil
	default:
		return nil, nil, fmt.Errorf("unexpected handshake response")
	}
}

// Download opens an authorized stream and transfers all requested available
// chunks into sink. Repeating the call resumes because already durable chunks
// are skipped by HasChunk.
func (n *Node) Download(ctx context.Context, peerID peer.ID, open *sharediskv1.TransferOpen, sink ChunkSink) (*sharediskv1.TransferAccept, error) {
	if open == nil || sink == nil {
		return nil, fmt.Errorf("open request and chunk sink are required")
	}
	if err := validateOpen(open); err != nil {
		return nil, err
	}
	stream, err := n.host.NewStream(ctx, peerID, TransferProtocolID)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()
	stopCancel := context.AfterFunc(ctx, func() { _ = stream.Reset() })
	defer stopCancel()
	if err := refreshDeadline(stream); err != nil {
		return nil, err
	}
	if err := writeMessage(stream, &sharediskv1.TransferMessage{Message: &sharediskv1.TransferMessage_Open{Open: open}}); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	var response sharediskv1.TransferMessage
	if err := readMessage(stream, &response); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if rejected, ok := response.Message.(*sharediskv1.TransferMessage_Reject); ok {
		return nil, fmt.Errorf("transfer rejected (%s): %s", rejected.Reject.GetReason(), rejected.Reject.GetMessage())
	}
	accepted, ok := response.Message.(*sharediskv1.TransferMessage_Accept)
	if !ok || accepted.Accept == nil {
		return nil, fmt.Errorf("unexpected handshake response")
	}
	accept := accepted.Accept
	if err := validateAccept(accept); err != nil {
		return nil, err
	}
	if !equalBytes(open.GetManifestDigest(), accept.GetManifestDigest()) {
		return nil, fmt.Errorf("accepted manifest does not match transfer plan")
	}
	indices, err := requestedIndices(open.GetRequestedChunks(), accept.GetAvailableChunks())
	if err != nil {
		return nil, err
	}
	for _, index := range indices {
		if sink.HasChunk(index) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := refreshDeadline(stream); err != nil {
			return nil, err
		}
		if err := writeMessage(stream, &sharediskv1.TransferMessage{Message: &sharediskv1.TransferMessage_ChunkRequest{ChunkRequest: &sharediskv1.ChunkRequest{Index: index}}}); err != nil {
			return nil, fmt.Errorf("request chunk %d: %w", index, err)
		}
		var chunkMessage sharediskv1.TransferMessage
		if err := readMessage(stream, &chunkMessage); err != nil {
			return nil, fmt.Errorf("read chunk %d: %w", index, err)
		}
		chunk, ok := chunkMessage.Message.(*sharediskv1.TransferMessage_ChunkData)
		if !ok || chunk.ChunkData == nil {
			return nil, fmt.Errorf("unexpected response for chunk %d", index)
		}
		data := chunk.ChunkData
		if data.GetIndex() != index || data.GetOffset() != uint64(index)*uint64(accept.GetChunkSize()) {
			return nil, fmt.Errorf("invalid metadata for chunk %d", index)
		}
		expectedLength := int(accept.GetChunkSize())
		if remaining := accept.GetTotalSize() - data.GetOffset(); remaining < uint64(expectedLength) {
			expectedLength = int(remaining)
		}
		if len(data.GetBytes()) != expectedLength {
			return nil, fmt.Errorf("chunk %d has length %d, expected %d", index, len(data.GetBytes()), expectedLength)
		}
		hash := sha256.Sum256(data.GetBytes())
		if len(data.GetChunkHash()) != sha256.Size || !equalBytes(hash[:], data.GetChunkHash()) {
			return nil, fmt.Errorf("chunk %d hash mismatch", index)
		}
		if err := sink.WriteChunk(ctx, index, data.GetOffset(), data.GetBytes()); err != nil {
			_ = stream.Reset()
			return nil, fmt.Errorf("persist chunk %d: %w", index, err)
		}
		if err := writeMessage(stream, &sharediskv1.TransferMessage{Message: &sharediskv1.TransferMessage_ChunkAck{ChunkAck: &sharediskv1.ChunkAck{Index: index}}}); err != nil {
			return nil, fmt.Errorf("ack chunk %d: %w", index, err)
		}
	}
	if err := writeMessage(stream, &sharediskv1.TransferMessage{Message: &sharediskv1.TransferMessage_Close{Close: &sharediskv1.TransferClose{Status: sharediskv1.TransferStatus_TRANSFER_STATUS_COMPLETED}}}); err != nil {
		return nil, fmt.Errorf("close transfer: %w", err)
	}
	return accept, nil
}

func requestedIndices(requested, available []uint32) ([]uint32, error) {
	availableSet := make(map[uint32]struct{}, len(available))
	for _, index := range available {
		availableSet[index] = struct{}{}
	}
	if len(requested) == 0 {
		result := make([]uint32, len(available))
		copy(result, available)
		return result, nil
	}
	result := make([]uint32, 0, len(requested))
	seen := make(map[uint32]struct{}, len(requested))
	for _, index := range requested {
		if _, ok := availableSet[index]; !ok {
			return nil, fmt.Errorf("requested chunk %d is unavailable", index)
		}
		if _, duplicate := seen[index]; duplicate {
			continue
		}
		seen[index] = struct{}{}
		result = append(result, index)
	}
	return result, nil
}

func validateOpen(open *sharediskv1.TransferOpen) error {
	if open.GetProtocolVersion() != ProtocolVersion {
		return fmt.Errorf("unsupported protocol version: %d", open.GetProtocolVersion())
	}
	if open.GetTaskId() == "" || len(open.GetTicket()) == 0 {
		return fmt.Errorf("task id and transfer ticket are required")
	}
	if len(open.GetObjectHash()) != sha256.Size {
		return fmt.Errorf("object hash must be SHA-256")
	}
	if len(open.GetManifestDigest()) != sha256.Size {
		return fmt.Errorf("manifest digest must be SHA-256")
	}
	return nil
}

func validateAccept(accept *sharediskv1.TransferAccept) error {
	if accept == nil {
		return fmt.Errorf("missing transfer acceptance")
	}
	if accept.GetChunkSize() == 0 || uint64(accept.GetChunkSize()) > uint64(maxFrameSize) {
		return fmt.Errorf("invalid accepted chunk size: %d", accept.GetChunkSize())
	}
	if len(accept.GetManifestDigest()) != sha256.Size {
		return fmt.Errorf("manifest digest must be SHA-256")
	}
	chunkSize := uint64(accept.GetChunkSize())
	chunkCount := accept.GetTotalSize() / chunkSize
	if accept.GetTotalSize()%chunkSize != 0 {
		chunkCount++
	}
	seen := make(map[uint32]struct{}, len(accept.GetAvailableChunks()))
	for _, index := range accept.GetAvailableChunks() {
		if uint64(index) >= chunkCount {
			return fmt.Errorf("available chunk %d exceeds object bounds", index)
		}
		if _, duplicate := seen[index]; duplicate {
			return fmt.Errorf("available chunk %d is duplicated", index)
		}
		seen[index] = struct{}{}
	}
	return nil
}

// handleStream handles incoming streams.
func (n *Node) handleStream(stream network.Stream) {
	defer stream.Close()

	remote := stream.Conn().RemotePeer()

	var msg sharediskv1.TransferMessage
	if err := readMessage(stream, &msg); err != nil {
		log.Printf("Failed to read message: %v", err)
		return
	}

	switch m := msg.Message.(type) {
	case *sharediskv1.TransferMessage_Open:
		n.handleOpen(stream, remote, m.Open)
	default:
		log.Printf("Unexpected message type on handshake stream: %T", msg.Message)
		writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, "unexpected message")
	}
}

// handleOpen validates and responds to a TransferOpen.
func (n *Node) handleOpen(stream network.Stream, remote peer.ID, open *sharediskv1.TransferOpen) {
	if open == nil {
		writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INVALID_TICKET, "missing open message")
		return
	}

	if err := validateOpen(open); err != nil {
		writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INVALID_TICKET, err.Error())
		return
	}

	// Authorization is delegated to the application handler. Without a
	// handler we must fail closed rather than silently accept.
	if n.handler == nil {
		writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INVALID_TICKET, "authorization not configured")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	accept, reject := n.handler.HandleOpen(ctx, remote, open)
	if reject != nil {
		if err := writeMessage(stream, &sharediskv1.TransferMessage{
			Message: &sharediskv1.TransferMessage_Reject{Reject: reject},
		}); err != nil {
			log.Printf("Failed to send reject: %v", err)
		}
		return
	}
	if err := validateAccept(accept); err != nil {
		writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, err.Error())
		return
	}

	if err := writeMessage(stream, &sharediskv1.TransferMessage{
		Message: &sharediskv1.TransferMessage_Accept{Accept: accept},
	}); err != nil {
		log.Printf("Failed to send accept: %v", err)
		return
	}
	provider, ok := n.handler.(ChunkProvider)
	if !ok {
		return
	}
	n.serveChunks(ctx, stream, remote, open, accept, provider)
}

func (n *Node) serveChunks(ctx context.Context, stream network.Stream, remote peer.ID, open *sharediskv1.TransferOpen, accept *sharediskv1.TransferAccept, provider ChunkProvider) {
	available := make(map[uint32]struct{}, len(accept.GetAvailableChunks()))
	for _, index := range accept.GetAvailableChunks() {
		available[index] = struct{}{}
	}
	for {
		if err := refreshDeadline(stream); err != nil {
			return
		}
		var request sharediskv1.TransferMessage
		if err := readMessage(stream, &request); err != nil {
			if err != io.EOF {
				log.Printf("Failed to read transfer message: %v", err)
			}
			return
		}
		switch message := request.Message.(type) {
		case *sharediskv1.TransferMessage_Close:
			return
		case *sharediskv1.TransferMessage_ChunkRequest:
			index := message.ChunkRequest.GetIndex()
			if _, ok := available[index]; !ok {
				writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_OBJECT_NOT_FOUND, "requested chunk unavailable")
				return
			}
			chunk, err := provider.ReadChunk(ctx, remote, open, index)
			expectedLength := int(accept.GetChunkSize())
			offset := uint64(index) * uint64(accept.GetChunkSize())
			if remaining := accept.GetTotalSize() - offset; remaining < uint64(expectedLength) {
				expectedLength = int(remaining)
			}
			if err != nil || len(chunk) != expectedLength {
				writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, "cannot read requested chunk")
				return
			}
			hash := sha256.Sum256(chunk)
			data := &sharediskv1.ChunkData{Index: index, Offset: offset, Bytes: chunk, ChunkHash: hash[:]}
			if err := writeMessage(stream, &sharediskv1.TransferMessage{Message: &sharediskv1.TransferMessage_ChunkData{ChunkData: data}}); err != nil {
				return
			}
			var ack sharediskv1.TransferMessage
			if err := readMessage(stream, &ack); err != nil {
				return
			}
			chunkAck, ok := ack.Message.(*sharediskv1.TransferMessage_ChunkAck)
			if !ok || chunkAck.ChunkAck.GetIndex() != index {
				writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INVALID_HASH, "invalid chunk acknowledgement")
				return
			}
		default:
			writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INTERNAL_ERROR, "unexpected transfer message")
			return
		}
	}
}

func refreshDeadline(stream network.Stream) error {
	if err := stream.SetDeadline(time.Now().Add(streamIdleTimeout)); err != nil {
		return fmt.Errorf("set stream deadline: %w", err)
	}
	return nil
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var different byte
	for index := range a {
		different |= a[index] ^ b[index]
	}
	return different == 0
}

func writeReject(stream network.Stream, reason sharediskv1.RejectReason, message string) {
	if err := writeMessage(stream, &sharediskv1.TransferMessage{
		Message: &sharediskv1.TransferMessage_Reject{
			Reject: &sharediskv1.TransferReject{Reason: reason, Message: message},
		},
	}); err != nil {
		log.Printf("Failed to send reject: %v", err)
	}
}

// GetHostID returns the host ID.
func (n *Node) GetHostID() peer.ID {
	return n.host.ID()
}

// GetAddresses returns the host addresses.
func (n *Node) GetAddresses() []multiaddr.Multiaddr {
	return n.host.Addrs()
}

// Close releases all listeners and active streams owned by the node.
func (n *Node) Close() error { return n.host.Close() }

// writeMessage writes a protobuf message to a stream using a 4-byte big-endian
// length prefix and an explicit frame size limit.
func writeMessage(stream network.Stream, msg proto.Message) error {
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	if len(data) > maxFrameSize {
		return fmt.Errorf("message exceeds frame size limit: %d", len(data))
	}

	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(data)))

	if _, err := stream.Write(prefix[:]); err != nil {
		return fmt.Errorf("failed to write frame prefix: %w", err)
	}
	if _, err := stream.Write(data); err != nil {
		return fmt.Errorf("failed to write frame body: %w", err)
	}
	return nil
}

// readMessage reads a length-prefixed protobuf message from a stream, enforcing
// a maximum frame size and rejecting oversized or truncated frames.
func readMessage(stream network.Stream, msg proto.Message) error {
	var prefix [4]byte
	if _, err := io.ReadFull(stream, prefix[:]); err != nil {
		return fmt.Errorf("failed to read frame prefix: %w", err)
	}

	size := binary.BigEndian.Uint32(prefix[:])
	if size == 0 {
		return fmt.Errorf("empty frame")
	}
	if size > maxFrameSize {
		return fmt.Errorf("frame size %d exceeds limit %d", size, maxFrameSize)
	}

	data := make([]byte, size)
	if _, err := io.ReadFull(stream, data); err != nil {
		return fmt.Errorf("failed to read frame body: %w", err)
	}

	if err := proto.Unmarshal(data, msg); err != nil {
		return fmt.Errorf("failed to unmarshal message: %w", err)
	}

	return nil
}
