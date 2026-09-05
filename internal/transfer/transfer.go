package transfer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound indicates the task does not exist, is not visible to the caller,
// or is not in a claimable state.
var ErrNotFound = errors.New("not found")

// ErrNotClaimable indicates the task cannot be claimed (wrong target, terminal
// state, or an active lease held by someone else).
var ErrNotClaimable = errors.New("task not claimable")

// ErrInvalidTransition indicates an illegal state transition.
var ErrInvalidTransition = errors.New("invalid state transition")

// ErrVersionConflict indicates an optimistic concurrency conflict.
var ErrVersionConflict = errors.New("version conflict")

// Terminal states for a transfer task.
var terminalStates = map[string]bool{
	"completed":        true,
	"canceled":         true,
	"failed_permanent": true,
}

// validStates is the set of states permitted by the schema.
var validStates = map[string]bool{
	"queued": true, "assigned": true, "discovering": true, "connecting": true,
	"transferring": true, "verifying": true, "completed": true,
	"waiting_source": true, "retry_wait": true, "paused": true,
	"canceled": true, "failed_permanent": true,
}

// validTransitions is an explicit adjacency list for the transfer state machine.
// A transition is legal only if the destination is listed for the source state.
// Terminal states (completed, canceled, failed_permanent) have no outgoing edges.
var validTransitions = map[string]map[string]bool{
	"queued": {
		"assigned": true,
		"canceled": true,
	},
	"assigned": {
		"discovering": true,
		"canceled":    true,
	},
	"discovering": {
		"connecting":       true,
		"retry_wait":       true,
		"failed_permanent": true,
		"canceled":         true,
	},
	"connecting": {
		"transferring":     true,
		"retry_wait":       true,
		"failed_permanent": true,
		"canceled":         true,
	},
	"transferring": {
		"verifying":        true,
		"waiting_source":   true,
		"paused":           true,
		"failed_permanent": true,
		"canceled":         true,
	},
	"verifying": {
		"completed":        true,
		"retry_wait":       true,
		"failed_permanent": true,
		"canceled":         true,
	},
	"waiting_source": {
		"queued":   true,
		"canceled": true,
	},
	"retry_wait": {
		"queued":   true,
		"canceled": true,
	},
	"paused": {
		"transferring": true,
		"canceled":     true,
	},
}

// validTransition reports whether from -> to is a legal transition.
func validTransition(from, to string) bool {
	if !validStates[to] {
		return false
	}
	return validTransitions[from][to]
}

// TransferTask represents a transfer task.
type TransferTask struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	ObjectID       string     `json:"object_id"`
	TargetDeviceID string     `json:"target_device_id"`
	Reason         string     `json:"reason"`
	State          string     `json:"state"`
	Priority       int        `json:"priority"`
	LeaseOwner     *string    `json:"lease_owner,omitempty"`
	LeaseUntil     *time.Time `json:"lease_until,omitempty"`
	Attempt        int        `json:"attempt"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// TransferSource represents a transfer source.
type TransferSource struct {
	TaskID         string     `json:"task_id"`
	SourceDeviceID string     `json:"source_device_id"`
	Rank           int        `json:"rank"`
	LastError      string     `json:"last_error,omitempty"`
	DisabledAt     *time.Time `json:"disabled_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// Repository handles transfer database operations.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new transfer Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

const taskColumns = `id, user_id, object_id, target_device_id, reason, state, priority, lease_owner, lease_until, attempt, version, created_at, updated_at`

func scanTask(scanner interface {
	Scan(dest ...interface{}) error
}) (*TransferTask, error) {
	task := &TransferTask{}
	err := scanner.Scan(
		&task.ID, &task.UserID, &task.ObjectID, &task.TargetDeviceID,
		&task.Reason, &task.State, &task.Priority, &task.LeaseOwner,
		&task.LeaseUntil, &task.Attempt, &task.Version,
		&task.CreatedAt, &task.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return task, nil
}

// CreateTask creates a new transfer task. The object and target device are
// validated to belong to the same account.
func (r *Repository) CreateTask(ctx context.Context, userID, objectID, targetDeviceID, reason string, priority int) (*TransferTask, error) {
	if !validReason(reason) {
		return nil, fmt.Errorf("invalid transfer reason: %s", reason)
	}

	// Validate ownership: object and target device must belong to the user.
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
		`SELECT user_id::text FROM devices WHERE id = $1`, targetDeviceID).Scan(&deviceUser)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}
	if deviceUser != userID {
		return nil, ErrNotFound
	}

	task := &TransferTask{
		ID:             uuid.New().String(),
		UserID:         userID,
		ObjectID:       objectID,
		TargetDeviceID: targetDeviceID,
		Reason:         reason,
		State:          "queued",
		Priority:       priority,
		Attempt:        0,
		Version:        1,
		CreatedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
	}

	query := `
		INSERT INTO transfer_tasks (id, user_id, object_id, target_device_id, reason, state, priority, attempt, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err = r.db.ExecContext(ctx, query,
		task.ID, task.UserID, task.ObjectID, task.TargetDeviceID,
		task.Reason, task.State, task.Priority, task.Attempt,
		task.Version, task.CreatedAt, task.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create transfer task: %w", err)
	}

	return task, nil
}

// GetTask retrieves a transfer task by ID, scoped to the owning account.
func (r *Repository) GetTask(ctx context.Context, userID, id string) (*TransferTask, error) {
	query := `
		SELECT ` + taskColumns + `
		FROM transfer_tasks
		WHERE id = $1 AND user_id = $2
	`

	task, err := scanTask(r.db.QueryRowContext(ctx, query, id, userID))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer task: %w", err)
	}

	return task, nil
}

// ListTasks lists transfer tasks for a user.
func (r *Repository) ListTasks(ctx context.Context, userID, state string, limit int) ([]*TransferTask, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT ` + taskColumns + `
		FROM transfer_tasks
		WHERE user_id = $1 AND ($2 = '' OR state = $2)
		ORDER BY priority DESC, created_at ASC
		LIMIT $3
	`

	rows, err := r.db.QueryContext(ctx, query, userID, state, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list transfer tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*TransferTask
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan transfer task: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate transfer tasks: %w", err)
	}

	return tasks, nil
}

// minLeaseDuration and maxLeaseDuration bound the lease duration accepted by
// the server so callers cannot pin a task with an unbounded lease.
const (
	minLeaseDuration = time.Second
	maxLeaseDuration = 24 * time.Hour
)

// AcquireLease acquires a lease on a transfer task. Only the target device may
// claim it, and only when the task is claimable and no unexpired lease exists.
// A device cannot re-acquire (or bump the attempt counter) while a valid lease
// is already held; renewals must go through a separate operation.
func (r *Repository) AcquireLease(ctx context.Context, userID, taskID, deviceID string, duration time.Duration) (*TransferTask, error) {
	if duration < minLeaseDuration || duration > maxLeaseDuration {
		return nil, fmt.Errorf("lease duration must be between %s and %s", minLeaseDuration, maxLeaseDuration)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
		SELECT ` + taskColumns + `
		FROM transfer_tasks
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`

	task, err := scanTask(tx.QueryRowContext(ctx, query, taskID, userID))
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get transfer task: %w", err)
	}

	if task.TargetDeviceID != deviceID {
		return nil, ErrNotClaimable
	}
	if terminalStates[task.State] {
		return nil, ErrNotClaimable
	}
	now := time.Now()
	if task.LeaseOwner != nil && task.LeaseUntil != nil && task.LeaseUntil.After(now) {
		// An active lease (even one held by this device) blocks re-acquisition.
		return nil, ErrNotClaimable
	}

	leaseUntil := now.Add(duration)
	nextState := "assigned"
	if task.State != "queued" {
		// Re-claiming after a retry/waiting_source window keeps attempt
		// semantics consistent.
		nextState = task.State
	}

	updateQuery := `
		UPDATE transfer_tasks
		SET lease_owner = $1, lease_until = $2, state = $3, attempt = attempt + 1, version = version + 1, updated_at = NOW()
		WHERE id = $4 AND version = $5
	`

	res, err := tx.ExecContext(ctx, updateQuery, deviceID, leaseUntil, nextState, taskID, task.Version)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lease: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return nil, ErrVersionConflict
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	task.LeaseOwner = &deviceID
	task.LeaseUntil = &leaseUntil
	task.State = nextState
	task.Attempt++
	task.Version++

	return task, nil
}

// UpdateState updates the state of a transfer task with optimistic concurrency
// and transition validation. The caller must be the target device and hold a
// valid (unexpired) lease on the task; late or unclaimed updates are rejected.
func (r *Repository) UpdateState(ctx context.Context, userID, taskID, deviceID string, expectedVersion int64, state string) error {
	if !validStates[state] {
		return fmt.Errorf("invalid state: %s", state)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var currentState string
	var targetDevice string
	var leaseOwner sql.NullString
	var leaseUntil sql.NullTime
	err = tx.QueryRowContext(ctx, `
		SELECT state, target_device_id::text, lease_owner::text, lease_until
		FROM transfer_tasks
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`, taskID, userID).Scan(&currentState, &targetDevice, &leaseOwner, &leaseUntil)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get transfer task: %w", err)
	}

	if targetDevice != deviceID {
		return ErrNotClaimable
	}
	if !validTransition(currentState, state) {
		return ErrInvalidTransition
	}

	// A task can only report progress while its lease is held and unexpired.
	// Terminal transitions initiated by an authorized owner still require the
	// reporting device to be the target; a missing or stale lease is rejected
	// so that a late or never-claimed agent cannot mutate a re-assigned task.
	if !leaseOwner.Valid || leaseOwner.String != deviceID || !leaseUntil.Valid || !leaseUntil.Time.After(time.Now()) {
		return ErrNotClaimable
	}

	res, err := tx.ExecContext(ctx, `
		UPDATE transfer_tasks
		SET state = $1, version = version + 1, updated_at = NOW()
		WHERE id = $2 AND version = $3
	`, state, taskID, expectedVersion)
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

// AddSource adds a source device to a transfer task. Both the task and the
// source device must belong to the calling account, and the source device must
// be active.
func (r *Repository) AddSource(ctx context.Context, userID, taskID, sourceDeviceID string, rank int) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var taskUser string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id::text FROM transfer_tasks WHERE id = $1`, taskID).Scan(&taskUser)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get task: %w", err)
	}
	if taskUser != userID {
		return ErrNotFound
	}

	var sourceUser string
	var sourceStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id::text, status FROM devices WHERE id = $1`, sourceDeviceID).Scan(&sourceUser, &sourceStatus)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get source device: %w", err)
	}
	if sourceUser != userID || sourceStatus != "active" {
		return ErrNotFound
	}

	query := `
		INSERT INTO transfer_sources (task_id, source_device_id, user_id, rank, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())
		ON CONFLICT (task_id, source_device_id) DO UPDATE
		SET rank = $4, updated_at = NOW()
	`

	if _, err := tx.ExecContext(ctx, query, taskID, sourceDeviceID, userID, rank); err != nil {
		return fmt.Errorf("failed to add source: %w", err)
	}

	return tx.Commit()
}

// GetSources gets the source devices for a transfer task, scoped to the owning
// account.
func (r *Repository) GetSources(ctx context.Context, userID, taskID string) ([]*TransferSource, error) {
	query := `
		SELECT ts.task_id, ts.source_device_id, ts.rank, ts.last_error, ts.disabled_at, ts.created_at, ts.updated_at
		FROM transfer_sources ts
		WHERE ts.task_id = $1 AND ts.user_id = $2 AND ts.disabled_at IS NULL
		ORDER BY ts.rank ASC
	`

	rows, err := r.db.QueryContext(ctx, query, taskID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get sources: %w", err)
	}
	defer rows.Close()

	var sources []*TransferSource
	for rows.Next() {
		source := &TransferSource{}
		if err := rows.Scan(
			&source.TaskID, &source.SourceDeviceID, &source.Rank,
			&source.LastError, &source.DisabledAt,
			&source.CreatedAt, &source.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan source: %w", err)
		}
		sources = append(sources, source)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate sources: %w", err)
	}

	return sources, nil
}

func validReason(reason string) bool {
	switch reason {
	case "pull", "push", "replication", "redundancy":
		return true
	}
	return false
}

// Service handles transfer business logic.
type Service struct {
	repo *Repository
}

// NewService creates a new transfer Service.
func NewService(db *sql.DB) *Service {
	return &Service{
		repo: NewRepository(db),
	}
}

// CreateTask creates a new transfer task.
func (s *Service) CreateTask(ctx context.Context, userID, objectID, targetDeviceID, reason string, priority int) (*TransferTask, error) {
	return s.repo.CreateTask(ctx, userID, objectID, targetDeviceID, reason, priority)
}

// GetTask retrieves a transfer task.
func (s *Service) GetTask(ctx context.Context, userID, id string) (*TransferTask, error) {
	return s.repo.GetTask(ctx, userID, id)
}

// ListTasks lists transfer tasks.
func (s *Service) ListTasks(ctx context.Context, userID, state string, limit int) ([]*TransferTask, error) {
	return s.repo.ListTasks(ctx, userID, state, limit)
}

// AcquireLease acquires a lease on a transfer task.
func (s *Service) AcquireLease(ctx context.Context, userID, taskID, deviceID string, duration time.Duration) (*TransferTask, error) {
	return s.repo.AcquireLease(ctx, userID, taskID, deviceID, duration)
}

// UpdateState updates the state of a transfer task.
func (s *Service) UpdateState(ctx context.Context, userID, taskID, deviceID string, expectedVersion int64, state string) error {
	return s.repo.UpdateState(ctx, userID, taskID, deviceID, expectedVersion, state)
}

// AddSource adds a source device.
func (s *Service) AddSource(ctx context.Context, userID, taskID, sourceDeviceID string, rank int) error {
	return s.repo.AddSource(ctx, userID, taskID, sourceDeviceID, rank)
}

// GetSources gets the source devices.
func (s *Service) GetSources(ctx context.Context, userID, taskID string) ([]*TransferSource, error) {
	return s.repo.GetSources(ctx, userID, taskID)
}

// CreateReplicaTask schedules replication of one catalog file to a target.
func (s *Service) CreateReplicaTask(ctx context.Context, userID, fileID, targetDeviceID string, priority int) (*TransferTask, error) {
	return s.repo.CreateReplicaTask(ctx, userID, fileID, targetDeviceID, priority)
}

// ClaimNext leases the next executable task for an Agent.
func (s *Service) ClaimNext(ctx context.Context, userID, deviceID string, lease time.Duration) (*Assignment, error) {
	return s.repo.ClaimNext(ctx, userID, deviceID, lease)
}

// CompleteReplica atomically registers the verified target replica and closes
// its transfer task.
func (s *Service) CompleteReplica(ctx context.Context, userID, deviceID, taskID string, req CompleteRequest) error {
	return s.repo.CompleteReplica(ctx, userID, deviceID, taskID, req)
}

// FailAttempt releases an unsuccessful attempt for bounded retry.
func (s *Service) FailAttempt(ctx context.Context, userID, deviceID, taskID string, expectedVersion int64) error {
	return s.repo.FailAttempt(ctx, userID, deviceID, taskID, expectedVersion)
}

// CancelTask cancels an account-owned transfer.
func (s *Service) CancelTask(ctx context.Context, userID, taskID string) error {
	return s.repo.CancelTask(ctx, userID, taskID)
}
