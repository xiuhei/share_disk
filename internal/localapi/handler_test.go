package localapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/share-disk/share-disk/internal/database"
	"github.com/share-disk/share-disk/internal/storage"
	"github.com/share-disk/share-disk/internal/testutil"
	sharediskv1 "github.com/share-disk/share-disk/proto/sharedisk/v1"
)

func TestLinuxIPCFileLifecyclePublishesDurableOperations(t *testing.T) {
	root := t.TempDir()
	db, err := database.OpenSQLite(filepath.Join(root, "agent.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := database.NewMigrator(db, testutil.SQLiteMigrationsDir(), "sqlite").MigrateUp(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO settings(key,value) VALUES('control_user_id','user-1')`); err != nil {
		t.Fatal(err)
	}
	managed, err := storage.New(filepath.Join(root, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	objects := storage.NewSQLiteObjectStore(managed, db, 4*1024*1024)
	if err := objects.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	handler := NewLocalHandler(objects, managed, "peer-1", "test", db, "http://agent.local:9090", time.Hour)
	source := filepath.Join(root, "hello.txt")
	if err := os.WriteFile(source, []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	imported := handler.Handle(context.Background(), &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_Import{Import: &sharediskv1.ImportRequest{SourcePath: source}}})
	if imported.GetError() != nil || imported.GetImport().GetFileEntryId() == "" {
		t.Fatalf("import was not published: %+v", imported)
	}
	fileID := imported.GetImport().GetFileEntryId()
	list := handler.Handle(context.Background(), &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_ListLocalFiles{ListLocalFiles: &sharediskv1.ListLocalFilesRequest{}}})
	if len(list.GetListLocalFiles().GetFiles()) != 1 {
		t.Fatalf("unexpected active list: %+v", list)
	}
	for _, mutation := range []*sharediskv1.MutateLocalFileRequest{
		{FileId: fileID, Action: "rename", Name: "renamed.txt"},
		{FileId: fileID, Action: "trash"},
		{FileId: fileID, Action: "restore"},
		{FileId: fileID, Action: "trash"},
		{FileId: fileID, Action: "purge"},
	} {
		resp := handler.Handle(context.Background(), &sharediskv1.LocalRequest{Payload: &sharediskv1.LocalRequest_MutateLocalFile{MutateLocalFile: mutation}})
		if resp.GetError() != nil {
			t.Fatalf("%s failed: %+v", mutation.Action, resp.GetError())
		}
	}
	var pending int
	if err := db.QueryRow(`SELECT count(*) FROM outgoing_ops WHERE status='pending'`).Scan(&pending); err != nil {
		t.Fatal(err)
	}
	if pending != 6 {
		t.Fatalf("expected six FIFO operations, got %d", pending)
	}
}
