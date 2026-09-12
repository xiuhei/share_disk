package controlapi

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/share-disk/share-disk/server/internal/catalog"
	"github.com/share-disk/share-disk/server/internal/identity"
)

func TestRetentionAndDeviceCommandLeases(t *testing.T) {
	db := browserTestDB(t)
	ctx := context.Background()
	user, err := identity.NewUserRepository(db).Create(ctx, "retention-owner", "unused-hash")
	if err != nil {
		t.Fatal(err)
	}
	device, err := identity.NewDeviceRepository(db).Create(ctx, user.ID, "Storage", "ubuntu", []byte("key"), "peer")
	if err != nil {
		t.Fatal(err)
	}
	repo := catalog.NewRepository(db)
	file, err := repo.RegisterVerifiedFile(ctx, user.ID, device.ID, catalog.RegisterFileRequest{LocalFileID: "retained", Name: "retained.txt", Size: 4, SHA256: strings.Repeat("a", 64), Endpoint: "http://127.0.0.1:9080"})
	if err != nil {
		t.Fatal(err)
	}
	// An Agent has already trashed its local file; no origin command is needed.
	_, err = repo.ApplyLifecycleReport(ctx, user.ID, device.ID, catalog.LifecycleReport{OperationID: "trash", LocalFileID: "retained", Action: "trash"})
	if err != nil {
		t.Fatal(err)
	}
	if count, err := repo.PurgeExpiredTrash(ctx, 100); err != nil || count != 0 {
		t.Fatalf("retention deleted a fresh entry: %d %v", count, err)
	}
	if _, err := db.Exec(`UPDATE trash_records SET purge_after=NOW()-INTERVAL '1 second' WHERE file_entry_id=$1`, file.ID); err != nil {
		t.Fatal(err)
	}
	// Two worker instances scanning the same candidate must commit exactly once.
	var wg sync.WaitGroup
	counts := make(chan int, 2)
	errorsCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() { count, err := repo.PurgeExpiredTrash(ctx, 100); counts <- count; errorsCh <- err })
	}
	wg.Wait()
	if total := <-counts + <-counts; total != 1 {
		t.Fatalf("duplicate retention: %d", total)
	}
	for i := 0; i < 2; i++ {
		if err := <-errorsCh; err != nil {
			t.Fatal(err)
		}
	}
	updated, err := repo.GetUnifiedFile(ctx, user.ID, file.ID)
	if err != nil || updated.Status != "purged" {
		t.Fatalf("logical purge missing: %v", err)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM replicas WHERE device_id=$1 AND client_file_id='retained'`, device.ID).Scan(&state); err != nil || state == "deleted" {
		t.Fatalf("physical purge acknowledged prematurely: %s %v", state, err)
	}
	first, err := repo.ClaimDeviceCommand(ctx, user.ID, device.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishDeviceCommand(ctx, user.ID, device.ID, first.ID, first.Attempt, false, "temporary failure"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ClaimDeviceCommand(ctx, user.ID, device.ID, time.Minute); !errors.Is(err, catalog.ErrCommandNotFound) {
		t.Fatalf("retry bypassed backoff: %v", err)
	}
	if _, err := db.Exec(`UPDATE device_commands SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	second, err := repo.ClaimDeviceCommand(ctx, user.ID, device.ID, time.Minute)
	if err != nil || second.Attempt != first.Attempt+1 {
		t.Fatalf("retry claim: %+v %v", second, err)
	}
	if err := repo.FinishDeviceCommand(ctx, user.ID, device.ID, first.ID, first.Attempt, true, ""); !errors.Is(err, catalog.ErrCommandLeaseConflict) {
		t.Fatalf("stale executor acknowledged purge: %v", err)
	}
	if _, err := db.Exec(`UPDATE device_commands SET lease_until=NOW()-INTERVAL '1 second' WHERE id=$1`, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.FinishDeviceCommand(ctx, user.ID, device.ID, second.ID, second.Attempt, true, ""); !errors.Is(err, catalog.ErrCommandLeaseConflict) {
		t.Fatalf("expired executor acknowledged purge: %v", err)
	}
	third, err := repo.ClaimDeviceCommand(ctx, user.ID, device.ID, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := repo.FinishDeviceCommand(ctx, user.ID, device.ID, third.ID, third.Attempt, true, ""); err != nil {
			t.Fatalf("completion retry: %v", err)
		}
	}
	if err := db.QueryRow(`SELECT state FROM replicas WHERE device_id=$1 AND client_file_id='retained'`, device.ID).Scan(&state); err != nil || state != "deleted" {
		t.Fatalf("physical purge acknowledgment missing: %s %v", state, err)
	}
}
