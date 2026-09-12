package lanapi

import (
	"context"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/share-disk/share-disk/client/agent/internal/storage"
	"github.com/share-disk/share-disk/internal/auth"
	"github.com/share-disk/share-disk/internal/database"
)

func TestBrowserDownloadStreamsVerifiedBytes(t *testing.T) {
	root := t.TempDir()
	db, err := database.OpenSQLite(filepath.Join(root, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.NewMigrator(db, filepath.Join("..", "..", "migrations", "sqlite"), "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	objects, err := storage.New(filepath.Join(root, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	store := storage.NewSQLiteObjectStore(objects, db, 4)
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "file.txt")
	if err := os.WriteFile(source, []byte("verified file bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	object, err := store.Import(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetStore().CreateLANFile("file1", "user1", object.Hash.String(), "file.txt", "file.txt", "text/plain"); err != nil {
		t.Fatal(err)
	}
	key, public, err := auth.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := auth.NewTokenManager(key, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := auth.NewTokenVerifier(public)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := signer.GenerateBrowserDownloadToken("user1", "file1", "browser-session", "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	normal, _ := signer.GenerateAccessToken("user1", "device1", "session1")
	wrongOwner, _ := signer.GenerateBrowserDownloadToken("user2", "file1", "session2", "http://localhost:3000")
	handler := browserDownload(verifier, store)
	for _, tc := range []struct {
		name, token, origin, method string
		want                        int
	}{
		{"real file", ticket, "http://localhost:3000", "POST", 200},
		{"wrong origin", ticket, "http://evil.test", "POST", 403},
		{"no origin", ticket, "", "POST", 403},
		{"ordinary bearer cannot use adapter", normal, "http://localhost:3000", "POST", 403},
		{"other account", wrongOwner, "http://localhost:3000", "POST", 404},
		{"bad ticket", "invalid", "http://localhost:3000", "POST", 401},
		{"get refused", ticket, "http://localhost:3000", "GET", 405},
	} {
		t.Run(tc.name, func(t *testing.T) {
			form := url.Values{"ticket": {tc.token}}
			request := httptest.NewRequest(tc.method, "http://agent/v1/lan/browser-download", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", tc.origin)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.want {
				t.Fatalf("status %d: %s", response.Code, response.Body.String())
			}
			if tc.want == 200 && (response.Body.String() != "verified file bytes" || !strings.Contains(response.Header().Get("Content-Disposition"), "attachment")) {
				t.Fatal("download bytes or attachment headers missing")
			}
		})
	}
	request := httptest.NewRequest("POST", "http://agent/v1/lan/browser-download?ticket="+url.QueryEscape(ticket), strings.NewReader(""))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 401 {
		t.Fatal("ticket in URL accepted")
	}
}
