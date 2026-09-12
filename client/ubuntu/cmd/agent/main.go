package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	agentapp "github.com/share-disk/share-disk/client/agent/app"
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
		migrationsDir = "client/agent/migrations/sqlite"
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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

	if err := agentapp.Run(ctx, cfg, migrationsDir); err != nil {
		logger.Error("Device agent stopped", "error", err)
		os.Exit(1)
	}
	logger.Info("Device agent exited")
}
