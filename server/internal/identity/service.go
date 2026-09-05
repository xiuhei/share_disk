package identity

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Service handles authentication business logic
type Service struct {
	db             *sql.DB
	userRepo       *UserRepository
	sessionRepo    *SessionRepository
	deviceRepo     *DeviceRepository
	tokenManager   *TokenManager
	bootstrapToken string
}

// NewService creates a new identity Service
func NewService(db *sql.DB, bootstrapToken string, tokenManager *TokenManager) *Service {
	return &Service{
		db:             db,
		userRepo:       NewUserRepository(db),
		sessionRepo:    NewSessionRepository(db),
		deviceRepo:     NewDeviceRepository(db),
		tokenManager:   tokenManager,
		bootstrapToken: bootstrapToken,
	}
}

// BootstrapRequest represents a bootstrap request
type BootstrapRequest struct {
	BootstrapToken string `json:"bootstrap_token"`
	Account        string `json:"account"`
	Password       string `json:"password"`

	// Optional device identity for the first device created during bootstrap.
	// When absent, placeholder values are generated so the account has a usable
	// device to log in with until real device registration (M3) exists.
	DeviceName string `json:"device_name"`
	Platform   string `json:"platform"`
	PublicKey  []byte `json:"public_key"`
	PeerID     string `json:"peer_id"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
	DeviceID string `json:"device_id"`
}

// RefreshRequest represents a refresh request
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RegisterDeviceRequest creates or reuses a stable identity for an
// authenticated device. PeerID is the device-generated retry key.
type RegisterDeviceRequest struct {
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	PublicKey []byte `json:"public_key,omitempty"`
	PeerID    string `json:"peer_id"`
}

// EnrollDeviceRequest authenticates an existing account while creating or
// resuming this client's stable device identity.
type EnrollDeviceRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	PeerID   string `json:"peer_id"`
}

// AuthResponse represents an authentication response
type AuthResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	UserID       string `json:"user_id"`
	DeviceID     string `json:"device_id"`
}

// AccountSummary is the safe account view exposed to authenticated clients.
// Password hashes and other authentication material are never serialized.
type AccountSummary struct {
	ID        string    `json:"id"`
	Account   string    `json:"account"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ChangePasswordRequest changes the current account password after verifying
// the existing password.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Service) GetAccount(ctx context.Context, userID string) (*AccountSummary, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}
	return &AccountSummary{ID: user.ID, Account: user.Account, Status: user.Status, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt}, nil
}

// ChangePassword updates the Argon2id password hash and revokes every other
// session. The authenticated session remains usable so the browser can confirm
// success; other clients must log in again with the new password.
func (s *Service) ChangePassword(ctx context.Context, userID, currentSessionID string, req ChangePasswordRequest) error {
	if req.CurrentPassword == "" || len(req.NewPassword) < 8 {
		return invalidInput("current_password is required and new_password must be at least 8 characters")
	}
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	if user == nil {
		return ErrInvalidCredentials
	}
	valid, err := VerifyPassword(req.CurrentPassword, user.PasswordHash)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidCredentials
	}
	passwordHash, err := HashPassword(req.NewPassword)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE users SET password_hash=$1,updated_at=NOW() WHERE id=$2 AND status='active'`, passwordHash, userID)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows != 1 {
		return ErrInvalidCredentials
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,NOW()),updated_at=NOW() WHERE user_id=$1 AND id<>$2`, userID, currentSessionID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeviceSummary is the account-visible scheduling view. It intentionally
// excludes public keys and stable peer identifiers.
type DeviceSummary struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Platform   string     `json:"platform"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
}

// ListDevices returns active devices owned by one authenticated account.
func (s *Service) ListDevices(ctx context.Context, userID string) ([]DeviceSummary, error) {
	devices, err := s.deviceRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	result := make([]DeviceSummary, 0, len(devices))
	for _, device := range devices {
		result = append(result, DeviceSummary{ID: device.ID, Name: device.Name, Platform: device.Platform, Status: device.Status, LastSeenAt: device.LastSeenAt})
	}
	return result, nil
}

// DeregisterDevice disables an account device and all of its sessions.
func (s *Service) DeregisterDevice(ctx context.Context, userID, deviceID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE devices SET status='deregistered',version=version+1,updated_at=NOW() WHERE id=$1 AND user_id=$2 AND status='active'`, deviceID, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrDeviceNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked_at=COALESCE(revoked_at,NOW()),updated_at=NOW() WHERE device_id=$1 AND user_id=$2`, deviceID, userID); err != nil {
		return err
	}
	return tx.Commit()
}

const refreshTokenTTL = 7 * 24 * time.Hour

// bootstrapAdvisoryLockKey is a fixed key for the transaction-scoped advisory
// lock that serializes the one-time bootstrap operation.
const bootstrapAdvisoryLockKey = 0x5348_4152_4544_4943 // "SHAREDIC"

// Bootstrap creates the first account, first device, and first session in a
// single transaction. Any failure rolls back all writes so a fresh instance is
// never left in a half-initialized state, and retries remain possible.
func (s *Service) Bootstrap(ctx context.Context, req *BootstrapRequest) (*AuthResponse, error) {
	// Constant-time comparison to avoid leaking the token via timing.
	if !constantTimeEqual(req.BootstrapToken, s.bootstrapToken) {
		return nil, ErrInvalidBootstrapToken
	}

	if req.Account == "" {
		return nil, invalidInput("account is required")
	}
	if len(req.Password) < 8 {
		return nil, invalidInput("password must be at least 8 characters")
	}

	// Hash password before opening the transaction (it is expensive and does
	// not touch the database).
	passwordHash, err := HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	publicKey := req.PublicKey
	if len(publicKey) == 0 {
		publicKey, err = randomBytes(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate device key placeholder: %w", err)
		}
	}

	peerID := req.PeerID
	if peerID == "" {
		peerID, err = randomHex(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate peer id placeholder: %w", err)
		}
	}

	deviceName := req.DeviceName
	if deviceName == "" {
		deviceName = "primary"
	}
	platform := req.Platform
	if platform == "" {
		platform = "server"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Serialize bootstrap across concurrent callers using a transaction-scoped
	// advisory lock so exactly one request can observe an empty users table.
	// The lock is released automatically on commit/rollback.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, bootstrapAdvisoryLockKey); err != nil {
		return nil, fmt.Errorf("failed to acquire bootstrap lock: %w", err)
	}

	hasUsers, err := s.userRepo.HasAnyUsersTx(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to check users: %w", err)
	}
	if hasUsers {
		return nil, ErrAlreadyBootstrapped
	}

	user, err := s.userRepo.CreateTx(ctx, tx, req.Account, passwordHash)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	device, err := s.deviceRepo.CreateTx(ctx, tx, user.ID, deviceName, platform, publicKey, peerID)
	if err != nil {
		return nil, fmt.Errorf("failed to create device: %w", err)
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	session, err := s.sessionRepo.CreateTx(ctx, tx, user.ID, device.ID, refreshToken, refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	accessToken, err := s.tokenManager.GenerateAccessToken(user.ID, device.ID, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.tokenManager.AccessTokenTTL(),
		UserID:       user.ID,
		DeviceID:     device.ID,
	}, nil
}

// Login authenticates a user and binds the session to an existing device.
func (s *Service) Login(ctx context.Context, req *LoginRequest) (*AuthResponse, error) {
	// Validate input
	if req.Account == "" {
		return nil, invalidInput("account is required")
	}
	if req.Password == "" {
		return nil, invalidInput("password is required")
	}
	if req.DeviceID == "" {
		return nil, invalidInput("device_id is required")
	}

	user, err := s.userRepo.GetByAccount(ctx, req.Account)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	valid, err := VerifyPassword(req.Password, user.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("failed to verify password: %w", err)
	}
	if !valid {
		return nil, ErrInvalidCredentials
	}

	// The device must exist, be active, and belong to this account.
	device, err := s.deviceRepo.GetByID(ctx, req.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	if device == nil || device.UserID != user.ID {
		return nil, ErrDeviceNotFound
	}

	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	session, err := s.sessionRepo.Create(ctx, user.ID, device.ID, refreshToken, refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	accessToken, err := s.tokenManager.GenerateAccessToken(user.ID, device.ID, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.tokenManager.AccessTokenTTL(),
		UserID:       user.ID,
		DeviceID:     device.ID,
	}, nil
}

// EnrollDevice allows a new client to join an existing account without an
// already-known server device UUID. Credential failures stay indistinguishable
// from an unknown account, matching Login.
func (s *Service) EnrollDevice(ctx context.Context, req *EnrollDeviceRequest) (*AuthResponse, error) {
	if strings.TrimSpace(req.Account) == "" || req.Password == "" {
		return nil, invalidInput("account and password are required")
	}
	user, err := s.userRepo.GetByAccount(ctx, req.Account)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}
	valid, err := VerifyPassword(req.Password, user.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("failed to verify password: %w", err)
	}
	if !valid {
		return nil, ErrInvalidCredentials
	}
	return s.RegisterDevice(ctx, user.ID, &RegisterDeviceRequest{Name: req.Name, Platform: req.Platform, PeerID: req.PeerID})
}

// RegisterDevice provisions a device-owned session without sharing the
// caller's refresh token. A same-account retry reuses the device.
func (s *Service) RegisterDevice(ctx context.Context, userID string, req *RegisterDeviceRequest) (*AuthResponse, error) {
	name := strings.TrimSpace(req.Name)
	platform := strings.TrimSpace(req.Platform)
	peerID := strings.TrimSpace(req.PeerID)
	if name == "" || len(name) > 128 {
		return nil, invalidInput("name must contain 1 to 128 bytes")
	}
	if platform == "" || len(platform) > 32 {
		return nil, invalidInput("platform must contain 1 to 32 bytes")
	}
	if peerID == "" || len(peerID) > 255 {
		return nil, invalidInput("peer_id must contain 1 to 255 bytes")
	}
	publicKey := req.PublicKey
	var err error
	if len(publicKey) == 0 {
		publicKey, err = randomBytes(32)
		if err != nil {
			return nil, fmt.Errorf("failed to generate device key placeholder: %w", err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin device registration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, peerID); err != nil {
		return nil, fmt.Errorf("failed to lock device peer id: %w", err)
	}
	device, err := s.deviceRepo.GetByPeerIDTx(ctx, tx, peerID)
	if err != nil {
		return nil, err
	}
	if device != nil && device.UserID != userID {
		return nil, ErrDeviceConflict
	}
	if device == nil {
		device, err = s.deviceRepo.CreateTx(ctx, tx, userID, name, platform, publicKey, peerID)
		if err != nil {
			return nil, fmt.Errorf("failed to create device: %w", err)
		}
	} else if device.Status != "active" {
		if err := s.deviceRepo.ReactivateTx(ctx, tx, device, name, platform, publicKey); err != nil {
			return nil, err
		}
	}
	refreshToken, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}
	session, err := s.sessionRepo.CreateTx(ctx, tx, userID, device.ID, refreshToken, refreshTokenTTL)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}
	accessToken, err := s.tokenManager.GenerateAccessToken(userID, device.ID, session.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit device registration: %w", err)
	}
	return &AuthResponse{AccessToken: accessToken, RefreshToken: refreshToken, TokenType: "Bearer", ExpiresIn: s.tokenManager.AccessTokenTTL(), UserID: userID, DeviceID: device.ID}, nil
}

// Refresh rotates a refresh token. A replayed (already used or revoked) token
// revokes the entire session family to contain theft. Lookup, replay detection,
// and rotation happen in a single transaction so concurrent replays serialize.
func (s *Service) Refresh(ctx context.Context, req *RefreshRequest) (*AuthResponse, error) {
	if req.RefreshToken == "" {
		return nil, invalidInput("refresh_token is required")
	}

	newRefreshToken, err := GenerateRefreshToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	newSession, err := s.sessionRepo.RotateRefreshAtomically(ctx, req.RefreshToken, newRefreshToken, refreshTokenTTL)
	if err != nil {
		return nil, err
	}

	accessToken, err := s.tokenManager.GenerateAccessToken(newSession.UserID, newSession.DeviceID, newSession.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate access token: %w", err)
	}

	return &AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.tokenManager.AccessTokenTTL(),
		UserID:       newSession.UserID,
		DeviceID:     newSession.DeviceID,
	}, nil
}

// Logout revokes the session identified by sessionID after verifying it
// belongs to the given user.
func (s *Service) Logout(ctx context.Context, userID, sessionID string) error {
	if sessionID == "" {
		return invalidInput("session_id is required")
	}

	session, err := s.sessionRepo.GetByID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil || session.UserID != userID {
		return ErrSessionNotFound
	}

	return s.sessionRepo.Revoke(ctx, sessionID)
}

// Heartbeat records liveness only for the authenticated device/account pair.
func (s *Service) Heartbeat(ctx context.Context, userID, deviceID string) error {
	device, err := s.deviceRepo.GetByID(ctx, deviceID)
	if err != nil {
		return err
	}
	if device == nil || device.UserID != userID {
		return ErrDeviceNotFound
	}
	return s.deviceRepo.UpdateLastSeen(ctx, deviceID)
}

// ValidateToken validates an access token
func (s *Service) ValidateToken(ctx context.Context, tokenString string) (*TokenClaims, error) {
	return s.tokenManager.ValidateAccessToken(tokenString)
}

// Authenticate validates an access token and re-checks server-side state: the
// session must not be revoked or expired, and the device and user must still be
// active and consistent with the token claims.
func (s *Service) Authenticate(ctx context.Context, tokenString string) (*TokenClaims, error) {
	claims, err := s.tokenManager.ValidateAccessToken(tokenString)
	if err != nil {
		return nil, err
	}

	session, err := s.sessionRepo.GetByID(ctx, claims.SessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}
	if session == nil || session.RevokedAt != nil || time.Now().After(session.ExpiresAt) {
		return nil, ErrSessionNotActive
	}
	if session.UserID != claims.UserID || session.DeviceID != claims.DeviceID {
		return nil, ErrSessionNotActive
	}

	device, err := s.deviceRepo.GetByID(ctx, claims.DeviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	if device == nil || device.UserID != claims.UserID {
		return nil, ErrDeviceNotFound
	}

	user, err := s.userRepo.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return nil, ErrInvalidCredentials
	}

	return claims, nil
}

// ValidationError indicates a malformed or invalid request input (missing
// field, short password, etc.). Handlers map it to HTTP 400 rather than 500.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

// invalidInput returns a ValidationError for a rejected input value.
func invalidInput(format string, args ...interface{}) error {
	return &ValidationError{Message: fmt.Sprintf(format, args...)}
}

var (
	// ErrInvalidBootstrapToken is returned when the bootstrap token is wrong.
	ErrInvalidBootstrapToken = fmt.Errorf("invalid bootstrap token")
	// ErrAlreadyBootstrapped is returned when bootstrap is attempted twice.
	ErrAlreadyBootstrapped = fmt.Errorf("bootstrap already completed")
	// ErrInvalidCredentials is returned for unknown accounts or wrong passwords.
	ErrInvalidCredentials = fmt.Errorf("invalid credentials")
	// ErrInvalidRefreshToken is returned for unknown, expired, or replayed refresh tokens.
	ErrInvalidRefreshToken = fmt.Errorf("invalid refresh token")
	// ErrSessionNotFound is returned when a session does not exist or belongs to another user.
	ErrSessionNotFound = fmt.Errorf("session not found")
	// ErrSessionNotActive is returned when a session is revoked or expired.
	ErrSessionNotActive = fmt.Errorf("session is not active")
	// ErrDeviceNotFound is returned when a device does not exist or belongs to another account.
	ErrDeviceNotFound = fmt.Errorf("device not found")
	// ErrDeviceConflict prevents a physical peer identity from crossing accounts.
	ErrDeviceConflict = fmt.Errorf("device peer id belongs to another account")
)

func constantTimeEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func randomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

func randomHex(n int) (string, error) {
	b, err := randomBytes(n)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
