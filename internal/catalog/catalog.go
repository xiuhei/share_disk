package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

// Folder represents a folder in the catalog
type Folder struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	ParentID       *string   `json:"parent_id,omitempty"`
	Name           string    `json:"name"`
	NormalizedName string    `json:"normalized_name"`
	Status         string    `json:"status"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// FileObject represents a content-addressed file object
type FileObject struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	SHA256     []byte    `json:"sha256"`
	Size       int64     `json:"size"`
	MIME       string    `json:"mime,omitempty"`
	ChunkSize  int       `json:"chunk_size"`
	ChunkCount int       `json:"chunk_count"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// FileEntry represents a logical file in a directory
type FileEntry struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	FolderID       string    `json:"folder_id"`
	ObjectID       string    `json:"object_id"`
	Name           string    `json:"name"`
	NormalizedName string    `json:"normalized_name"`
	OriginDeviceID *string   `json:"origin_device_id,omitempty"`
	Status         string    `json:"status"`
	Version        int64     `json:"version"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Repository handles catalog database operations
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new catalog Repository
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CreateFolder creates a new folder
func (r *Repository) CreateFolder(ctx context.Context, userID string, parentID *string, name string) (*Folder, error) {
	normalizedName, err := normalizeName(name)
	if err != nil {
		return nil, err
	}

	// Validate parent folder ownership when a parent is specified.
	if parentID != nil {
		var parentUser string
		err := r.db.QueryRowContext(ctx,
			`SELECT user_id::text FROM folders WHERE id = $1`, *parentID).Scan(&parentUser)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("parent folder not found")
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get parent folder: %w", err)
		}
		if parentUser != userID {
			return nil, fmt.Errorf("parent folder not found")
		}
	}

	folder := &Folder{
		ID:             uuid.New().String(),
		UserID:         userID,
		ParentID:       parentID,
		Name:           name,
		NormalizedName: normalizedName,
		Status:         "active",
		Version:        1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	query := `
		INSERT INTO folders (id, user_id, parent_id, name, normalized_name, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	_, err = r.db.ExecContext(ctx, query,
		folder.ID, folder.UserID, folder.ParentID, folder.Name,
		folder.NormalizedName, folder.Status, folder.Version,
		folder.CreatedAt, folder.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	return folder, nil
}

// GetFolder retrieves a folder by ID
func (r *Repository) GetFolder(ctx context.Context, id, userID string) (*Folder, error) {
	folder := &Folder{}
	query := `
		SELECT id, user_id, parent_id, name, normalized_name, status, version, created_at, updated_at
		FROM folders
		WHERE id = $1 AND user_id = $2 AND status = 'active'
	`

	err := r.db.QueryRowContext(ctx, query, id, userID).Scan(
		&folder.ID, &folder.UserID, &folder.ParentID, &folder.Name,
		&folder.NormalizedName, &folder.Status, &folder.Version,
		&folder.CreatedAt, &folder.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}

	return folder, nil
}

// ListFolders lists folders in a parent folder
func (r *Repository) ListFolders(ctx context.Context, userID string, parentID *string) ([]*Folder, error) {
	var query string
	var args []interface{}

	if parentID == nil {
		query = `
			SELECT id, user_id, parent_id, name, normalized_name, status, version, created_at, updated_at
			FROM folders
			WHERE user_id = $1 AND parent_id IS NULL AND status = 'active'
			ORDER BY normalized_name
		`
		args = []interface{}{userID}
	} else {
		query = `
			SELECT id, user_id, parent_id, name, normalized_name, status, version, created_at, updated_at
			FROM folders
			WHERE user_id = $1 AND parent_id = $2 AND status = 'active'
			ORDER BY normalized_name
		`
		args = []interface{}{userID, *parentID}
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}
	defer rows.Close()

	var folders []*Folder
	for rows.Next() {
		folder := &Folder{}
		err := rows.Scan(
			&folder.ID, &folder.UserID, &folder.ParentID, &folder.Name,
			&folder.NormalizedName, &folder.Status, &folder.Version,
			&folder.CreatedAt, &folder.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan folder: %w", err)
		}
		folders = append(folders, folder)
	}

	return folders, nil
}

// CreateFileObject creates a new file object
func (r *Repository) CreateFileObject(ctx context.Context, userID string, sha256 []byte, size int64, mime string, chunkSize, chunkCount int) (*FileObject, error) {
	obj := &FileObject{
		ID:         uuid.New().String(),
		UserID:     userID,
		SHA256:     sha256,
		Size:       size,
		MIME:       mime,
		ChunkSize:  chunkSize,
		ChunkCount: chunkCount,
		Status:     "pending",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	query := `
		INSERT INTO file_objects (id, user_id, sha256, size, mime, chunk_size, chunk_count, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.ExecContext(ctx, query,
		obj.ID, obj.UserID, obj.SHA256, obj.Size, obj.MIME,
		obj.ChunkSize, obj.ChunkCount, obj.Status,
		obj.CreatedAt, obj.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create file object: %w", err)
	}

	return obj, nil
}

// GetFileObject retrieves a file object by ID
func (r *Repository) GetFileObject(ctx context.Context, id, userID string) (*FileObject, error) {
	obj := &FileObject{}
	query := `
		SELECT id, user_id, sha256, size, mime, chunk_size, chunk_count, status, created_at, updated_at
		FROM file_objects
		WHERE id = $1 AND user_id = $2
	`

	err := r.db.QueryRowContext(ctx, query, id, userID).Scan(
		&obj.ID, &obj.UserID, &obj.SHA256, &obj.Size, &obj.MIME,
		&obj.ChunkSize, &obj.ChunkCount, &obj.Status,
		&obj.CreatedAt, &obj.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file object: %w", err)
	}

	return obj, nil
}

// CreateFileEntry creates a new file entry. The folder and object must belong
// to the same account as the caller.
func (r *Repository) CreateFileEntry(ctx context.Context, userID, folderID, objectID, name string, originDeviceID *string) (*FileEntry, error) {
	normalizedName, err := normalizeName(name)
	if err != nil {
		return nil, err
	}

	// Validate folder ownership.
	var folderUser string
	err = r.db.QueryRowContext(ctx,
		`SELECT user_id::text FROM folders WHERE id = $1`, folderID).Scan(&folderUser)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("folder not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get folder: %w", err)
	}
	if folderUser != userID {
		return nil, fmt.Errorf("folder not found")
	}

	// Validate object ownership.
	var objectUser string
	err = r.db.QueryRowContext(ctx,
		`SELECT user_id::text FROM file_objects WHERE id = $1`, objectID).Scan(&objectUser)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("object not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	if objectUser != userID {
		return nil, fmt.Errorf("object not found")
	}

	entry := &FileEntry{
		ID:             uuid.New().String(),
		UserID:         userID,
		FolderID:       folderID,
		ObjectID:       objectID,
		Name:           name,
		NormalizedName: normalizedName,
		OriginDeviceID: originDeviceID,
		Status:         "active",
		Version:        1,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	query := `
		INSERT INTO file_entries (id, user_id, folder_id, object_id, name, normalized_name, origin_device_id, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err = r.db.ExecContext(ctx, query,
		entry.ID, entry.UserID, entry.FolderID, entry.ObjectID,
		entry.Name, entry.NormalizedName, entry.OriginDeviceID,
		entry.Status, entry.Version, entry.CreatedAt, entry.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create file entry: %w", err)
	}

	return entry, nil
}

// ListFileEntries lists file entries in a folder
func (r *Repository) ListFileEntries(ctx context.Context, userID, folderID string) ([]*FileEntry, error) {
	query := `
		SELECT id, user_id, folder_id, object_id, name, normalized_name, origin_device_id, status, version, created_at, updated_at
		FROM file_entries
		WHERE user_id = $1 AND folder_id = $2 AND status = 'active'
		ORDER BY normalized_name
	`

	rows, err := r.db.QueryContext(ctx, query, userID, folderID)
	if err != nil {
		return nil, fmt.Errorf("failed to list file entries: %w", err)
	}
	defer rows.Close()

	var entries []*FileEntry
	for rows.Next() {
		entry := &FileEntry{}
		err := rows.Scan(
			&entry.ID, &entry.UserID, &entry.FolderID, &entry.ObjectID,
			&entry.Name, &entry.NormalizedName, &entry.OriginDeviceID,
			&entry.Status, &entry.Version, &entry.CreatedAt, &entry.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan file entry: %w", err)
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// normalizeName validates and normalizes a name using Unicode NFC. It rejects
// empty names, "." / "..", platform reserved names (with or without
// extension), invalid characters, control characters, and trailing dots or
// spaces. This is the single domain contract shared by the control plane.
func normalizeName(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty name")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("reserved name: %s", name)
	}

	normalized := norm.NFC.String(name)

	baseName := normalized
	if dotIndex := strings.Index(normalized, "."); dotIndex != -1 {
		baseName = normalized[:dotIndex]
	}
	upper := strings.ToUpper(baseName)
	for _, r := range reservedNames {
		if upper == r {
			return "", fmt.Errorf("reserved name: %s", name)
		}
	}

	for _, r := range normalized {
		if r == '/' || r == '\\' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return "", fmt.Errorf("invalid character in name: %c", r)
		}
		if unicode.IsControl(r) {
			return "", fmt.Errorf("control character in name")
		}
	}

	if strings.HasSuffix(normalized, ".") || strings.HasSuffix(normalized, " ") {
		return "", fmt.Errorf("trailing dots or spaces not allowed: %s", name)
	}

	return normalized, nil
}

var reservedNames = []string{
	"CON", "PRN", "AUX", "NUL",
	"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
	"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9",
}
