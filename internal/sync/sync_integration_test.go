//go:build integration

package sync_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/share-disk/share-disk/internal/sync"
	"github.com/share-disk/share-disk/internal/testutil"
)

func setupSync(t *testing.T) (*testutil.DB, string, *sync.Service) {
	t.Helper()
	db := testutil.NewDB(t)
	userID := testutil.InsertUser(t, db, "admin")
	return db, userID, sync.NewService(db.DB)
}

// 3.1.2: has_more must not produce a false positive when the final page is
// exactly `limit` events.
func TestDeltaSyncHasMore(t *testing.T) {
	_, userID, svc := setupSync(t)
	ctx := context.Background()

	publish := func(n int) {
		for i := 0; i < n; i++ {
			if _, err := svc.PublishEvent(ctx, userID, "file.created", uuid.NewString(), nil, nil); err != nil {
				t.Fatalf("publish event: %v", err)
			}
		}
	}

	publish(3)
	resp, err := svc.DeltaSync(ctx, userID, &sync.DeltaSyncRequest{SinceSeq: 0, Limit: 3})
	if err != nil {
		t.Fatalf("delta sync: %v", err)
	}
	if resp.HasMore {
		t.Fatal("expected has_more=false when the final page is exactly limit")
	}
	if len(resp.Events) != 3 || resp.LatestSeq != 3 {
		t.Fatalf("unexpected page: events=%d latest=%d", len(resp.Events), resp.LatestSeq)
	}

	publish(2)
	resp, err = svc.DeltaSync(ctx, userID, &sync.DeltaSyncRequest{SinceSeq: 0, Limit: 3})
	if err != nil {
		t.Fatalf("delta sync: %v", err)
	}
	if !resp.HasMore {
		t.Fatal("expected has_more=true when more events follow the first page")
	}
	if len(resp.Events) != 3 {
		t.Fatalf("expected 3 events on first page, got %d", len(resp.Events))
	}
}

// DeltaSync with a cursor past the end must report no events and no more.
func TestDeltaSyncBeyondLatest(t *testing.T) {
	_, userID, svc := setupSync(t)
	ctx := context.Background()

	if _, err := svc.PublishEvent(ctx, userID, "file.created", uuid.NewString(), nil, nil); err != nil {
		t.Fatalf("publish event: %v", err)
	}

	resp, err := svc.DeltaSync(ctx, userID, &sync.DeltaSyncRequest{SinceSeq: 1, Limit: 3})
	if err != nil {
		t.Fatalf("delta sync: %v", err)
	}
	if resp.HasMore || len(resp.Events) != 0 {
		t.Fatalf("expected empty page, got events=%d has_more=%v", len(resp.Events), resp.HasMore)
	}
}
