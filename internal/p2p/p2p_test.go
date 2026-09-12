package p2p

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	sharediskv1 "github.com/share-disk/share-disk/contracts/proto/sharedisk/v1"
)

func TestLoadOrCreateIdentityIsStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.key")
	first, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateIdentity(path)
	if err != nil {
		t.Fatal(err)
	}
	firstID, err := peer.IDFromPrivateKey(first)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := peer.IDFromPrivateKey(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstID != secondID {
		t.Fatalf("peer identity changed across reload: %s != %s", firstID, secondID)
	}
}

func TestDownloadResumesVerifiedChunksAfterInterruption(t *testing.T) {
	chunks := map[uint32][]byte{
		0: []byte("abcd"),
		1: []byte("efgh"),
		2: []byte("ijkl"),
	}
	handler := &memoryHandler{chunks: chunks, reads: make(map[uint32]int)}
	source := newTestNode(t, "source", handler)
	target := newTestNode(t, "target", nil)
	connectNodes(t, target, source)

	sink := &memorySink{chunks: make(map[uint32][]byte), failOnceAt: 1}
	open := &sharediskv1.TransferOpen{ProtocolVersion: ProtocolVersion, TaskId: "task-1", Ticket: []byte("authorized-by-test"), ObjectHash: make([]byte, sha256.Size), ManifestDigest: make([]byte, sha256.Size)}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := target.Download(ctx, source.GetHostID(), open, sink); err == nil {
		t.Fatal("first download unexpectedly completed")
	}
	if !sink.HasChunk(0) || sink.HasChunk(1) {
		t.Fatalf("unexpected durable progress after interruption: %#v", sink.chunks)
	}
	if _, err := target.Download(ctx, source.GetHostID(), open, sink); err != nil {
		t.Fatalf("resume download: %v", err)
	}
	if got := string(sink.join()); got != "abcdefghijkl" {
		t.Fatalf("downloaded content = %q", got)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if handler.reads[0] != 1 || handler.reads[1] != 2 || handler.reads[2] != 1 {
		t.Fatalf("unexpected source reads: %#v", handler.reads)
	}
}

func TestDownloadRejectsUnavailableRequestedChunk(t *testing.T) {
	accept := []uint32{0}
	if _, err := requestedIndices([]uint32{1}, accept); err == nil {
		t.Fatal("unavailable chunk was accepted")
	}
}

func TestDownloadRejectsManifestMismatch(t *testing.T) {
	digest := make([]byte, sha256.Size)
	digest[0] = 1
	handler := &memoryHandler{
		chunks: map[uint32][]byte{0: []byte("data")},
		reads:  make(map[uint32]int),
		accept: &sharediskv1.TransferAccept{
			ChunkSize: 4, TotalSize: 4, ManifestDigest: digest, AvailableChunks: []uint32{0},
		},
	}
	source := newTestNode(t, "source", handler)
	target := newTestNode(t, "target", nil)
	connectNodes(t, target, source)
	open := &sharediskv1.TransferOpen{ProtocolVersion: ProtocolVersion, TaskId: "task-2", Ticket: []byte("authorized-by-test"), ObjectHash: make([]byte, sha256.Size), ManifestDigest: make([]byte, sha256.Size)}
	sink := &memorySink{chunks: make(map[uint32][]byte)}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := target.Download(ctx, source.GetHostID(), open, sink); err == nil {
		t.Fatal("mismatched manifest was accepted")
	}
	if sink.HasChunk(0) {
		t.Fatal("chunk was persisted before manifest validation")
	}
}

func newTestNode(t *testing.T, deviceID string, handler TransferHandler) *Node {
	t.Helper()
	privateKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	node, err := NewNodeWithIdentity("user-1", deviceID, "/ip4/127.0.0.1/tcp/0", handler, privateKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = node.Close() })
	return node
}

func connectNodes(t *testing.T, target, source *Node) {
	t.Helper()
	addresses := source.GetAddresses()
	if len(addresses) == 0 {
		t.Fatal("source has no listen address")
	}
	peerComponent, err := multiaddr.NewMultiaddr("/p2p/" + source.GetHostID().String())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := target.Connect(ctx, addresses[0].Encapsulate(peerComponent).String()); err != nil {
		t.Fatal(err)
	}
}

type memoryHandler struct {
	mu     sync.Mutex
	chunks map[uint32][]byte
	reads  map[uint32]int
	accept *sharediskv1.TransferAccept
}

func (h *memoryHandler) HandleOpen(_ context.Context, _ peer.ID, open *sharediskv1.TransferOpen) (*sharediskv1.TransferAccept, *sharediskv1.TransferReject) {
	if open.GetTaskId() == "" || len(open.GetTicket()) == 0 {
		return nil, &sharediskv1.TransferReject{Reason: sharediskv1.RejectReason_REJECT_REASON_INVALID_TICKET}
	}
	if h.accept != nil {
		return h.accept, nil
	}
	return &sharediskv1.TransferAccept{ChunkSize: 4, TotalSize: 12, ManifestDigest: make([]byte, sha256.Size), AvailableChunks: []uint32{0, 1, 2}}, nil
}

func (h *memoryHandler) ReadChunk(_ context.Context, _ peer.ID, _ *sharediskv1.TransferOpen, index uint32) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	chunk, ok := h.chunks[index]
	if !ok {
		return nil, fmt.Errorf("chunk %d not found", index)
	}
	h.reads[index]++
	return append([]byte(nil), chunk...), nil
}

type memorySink struct {
	mu         sync.Mutex
	chunks     map[uint32][]byte
	failOnceAt uint32
	failed     bool
}

func (s *memorySink) HasChunk(index uint32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.chunks[index]
	return ok
}

func (s *memorySink) WriteChunk(_ context.Context, index uint32, _ uint64, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index == s.failOnceAt && !s.failed {
		s.failed = true
		return errors.New("simulated durable write interruption")
	}
	s.chunks[index] = append([]byte(nil), data...)
	return nil
}

func (s *memorySink) join() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []byte
	for index := uint32(0); index < uint32(len(s.chunks)); index++ {
		result = append(result, s.chunks[index]...)
	}
	return result
}
