// Package agent assembles and runs the local device agent: it owns the SQLite
// authoritative state, the object store, and the restricted IPC server that the
// CLI and GUI talk to.
package agent

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/google/uuid"

	"github.com/share-disk/share-disk/client/ubuntu/internal/agentsync"
	"github.com/share-disk/share-disk/client/ubuntu/internal/desktopui"
	"github.com/share-disk/share-disk/client/ubuntu/internal/discovery"
	"github.com/share-disk/share-disk/client/ubuntu/internal/lanapi"
	"github.com/share-disk/share-disk/client/ubuntu/internal/localapi"
	"github.com/share-disk/share-disk/client/ubuntu/internal/storage"
	"github.com/share-disk/share-disk/internal/config"
	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/internal/version"
)

// Agent is a running local agent instance.
type Agent struct {
	cfg         *config.Config
	db          *sql.DB
	store       *storage.SQLiteObjectStore
	server      *localapi.Server
	lan         *lanapi.Server
	discovery   *discovery.Advertiser
	desktop     *desktopui.Server
	coordinator *agentsync.Client
	lockFile    *os.File
}

// New opens SQLite, applies migrations, initializes the object store (including
// startup recovery), and binds the IPC server. The agent is not running until
// Run is called.
func New(cfg *config.Config, sqliteMigrationsDir string) (*Agent, error) {
	lockFile, err := acquireLock(cfg.Agent.StorageRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire agent lock: %w", err)
	}

	db, err := database.OpenSQLite(cfg.Database.SQLite)
	if err != nil {
		if lockFile != nil {
			lockFile.Close()
		}
		return nil, fmt.Errorf("failed to open sqlite: %w", err)
	}

	if err := database.NewMigrator(db, sqliteMigrationsDir, "sqlite").MigrateUp(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate sqlite: %w", err)
	}

	st, err := storage.New(cfg.Agent.StorageRoot)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	store := storage.NewSQLiteObjectStore(st, db, cfg.Agent.ChunkSize)
	if err := store.Init(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize object store: %w", err)
	}

	deviceID, err := ensureDeviceID(context.Background(), db)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to load device id: %w", err)
	}

	handler := localapi.NewLocalHandler(store, st, deviceID, versionString(), db, cfg.Agent.LANAdvertiseURL, cfg.Agent.TrashRetention)
	server := localapi.NewServer(cfg.Agent.SocketPath, handler)
	if err := server.Listen(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to start IPC server: %w", err)
	}

	var lanServer *lanapi.Server
	if cfg.Agent.LANEnabled {
		lanServer, err = lanapi.New(
			cfg.Agent.LANAddress(),
			cfg.Auth.AccessPublicKey,
			st.IncomingDir(),
			cfg.Agent.LANMaxUploadSize,
			cfg.Agent.TrashRetention,
			cfg.Agent.LANAdvertiseURL,
			store,
		)
		if err != nil {
			_ = server.Close()
			_ = db.Close()
			_ = lockFile.Close()
			return nil, fmt.Errorf("failed to configure LAN API: %w", err)
		}
		if err := lanServer.Listen(); err != nil {
			_ = server.Close()
			_ = db.Close()
			_ = lockFile.Close()
			return nil, fmt.Errorf("failed to start LAN API: %w", err)
		}
	}

	var desktopServer *desktopui.Server
	if cfg.Agent.DesktopUIEnabled {
		desktopServer, err = desktopui.New(cfg.Agent.DesktopUIAddress(), st.IncomingDir(), handler)
		if err != nil {
			if lanServer != nil {
				_ = lanServer.Close()
			}
			_ = server.Close()
			_ = db.Close()
			_ = lockFile.Close()
			return nil, fmt.Errorf("failed to configure desktop UI: %w", err)
		}
	}

	var coordinator *agentsync.Client
	if cfg.Agent.ControlURL != "" {
		deviceName, hostnameErr := os.Hostname()
		if hostnameErr != nil || deviceName == "" {
			deviceName = "share-disk-agent"
		}
		coordinator = agentsync.New(db, cfg.Agent.ControlURL, cfg.Agent.LANAdvertiseURL, deviceID, deviceName, cfg.Agent.SyncInterval, cfg.Agent.HeartbeatInterval)
		coordinator.SetTransferStore(store, st.IncomingDir())
		handler.SetCoordinator(coordinator)
		if err := coordinator.Recover(context.Background()); err != nil {
			_ = lanServer.Close()
			if desktopServer != nil {
				_ = desktopServer.Close()
			}
			_ = server.Close()
			_ = db.Close()
			_ = lockFile.Close()
			return nil, fmt.Errorf("failed to recover control-plane outbox: %w", err)
		}
		if lanServer != nil {
			lanServer.SetCoordinator(coordinator)
		}
	}

	var advertiser *discovery.Advertiser
	if cfg.Agent.LANDiscoveryEnabled {
		advertiser, err = discovery.Start(deviceID, cfg.Agent.LANPort, versionString())
		if err != nil {
			_ = lanServer.Close()
			if desktopServer != nil {
				_ = desktopServer.Close()
			}
			_ = server.Close()
			_ = db.Close()
			_ = lockFile.Close()
			return nil, fmt.Errorf("failed to start LAN discovery: %w", err)
		}
	}

	return &Agent{cfg: cfg, db: db, store: store, server: server, lan: lanServer, desktop: desktopServer, discovery: advertiser, coordinator: coordinator, lockFile: lockFile}, nil
}

// Run serves local IPC and, when explicitly enabled, the authenticated LAN API.
func (a *Agent) Run(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	serverCount := 1
	errCh := make(chan error, 3)
	go func() { errCh <- a.server.Serve(runCtx) }()
	if a.lan != nil {
		serverCount++
		go func() { errCh <- a.lan.Serve(runCtx) }()
	}
	if a.desktop != nil {
		serverCount++
		go func() { errCh <- a.desktop.Serve(runCtx) }()
	}
	maintenanceDone := make(chan struct{})
	go func() {
		defer close(maintenanceDone)
		a.purgeExpiredTrash(runCtx)
	}()
	if a.coordinator != nil {
		go a.coordinator.Run(runCtx)
	}

	var runErr error
	for i := 0; i < serverCount; i++ {
		err := <-errCh
		if err != nil && ctx.Err() == nil {
			runErr = err
			break
		}
	}
	cancel()
	<-maintenanceDone
	return runErr
}

func (a *Agent) purgeExpiredTrash(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		var err error
		if a.coordinator != nil {
			err = a.store.PurgeExpiredLANFilesCoordinated(ctx, time.Now())
		} else {
			err = a.store.PurgeExpiredLANFiles(ctx, time.Now())
		}
		if err != nil && ctx.Err() == nil {
			slog.Error("failed to purge expired LAN trash", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Close releases the socket, database, and process lock.
func (a *Agent) Close() error {
	var firstErr error
	if a.discovery != nil {
		a.discovery.Close()
		a.discovery = nil
	}
	if a.server != nil {
		if err := a.server.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if a.lan != nil {
		if err := a.lan.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if a.desktop != nil {
		if err := a.desktop.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if a.db != nil {
		if err := a.db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if a.lockFile != nil {
		if err := a.lockFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		a.lockFile = nil
	}
	return firstErr
}

// ensureDeviceID returns a stable local device id, persisting a newly generated
// one in the settings table. Real device registration/identity is an M3 concern;
// this placeholder is only for a stable local id and status reporting.
func ensureDeviceID(ctx context.Context, db *sql.DB) (string, error) {
	var id string
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'device_id'`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}

	id = uuid.NewString()
	if _, err := db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES ('device_id', $1)`, id); err != nil {
		return "", err
	}
	return id, nil
}

func versionString() string {
	return version.Get().Version
}
