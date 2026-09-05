package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Status represents the health status.
type Status string

const (
	StatusUp   Status = "UP"
	StatusDown Status = "DOWN"
)

// Check represents a health check.
type Check struct {
	Name      string    `json:"name"`
	Status    Status    `json:"status"`
	Message   string    `json:"message,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Health represents the overall health status.
type Health struct {
	Status  Status            `json:"status"`
	Checks  []Check           `json:"checks"`
	Details map[string]string `json:"details,omitempty"`
}

// Checker is a function that performs a health check.
type Checker func() Check

// Manager manages health checks.
type Manager struct {
	mu      sync.RWMutex
	checks  map[string]Checker
	started time.Time
}

// NewManager creates a new health check manager.
func NewManager() *Manager {
	return &Manager{
		checks:  make(map[string]Checker),
		started: time.Now(),
	}
}

// Register registers a health check.
func (m *Manager) Register(name string, checker Checker) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.checks[name] = checker
}

// RunChecks runs all registered health checks and returns the overall health.
func (m *Manager) RunChecks() Health {
	m.mu.RLock()
	defer m.mu.RUnlock()

	health := Health{
		Status:  StatusUp,
		Checks:  make([]Check, 0, len(m.checks)),
		Details: make(map[string]string),
	}

	for _, checker := range m.checks {
		check := checker()
		health.Checks = append(health.Checks, check)

		if check.Status == StatusDown {
			health.Status = StatusDown
		}
	}

	health.Details["uptime"] = time.Since(m.started).String()

	return health
}

// LivezHandler returns an HTTP handler for liveness probe.
// This only checks if the process is running.
func LivezHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "alive"})
	}
}

// ReadyzHandler returns an HTTP handler for readiness probe.
// This checks if the service is ready to accept requests.
func ReadyzHandler(manager *Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := manager.RunChecks()

		w.Header().Set("Content-Type", "application/json")

		if health.Status == StatusDown {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(health)
	}
}

// DatabaseChecker creates a health checker for database connectivity.
func DatabaseChecker(name string, pingFunc func() error) Checker {
	return func() Check {
		check := Check{
			Name:      name,
			Timestamp: time.Now(),
		}

		if err := pingFunc(); err != nil {
			check.Status = StatusDown
			// Don't expose detailed error messages to avoid leaking sensitive information
			check.Message = "database connection failed"
		} else {
			check.Status = StatusUp
		}

		return check
	}
}

// SchemaChecker creates a health checker for database schema version. The
// current version must be greater than or equal to the expected version.
func SchemaChecker(name string, expectedVersion int, versionFunc func() (int, error)) Checker {
	return func() Check {
		check := Check{
			Name:      name,
			Timestamp: time.Now(),
		}

		version, err := versionFunc()
		if err != nil {
			check.Status = StatusDown
			check.Message = "schema version check failed"
			return check
		}

		if version < expectedVersion {
			check.Status = StatusDown
			check.Message = fmt.Sprintf("schema version %d is behind required %d", version, expectedVersion)
			return check
		}

		check.Status = StatusUp
		check.Message = fmt.Sprintf("schema version: %d", version)
		return check
	}
}
