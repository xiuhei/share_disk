package identity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Session represents a user session
type Session struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	DeviceID    string     `json:"device_id"`
	FamilyID    string     `json:"family_id"`
	RefreshHash []byte     `json:"-"`
	ExpiresAt   time.Time  `json:"expires_at"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	RotatedFrom *string    `json:"rotated_from,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// SessionRepository handles session database operations
type SessionRepository struct {
	db *sql.DB
}

// NewSessionRepository creates a new SessionRepository
func NewSessionRepository(db *sql.DB) *SessionRepository {
	return &SessionRepository{db: db}
}

// Create creates a new session
func (r *SessionRepository) Create(ctx context.Context, userID, deviceID, refreshToken string, ttl time.Duration) (*Session, error) {
	session := &Session{
		ID:          uuid.New().String(),
		UserID:      userID,
		DeviceID:    deviceID,
		FamilyID:    uuid.New().String(),
		RefreshHash: hashRefreshToken(refreshToken),
		ExpiresAt:   time.Now().Add(ttl),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	query := `
		INSERT INTO sessions (id, user_id, device_id, family_id, refresh_hash, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := r.db.ExecContext(ctx, query,
		session.ID, session.UserID, session.DeviceID, session.FamilyID,
		session.RefreshHash, session.ExpiresAt, session.CreatedAt, session.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// CreateTx creates a new session within an existing transaction.
func (r *SessionRepository) CreateTx(ctx context.Context, tx *sql.Tx, userID, deviceID, refreshToken string, ttl time.Duration) (*Session, error) {
	session := &Session{
		ID:          uuid.New().String(),
		UserID:      userID,
		DeviceID:    deviceID,
		FamilyID:    uuid.New().String(),
		RefreshHash: hashRefreshToken(refreshToken),
		ExpiresAt:   time.Now().Add(ttl),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	query := `
		INSERT INTO sessions (id, user_id, device_id, family_id, refresh_hash, expires_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := tx.ExecContext(ctx, query,
		session.ID, session.UserID, session.DeviceID, session.FamilyID,
		session.RefreshHash, session.ExpiresAt, session.CreatedAt, session.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// GetByRefreshToken retrieves a session by refresh token
func (r *SessionRepository) GetByRefreshToken(ctx context.Context, refreshToken string) (*Session, error) {
	session := &Session{}
	refreshHash := hashRefreshToken(refreshToken)

	query := `
		SELECT id, user_id, device_id, family_id, refresh_hash, expires_at, revoked_at, rotated_from, created_at, updated_at
		FROM sessions
		WHERE refresh_hash = $1 AND expires_at > NOW() AND revoked_at IS NULL
	`

	err := r.db.QueryRowContext(ctx, query, refreshHash).Scan(
		&session.ID, &session.UserID, &session.DeviceID, &session.FamilyID,
		&session.RefreshHash, &session.ExpiresAt, &session.RevokedAt,
		&session.RotatedFrom, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// GetByRefreshTokenIncludingRevoked retrieves a session by refresh token hash
// regardless of revocation state. Used to detect replay of rotated tokens.
func (r *SessionRepository) GetByRefreshTokenIncludingRevoked(ctx context.Context, refreshToken string) (*Session, error) {
	session := &Session{}
	refreshHash := hashRefreshToken(refreshToken)

	query := `
		SELECT id, user_id, device_id, family_id, refresh_hash, expires_at, revoked_at, rotated_from, created_at, updated_at
		FROM sessions
		WHERE refresh_hash = $1
	`

	err := r.db.QueryRowContext(ctx, query, refreshHash).Scan(
		&session.ID, &session.UserID, &session.DeviceID, &session.FamilyID,
		&session.RefreshHash, &session.ExpiresAt, &session.RevokedAt,
		&session.RotatedFrom, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// GetByID retrieves a session by its primary key regardless of state.
func (r *SessionRepository) GetByID(ctx context.Context, sessionID string) (*Session, error) {
	session := &Session{}
	query := `
		SELECT id, user_id, device_id, family_id, refresh_hash, expires_at, revoked_at, rotated_from, created_at, updated_at
		FROM sessions
		WHERE id = $1
	`

	err := r.db.QueryRowContext(ctx, query, sessionID).Scan(
		&session.ID, &session.UserID, &session.DeviceID, &session.FamilyID,
		&session.RefreshHash, &session.ExpiresAt, &session.RevokedAt,
		&session.RotatedFrom, &session.CreatedAt, &session.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	return session, nil
}

// Revoke revokes a session
func (r *SessionRepository) Revoke(ctx context.Context, sessionID string) error {
	query := `
		UPDATE sessions
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND revoked_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, sessionID)
	if err != nil {
		return fmt.Errorf("failed to revoke session: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("session not found or already revoked")
	}

	return nil
}

// RevokeFamily revokes all sessions in a family
func (r *SessionRepository) RevokeFamily(ctx context.Context, familyID string) error {
	query := `
		UPDATE sessions
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE family_id = $1 AND revoked_at IS NULL
	`

	_, err := r.db.ExecContext(ctx, query, familyID)
	if err != nil {
		return fmt.Errorf("failed to revoke session family: %w", err)
	}

	return nil
}

// Rotate refreshes a session with a new refresh token. It assumes the caller
// has already established the token is valid; prefer RotateRefreshAtomically
// for the request path, which detects replays within a single transaction.
func (r *SessionRepository) Rotate(ctx context.Context, sessionID, newRefreshToken string, ttl time.Duration) (*Session, error) {
	// Start transaction
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Get current session
	var currentSession Session
	query := `
		SELECT id, user_id, device_id, family_id, refresh_hash, expires_at, created_at
		FROM sessions
		WHERE id = $1 AND expires_at > NOW() AND revoked_at IS NULL
		FOR UPDATE
	`
	err = tx.QueryRowContext(ctx, query, sessionID).Scan(
		&currentSession.ID, &currentSession.UserID, &currentSession.DeviceID,
		&currentSession.FamilyID, &currentSession.RefreshHash, &currentSession.ExpiresAt,
		&currentSession.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// Revoke current session
	revokeQuery := `
		UPDATE sessions
		SET revoked_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`
	_, err = tx.ExecContext(ctx, revokeQuery, sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke session: %w", err)
	}

	// Create new session
	newSession := &Session{
		ID:          uuid.New().String(),
		UserID:      currentSession.UserID,
		DeviceID:    currentSession.DeviceID,
		FamilyID:    currentSession.FamilyID,
		RefreshHash: hashRefreshToken(newRefreshToken),
		ExpiresAt:   time.Now().Add(ttl),
		RotatedFrom: &sessionID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	insertQuery := `
		INSERT INTO sessions (id, user_id, device_id, family_id, refresh_hash, expires_at, rotated_from, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err = tx.ExecContext(ctx, insertQuery,
		newSession.ID, newSession.UserID, newSession.DeviceID, newSession.FamilyID,
		newSession.RefreshHash, newSession.ExpiresAt, newSession.RotatedFrom,
		newSession.CreatedAt, newSession.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create new session: %w", err)
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return newSession, nil
}

// RotateRefreshAtomically rotates a refresh token within a single transaction.
// It locks the session row, so concurrent replays are serialized: exactly one
// caller succeeds, and any caller that observes a revoked/expired token revokes
// the whole session family before returning ErrInvalidRefreshToken.
func (r *SessionRepository) RotateRefreshAtomically(ctx context.Context, refreshToken, newRefreshToken string, ttl time.Duration) (*Session, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	refreshHash := hashRefreshToken(refreshToken)

	var current Session
	err = tx.QueryRowContext(ctx, `
		SELECT id, user_id, device_id, family_id, expires_at, revoked_at
		FROM sessions
		WHERE refresh_hash = $1
		FOR UPDATE
	`, refreshHash).Scan(
		&current.ID, &current.UserID, &current.DeviceID,
		&current.FamilyID, &current.ExpiresAt, &current.RevokedAt)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidRefreshToken
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get session: %w", err)
	}

	// A revoked or expired token is a replay (or expired credential): revoke
	// the entire family to contain theft, then fail closed.
	if current.RevokedAt != nil || time.Now().After(current.ExpiresAt) {
		if _, err := tx.ExecContext(ctx, `
			UPDATE sessions
			SET revoked_at = NOW(), updated_at = NOW()
			WHERE family_id = $1 AND revoked_at IS NULL
		`, current.FamilyID); err != nil {
			return nil, fmt.Errorf("failed to revoke session family: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("failed to commit transaction: %w", err)
		}
		return nil, ErrInvalidRefreshToken
	}

	// Revoke the consumed session and issue a new one in the same family.
	if _, err := tx.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = NOW(), updated_at = NOW() WHERE id = $1
	`, current.ID); err != nil {
		return nil, fmt.Errorf("failed to revoke session: %w", err)
	}

	newSession := &Session{
		ID:          uuid.New().String(),
		UserID:      current.UserID,
		DeviceID:    current.DeviceID,
		FamilyID:    current.FamilyID,
		RefreshHash: hashRefreshToken(newRefreshToken),
		ExpiresAt:   time.Now().Add(ttl),
		RotatedFrom: &current.ID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, device_id, family_id, refresh_hash, expires_at, rotated_from, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, newSession.ID, newSession.UserID, newSession.DeviceID, newSession.FamilyID,
		newSession.RefreshHash, newSession.ExpiresAt, newSession.RotatedFrom,
		newSession.CreatedAt, newSession.UpdatedAt); err != nil {
		return nil, fmt.Errorf("failed to create new session: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return newSession, nil
}

func hashRefreshToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}
