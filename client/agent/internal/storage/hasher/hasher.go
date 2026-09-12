package hasher

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
)

// Hasher computes SHA-256 hashes in a streaming fashion.
type Hasher struct {
	hasher hash.Hash
	size   int64
}

// New creates a new Hasher.
func New() *Hasher {
	return &Hasher{
		hasher: sha256.New(),
	}
}

// Write implements io.Writer.
func (h *Hasher) Write(p []byte) (n int, err error) {
	n, err = h.hasher.Write(p)
	h.size += int64(n)
	return n, err
}

// Size returns the total bytes written.
func (h *Hasher) Size() int64 {
	return h.size
}

// Sum returns the final hash and resets the hasher.
func (h *Hasher) Sum() Hash {
	var hash Hash
	copy(hash[:], h.hasher.Sum(nil))
	return hash
}

// Reset resets the hasher to its initial state.
func (h *Hasher) Reset() {
	h.hasher.Reset()
	h.size = 0
}

// Hash represents a SHA-256 hash.
type Hash [sha256.Size]byte

// String returns the hex-encoded hash string.
func (h Hash) String() string {
	return hex.EncodeToString(h[:])
}

// ParseHash parses a hex-encoded hash string.
func ParseHash(s string) (Hash, error) {
	var h Hash
	b, err := hex.DecodeString(s)
	if err != nil {
		return h, fmt.Errorf("invalid hash format: %w", err)
	}
	if len(b) != sha256.Size {
		return h, fmt.Errorf("invalid hash length: expected %d, got %d", sha256.Size, len(b))
	}
	copy(h[:], b)
	return h, nil
}

// HashReader reads from r and returns the hash and size.
func HashReader(r io.Reader) (Hash, int64, error) {
	h := New()
	if _, err := io.Copy(h, r); err != nil {
		return Hash{}, 0, fmt.Errorf("failed to hash data: %w", err)
	}
	return h.Sum(), h.Size(), nil
}

// ChunkHasher computes hashes for chunks of data.
type ChunkHasher struct {
	chunkSize int64
	chunks    []ChunkInfo
	current   *Hasher
	offset    int64
	index     int
}

// ChunkInfo represents information about a chunk.
type ChunkInfo struct {
	Index  int   `json:"index"`
	Offset int64 `json:"offset"`
	Size   int64 `json:"size"`
	Hash   Hash  `json:"hash"`
}

// NewChunkHasher creates a new ChunkHasher with the given chunk size.
// Chunk size must be positive.
func NewChunkHasher(chunkSize int64) *ChunkHasher {
	if chunkSize <= 0 {
		chunkSize = 4 * 1024 * 1024 // Default to 4 MiB
	}
	return &ChunkHasher{
		chunkSize: chunkSize,
		chunks:    make([]ChunkInfo, 0),
		current:   New(),
	}
}

// Write implements io.Writer. It splits the data into chunks and computes hashes.
func (ch *ChunkHasher) Write(p []byte) (n int, err error) {
	totalWritten := 0

	for len(p) > 0 {
		// Calculate remaining space in current chunk
		remaining := ch.chunkSize - (ch.offset % ch.chunkSize)

		// Determine how much to write to current chunk
		toWrite := int64(len(p))
		if toWrite > remaining {
			toWrite = remaining
		}

		// Write to current chunk hasher
		written, err := ch.current.Write(p[:toWrite])
		if err != nil {
			return totalWritten, err
		}

		totalWritten += written
		ch.offset += int64(written)
		p = p[written:]

		// Check if chunk is complete
		if ch.offset%ch.chunkSize == 0 {
			ch.finishChunk()
		}
	}

	return totalWritten, nil
}

// finishChunk completes the current chunk and starts a new one.
func (ch *ChunkHasher) finishChunk() {
	hash := ch.current.Sum()
	ch.chunks = append(ch.chunks, ChunkInfo{
		Index:  ch.index,
		Offset: int64(ch.index) * ch.chunkSize,
		Size:   ch.current.Size(),
		Hash:   hash,
	})
	ch.index++
	ch.current.Reset()
}

// Finish completes the hashing and returns all chunk information.
func (ch *ChunkHasher) Finish() []ChunkInfo {
	// Finish the last chunk if it has data
	if ch.current.Size() > 0 {
		ch.finishChunk()
	}
	return ch.chunks
}

// ChunkCount returns the number of chunks.
func (ch *ChunkHasher) ChunkCount() int {
	return len(ch.chunks)
}

// TotalSize returns the total size of all data written.
func (ch *ChunkHasher) TotalSize() int64 {
	return ch.offset
}

// Manifest represents a file manifest with chunk hashes.
type Manifest struct {
	Hash       Hash        `json:"hash"`
	Size       int64       `json:"size"`
	ChunkSize  int64       `json:"chunk_size"`
	ChunkCount int         `json:"chunk_count"`
	Chunks     []ChunkInfo `json:"chunks"`
}

// ComputeManifest computes a manifest for the given reader.
func ComputeManifest(r io.Reader, chunkSize int64) (*Manifest, error) {
	chunker := NewChunkHasher(chunkSize)

	// We need to compute both the overall hash and chunk hashes
	// So we'll use a multi-writer approach
	overallHasher := New()
	multiWriter := io.MultiWriter(chunker, overallHasher)

	if _, err := io.Copy(multiWriter, r); err != nil {
		return nil, fmt.Errorf("failed to compute manifest: %w", err)
	}

	chunks := chunker.Finish()

	return &Manifest{
		Hash:       overallHasher.Sum(),
		Size:       chunker.TotalSize(),
		ChunkSize:  chunkSize,
		ChunkCount: len(chunks),
		Chunks:     chunks,
	}, nil
}
