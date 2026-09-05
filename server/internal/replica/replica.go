package replica

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound indicates the replica does not exist or is not visible.
var ErrNotFound = errors.New("not found")

// ErrVersionConflict indicates an optimistic concurrency conflict.
var ErrVersionConflict = errors.New("version conflict")

// ErrInvalidTransition indicates an illegal replica state transition.
var ErrInvalidTransition = errors.New("invalid state transition")

// Replica represents a replica of a file object.
type Replica struct {
	ID         string     `json:"id"`
	ObjectID   string     `json:"object_id"`
	DeviceID   string     `json:"device_id"`
	State      string     `json:"state"`
	Size       *int64     `json:"size,omitempty"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	ReportedAt *time.Time `json:"reported_at,omitempty"`
	Version    int64      `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func scanReplica(scanner interface {
	Scan(dest ...interface{}) error
}) (*Replica, error) {
	r := &Replica{}
	err := scanner.Scan(
		&r.ID, &r.ObjectID, &r.DeviceID, &r.State,
		&r.Size, &r.VerifiedAt, &r.ReportedAt,
		&r.Version, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// Repository handles replica database operations.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new replica Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// Create creates a new replica. The object and device must belong to the same
// account as the caller.
func (r *Repository) Create(ctx context.Context, userID, objectID, deviceID string) (*Replica, error) {
	var objectUser, deviceUser string
	err := r.db.QueryRowContext(ctx,
		`SELECT user_id::text FROM file_objects WHERE id = $1`, objectID).Scan(&objectUser)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get object: %w", err)
	}
	if objectUser != userID {
		return nil, ErrNotFound
	}

	err = r.db.QueryRowContext(ctx,
		`SELECT user_id::text FROM devices WHERE id = $1`, deviceID).Scan(&deviceUser)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	if deviceUser != userID {
		return nil, ErrNotFound
	}

	replica := &Replica{
		ID:        uuid.New().String(),
		ObjectID:  objectID,
		DeviceID:  deviceID,
		State:     "pending",
		Version:   1,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	query := `
		INSERT INTO replicas (id, object_id, device_id, user_id, state, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = r.db.ExecContext(ctx, query,
		replica.ID, replica.ObjectID, replica.DeviceID, userID,
		replica.State, replica.Version, replica.CreatedAt, replica.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create replica: %w", err)
	}

	return replica, nil
}

// Get retrieves a replica by ID, scoped to the owning account.
func (r *Repository) Get(ctx context.Context, userID, id string) (*Replica, error) {
	query := `
		SELECT r.id, r.object_id, r.device_id, r.state, r.size, r.verified_at, r.reported_at, r.version, r.created_at, r.updated_at
		FROM replicas r
		JOIN file_objects fo ON r.object_id = fo.id
		WHERE r.id = $1 AND fo.user_id::text = $2
	`

	replica, err := scanReplica(r.db.QueryRowContext(ctx, query, id, userID))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get replica: %w", err)
	}

	return replica, nil
}

// ListByObject lists replicas for an object, scoped to the owning account.
func (r *Repository) ListByObject(ctx context.Context, userID, objectID string) ([]*Replica, error) {
	query := `
		SELECT r.id, r.object_id, r.device_id, r.state, r.size, r.verified_at, r.reported_at, r.version, r.created_at, r.updated_at
		FROM replicas r
		JOIN file_objects fo ON r.object_id = fo.id
		WHERE r.object_id = $1 AND fo.user_id::text = $2
		ORDER BY r.created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, objectID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list replicas: %w", err)
	}
	defer rows.Close()

	var replicas []*Replica
	for rows.Next() {
		replica, err := scanReplica(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan replica: %w", err)
		}
		replicas = append(replicas, replica)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate replicas: %w", err)
	}

	return replicas, nil
}

// CountReady counts the number of ready replicas for an object, scoped to the
// owning account.
func (r *Repository) CountReady(ctx context.Context, userID, objectID string) (int, error) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM replicas r
		JOIN file_objects fo ON r.object_id = fo.id
		WHERE r.object_id = $1 AND r.state = 'ready' AND fo.user_id::text = $2
	`

	err := r.db.QueryRowContext(ctx, query, objectID, userID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count ready replicas: %w", err)
	}

	return count, nil
}

// UpdateState updates the state of a replica with optimistic concurrency and a
// state-machine check.
func (r *Repository) UpdateState(ctx context.Context, userID, id string, expectedVersion int64, state string) error {
	if !validState(state) {
		return fmt.Errorf("invalid replica state: %s", state)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var currentState string
	err = tx.QueryRowContext(ctx, `
		SELECT r.state
		FROM replicas r
		JOIN file_objects fo ON r.object_id = fo.id
		WHERE r.id = $1 AND fo.user_id::text = $2
		FOR UPDATE OF r
	`, id, userID).Scan(&currentState)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get replica: %w", err)
	}

	if !validTransition(currentState, state) {
		return ErrInvalidTransition
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE replicas
		SET state = $1, version = version + 1, updated_at = NOW()
		WHERE id = $2 AND version = $3
	`, state, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("failed to update state: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return ErrVersionConflict
	}

	return tx.Commit()
}

// UpdateVerified marks a replica as ready and verified. Only the device that
// owns the replica may report the result, the reported size must match the
// authoritative object size, and the object must be active (not deleted).
func (r *Repository) UpdateVerified(ctx context.Context, userID, deviceID, id string, size int64, expectedVersion int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var currentState string
	var replicaDevice string
	var objectSize int64
	var objectStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT r.state, r.device_id::text, fo.size, fo.status
		FROM replicas r
		JOIN file_objects fo ON r.object_id = fo.id
		WHERE r.id = $1 AND fo.user_id::text = $2
		FOR UPDATE OF r
	`, id, userID).Scan(&currentState, &replicaDevice, &objectSize, &objectStatus)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get replica: %w", err)
	}
	if currentState == "deleted" {
		return ErrInvalidTransition
	}
	if replicaDevice != deviceID {
		return ErrNotFound
	}
	if objectStatus == "deleted" {
		return ErrInvalidTransition
	}
	if size != objectSize {
		return fmt.Errorf("reported size %d does not match object size %d", size, objectSize)
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE replicas
		SET state = 'ready', size = $1, verified_at = NOW(), version = version + 1, updated_at = NOW()
		WHERE id = $2 AND version = $3
	`, size, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("failed to update verified: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return ErrVersionConflict
	}

	return tx.Commit()
}

// UpdateReported updates the reported timestamp.
func (r *Repository) UpdateReported(ctx context.Context, userID, id string, expectedVersion int64) error {
	query := `
		UPDATE replicas
		SET reported_at = NOW(), version = version + 1, updated_at = NOW()
		WHERE id = $1 AND version = $2
		  AND object_id IN (SELECT id FROM file_objects WHERE user_id::text = $3)
	`

	res, err := r.db.ExecContext(ctx, query, id, expectedVersion, userID)
	if err != nil {
		return fmt.Errorf("failed to update reported: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return ErrVersionConflict
	}

	return nil
}

// Delete marks a replica as deleted.
func (r *Repository) Delete(ctx context.Context, userID, id string, expectedVersion int64) error {
	query := `
		UPDATE replicas
		SET state = 'deleted', version = version + 1, updated_at = NOW()
		WHERE id = $1 AND version = $2
		  AND object_id IN (SELECT id FROM file_objects WHERE user_id::text = $3)
	`

	res, err := r.db.ExecContext(ctx, query, id, expectedVersion, userID)
	if err != nil {
		return fmt.Errorf("failed to delete replica: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return ErrVersionConflict
	}

	return nil
}

var validStates = map[string]bool{
	"pending": true, "ready": true, "missing": true,
	"corrupt": true, "deleting": true, "deleted": true,
}

func validState(s string) bool { return validStates[s] }

func validTransition(from, to string) bool {
	if !validState(to) {
		return false
	}
	if from == "deleted" {
		return false
	}
	if to == "ready" {
		return from == "pending" || from == "missing" || from == "corrupt"
	}
	return true
}

// Service handles replica business logic.
type Service struct {
	repo *Repository
}

// NewService creates a new replica Service.
func NewService(db *sql.DB) *Service {
	return &Service{
		repo: NewRepository(db),
	}
}

// Create creates a new replica.
func (s *Service) Create(ctx context.Context, userID, objectID, deviceID string) (*Replica, error) {
	return s.repo.Create(ctx, userID, objectID, deviceID)
}

// Get retrieves a replica.
func (s *Service) Get(ctx context.Context, userID, id string) (*Replica, error) {
	return s.repo.Get(ctx, userID, id)
}

// ListByObject lists replicas for an object.
func (s *Service) ListByObject(ctx context.Context, userID, objectID string) ([]*Replica, error) {
	return s.repo.ListByObject(ctx, userID, objectID)
}

// UpdateState updates the state of a replica.
func (s *Service) UpdateState(ctx context.Context, userID, id string, expectedVersion int64, state string) error {
	return s.repo.UpdateState(ctx, userID, id, expectedVersion, state)
}

// UpdateVerified updates the verified timestamp.
func (s *Service) UpdateVerified(ctx context.Context, userID, deviceID, id string, size int64, expectedVersion int64) error {
	return s.repo.UpdateVerified(ctx, userID, deviceID, id, size, expectedVersion)
}

// UpdateReported updates the reported timestamp.
func (s *Service) UpdateReported(ctx context.Context, userID, id string, expectedVersion int64) error {
	return s.repo.UpdateReported(ctx, userID, id, expectedVersion)
}

// Delete deletes a replica.
func (s *Service) Delete(ctx context.Context, userID, id string, expectedVersion int64) error {
	return s.repo.Delete(ctx, userID, id, expectedVersion)
}

// CountReady counts the number of ready replicas.
func (s *Service) CountReady(ctx context.Context, userID, objectID string) (int, error) {
	return s.repo.CountReady(ctx, userID, objectID)
}
