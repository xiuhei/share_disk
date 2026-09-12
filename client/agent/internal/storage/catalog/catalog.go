package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Entry represents a file or directory entry.
type Entry struct {
	Name           string `json:"name"`
	NormalizedName string `json:"normalized_name"`
	Path           string `json:"path"`
	IsDir          bool   `json:"is_dir"`
}

// Catalog manages the logical directory structure.
type Catalog struct {
	rootDir string
}

// New creates a new Catalog.
func New(rootDir string) *Catalog {
	return &Catalog{
		rootDir: rootDir,
	}
}

// NormalizeName normalizes a file or directory name.
// Rules:
// - Unicode NFC normalization
// - Preserves original case
// - Rejects empty names, ".", ".."
// - Rejects platform reserved names (including with any extension, e.g. CON.txt)
// - Rejects trailing dots and spaces
func NormalizeName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty name")
	}

	if name == "." || name == ".." {
		return "", fmt.Errorf("reserved name: %s", name)
	}

	// Unicode NFC normalization
	normalized := norm.NFC.String(name)

	// Check for Windows reserved names, with or without an extension. The base
	// name is the part before the first dot.
	baseName := normalized
	if dotIndex := strings.Index(normalized, "."); dotIndex != -1 {
		baseName = normalized[:dotIndex]
	}

	upper := strings.ToUpper(baseName)
	reserved := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	for _, r := range reserved {
		if upper == r {
			return "", fmt.Errorf("reserved name: %s", name)
		}
	}

	// Check for invalid characters
	for _, r := range normalized {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return "", fmt.Errorf("invalid character in name: %c", r)
		}
		if unicode.IsControl(r) {
			return "", fmt.Errorf("control character in name")
		}
	}

	// Check for trailing dots and spaces
	if strings.HasSuffix(normalized, ".") || strings.HasSuffix(normalized, " ") {
		return "", fmt.Errorf("trailing dots or spaces not allowed: %s", name)
	}

	return normalized, nil
}

// NormalizePath normalizes a logical path.
func NormalizePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("empty path")
	}

	// Convert to forward slashes
	path = filepath.ToSlash(path)

	// Reject absolute paths
	if strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("absolute path not allowed")
	}

	// Split into segments
	segments := strings.Split(path, "/")

	// Normalize each segment
	normalized := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg == "" {
			return "", fmt.Errorf("empty path segment")
		}

		normSeg, err := NormalizeName(seg)
		if err != nil {
			return "", fmt.Errorf("invalid path segment '%s': %w", seg, err)
		}
		normalized = append(normalized, normSeg)
	}

	if len(normalized) == 0 {
		return "", fmt.Errorf("empty path after normalization")
	}

	return strings.Join(normalized, "/"), nil
}

// ValidatePath validates a logical path.
func ValidatePath(path string) error {
	_, err := NormalizePath(path)
	return err
}

// JoinPath joins multiple path segments into a single path.
func JoinPath(parts ...string) (string, error) {
	var result string
	for i, part := range parts {
		if i == 0 {
			result = part
		} else {
			result = result + "/" + part
		}
	}
	return NormalizePath(result)
}

// SplitPath splits a path into directory and file name.
func SplitPath(path string) (dir, name string) {
	path = filepath.ToSlash(path)
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash < 0 {
		return "", path
	}
	return path[:lastSlash], path[lastSlash+1:]
}

// BaseName returns the base name of a path.
func BaseName(path string) string {
	_, name := SplitPath(path)
	return name
}

// DirName returns the directory name of a path.
func DirName(path string) string {
	dir, _ := SplitPath(path)
	return dir
}

// IsSubPath checks if child is a subpath of parent.
func IsSubPath(parent, child string) bool {
	parent = filepath.ToSlash(parent)
	child = filepath.ToSlash(child)

	// Ensure parent ends with /
	if !strings.HasSuffix(parent, "/") {
		parent = parent + "/"
	}

	return strings.HasPrefix(child, parent)
}

// SafePath checks if a physical path is safely within the root directory.
// It resolves symlinks and checks that the resolved path is within the root.
func SafePath(rootDir, physicalPath string) (string, error) {
	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(physicalPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Path doesn't exist yet, check parent directory
			parent := filepath.Dir(physicalPath)
			resolvedParent, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", fmt.Errorf("failed to resolve parent path: %w", err)
			}
			resolved = filepath.Join(resolvedParent, filepath.Base(physicalPath))
		} else {
			return "", fmt.Errorf("failed to resolve symlinks: %w", err)
		}
	}

	// Resolve root directory
	resolvedRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve root directory: %w", err)
	}

	// Check if resolved path is within root
	if !IsSubPath(resolvedRoot, resolved) {
		return "", fmt.Errorf("path is outside root directory: %s", physicalPath)
	}

	return resolved, nil
}

// ListDir lists entries in a directory.
func (c *Catalog) ListDir(logicalPath string) ([]Entry, error) {
	normalizedPath, err := NormalizePath(logicalPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// Map logical path to physical path
	physicalPath := filepath.Join(c.rootDir, filepath.FromSlash(normalizedPath))

	// Check for symlink safety
	safePath, err := SafePath(c.rootDir, physicalPath)
	if err != nil {
		return nil, fmt.Errorf("unsafe path: %w", err)
	}

	// Check if directory exists
	info, err := os.Stat(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("directory not found: %s", logicalPath)
		}
		return nil, fmt.Errorf("failed to stat directory: %w", err)
	}

	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", logicalPath)
	}

	// Read directory entries
	entries, err := os.ReadDir(safePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	// Convert to catalog entries
	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files and special entries
		if strings.HasPrefix(name, ".") {
			continue
		}

		normalized, err := NormalizeName(name)
		if err != nil {
			// Skip entries with invalid names
			continue
		}

		result = append(result, Entry{
			Name:           name,
			NormalizedName: normalized,
			Path:           normalizedPath + "/" + normalized,
			IsDir:          entry.IsDir(),
		})
	}

	return result, nil
}

// CreateDir creates a directory.
func (c *Catalog) CreateDir(logicalPath string) error {
	normalizedPath, err := NormalizePath(logicalPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	physicalPath := filepath.Join(c.rootDir, filepath.FromSlash(normalizedPath))

	// Check for symlink safety
	safePath, err := SafePath(c.rootDir, physicalPath)
	if err != nil {
		return fmt.Errorf("unsafe path: %w", err)
	}

	// Check if already exists
	info, err := os.Stat(safePath)
	if err == nil {
		if info.IsDir() {
			return nil // Already exists
		}
		return fmt.Errorf("path exists but is not a directory: %s", logicalPath)
	}

	// Create directory
	if err := os.MkdirAll(safePath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

// DeleteDir deletes an empty directory.
func (c *Catalog) DeleteDir(logicalPath string) error {
	normalizedPath, err := NormalizePath(logicalPath)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	physicalPath := filepath.Join(c.rootDir, filepath.FromSlash(normalizedPath))

	// Check for symlink safety
	safePath, err := SafePath(c.rootDir, physicalPath)
	if err != nil {
		return fmt.Errorf("unsafe path: %w", err)
	}

	// Check if directory exists
	info, err := os.Stat(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("directory not found: %s", logicalPath)
		}
		return fmt.Errorf("failed to stat directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", logicalPath)
	}

	// Check if directory is empty
	entries, err := os.ReadDir(safePath)
	if err != nil {
		return fmt.Errorf("failed to read directory: %w", err)
	}

	if len(entries) > 0 {
		return fmt.Errorf("directory not empty: %s", logicalPath)
	}

	// Delete directory
	if err := os.Remove(safePath); err != nil {
		return fmt.Errorf("failed to delete directory: %w", err)
	}

	return nil
}

// Stat returns information about a file or directory.
func (c *Catalog) Stat(logicalPath string) (*Entry, error) {
	normalizedPath, err := NormalizePath(logicalPath)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	physicalPath := filepath.Join(c.rootDir, filepath.FromSlash(normalizedPath))

	// Check for symlink safety
	safePath, err := SafePath(c.rootDir, physicalPath)
	if err != nil {
		return nil, fmt.Errorf("unsafe path: %w", err)
	}

	info, err := os.Stat(safePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("path not found: %s", logicalPath)
		}
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	name := BaseName(normalizedPath)
	normalized, err := NormalizeName(name)
	if err != nil {
		return nil, fmt.Errorf("invalid name: %w", err)
	}

	return &Entry{
		Name:           name,
		NormalizedName: normalized,
		Path:           normalizedPath,
		IsDir:          info.IsDir(),
	}, nil
}
