package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/share-disk/share-disk/internal/agent"
	"github.com/share-disk/share-disk/internal/config"
	"github.com/share-disk/share-disk/internal/logging"
	"github.com/share-disk/share-disk/internal/version"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	logger, err := logging.New(cfg.Logging.Level, cfg.Logging.Format, cfg.Logging.Output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	migrationsDir := os.Getenv("SHARE_DISK_SQLITE_MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations/sqlite"
	}

	a, err := agent.New(cfg, migrationsDir)
	if err != nil {
		logger.Error("Failed to start agent", "error", err)
		os.Exit(1)
	}
	defer a.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger.Info("Starting device agent",
		"version", version.Get().Version,
		"storage_root", cfg.Agent.StorageRoot,
		"socket", cfg.Agent.SocketPath,
		"lan_enabled", cfg.Agent.LANEnabled,
		"lan_address", cfg.Agent.LANAddress(),
		"lan_discovery_enabled", cfg.Agent.LANDiscoveryEnabled,
		"desktop_ui_enabled", cfg.Agent.DesktopUIEnabled,
		"desktop_ui_address", cfg.Agent.DesktopUIAddress(),
	)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- a.Run(ctx)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		logger.Info("Shutting down device agent", "signal", sig.String())
		cancel()
	case err := <-serveErr:
		if err != nil {
			logger.Error("Agent IPC server stopped", "error", err)
			os.Exit(1)
		}
	}

	select {
	case <-serveErr:
	case <-time.After(10 * time.Second):
		logger.Warn("Timed out waiting for agent shutdown")
	}

	logger.Info("Device agent exited")
}
