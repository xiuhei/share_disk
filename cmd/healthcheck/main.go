package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/share-disk/share-disk/internal/config"
	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/internal/version"

	_ "github.com/lib/pq"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <command> [args...]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  healthcheck [--url=<url>]  Perform HTTP health check\n")
		fmt.Fprintf(os.Stderr, "  dbcheck                    Perform PostgreSQL connectivity and schema check\n")
		fmt.Fprintf(os.Stderr, "  version                    Show version information\n")
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "healthcheck":
		if err := runHealthcheck(); err != nil {
			fmt.Fprintf(os.Stderr, "Health check failed: %v\n", err)
			os.Exit(1)
		}
	case "dbcheck":
		if err := runDBCheck(); err != nil {
			fmt.Fprintf(os.Stderr, "Database check failed: %v\n", err)
			os.Exit(1)
		}
	case "version":
		fmt.Printf("Share Disk Control Server %s (commit: %s, built: %s)\n",
			version.Get().Version, version.Get().Commit, version.Get().BuildTime)
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", command)
		os.Exit(1)
	}
}

func runHealthcheck() error {
	// Parse URL flag
	url := "http://127.0.0.1:8080/livez"
	for _, arg := range os.Args[2:] {
		if len(arg) > 6 && arg[:6] == "--url=" {
			url = arg[6:]
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Make health check request
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// runDBCheck verifies PostgreSQL connectivity and schema version. It is used as
// the container probe for components (like the worker) that expose no HTTP API.
func runDBCheck() error {
	cfg, err := config.LoadServer("conf/server.json")
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if cfg.Database.PostgreSQL == "" {
		return fmt.Errorf("PostgreSQL URL is required")
	}

	db, err := sql.Open("postgres", cfg.Database.PostgreSQL)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	migrationsDir := cfg.Server.MigrationsDir
	if _, err := os.Stat(migrationsDir); err != nil {
		// Fall back to the repository-relative default for local development.
		migrationsDir = "migrations/postgres"
	}

	migrator := database.NewMigrator(db, migrationsDir, "postgres")
	expected, err := migrator.LatestVersion()
	if err != nil {
		return fmt.Errorf("failed to determine required schema version: %w", err)
	}
	current, err := migrator.GetCurrentVersion()
	if err != nil {
		return fmt.Errorf("failed to get current schema version: %w", err)
	}
	if current < expected {
		return fmt.Errorf("schema version %d is behind required %d", current, expected)
	}

	return nil
}
