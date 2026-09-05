package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadServerUsesSingleJSONFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	content := `{
		"server":{"host":"0.0.0.0","port":8088,"read_timeout":"10s","write_timeout":"20s","idle_timeout":"1m","shutdown_timeout":"15s","migrations_dir":"/app/migrations/postgres","worker_interval":"5s"},
		"database":{"host":"postgres","port":5432,"name":"share_disk","user":"share_disk","password":"p@ss:word","ssl_mode":"disable","max_open_conns":12,"max_idle_conns":4,"conn_max_lifetime":"3m"},
		"logging":{"level":"debug","format":"json","output":"stdout"},
		"monitoring":{"enabled":true,"port":8181,"external_access":true},
		"auth":{"bootstrap_token":"bootstrap","access_private_key":"private-key","access_token_ttl":"12m"}
	}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadServer(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8088 || cfg.Server.WorkerInterval != 5*time.Second || cfg.Auth.AccessTokenTTL != 12*time.Minute {
		t.Fatalf("server JSON was not applied: %+v", cfg.Server)
	}
	if !cfg.Monitoring.Enabled || cfg.Monitoring.Port != 8181 || !cfg.Monitoring.ExternalAccess {
		t.Fatalf("monitoring configuration was not applied: %+v", cfg.Monitoring)
	}
	if !strings.Contains(cfg.Database.PostgreSQL, "p%40ss%3Aword") {
		t.Fatalf("database password was not URL encoded: %s", cfg.Database.PostgreSQL)
	}
}

func TestLoadServerRejectsWritableConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.json")
	if err := os.WriteFile(path, []byte(`{}`), 0666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0666); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadServer(path); err == nil {
		t.Fatal("expected writable server config to be rejected")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Expected server host '127.0.0.1', got '%s'", cfg.Server.Host)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Expected server port 8080, got %d", cfg.Server.Port)
	}

	if cfg.Server.ReadTimeout != 30*time.Second {
		t.Errorf("Expected read timeout 30s, got %v", cfg.Server.ReadTimeout)
	}

	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("Expected max open conns 25, got %d", cfg.Database.MaxOpenConns)
	}

	if cfg.Logging.Level != "info" {
		t.Errorf("Expected log level 'info', got '%s'", cfg.Logging.Level)
	}

	if cfg.Agent.ChunkSize != 4*1024*1024 {
		t.Errorf("Expected chunk size 4MB, got %d", cfg.Agent.ChunkSize)
	}

	if cfg.Agent.ParallelChunks != 4 {
		t.Errorf("Expected parallel chunks 4, got %d", cfg.Agent.ParallelChunks)
	}

	if cfg.Agent.TrashRetention != 7*24*time.Hour {
		t.Errorf("Expected trash retention 7 days, got %v", cfg.Agent.TrashRetention)
	}
}

func TestServerConfigAddress(t *testing.T) {
	cfg := ServerConfig{
		Host: "localhost",
		Port: 9090,
	}

	expected := "localhost:9090"
	if addr := cfg.Address(); addr != expected {
		t.Errorf("Expected address '%s', got '%s'", expected, addr)
	}
}

func TestLoadFromEnvironment(t *testing.T) {
	// Set environment variables
	os.Setenv("SHARE_DISK_SERVER_HOST", "127.0.0.1")
	os.Setenv("SHARE_DISK_SERVER_PORT", "9090")
	os.Setenv("SHARE_DISK_POSTGRESQL_URL", "postgresql://user:pass@localhost:5432/db")
	os.Setenv("SHARE_DISK_LOG_LEVEL", "debug")
	os.Setenv("SHARE_DISK_CHUNK_SIZE", "8388608")
	defer func() {
		os.Unsetenv("SHARE_DISK_SERVER_HOST")
		os.Unsetenv("SHARE_DISK_SERVER_PORT")
		os.Unsetenv("SHARE_DISK_POSTGRESQL_URL")
		os.Unsetenv("SHARE_DISK_LOG_LEVEL")
		os.Unsetenv("SHARE_DISK_CHUNK_SIZE")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Expected server host '127.0.0.1', got '%s'", cfg.Server.Host)
	}

	if cfg.Server.Port != 9090 {
		t.Errorf("Expected server port 9090, got %d", cfg.Server.Port)
	}

	if cfg.Logging.Level != "debug" {
		t.Errorf("Expected log level 'debug', got '%s'", cfg.Logging.Level)
	}

	if cfg.Agent.ChunkSize != 8388608 {
		t.Errorf("Expected chunk size 8388608, got %d", cfg.Agent.ChunkSize)
	}
}

func TestLoadInvalidPort(t *testing.T) {
	os.Setenv("SHARE_DISK_SERVER_PORT", "invalid")
	os.Setenv("SHARE_DISK_POSTGRESQL_URL", "postgresql://user:pass@localhost:5432/db")
	defer func() {
		os.Unsetenv("SHARE_DISK_SERVER_PORT")
		os.Unsetenv("SHARE_DISK_POSTGRESQL_URL")
	}()

	_, err := Load()
	if err == nil {
		t.Error("Expected error for invalid port, got nil")
	}
}

func TestLoadInvalidChunkSize(t *testing.T) {
	os.Setenv("SHARE_DISK_CHUNK_SIZE", "invalid")
	os.Setenv("SHARE_DISK_POSTGRESQL_URL", "postgresql://user:pass@localhost:5432/db")
	defer func() {
		os.Unsetenv("SHARE_DISK_CHUNK_SIZE")
		os.Unsetenv("SHARE_DISK_POSTGRESQL_URL")
	}()

	_, err := Load()
	if err == nil {
		t.Error("Expected error for invalid chunk size, got nil")
	}
}

func TestLANDiscoveryRequiresLANAPI(t *testing.T) {
	t.Setenv("SHARE_DISK_LAN_DISCOVERY_ENABLED", "true")
	if _, err := Load(); err == nil {
		t.Fatal("expected LAN discovery without LAN API to fail")
	}
}

func TestDesktopUIRequiresLoopback(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.DesktopUIEnabled = true
	cfg.Agent.DesktopUIHost = "0.0.0.0"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected non-loopback desktop UI to be rejected")
	}
}

func TestLoadInvalidLANDiscoveryValue(t *testing.T) {
	t.Setenv("SHARE_DISK_LAN_DISCOVERY_ENABLED", "sometimes")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid LAN discovery boolean to fail")
	}
}

func TestLoadTrashRetention(t *testing.T) {
	t.Setenv("SHARE_DISK_TRASH_RETENTION", "36h")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.TrashRetention != 36*time.Hour {
		t.Fatalf("trash retention = %v, want 36h", cfg.Agent.TrashRetention)
	}
}

func TestLoadInvalidTrashRetention(t *testing.T) {
	t.Setenv("SHARE_DISK_TRASH_RETENTION", "next week")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid trash retention to fail")
	}
}

func TestCoordinatorURLsMustBePairedAndAbsolute(t *testing.T) {
	t.Setenv("SHARE_DISK_LAN_ENABLED", "true")
	t.Setenv("SHARE_DISK_ACCESS_PUBLIC_KEY", "configured-for-validation")
	t.Setenv("SHARE_DISK_CONTROL_URL", "https://control.example.test")
	if _, err := Load(); err == nil {
		t.Fatal("expected unpaired coordinator URL to fail")
	}
	t.Setenv("SHARE_DISK_LAN_ADVERTISE_URL", "http://192.0.2.10:9090")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agent.ControlURL != "https://control.example.test" || cfg.Agent.LANAdvertiseURL != "http://192.0.2.10:9090" {
		t.Fatalf("unexpected coordinator configuration: %+v", cfg.Agent)
	}
}

func TestCoordinatorURLRejectsCredentials(t *testing.T) {
	t.Setenv("SHARE_DISK_LAN_ENABLED", "true")
	t.Setenv("SHARE_DISK_ACCESS_PUBLIC_KEY", "configured-for-validation")
	t.Setenv("SHARE_DISK_CONTROL_URL", "https://user:secret@control.example.test")
	t.Setenv("SHARE_DISK_LAN_ADVERTISE_URL", "http://192.0.2.10:9090")
	if _, err := Load(); err == nil {
		t.Fatal("expected credential-bearing control URL to fail")
	}
}

func TestLoadFromFileSecret(t *testing.T) {
	// Create a temporary secret file
	tmpFile, err := os.CreateTemp("", "postgresql_url")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write secret to file
	secretValue := "postgresql://user:pass@localhost:5432/db"
	if _, err := tmpFile.WriteString(secretValue); err != nil {
		t.Fatalf("Failed to write secret: %v", err)
	}
	tmpFile.Close()

	// Set environment variable to point to file
	os.Setenv("SHARE_DISK_POSTGRESQL_URL_FILE", tmpFile.Name())
	defer os.Unsetenv("SHARE_DISK_POSTGRESQL_URL_FILE")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Database.PostgreSQL != secretValue {
		t.Errorf("Expected PostgreSQL URL '%s', got '%s'", secretValue, cfg.Database.PostgreSQL)
	}
}

func TestLoadBothDirectAndFileFails(t *testing.T) {
	// Create a temporary secret file
	tmpFile, err := os.CreateTemp("", "postgresql_url")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write secret to file
	if _, err := tmpFile.WriteString("postgresql://user:pass@localhost:5432/db"); err != nil {
		t.Fatalf("Failed to write secret: %v", err)
	}
	tmpFile.Close()

	// Set both direct and file environment variables
	os.Setenv("SHARE_DISK_POSTGRESQL_URL", "postgresql://direct:pass@localhost:5432/db")
	os.Setenv("SHARE_DISK_POSTGRESQL_URL_FILE", tmpFile.Name())
	defer func() {
		os.Unsetenv("SHARE_DISK_POSTGRESQL_URL")
		os.Unsetenv("SHARE_DISK_POSTGRESQL_URL_FILE")
	}()

	_, err = Load()
	if err == nil {
		t.Error("Expected error when both direct and file values are set, got nil")
	}
}
