package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds the application configuration.
type Config struct {
	// Server configuration
	Server ServerConfig `json:"server"`

	// Database configuration
	Database DatabaseConfig `json:"database"`

	// Logging configuration
	Logging LoggingConfig `json:"logging"`

	// Optional server-side operations console.
	Monitoring MonitoringConfig `json:"monitoring"`

	// Agent configuration
	Agent AgentConfig `json:"agent"`

	// Auth configuration
	Auth AuthConfig `json:"auth"`
}

type MonitoringConfig struct {
	Enabled        bool `json:"enabled"`
	Port           int  `json:"port"`
	ExternalAccess bool `json:"external_access"`
}

// AuthConfig holds authentication material configuration. Values are only ever
// loaded from environment variables or *_FILE secret files; there are no
// defaults and missing values must fail closed for server components.
type AuthConfig struct {
	// BootstrapToken authorizes one-time first-account creation.
	BootstrapToken string `json:"-"`

	// AccessPrivateKey is the Ed25519 private key (PKCS#8 PEM) the control
	// server uses to sign access tokens. It must never be shared with Agents.
	AccessPrivateKey string `json:"-"`

	// AccessPublicKey is the Ed25519 public key (PKIX PEM) the Agent uses to
	// verify access tokens. The Agent must not hold signing material.
	AccessPublicKey string `json:"-"`

	AccessTokenTTL time.Duration `json:"-"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`

	// ShutdownTimeout is the timeout for graceful shutdown
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
	MigrationsDir   string        `json:"migrations_dir"`
	WorkerInterval  time.Duration `json:"worker_interval"`
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	// PostgreSQL connection string
	PostgreSQL string `json:"postgresql"`

	// SQLite database file path
	SQLite string `json:"sqlite"`

	// Connection pool settings
	MaxOpenConns    int           `json:"max_open_conns"`
	MaxIdleConns    int           `json:"max_idle_conns"`
	ConnMaxLifetime time.Duration `json:"conn_max_lifetime"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `json:"level"`
	Format string `json:"format"` // "json" or "text"
	Output string `json:"output"` // "stdout", "stderr", or file path
}

// AgentConfig holds agent-specific configuration.
type AgentConfig struct {
	// StorageRoot is the root directory for object storage
	StorageRoot string `json:"storage_root"`

	// SocketPath is the local IPC socket path for CLI/GUI to reach the agent.
	SocketPath string `json:"socket_path"`

	// ChunkSize is the size of each chunk in bytes (default 4 MiB)
	ChunkSize int64 `json:"chunk_size"`

	// ParallelChunks is the number of parallel chunk transfers
	ParallelChunks int `json:"parallel_chunks"`

	// MaxActiveTransfers is the maximum number of concurrent transfers
	MaxActiveTransfers int `json:"max_active_transfers"`

	// TicketTTL is the transfer ticket time-to-live
	TicketTTL time.Duration `json:"ticket_ttl"`

	// HeartbeatInterval is the interval for device heartbeat
	HeartbeatInterval time.Duration `json:"heartbeat_interval"`

	// TaskLease is the task lease duration
	TaskLease time.Duration `json:"task_lease"`

	// TrashRetention is the trash retention period
	TrashRetention time.Duration `json:"trash_retention"`

	// TombstoneRetention is the tombstone retention period
	TombstoneRetention time.Duration `json:"tombstone_retention"`

	// MinFreeSpace is the minimum free space to maintain
	MinFreeSpace int64 `json:"min_free_space"`

	// LANEnabled exposes the authenticated Android/Ubuntu LAN API. It is off by
	// default so a local Agent never starts listening on a network implicitly.
	LANEnabled bool `json:"lan_enabled"`

	// LANHost and LANPort are the dedicated LAN API listen address.
	LANHost string `json:"lan_host"`
	LANPort int    `json:"lan_port"`

	// LANMaxUploadSize bounds one resumable upload.
	LANMaxUploadSize int64 `json:"lan_max_upload_size"`

	// LANDiscoveryEnabled publishes the LAN API through DNS-SD/mDNS. It is a
	// separate opt-in because bridged container networks may not carry multicast.
	LANDiscoveryEnabled bool `json:"lan_discovery_enabled"`

	ControlURL      string        `json:"control_url"`
	LANAdvertiseURL string        `json:"lan_advertise_url"`
	SyncInterval    time.Duration `json:"sync_interval"`

	// Desktop UI is a loopback-only browser client for the Ubuntu package.
	DesktopUIEnabled bool   `json:"desktop_ui_enabled"`
	DesktopUIHost    string `json:"desktop_ui_host"`
	DesktopUIPort    int    `json:"desktop_ui_port"`
}

// DefaultConfig returns a configuration with default values.
// WARNING: These defaults are for local development only.
// For production, you must set proper database credentials and TLS.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:            "127.0.0.1",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
			MigrationsDir:   "migrations/postgres",
			WorkerInterval:  30 * time.Second,
		},
		Database: DatabaseConfig{
			PostgreSQL:      "", // Must be set via environment variable
			SQLite:          "./data/agent.db",
			MaxOpenConns:    25,
			MaxIdleConns:    10,
			ConnMaxLifetime: 5 * time.Minute,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "stdout",
		},
		Monitoring: MonitoringConfig{Port: 8081},
		Auth:       AuthConfig{AccessTokenTTL: 15 * time.Minute},
		Agent: AgentConfig{
			StorageRoot:         "./data/objects",
			SocketPath:          "./data/agent.sock",
			ChunkSize:           4 * 1024 * 1024, // 4 MiB
			ParallelChunks:      4,
			MaxActiveTransfers:  3,
			TicketTTL:           5 * time.Minute,
			HeartbeatInterval:   20 * time.Second,
			TaskLease:           60 * time.Second,
			TrashRetention:      7 * 24 * time.Hour,     // 7 days
			TombstoneRetention:  30 * 24 * time.Hour,    // 30 days
			MinFreeSpace:        5 * 1024 * 1024 * 1024, // 5 GiB
			LANEnabled:          false,
			LANHost:             "127.0.0.1",
			LANPort:             9090,
			LANMaxUploadSize:    10 * 1024 * 1024 * 1024, // 10 GiB
			LANDiscoveryEnabled: false,
			SyncInterval:        2 * time.Second,
			DesktopUIEnabled:    false,
			DesktopUIHost:       "127.0.0.1",
			DesktopUIPort:       9191,
		},
	}
}

// serverFileConfig is the single server-side configuration contract. Secrets
// intentionally live in the same ignored, mode-0600 JSON file so deployment
// cannot silently combine stale environment variables and secret fragments.
type serverFileConfig struct {
	Server struct {
		Host            string `json:"host"`
		Port            int    `json:"port"`
		ReadTimeout     string `json:"read_timeout"`
		WriteTimeout    string `json:"write_timeout"`
		IdleTimeout     string `json:"idle_timeout"`
		ShutdownTimeout string `json:"shutdown_timeout"`
		MigrationsDir   string `json:"migrations_dir"`
		WorkerInterval  string `json:"worker_interval"`
	} `json:"server"`
	Database struct {
		Host            string `json:"host"`
		Port            int    `json:"port"`
		Name            string `json:"name"`
		User            string `json:"user"`
		Password        string `json:"password"`
		SSLMode         string `json:"ssl_mode"`
		MaxOpenConns    int    `json:"max_open_conns"`
		MaxIdleConns    int    `json:"max_idle_conns"`
		ConnMaxLifetime string `json:"conn_max_lifetime"`
	} `json:"database"`
	Logging    LoggingConfig `json:"logging"`
	Monitoring struct {
		Enabled        bool `json:"enabled"`
		Port           int  `json:"port"`
		ExternalAccess bool `json:"external_access"`
	} `json:"monitoring"`
	Auth struct {
		BootstrapToken   string `json:"bootstrap_token"`
		AccessPrivateKey string `json:"access_private_key"`
		AccessTokenTTL   string `json:"access_token_ttl"`
	} `json:"auth"`
}

// LoadServer loads every server-side setting from one JSON file. Environment
// variables are deliberately ignored; Agent configuration continues to use
// Load because it is a separately installed client product.
func LoadServer(path string) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat server config %s: %w", path, err)
	}
	if info.Mode().Perm()&0o022 != 0 {
		return nil, fmt.Errorf("server config %s must not be writable by group or others", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read server config %s: %w", path, err)
	}
	var file serverFileConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return nil, fmt.Errorf("decode server config %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode server config %s: trailing content", path)
	}

	cfg := DefaultConfig()
	cfg.Server.Host = file.Server.Host
	cfg.Server.Port = file.Server.Port
	cfg.Server.MigrationsDir = file.Server.MigrationsDir
	cfg.Database.MaxOpenConns = file.Database.MaxOpenConns
	cfg.Database.MaxIdleConns = file.Database.MaxIdleConns
	cfg.Logging = file.Logging
	cfg.Monitoring = MonitoringConfig{
		Enabled:        file.Monitoring.Enabled,
		Port:           file.Monitoring.Port,
		ExternalAccess: file.Monitoring.ExternalAccess,
	}
	cfg.Auth.BootstrapToken = file.Auth.BootstrapToken
	cfg.Auth.AccessPrivateKey = file.Auth.AccessPrivateKey

	for _, item := range []struct {
		label string
		raw   string
		dst   *time.Duration
	}{
		{"server.read_timeout", file.Server.ReadTimeout, &cfg.Server.ReadTimeout},
		{"server.write_timeout", file.Server.WriteTimeout, &cfg.Server.WriteTimeout},
		{"server.idle_timeout", file.Server.IdleTimeout, &cfg.Server.IdleTimeout},
		{"server.shutdown_timeout", file.Server.ShutdownTimeout, &cfg.Server.ShutdownTimeout},
		{"server.worker_interval", file.Server.WorkerInterval, &cfg.Server.WorkerInterval},
		{"database.conn_max_lifetime", file.Database.ConnMaxLifetime, &cfg.Database.ConnMaxLifetime},
		{"auth.access_token_ttl", file.Auth.AccessTokenTTL, &cfg.Auth.AccessTokenTTL},
	} {
		value, err := time.ParseDuration(item.raw)
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", item.label, err)
		}
		*item.dst = value
	}
	if file.Database.Host == "" || file.Database.Port < 1 || file.Database.Port > 65535 || file.Database.Name == "" || file.Database.User == "" || file.Database.Password == "" {
		return nil, fmt.Errorf("database host, port, name, user and password are required")
	}
	sslMode := file.Database.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	dsn := &url.URL{Scheme: "postgres", Host: net.JoinHostPort(file.Database.Host, strconv.Itoa(file.Database.Port)), Path: "/" + file.Database.Name}
	dsn.User = url.UserPassword(file.Database.User, file.Database.Password)
	query := dsn.Query()
	query.Set("sslmode", sslMode)
	dsn.RawQuery = query.Encode()
	cfg.Database.PostgreSQL = dsn.String()

	if cfg.Server.Host == "" || cfg.Server.MigrationsDir == "" || cfg.Server.WorkerInterval <= 0 || cfg.Auth.AccessTokenTTL <= 0 {
		return nil, fmt.Errorf("server host, migrations_dir, worker_interval and access_token_ttl are required")
	}
	if err := cfg.ValidateServer(); err != nil {
		return nil, fmt.Errorf("invalid server configuration: %w", err)
	}
	return cfg, nil
}

// Load loads configuration from environment variables with fallback to defaults.
func Load() (*Config, error) {
	cfg := DefaultConfig()

	// Server configuration
	if host := os.Getenv("SHARE_DISK_SERVER_HOST"); host != "" {
		cfg.Server.Host = host
	}
	if portStr := os.Getenv("SHARE_DISK_SERVER_PORT"); portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_SERVER_PORT: %w", err)
		}
		cfg.Server.Port = port
	}
	if shutdownTimeoutStr := os.Getenv("SHARE_DISK_SHUTDOWN_TIMEOUT"); shutdownTimeoutStr != "" {
		shutdownTimeout, err := time.ParseDuration(shutdownTimeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.Server.ShutdownTimeout = shutdownTimeout
	}

	// Database configuration - support both direct value and file-based secrets
	pgURL, err := getSecretValue("SHARE_DISK_POSTGRESQL_URL", "SHARE_DISK_POSTGRESQL_URL_FILE")
	if err != nil {
		return nil, fmt.Errorf("failed to get PostgreSQL URL: %w", err)
	}
	if pgURL != "" {
		cfg.Database.PostgreSQL = pgURL
	}

	if sqlite := os.Getenv("SHARE_DISK_SQLITE_PATH"); sqlite != "" {
		cfg.Database.SQLite = sqlite
	}

	// Logging configuration
	if level := os.Getenv("SHARE_DISK_LOG_LEVEL"); level != "" {
		cfg.Logging.Level = level
	}
	if format := os.Getenv("SHARE_DISK_LOG_FORMAT"); format != "" {
		cfg.Logging.Format = format
	}

	// Agent configuration
	if root := os.Getenv("SHARE_DISK_STORAGE_ROOT"); root != "" {
		cfg.Agent.StorageRoot = root
	}
	if socket := os.Getenv("SHARE_DISK_AGENT_SOCKET"); socket != "" {
		cfg.Agent.SocketPath = socket
	}
	if chunkSizeStr := os.Getenv("SHARE_DISK_CHUNK_SIZE"); chunkSizeStr != "" {
		chunkSize, err := strconv.ParseInt(chunkSizeStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_CHUNK_SIZE: %w", err)
		}
		cfg.Agent.ChunkSize = chunkSize
	}
	if retentionString := os.Getenv("SHARE_DISK_TRASH_RETENTION"); retentionString != "" {
		retention, err := time.ParseDuration(retentionString)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_TRASH_RETENTION: %w", err)
		}
		cfg.Agent.TrashRetention = retention
	}
	if enabled := os.Getenv("SHARE_DISK_LAN_ENABLED"); enabled != "" {
		value, err := strconv.ParseBool(enabled)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_LAN_ENABLED: %w", err)
		}
		cfg.Agent.LANEnabled = value
	}
	if host := os.Getenv("SHARE_DISK_LAN_HOST"); host != "" {
		cfg.Agent.LANHost = host
	}
	if portString := os.Getenv("SHARE_DISK_LAN_PORT"); portString != "" {
		port, err := strconv.Atoi(portString)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_LAN_PORT: %w", err)
		}
		cfg.Agent.LANPort = port
	}
	if maxSizeString := os.Getenv("SHARE_DISK_LAN_MAX_UPLOAD_SIZE"); maxSizeString != "" {
		maxSize, err := strconv.ParseInt(maxSizeString, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_LAN_MAX_UPLOAD_SIZE: %w", err)
		}
		cfg.Agent.LANMaxUploadSize = maxSize
	}
	if enabled := os.Getenv("SHARE_DISK_LAN_DISCOVERY_ENABLED"); enabled != "" {
		value, err := strconv.ParseBool(enabled)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_LAN_DISCOVERY_ENABLED: %w", err)
		}
		cfg.Agent.LANDiscoveryEnabled = value
	}
	if controlURL := os.Getenv("SHARE_DISK_CONTROL_URL"); controlURL != "" {
		cfg.Agent.ControlURL = strings.TrimRight(controlURL, "/")
	}
	if advertiseURL := os.Getenv("SHARE_DISK_LAN_ADVERTISE_URL"); advertiseURL != "" {
		cfg.Agent.LANAdvertiseURL = strings.TrimRight(advertiseURL, "/")
	}
	if interval := os.Getenv("SHARE_DISK_SYNC_INTERVAL"); interval != "" {
		value, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_SYNC_INTERVAL: %w", err)
		}
		cfg.Agent.SyncInterval = value
	}
	if enabled := os.Getenv("SHARE_DISK_DESKTOP_UI_ENABLED"); enabled != "" {
		value, err := strconv.ParseBool(enabled)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_DESKTOP_UI_ENABLED: %w", err)
		}
		cfg.Agent.DesktopUIEnabled = value
	}
	if host := os.Getenv("SHARE_DISK_DESKTOP_UI_HOST"); host != "" {
		cfg.Agent.DesktopUIHost = host
	}
	if portString := os.Getenv("SHARE_DISK_DESKTOP_UI_PORT"); portString != "" {
		port, err := strconv.Atoi(portString)
		if err != nil {
			return nil, fmt.Errorf("invalid SHARE_DISK_DESKTOP_UI_PORT: %w", err)
		}
		cfg.Agent.DesktopUIPort = port
	}

	// Auth configuration - secrets support both direct env and *_FILE forms.
	// Both being set at once is ambiguous and fails closed.
	bootstrapToken, err := getSecretValue("SHARE_DISK_BOOTSTRAP_TOKEN", "SHARE_DISK_BOOTSTRAP_TOKEN_FILE")
	if err != nil {
		return nil, fmt.Errorf("failed to get bootstrap token: %w", err)
	}
	cfg.Auth.BootstrapToken = bootstrapToken

	accessKey, err := getSecretValue("SHARE_DISK_ACCESS_PRIVATE_KEY", "SHARE_DISK_ACCESS_PRIVATE_KEY_FILE")
	if err != nil {
		return nil, fmt.Errorf("failed to get access private key: %w", err)
	}
	cfg.Auth.AccessPrivateKey = accessKey

	publicKey, err := getSecretValue("SHARE_DISK_ACCESS_PUBLIC_KEY", "SHARE_DISK_ACCESS_PUBLIC_KEY_FILE")
	if err != nil {
		return nil, fmt.Errorf("failed to get access public key: %w", err)
	}
	cfg.Auth.AccessPublicKey = publicKey

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// getSecretValue gets a secret value from either a direct environment variable or a file.
// If both are set, it returns an error (fail closed).
func getSecretValue(envVar, fileEnvVar string) (string, error) {
	directValue := os.Getenv(envVar)
	filePath := os.Getenv(fileEnvVar)

	// If both are set, fail closed
	if directValue != "" && filePath != "" {
		return "", fmt.Errorf("both %s and %s are set, but only one should be provided", envVar, fileEnvVar)
	}

	// If direct value is set, use it
	if directValue != "" {
		return directValue, nil
	}

	// If file path is set, read from file
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("failed to read secret file %s: %w", filePath, err)
		}
		// Trim whitespace and newlines
		return string(bytes.TrimRight(data, "\n\r\t ")), nil
	}

	return "", nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	// Validate server configuration
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	if c.Server.MigrationsDir == "" || c.Server.WorkerInterval <= 0 {
		return fmt.Errorf("server migrations directory and worker interval are required")
	}

	// PostgreSQL is only required for server components (see ValidateServer);
	// Agents and the CLI run locally against SQLite.

	// Validate agent configuration
	if c.Agent.ChunkSize <= 0 {
		return fmt.Errorf("chunk size must be positive: %d", c.Agent.ChunkSize)
	}

	if c.Agent.ChunkSize > 1024*1024*1024 { // 1 GiB max
		return fmt.Errorf("chunk size too large: %d", c.Agent.ChunkSize)
	}

	if c.Agent.ParallelChunks <= 0 {
		return fmt.Errorf("parallel chunks must be positive: %d", c.Agent.ParallelChunks)
	}

	if c.Agent.MaxActiveTransfers <= 0 {
		return fmt.Errorf("max active transfers must be positive: %d", c.Agent.MaxActiveTransfers)
	}

	if c.Agent.TicketTTL <= 0 {
		return fmt.Errorf("ticket TTL must be positive: %v", c.Agent.TicketTTL)
	}

	if c.Agent.HeartbeatInterval <= 0 {
		return fmt.Errorf("heartbeat interval must be positive: %v", c.Agent.HeartbeatInterval)
	}

	if c.Agent.TaskLease <= 0 {
		return fmt.Errorf("task lease must be positive: %v", c.Agent.TaskLease)
	}

	if c.Agent.TrashRetention <= 0 {
		return fmt.Errorf("trash retention must be positive: %v", c.Agent.TrashRetention)
	}

	if c.Agent.TombstoneRetention <= 0 {
		return fmt.Errorf("tombstone retention must be positive: %v", c.Agent.TombstoneRetention)
	}

	if c.Agent.MinFreeSpace < 0 {
		return fmt.Errorf("min free space cannot be negative: %d", c.Agent.MinFreeSpace)
	}
	if c.Agent.LANPort < 1 || c.Agent.LANPort > 65535 {
		return fmt.Errorf("invalid LAN port: %d", c.Agent.LANPort)
	}
	if c.Agent.DesktopUIPort < 1 || c.Agent.DesktopUIPort > 65535 {
		return fmt.Errorf("invalid desktop UI port: %d", c.Agent.DesktopUIPort)
	}
	if c.Agent.DesktopUIEnabled && c.Agent.DesktopUIHost != "127.0.0.1" && c.Agent.DesktopUIHost != "::1" && c.Agent.DesktopUIHost != "localhost" {
		return fmt.Errorf("desktop UI must bind to a loopback address")
	}
	if c.Agent.LANMaxUploadSize <= 0 {
		return fmt.Errorf("LAN max upload size must be positive: %d", c.Agent.LANMaxUploadSize)
	}
	if c.Agent.LANEnabled && len(c.Auth.AccessPublicKey) == 0 {
		return fmt.Errorf("LAN API requires an access public key (SHARE_DISK_ACCESS_PUBLIC_KEY)")
	}
	if c.Agent.LANDiscoveryEnabled && !c.Agent.LANEnabled {
		return fmt.Errorf("LAN discovery requires SHARE_DISK_LAN_ENABLED=true")
	}
	if c.Agent.SyncInterval <= 0 {
		return fmt.Errorf("sync interval must be positive: %v", c.Agent.SyncInterval)
	}
	if (c.Agent.ControlURL == "") != (c.Agent.LANAdvertiseURL == "") {
		return fmt.Errorf("control URL and LAN advertise URL must be configured together")
	}
	if c.Agent.ControlURL != "" && !c.Agent.LANEnabled {
		return fmt.Errorf("control-plane coordination requires SHARE_DISK_LAN_ENABLED=true")
	}
	for label, raw := range map[string]string{"control URL": c.Agent.ControlURL, "LAN advertise URL": c.Agent.LANAdvertiseURL} {
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
			return fmt.Errorf("invalid %s", label)
		}
	}

	// Validate database configuration
	if c.Database.MaxOpenConns < 0 {
		return fmt.Errorf("max open conns cannot be negative: %d", c.Database.MaxOpenConns)
	}

	if c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("max idle conns cannot be negative: %d", c.Database.MaxIdleConns)
	}

	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("max idle conns (%d) cannot be greater than max open conns (%d)",
			c.Database.MaxIdleConns, c.Database.MaxOpenConns)
	}

	if c.Database.ConnMaxLifetime < 0 {
		return fmt.Errorf("conn max lifetime cannot be negative: %v", c.Database.ConnMaxLifetime)
	}

	return nil
}

// ValidateServer validates server-component configuration. It requires a
// PostgreSQL DSN and all authentication secrets. Server components (control
// server, worker, db-migrate) must fail closed when any of these are missing.
func (c *Config) ValidateServer() error {
	if err := c.Validate(); err != nil {
		return err
	}

	if c.Database.PostgreSQL == "" {
		return fmt.Errorf("PostgreSQL URL is required for server components")
	}

	if c.Auth.BootstrapToken == "" {
		return fmt.Errorf("bootstrap token is required (SHARE_DISK_BOOTSTRAP_TOKEN or SHARE_DISK_BOOTSTRAP_TOKEN_FILE)")
	}

	if c.Auth.AccessPrivateKey == "" {
		return fmt.Errorf("access private key is required (SHARE_DISK_ACCESS_PRIVATE_KEY or SHARE_DISK_ACCESS_PRIVATE_KEY_FILE)")
	}
	if c.Monitoring.Enabled {
		if c.Monitoring.Port < 1 || c.Monitoring.Port > 65535 {
			return fmt.Errorf("invalid monitoring port: %d", c.Monitoring.Port)
		}
		if c.Monitoring.Port == c.Server.Port {
			return fmt.Errorf("monitoring port must differ from server port")
		}
	}

	return nil
}

// Address returns the monitoring listener address. In Compose the listener is
// isolated in the container; external_access controls the host-side publish
// address generated from this same configuration file.
func (c *MonitoringConfig) Address() string {
	return net.JoinHostPort("0.0.0.0", strconv.Itoa(c.Port))
}

// Address returns the server address in host:port format.
func (c *ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// LANAddress returns the Agent LAN API address in host:port format.
func (c *AgentConfig) LANAddress() string {
	return fmt.Sprintf("%s:%d", c.LANHost, c.LANPort)
}

func (c *AgentConfig) DesktopUIAddress() string {
	return net.JoinHostPort(c.DesktopUIHost, strconv.Itoa(c.DesktopUIPort))
}
