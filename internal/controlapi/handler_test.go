package controlapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/share-disk/share-disk/internal/identity"
)

func TestLivez(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	router := NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestVersionReturnsFullInfo(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	router := NewRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	for _, field := range []string{"version", "commit", "build_time", "go_version", "os", "arch"} {
		if _, ok := body[field]; !ok {
			t.Errorf("version response missing field %q", field)
		}
	}
}

func TestAdminConsoleServesResponsiveLoginShell(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	router := NewMonitoringRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	for _, marker := range []string{"Share Disk 管理监控台", "文件消息", "设备连接", "修改密码"} {
		if !strings.Contains(rr.Body.String(), marker) {
			t.Fatalf("admin console missing %q", marker)
		}
	}
}

func TestAdminConsoleIsDisabledByDefault(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	router := NewRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected disabled console to return 404, got %d", rr.Code)
	}
}

func TestLogoutRequiresAuth(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	router := NewRouter(h)

	req := httptest.NewRequest(http.MethodPost, "/v1/auth/logout", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}

	// Error body must match the OpenAPI shape: {error: {code, message}}.
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if body.Error.Code == "" || body.Error.Message == "" {
		t.Fatalf("error response missing code/message: %+v", body)
	}
}

func TestBootstrapRejectsTrailingData(t *testing.T) {
	h := NewHandler(nil, nil, nil)
	router := NewRouter(h)

	body := `{"account":"a","password":"password123","bootstrap_token":"x"} {"extra":"junk"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bootstrap", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing data, got %d", rr.Code)
	}
}

// 3.1.4: semantic validation errors must map to 400, not 500.
func TestBootstrapValidationErrorMapsToBadRequest(t *testing.T) {
	svc := identity.NewService(nil, "test-bootstrap-token", nil)
	h := NewHandler(svc, nil, nil)
	router := NewRouter(h)

	body := `{"bootstrap_token":"test-bootstrap-token","account":"","password":""}`
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/bootstrap", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing account, got %d", rr.Code)
	}

	var errBody struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("failed to decode error body: %v", err)
	}
	if errBody.Error.Code != "INVALID_REQUEST" {
		t.Fatalf("expected INVALID_REQUEST code, got %s", errBody.Error.Code)
	}
}
