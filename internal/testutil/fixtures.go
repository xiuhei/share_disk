package testutil

import (
	"crypto/rand"
	"testing"

	"github.com/google/uuid"
)

// InsertUser inserts an active user and returns its id. It bypasses the
// bootstrap flow so cross-account tests can create a second account.
func InsertUser(t *testing.T, db *DB, account string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`
		INSERT INTO users (id, account, password_hash, status, created_at, updated_at)
		VALUES ($1, $2, 'unused', 'active', NOW(), NOW())
	`, id, account); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	return id
}

// InsertDevice inserts an active device for the given user and returns its id.
func InsertDevice(t *testing.T, db *DB, userID string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`
		INSERT INTO devices (id, user_id, name, platform, public_key, peer_id, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, 'linux', $4, $5, 'active', 1, NOW(), NOW())
	`, id, userID, "device-"+id, randomBytes(t, 32), "peer-"+id); err != nil {
		t.Fatalf("insert device: %v", err)
	}
	return id
}

// InsertObject inserts a ready file_object for the given user and returns its id.
func InsertObject(t *testing.T, db *DB, userID string, size int64) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`
		INSERT INTO file_objects (id, user_id, sha256, size, mime, chunk_size, chunk_count, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, 'application/octet-stream', 4194304, 1, 'ready', NOW(), NOW())
	`, id, userID, randomBytes(t, 32), size); err != nil {
		t.Fatalf("insert object: %v", err)
	}
	return id
}

// InsertFolder inserts an active root folder for the given user and returns its id.
func InsertFolder(t *testing.T, db *DB, userID string) string {
	t.Helper()
	id := uuid.NewString()
	name := "folder-" + id
	if _, err := db.Exec(`
		INSERT INTO folders (id, user_id, parent_id, name, normalized_name, status, version, created_at, updated_at)
		VALUES ($1, $2, NULL, $3, $3, 'active', 1, NOW(), NOW())
	`, id, userID, name); err != nil {
		t.Fatalf("insert folder: %v", err)
	}
	return id
}

// InsertFileEntry inserts an active file entry for the given user and returns
// its id.
func InsertFileEntry(t *testing.T, db *DB, userID, folderID, objectID, name string) string {
	t.Helper()
	id := uuid.NewString()
	if _, err := db.Exec(`
		INSERT INTO file_entries (id, user_id, folder_id, object_id, name, normalized_name, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5, 'active', 1, NOW(), NOW())
	`, id, userID, folderID, objectID, name); err != nil {
		t.Fatalf("insert file entry: %v", err)
	}
	return id
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("random bytes: %v", err)
	}
	return b
}
