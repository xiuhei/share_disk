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

func TestShareCreateResolveLimitAndRevoke(t *testing.T) {
	db := testutil.NewDB(t)
	ctx := context.Background()
	userID, deviceID := uuid.NewString(), uuid.NewString()
	if _, err := db.ExecContext(ctx, `INSERT INTO users(id,account,password_hash) VALUES($1,'share-user','hash')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO devices(id,user_id,name,platform,public_key,peer_id) VALUES($1,$2,'Agent','linux',$3,$4)`, deviceID, userID, []byte("share-key"), "share-peer"); err != nil {
		t.Fatal(err)
	}
	repo := catalog.NewRepository(db.DB)
	file, err := repo.RegisterVerifiedFile(ctx, userID, deviceID, catalog.RegisterFileRequest{
		LocalFileID: "share-local", Name: "shared.txt", MIME: "text/plain", Size: 5,
		SHA256: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824", Endpoint: "http://10.0.0.2:9090",
	})
	if err != nil {
		t.Fatal(err)
	}
	share, err := repo.CreateShare(ctx, userID, file.ID, time.Hour, 1)
	if err != nil || share.Token == "" {
		t.Fatalf("create share: share=%+v err=%v", share, err)
	}
	shares, err := repo.ListShares(ctx, userID)
	if err != nil || len(shares) != 1 || shares[0].FileName != "shared.txt" || shares[0].Token != "" {
		t.Fatalf("list shares: shares=%+v err=%v", shares, err)
	}
	resolved, err := repo.ResolveShare(ctx, share.Token)
	if err != nil || resolved.File.ID != file.ID || resolved.File.ContentLocalFileID != "share-local" || resolved.Share.DownloadCount != 1 {
		t.Fatalf("resolve share: result=%+v err=%v", resolved, err)
	}
	if _, err := repo.ResolveShare(ctx, share.Token); !errors.Is(err, catalog.ErrShareExpired) {
		t.Fatalf("second resolve = %v, want limit error", err)
	}

	revocable, err := repo.CreateShare(ctx, userID, file.ID, time.Hour, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.RevokeShare(ctx, userID, revocable.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ResolveShare(ctx, revocable.Token); !errors.Is(err, catalog.ErrShareExpired) {
		t.Fatalf("revoked resolve = %v, want unavailable error", err)
	}
}
