package controlapi

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/share-disk/share-disk/server/internal/catalog"
	"github.com/share-disk/share-disk/server/internal/identity"
)

func TestCatalogActionsAtomicBatchAndDeviceCommands(t *testing.T) {
	db := browserTestDB(t)
	ctx := context.Background()
	user, err := identity.NewUserRepository(db).Create(ctx, "actions-user", "unused-test-hash")
	if err != nil {
		t.Fatal(err)
	}
	device, err := identity.NewDeviceRepository(db).Create(ctx, user.ID, "Storage", "ubuntu", []byte("test-key"), "test-peer")
	if err != nil {
		t.Fatal(err)
	}
	repo := catalog.NewRepository(db)
	first, err := repo.RegisterVerifiedFile(ctx, user.ID, device.ID, catalog.RegisterFileRequest{LocalFileID: "local-one", Name: "one.txt", MIME: "text/plain", Size: 4, SHA256: strings.Repeat("a", 64), Endpoint: "http://127.0.0.1:9080"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.RegisterVerifiedFile(ctx, user.ID, device.ID, catalog.RegisterFileRequest{LocalFileID: "local-two", Name: "two.txt", MIME: "text/plain", Size: 4, SHA256: strings.Repeat("b", 64), Endpoint: "http://127.0.0.1:9080"})
	if err != nil {
		t.Fatal(err)
	}
	// The second missing item must roll back the first mutation and its command.
	destination, err := repo.CreateManagedFolder(ctx, user.ID, "", "Destination")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MoveFiles(ctx, user.ID, destination.ID, []string{first.ID, uuid.NewString()}); !errors.Is(err, catalog.ErrFileNotFound) {
		t.Fatalf("missing move item accepted: %v", err)
	}
	afterRollback, err := repo.GetUnifiedFile(ctx, user.ID, first.ID)
	if err != nil || afterRollback.FolderID != first.FolderID || afterRollback.Version != first.Version {
		t.Fatalf("partial move committed: %+v %v", afterRollback, err)
	}
	versions := map[string]int64{first.ID: first.Version, second.ID: second.Version}
	if err := repo.MoveFiles(ctx, user.ID, destination.ID, []string{first.ID, second.ID}, versions); err != nil {
		t.Fatal(err)
	}
	if err := repo.MoveFiles(ctx, user.ID, first.FolderID, []string{first.ID, second.ID}, versions); !errors.Is(err, catalog.ErrVersionConflict) {
		t.Fatalf("stale move accepted: %v", err)
	}
	first, err = repo.GetUnifiedFile(ctx, user.ID, first.ID)
	if err != nil || first.FolderID != destination.ID {
		t.Fatalf("move missing: %v", err)
	}
	second, err = repo.GetUnifiedFile(ctx, user.ID, second.ID)
	if err != nil || second.FolderID != destination.ID {
		t.Fatalf("second move missing: %v", err)
	}
	other, err := identity.NewUserRepository(db).Create(ctx, "other-actions-user", "unused-test-hash")
	if err != nil {
		t.Fatal(err)
	}
	otherFolder, err := repo.CreateManagedFolder(ctx, other.ID, "", "Private")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.MoveFiles(ctx, user.ID, otherFolder.ID, []string{first.ID}); !errors.Is(err, catalog.ErrFolderNotFound) {
		t.Fatalf("cross-account destination accepted: %v", err)
	}
	_, err = repo.ApplyActions(ctx, user.ID, catalog.ActionsRequest{OperationID: "rollback", FileIDs: []string{first.ID, uuid.NewString()}, Action: "trash"})
	if !errors.Is(err, catalog.ErrFileNotFound) {
		t.Fatalf("expected missing file, got %v", err)
	}
	unchanged, err := repo.GetUnifiedFile(ctx, user.ID, first.ID)
	if err != nil || unchanged.Status != "active" {
		t.Fatalf("partial batch committed: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM device_commands`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rolled-back command exists: %v, %d", err, count)
	}
	req := catalog.ActionsRequest{OperationID: "trash-batch", FileIDs: []string{first.ID, second.ID}, Action: "trash", ExpectedVersions: map[string]int64{first.ID: first.Version, second.ID: second.Version}}
	files, err := repo.ApplyActions(ctx, user.ID, req)
	if err != nil || len(files) != 2 {
		t.Fatalf("batch: %v", err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM device_commands WHERE device_id=$1`, device.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("origin commands: %v, %d", err, count)
	}
	replay, err := repo.ApplyActions(ctx, user.ID, req)
	if err != nil || replay[0].Version != files[0].Version {
		t.Fatalf("retry was not idempotent: %v", err)
	}
	changed := req
	changed.FileIDs = []string{second.ID, first.ID}
	if _, err := repo.ApplyActions(ctx, user.ID, changed); !errors.Is(err, catalog.ErrIdempotencyConflict) {
		t.Fatalf("changed request accepted: %v", err)
	}
	if _, err := repo.ApplyActions(ctx, user.ID, catalog.ActionsRequest{OperationID: "stale", FileIDs: []string{first.ID}, Action: "restore", ExpectedVersions: map[string]int64{first.ID: first.Version}}); !errors.Is(err, catalog.ErrVersionConflict) {
		t.Fatalf("stale version accepted: %v", err)
	}
	if _, err := repo.ApplyActions(ctx, user.ID, catalog.ActionsRequest{OperationID: "purge", FileIDs: []string{first.ID}, Action: "purge"}); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM replicas WHERE device_id=$1 AND client_file_id='local-one'`, device.ID).Scan(&state); err != nil || state == "deleted" {
		t.Fatalf("physical removal acknowledged before Agent confirmation: %s %v", state, err)
	}
}
