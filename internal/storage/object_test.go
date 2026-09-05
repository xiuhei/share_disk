package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestObjectManagerImportIdempotent(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	om := NewObjectManager(store)

	src := filepath.Join(t.TempDir(), "source.bin")
	content := []byte("hello share disk object content")
	if err := os.WriteFile(src, content, 0600); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	ctx := context.Background()

	first, err := om.Import(ctx, src)
	if err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	if first.Size != int64(len(content)) {
		t.Fatalf("unexpected size: %d", first.Size)
	}
	if first.Status != ObjectStatusReady {
		t.Fatalf("expected ready status, got %s", first.Status)
	}
	if first.ChunkCount != 1 {
		t.Fatalf("expected 1 chunk, got %d", first.ChunkCount)
	}
	if len(first.Chunks) != 1 {
		t.Fatalf("expected chunk manifest, got %d chunks", len(first.Chunks))
	}

	// Importing the same content again must reuse the object and not corrupt it.
	second, err := om.Import(ctx, src)
	if err != nil {
		t.Fatalf("second import failed: %v", err)
	}
	if second.Hash != first.Hash {
		t.Fatalf("hash mismatch on duplicate import: %s != %s", second.Hash, first.Hash)
	}

	// The object must still exist and be valid.
	objects, err := om.ListObjects()
	if err != nil {
		t.Fatalf("failed to list objects: %v", err)
	}
	if len(objects) != 1 {
		t.Fatalf("expected exactly one object, got %d", len(objects))
	}

	// No leftover .part files.
	entries, err := os.ReadDir(store.IncomingDir())
	if err != nil {
		t.Fatalf("failed to read incoming dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no leftover .part files, got %d", len(entries))
	}
}

func TestObjectManagerImportLargeContentStreams(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	om := NewObjectManager(store)

	// A bit more than two chunks to exercise chunk manifest generation.
	content := make([]byte, DefaultChunkSize*2+1234)
	for i := range content {
		content[i] = byte(i % 251)
	}
	src := filepath.Join(t.TempDir(), "large.bin")
	if err := os.WriteFile(src, content, 0600); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}

	info, err := om.Import(context.Background(), src)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if info.ChunkCount != 3 {
		t.Fatalf("expected 3 chunks, got %d", info.ChunkCount)
	}
	if int64(info.ChunkCount) != info.Size/info.ChunkSize+1 {
		t.Fatalf("chunk count mismatch: %d chunks for size %d", info.ChunkCount, info.Size)
	}
}

// 3.1.5: a symlink placed at the object path must not be treated as the object.
func TestGetObjectInfoRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	target := filepath.Join(t.TempDir(), "target.bin")
	if err := os.WriteFile(target, []byte("real content"), 0600); err != nil {
		t.Fatalf("failed to write target: %v", err)
	}
	hash, err := HashFile(target)
	if err != nil {
		t.Fatalf("failed to hash target: %v", err)
	}

	objectPath := store.ObjectPath(hash)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0700); err != nil {
		t.Fatalf("failed to create object dir: %v", err)
	}
	if err := os.Symlink(target, objectPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// ObjectExists must not report the symlink as a valid object.
	if store.ObjectExists(hash) {
		t.Fatal("ObjectExists must reject a symlink")
	}

	// GetObjectInfo must reject the symlink.
	if _, err := store.GetObjectInfo(hash); err == nil {
		t.Fatal("GetObjectInfo must reject a symlink")
	}
}
