package controlapi

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/share-disk/share-disk/server/internal/catalog"
	"github.com/share-disk/share-disk/server/internal/identity"
	"github.com/share-disk/share-disk/server/internal/transfer"
)

// An opt-in, loopback-only real API for manual/browser acceptance. It uses the
// same isolated schema helper as integration tests, and cleans up on shutdown.
func TestBrowserAcceptanceServer(t *testing.T) {
	if os.Getenv("SHARE_DISK_BROWSER_ACCEPTANCE") != "1" {
		t.Skip("opt-in browser acceptance server")
	}
	db := browserTestDB(t)
	hash, err := identity.HashPassword("acceptance-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := identity.NewUserRepository(db).Create(context.Background(), "acceptance-user", hash); err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(identity.NewService(db, "", nil), nil, nil, catalog.NewRepository(db))
	handler.SetTransferService(transfer.NewService(db))
	router := NewRouter(handler)
	done := make(chan struct{}, 1)
	router.Post("/__test_shutdown", func(w http.ResponseWriter, r *http.Request) {
		select {
		case done <- struct{}{}:
		default:
		}
		w.WriteHeader(204)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: router, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	t.Log("Browser acceptance API listening on 127.0.0.1:8080 (synthetic acceptance-user)")
	select {
	case <-done:
	case <-time.After(10 * time.Minute):
	}
}
