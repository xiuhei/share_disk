package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	manager := NewManager()
	if manager == nil {
		t.Fatal("Expected non-nil manager")
	}

	if manager.checks == nil {
		t.Error("Expected checks map to be initialized")
	}

	if manager.started.IsZero() {
		t.Error("Expected started time to be set")
	}
}

func TestRegisterAndRunChecks(t *testing.T) {
	manager := NewManager()

	// Register a passing check
	manager.Register("test-pass", func() Check {
		return Check{
			Name:      "test-pass",
			Status:    StatusUp,
			Message:   "OK",
			Timestamp: time.Now(),
		}
	})

	// Register a failing check
	manager.Register("test-fail", func() Check {
		return Check{
			Name:      "test-fail",
			Status:    StatusDown,
			Message:   "failed",
			Timestamp: time.Now(),
		}
	})

	health := manager.RunChecks()

	if health.Status != StatusDown {
		t.Errorf("Expected status DOWN, got %s", health.Status)
	}

	if len(health.Checks) != 2 {
		t.Errorf("Expected 2 checks, got %d", len(health.Checks))
	}

	if health.Details["uptime"] == "" {
		t.Error("Expected uptime to be set")
	}
}

func TestRunChecksAllPass(t *testing.T) {
	manager := NewManager()

	manager.Register("test1", func() Check {
		return Check{
			Name:   "test1",
			Status: StatusUp,
		}
	})

	manager.Register("test2", func() Check {
		return Check{
			Name:   "test2",
			Status: StatusUp,
		}
	})

	health := manager.RunChecks()

	if health.Status != StatusUp {
		t.Errorf("Expected status UP, got %s", health.Status)
	}
}

func TestLivezHandler(t *testing.T) {
	handler := LivezHandler()

	req := httptest.NewRequest("GET", "/livez", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "alive" {
		t.Errorf("Expected status 'alive', got '%s'", response["status"])
	}
}

func TestReadyzHandler(t *testing.T) {
	manager := NewManager()

	// Register a passing check
	manager.Register("test", func() Check {
		return Check{
			Name:   "test",
			Status: StatusUp,
		}
	})

	handler := ReadyzHandler(manager)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response Health
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Status != StatusUp {
		t.Errorf("Expected status UP, got %s", response.Status)
	}
}

func TestReadyzHandlerWithFailingCheck(t *testing.T) {
	manager := NewManager()

	// Register a failing check
	manager.Register("test", func() Check {
		return Check{
			Name:   "test",
			Status: StatusDown,
		}
	})

	handler := ReadyzHandler(manager)

	req := httptest.NewRequest("GET", "/readyz", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 503, got %d", w.Code)
	}
}

func TestDatabaseChecker(t *testing.T) {
	// Test with successful ping
	checker := DatabaseChecker("test-db", func() error {
		return nil
	})

	check := checker()
	if check.Status != StatusUp {
		t.Errorf("Expected status UP, got %s", check.Status)
	}
	if check.Name != "test-db" {
		t.Errorf("Expected name 'test-db', got '%s'", check.Name)
	}

	// Test with failing ping
	checker = DatabaseChecker("test-db", func() error {
		return errors.New("connection failed")
	})

	check = checker()
	if check.Status != StatusDown {
		t.Errorf("Expected status DOWN, got %s", check.Status)
	}
	if check.Message != "database connection failed" {
		t.Errorf("Expected message 'database connection failed', got '%s'", check.Message)
	}
}
