package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Migrator handles database migrations.
type Migrator struct {
	db            *sql.DB
	migrationsDir string
	dialect       string // "postgres" or "sqlite"
}

// NewMigrator creates a new Migrator.
func NewMigrator(db *sql.DB, migrationsDir, dialect string) *Migrator {
	return &Migrator{
		db:            db,
		migrationsDir: migrationsDir,
		dialect:       dialect,
	}
}

// Migration represents a single migration.
type Migration struct {
	Version int
	Up      string
	Down    string
}

// LoadMigrations loads all migrations from the migrations directory.
func (m *Migrator) LoadMigrations() ([]Migration, error) {
	files, err := os.ReadDir(m.migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	migrationMap := make(map[int]*Migration)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Parse version from filename (e.g., "001_initial_schema.up.sql")
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}

		var version int
		if _, err := fmt.Sscanf(parts[0], "%d", &version); err != nil {
			continue
		}

		content, err := os.ReadFile(filepath.Join(m.migrationsDir, name))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", name, err)
		}

		if migrationMap[version] == nil {
			migrationMap[version] = &Migration{Version: version}
		}

		if strings.HasSuffix(name, ".up.sql") {
			migrationMap[version].Up = string(content)
		} else if strings.HasSuffix(name, ".down.sql") {
			migrationMap[version].Down = string(content)
		}
	}

	// Convert map to sorted slice
	migrations := make([]Migration, 0, len(migrationMap))
	for _, migration := range migrationMap {
		migrations = append(migrations, *migration)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// LatestVersion returns the highest migration version available on disk.
func (m *Migrator) LatestVersion() (int, error) {
	migrations, err := m.LoadMigrations()
	if err != nil {
		return 0, err
	}
	if len(migrations) == 0 {
		return 0, nil
	}
	return migrations[len(migrations)-1].Version, nil
}

// GetCurrentVersion returns the current migration version from the database.
func (m *Migrator) GetCurrentVersion() (int, error) {
	// First check if schema_migrations table exists
	var exists bool
	if m.dialect == "sqlite" {
		err := m.db.QueryRow("SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&exists)
		if err != nil {
			return 0, fmt.Errorf("failed to check if schema_migrations table exists: %w", err)
		}
	} else {
		err := m.db.QueryRow("SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'schema_migrations')").Scan(&exists)
		if err != nil {
			return 0, fmt.Errorf("failed to check if schema_migrations table exists: %w", err)
		}
	}

	if !exists {
		return 0, nil
	}

	var version int
	var dirty bool

	err := m.db.QueryRow("SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version, &dirty)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to get current version: %w", err)
	}

	if dirty {
		return 0, fmt.Errorf("database is in dirty state at version %d", version)
	}

	return version, nil
}

// MigrateUp applies all pending migrations.
func (m *Migrator) MigrateUp() error {
	return m.MigrateUpTo(int(^uint(0) >> 1))
}

// MigrateUpTo applies pending migrations up to and including the given version.
// Migrations above the target version are left unapplied.
func (m *Migrator) MigrateUpTo(version int) error {
	migrations, err := m.LoadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Ensure schema_migrations table exists
	if err := m.ensureSchemaMigrationsTable(); err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}

	currentVersion, err := m.GetCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	for _, migration := range migrations {
		if migration.Version <= currentVersion {
			continue
		}
		if migration.Version > version {
			break
		}

		if migration.Up == "" {
			return fmt.Errorf("missing up migration for version %d", migration.Version)
		}

		fmt.Printf("Applying migration %d...\n", migration.Version)

		tx, err := m.db.Begin()
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}

		// Mark migration as dirty
		_, err = tx.Exec("INSERT INTO schema_migrations (version, dirty) VALUES ($1, TRUE)", migration.Version)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to mark migration as dirty: %w", err)
		}

		// Apply migration
		_, err = tx.Exec(migration.Up)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
		}

		// Mark migration as clean
		_, err = tx.Exec("UPDATE schema_migrations SET dirty = FALSE WHERE version = $1", migration.Version)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to mark migration as clean: %w", err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %d: %w", migration.Version, err)
		}
	}

	return nil
}

// ensureSchemaMigrationsTable creates the schema_migrations table if it doesn't exist.
func (m *Migrator) ensureSchemaMigrationsTable() error {
	var createSQL string
	if m.dialect == "sqlite" {
		createSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			dirty INTEGER NOT NULL DEFAULT 0,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`
	} else {
		createSQL = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			dirty BOOLEAN NOT NULL DEFAULT FALSE,
			applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)`
	}

	_, err := m.db.Exec(createSQL)
	if err != nil {
		return fmt.Errorf("failed to create schema_migrations table: %w", err)
	}

	return nil
}

// MigrateDown rolls back the last migration.
func (m *Migrator) MigrateDown() error {
	migrations, err := m.LoadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	currentVersion, err := m.GetCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	if currentVersion == 0 {
		fmt.Println("No migrations to roll back")
		return nil
	}

	// Find the migration to roll back
	var targetMigration *Migration
	for i := len(migrations) - 1; i >= 0; i-- {
		if migrations[i].Version == currentVersion {
			targetMigration = &migrations[i]
			break
		}
	}

	if targetMigration == nil {
		return fmt.Errorf("migration %d not found", currentVersion)
	}

	if targetMigration.Down == "" {
		return fmt.Errorf("missing down migration for version %d", currentVersion)
	}

	fmt.Printf("Rolling back migration %d...\n", currentVersion)

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Apply down migration
	_, err = tx.Exec(targetMigration.Down)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to roll back migration %d: %w", currentVersion, err)
	}

	// Remove migration record
	_, err = tx.Exec("DELETE FROM schema_migrations WHERE version = $1", currentVersion)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit rollback: %w", err)
	}

	return nil
}
