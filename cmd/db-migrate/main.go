package main

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/share-disk/share-disk/internal/config"
	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/internal/logging"

	_ "github.com/lib/pq"
)

func main() {
	// Load configuration
	cfg, err := config.LoadServer("conf/server.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// The migrator requires a PostgreSQL DSN.
	if cfg.Database.PostgreSQL == "" {
		fmt.Fprintln(os.Stderr, "PostgreSQL URL is required for migration")
		os.Exit(1)
	}

	// Initialize logger
	logger, err := logging.New(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Validate PostgreSQL URL is configured
	if cfg.Database.PostgreSQL == "" {
		logger.Error("PostgreSQL URL is required for migration")
		os.Exit(1)
	}

	migrationsDir := cfg.Server.MigrationsDir

	// Initialize database connection
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL)
	if err != nil {
		logger.Error("Failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Test database connection
	if err := db.Ping(); err != nil {
		logger.Error("Failed to ping database", "error", err)
		os.Exit(1)
	}

	// Acquire advisory lock to prevent concurrent migrations
	lockID := int64(1234567890) // Fixed lock ID for share-disk migrations
	var locked bool
	err = db.QueryRow("SELECT pg_try_advisory_lock($1)", lockID).Scan(&locked)
	if err != nil {
		logger.Error("Failed to acquire advisory lock", "error", err)
		os.Exit(1)
	}
	if !locked {
		logger.Error("Another migration is already running")
		os.Exit(1)
	}
	defer func() {
		_, err := db.Exec("SELECT pg_advisory_unlock($1)", lockID)
		if err != nil {
			logger.Error("Failed to release advisory lock", "error", err)
		}
	}()

	// Create migrator
	migrator := database.NewMigrator(db, migrationsDir, "postgres")

	// Run migrations
	logger.Info("Starting database migrations", "dir", migrationsDir)

	if err := migrator.MigrateUp(); err != nil {
		logger.Error("Migration failed", "error", err)
		os.Exit(1)
	}

	// Get final version
	version, err := migrator.GetCurrentVersion()
	if err != nil {
		logger.Error("Failed to get final migration version", "error", err)
		os.Exit(1)
	}

	logger.Info("Database migrations completed successfully", "version", version)
}
