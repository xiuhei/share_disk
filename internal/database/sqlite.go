package database

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// OpenSQLite opens a SQLite database with the connection pragmas required by
// the agent: WAL journal mode, foreign keys enabled on every connection, and a
// busy timeout. Unlike PRAGMA statements run inside a migration (which only
// affect the single connection that executed them), DSN options apply to every
// connection the pool opens.
//
// The agent must use a single writer: WAL allows concurrent readers but only
// one writer at a time, so all writes go through one store component.
func OpenSQLite(path string) (*sql.DB, error) {
	// _foreign_keys=on and _busy_timeout apply per connection. _journal_mode=WAL
	// and _synchronous=NORMAL are persistent database settings.
	dsn := fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_txlock=immediate", path)

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}

	// SQLite serializes writes at the database level; a single connection keeps
	// writer semantics explicit and avoids SQLITE_BUSY churn.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}

	return db, nil
}
