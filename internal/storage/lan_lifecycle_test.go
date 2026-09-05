package storage

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestLANFileLifecyclePreservesSharedObjectUntilLastPurge(t *testing.T) {
	store, _ := newTestSQLiteObjectStore(t)
	info, err := store.Import(context.Background(), writeSource(t, []byte("shared content")))
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.store.CreateLANFile("file-1", "user-1", info.Hash.String(), "one.txt", "one.txt", "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.CreateLANFile("file-2", "user-1", info.Hash.String(), "two.txt", "two.txt", "text/plain"); err != nil {
		t.Fatal(err)
	}

	renamed, err := store.store.RenameLANFile(first.ID, "user-1", "renamed.txt", "renamed.txt")
	if err != nil {
		t.Fatal(err)
	}
	if renamed.ObjectID != first.ObjectID || renamed.Name != "renamed.txt" {
		t.Fatalf("rename changed object identity: %+v", renamed)
	}
	if _, err := store.store.RenameLANFile("file-2", "user-1", "renamed.txt", "renamed.txt"); !errors.Is(err, ErrLANNameConflict) {
		t.Fatalf("expected name conflict, got %v", err)
	}

	trashed, err := store.store.TrashLANFile(first.ID, "user-1", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if trashed.Status != "trashed" || trashed.DeletedAt == nil || trashed.PurgeAfter == nil {
		t.Fatalf("unexpected trash record: %+v", trashed)
	}
	active, err := store.store.ListLANFiles("user-1")
	if err != nil || len(active) != 1 || active[0].ID != "file-2" {
		t.Fatalf("unexpected active files: %+v, err=%v", active, err)
	}

	if err := store.PurgeLANFile(context.Background(), first.ID, "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Path); err != nil {
		t.Fatalf("shared object was removed too early: %v", err)
	}
	if _, err := store.store.TrashLANFile("file-2", "user-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := store.PurgeLANFile(context.Background(), "file-2", "user-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Fatalf("last-reference purge left object bytes: %v", err)
	}
}

func TestRestoreLANFileRejectsActiveNameConflict(t *testing.T) {
	store, _ := newTestSQLiteObjectStore(t)
	info, err := store.Import(context.Background(), writeSource(t, []byte("content")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.CreateLANFile("old", "user-1", info.Hash.String(), "same.txt", "same.txt", "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.TrashLANFile("old", "user-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.CreateLANFile("new", "user-1", info.Hash.String(), "same.txt", "same.txt", "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.RestoreLANFile("old", "user-1"); !errors.Is(err, ErrLANNameConflict) {
		t.Fatalf("expected restore conflict, got %v", err)
	}
	trash, err := store.store.ListLANTrash("user-1")
	if err != nil || len(trash) != 1 || trash[0].ID != "old" {
		t.Fatalf("conflicting restore lost trash record: %+v, err=%v", trash, err)
	}
}

func TestCoordinatedLifecycleCommitsOutboxWithLocalState(t *testing.T) {
	store, _ := newTestSQLiteObjectStore(t)
	info, err := store.Import(context.Background(), writeSource(t, []byte("coordinated")))
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.store.CreateLANFileCoordinated(context.Background(), "coordinated-file", "user-1", info.Hash.String(), "one.txt", "one.txt", "text/plain", "http://agent.local:9090")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.RenameLANFileCoordinated(context.Background(), file.ID, "user-1", "two.txt", "two.txt", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.TrashLANFileCoordinated(context.Background(), file.ID, "user-1", time.Hour, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.RestoreLANFileCoordinated(context.Background(), file.ID, "user-1", true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.TrashLANFileCoordinated(context.Background(), file.ID, "user-1", time.Hour, true); err != nil {
		t.Fatal(err)
	}
	if err := store.PurgeLANFileCoordinated(context.Background(), file.ID, "user-1"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.store.db.QueryRow(`SELECT count(*) FROM outgoing_ops WHERE status='pending'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 6 {
		t.Fatalf("expected register and five lifecycle operations, got %d", count)
	}
}

func TestPurgeExpiredLANFiles(t *testing.T) {
	store, _ := newTestSQLiteObjectStore(t)
	info, err := store.Import(context.Background(), writeSource(t, []byte("expired")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.CreateLANFile("expired-file", "user-1", info.Hash.String(), "expired.txt", "expired.txt", "text/plain"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.TrashLANFile("expired-file", "user-1", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.db.Exec(`UPDATE lan_files SET purge_after = ? WHERE id = ?`, time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano), "expired-file"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.store.BeginPurgeLANFile("expired-file", "user-1"); err != nil {
		t.Fatal(err)
	}
	if err := store.store.UpdateObjectStatus(info.Hash, ObjectStatusDeleting); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(info.Path); err != nil {
		t.Fatal(err)
	}
	if err := store.PurgeExpiredLANFiles(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Fatalf("expired trash left object bytes: %v", err)
	}
}

func TestLANFileLifecycleIsAccountScoped(t *testing.T) {
	store, _ := newTestSQLiteObjectStore(t)
	info, err := store.Import(context.Background(), writeSource(t, []byte("private")))
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.store.CreateLANFile("private-file", "owner", info.Hash.String(), "private.txt", "private.txt", "text/plain")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.store.RenameLANFile(file.ID, "other", "stolen.txt", "stolen.txt"); !errors.Is(err, ErrLANFileNotFound) {
		t.Fatalf("cross-account rename = %v", err)
	}
	if _, err := store.store.TrashLANFile(file.ID, "other", time.Hour); !errors.Is(err, ErrLANFileNotFound) {
		t.Fatalf("cross-account trash = %v", err)
	}
	if err := store.PurgeLANFile(context.Background(), file.ID, "other"); !errors.Is(err, ErrLANFileNotFound) {
		t.Fatalf("cross-account purge = %v", err)
	}
	active, err := store.store.GetLANFile(file.ID, "owner")
	if err != nil || active == nil || active.Name != "private.txt" {
		t.Fatalf("owner file changed after rejected operations: file=%+v err=%v", active, err)
	}

	if _, err := store.store.TrashLANFile(file.ID, "owner", time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := store.store.RestoreLANFile(file.ID, "other"); !errors.Is(err, ErrLANFileNotFound) {
		t.Fatalf("cross-account restore = %v", err)
	}
	trash, err := store.store.GetLANTrashFile(file.ID, "owner")
	if err != nil || trash == nil || trash.Status != "trashed" {
		t.Fatalf("owner trash changed after rejected restore: file=%+v err=%v", trash, err)
	}
}
