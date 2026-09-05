//go:build integration

package agentsync

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/share-disk/share-disk/internal/catalog"
	"github.com/share-disk/share-disk/internal/controlapi"
	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/internal/identity"
	"github.com/share-disk/share-disk/internal/lanapi"
	"github.com/share-disk/share-disk/internal/storage"
	"github.com/share-disk/share-disk/internal/testutil"
	"github.com/share-disk/share-disk/internal/transfer"
)

// TestAgentToControlCatalogFlow is the executable vertical boundary: an
// authenticated Android session provisions a Linux device, whose durable
// SQLite outbox registers a file through the real HTTP handler into PostgreSQL.
func TestAgentToControlCatalogFlow(t *testing.T) {
	postgres := testutil.NewDB(t)
	privateKey, _, err := identity.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := identity.NewTokenManager(privateKey, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	identities := identity.NewService(postgres.DB, "bootstrap-token", tokens)
	phone, err := identities.Bootstrap(context.Background(), &identity.BootstrapRequest{BootstrapToken: "bootstrap-token", Account: "owner", Password: "password123", DeviceName: "Android", Platform: "android", PeerID: "phone-peer"})
	if err != nil {
		t.Fatal(err)
	}
	catalogRepo := catalog.NewRepository(postgres.DB)
	handler := controlapi.NewHandler(identities, nil, tokens, catalogRepo)
	handler.SetTransferService(transfer.NewService(postgres.DB))
	control := httptest.NewServer(controlapi.NewRouter(handler))
	defer control.Close()

	sqlite, err := database.OpenSQLite(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlite.Close()
	if err := database.NewMigrator(sqlite, testutil.SQLiteMigrationsDir(), "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	client := New(sqlite, control.URL, "http://10.0.0.2:9090", "linux-peer", "Ubuntu Agent", 10*time.Millisecond, 20*time.Millisecond)
	if err := client.Provision(context.Background(), "Bearer "+phone.AccessToken, phone.UserID); err != nil {
		t.Fatal(err)
	}
	if _, err := client.repo.CreateOutgoingOp(context.Background(), "catalog.register", "upload-1", map[string]interface{}{"local_file_id": "upload-1", "name": "hello.txt", "mime": "text/plain", "size": 5, "sha256": "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", "endpoint": "http://10.0.0.2:9090"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go client.Run(ctx)
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		files, err := catalogRepo.ListUnifiedFiles(context.Background(), phone.UserID, false)
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 1 {
			if files[0].Name != "hello.txt" || files[0].ReplicaEndpoint != "http://10.0.0.2:9090" || files[0].OriginDeviceName != "Ubuntu Agent" || files[0].OriginLastSeenAt == nil {
				t.Fatalf("unexpected unified file: %+v", files[0])
			}
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("Agent outbox did not reach PostgreSQL catalog")
		case <-ticker.C:
		}
	}
}

// TestServerScheduledReplicaExecution proves the complete coordinator path:
// the phone schedules a file, the target Agent leases it, downloads real
// bytes from the source, verifies/persists them, and registers a READY replica.
func TestServerScheduledReplicaExecution(t *testing.T) {
	postgres := testutil.NewDB(t)
	privateKey, publicKey, err := identity.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := identity.NewTokenManager(privateKey, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	identities := identity.NewService(postgres.DB, "bootstrap-token", tokens)
	phone, err := identities.Bootstrap(context.Background(), &identity.BootstrapRequest{BootstrapToken: "bootstrap-token", Account: "owner", Password: "password123", DeviceName: "Android", Platform: "android", PeerID: "phone-peer"})
	if err != nil {
		t.Fatal(err)
	}
	catalogRepo := catalog.NewRepository(postgres.DB)
	handler := controlapi.NewHandler(identities, nil, tokens, catalogRepo)
	transfers := transfer.NewService(postgres.DB)
	handler.SetTransferService(transfers)
	control := httptest.NewServer(controlapi.NewRouter(handler))
	defer control.Close()

	content := []byte("hello")
	sourceDB := openAgentDB(t)
	defer sourceDB.Close()
	sourceFilesystem, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sourceStore := storage.NewSQLiteObjectStore(sourceFilesystem, sourceDB, 1024)
	if err := sourceStore.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(t.TempDir(), "hello.txt")
	if err := os.WriteFile(sourcePath, content, 0600); err != nil {
		t.Fatal(err)
	}
	sourceObject, err := sourceStore.Import(context.Background(), sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sourceStore.GetStore().CreateLANFile("source-file", phone.UserID, sourceObject.Hash.String(), "hello.txt", "hello.txt", "text/plain"); err != nil {
		t.Fatal(err)
	}
	sourceLAN, err := lanapi.New("127.0.0.1:0", publicKey, sourceFilesystem.IncomingDir(), 1<<20, 24*time.Hour, "", sourceStore)
	if err != nil {
		t.Fatal(err)
	}
	if err := sourceLAN.Listen(); err != nil {
		t.Fatal(err)
	}
	sourceCtx, stopSource := context.WithCancel(context.Background())
	defer stopSource()
	go func() { _ = sourceLAN.Serve(sourceCtx) }()
	defer sourceLAN.Close()
	sourceEndpoint := "http://" + sourceLAN.Address()

	source := New(sourceDB, control.URL, sourceEndpoint, "source-peer", "Source Agent", 10*time.Millisecond, time.Hour)
	if err := source.Provision(context.Background(), "Bearer "+phone.AccessToken, phone.UserID); err != nil {
		t.Fatal(err)
	}
	sourceDeviceID, err := source.setting(context.Background(), settingDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := catalogRepo.RegisterVerifiedFile(context.Background(), phone.UserID, sourceDeviceID, catalog.RegisterFileRequest{LocalFileID: "source-file", Name: "hello.txt", MIME: "text/plain", Size: 5, SHA256: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", Endpoint: sourceEndpoint})
	if err != nil {
		t.Fatal(err)
	}

	targetDB := openAgentDB(t)
	defer targetDB.Close()
	root := t.TempDir()
	filesystem, err := storage.New(root)
	if err != nil {
		t.Fatal(err)
	}
	targetStore := storage.NewSQLiteObjectStore(filesystem, targetDB, 1024)
	if err := targetStore.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	targetEndpoint := "http://127.0.0.1:19091"
	target := New(targetDB, control.URL, targetEndpoint, "target-peer", "Target Agent", 10*time.Millisecond, time.Hour)
	target.SetTransferStore(targetStore, filesystem.IncomingDir())
	if err := target.Provision(context.Background(), "Bearer "+phone.AccessToken, phone.UserID); err != nil {
		t.Fatal(err)
	}
	targetDeviceID, err := target.setting(context.Background(), settingDeviceID)
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]interface{}{"file_id": registered.ID, "target_device_id": targetDeviceID, "priority": 10})
	req, _ := http.NewRequest(http.MethodPost, control.URL+"/v1/transfers/", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer "+phone.AccessToken)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("schedule returned %d: %s", resp.StatusCode, body)
	}
	var task transfer.TransferTask
	if err := json.Unmarshal(body, &task); err != nil {
		t.Fatal(err)
	}
	retryReq, _ := http.NewRequest(http.MethodPost, control.URL+"/v1/transfers/", bytes.NewReader(payload))
	retryReq.Header.Set("Authorization", "Bearer "+phone.AccessToken)
	retryReq.Header.Set("Content-Type", "application/json")
	retryResp, err := http.DefaultClient.Do(retryReq)
	if err != nil {
		t.Fatal(err)
	}
	var retryTask transfer.TransferTask
	if err := json.NewDecoder(retryResp.Body).Decode(&retryTask); err != nil {
		retryResp.Body.Close()
		t.Fatal(err)
	}
	retryResp.Body.Close()
	if retryResp.StatusCode != http.StatusCreated || retryTask.ID != task.ID {
		t.Fatalf("schedule retry was not idempotent: status=%d first=%s retry=%s", retryResp.StatusCode, task.ID, retryTask.ID)
	}

	target.executeOneTransfer(context.Background())
	current, err := transfers.GetTask(context.Background(), phone.UserID, task.ID)
	if err != nil || current.State != "completed" {
		t.Fatalf("target Agent did not complete scheduled replica: task=%+v err=%v", current, err)
	}
	file, err := targetStore.GetStore().GetLANFile(task.ID, phone.UserID)
	if err != nil || file == nil || file.SHA256 != registered.SHA256 {
		t.Fatalf("target file was not persisted: file=%+v err=%v", file, err)
	}
	hash, err := storage.ParseHash(registered.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(filesystem.ObjectPath(hash))
	if err != nil || !bytes.Equal(stored, content) {
		t.Fatalf("target bytes mismatch: bytes=%q err=%v", stored, err)
	}
	var replicaState, localID, endpoint string
	if err := postgres.DB.QueryRow(`SELECT state,client_file_id,endpoint FROM replicas WHERE user_id=$1 AND object_id=$2 AND device_id=$3`, phone.UserID, task.ObjectID, targetDeviceID).Scan(&replicaState, &localID, &endpoint); err != nil {
		t.Fatal(err)
	}
	if replicaState != "ready" || localID != task.ID || endpoint != targetEndpoint {
		t.Fatalf("unexpected target replica: state=%s local=%s endpoint=%s", replicaState, localID, endpoint)
	}
	if _, err := catalogRepo.ApplyLifecycleReport(context.Background(), phone.UserID, sourceDeviceID, catalog.LifecycleReport{OperationID: "rename-all", LocalFileID: "source-file", Action: "rename", Name: "renamed.txt"}); err != nil {
		t.Fatal(err)
	}
	target.executeOneDeviceCommand(context.Background())
	renamed, err := targetStore.GetStore().GetLANFile(task.ID, phone.UserID)
	if err != nil || renamed == nil || renamed.Name != "renamed.txt" {
		t.Fatalf("replica lifecycle command did not converge: file=%+v err=%v", renamed, err)
	}
}

func openAgentDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := database.OpenSQLite(filepath.Join(t.TempDir(), "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.NewMigrator(db, testutil.SQLiteMigrationsDir(), "sqlite").MigrateUp(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}
