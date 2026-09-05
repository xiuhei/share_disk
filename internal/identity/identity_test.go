package identity

import (
	"errors"
	"testing"
	"time"
)

func TestHashPassword(t *testing.T) {
	password := "test-password-123"

	// Hash password
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	// Verify hash format
	if len(hash) == 0 {
		t.Fatal("Hash is empty")
	}

	// Verify password
	valid, err := VerifyPassword(password, hash)
	if err != nil {
		t.Fatalf("Failed to verify password: %v", err)
	}

	if !valid {
		t.Fatal("Password verification failed")
	}

	// Verify wrong password
	wrongValid, err := VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("Failed to verify wrong password: %v", err)
	}

	if wrongValid {
		t.Fatal("Wrong password should not verify")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	token1, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("Failed to generate refresh token: %v", err)
	}

	token2, err := GenerateRefreshToken()
	if err != nil {
		t.Fatalf("Failed to generate refresh token: %v", err)
	}

	if token1 == token2 {
		t.Fatal("Refresh tokens should be unique")
	}

	if len(token1) == 0 {
		t.Fatal("Refresh token is empty")
	}
}

func TestTokenManager(t *testing.T) {
	privPEM, pubPEM, err := GenerateSigningKey()
	if err != nil {
		t.Fatalf("failed to generate signing key: %v", err)
	}
	manager, err := NewTokenManager(privPEM, 15*time.Minute)
	if err != nil {
		t.Fatalf("failed to create token manager: %v", err)
	}

	userID := "test-user-id"
	deviceID := "test-device-id"
	sessionID := "test-session-id"

	token, err := manager.GenerateAccessToken(userID, deviceID, sessionID)
	if err != nil {
		t.Fatalf("Failed to generate access token: %v", err)
	}
	if token == "" {
		t.Fatal("Access token is empty")
	}

	// A verifier holding only the public key must accept the token.
	verifier, err := NewTokenVerifier(pubPEM)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}
	claims, err := verifier.ValidateAccessToken(token)
	if err != nil {
		t.Fatalf("Failed to validate access token: %v", err)
	}
	if claims.UserID != userID || claims.DeviceID != deviceID || claims.SessionID != sessionID {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	shareToken, err := manager.GenerateShareAccessToken(userID, "local-file-1", "share-1", time.Minute)
	if err != nil {
		t.Fatalf("generate share token: %v", err)
	}
	shareClaims, err := verifier.ValidateAccessToken(shareToken)
	if err != nil || shareClaims.Scope != "share_download" || shareClaims.FileID != "local-file-1" {
		t.Fatalf("unexpected share claims: claims=%+v err=%v", shareClaims, err)
	}

	// A verifier holding a different public key must reject the token.
	_, otherPubPEM, err := GenerateSigningKey()
	if err != nil {
		t.Fatalf("failed to generate second key: %v", err)
	}
	otherVerifier, err := NewTokenVerifier(otherPubPEM)
	if err != nil {
		t.Fatalf("failed to create second verifier: %v", err)
	}
	if _, err := otherVerifier.ValidateAccessToken(token); err == nil {
		t.Fatal("token signed with one key must be rejected by a different key")
	}
}

func TestValidationError(t *testing.T) {
	err := invalidInput("account is required")
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatal("expected *ValidationError")
	}
	if ve.Message != "account is required" {
		t.Fatalf("unexpected message: %s", ve.Message)
	}
}
