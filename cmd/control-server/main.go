package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/share-disk/share-disk/internal/catalog"
	"github.com/share-disk/share-disk/internal/config"
	"github.com/share-disk/share-disk/internal/controlapi"
	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/internal/health"
	"github.com/share-disk/share-disk/internal/identity"
	"github.com/share-disk/share-disk/internal/logging"
	"github.com/share-disk/share-disk/internal/transfer"
	"github.com/share-disk/share-disk/internal/version"

	_ "github.com/lib/pq"
)

func main() {
	// Load configuration
	cfg, err := config.LoadServer("conf/server.json")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Server components must fail closed when PostgreSQL or auth secrets are
	// missing rather than silently running with guessable credentials.
	if err := cfg.ValidateServer(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid server configuration: %v\n", err)
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

	// Initialize identity service with secrets from configuration. There are no
	// fallback defaults: ValidateServer has already enforced their presence.
	bootstrapToken := cfg.Auth.BootstrapToken

	tokenManager, err := identity.NewTokenManager(
		cfg.Auth.AccessPrivateKey,
		cfg.Auth.AccessTokenTTL,
	)
	if err != nil {
		logger.Error("Failed to load access signing key", "error", err)
		os.Exit(1)
	}

	identityService := identity.NewService(db, bootstrapToken, tokenManager)

	// Create API handler
	apiHandler := controlapi.NewHandler(identityService, healthManager, tokenManager, catalog.NewRepository(db))
	apiHandler.SetTransferService(transfer.NewService(db))

	// Create router
	router := controlapi.NewRouter(apiHandler)

	// Create HTTP server
	server := &http.Server{
		Addr:              cfg.Server.Address(),
		Handler:           router,
		ReadTimeout:       cfg.Server.ReadTimeout,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      cfg.Server.WriteTimeout,
		IdleTimeout:       cfg.Server.IdleTimeout,
	}
	var monitoringServer *http.Server
	if cfg.Monitoring.Enabled {
		monitoringServer = &http.Server{
			Addr:              cfg.Monitoring.Address(),
			Handler:           controlapi.NewMonitoringRouter(apiHandler),
			ReadTimeout:       cfg.Server.ReadTimeout,
			ReadHeaderTimeout: 5 * time.Second,
			WriteTimeout:      cfg.Server.WriteTimeout,
			IdleTimeout:       cfg.Server.IdleTimeout,
		}
	}

	// Create context that listens for the interrupt signal
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Create wait group for goroutines
	var wg sync.WaitGroup

	// Start server in a goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("Starting control server",
			"address", cfg.Server.Address(),
			"version", version.Get().Version,
			"commit", version.Get().Commit,
		)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Failed to start server", "error", err)
			stop() // Trigger shutdown on server error
		}
	}()
	if monitoringServer != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("Starting optional monitoring console",
				"address", cfg.Monitoring.Address(),
				"external_access", cfg.Monitoring.ExternalAccess,
			)
			if err := monitoringServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("Failed to start monitoring console", "error", err)
				stop()
			}
		}()
	}

	// Wait for interrupt signal
	<-ctx.Done()
	logger.Info("Shutting down server...")

	// Create a deadline for server shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}
	if monitoringServer != nil {
		if err := monitoringServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Monitoring console forced to shutdown", "error", err)
		}
	}

	// Wait for all goroutines to finish
	wg.Wait()

	logger.Info("Server exited properly")
}
