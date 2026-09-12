package controlapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/server/internal/catalog"
	"github.com/share-disk/share-disk/server/internal/identity"
)

// Each run uses its own schema; never truncate shared or deployment tables.
func browserTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("SHARE_DISK_TEST_POSTGRES")
	if dsn == "" {
		t.Skip("SHARE_DISK_TEST_POSTGRES is required for PostgreSQL integration tests")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Fatal("test DSN must be a PostgreSQL URL")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Install shared functions outside disposable schemas. Otherwise concurrent
	// fixtures see the extension installed but cannot resolve its UUID function.
	if _, err := admin.Exec(`CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public`); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	schema := "browser_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(`CREATE SCHEMA "` + schema + `"`); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, err := admin.Exec(`DROP SCHEMA "` + schema + `" CASCADE`)
		if err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	q := parsed.Query()
	q.Set("search_path", schema+",public")
	parsed.RawQuery = q.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	migrations := filepath.Join("..", "..", "migrations", "postgres")
	if err := database.NewMigrator(db, migrations, "postgres").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestBrowserSessionIntegration(t *testing.T) {
	db := browserTestDB(t)
	service := identity.NewService(db, "", nil)
	hash, err := identity.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	user, err := identity.NewUserRepository(db).Create(context.Background(), "browser-owner", hash)
	if err != nil {
		t.Fatal(err)
	}
	other, err := identity.NewUserRepository(db).Create(context.Background(), "other-owner", hash)
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(NewHandler(service, nil, nil, catalog.NewRepository(db)))
	call := func(method, path string, payload interface{}, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
		t.Helper()
		var body []byte
		if payload != nil {
			body, _ = json.Marshal(payload)
		}
		r := httptest.NewRequest(method, "http://localhost"+path, bytes.NewReader(body))
		r.Header.Set("Origin", "http://localhost")
		if payload != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		r.Header.Set("X-CSRF-Token", csrf)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w
	}
	login := func(account, password string, remember bool) (*http.Cookie, string) {
		t.Helper()
		response := call("POST", "/v1/browser/login", map[string]interface{}{"account": account, "password": password, "remember": remember}, nil, "")
		if response.Code != 200 {
			t.Fatalf("login status %d: %s", response.Code, response.Body.String())
		}
		cookies := response.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/v1" {
			t.Fatal("invalid session cookie policy")
		}
		if remember != (cookies[0].MaxAge > 0) {
			t.Fatal("remember-me policy ignored")
		}
		var result struct {
			CSRF string `json:"csrf_token"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.CSRF == "" {
			t.Fatal("missing CSRF token")
		}
		if strings.Contains(response.Body.String(), cookies[0].Value) || strings.Contains(response.Body.String(), "refresh_token") {
			t.Fatal("credential exposed in JSON")
		}
		return cookies[0], result.CSRF
	}
	bad := call("POST", "/v1/browser/login", map[string]string{"account": user.Account, "password": "wrong"}, nil, "")
	if bad.Code != 401 {
		t.Fatalf("bad password: %d", bad.Code)
	}
	cookie, csrf := login(user.Account, "correct-password", true)
	second, _ := login(user.Account, "correct-password", false)
	otherCookie, otherCSRF := login(other.Account, "correct-password", false)
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM browser_sessions WHERE token_hash=$1`, cookie.Value).Scan(&count); err != nil || count != 0 {
		t.Fatal("cookie stored in plaintext")
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM devices`).Scan(&count); err != nil || count != 0 {
		t.Fatal("browser login fabricated a storage device")
	}
	if got := call("GET", "/v1/browser/session", nil, cookie, ""); got.Code != 200 {
		t.Fatalf("session restore: %d", got.Code)
	}
	if got := call("POST", "/v1/catalog/folders", map[string]string{"name": "Secret"}, cookie, ""); got.Code != 403 {
		t.Fatalf("missing CSRF: %d", got.Code)
	}
	if got := call("POST", "/v1/catalog/files/register", map[string]string{}, cookie, csrf); got.Code != 403 {
		t.Fatalf("browser impersonated Agent: %d", got.Code)
	}
	created := call("POST", "/v1/catalog/folders", map[string]string{"name": "Secret"}, cookie, csrf)
	if created.Code != 201 {
		t.Fatalf("create folder: %d %s", created.Code, created.Body.String())
	}
	var folder catalog.Folder
	if err := json.Unmarshal(created.Body.Bytes(), &folder); err != nil {
		t.Fatal(err)
	}
	if got := call("PATCH", "/v1/catalog/folders/"+folder.ID, map[string]string{"name": "stolen"}, otherCookie, otherCSRF); got.Code != 404 {
		t.Fatalf("cross account write: %d", got.Code)
	}
	if got := call("GET", "/v1/catalog/folders", nil, otherCookie, ""); strings.Contains(got.Body.String(), "Secret") {
		t.Fatal("cross account read leaked folder")
	}
	changed := call("PATCH", "/v1/account/password", map[string]string{"current_password": "correct-password", "new_password": "changed-password"}, cookie, csrf)
	if changed.Code != 200 {
		t.Fatalf("change password: %d %s", changed.Code, changed.Body.String())
	}
	if got := call("GET", "/v1/browser/session", nil, second, ""); got.Code != 401 {
		t.Fatal("other browser session survived password change")
	}
	if got := call("GET", "/v1/browser/session", nil, cookie, ""); got.Code != 200 {
		t.Fatal("current session revoked by password change")
	}
	if got := call("POST", "/v1/browser/logout", nil, cookie, csrf); got.Code != 204 {
		t.Fatalf("logout: %d", got.Code)
	}
	if got := call("GET", "/v1/account", nil, cookie, ""); got.Code != 401 {
		t.Fatal("logged-out cookie replay accepted")
	}
	expired, _ := login(user.Account, "changed-password", false)
	if _, err := db.Exec(`UPDATE browser_sessions SET expires_at=$1 WHERE user_id=$2`, time.Now().Add(-time.Hour), user.ID); err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "/v1/browser/session", nil, expired, ""); got.Code != 401 {
		t.Fatal("expired session accepted")
	}
}
