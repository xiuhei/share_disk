//go:build integration

package replica_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/share-disk/share-disk/internal/identity"
	"github.com/share-disk/share-disk/internal/replica"
	"github.com/share-disk/share-disk/internal/testutil"
)

func setupReplica(t *testing.T) (*testutil.DB, string, string, string, *replica.Service) {
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
	objectID := testutil.InsertObject(t, db, resp.UserID, 1234)
	return db, resp.UserID, resp.DeviceID, objectID, replica.NewService(db.DB)
}

// P0-10: a replica can only be verified by its owner with a matching size.
func TestReplicaUpdateVerifiedRequiresOwnerAndSize(t *testing.T) {
	db, userID, ownerDevice, objectID, svc := setupReplica(t)
	ctx := context.Background()

	rep, err := svc.Create(ctx, userID, objectID, ownerDevice)
	if err != nil {
		t.Fatalf("create replica: %v", err)
	}

	// Owner with matching size succeeds.
	if err := svc.UpdateVerified(ctx, userID, ownerDevice, rep.ID, 1234, rep.Version); err != nil {
		t.Fatalf("update verified: %v", err)
	}
	got, err := svc.Get(ctx, userID, rep.ID)
	if err != nil {
		t.Fatalf("get replica: %v", err)
	}
	if got.State != "ready" {
		t.Fatalf("expected ready, got %s", got.State)
	}

	// A different device cannot report verification.
	otherDevice := testutil.InsertDevice(t, db, userID)
	rep2, err := svc.Create(ctx, userID, objectID, otherDevice)
	if err != nil {
		t.Fatalf("create replica 2: %v", err)
	}
	if err := svc.UpdateVerified(ctx, userID, ownerDevice, rep2.ID, 1234, rep2.Version); !errors.Is(err, replica.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for non-owner, got %v", err)
	}

	// A size mismatch is rejected.
	dev3 := testutil.InsertDevice(t, db, userID)
	rep3, err := svc.Create(ctx, userID, objectID, dev3)
	if err != nil {
		t.Fatalf("create replica 3: %v", err)
	}
	if err := svc.UpdateVerified(ctx, userID, dev3, rep3.ID, 9999, rep3.Version); err == nil {
		t.Fatal("expected size mismatch error")
	}
}

// Cross-account: a replica cannot reference another account's object.
func TestReplicaCrossAccountObject(t *testing.T) {
	db, userID, deviceID, _, svc := setupReplica(t)
	ctx := context.Background()

	other := testutil.InsertUser(t, db, "other")
	otherObject := testutil.InsertObject(t, db, other, 1234)

	if _, err := svc.Create(ctx, userID, otherObject, deviceID); !errors.Is(err, replica.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-account object, got %v", err)
	}
}
