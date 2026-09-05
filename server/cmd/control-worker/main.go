package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/share-disk/share-disk/internal/config"
	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/internal/logging"
	"github.com/share-disk/share-disk/internal/version"
	"github.com/share-disk/share-disk/server/internal/health"

	_ "github.com/lib/pq"
)

func main() {
	// Load configuration
	cfg, err := config.LoadServer("conf/server.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// The worker only requires a PostgreSQL DSN (it does not mint tokens), but
	// must still fail closed when it is missing.
	if cfg.Database.PostgreSQL == "" {
		fmt.Fprintln(os.Stderr, "PostgreSQL URL is required for control-worker")
		os.Exit(1)
	}

	// Initialize logger
	logger, err := logging.New(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Create health check manager
	healthManager := health.NewManager()

	// Initialize database connection
	db, err := sql.Open("postgres", cfg.Database.PostgreSQL)
	if err != nil {
		logger.Error("Failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Configure connection pool
	db.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	db.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.Database.ConnMaxLifetime)

	// Register database health check
	healthManager.Register("database", health.DatabaseChecker("postgresql", func() error {
		return db.Ping()
	}))

	// Register schema version health check. The migrator resolves the required
	// version from the migration files shipped with this binary.
	migrator := database.NewMigrator(db, cfg.Server.MigrationsDir, "postgres")
	expectedVersion, err := migrator.LatestVersion()
	if err != nil {
		logger.Error("Failed to determine required schema version", "error", err)
		os.Exit(1)
	}
	healthManager.Register("schema", health.SchemaChecker("postgresql", expectedVersion, func() (int, error) {
		return migrator.GetCurrentVersion()
	}))

	// Create context that listens for the interrupt signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create wait group for goroutines
	var wg sync.WaitGroup

	// Start worker in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("Starting control worker",
			"version", version.Get().Version,
			"commit", version.Get().Commit,
		)

		runWorker(ctx, logger, db, healthManager, cfg.Server.WorkerInterval)
		logger.Info("Control worker stopping...")
	}()

	// Wait for interrupt signal
	<-ctx.Done()
	logger.Info("Shutting down control worker...")

	// Create a deadline for worker shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// Wait for all goroutines to finish with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("Control worker exited properly")
	case <-shutdownCtx.Done():
		logger.Error("Control worker forced to shutdown")
	}
}

// runWorker runs the worker's periodic loop until ctx is cancelled. Business
// sub-loops (task claiming, outbox, redundancy, GC) are not yet implemented;
// the loop verifies the database/schema each cycle so the readiness probe and
// startup failure behavior are real rather than a silent no-op.
func runWorker(ctx context.Context, logger *logging.Logger, db *sql.DB, healthManager *health.Manager, cycleInterval time.Duration) {
	if err := db.Ping(); err != nil {
		logger.Error("Worker initial database check failed", "error", err)
		return
	}

	ticker := time.NewTicker(cycleInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			result := healthManager.RunChecks()
			if result.Status == health.StatusDown {
				logger.Warn("Worker dependency checks failed", "checks", result.Checks)
				continue
			}
			logger.Debug("Worker dependency checks passed")
		}
	}
}
