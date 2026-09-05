package sync

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// Event represents an account event
type Event struct {
	UserID        string          `json:"user_id"`
	Seq           int64           `json:"seq"`
	Type          string          `json:"type"`
	EntityID      string          `json:"entity_id"`
	EntityVersion *int64          `json:"entity_version,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Repository handles sync database operations.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new sync Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// PublishEvent publishes an account event. Sequence numbers are allocated
// atomically per account by locking the account row, so concurrent writers
// cannot collide.
func (r *Repository) PublishEvent(ctx context.Context, userID, eventType, entityID string, entityVersion *int64, payload interface{}) (*Event, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	event, err := r.PublishEventTx(ctx, tx, userID, eventType, entityID, entityVersion, payload)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit event transaction: %w", err)
	}

	return event, nil
}

// PublishEventTx publishes an account event within an existing transaction.
// The account row is locked to serialize sequence allocation so the event can
// be committed atomically with the domain write that produced it.
func (r *Repository) PublishEventTx(ctx context.Context, tx *sql.Tx, userID, eventType, entityID string, entityVersion *int64, payload interface{}) (*Event, error) {
	var locked int
	err := tx.QueryRowContext(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&locked)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("user not found: %s", userID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lock account for sequence allocation: %w", err)
	}

	var nextSeq int64
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM account_events WHERE user_id = $1`, userID).Scan(&nextSeq)
	if err != nil {
		return nil, fmt.Errorf("failed to get next sequence: %w", err)
	}

	var payloadBytes []byte
	if payload != nil {
		payloadBytes, err = json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal payload: %w", err)
		}
	}

	event := &Event{
		UserID:        userID,
		Seq:           nextSeq,
		Type:          eventType,
		EntityID:      entityID,
		EntityVersion: entityVersion,
		Payload:       payloadBytes,
		CreatedAt:     time.Now().UTC(),
	}

	insertQuery := `
		INSERT INTO account_events (user_id, seq, type, entity_id, entity_version, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err = tx.ExecContext(ctx, insertQuery,
		event.UserID, event.Seq, event.Type, event.EntityID,
		event.EntityVersion, event.Payload, event.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert event: %w", err)
	}

	return event, nil
}

// GetEvents retrieves events for a user after a sequence number.
func (r *Repository) GetEvents(ctx context.Context, userID string, sinceSeq int64, limit int) ([]*Event, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT user_id, seq, type, entity_id, entity_version, payload, created_at
		FROM account_events
		WHERE user_id = $1 AND seq > $2
		ORDER BY seq ASC
		LIMIT $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, sinceSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		event := &Event{}
		var payload []byte
		if err := rows.Scan(
			&event.UserID, &event.Seq, &event.Type, &event.EntityID,
			&event.EntityVersion, &payload, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		event.Payload = payload
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate events: %w", err)
	}

	return events, nil
}

// GetLatestSeq returns the latest sequence number for a user.
func (r *Repository) GetLatestSeq(ctx context.Context, userID string) (int64, error) {
	var seq int64
	query := `SELECT COALESCE(MAX(seq), 0) FROM account_events WHERE user_id = $1`

	err := r.db.QueryRowContext(ctx, query, userID).Scan(&seq)
	if err != nil {
		return 0, fmt.Errorf("failed to get latest sequence: %w", err)
	}

	return seq, nil
}

// DeltaSyncRequest represents a delta sync request.
type DeltaSyncRequest struct {
	SinceSeq int64 `json:"since_seq"`
	Limit    int   `json:"limit"`
}

// DeltaSyncResponse represents a delta sync response.
type DeltaSyncResponse struct {
	Events    []*Event `json:"events"`
	HasMore   bool     `json:"has_more"`
	LatestSeq int64    `json:"latest_seq"`
}

// Service handles sync business logic.
type Service struct {
	repo *Repository
}

// NewService creates a new sync Service.
func NewService(db *sql.DB) *Service {
	return &Service{
		repo: NewRepository(db),
	}
}

// PublishEvent publishes an account event.
func (s *Service) PublishEvent(ctx context.Context, userID, eventType, entityID string, entityVersion *int64, payload interface{}) (*Event, error) {
	return s.repo.PublishEvent(ctx, userID, eventType, entityID, entityVersion, payload)
}

// GetEvents retrieves events for a user.
func (s *Service) GetEvents(ctx context.Context, userID string, sinceSeq int64, limit int) ([]*Event, error) {
	return s.repo.GetEvents(ctx, userID, sinceSeq, limit)
}

// GetLatestSeq retrieves the latest sequence for a user.
func (s *Service) GetLatestSeq(ctx context.Context, userID string) (int64, error) {
	return s.repo.GetLatestSeq(ctx, userID)
}

// DeltaSync performs a delta sync. The Agent cursor is persisted locally in
// SQLite, never on the server, so this only returns events and the latest
// sequence.
func (s *Service) DeltaSync(ctx context.Context, userID string, req *DeltaSyncRequest) (*DeltaSyncResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	events, err := s.repo.GetEvents(ctx, userID, req.SinceSeq, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	latestSeq, err := s.repo.GetLatestSeq(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest sequence: %w", err)
	}

	// has_more must not produce a false positive when the final page is exactly
	// `limit` events. Compare the last returned seq against the latest seq
	// instead of relying on len(events) == limit alone.
	hasMore := false
	if len(events) > 0 && len(events) == limit && events[len(events)-1].Seq < latestSeq {
		hasMore = true
	}

	return &DeltaSyncResponse{
		Events:    events,
		HasMore:   hasMore,
		LatestSeq: latestSeq,
	}, nil
}
