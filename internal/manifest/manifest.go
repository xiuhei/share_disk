// Package manifest defines the transport-independent, canonical description of
// one immutable object. The same bytes are used for tickets, HTTP transfers and
// libp2p transfers so a transport cannot redefine object identity.
package manifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

const (
	Version1         uint16 = 1
	SHA256           byte   = 1
	headerSize              = 4 + 2 + 1 + 1 + 8 + 4 + 4 + sha256.Size
	encodedChunkSize        = 4 + 8 + 4 + sha256.Size
)

var magic = [4]byte{'S', 'D', 'M', 'F'}

// Chunk describes one contiguous object block.
type Chunk struct {
	Index  uint32
	Offset uint64
	Size   uint32
	Hash   [sha256.Size]byte
}

// Manifest is version 1 of the Share Disk object manifest.
type Manifest struct {
	Version    uint16
	ObjectSize uint64
	ChunkSize  uint32
	ObjectHash [sha256.Size]byte
	Chunks     []Chunk
}

// Validate enforces the canonical contiguous layout. Empty objects contain no
// chunks but still carry the SHA-256 hash of the empty byte sequence.
func (m Manifest) Validate() error {
	if m.Version != Version1 {
		return fmt.Errorf("unsupported manifest version %d", m.Version)
	}
	if m.ChunkSize == 0 {
		return fmt.Errorf("chunk size must be positive")
	}
	expectedCount := m.ObjectSize / uint64(m.ChunkSize)
	if m.ObjectSize%uint64(m.ChunkSize) != 0 {
		expectedCount++
	}
	if expectedCount > uint64(^uint32(0)) || uint64(len(m.Chunks)) != expectedCount {
		return fmt.Errorf("chunk count %d does not match object layout %d", len(m.Chunks), expectedCount)
	}
	for i, chunk := range m.Chunks {
		index := uint32(i)
		expectedOffset := uint64(index) * uint64(m.ChunkSize)
		expectedSize := uint64(m.ChunkSize)
		if remaining := m.ObjectSize - expectedOffset; remaining < expectedSize {
			expectedSize = remaining
		}
		if chunk.Index != index || chunk.Offset != expectedOffset || uint64(chunk.Size) != expectedSize {
			return fmt.Errorf("chunk %d is not canonical", i)
		}
	}
	return nil
}

// MarshalBinary returns the canonical network-order representation:
// magic, version, hash algorithm, reserved byte, object size, chunk size,
// chunk count, object hash, then index/offset/size/hash for each chunk.
func (m Manifest) MarshalBinary() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	output.Grow(headerSize + len(m.Chunks)*encodedChunkSize)
	output.Write(magic[:])
	_ = binary.Write(&output, binary.BigEndian, m.Version)
	output.WriteByte(SHA256)
	output.WriteByte(0)
	_ = binary.Write(&output, binary.BigEndian, m.ObjectSize)
	_ = binary.Write(&output, binary.BigEndian, m.ChunkSize)
	_ = binary.Write(&output, binary.BigEndian, uint32(len(m.Chunks)))
	output.Write(m.ObjectHash[:])
	for _, chunk := range m.Chunks {
		_ = binary.Write(&output, binary.BigEndian, chunk.Index)
		_ = binary.Write(&output, binary.BigEndian, chunk.Offset)
		_ = binary.Write(&output, binary.BigEndian, chunk.Size)
		output.Write(chunk.Hash[:])
	}
	return output.Bytes(), nil
}

// Digest returns SHA-256 over the canonical manifest bytes.
func (m Manifest) Digest() ([sha256.Size]byte, error) {
	encoded, err := m.MarshalBinary()
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	return sha256.Sum256(encoded), nil
}
