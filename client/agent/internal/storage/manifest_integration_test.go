package storage

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/share-disk/share-disk/internal/database"
)

func TestImportPersistsCanonicalManifestAtReadyBoundary(t *testing.T) {
	root := t.TempDir()
	db, err := database.OpenSQLite(filepath.Join(root, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	migrations := filepath.Join("..", "..", "migrations", "sqlite")
	if err := database.NewMigrator(db, migrations, "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	objectRoot := filepath.Join(root, "objects")
	storage, err := New(objectRoot)
	if err != nil {
		t.Fatal(err)
	}
	store := NewSQLiteObjectStore(storage, db, 4)
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.bin")
	if err := os.WriteFile(source, []byte("abcdef"), 0600); err != nil {
		t.Fatal(err)
	}
	info, err := store.Import(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	var status string
	var version int
	var digest []byte
	if err := db.QueryRow(`SELECT status, manifest_version, manifest_digest FROM local_objects WHERE id = ?`, info.Hash.String()).Scan(&status, &version, &digest); err != nil {
		t.Fatal(err)
	}
	if status != string(ObjectStatusReady) || version != 1 {
		t.Fatalf("final state = %s manifest v%d", status, version)
	}
	const expected = "5f471dd3266cda5d000188bad27a55ec2bd34b2a591458f01c96731ce054acc1"
	if got := hex.EncodeToString(digest); got != expected {
		t.Fatalf("manifest digest = %s, want %s", got, expected)
	}
	var chunks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM local_object_chunks WHERE object_id = ?`, info.Hash.String()).Scan(&chunks); err != nil {
		t.Fatal(err)
	}
	if chunks != 2 {
		t.Fatalf("persisted chunks = %d, want 2", chunks)
	}
}
