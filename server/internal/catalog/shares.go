package catalog

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrShareNotFound = errors.New("share not found")
	ErrShareExpired  = errors.New("share expired or download limit reached")
)

type Share struct {
	ID            string    `json:"id"`
	FileID        string    `json:"file_id"`
	FileName      string    `json:"file_name,omitempty"`
	Token         string    `json:"token,omitempty"`
	Status        string    `json:"status"`
	ExpiresAt     time.Time `json:"expires_at"`
	MaxDownloads  int       `json:"max_downloads"`
	DownloadCount int       `json:"download_count"`
	CreatedAt     time.Time `json:"created_at"`
}

func (r *Repository) ListShares(ctx context.Context, userID string) ([]Share, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id::text,s.file_entry_id::text,e.name,s.status,s.expires_at,s.max_downloads,s.download_count,s.created_at
		FROM shares s JOIN file_entries e ON e.id=s.file_entry_id
		WHERE s.user_id=$1 ORDER BY s.created_at DESC LIMIT 200
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	shares := make([]Share, 0)
	for rows.Next() {
		var share Share
		if err := rows.Scan(&share.ID, &share.FileID, &share.FileName, &share.Status, &share.ExpiresAt, &share.MaxDownloads, &share.DownloadCount, &share.CreatedAt); err != nil {
			return nil, err
		}
		shares = append(shares, share)
	}
	return shares, rows.Err()
}

type ResolvedShare struct {
	Share  *Share
	File   *UnifiedFile
	UserID string
}

func (r *Repository) CreateShare(ctx context.Context, userID, fileID string, expiresIn time.Duration, maxDownloads int) (*Share, error) {
	if expiresIn <= 0 || expiresIn > 30*24*time.Hour || maxDownloads < 0 {
		return nil, fmt.Errorf("%w: invalid share lifetime or download limit", ErrInvalidRequest)
	}
	file, err := r.GetUnifiedFile(ctx, userID, fileID)
	if err != nil {
		return nil, err
	}
	if file.Status != "active" {
		return nil, ErrInvalidState
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	share := &Share{ID: uuid.NewString(), FileID: fileID, Token: token, Status: "active", ExpiresAt: time.Now().Add(expiresIn), MaxDownloads: maxDownloads}
	err = r.db.QueryRowContext(ctx, `INSERT INTO shares(id,user_id,file_entry_id,token_hash,status,expires_at,max_downloads) VALUES($1,$2,$3,$4,'active',$5,$6) RETURNING created_at`, share.ID, userID, fileID, hash[:], share.ExpiresAt, maxDownloads).Scan(&share.CreatedAt)
	return share, err
}

// ResolveShare atomically consumes one use and resolves the current ready
// replica. A zero max_downloads value means unlimited uses until expiry.
func (r *Repository) ResolveShare(ctx context.Context, token string) (*ResolvedShare, error) {
	if token == "" || len(token) > 128 {
		return nil, ErrShareNotFound
	}
	hash := sha256.Sum256([]byte(token))
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	share := &Share{}
	var userID string
	err = tx.QueryRowContext(ctx, `
		UPDATE shares SET download_count=download_count+1,updated_at=NOW()
		WHERE token_hash=$1 AND status='active' AND expires_at>NOW()
		  AND (max_downloads=0 OR download_count<max_downloads)
		RETURNING id::text,user_id::text,file_entry_id::text,status,expires_at,max_downloads,download_count,created_at
	`, hash[:]).Scan(&share.ID, &userID, &share.FileID, &share.Status, &share.ExpiresAt, &share.MaxDownloads, &share.DownloadCount, &share.CreatedAt)
	if err == sql.ErrNoRows {
		var exists bool
		if scanErr := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM shares WHERE token_hash=$1)`, hash[:]).Scan(&exists); scanErr != nil {
			return nil, scanErr
		}
		if exists {
			return nil, ErrShareExpired
		}
		return nil, ErrShareNotFound
	}
	if err != nil {
		return nil, err
	}
	file, err := getUnifiedFile(ctx, tx, userID, share.FileID)
	if err != nil {
		return nil, err
	}
	if file == nil || file.Status != "active" || file.ReplicaEndpoint == "" || file.ContentLocalFileID == "" {
		return nil, ErrInvalidState
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ResolvedShare{Share: share, File: file, UserID: userID}, nil
}

func (r *Repository) RevokeShare(ctx context.Context, userID, shareID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE shares SET status='revoked',updated_at=NOW() WHERE id=$1 AND user_id=$2 AND status='active'`, shareID, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrShareNotFound
	}
	return nil
}
