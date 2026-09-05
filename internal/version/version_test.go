package version

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestGet(t *testing.T) {
	info := Get()

	if info.Version == "" {
		t.Error("Expected version to be set")
	}

	if info.Commit == "" {
		t.Error("Expected commit to be set")
	}

	if info.BuildTime == "" {
		t.Error("Expected build time to be set")
	}

	if info.GoVersion == "" {
		t.Error("Expected Go version to be set")
	}

	if info.OS == "" {
		t.Error("Expected OS to be set")
	}

	if info.Arch == "" {
		t.Error("Expected arch to be set")
	}

	// Check that Go version matches runtime
	if info.GoVersion != runtime.Version() {
		t.Errorf("Expected Go version %s, got %s", runtime.Version(), info.GoVersion)
	}

	// Check that OS matches runtime
	if info.OS != runtime.GOOS {
		t.Errorf("Expected OS %s, got %s", runtime.GOOS, info.OS)
	}

	// Check that arch matches runtime
	if info.Arch != runtime.GOARCH {
		t.Errorf("Expected arch %s, got %s", runtime.GOARCH, info.Arch)
	}
}

func TestInfoString(t *testing.T) {
	info := Info{
		Version:   "1.0.0",
		Commit:    "abc123",
		BuildTime: "2024-01-01T00:00:00Z",
		GoVersion: "go1.21.0",
		OS:        "linux",
		Arch:      "amd64",
	}

	str := info.String()

	if str == "" {
		t.Error("Expected non-empty string")
	}

	// Check that string contains key information
	if !contains(str, "share-disk") {
		t.Error("Expected string to contain 'share-disk'")
	}
	if !contains(str, "1.0.0") {
		t.Error("Expected string to contain version")
	}
	if !contains(str, "abc123") {
		t.Error("Expected string to contain commit")
	}
}

func TestHandler(t *testing.T) {
	handler := Handler()

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected content type 'application/json', got '%s'", contentType)
	}

	// Parse response
	var info Info
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Check that response contains valid information
	if info.Version == "" {
		t.Error("Expected version in response")
	}
	if info.GoVersion == "" {
		t.Error("Expected Go version in response")
	}
}

func TestDefaultValues(t *testing.T) {
	// Test that default values are set correctly
	if Version != "dev" {
		t.Errorf("Expected default version 'dev', got '%s'", Version)
	}
	if Commit != "unknown" {
		t.Errorf("Expected default commit 'unknown', got '%s'", Commit)
	}
	if BuildTime != "unknown" {
		t.Errorf("Expected default build time 'unknown', got '%s'", BuildTime)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[0:len(substr)] == substr || contains(s[1:], substr)))
}
