//go:build integration

package catalog_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/share-disk/share-disk/internal/catalog"
	"github.com/share-disk/share-disk/internal/testutil"
)

func TestUnifiedCatalogRegistrationAndLifecycle(t *testing.T) {
	db := testutil.NewDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	deviceID := uuid.NewString()
	otherDeviceID := uuid.NewString()
	mustExec := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	mustExec(`INSERT INTO users (id,account,password_hash) VALUES ($1,'catalog-user','hash')`, userID)
	mustExec(`INSERT INTO devices (id,user_id,name,platform,public_key,peer_id) VALUES ($1,$2,'Ubuntu Agent','linux',$3,$4)`, deviceID, userID, []byte("key-1"), "peer-1")
	mustExec(`INSERT INTO devices (id,user_id,name,platform,public_key,peer_id) VALUES ($1,$2,'Other Agent','linux',$3,$4)`, otherDeviceID, userID, []byte("key-2"), "peer-2")

	repo := catalog.NewRepository(db.DB)
	req := catalog.RegisterFileRequest{
		LocalFileID: "local-file-1",
		Name:        "hello.txt",
		MIME:        "text/plain",
		Size:        5,
		SHA256:      "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
		Endpoint:    "http://10.0.0.2:9090",
	}
	created, err := repo.RegisterVerifiedFile(ctx, userID, deviceID, req)
	if err != nil {
		t.Fatal(err)
	}
	if created.LocalFileID != req.LocalFileID || created.OriginDeviceID != deviceID || created.ReplicaEndpoint != req.Endpoint {
		t.Fatalf("unexpected registered file: %+v", created)
	}
	retried, err := repo.RegisterVerifiedFile(ctx, userID, deviceID, req)
	if err != nil || retried.ID != created.ID {
		t.Fatalf("registration retry created a different entry: file=%+v err=%v", retried, err)
	}

	files, err := repo.ListUnifiedFiles(ctx, userID, false)
	if err != nil || len(files) != 1 || files[0].ID != created.ID {
		t.Fatalf("unexpected unified list: %+v err=%v", files, err)
	}

	rename := catalog.LifecycleReport{OperationID: "op-rename", LocalFileID: req.LocalFileID, Action: "rename", Name: "renamed.txt"}
	renamed, err := repo.ApplyLifecycleReport(ctx, userID, deviceID, rename)
	if err != nil || renamed.Name != "renamed.txt" {
		t.Fatalf("rename failed: file=%+v err=%v", renamed, err)
	}
	renamedRetry, err := repo.ApplyLifecycleReport(ctx, userID, deviceID, rename)
	if err != nil || renamedRetry.Version != renamed.Version {
		t.Fatalf("rename retry was not idempotent: first=%+v retry=%+v err=%v", renamed, renamedRetry, err)
	}
	if _, err := repo.ApplyLifecycleReport(ctx, userID, otherDeviceID, catalog.LifecycleReport{OperationID: "cross-device", LocalFileID: req.LocalFileID, Action: "trash"}); !errors.Is(err, catalog.ErrFileNotFound) {
		t.Fatalf("cross-device report = %v, want not found", err)
	}

	purgeAfter := time.Now().UTC().Add(time.Hour)
	trashed, err := repo.ApplyLifecycleReport(ctx, userID, deviceID, catalog.LifecycleReport{OperationID: "op-trash", LocalFileID: req.LocalFileID, Action: "trash", PurgeAfter: &purgeAfter})
	if err != nil || trashed.Status != "trashed" || trashed.PurgeAfter == nil {
		t.Fatalf("trash failed: file=%+v err=%v", trashed, err)
	}
	trash, err := repo.ListUnifiedFiles(ctx, userID, true)
	if err != nil || len(trash) != 1 || trash[0].ID != created.ID {
		t.Fatalf("unexpected unified trash: %+v err=%v", trash, err)
	}
	restored, err := repo.ApplyLifecycleReport(ctx, userID, deviceID, catalog.LifecycleReport{OperationID: "op-restore", LocalFileID: req.LocalFileID, Action: "restore"})
	if err != nil || restored.Status != "active" {
		t.Fatalf("restore failed: file=%+v err=%v", restored, err)
	}
	if _, err := repo.ApplyLifecycleReport(ctx, userID, deviceID, catalog.LifecycleReport{OperationID: "op-trash-2", LocalFileID: req.LocalFileID, Action: "trash", PurgeAfter: &purgeAfter}); err != nil {
		t.Fatal(err)
	}
	purged, err := repo.ApplyLifecycleReport(ctx, userID, deviceID, catalog.LifecycleReport{OperationID: "op-purge", LocalFileID: req.LocalFileID, Action: "purge"})
	if err != nil || purged.Status != "purged" {
		t.Fatalf("purge failed: file=%+v err=%v", purged, err)
	}
	files, err = repo.ListUnifiedFiles(ctx, userID, false)
	if err != nil || len(files) != 0 {
		t.Fatalf("purged file remains active: %+v err=%v", files, err)
	}
}

func TestUnifiedCatalogRejectsNameConflict(t *testing.T) {
	db := testutil.NewDB(t)
	ctx := context.Background()
	userID := uuid.NewString()
	deviceID := uuid.NewString()
	_, _ = db.ExecContext(ctx, `INSERT INTO users (id,account,password_hash) VALUES ($1,'conflict-user','hash')`, userID)
	_, _ = db.ExecContext(ctx, `INSERT INTO devices (id,user_id,name,platform,public_key,peer_id) VALUES ($1,$2,'Agent','linux',$3,$4)`, deviceID, userID, []byte("key"), "peer")
	repo := catalog.NewRepository(db.DB)
	base := catalog.RegisterFileRequest{Name: "same.txt", MIME: "text/plain", Size: 1, Endpoint: "http://127.0.0.1:9090"}
	base.LocalFileID = "one"
	base.SHA256 = "ca978112ca1bbdcafac231b39a23dc4da786eff8147c4e72b9807785afee48bb"
	if _, err := repo.RegisterVerifiedFile(ctx, userID, deviceID, base); err != nil {
		t.Fatal(err)
	}
	base.LocalFileID = "two"
	base.SHA256 = "3e23e8160039594a33894f6564e1b1348bbd7a0088d42c4acb73eeaed59c009d"
	if _, err := repo.RegisterVerifiedFile(ctx, userID, deviceID, base); !errors.Is(err, catalog.ErrNameConflict) {
		t.Fatalf("second registration = %v, want name conflict", err)
	}
}
