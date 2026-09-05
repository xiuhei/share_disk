//go:build integration

package transfer_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/share-disk/share-disk/internal/identity"
	"github.com/share-disk/share-disk/internal/testutil"
	"github.com/share-disk/share-disk/internal/transfer"
)

func setupTransfer(t *testing.T) (*testutil.DB, string, string, string, *transfer.Service) {
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
	return db, resp.UserID, resp.DeviceID, objectID, transfer.NewService(db.DB)
}

// P0-08/P0-09: lease acquisition and the full state machine must be enforced.
func TestTransferLeaseAndStateMachine(t *testing.T) {
	_, userID, deviceID, objectID, svc := setupTransfer(t)
	ctx := context.Background()

	task, err := svc.CreateTask(ctx, userID, objectID, deviceID, "pull", 0)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if task.State != "queued" {
		t.Fatalf("expected queued, got %s", task.State)
	}

	leased, err := svc.AcquireLease(ctx, userID, task.ID, deviceID, time.Minute)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if leased.State != "assigned" {
		t.Fatalf("expected assigned after lease, got %s", leased.State)
	}

	steps := []string{"discovering", "connecting", "transferring", "verifying", "completed"}
	version := leased.Version
	for _, s := range steps {
		if err := svc.UpdateState(ctx, userID, task.ID, deviceID, version, s); err != nil {
			t.Fatalf("transition to %s: %v", s, err)
		}
		version++
	}

	// completed is terminal.
	if err := svc.UpdateState(ctx, userID, task.ID, deviceID, version, "canceled"); !errors.Is(err, transfer.ErrInvalidTransition) {
		t.Fatalf("expected invalid transition from completed, got %v", err)
	}
}

// P0-08: a task cannot jump straight to a terminal state.
func TestTransferCannotSkipToCompleted(t *testing.T) {
	_, userID, deviceID, objectID, svc := setupTransfer(t)
	ctx := context.Background()

	task, err := svc.CreateTask(ctx, userID, objectID, deviceID, "pull", 0)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	leased, err := svc.AcquireLease(ctx, userID, task.ID, deviceID, time.Minute)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	if err := svc.UpdateState(ctx, userID, task.ID, deviceID, leased.Version, "completed"); !errors.Is(err, transfer.ErrInvalidTransition) {
		t.Fatalf("expected invalid transition, got %v", err)
	}
}

// P0-09: only the target device may claim the lease.
func TestTransferLeaseRequiresTargetDevice(t *testing.T) {
	db, userID, _, objectID, svc := setupTransfer(t)
	ctx := context.Background()

	target := testutil.InsertDevice(t, db, userID)
	other := testutil.InsertDevice(t, db, userID)

	task, err := svc.CreateTask(ctx, userID, objectID, target, "pull", 0)
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := svc.AcquireLease(ctx, userID, task.ID, other, time.Minute); !errors.Is(err, transfer.ErrNotClaimable) {
		t.Fatalf("expected ErrNotClaimable for non-target device, got %v", err)
	}
}

// Cross-account: a task cannot reference another account's object.
func TestTransferCrossAccountObject(t *testing.T) {
	db, userID, deviceID, _, svc := setupTransfer(t)
	ctx := context.Background()

	other := testutil.InsertUser(t, db, "other")
	otherObject := testutil.InsertObject(t, db, other, 1234)

	if _, err := svc.CreateTask(ctx, userID, otherObject, deviceID, "pull", 0); !errors.Is(err, transfer.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for cross-account object, got %v", err)
	}
}
