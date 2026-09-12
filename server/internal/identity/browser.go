package identity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"strings"
	"time"

	"github.com/google/uuid"
)

// BrowserSession is independent of storage devices and native refresh tokens.
// Only a hash of the opaque cookie is persisted. CSRF tokens are not credentials.
type BrowserSession struct {
	ID        string    `json:"-"`
	UserID    string    `json:"-"`
	CSRFToken string    `json:"csrf_token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func browserTokenHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (s *Service) LoginBrowser(ctx context.Context, account, password string, remember bool) (*BrowserSession, string, error) {
	if account == "" || password == "" {
		return nil, "", invalidInput("account and password are required")
	}
	user, err := s.userRepo.GetByAccount(ctx, account)
	if err != nil {
		return nil, "", err
	}
	if user == nil {
		return nil, "", ErrInvalidCredentials
	}
	valid, err := VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return nil, "", err
	}
	if !valid {
		return nil, "", ErrInvalidCredentials
	}
	token, err := GenerateRefreshToken()
	if err != nil {
		return nil, "", err
	}
	csrf, err := GenerateRefreshToken()
	if err != nil {
		return nil, "", err
	}
	ttl := 12 * time.Hour
	if remember {
		ttl = 7 * 24 * time.Hour
	}
	session := &BrowserSession{ID: uuid.NewString(), UserID: user.ID, CSRFToken: csrf, ExpiresAt: time.Now().Add(ttl)}
	// Compare the password hash again in the insert to fence concurrent password changes.
	result, err := s.db.ExecContext(ctx, `INSERT INTO browser_sessions(id,user_id,token_hash,csrf_token,expires_at)
        SELECT $1,id,$2,$3,$4 FROM users WHERE id=$5 AND status='active' AND password_hash=$6`,
		session.ID, browserTokenHash(token), csrf, session.ExpiresAt, user.ID, user.PasswordHash)
	if err != nil {
		return nil, "", err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return nil, "", ErrInvalidCredentials
	}
	return session, token, nil
}

func (s *Service) AuthenticateBrowser(ctx context.Context, token string) (*BrowserSession, error) {
	if len(token) < 32 || len(token) > 512 {
		return nil, ErrInvalidCredentials
	}
	session := &BrowserSession{}
	err := s.db.QueryRowContext(ctx, `SELECT b.id,b.user_id,b.csrf_token,b.expires_at
        FROM browser_sessions b JOIN users u ON u.id=b.user_id
        WHERE b.token_hash=$1 AND b.expires_at>NOW() AND u.status='active'`, browserTokenHash(token)).
		Scan(&session.ID, &session.UserID, &session.CSRFToken, &session.ExpiresAt)
	if err == sql.ErrNoRows {
		return nil, ErrInvalidCredentials
	}
	return session, err
}

func (s *Service) LogoutBrowser(ctx context.Context, userID, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM browser_sessions WHERE id=$1 AND user_id=$2`, sessionID, userID)
	return err
}

func (s *Service) RenameDevice(ctx context.Context, userID, deviceID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 128 {
		return invalidInput("name must contain 1 to 128 bytes")
	}
	result, err := s.db.ExecContext(ctx, `UPDATE devices SET name=$1,version=version+1,updated_at=NOW() WHERE id=$2 AND user_id=$3 AND status='active'`, name, deviceID, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrDeviceNotFound
	}
	return nil
}
