package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSQLiteMigrations(t *testing.T) {
	// Create a temporary directory for test database
	tempDir, err := os.MkdirTemp("", "share-disk-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create SQLite database
	dbPath := filepath.Join(tempDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Enable WAL mode and foreign keys
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("Failed to enable WAL mode: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	// Create migrator
	migrationsDir := filepath.Join("..", "..", "migrations", "sqlite")
	migrator := NewMigrator(db, migrationsDir, "sqlite")

	// Test loading migrations
	migrations, err := migrator.LoadMigrations()
	if err != nil {
		t.Fatalf("Failed to load migrations: %v", err)
	}

	if len(migrations) == 0 {
		t.Fatal("No migrations found")
	}

	t.Logf("Found %d migrations", len(migrations))

	// Test getting current version (should be 0 for empty database)
	version, err := migrator.GetCurrentVersion()
	if err != nil {
		t.Fatalf("Failed to get current version: %v", err)
	}

	if version != 0 {
		t.Fatalf("Expected version 0, got %d", version)
	}

	// Test applying migrations
	if err := migrator.MigrateUp(); err != nil {
		t.Fatalf("Failed to apply migrations: %v", err)
	}

	// Test getting current version after migration
	version, err = migrator.GetCurrentVersion()
	if err != nil {
		t.Fatalf("Failed to get current version after migration: %v", err)
	}

	if version == 0 {
		t.Fatal("Expected version > 0 after migration")
	}

	t.Logf("Current version after migration: %d", version)

	// Test that tables were created
	tables := []string{
		"local_objects",
		"local_replicas",
		"local_transfers",
		"transfer_chunks",
		"catalog_cache",
		"outgoing_ops",
		"sync_state",
		"settings",
		"local_object_chunks",
		"schema_migrations",
	}

	for _, table := range tables {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		if err != nil {
			t.Errorf("Table %s not found or error: %v", table, err)
		} else {
			t.Logf("Table %s exists with %d rows", table, count)
		}
	}

	// Test rolling back all migrations (one MigrateDown per applied migration)
	for {
		currentVer, err := migrator.GetCurrentVersion()
		if err != nil {
			t.Fatalf("Failed to get current version during rollback: %v", err)
		}
		if currentVer == 0 {
			break
		}
		if err := migrator.MigrateDown(); err != nil {
			t.Fatalf("Failed to roll back migration from version %d: %v", currentVer, err)
		}
	}

	// Test getting current version after full rollback
	version, err = migrator.GetCurrentVersion()
	if err != nil {
		t.Fatalf("Failed to get current version after rollback: %v", err)
	}

	if version != 0 {
		t.Fatalf("Expected version 0 after rollback, got %d", version)
	}
}
