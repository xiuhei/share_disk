//go:build integration

package identity_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/share-disk/share-disk/internal/identity"
	"github.com/share-disk/share-disk/internal/testutil"
)

const (
	testBootstrapTok = "bootstrap-token"
)

func newService(t *testing.T) *identity.Service {
	t.Helper()
	db := testutil.NewDB(t)
	tm := newTokenManager(t)
	return identity.NewService(db.DB, testBootstrapTok, tm)
}

// newTokenManager returns a signing TokenManager backed by a fresh Ed25519 key.
func newTokenManager(t *testing.T) *identity.TokenManager {
	t.Helper()
	privPEM, _, err := identity.GenerateSigningKey()
	if err != nil {
		t.Fatalf("failed to generate signing key: %v", err)
	}
	tm, err := identity.NewTokenManager(privPEM, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}
	return tm
}

// P0-06: concurrent bootstrap must create exactly one administrator.
func TestBootstrapConcurrentSingleAdmin(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Bootstrap(ctx, &identity.BootstrapRequest{
				BootstrapToken: testBootstrapTok,
				Account:        "admin",
				Password:       "password123",
			})
			errs[i] = err
		}(i)
	}
	wg.Wait()

	var success, already int
	for _, err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, identity.ErrAlreadyBootstrapped):
			already++
		default:
			t.Fatalf("unexpected bootstrap error: %v", err)
		}
	}
	if success != 1 {
		t.Fatalf("expected exactly 1 successful bootstrap, got %d", success)
	}
	if already != n-1 {
		t.Fatalf("expected %d already-bootstrapped, got %d", n-1, already)
	}
}

// P0-06 (one-time state): bootstrap must stay one-time even after the only
// account is suspended, because installation is irreversible.
func TestBootstrapStaysOneTimeAfterSuspension(t *testing.T) {
	db := testutil.NewDB(t)
	tm := newTokenManager(t)
	svc := identity.NewService(db.DB, testBootstrapTok, tm)
	ctx := context.Background()

	resp, err := svc.Bootstrap(ctx, &identity.BootstrapRequest{
		BootstrapToken: testBootstrapTok,
		Account:        "admin",
		Password:       "password123",
	})
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	if _, err := db.Exec(`UPDATE users SET status = 'suspended' WHERE id = $1`, resp.UserID); err != nil {
		t.Fatalf("failed to suspend user: %v", err)
	}

	_, err = svc.Bootstrap(ctx, &identity.BootstrapRequest{
		BootstrapToken: testBootstrapTok,
		Account:        "admin2",
		Password:       "password123",
	})
	if !errors.Is(err, identity.ErrAlreadyBootstrapped) {
		t.Fatalf("expected ErrAlreadyBootstrapped after suspension, got %v", err)
	}
}

// P0-07: a replayed refresh token must be rejected and revoke the session family.
func TestRefreshReplayRevokesFamily(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()

	boot, err := svc.Bootstrap(ctx, &identity.BootstrapRequest{
		BootstrapToken: testBootstrapTok,
		Account:        "admin",
		Password:       "password123",
	})
	if err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	// Replay the same refresh token concurrently.
	type result struct {
		resp *identity.AuthResponse
		err  error
	}
	results := make(chan result, 2)
	for i := 0; i < 2; i++ {
		go func() {
			r, e := svc.Refresh(ctx, &identity.RefreshRequest{RefreshToken: boot.RefreshToken})
			results <- result{resp: r, err: e}
		}()
	}

	var winners []*identity.AuthResponse
	var invalid int
	for i := 0; i < 2; i++ {
		res := <-results
		switch {
		case res.err == nil:
			winners = append(winners, res.resp)
		case errors.Is(res.err, identity.ErrInvalidRefreshToken):
			invalid++
		default:
			t.Fatalf("unexpected refresh error: %v", res.err)
		}
	}
	if len(winners) != 1 {
		t.Fatalf("expected exactly 1 successful replay, got %d", len(winners))
	}
	if invalid != 1 {
		t.Fatalf("expected 1 invalid refresh, got %d", invalid)
	}

	// The family must be fully revoked: the winner's fresh token is unusable.
	_, err = svc.Refresh(ctx, &identity.RefreshRequest{RefreshToken: winners[0].RefreshToken})
	if !errors.Is(err, identity.ErrInvalidRefreshToken) {
		t.Fatalf("expected family revocation to invalidate the winner's token, got %v", err)
	}
}

func TestEnrollExistingAccountCreatesStableIndependentDevice(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	boot, err := svc.Bootstrap(ctx, &identity.BootstrapRequest{BootstrapToken: testBootstrapTok, Account: "admin", Password: "password123", DeviceName: "first", Platform: "android", PeerID: "first-peer"})
	if err != nil {
		t.Fatal(err)
	}
	request := &identity.EnrollDeviceRequest{Account: "admin", Password: "password123", Name: "second", Platform: "android", PeerID: "second-peer"}
	first, err := svc.EnrollDevice(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := svc.EnrollDevice(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if first.DeviceID == boot.DeviceID || retry.DeviceID != first.DeviceID {
		t.Fatalf("device identity was not independent and stable: bootstrap=%s first=%s retry=%s", boot.DeviceID, first.DeviceID, retry.DeviceID)
	}
	if first.RefreshToken == boot.RefreshToken || retry.RefreshToken == first.RefreshToken {
		t.Fatal("enrollment reused another session refresh token")
	}
	if _, err := svc.EnrollDevice(ctx, &identity.EnrollDeviceRequest{Account: "admin", Password: "wrong", Name: "third", Platform: "android", PeerID: "third-peer"}); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("wrong password returned %v", err)
	}
}

func TestDeregisteredPhysicalDeviceCanBeEnrolledAgain(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	boot, err := svc.Bootstrap(ctx, &identity.BootstrapRequest{BootstrapToken: testBootstrapTok, Account: "admin", Password: "password123", DeviceName: "phone", Platform: "android", PeerID: "phone-peer"})
	if err != nil {
		t.Fatal(err)
	}
	request := &identity.EnrollDeviceRequest{Account: "admin", Password: "password123", Name: "Ubuntu", Platform: "linux", PeerID: "stable-linux-peer"}
	first, err := svc.EnrollDevice(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.DeregisterDevice(ctx, boot.UserID, first.DeviceID); err != nil {
		t.Fatal(err)
	}
	reenrolled, err := svc.EnrollDevice(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if reenrolled.DeviceID != first.DeviceID {
		t.Fatalf("physical device identity changed after reenrollment: first=%s second=%s", first.DeviceID, reenrolled.DeviceID)
	}
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	current, err := svc.Bootstrap(ctx, &identity.BootstrapRequest{BootstrapToken: testBootstrapTok, Account: "admin", Password: "password123", DeviceName: "console", Platform: "web", PeerID: "console-peer"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.EnrollDevice(ctx, &identity.EnrollDeviceRequest{Account: "admin", Password: "password123", Name: "phone", Platform: "android", PeerID: "phone-peer"})
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.Authenticate(ctx, current.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(ctx, current.UserID, claims.SessionID, identity.ChangePasswordRequest{CurrentPassword: "password123", NewPassword: "new-password-456"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, current.AccessToken); err != nil {
		t.Fatalf("current session was revoked: %v", err)
	}
	if _, err := svc.Authenticate(ctx, other.AccessToken); err == nil {
		t.Fatal("other session remained active after password change")
	}
	if _, err := svc.EnrollDevice(ctx, &identity.EnrollDeviceRequest{Account: "admin", Password: "password123", Name: "old", Platform: "web", PeerID: "old-peer"}); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatalf("old password returned %v", err)
	}
	if _, err := svc.EnrollDevice(ctx, &identity.EnrollDeviceRequest{Account: "admin", Password: "new-password-456", Name: "new", Platform: "web", PeerID: "new-peer"}); err != nil {
		t.Fatalf("new password failed: %v", err)
	}
}
