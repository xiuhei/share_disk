// Package testutil provides helpers for integration tests that require a real
// PostgreSQL instance. Helpers connect to an admin DSN, create an isolated
// database per test, apply migrations, and fail closed when no database is
// configured. These helpers are intended for tests carrying the `integration`
// build tag.
package testutil

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/share-disk/share-disk/internal/database"
)

// AdminDSNEnv names the environment variable holding a PostgreSQL admin DSN
// (URL form, e.g. postgres://user:pass@host:port/postgres?sslmode=disable) with
// permission to CREATE/DROP DATABASE. Integration tests fail closed when it is
// unset or unreachable.
const AdminDSNEnv = "SHARE_DISK_TEST_POSTGRESQL_URL"

// DB is an isolated PostgreSQL database with migrations applied.
type DB struct {
	*sql.DB
	name string
}

// NewDB connects to PostgreSQL, creates an isolated database, applies all
// migrations, and returns it. Cleanup (drop database) is registered with t.
func NewDB(t *testing.T) *DB {
	t.Helper()
	d := NewEmptyDB(t)
	d.MigrateUpTo(t, latestVersion(d.Migrator(), t))
	return d
}

// NewEmptyDB connects to PostgreSQL and creates an isolated database without
// applying migrations, so callers can control the migration sequence (e.g. for
// upgrade tests). Cleanup (drop database) is registered with t.
func NewEmptyDB(t *testing.T) *DB {
	t.Helper()

	adminDSN := os.Getenv(AdminDSNEnv)
	if adminDSN == "" {
		t.Fatalf("%s is not set: integration tests require a real PostgreSQL", AdminDSNEnv)
	}

	admin, err := sql.Open("postgres", adminDSN)
	if err != nil {
		t.Fatalf("failed to open admin connection: %v", err)
	}
	if err := admin.Ping(); err != nil {
		admin.Close()
		t.Fatalf("failed to reach PostgreSQL (%s): %v", AdminDSNEnv, err)
	}

	name := fmt.Sprintf("share_disk_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + quoteIdent(name)); err != nil {
		admin.Close()
		t.Fatalf("failed to create test database: %v", err)
	}
	admin.Close()

	testDSN, err := dsnForDatabase(adminDSN, name)
	if err != nil {
		t.Fatalf("failed to derive test DSN: %v", err)
	}

	db, err := sql.Open("postgres", testDSN)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	d := &DB{DB: db, name: name}

	t.Cleanup(func() {
		_ = db.Close()
		admin, err := sql.Open("postgres", adminDSN)
		if err != nil {
			return
		}
		defer admin.Close()
		_, _ = admin.Exec("DROP DATABASE IF EXISTS " + quoteIdent(name))
	})

	return d
}

// latestVersion returns the highest available migration version.
func latestVersion(m *database.Migrator, t *testing.T) int {
	v, err := m.LatestVersion()
	if err != nil {
		t.Fatalf("failed to determine latest migration version: %v", err)
	}
	return v
}

// Migrator returns a Migrator targeting the PostgreSQL migrations directory.
func (d *DB) Migrator() *database.Migrator {
	return database.NewMigrator(d.DB, PostgresMigrationsDir(), "postgres")
}

// MigrateUpTo applies pending PostgreSQL migrations up to the given version.
func (d *DB) MigrateUpTo(t *testing.T, version int) {
	t.Helper()
	if err := d.Migrator().MigrateUpTo(version); err != nil {
		t.Fatalf("failed to migrate up to version %d: %v", version, err)
	}
}

// PostgresMigrationsDir returns the repository PostgreSQL migrations directory,
// locating it by walking up from the current working directory.
func PostgresMigrationsDir() string {
	return migrationsDirFor("postgres")
}

// SQLiteMigrationsDir returns the repository SQLite migrations directory.
func SQLiteMigrationsDir() string {
	return migrationsDirFor("sqlite")
}

func migrationsDirFor(dialect string) string {
	dir, _ := os.Getwd()
	for i := 0; i < 10; i++ {
		candidate := filepath.Join(dir, "migrations", dialect)
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return filepath.Join("migrations", dialect)
}

// quoteIdent quotes a database name as a SQL identifier.
func quoteIdent(s string) string {
	return `"` + s + `"`
}

// dsnForDatabase returns a copy of the admin URL DSN with its database path
// replaced by dbname.
func dsnForDatabase(adminDSN, dbname string) (string, error) {
	u, err := url.Parse(adminDSN)
	if err != nil {
		return "", fmt.Errorf("parse admin DSN: %w", err)
	}
	u.Path = "/" + dbname
	return u.String(), nil
}
