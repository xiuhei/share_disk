package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/share-disk/share-disk/internal/storage/hasher"
)

// DefaultChunkSize is the fixed V1 chunk size (4 MiB).
const DefaultChunkSize = 4 * 1024 * 1024

// ObjectManager manages local objects and their state.
type ObjectManager struct {
	storage   *Storage
	chunkSize int64
	mu        sync.RWMutex
}

// NewObjectManager creates a new ObjectManager with the default chunk size.
func NewObjectManager(storage *Storage) *ObjectManager {
	return NewObjectManagerWithChunkSize(storage, DefaultChunkSize)
}

// NewObjectManagerWithChunkSize creates a new ObjectManager with an explicit
// chunk size. The chunk size must be positive; values <= 0 fall back to
// DefaultChunkSize.
func NewObjectManagerWithChunkSize(storage *Storage, chunkSize int64) *ObjectManager {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	return &ObjectManager{
		storage:   storage,
		chunkSize: chunkSize,
	}
}

// StagedImport represents a file whose content has been streamed into an
// incoming `.part` file and fully hashed, but not yet committed into the
// objects directory. The caller must either Commit or Discard it.
type StagedImport struct {
	incomingPath string
	objectPath   string
	hash         Hash
	size         int64
	chunkSize    int64
	chunks       []ChunkInfo
}

// Hash returns the computed object hash.
func (s *StagedImport) Hash() Hash { return s.hash }

// Size returns the object size in bytes.
func (s *StagedImport) Size() int64 { return s.size }

// ObjectPath returns the destination object path.
func (s *StagedImport) ObjectPath() string { return s.objectPath }

// ChunkSize returns the chunk size used for the manifest.
func (s *StagedImport) ChunkSize() int64 { return s.chunkSize }

// Chunks returns the per-chunk hashes.
func (s *StagedImport) Chunks() []ChunkInfo { return s.chunks }

// Discard removes the staged `.part` file. It is safe to call after Commit
// (the file no longer exists, which is ignored).
func (s *StagedImport) Discard() error {
	if s.incomingPath == "" {
		return nil
	}
	if err := os.Remove(s.incomingPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove staged file: %w", err)
	}
	s.incomingPath = ""
	return nil
}

// Commit atomically renames the staged file into the objects directory.
// The rename itself is the critical durable operation. Post-rename chmod and
// parent fsync are best-effort; a failure there means the object is present
// and hash-verifiable but may have looser permissions than intended. The
// startup recovery (verifyPersistedObject) re-validates on next start.
func (s *StagedImport) Commit() error {
	if err := os.MkdirAll(filepath.Dir(s.objectPath), 0700); err != nil {
		return fmt.Errorf("failed to create object directory: %w", err)
	}

	if err := os.Rename(s.incomingPath, s.objectPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}
	s.incomingPath = ""

	// Best-effort: tighten permissions to owner-only.
	if err := os.Chmod(s.objectPath, 0600); err != nil {
		// Log but do not fail: the object is on disk and hash-verifiable.
		// A future startup recovery will detect and can re-commit.
		_ = err
	}

	// Best-effort: sync parent directory to ensure rename is durable.
	parentFile, err := os.Open(filepath.Dir(s.objectPath))
	if err != nil {
		return nil // rename succeeded; parent sync is best-effort
	}
	defer parentFile.Close()

	_ = parentFile.Sync()
	return nil
}

// Stage streams a source file into a `.part` file, computing the overall
// SHA-256 and the chunk manifest along the way, and fsyncs the result. The
// caller owns the returned StagedImport and must Commit or Discard it.
func (om *ObjectManager) Stage(ctx context.Context, sourcePath string) (staged *StagedImport, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open source file: %w", err)
	}
	defer sourceFile.Close()

	sourceInfo, err := sourceFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat source file: %w", err)
	}
	if sourceInfo.IsDir() {
		return nil, fmt.Errorf("cannot import directory")
	}

	incomingDir := om.storage.IncomingDir()
	if err := os.MkdirAll(incomingDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create incoming directory: %w", err)
	}

	partFile, err := os.CreateTemp(incomingDir, "import-*.part")
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}
	incomingPath := partFile.Name()

	cleanup := func() {
		_ = partFile.Close()
		_ = os.Remove(incomingPath)
	}

	overallHasher := hasher.New()
	chunkHasher := hasher.NewChunkHasher(om.chunkSize)
	writer := io.MultiWriter(partFile, overallHasher, chunkHasher)

	buf := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			cleanup()
			return nil, err
		}

		n, readErr := sourceFile.Read(buf)
		if n > 0 {
			if _, err := writer.Write(buf[:n]); err != nil {
				cleanup()
				return nil, fmt.Errorf("failed to write file content: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			cleanup()
			return nil, fmt.Errorf("failed to read file content: %w", readErr)
		}
	}

	if err := partFile.Sync(); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to sync file: %w", err)
	}
	if err := partFile.Close(); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to close temporary file: %w", err)
	}

	hash := overallHasher.Sum()
	chunks := chunkHasher.Finish()

	staged = &StagedImport{
		incomingPath: incomingPath,
		objectPath:   om.storage.ObjectPath(hash),
		hash:         hash,
		size:         overallHasher.Size(),
		chunkSize:    om.chunkSize,
		chunks:       chunks,
	}
	return staged, nil
}

// Import imports a file into the managed object area. It is idempotent: if the
// object already exists, the staged `.part` file is discarded and the existing
// object is returned without any destructive change.
func (om *ObjectManager) Import(ctx context.Context, sourcePath string) (*ObjectInfo, error) {
	om.mu.Lock()
	defer om.mu.Unlock()

	staged, err := om.Stage(ctx, sourcePath)
	if err != nil {
		return nil, err
	}
	defer staged.Discard()

	if om.storage.ObjectExists(staged.hash) {
		existing, err := om.storage.GetObjectInfo(staged.hash)
		if err != nil {
			return nil, fmt.Errorf("failed to get existing object info: %w", err)
		}
		if existing.Size != staged.size {
			return nil, fmt.Errorf("object size mismatch for hash %s", staged.hash.String())
		}
		return om.objectInfo(existing, staged), nil
	}

	if err := staged.Commit(); err != nil {
		return nil, err
	}

	return &ObjectInfo{
		Hash:       staged.hash,
		Size:       staged.size,
		ChunkSize:  staged.chunkSize,
		ChunkCount: len(staged.chunks),
		Chunks:     staged.chunks,
		Status:     ObjectStatusReady,
		Path:       staged.objectPath,
	}, nil
}

func (om *ObjectManager) objectInfo(existing *ObjectInfo, staged *StagedImport) *ObjectInfo {
	existing.ChunkSize = staged.chunkSize
	existing.ChunkCount = len(staged.chunks)
	existing.Chunks = staged.chunks
	return existing
}

// Verify verifies an existing object by recomputing its hash.
func (om *ObjectManager) Verify(ctx context.Context, hash Hash) error {
	om.mu.Lock()
	defer om.mu.Unlock()

	objectPath := om.storage.ObjectPath(hash)

	if !om.storage.ObjectExists(hash) {
		return fmt.Errorf("object not found: %s", hash.String())
	}

	actualHash, err := HashFile(objectPath)
	if err != nil {
		return fmt.Errorf("failed to hash object: %w", err)
	}

	if actualHash != hash {
		return fmt.Errorf("hash mismatch: expected %s, got %s", hash.String(), actualHash.String())
	}

	return nil
}

// HashFile computes the SHA-256 hash of a file.
func HashFile(path string) (Hash, error) {
	f, err := os.Open(path)
	if err != nil {
		return Hash{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer f.Close()

	h := hasher.New()
	if _, err := io.Copy(h, f); err != nil {
		return Hash{}, fmt.Errorf("failed to hash file: %w", err)
	}

	return h.Sum(), nil
}

// GetObject returns information about an object.
func (om *ObjectManager) GetObject(hash Hash) (*ObjectInfo, error) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	return om.storage.GetObjectInfo(hash)
}

// DeleteObject deletes an object.
func (om *ObjectManager) DeleteObject(hash Hash) error {
	om.mu.Lock()
	defer om.mu.Unlock()

	return om.storage.DeleteObject(hash)
}

// ListObjects lists all objects in the storage.
func (om *ObjectManager) ListObjects() ([]ObjectInfo, error) {
	om.mu.RLock()
	defer om.mu.RUnlock()

	objectsDir := om.storage.ObjectsDir()
	var objects []ObjectInfo

	err := filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		hashStr := filepath.Base(path)
		hash, err := ParseHash(hashStr)
		if err != nil {
			return nil
		}

		objectInfo, err := om.storage.GetObjectInfo(hash)
		if err != nil {
			return nil
		}

		objects = append(objects, *objectInfo)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}

	return objects, nil
}

// ScanAndRepair scans the storage and repairs any inconsistencies.
func (om *ObjectManager) ScanAndRepair(ctx context.Context) error {
	om.mu.Lock()
	defer om.mu.Unlock()

	if err := om.storage.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to ensure directories: %w", err)
	}

	incomingDir := om.storage.IncomingDir()
	entries, err := os.ReadDir(incomingDir)
	if err != nil {
		return fmt.Errorf("failed to read incoming directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		path := filepath.Join(incomingDir, entry.Name())

		if !strings.HasSuffix(entry.Name(), ".part") {
			fmt.Printf("Warning: unknown file in incoming directory: %s\n", path)
			continue
		}

		info, err := entry.Info()
		if err != nil {
			fmt.Printf("Warning: failed to get file info for %s: %v\n", path, err)
			continue
		}

		if time.Since(info.ModTime()) > 1*time.Hour {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("failed to remove stale part file: %w", err)
			}
		}
	}

	return nil
}
