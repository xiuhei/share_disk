package agentsync

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/internal/testutil"
	"github.com/share-disk/share-disk/internal/transfer"
)

func TestProvisionAndDeliverDurableOutbox(t *testing.T) {
	db, err := database.OpenSQLite(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.NewMigrator(db, testutil.SQLiteMigrationsDir(), "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	var delivered atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices/register":
			if r.Header.Get("Authorization") != "Bearer phone-token" {
				t.Errorf("unexpected provisioning authorization")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "agent-token", "refresh_token": "refresh-token", "token_type": "Bearer", "expires_in": 900, "user_id": "user-1", "device_id": "server-device-1"})
		case "/v1/catalog/files/register":
			if r.Header.Get("Authorization") != "Bearer agent-token" {
				t.Errorf("outbox reused caller token")
			}
			delivered.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]string{"id": "server-file-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(db, server.URL, "http://agent.local:9090", "peer-1", "linux-1", 10*time.Millisecond, time.Hour)
	if err := client.Provision(context.Background(), "Bearer phone-token", "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.repo.CreateOutgoingOp(context.Background(), "catalog.register", "op-1", map[string]interface{}{"local_file_id": "local-1"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var status string
	for status != "completed" {
		if err := db.QueryRow(`SELECT status FROM outgoing_ops WHERE idempotency_key='op-1'`).Scan(&status); err != nil {
			t.Fatal(err)
		}
		select {
		case <-deadline.C:
			t.Fatal("outbox was not delivered")
		case <-ticker.C:
		}
	}
	if status != "completed" {
		t.Fatalf("unexpected outbox status %q", status)
	}
	if delivered.Load() != 1 {
		t.Fatalf("expected one delivery, got %d", delivered.Load())
	}
}

func TestOutboxRetryBlocksLaterLifecycleOperation(t *testing.T) {
	db, err := database.OpenSQLite(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.NewMigrator(db, testutil.SQLiteMigrationsDir(), "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	client := New(db, "http://control.invalid", "http://agent.local:9090", "peer-1", "linux-1", time.Second, time.Hour)
	first, err := client.repo.CreateOutgoingOp(context.Background(), "catalog.register", "first", map[string]string{"local_file_id": "one"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.repo.CreateOutgoingOp(context.Background(), "catalog.lifecycle", "second", map[string]string{"local_file_id": "one", "action": "trash"}); err != nil {
		t.Fatal(err)
	}
	if err := client.repo.MarkOpRetryable(context.Background(), first.ID, time.Hour); err != nil {
		t.Fatal(err)
	}
	ops, err := client.repo.GetPendingOps(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Fatalf("later operation bypassed retrying predecessor: %+v", ops)
	}
}

func TestTransientControlFailureNeverPermanentlyDropsRegistration(t *testing.T) {
	db, err := database.OpenSQLite(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.NewMigrator(db, testutil.SQLiteMigrationsDir(), "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "temporarily unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := New(db, server.URL, "http://agent.local:9090", "peer-1", "linux-1", time.Second, time.Hour)
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES
		('control_access_token','agent-token'),
		('control_refresh_token','refresh-token')`); err != nil {
		t.Fatal(err)
	}
	op, err := client.repo.CreateOutgoingOp(context.Background(), "catalog.register", "durable-registration", map[string]string{"local_file_id": "one"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE outgoing_ops SET retry_count=100 WHERE id=?`, op.ID); err != nil {
		t.Fatal(err)
	}
	op.RetryCount = 100

	if err := client.deliver(context.Background(), op); err != nil {
		t.Fatal(err)
	}
	var status string
	var retries int
	if err := db.QueryRow(`SELECT status,retry_count FROM outgoing_ops WHERE id=?`, op.ID).Scan(&status, &retries); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || retries != 101 {
		t.Fatalf("transient registration was not retained for retry: status=%s retries=%d", status, retries)
	}
}

func TestProvisionReplacesRevokedStoredSession(t *testing.T) {
	db, err := database.OpenSQLite(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.NewMigrator(db, testutil.SQLiteMigrationsDir(), "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES
		('control_access_token','revoked-access'),
		('control_refresh_token','revoked-refresh'),
		('control_user_id','user-1'),
		('control_device_id','device-1')`); err != nil {
		t.Fatal(err)
	}
	var registrations atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/devices/heartbeat", "/v1/auth/refresh":
			w.WriteHeader(http.StatusUnauthorized)
		case "/v1/devices/register":
			registrations.Add(1)
			if r.Header.Get("Authorization") != "Bearer caller-access" {
				t.Errorf("registration did not use caller session")
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"access_token": "replacement-access", "refresh_token": "replacement-refresh", "user_id": "user-1", "device_id": "device-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(db, server.URL, "http://agent.local:9090", "peer-1", "linux-1", time.Second, time.Hour)
	if err := client.Provision(context.Background(), "Bearer caller-access", "user-1"); err != nil {
		t.Fatal(err)
	}
	if registrations.Load() != 1 {
		t.Fatalf("expected one replacement registration, got %d", registrations.Load())
	}
	var access string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key='control_access_token'`).Scan(&access); err != nil {
		t.Fatal(err)
	}
	if access != "replacement-access" {
		t.Fatalf("stored Agent session was not replaced: %q", access)
	}
}

func TestDownloadReplicaResumesStablePartialFile(t *testing.T) {
	db, err := database.OpenSQLite(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.NewMigrator(db, testutil.SQLiteMigrationsDir(), "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES('control_access_token','agent-token')`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") != "bytes=2-" || r.Header.Get("If-Range") != `"expected-hash"` {
			t.Errorf("unexpected resume headers: Range=%q If-Range=%q", r.Header.Get("Range"), r.Header.Get("If-Range"))
		}
		w.Header().Set("Content-Range", "bytes 2-4/5")
		w.Header().Set("X-Content-SHA256", "expected-hash")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write([]byte("llo"))
	}))
	defer server.Close()

	path := filepath.Join(t.TempDir(), "transfer.part")
	if err := os.WriteFile(path, []byte("he"), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	client := New(db, "http://control.invalid", "", "peer", "agent", time.Second, time.Hour)
	assignment := transfer.Assignment{TaskID: "task-1", SourceEndpoint: server.URL, SourceLocalFileID: "source-file", Size: 5, SHA256: "expected-hash"}
	if err := client.downloadReplica(context.Background(), assignment, file); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, []byte("hello")) {
		t.Fatalf("resumed content mismatch: %q", content)
	}
}
