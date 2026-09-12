package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/share-disk/share-disk/client/ubuntu/internal/platform"
	"github.com/share-disk/share-disk/client/ubuntu/internal/storage/hasher"
	"golang.org/x/text/unicode/norm"
)

// ObjectStatus represents the status of a local object.
type ObjectStatus string

const (
	ObjectStatusAbsent      ObjectStatus = "absent"
	ObjectStatusImporting   ObjectStatus = "importing"
	ObjectStatusDownloading ObjectStatus = "downloading"
	ObjectStatusVerifying   ObjectStatus = "verifying"
	ObjectStatusReady       ObjectStatus = "ready"
	ObjectStatusFailed      ObjectStatus = "failed"
	ObjectStatusQuarantined ObjectStatus = "quarantined"
	ObjectStatusDeleting    ObjectStatus = "deleting"
	ObjectStatusMissing     ObjectStatus = "missing"
	ObjectStatusCorrupt     ObjectStatus = "corrupt"
)

// Hash is an alias for hasher.Hash.
type Hash = hasher.Hash

// ParseHash parses a hex-encoded hash string.
func ParseHash(s string) (Hash, error) {
	return hasher.ParseHash(s)
}

// ChunkInfo represents information about a chunk.
type ChunkInfo = hasher.ChunkInfo

// ObjectInfo represents information about a stored object.
type ObjectInfo struct {
	Hash       Hash         `json:"hash"`
	Size       int64        `json:"size"`
	ChunkSize  int64        `json:"chunk_size"`
	ChunkCount int          `json:"chunk_count"`
	Chunks     []ChunkInfo  `json:"chunks"`
	Status     ObjectStatus `json:"status"`
	Path       string       `json:"path"`
}

// Storage manages local object storage.
type Storage struct {
	rootDir string
}

// New creates a new Storage instance.
func New(rootDir string) (*Storage, error) {
	// Create root directory if it doesn't exist with secure permissions
	if err := os.MkdirAll(rootDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create root directory: %w", err)
	}

	// Verify and fix permissions if needed
	if err := verifyAndFixPermissions(rootDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to verify root directory permissions: %w", err)
	}

	return &Storage{
		rootDir: rootDir,
	}, nil
}

// RootDir returns the root directory path.
func (s *Storage) RootDir() string {
	return s.rootDir
}

// ObjectsDir returns the objects directory path.
func (s *Storage) ObjectsDir() string {
	return filepath.Join(s.rootDir, "objects")
}

// IncomingDir returns the incoming directory path.
func (s *Storage) IncomingDir() string {
	return filepath.Join(s.rootDir, "incoming")
}

// ObjectPath returns the path for an object with the given hash.
func (s *Storage) ObjectPath(hash Hash) string {
	hashStr := hash.String()
	// Use first 2 chars as directory prefix for better filesystem performance
	return filepath.Join(s.ObjectsDir(), hashStr[:2], hashStr)
}

// IncomingPath returns the path for an incoming file with the given task ID.
func (s *Storage) IncomingPath(taskID string) string {
	return filepath.Join(s.IncomingDir(), taskID+".part")
}

// ObjectExists checks if an object exists and is ready.
func (s *Storage) ObjectExists(hash Hash) bool {
	path := s.ObjectPath(hash)
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	return !info.IsDir()
}

// GetObjectInfo returns information about a stored object.
func (s *Storage) GetObjectInfo(hash Hash) (*ObjectInfo, error) {
	path := s.ObjectPath(hash)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object not found: %s", hash.String())
		}
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	// Verify it's a regular file, not a directory or symlink. Lstat does not
	// follow symlinks, so a symlink (even one pointing at a valid object) is
	// rejected rather than treated as the object itself.
	if info.IsDir() {
		return nil, fmt.Errorf("object path is a directory: %s", hash.String())
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("object path is a symlink: %s", hash.String())
	}

	// Verify the object by computing its hash
	actualHash, err := HashFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to verify object: %w", err)
	}

	if actualHash != hash {
		return nil, fmt.Errorf("object hash mismatch: expected %s, got %s", hash.String(), actualHash.String())
	}

	return &ObjectInfo{
		Hash:   hash,
		Size:   info.Size(),
		Status: ObjectStatusReady,
		Path:   path,
	}, nil
}

// DeleteObject deletes an object by hash.
func (s *Storage) DeleteObject(hash Hash) error {
	path := s.ObjectPath(hash)

	// Check if object exists. Lstat does not follow symlinks, so a symlink is
	// rejected rather than followed and deleted as if it were the object.
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to stat object: %w", err)
	}

	// Verify it's a regular file, not a directory or symlink
	if info.IsDir() {
		return fmt.Errorf("cannot delete directory as object: %s", hash.String())
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("cannot delete symlink as object: %s", hash.String())
	}

	// Remove the file
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	// Sync parent directory to ensure deletion is durable
	parentDir := filepath.Dir(path)
	parentFile, err := os.Open(parentDir)
	if err != nil {
		return fmt.Errorf("failed to open parent directory: %w", err)
	}
	defer parentFile.Close()

	if err := parentFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync parent directory: %w", err)
	}

	return nil
}

// EnsureDirectories creates all necessary directories.
func (s *Storage) EnsureDirectories() error {
	dirs := []string{
		s.ObjectsDir(),
		s.IncomingDir(),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Verify and fix permissions
		if err := verifyAndFixPermissions(dir, 0700); err != nil {
			return fmt.Errorf("failed to verify directory permissions %s: %w", dir, err)
		}
	}

	return nil
}

// NormalizePath normalizes a logical path.
// Rules:
// - Uses forward slashes
// - Unicode NFC normalization
// - Case-sensitive
// - Rejects empty segments, ".", "..", absolute paths, and platform reserved names
// - Rejects Windows reserved names with any extension (e.g., CON.txt)
// - Rejects trailing dots and spaces in names
func NormalizePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}

	// Convert to forward slashes
	path = filepath.ToSlash(path)

	// Reject absolute paths
	if strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("absolute path not allowed: %s", path)
	}

	// Split into segments
	segments := strings.Split(path, "/")

	// Validate and normalize each segment
	normalized := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg == "" {
			return "", fmt.Errorf("empty path segment")
		}
		if seg == "." {
			return "", fmt.Errorf("'.' path segment not allowed")
		}
		if seg == ".." {
			return "", fmt.Errorf("'..' path segment not allowed")
		}

		// Unicode NFC normalization
		seg = norm.NFC.String(seg)

		// Check for Windows reserved names (even on other platforms)
		// This includes names like CON, PRN, AUX, NUL, COM1-COM9, LPT1-LPT9
		// Also reject names like CON.txt, PRN.pdf, etc.
		baseName := seg
		if dotIndex := strings.Index(seg, "."); dotIndex != -1 {
			baseName = seg[:dotIndex]
		}

		upper := strings.ToUpper(baseName)
		reserved := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
		for _, r := range reserved {
			if upper == r {
				return "", fmt.Errorf("reserved name not allowed: %s", seg)
			}
		}

		// Check for invalid characters
		for _, r := range seg {
			if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
				return "", fmt.Errorf("invalid character in name: %c", r)
			}
			if unicode.IsControl(r) {
				return "", fmt.Errorf("control character in name")
			}
		}

		// Check for trailing dots and spaces
		if strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " ") {
			return "", fmt.Errorf("trailing dots or spaces not allowed: %s", seg)
		}

		normalized = append(normalized, seg)
	}

	if len(normalized) == 0 {
		return "", fmt.Errorf("empty path after normalization")
	}

	return strings.Join(normalized, "/"), nil
}

// HashReader reads from r and returns the SHA-256 hash.
func HashReader(r io.Reader) (Hash, error) {
	h := hasher.New()
	if _, err := io.Copy(h, r); err != nil {
		return Hash{}, fmt.Errorf("failed to hash data: %w", err)
	}
	return h.Sum(), nil
}

// GetDiskSpace returns the available disk space in bytes for the given path.
func GetDiskSpace(path string) (int64, error) {
	return platform.GetDiskSpace(path)
}

// verifyAndFixPermissions verifies that a directory has the expected permissions
// and fixes them if they are too permissive.
func verifyAndFixPermissions(path string, expected os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path: %w", err)
	}

	// Check if permissions are too permissive
	currentPerm := info.Mode().Perm()
	if currentPerm&^expected != 0 {
		// Permissions are too permissive, fix them
		if err := os.Chmod(path, expected); err != nil {
			return fmt.Errorf("failed to fix permissions: %w", err)
		}
	}

	return nil
}
