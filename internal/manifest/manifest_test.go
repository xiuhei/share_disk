package manifest

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestVersion1Vector(t *testing.T) {
	type vectorChunk struct {
		Index  uint32 `json:"index"`
		Offset uint64 `json:"offset"`
		Size   uint32 `json:"size"`
		Hash   string `json:"hash"`
	}
	type vectorFile struct {
		Version        uint16        `json:"version"`
		ObjectSize     uint64        `json:"object_size"`
		ChunkSize      uint32        `json:"chunk_size"`
		ObjectHash     string        `json:"object_hash"`
		Chunks         []vectorChunk `json:"chunks"`
		CanonicalHex   string        `json:"canonical_hex"`
		ManifestDigest string        `json:"manifest_digest"`
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "contracts", "testdata", "manifest-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var vector vectorFile
	if err := json.Unmarshal(data, &vector); err != nil {
		t.Fatal(err)
	}
	value := Manifest{
		Version: vector.Version, ObjectSize: vector.ObjectSize, ChunkSize: vector.ChunkSize,
		Chunks: make([]Chunk, len(vector.Chunks)),
	}
	decodeHash(t, value.ObjectHash[:], vector.ObjectHash)
	for i, chunk := range vector.Chunks {
		value.Chunks[i] = Chunk{Index: chunk.Index, Offset: chunk.Offset, Size: chunk.Size}
		decodeHash(t, value.Chunks[i].Hash[:], chunk.Hash)
	}
	encoded, err := value.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(encoded); got != vector.CanonicalHex {
		t.Fatalf("canonical encoding changed\n got: %s\nwant: %s", got, vector.CanonicalHex)
	}
	digest, err := value.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(digest[:]); got != vector.ManifestDigest {
		t.Fatalf("manifest digest changed: got %s, want %s", got, vector.ManifestDigest)
	}
}

func decodeHash(t *testing.T, target []byte, source string) {
	t.Helper()
	decoded, err := hex.DecodeString(source)
	if err != nil || len(decoded) != len(target) {
		t.Fatalf("invalid hash vector %q", source)
	}
	copy(target, decoded)
}

func TestValidateRejectsNonCanonicalChunk(t *testing.T) {
	value := Manifest{Version: Version1, ObjectSize: 5, ChunkSize: 4, Chunks: []Chunk{{Index: 0, Size: 4}, {Index: 1, Offset: 3, Size: 1}}}
	if err := value.Validate(); err == nil {
		t.Fatal("non-contiguous chunk was accepted")
	}
}
