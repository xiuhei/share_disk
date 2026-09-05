package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/share-disk/share-disk/internal/database"
	_ "modernc.org/sqlite"
)

func newTestSQLiteObjectStore(t *testing.T) (*SQLiteObjectStore, string) {
	t.Helper()

	root := t.TempDir()
	st, err := New(root)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("failed to enable WAL: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	migrator := database.NewMigrator(db, filepath.Join("..", "..", "migrations", "sqlite"), "sqlite")
	if err := migrator.MigrateUp(); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}

	store := NewSQLiteObjectStore(st, db, 0)
	if err := store.Init(context.Background()); err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	return store, root
}

func writeSource(t *testing.T, content []byte) string {
	t.Helper()
	src := filepath.Join(t.TempDir(), "source.bin")
	if err := os.WriteFile(src, content, 0600); err != nil {
		t.Fatalf("failed to write source: %v", err)
	}
	return src
}

func TestSQLiteObjectStoreImportAndGetObject(t *testing.T) {
	store, _ := newTestSQLiteObjectStore(t)
	content := []byte("hello sqlite object store")
	info, err := store.Import(context.Background(), writeSource(t, content))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if info.Status != ObjectStatusReady {
		t.Fatalf("expected ready, got %s", info.Status)
	}

	got, err := store.GetObject(info.Hash)
	if err != nil {
		t.Fatalf("get object failed: %v", err)
	}
	if got.Status != ObjectStatusReady {
		t.Fatalf("expected ready from sqlite, got %s", got.Status)
	}

	// ListObjects must also reflect SQLite state.
	objs, err := store.ListObjects()
	if err != nil {
		t.Fatalf("list objects failed: %v", err)
	}
	if len(objs) != 1 || objs[0].Status != ObjectStatusReady {
		t.Fatalf("expected one ready object, got %+v", objs)
	}
}

// P0-03: GetObject must not infer READY from file existence; it reads the
// authoritative SQLite status.
func TestSQLiteObjectStoreGetObjectUsesSQLiteStatus(t *testing.T) {
	store, _ := newTestSQLiteObjectStore(t)
	info, err := store.Import(context.Background(), writeSource(t, []byte("content")))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Flip the SQLite status away from ready while the file is still present.
	if err := store.store.UpdateObjectStatus(info.Hash, ObjectStatusImporting); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	got, err := store.GetObject(info.Hash)
	if err != nil {
		t.Fatalf("get object failed: %v", err)
	}
	if got.Status != ObjectStatusImporting {
		t.Fatalf("expected importing status from sqlite, got %s", got.Status)
	}
}

// P0-02: a READY record whose file disappeared must be downgraded to MISSING on
// startup recovery, not left permanently READY.
func TestSQLiteObjectStoreStartupDowngradesMissingReady(t *testing.T) {
	store, root := newTestSQLiteObjectStore(t)
	info, err := store.Import(context.Background(), writeSource(t, []byte("content")))
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Remove the backing file.
	objectPath := filepath.Join(root, "objects", info.Hash.String()[:2], info.Hash.String())
	if err := os.Remove(objectPath); err != nil {
		t.Fatalf("failed to remove object file: %v", err)
	}

	if err := store.ScanAndRepair(context.Background()); err != nil {
		t.Fatalf("scan and repair failed: %v", err)
	}

	got, err := store.GetObject(info.Hash)
	if err != nil {
		t.Fatalf("get object failed: %v", err)
	}
	if got.Status != ObjectStatusMissing {
		t.Fatalf("expected missing after recovery, got %s", got.Status)
	}
}

// P0-04: a duplicate import must not return READY when the persisted file is
// corrupt; it must detect the mismatch and re-import.
func TestSQLiteObjectStoreDuplicateImportRepairsCorruptFile(t *testing.T) {
	store, root := newTestSQLiteObjectStore(t)
	content := []byte("durable object content")
	src := writeSource(t, content)
	first, err := store.Import(context.Background(), src)
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Corrupt the backing file.
	objectPath := filepath.Join(root, "objects", first.Hash.String()[:2], first.Hash.String())
	if err := os.WriteFile(objectPath, []byte("corrupted"), 0600); err != nil {
		t.Fatalf("failed to corrupt object: %v", err)
	}

	second, err := store.Import(context.Background(), src)
	if err != nil {
		t.Fatalf("re-import failed: %v", err)
	}
	if second.Hash != first.Hash {
		t.Fatalf("hash mismatch: %s != %s", second.Hash, first.Hash)
	}

	got, err := store.GetObject(first.Hash)
	if err != nil {
		t.Fatalf("get object failed: %v", err)
	}
	if got.Status != ObjectStatusReady {
		t.Fatalf("expected ready after repair, got %s", got.Status)
	}

	data, err := os.ReadFile(objectPath)
	if err != nil {
		t.Fatalf("failed to read repaired object: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("repaired content mismatch")
	}
}
