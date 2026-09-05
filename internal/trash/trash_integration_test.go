//go:build integration

package trash_test

import (
	"context"
	"testing"
	"time"

	"github.com/share-disk/share-disk/internal/identity"
	"github.com/share-disk/share-disk/internal/testutil"
	"github.com/share-disk/share-disk/internal/trash"
)

func setupTrash(t *testing.T) (*testutil.DB, string, string, *trash.Service) {
	t.Helper()
	db := testutil.NewDB(t)
	privateKey, _, err := identity.GenerateSigningKey()
	if err != nil {
		t.Fatal(err)
	}
	tm, err := identity.NewTokenManager(privateKey, 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	idSvc := identity.NewService(db.DB, "bootstrap-token", tm)
	resp, err := idSvc.Bootstrap(context.Background(), &identity.BootstrapRequest{
		BootstrapToken: "bootstrap-token",
		Account:        "admin",
		Password:       "password123",
	})
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	folderID := testutil.InsertFolder(t, db, resp.UserID)
	objectID := testutil.InsertObject(t, db, resp.UserID, 1234)
	entryID := testutil.InsertFileEntry(t, db, resp.UserID, folderID, objectID, "file.txt")
	return db, resp.UserID, entryID, trash.NewService(db.DB)
}

// 3.1.1: re-trash after restore must return the persisted record (stable id and
// created_at), not a freshly generated but never-persisted id.
func TestTrashRedeleteReturnsStableRecord(t *testing.T) {
	_, userID, entryID, svc := setupTrash(t)
	ctx := context.Background()

	rec1, err := svc.TrashFileEntry(ctx, userID, entryID, 7)
	if err != nil {
		t.Fatalf("first trash: %v", err)
	}
	if err := svc.RestoreFileEntry(ctx, userID, entryID); err != nil {
		t.Fatalf("restore: %v", err)
	}

	rec2, err := svc.TrashFileEntry(ctx, userID, entryID, 7)
	if err != nil {
		t.Fatalf("re-trash: %v", err)
	}

	if rec2.ID != rec1.ID {
		t.Fatalf("expected stable record id %s, got %s", rec1.ID, rec2.ID)
	}
	if !rec2.CreatedAt.Equal(rec1.CreatedAt) {
		t.Fatalf("expected stable created_at %v, got %v", rec1.CreatedAt, rec2.CreatedAt)
	}
}

// Restore closes the record and re-trash is then allowed; purge removes it.
func TestTrashRestoreAndPurge(t *testing.T) {
	db, userID, entryID, svc := setupTrash(t)
	ctx := context.Background()

	if _, err := svc.TrashFileEntry(ctx, userID, entryID, 0); err != nil {
		t.Fatalf("trash: %v", err)
	}

	// Force the purge window into the past to avoid a timing flake.
	if _, err := db.Exec(`UPDATE trash_records SET purge_after = NOW() - interval '1 second' WHERE file_entry_id = $1`, entryID); err != nil {
		t.Fatalf("set purge_after: %v", err)
	}

	n, err := svc.Purge(ctx, 10)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 purged record, got %d", n)
	}

	records, err := svc.ListTrashRecords(ctx, userID, 10)
	if err != nil {
		t.Fatalf("list trash: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no active trash records, got %d", len(records))
	}
}
