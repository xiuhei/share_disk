package p2p

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"google.golang.org/protobuf/proto"

	sharediskv1 "github.com/share-disk/share-disk/proto/sharedisk/v1"
)

// TransferProtocolID is the protocol ID for object transfers.
const TransferProtocolID = protocol.ID("/sharedisk/object-transfer/1.0.0")

// ProtocolVersion is the supported transfer protocol version.
const ProtocolVersion = 1

// maxFrameSize bounds a single protocol frame. The largest frame is a ChunkData
// message carrying one 4 MiB chunk plus framing overhead.
const maxFrameSize = 4*1024*1024 + 64*1024

// TransferHandler authorizes incoming transfer requests. The Node delegates all
// business authorization (ticket validation, account/object/source/target
// binding) to this interface; the default is to reject.
type TransferHandler interface {
	// HandleOpen validates a TransferOpen and returns either an accept or a
	// reject. It is called with the authenticated remote peer ID.
	HandleOpen(ctx context.Context, remote peer.ID, open *sharediskv1.TransferOpen) (*sharediskv1.TransferAccept, *sharediskv1.TransferReject)
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
	addr, err := multiaddr.NewMultiaddr(listenAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse listen address: %w", err)
	}

	h, err := libp2p.New(libp2p.ListenAddrs(addr))
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

	if open.GetProtocolVersion() != ProtocolVersion {
		writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INVALID_TICKET, "unsupported protocol version")
		return
	}

	// Authorization is delegated to the application handler. Without a
	// handler we must fail closed rather than silently accept.
	if n.handler == nil {
		writeReject(stream, sharediskv1.RejectReason_REJECT_REASON_INVALID_TICKET, "authorization not configured")
		return
	}

	accept, reject := n.handler.HandleOpen(context.Background(), remote, open)
	if reject != nil {
		if err := writeMessage(stream, &sharediskv1.TransferMessage{
			Message: &sharediskv1.TransferMessage_Reject{Reject: reject},
		}); err != nil {
			log.Printf("Failed to send reject: %v", err)
		}
		return
	}

	if err := writeMessage(stream, &sharediskv1.TransferMessage{
		Message: &sharediskv1.TransferMessage_Accept{Accept: accept},
	}); err != nil {
		log.Printf("Failed to send accept: %v", err)
	}
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
