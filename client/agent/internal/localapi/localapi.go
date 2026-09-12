package localapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OutgoingOp represents an outgoing operation waiting to be synced with the
// control server. It maps 1:1 onto the SQLite outgoing_ops table.
type OutgoingOp struct {
	ID             string          `json:"id"`
	OpType         string          `json:"op_type"`
	IdempotencyKey string          `json:"idempotency_key"`
	RequestData    json.RawMessage `json:"request_data"`
	Status         string          `json:"status"`
	ResponseCode   *int            `json:"response_code,omitempty"`
	ResponseData   json.RawMessage `json:"response_data,omitempty"`
	RetryCount     int             `json:"retry_count"`
	NextRetryAt    *time.Time      `json:"next_retry_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

const (
	// OpStatusPending marks an op ready to be attempted.
	OpStatusPending = "pending"
	// OpStatusProcessing marks an op currently being attempted.
	OpStatusProcessing = "processing"
	// OpStatusCompleted marks an op successfully applied on the server.
	OpStatusCompleted = "completed"
	// OpStatusFailed marks an op rejected by a non-retryable request error.
	OpStatusFailed = "failed"
)

// sqliteTimeFormat matches SQLite's datetime('now') format so string values
// stored by the application sort consistently with defaults.
const sqliteTimeFormat = "2006-01-02 15:04:05"

func formatTime(t time.Time) string {
	return t.UTC().Format(sqliteTimeFormat)
}

// Repository handles local API database operations.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new local API Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// CreateOutgoingOp creates a new outgoing operation.
func (r *Repository) CreateOutgoingOp(ctx context.Context, opType, idempotencyKey string, requestData interface{}) (*OutgoingOp, error) {
	requestBytes, err := json.Marshal(requestData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	now := time.Now().UTC()
	op := &OutgoingOp{
		ID:             newID(),
		OpType:         opType,
		IdempotencyKey: idempotencyKey,
		RequestData:    requestBytes,
		Status:         OpStatusPending,
		RetryCount:     0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	query := `
		INSERT INTO outgoing_ops (id, op_type, idempotency_key, request_data, status, retry_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = r.db.ExecContext(ctx, query,
		op.ID, op.OpType, op.IdempotencyKey, op.RequestData,
		op.Status, op.RetryCount, formatTime(op.CreatedAt), formatTime(op.UpdatedAt))
	if err != nil {
		return nil, fmt.Errorf("failed to create outgoing op: %w", err)
	}

	return op, nil
}

// GetPendingOps retrieves operations that are due for a retry.
func (r *Repository) GetPendingOps(ctx context.Context, limit int) ([]*OutgoingOp, error) {
	query := `
		SELECT id, op_type, idempotency_key, request_data, status, response_code, response_data, retry_count, next_retry_at, created_at, updated_at
		FROM outgoing_ops
		WHERE rowid = (
			SELECT MIN(rowid) FROM outgoing_ops WHERE status IN ('pending','processing')
		)
		  AND status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)
		ORDER BY rowid ASC
		LIMIT ?
	`

	rows, err := r.db.QueryContext(ctx, query, OpStatusPending, formatTime(time.Now().UTC()), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending ops: %w", err)
	}
	defer rows.Close()

	var ops []*OutgoingOp
	for rows.Next() {
		op, err := scanOp(rows)
		if err != nil {
			return nil, err
		}
		ops = append(ops, op)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate pending ops: %w", err)
	}

	return ops, nil
}

// MarkProcessing transitions an op to the processing state.
func (r *Repository) MarkProcessing(ctx context.Context, opID string) error {
	query := `
		UPDATE outgoing_ops
		SET status = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query, OpStatusProcessing, formatTime(time.Now().UTC()), opID)
	if err != nil {
		return fmt.Errorf("failed to mark op processing: %w", err)
	}
	return nil
}

// MarkOpCompleted marks an operation as completed with the server response.
func (r *Repository) MarkOpCompleted(ctx context.Context, opID string, responseCode int, responseData interface{}) error {
	responseBytes, err := json.Marshal(responseData)
	if err != nil {
		return fmt.Errorf("failed to marshal response data: %w", err)
	}

	query := `
		UPDATE outgoing_ops
		SET status = ?, response_code = ?, response_data = ?, updated_at = ?
		WHERE id = ?
	`
	_, err = r.db.ExecContext(ctx, query,
		OpStatusCompleted, responseCode, responseBytes, formatTime(time.Now().UTC()), opID)
	if err != nil {
		return fmt.Errorf("failed to complete op: %w", err)
	}
	return nil
}

// MarkOpRetryable marks an operation for a later retry with backoff.
func (r *Repository) MarkOpRetryable(ctx context.Context, opID string, retryAfter time.Duration) error {
	query := `
		UPDATE outgoing_ops
		SET status = ?, retry_count = retry_count + 1, next_retry_at = ?, updated_at = ?
		WHERE id = ?
	`
	// The initial schema stores sortable timestamps at one-second precision.
	// Clamp sub-second retry delays so formatting cannot turn a retry into an
	// immediate busy loop that exhausts the budget.
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	nextRetry := time.Now().UTC().Add(retryAfter)
	_, err := r.db.ExecContext(ctx, query,
		OpStatusPending, formatTime(nextRetry), formatTime(time.Now().UTC()), opID)
	if err != nil {
		return fmt.Errorf("failed to mark op retryable: %w", err)
	}
	return nil
}

// MarkOpFailed marks an operation as permanently failed after the control
// plane rejected it with a deterministic, non-retryable response.
func (r *Repository) MarkOpFailed(ctx context.Context, opID string, responseCode int, responseData interface{}) error {
	responseBytes, err := json.Marshal(responseData)
	if err != nil {
		return fmt.Errorf("failed to marshal response data: %w", err)
	}

	query := `
		UPDATE outgoing_ops
		SET status = ?, response_code = ?, response_data = ?, updated_at = ?
		WHERE id = ?
	`
	_, err = r.db.ExecContext(ctx, query,
		OpStatusFailed, responseCode, responseBytes, formatTime(time.Now().UTC()), opID)
	if err != nil {
		return fmt.Errorf("failed to fail op: %w", err)
	}
	return nil
}

// CleanupCompletedOps removes completed operations older than the duration.
func (r *Repository) CleanupCompletedOps(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-olderThan)
	query := `
		DELETE FROM outgoing_ops
		WHERE status = ? AND updated_at < ?
	`

	result, err := r.db.ExecContext(ctx, query, OpStatusCompleted, formatTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup completed ops: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}
	return rows, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanOp(scanner rowScanner) (*OutgoingOp, error) {
	op := &OutgoingOp{}
	var createdRaw, updatedRaw string
	var nextRetryRaw sql.NullString
	var responseDataRaw sql.NullString

	err := scanner.Scan(
		&op.ID, &op.OpType, &op.IdempotencyKey, &op.RequestData,
		&op.Status, &op.ResponseCode, &responseDataRaw, &op.RetryCount,
		&nextRetryRaw, &createdRaw, &updatedRaw)
	if err != nil {
		return nil, fmt.Errorf("failed to scan outgoing op: %w", err)
	}

	op.CreatedAt, _ = time.Parse(sqliteTimeFormat, createdRaw)
	op.UpdatedAt, _ = time.Parse(sqliteTimeFormat, updatedRaw)
	if nextRetryRaw.Valid && nextRetryRaw.String != "" {
		if t, err := time.Parse(sqliteTimeFormat, nextRetryRaw.String); err == nil {
			op.NextRetryAt = &t
		}
	}
	if responseDataRaw.Valid {
		op.ResponseData = json.RawMessage(responseDataRaw.String)
	}

	return op, nil
}

// Service handles local API business logic.
type Service struct {
	repo *Repository
}

// NewService creates a new local API Service.
func NewService(db *sql.DB) *Service {
	return &Service{
		repo: NewRepository(db),
	}
}

// CreateOutgoingOp creates a new outgoing operation.
func (s *Service) CreateOutgoingOp(ctx context.Context, opType, idempotencyKey string, requestData interface{}) (*OutgoingOp, error) {
	return s.repo.CreateOutgoingOp(ctx, opType, idempotencyKey, requestData)
}

// GetPendingOps retrieves pending operations.
func (s *Service) GetPendingOps(ctx context.Context, limit int) ([]*OutgoingOp, error) {
	return s.repo.GetPendingOps(ctx, limit)
}

// MarkOpCompleted marks an operation as completed.
func (s *Service) MarkOpCompleted(ctx context.Context, opID string, responseCode int, responseData interface{}) error {
	return s.repo.MarkOpCompleted(ctx, opID, responseCode, responseData)
}

// MarkOpFailed marks an operation as permanently failed.
func (s *Service) MarkOpFailed(ctx context.Context, opID string, responseCode int, responseData interface{}) error {
	return s.repo.MarkOpFailed(ctx, opID, responseCode, responseData)
}

// MarkOpRetryable marks an operation for a later retry with backoff.
func (s *Service) MarkOpRetryable(ctx context.Context, opID string, retryAfter time.Duration) error {
	return s.repo.MarkOpRetryable(ctx, opID, retryAfter)
}

// SyncManager manages synchronization with retry logic.
type SyncManager struct {
	localService *Service
	maxRetries   int
	baseDelay    time.Duration
}

// NewSyncManager creates a new SyncManager.
func NewSyncManager(localService *Service, maxRetries int, baseDelay time.Duration) *SyncManager {
	return &SyncManager{
		localService: localService,
		maxRetries:   maxRetries,
		baseDelay:    baseDelay,
	}
}

// ProcessOutgoingOps processes pending outgoing operations.
func (m *SyncManager) ProcessOutgoingOps(ctx context.Context, processFn func(ctx context.Context, op *OutgoingOp) error) error {
	ops, err := m.localService.GetPendingOps(ctx, 10)
	if err != nil {
		return fmt.Errorf("failed to get pending ops: %w", err)
	}

	for _, op := range ops {
		if err := processFn(ctx, op); err != nil {
			if op.RetryCount >= m.maxRetries {
				if ferr := m.localService.MarkOpFailed(ctx, op.ID, 0, map[string]string{"error": err.Error()}); ferr != nil {
					return ferr
				}
				continue
			}

			delay := m.baseDelay * time.Duration(1<<uint(op.RetryCount))
			if delay > 5*time.Minute {
				delay = 5 * time.Minute
			}
			if rerr := m.localService.MarkOpRetryable(ctx, op.ID, delay); rerr != nil {
				return rerr
			}
			continue
		}

		if cerr := m.localService.MarkOpCompleted(ctx, op.ID, 0, nil); cerr != nil {
			return cerr
		}
	}

	return nil
}

func newID() string {
	return uuid.New().String()
}
