package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrLANFileNotFound hides whether a file belongs to another account.
	ErrLANFileNotFound = errors.New("LAN file not found")
	// ErrLANNameConflict indicates an active file already owns the normalized name.
	ErrLANNameConflict = errors.New("LAN file name conflict")
	// ErrLANInvalidState indicates that the lifecycle transition is not allowed.
	ErrLANInvalidState = errors.New("invalid LAN file state")
)

// SQLiteStore manages object state in SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLiteStore.
func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

// ObjectState represents the state of an object in the store.
type ObjectState struct {
	ID             string
	Sha256         []byte
	Size           int64
	RelativePath   string
	Status         ObjectStatus
	LastVerifiedAt *time.Time
	InodeSnapshot  *int64
	FileIDSnapShot *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// LANFile is a user-visible name that points at a verified local object.
// Object bytes remain content-addressed in local_objects.
type LANFile struct {
	ID          string     `json:"id"`
	OwnerUserID string     `json:"-"`
	ObjectID    string     `json:"object_id"`
	Name        string     `json:"name"`
	MIME        string     `json:"mime"`
	Size        int64      `json:"size"`
	SHA256      string     `json:"sha256"`
	Status      string     `json:"status"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"`
	PurgeAfter  *time.Time `json:"purge_after,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// LANPurgeTarget identifies one expired trash record without exposing content.
type LANPurgeTarget struct {
	ID          string
	OwnerUserID string
}

// sqliteTimeLayouts are the timestamp layouts produced by go-sqlite3 when
// binding a time.Time value and by SQLite's own datetime() function. Timestamps
// are stored as TEXT, so scanning must parse them back manually.
var sqliteTimeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999-07",
	"2006-01-02 15:04:05.999999999Z07:00",
	time.RFC3339Nano,
	"2006-01-02 15:04:05",
}

func parseSQLiteTime(s string) (time.Time, error) {
	for _, layout := range sqliteTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse sqlite timestamp %q", s)
}

// nowTimestamp returns the canonical TEXT timestamp used for all agent-written
// SQLite timestamps. UTC RFC 3339 with nanosecond precision sorts
// lexicographically, so SQL TEXT comparisons for staleness stay correct, and it
// parses unambiguously regardless of the SQLite driver binding time.Time.
func nowTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

// scanObjectState scans a single object row. Timestamps are stored as TEXT and
// therefore scanned into string/numeric intermediates before being converted.
func scanObjectState(scanner interface {
	Scan(dest ...interface{}) error
}) (*ObjectState, error) {
	var obj ObjectState
	var lastVerified sql.NullString
	var inodeSnap sql.NullInt64
	var fileIDSnap sql.NullString
	var createdAt, updatedAt string

	if err := scanner.Scan(
		&obj.ID, &obj.Sha256, &obj.Size, &obj.RelativePath, &obj.Status,
		&lastVerified, &inodeSnap, &fileIDSnap, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	if lastVerified.Valid {
		t, err := parseSQLiteTime(lastVerified.String)
		if err != nil {
			return nil, err
		}
		obj.LastVerifiedAt = &t
	}
	if inodeSnap.Valid {
		v := inodeSnap.Int64
		obj.InodeSnapshot = &v
	}
	if fileIDSnap.Valid {
		v := fileIDSnap.String
		obj.FileIDSnapShot = &v
	}

	created, err := parseSQLiteTime(createdAt)
	if err != nil {
		return nil, err
	}
	obj.CreatedAt = created

	updated, err := parseSQLiteTime(updatedAt)
	if err != nil {
		return nil, err
	}
	obj.UpdatedAt = updated

	return &obj, nil
}

// CreateObject creates a new object record in the store.
func (s *SQLiteStore) CreateObject(hash Hash, size int64, relativePath string, status ObjectStatus) error {
	now := nowTimestamp()
	_, err := s.db.Exec(`
		INSERT INTO local_objects (id, sha256, size, relative_path, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, hash.String(), hash[:], size, relativePath, string(status), now, now)
	if err != nil {
		return fmt.Errorf("failed to create object: %w", err)
	}
	return nil
}

// UpdateObjectStatus updates the status of an object.
func (s *SQLiteStore) UpdateObjectStatus(hash Hash, status ObjectStatus) error {
	now := nowTimestamp()
	result, err := s.db.Exec(`
		UPDATE local_objects
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, string(status), now, hash.String())
	if err != nil {
		return fmt.Errorf("failed to update object status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("object not found: %s", hash.String())
	}

	return nil
}

// UpdateObjectVerification updates the verification timestamp of an object.
func (s *SQLiteStore) UpdateObjectVerification(hash Hash) error {
	now := nowTimestamp()
	_, err := s.db.Exec(`
		UPDATE local_objects
		SET last_verified_at = ?, updated_at = ?
		WHERE id = ?
	`, now, now, hash.String())
	if err != nil {
		return fmt.Errorf("failed to update object verification: %w", err)
	}

	return nil
}

// GetObject retrieves an object by hash.
func (s *SQLiteStore) GetObject(hash Hash) (*ObjectState, error) {
	obj, err := scanObjectState(s.db.QueryRow(`
		SELECT id, sha256, size, relative_path, status, last_verified_at,
			   inode_snapshot, file_id_snapshot, created_at, updated_at
		FROM local_objects
		WHERE id = ?
	`, hash.String()))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("object not found: %s", hash.String())
		}
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	return obj, nil
}

// LookupObject retrieves an object by hash, returning (nil, nil) when absent.
func (s *SQLiteStore) LookupObject(hash Hash) (*ObjectState, error) {
	obj, err := scanObjectState(s.db.QueryRow(`
		SELECT id, sha256, size, relative_path, status, last_verified_at
			   , inode_snapshot, file_id_snapshot, created_at, updated_at
		FROM local_objects
		WHERE id = ?
	`, hash.String()))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	return obj, nil
}

// UpsertObject inserts or updates an object record, used by the import state
// machine which may re-record an object across retries without deleting valid
// data.
func (s *SQLiteStore) UpsertObject(hash Hash, size int64, relativePath string, status ObjectStatus) error {
	now := nowTimestamp()
	_, err := s.db.Exec(`
		INSERT INTO local_objects (id, sha256, size, relative_path, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			size = excluded.size,
			relative_path = excluded.relative_path,
			status = excluded.status,
			updated_at = excluded.updated_at
	`, hash.String(), hash[:], size, relativePath, string(status), now, now)
	if err != nil {
		return fmt.Errorf("failed to upsert object: %w", err)
	}
	return nil
}

// GetObjectByStatus retrieves objects by status.
func (s *SQLiteStore) GetObjectsByStatus(status ObjectStatus) ([]ObjectState, error) {
	rows, err := s.db.Query(`
		SELECT id, sha256, size, relative_path, status, last_verified_at,
			   inode_snapshot, file_id_snapshot, created_at, updated_at
		FROM local_objects
		WHERE status = ?
	`, string(status))
	if err != nil {
		return nil, fmt.Errorf("failed to get objects by status: %w", err)
	}
	defer rows.Close()

	var objects []ObjectState
	for rows.Next() {
		obj, err := scanObjectState(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan object: %w", err)
		}
		objects = append(objects, *obj)
	}

	return objects, nil
}

// DeleteObject deletes an object from the store.
func (s *SQLiteStore) DeleteObject(hash Hash) error {
	result, err := s.db.Exec("DELETE FROM local_objects WHERE id = ?", hash.String())
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("object not found: %s", hash.String())
	}

	return nil
}

// ListObjects lists all objects in the store.
func (s *SQLiteStore) ListObjects() ([]ObjectState, error) {
	rows, err := s.db.Query(`
		SELECT id, sha256, size, relative_path, status, last_verified_at,
			   inode_snapshot, file_id_snapshot, created_at, updated_at
		FROM local_objects
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects: %w", err)
	}
	defer rows.Close()

	var objects []ObjectState
	for rows.Next() {
		obj, err := scanObjectState(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan object: %w", err)
		}
		objects = append(objects, *obj)
	}

	return objects, nil
}

// GetObjectsNeedingVerification returns objects that haven't been verified recently.
func (s *SQLiteStore) GetObjectsNeedingVerification(maxAge time.Duration) ([]ObjectState, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339Nano)
	rows, err := s.db.Query(`
		SELECT id, sha256, size, relative_path, status, last_verified_at,
			   inode_snapshot, file_id_snapshot, created_at, updated_at
		FROM local_objects
		WHERE status = ? AND (last_verified_at IS NULL OR last_verified_at < ?)
	`, string(ObjectStatusReady), cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to get objects needing verification: %w", err)
	}
	defer rows.Close()

	var objects []ObjectState
	for rows.Next() {
		obj, err := scanObjectState(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan object: %w", err)
		}
		objects = append(objects, *obj)
	}

	return objects, nil
}

// GetStaleObjects returns objects that are in non-ready state for too long.
func (s *SQLiteStore) GetStaleObjects(maxAge time.Duration) ([]ObjectState, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(time.RFC3339Nano)
	rows, err := s.db.Query(`
		SELECT id, sha256, size, relative_path, status, last_verified_at,
			   inode_snapshot, file_id_snapshot, created_at, updated_at
		FROM local_objects
		WHERE status != ? AND updated_at < ?
	`, string(ObjectStatusReady), cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to get stale objects: %w", err)
	}
	defer rows.Close()

	var objects []ObjectState
	for rows.Next() {
		obj, err := scanObjectState(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan object: %w", err)
		}
		objects = append(objects, *obj)
	}

	return objects, nil
}

// BeginTransaction begins a new transaction.
func (s *SQLiteStore) BeginTransaction() (*sql.Tx, error) {
	return s.db.Begin()
}

// WithTransaction executes a function within a transaction.
func (s *SQLiteStore) WithTransaction(fn func(tx *sql.Tx) error) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			return fmt.Errorf("failed to rollback transaction: %v (original error: %w)", rbErr, err)
		}
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// CreateObjectInTransaction creates an object within a transaction.
func (s *SQLiteStore) CreateObjectInTransaction(tx *sql.Tx, hash Hash, size int64, relativePath string, status ObjectStatus) error {
	now := nowTimestamp()
	_, err := tx.Exec(`
		INSERT INTO local_objects (id, sha256, size, relative_path, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, hash.String(), hash[:], size, relativePath, string(status), now, now)
	if err != nil {
		return fmt.Errorf("failed to create object in transaction: %w", err)
	}
	return nil
}

// FinalizeObject atomically stores the canonical chunk manifest and moves an
// imported object to READY. The filesystem rename happens before this boundary;
// startup recovery can safely retry if the process exits before commit.
func (s *SQLiteStore) FinalizeObject(hash Hash, chunks []ChunkInfo, manifestDigest [32]byte) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM local_object_chunks WHERE object_id = ?`, hash.String()); err != nil {
		return fmt.Errorf("failed to clear old chunks: %w", err)
	}

	for _, c := range chunks {
		if _, err := tx.Exec(`
			INSERT INTO local_object_chunks (object_id, chunk_index, chunk_offset, chunk_size, chunk_hash)
			VALUES (?, ?, ?, ?, ?)
		`, hash.String(), c.Index, c.Offset, c.Size, c.Hash[:]); err != nil {
			return fmt.Errorf("failed to insert chunk %d: %w", c.Index, err)
		}
	}
	now := nowTimestamp()
	result, err := tx.Exec(`
		UPDATE local_objects
		SET status = ?, last_verified_at = ?, manifest_version = 1,
			manifest_digest = ?, updated_at = ?
		WHERE id = ?
	`, string(ObjectStatusReady), now, manifestDigest[:], now, hash.String())
	if err != nil {
		return fmt.Errorf("failed to finalize object manifest: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to inspect finalized object: %w", err)
	}
	if rows != 1 {
		return fmt.Errorf("object not found while finalizing: %s", hash.String())
	}

	return tx.Commit()
}

// LoadChunks retrieves the persisted chunk manifest for an object, or nil if
// no manifest has been stored (e.g. the object was imported before chunks were
// tracked).
func (s *SQLiteStore) LoadChunks(hash Hash) ([]ChunkInfo, error) {
	rows, err := s.db.Query(`
		SELECT chunk_index, chunk_offset, chunk_size, chunk_hash
		FROM local_object_chunks
		WHERE object_id = ?
		ORDER BY chunk_index ASC
	`, hash.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query chunks: %w", err)
	}
	defer rows.Close()

	var chunks []ChunkInfo
	for rows.Next() {
		var c ChunkInfo
		var hashBytes []byte
		if err := rows.Scan(&c.Index, &c.Offset, &c.Size, &hashBytes); err != nil {
			return nil, fmt.Errorf("failed to scan chunk: %w", err)
		}
		if len(hashBytes) == len(c.Hash) {
			copy(c.Hash[:], hashBytes)
		}
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// CreateLANFile publishes a logical name only when the referenced object is
// READY. The owner/name uniqueness constraint prevents silent replacement.
func (s *SQLiteStore) CreateLANFile(id, ownerUserID, objectID, name, normalizedName, mime string) (*LANFile, error) {
	return s.CreateLANFileCoordinated(context.Background(), id, ownerUserID, objectID, name, normalizedName, mime, "")
}

// CreateLANFileCoordinated publishes local metadata and its server
// registration in one SQLite transaction.
func (s *SQLiteStore) CreateLANFileCoordinated(ctx context.Context, id, ownerUserID, objectID, name, normalizedName, mime, endpoint string) (*LANFile, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin LAN file creation: %w", err)
	}
	defer tx.Rollback()
	var existing string
	err = tx.QueryRow(`SELECT id FROM lan_files WHERE owner_user_id = ? AND normalized_name = ? AND status = 'active' LIMIT 1`, ownerUserID, normalizedName).Scan(&existing)
	if err == nil {
		return nil, ErrLANNameConflict
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("check LAN file name: %w", err)
	}
	result, err := tx.Exec(`
		INSERT INTO lan_files (id, owner_user_id, object_id, name, normalized_name, mime)
		SELECT ?, ?, id, ?, ?, ?
		FROM local_objects
		WHERE id = ? AND status = 'ready'
	`, id, ownerUserID, name, normalizedName, mime, objectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create LAN file: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect LAN file insert: %w", err)
	}
	if rows != 1 {
		return nil, fmt.Errorf("object is not ready")
	}
	if endpoint != "" {
		var size int64
		if err := tx.QueryRowContext(ctx, `SELECT size FROM local_objects WHERE id = ?`, objectID).Scan(&size); err != nil {
			return nil, err
		}
		if err := enqueueOutgoingTx(ctx, tx, "catalog.register", map[string]interface{}{"local_file_id": id, "name": name, "mime": mime, "size": size, "sha256": objectID, "endpoint": endpoint}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit LAN file creation: %w", err)
	}
	return s.GetLANFile(id, ownerUserID)
}

// ActiveLANNameExists checks the serialized Agent catalog before an upload is
// accepted, avoiding transfer and orphan-object work for the common conflict.
func (s *SQLiteStore) ActiveLANNameExists(ownerUserID, normalizedName string) (bool, error) {
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM lan_files WHERE owner_user_id = ? AND normalized_name = ? AND status = 'active' LIMIT 1`, ownerUserID, normalizedName).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// ListLANFiles returns files owned by one authenticated account.
func (s *SQLiteStore) ListLANFiles(ownerUserID string) ([]LANFile, error) {
	rows, err := s.db.Query(`
		SELECT f.id, f.owner_user_id, f.object_id, f.name, f.mime,
		       o.size, lower(hex(o.sha256)), f.status, f.deleted_at,
		       f.purge_after, f.created_at, f.updated_at
		FROM lan_files f
		JOIN local_objects o ON o.id = f.object_id
		WHERE f.owner_user_id = ? AND f.status = 'active' AND o.status = 'ready'
		ORDER BY f.created_at DESC, f.id
	`, ownerUserID)
	if err != nil {
		return nil, fmt.Errorf("failed to list LAN files: %w", err)
	}
	defer rows.Close()

	files := make([]LANFile, 0)
	for rows.Next() {
		file, err := scanLANFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, *file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate LAN files: %w", err)
	}
	return files, nil
}

// GetLANFile resolves a file only inside its authenticated account.
func (s *SQLiteStore) GetLANFile(id, ownerUserID string) (*LANFile, error) {
	row := s.db.QueryRow(`
		SELECT f.id, f.owner_user_id, f.object_id, f.name, f.mime,
		       o.size, lower(hex(o.sha256)), f.status, f.deleted_at,
		       f.purge_after, f.created_at, f.updated_at
		FROM lan_files f
		JOIN local_objects o ON o.id = f.object_id
		WHERE f.id = ? AND f.owner_user_id = ? AND f.status = 'active' AND o.status = 'ready'
	`, id, ownerUserID)
	file, err := scanLANFile(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return file, nil
}

func scanLANFile(scanner interface{ Scan(...interface{}) error }) (*LANFile, error) {
	var file LANFile
	var deleted, purge sql.NullString
	var created, updated string
	if err := scanner.Scan(&file.ID, &file.OwnerUserID, &file.ObjectID, &file.Name,
		&file.MIME, &file.Size, &file.SHA256, &file.Status, &deleted, &purge,
		&created, &updated); err != nil {
		return nil, err
	}
	createdAt, err := parseSQLiteTime(created)
	if err != nil {
		return nil, err
	}
	file.CreatedAt = createdAt
	updatedAt, err := parseSQLiteTime(updated)
	if err != nil {
		return nil, err
	}
	file.UpdatedAt = updatedAt
	if deleted.Valid {
		t, err := parseSQLiteTime(deleted.String)
		if err != nil {
			return nil, err
		}
		file.DeletedAt = &t
	}
	if purge.Valid {
		t, err := parseSQLiteTime(purge.String)
		if err != nil {
			return nil, err
		}
		file.PurgeAfter = &t
	}
	return &file, nil
}

// RenameLANFile changes only the logical name of an active file.
func (s *SQLiteStore) RenameLANFile(id, ownerUserID, name, normalizedName string) (*LANFile, error) {
	return s.RenameLANFileCoordinated(context.Background(), id, ownerUserID, name, normalizedName, false)
}

func (s *SQLiteStore) RenameLANFileCoordinated(ctx context.Context, id, ownerUserID, name, normalizedName string, coordinated bool) (*LANFile, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRow(`SELECT status FROM lan_files WHERE id = ? AND owner_user_id = ?`, id, ownerUserID).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrLANFileNotFound
		}
		return nil, err
	}
	if status != "active" {
		return nil, ErrLANInvalidState
	}
	var conflict string
	err = tx.QueryRow(`SELECT id FROM lan_files WHERE owner_user_id = ? AND normalized_name = ? AND status = 'active' AND id <> ? LIMIT 1`, ownerUserID, normalizedName, id).Scan(&conflict)
	if err == nil {
		return nil, ErrLANNameConflict
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE lan_files SET name = ?, normalized_name = ?, updated_at = ? WHERE id = ?`, name, normalizedName, nowTimestamp(), id); err != nil {
		return nil, err
	}
	if coordinated {
		if err := enqueueLifecycleTx(ctx, tx, id, "rename", name, nil); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetLANFile(id, ownerUserID)
}

// TrashLANFile moves an active file into the recoverable trash.
func (s *SQLiteStore) TrashLANFile(id, ownerUserID string, retention time.Duration) (*LANFile, error) {
	return s.TrashLANFileCoordinated(context.Background(), id, ownerUserID, retention, false)
}

func (s *SQLiteStore) TrashLANFileCoordinated(ctx context.Context, id, ownerUserID string, retention time.Duration, coordinated bool) (*LANFile, error) {
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	purgeAfter := now.Add(retention)
	result, err := tx.ExecContext(ctx, `UPDATE lan_files SET status = 'trashed', deleted_at = ?, purge_after = ?, updated_at = ? WHERE id = ? AND owner_user_id = ? AND status = 'active'`, now.Format(time.RFC3339Nano), purgeAfter.Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), id, ownerUserID)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, ErrLANFileNotFound
	}
	if coordinated {
		if err := enqueueLifecycleTx(ctx, tx, id, "trash", "", &purgeAfter); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetLANTrashFile(id, ownerUserID)
}

// ListLANTrash returns recoverable and in-progress purge records.
func (s *SQLiteStore) ListLANTrash(ownerUserID string) ([]LANFile, error) {
	rows, err := s.db.Query(`SELECT f.id, f.owner_user_id, f.object_id, f.name, f.mime, o.size, lower(hex(o.sha256)), f.status, f.deleted_at, f.purge_after, f.created_at, f.updated_at FROM lan_files f JOIN local_objects o ON o.id = f.object_id WHERE f.owner_user_id = ? AND f.status IN ('trashed','purging') ORDER BY f.deleted_at DESC, f.id`, ownerUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := make([]LANFile, 0)
	for rows.Next() {
		file, err := scanLANFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, *file)
	}
	return files, rows.Err()
}

// GetLANTrashFile resolves a non-active entry within one account.
func (s *SQLiteStore) GetLANTrashFile(id, ownerUserID string) (*LANFile, error) {
	row := s.db.QueryRow(`SELECT f.id, f.owner_user_id, f.object_id, f.name, f.mime, o.size, lower(hex(o.sha256)), f.status, f.deleted_at, f.purge_after, f.created_at, f.updated_at FROM lan_files f JOIN local_objects o ON o.id = f.object_id WHERE f.id = ? AND f.owner_user_id = ? AND f.status IN ('trashed','purging')`, id, ownerUserID)
	file, err := scanLANFile(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return file, err
}

// RestoreLANFile returns a trashed file to the active list without overwriting.
func (s *SQLiteStore) RestoreLANFile(id, ownerUserID string) (*LANFile, error) {
	return s.RestoreLANFileCoordinated(context.Background(), id, ownerUserID, false)
}

func (s *SQLiteStore) RestoreLANFileCoordinated(ctx context.Context, id, ownerUserID string, coordinated bool) (*LANFile, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var normalized, status string
	if err := tx.QueryRow(`SELECT normalized_name, status FROM lan_files WHERE id = ? AND owner_user_id = ?`, id, ownerUserID).Scan(&normalized, &status); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrLANFileNotFound
		}
		return nil, err
	}
	if status != "trashed" {
		return nil, ErrLANInvalidState
	}
	var conflict string
	err = tx.QueryRow(`SELECT id FROM lan_files WHERE owner_user_id = ? AND normalized_name = ? AND status = 'active' LIMIT 1`, ownerUserID, normalized).Scan(&conflict)
	if err == nil {
		return nil, ErrLANNameConflict
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE lan_files SET status = 'active', deleted_at = NULL, purge_after = NULL, updated_at = ? WHERE id = ?`, nowTimestamp(), id); err != nil {
		return nil, err
	}
	if coordinated {
		if err := enqueueLifecycleTx(ctx, tx, id, "restore", "", nil); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetLANFile(id, ownerUserID)
}

func enqueueLifecycleTx(ctx context.Context, tx *sql.Tx, localFileID, action, name string, purgeAfter *time.Time) error {
	_, err := enqueueLifecycleWithStatusTx(ctx, tx, localFileID, action, name, purgeAfter, "pending")
	return err
}

func enqueueLifecycleWithStatusTx(ctx context.Context, tx *sql.Tx, localFileID, action, name string, purgeAfter *time.Time, status string) (string, error) {
	request := map[string]interface{}{"local_file_id": localFileID, "action": action}
	if name != "" {
		request["name"] = name
	}
	if purgeAfter != nil {
		request["purge_after"] = purgeAfter.UTC()
	}
	return enqueueOutgoingWithStatusTx(ctx, tx, "catalog.lifecycle", request, status)
}

// EnqueueLifecycle persists a lifecycle report when the filesystem portion of
// a local saga has completed (currently permanent purge).
func (s *SQLiteStore) EnqueueLifecycle(ctx context.Context, localFileID, action, name string, purgeAfter *time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := enqueueLifecycleTx(ctx, tx, localFileID, action, name, purgeAfter); err != nil {
		return err
	}
	return tx.Commit()
}

func enqueueOutgoingTx(ctx context.Context, tx *sql.Tx, opType string, request map[string]interface{}) error {
	_, err := enqueueOutgoingWithStatusTx(ctx, tx, opType, request, "pending")
	return err
}

func enqueueOutgoingWithStatusTx(ctx context.Context, tx *sql.Tx, opType string, request map[string]interface{}, status string) (string, error) {
	opID := uuid.NewString()
	if opType == "catalog.lifecycle" {
		request["operation_id"] = opID
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().Format("2006-01-02 15:04:05")
	_, err = tx.ExecContext(ctx, `INSERT INTO outgoing_ops (id, op_type, idempotency_key, request_data, status, retry_count, created_at, updated_at) VALUES (?, ?, ?, ?, ?, 0, ?, ?)`, opID, opType, opID, payload, status, now, now)
	if err != nil {
		return "", fmt.Errorf("enqueue control operation: %w", err)
	}
	return opID, nil
}

// BeginPurgeLANFile makes a trash record inaccessible and reports whether it
// is the last logical reference to its content-addressed object.
func (s *SQLiteStore) BeginPurgeLANFile(id, ownerUserID string) (*LANFile, bool, error) {
	file, last, _, err := s.beginPurgeLANFile(context.Background(), id, ownerUserID, false)
	return file, last, err
}

// BeginPurgeLANFileCoordinated durably records a blocked server report in the
// same transaction that makes the local entry inaccessible. The report is
// activated only after byte deletion succeeds.
func (s *SQLiteStore) BeginPurgeLANFileCoordinated(ctx context.Context, id, ownerUserID string) (*LANFile, bool, string, error) {
	return s.beginPurgeLANFile(ctx, id, ownerUserID, true)
}

func (s *SQLiteStore) beginPurgeLANFile(ctx context.Context, id, ownerUserID string, coordinated bool) (*LANFile, bool, string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, "", err
	}
	defer tx.Rollback()
	row := tx.QueryRow(`SELECT f.id, f.owner_user_id, f.object_id, f.name, f.mime, o.size, lower(hex(o.sha256)), f.status, f.deleted_at, f.purge_after, f.created_at, f.updated_at FROM lan_files f JOIN local_objects o ON o.id = f.object_id WHERE f.id = ? AND f.owner_user_id = ?`, id, ownerUserID)
	file, err := scanLANFile(row)
	if err == sql.ErrNoRows {
		return nil, false, "", ErrLANFileNotFound
	}
	if err != nil {
		return nil, false, "", err
	}
	if file.Status != "trashed" && file.Status != "purging" {
		return nil, false, "", ErrLANInvalidState
	}
	if file.Status == "trashed" {
		if _, err := tx.Exec(`UPDATE lan_files SET status = 'purging', updated_at = ? WHERE id = ?`, nowTimestamp(), id); err != nil {
			return nil, false, "", err
		}
		file.Status = "purging"
	}
	var otherRefs int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM lan_files WHERE object_id = ? AND id <> ?`, file.ObjectID, id).Scan(&otherRefs); err != nil {
		return nil, false, "", err
	}
	var opID string
	if coordinated {
		opID, err = enqueueLifecycleWithStatusTx(ctx, tx, id, "purge", "", nil, "processing")
		if err != nil {
			return nil, false, "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, false, "", err
	}
	return file, otherRefs == 0, opID, nil
}

func (s *SQLiteStore) ActivateOutgoing(ctx context.Context, opID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE outgoing_ops SET status='pending',updated_at=datetime('now') WHERE id=? AND status='processing'`, opID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("purge report intent not found")
	}
	return nil
}

// FinalizePurgeLANFile removes a logical row when another reference keeps the object alive.
func (s *SQLiteStore) FinalizePurgeLANFile(id, ownerUserID string) error {
	result, err := s.db.Exec(`DELETE FROM lan_files WHERE id = ? AND owner_user_id = ? AND status = 'purging'`, id, ownerUserID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrLANFileNotFound
	}
	return nil
}

// ListExpiredLANTrash returns trash records whose retention deadline elapsed.
func (s *SQLiteStore) ListExpiredLANTrash(now time.Time) ([]LANPurgeTarget, error) {
	rows, err := s.db.Query(`SELECT id, owner_user_id FROM lan_files WHERE status IN ('trashed','purging') AND purge_after <= ? ORDER BY purge_after, id`, now.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []LANPurgeTarget
	for rows.Next() {
		var target LANPurgeTarget
		if err := rows.Scan(&target.ID, &target.OwnerUserID); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

// UpdateObjectStatusInTransaction updates object status within a transaction.
func (s *SQLiteStore) UpdateObjectStatusInTransaction(tx *sql.Tx, hash Hash, status ObjectStatus) error {
	now := nowTimestamp()
	_, err := tx.Exec(`
		UPDATE local_objects
		SET status = ?, updated_at = ?
		WHERE id = ?
	`, string(status), now, hash.String())
	if err != nil {
		return fmt.Errorf("failed to update object status in transaction: %w", err)
	}
	return nil
}
