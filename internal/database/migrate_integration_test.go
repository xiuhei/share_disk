//go:build integration

package database_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/share-disk/share-disk/internal/testutil"
)

// 3.5.5 / P0-11: applying 002 on top of an existing 001 database must preserve
// existing data and backfill user_id on child tables.
func TestMigration001To002WithData(t *testing.T) {
	db := testutil.NewEmptyDB(t)
	db.MigrateUpTo(t, 1)

	userID := uuid.NewString()
	deviceID := uuid.NewString()
	objectID := uuid.NewString()
	folderID := uuid.NewString()
	entryID := uuid.NewString()
	replicaID := uuid.NewString()
	taskID := uuid.NewString()
	trashID := uuid.NewString()

	mustExec := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}

	mustExec(`INSERT INTO users (id, account, password_hash, status, created_at, updated_at) VALUES ($1, 'admin', 'h', 'active', NOW(), NOW())`, userID)
	mustExec(`INSERT INTO devices (id, user_id, name, platform, public_key, peer_id, status, version, created_at, updated_at) VALUES ($1, $2, 'd', 'linux', $3, $4, 'active', 1, NOW(), NOW())`, deviceID, userID, []byte("pk"), "peer-1")
	mustExec(`INSERT INTO file_objects (id, user_id, sha256, size, mime, chunk_size, chunk_count, status, created_at, updated_at) VALUES ($1, $2, $3, 1234, 'application/octet-stream', 4194304, 1, 'ready', NOW(), NOW())`, objectID, userID, []byte("sha"))
	mustExec(`INSERT INTO folders (id, user_id, parent_id, name, normalized_name, status, version, created_at, updated_at) VALUES ($1, $2, NULL, 'root', 'root', 'active', 1, NOW(), NOW())`, folderID, userID)
	mustExec(`INSERT INTO file_entries (id, user_id, folder_id, object_id, name, normalized_name, status, version, created_at, updated_at) VALUES ($1, $2, $3, $4, 'f', 'f', 'active', 1, NOW(), NOW())`, entryID, userID, folderID, objectID)
	mustExec(`INSERT INTO replicas (id, object_id, device_id, state, version, created_at, updated_at) VALUES ($1, $2, $3, 'pending', 1, NOW(), NOW())`, replicaID, objectID, deviceID)
	mustExec(`INSERT INTO transfer_tasks (id, user_id, object_id, target_device_id, reason, state, priority, attempt, version, created_at, updated_at) VALUES ($1, $2, $3, $4, 'pull', 'queued', 0, 0, 1, NOW(), NOW())`, taskID, userID, objectID, deviceID)
	mustExec(`INSERT INTO transfer_sources (task_id, source_device_id, rank, created_at, updated_at) VALUES ($1, $2, 0, NOW(), NOW())`, taskID, deviceID)
	mustExec(`INSERT INTO trash_records (id, file_entry_id, original_folder_id, original_name, deleted_at, purge_after, status, created_at, updated_at) VALUES ($1, $2, $3, 'f', NOW(), NOW(), 'trashed', NOW(), NOW())`, trashID, entryID, folderID)

	// Upgrade to 002.
	db.MigrateUpTo(t, 2)

	// Backfilled user_id must equal the parent's user_id.
	assertUserID := func(table, id string, want string) {
		t.Helper()
		var got string
		if err := db.QueryRow(`SELECT user_id::text FROM `+table+` WHERE id = $1`, id).Scan(&got); err != nil {
			t.Fatalf("query %s.user_id: %v", table, err)
		}
		if got != want {
			t.Fatalf("%s.user_id = %s, want %s", table, got, want)
		}
	}
	assertUserID("replicas", replicaID, userID)
	assertUserID("trash_records", trashID, userID)

	// transfer_sources has no id column; assert via task_id.
	var srcUser string
	if err := db.QueryRow(`SELECT user_id::text FROM transfer_sources WHERE task_id = $1`, taskID).Scan(&srcUser); err != nil {
		t.Fatalf("query transfer_sources.user_id: %v", err)
	}
	if srcUser != userID {
		t.Fatalf("transfer_sources.user_id = %s, want %s", srcUser, userID)
	}

	// Existing data survives.
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM file_entries WHERE id = $1`, entryID).Scan(&count); err != nil {
		t.Fatalf("count file_entries: %v", err)
	}
	if count != 1 {
		t.Fatalf("file entry missing after upgrade")
	}
}

// P0-11: after 002, the composite foreign keys must reject cross-account rows.
func TestCrossAccountForeignKeyEnforced(t *testing.T) {
	db := testutil.NewEmptyDB(t)
	db.MigrateUpTo(t, 2)

	u1 := uuid.NewString()
	u2 := uuid.NewString()
	d1 := uuid.NewString()
	d2 := uuid.NewString()
	o1 := uuid.NewString()

	mustExec := func(q string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatalf("exec: %v\n%s", err, q)
		}
	}
	mustExec(`INSERT INTO users (id, account, password_hash, status, created_at, updated_at) VALUES ($1, 'a', 'h', 'active', NOW(), NOW())`, u1)
	mustExec(`INSERT INTO users (id, account, password_hash, status, created_at, updated_at) VALUES ($1, 'b', 'h', 'active', NOW(), NOW())`, u2)
	mustExec(`INSERT INTO devices (id, user_id, name, platform, public_key, peer_id, status, version, created_at, updated_at) VALUES ($1, $2, 'd1', 'linux', 'pk1', 'peer-1', 'active', 1, NOW(), NOW())`, d1, u1)
	mustExec(`INSERT INTO devices (id, user_id, name, platform, public_key, peer_id, status, version, created_at, updated_at) VALUES ($1, $2, 'd2', 'linux', 'pk2', 'peer-2', 'active', 1, NOW(), NOW())`, d2, u2)
	mustExec(`INSERT INTO file_objects (id, user_id, sha256, size, mime, chunk_size, chunk_count, status, created_at, updated_at) VALUES ($1, $2, 'sha', 1, 'application/octet-stream', 4194304, 1, 'ready', NOW(), NOW())`, o1, u1)

	// A replica whose device belongs to a different account must be rejected by
	// the composite FK replicas_device_same_account_fk.
	_, err := db.Exec(`INSERT INTO replicas (id, object_id, device_id, user_id, state, version, created_at, updated_at) VALUES ($1, $2, $3, $4, 'pending', 1, NOW(), NOW())`, uuid.NewString(), o1, d2, u1)
	if err == nil {
		t.Fatal("expected cross-account replica insert to be rejected by FK")
	}
}
