package contract

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// specPath locates the OpenAPI spec relative to this package directory.
func specPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	// Walk up to the repository root (module github.com/share-disk/share-disk).
	for i := 0; i < 5; i++ {
		candidate := filepath.Join(dir, "api", "openapi", "openapi.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("openapi.yaml not found")
	return ""
}

func loadSpec(t *testing.T) map[string]interface{} {
	t.Helper()
	data, err := os.ReadFile(specPath(t))
	if err != nil {
		t.Fatalf("failed to read openapi.yaml: %v", err)
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("failed to parse openapi.yaml: %v", err)
	}
	return root
}

func TestOpenAPIContract(t *testing.T) {
	root := loadSpec(t)

	if root["openapi"] != "3.1.0" {
		t.Fatalf("expected openapi 3.1.0, got %v", root["openapi"])
	}

	info, ok := root["info"].(map[string]interface{})
	if !ok || info["title"] == nil || info["version"] == nil {
		t.Fatal("info.title and info.version are required")
	}

	paths, ok := root["paths"].(map[string]interface{})
	if !ok || len(paths) == 0 {
		t.Fatal("paths must be present and non-empty")
	}

	required := []string{
		"/livez", "/readyz", "/version",
		"/v1/auth/bootstrap", "/v1/auth/login", "/v1/auth/refresh", "/v1/auth/logout",
		"/v1/lan/files", "/v1/lan/files/{file_id}", "/v1/lan/files/{file_id}/content",
		"/v1/lan/trash", "/v1/lan/trash/{file_id}", "/v1/lan/trash/{file_id}/restore",
	}
	for _, p := range required {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing required path %s in OpenAPI spec", p)
		}
	}
}

func TestOpenAPIErrorSchema(t *testing.T) {
	root := loadSpec(t)

	components, ok := root["components"].(map[string]interface{})
	if !ok {
		t.Fatal("components is required")
	}
	schemas, ok := components["schemas"].(map[string]interface{})
	if !ok {
		t.Fatal("components.schemas is required")
	}

	errSchema, ok := schemas["Error"].(map[string]interface{})
	if !ok {
		t.Fatal("Error schema is required")
	}
	errProps, ok := errSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Error schema must have properties")
	}
	errObj, ok := errProps["error"].(map[string]interface{})
	if !ok {
		t.Fatal("Error.error must be an object")
	}
	errFields, ok := errObj["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Error.error must have code/message")
	}
	if _, ok := errFields["code"]; !ok {
		t.Error("Error.error.code is required")
	}
	if _, ok := errFields["message"]; !ok {
		t.Error("Error.error.message is required")
	}
}

func TestOpenAPIAuthResponse(t *testing.T) {
	root := loadSpec(t)

	components := root["components"].(map[string]interface{})
	schemas := components["schemas"].(map[string]interface{})
	auth := schemas["AuthResponse"].(map[string]interface{})

	required, ok := auth["required"].([]interface{})
	if !ok {
		t.Fatal("AuthResponse must declare required fields")
	}
	requiredSet := map[string]bool{}
	for _, r := range required {
		requiredSet[r.(string)] = true
	}
	for _, f := range []string{"access_token", "refresh_token", "expires_in", "token_type"} {
		if !requiredSet[f] {
			t.Errorf("AuthResponse must require %q", f)
		}
	}
}
