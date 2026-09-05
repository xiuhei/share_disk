package trash

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound indicates the requested file entry or trash record does not
// exist, is not in a valid state, or belongs to another account.
var ErrNotFound = errors.New("not found")

// ErrNameConflict indicates the original name is already taken during restore.
var ErrNameConflict = errors.New("name conflict")

// ErrNotTrashed indicates the entry is not in the trashed state.
var ErrNotTrashed = errors.New("not trashed")

// TrashRecord represents a trashed file entry.
type TrashRecord struct {
	ID               string    `json:"id"`
	FileEntryID      string    `json:"file_entry_id"`
	OriginalFolderID string    `json:"original_folder_id"`
	OriginalName     string    `json:"original_name"`
	DeletedAt        time.Time `json:"deleted_at"`
	PurgeAfter       time.Time `json:"purge_after"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Tombstone represents a tombstone for a deleted object.
type Tombstone struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	ObjectID   string    `json:"object_id"`
	Generation int64     `json:"generation"`
	ExpiresAt  time.Time `json:"expires_at"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// TombstoneAck represents an acknowledgment of a tombstone.
type TombstoneAck struct {
	TombstoneID string    `json:"tombstone_id"`
	DeviceID    string    `json:"device_id"`
	AckedAt     time.Time `json:"acked_at"`
	Result      string    `json:"result"`
}

// Repository handles trash database operations.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new trash Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// TrashFileEntry moves an active file entry to the trash atomically: the file
// entry is flipped to 'trashed' and a trash record is created in the same
// transaction.
func (r *Repository) TrashFileEntry(ctx context.Context, userID, fileEntryID string, retentionDays int) (*TrashRecord, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var folderID, name, status string
	err = tx.QueryRowContext(ctx, `
		SELECT folder_id, name, status
		FROM file_entries
		WHERE id = $1 AND user_id = $2
		FOR UPDATE
	`, fileEntryID, userID).Scan(&folderID, &name, &status)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get file entry: %w", err)
	}
	if status != "active" {
		return nil, ErrNotTrashed
	}

	now := time.Now().UTC()
	deletedAt := now
	purgeAfter := now.AddDate(0, 0, retentionDays)

	if _, err := tx.ExecContext(ctx, `
		UPDATE file_entries
		SET status = 'trashed', version = version + 1, updated_at = NOW()
		WHERE id = $1 AND user_id = $2
	`, fileEntryID, userID); err != nil {
		return nil, fmt.Errorf("failed to update file entry: %w", err)
	}

	// Insert-or-reuse a trash record and read back the actual persisted row.
	// On conflict (re-trash after restore) the existing row's id and created_at
	// are retained; RETURNING ensures the caller receives the true record rather
	// than the freshly generated, non-persisted values.
	record := &TrashRecord{}
	err = tx.QueryRowContext(ctx, `
		INSERT INTO trash_records (id, file_entry_id, user_id, original_folder_id, original_name, deleted_at, purge_after, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (file_entry_id) DO UPDATE SET
			original_folder_id = EXCLUDED.original_folder_id,
			original_name = EXCLUDED.original_name,
			deleted_at = EXCLUDED.deleted_at,
			purge_after = EXCLUDED.purge_after,
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at
		RETURNING id, file_entry_id, original_folder_id, original_name, deleted_at, purge_after, status, created_at, updated_at
	`, uuid.New().String(), fileEntryID, userID, folderID, name,
		deletedAt, purgeAfter, "trashed", now, now).Scan(
		&record.ID, &record.FileEntryID, &record.OriginalFolderID, &record.OriginalName,
		&record.DeletedAt, &record.PurgeAfter, &record.Status, &record.CreatedAt, &record.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create trash record: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return record, nil
}

// GetTrashRecord retrieves a trash record by file entry ID.
func (r *Repository) GetTrashRecord(ctx context.Context, userID, fileEntryID string) (*TrashRecord, error) {
	record := &TrashRecord{}
	query := `
		SELECT tr.id, tr.file_entry_id, tr.original_folder_id, tr.original_name, tr.deleted_at, tr.purge_after, tr.status, tr.created_at, tr.updated_at
		FROM trash_records tr
		JOIN file_entries fe ON tr.file_entry_id = fe.id
		WHERE tr.file_entry_id = $1 AND fe.user_id = $2 AND tr.status = 'trashed'
	`

	err := r.db.QueryRowContext(ctx, query, fileEntryID, userID).Scan(
		&record.ID, &record.FileEntryID, &record.OriginalFolderID,
		&record.OriginalName, &record.DeletedAt, &record.PurgeAfter,
		&record.Status, &record.CreatedAt, &record.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get trash record: %w", err)
	}

	return record, nil
}

// ListTrashRecords lists trash records for a user with pagination.
func (r *Repository) ListTrashRecords(ctx context.Context, userID string, limit int) ([]*TrashRecord, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	query := `
		SELECT tr.id, tr.file_entry_id, tr.original_folder_id, tr.original_name, tr.deleted_at, tr.purge_after, tr.status, tr.created_at, tr.updated_at
		FROM trash_records tr
		JOIN file_entries fe ON tr.file_entry_id = fe.id
		WHERE fe.user_id = $1 AND tr.status = 'trashed'
		ORDER BY tr.deleted_at DESC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list trash records: %w", err)
	}
	defer rows.Close()

	var records []*TrashRecord
	for rows.Next() {
		record := &TrashRecord{}
		if err := rows.Scan(
			&record.ID, &record.FileEntryID, &record.OriginalFolderID,
			&record.OriginalName, &record.DeletedAt, &record.PurgeAfter,
			&record.Status, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan trash record: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate trash records: %w", err)
	}

	return records, nil
}

// Restore restores a trashed file entry back to its original folder. It fails
// with ErrNameConflict if the original name is taken.
func (r *Repository) Restore(ctx context.Context, userID, fileEntryID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var recordID, originalFolderID, normalizedName string
	err = tx.QueryRowContext(ctx, `
		SELECT tr.id, tr.original_folder_id, fe.normalized_name
		FROM trash_records tr
		JOIN file_entries fe ON tr.file_entry_id = fe.id
		WHERE tr.file_entry_id = $1 AND fe.user_id = $2 AND tr.status = 'trashed'
		FOR UPDATE OF tr
	`, fileEntryID, userID).Scan(&recordID, &originalFolderID, &normalizedName)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get trash record: %w", err)
	}

	// Detect name conflict in the original folder.
	var conflict int
	err = tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM file_entries
		WHERE folder_id = $1 AND normalized_name = $2 AND status = 'active' AND id <> $3
	`, originalFolderID, normalizedName, fileEntryID).Scan(&conflict)
	if err != nil {
		return fmt.Errorf("failed to check name conflict: %w", err)
	}
	if conflict > 0 {
		return ErrNameConflict
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE file_entries
		SET folder_id = $1, status = 'active', version = version + 1, updated_at = NOW()
		WHERE id = $2 AND user_id = $3
	`, originalFolderID, fileEntryID, userID); err != nil {
		return fmt.Errorf("failed to update file entry: %w", err)
	}

	// Close the trash record. The schema only allows trashed/purging/purged.
	if _, err := tx.ExecContext(ctx, `
		UPDATE trash_records
		SET status = 'purged', updated_at = NOW()
		WHERE id = $1
	`, recordID); err != nil {
		return fmt.Errorf("failed to close trash record: %w", err)
	}

	return tx.Commit()
}

// Purge permanently deletes file entries that are past their purge time.
func (r *Repository) Purge(ctx context.Context, limit int) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, file_entry_id
		FROM trash_records
		WHERE status = 'trashed' AND purge_after <= NOW()
		LIMIT $1
		FOR UPDATE
	`, limit)
	if err != nil {
		return 0, fmt.Errorf("failed to get records to purge: %w", err)
	}

	type purgeTarget struct {
		ID          string
		FileEntryID string
	}
	var targets []purgeTarget
	for rows.Next() {
		var t purgeTarget
		if err := rows.Scan(&t.ID, &t.FileEntryID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("failed to scan record: %w", err)
		}
		targets = append(targets, t)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("failed to iterate records: %w", err)
	}

	for _, t := range targets {
		if _, err := tx.ExecContext(ctx, `
			UPDATE file_entries
			SET status = 'purged', version = version + 1, updated_at = NOW()
			WHERE id = $1
		`, t.FileEntryID); err != nil {
			return 0, fmt.Errorf("failed to update file entry: %w", err)
		}

		if _, err := tx.ExecContext(ctx, `
			UPDATE trash_records
			SET status = 'purged', updated_at = NOW()
			WHERE id = $1
		`, t.ID); err != nil {
			return 0, fmt.Errorf("failed to update trash record: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return int64(len(targets)), nil
}

// CreateTombstone creates a new tombstone. The object must belong to the
// calling account.
func (r *Repository) CreateTombstone(ctx context.Context, userID, objectID string, generation int64, retentionDays int) (*Tombstone, error) {
	var objectUser string
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

	now := time.Now().UTC()
	tombstone := &Tombstone{
		ID:         uuid.New().String(),
		UserID:     userID,
		ObjectID:   objectID,
		Generation: generation,
		ExpiresAt:  now.AddDate(0, 0, retentionDays),
		Status:     "active",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	query := `
		INSERT INTO tombstones (id, user_id, object_id, generation, expires_at, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err = r.db.ExecContext(ctx, query,
		tombstone.ID, tombstone.UserID, tombstone.ObjectID,
		tombstone.Generation, tombstone.ExpiresAt, tombstone.Status,
		tombstone.CreatedAt, tombstone.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create tombstone: %w", err)
	}

	return tombstone, nil
}

// AckTombstone acknowledges a tombstone. The tombstone and the acknowledging
// device must belong to the same account.
func (r *Repository) AckTombstone(ctx context.Context, userID, tombstoneID, deviceID, result string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var tombstoneUser string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id::text FROM tombstones WHERE id = $1`, tombstoneID).Scan(&tombstoneUser)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get tombstone: %w", err)
	}
	if tombstoneUser != userID {
		return ErrNotFound
	}

	var deviceUser string
	var deviceStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT user_id::text, status FROM devices WHERE id = $1`, deviceID).Scan(&deviceUser, &deviceStatus)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("failed to get device: %w", err)
	}
	if deviceUser != userID || deviceStatus != "active" {
		return ErrNotFound
	}

	query := `
		INSERT INTO tombstone_acks (tombstone_id, device_id, user_id, acked_at, result)
		VALUES ($1, $2, $3, NOW(), $4)
		ON CONFLICT (tombstone_id, device_id) DO UPDATE
		SET acked_at = NOW(), result = $4
	`

	if _, err := tx.ExecContext(ctx, query, tombstoneID, deviceID, userID, result); err != nil {
		return fmt.Errorf("failed to ack tombstone: %w", err)
	}

	return tx.Commit()
}

// Service handles trash business logic.
type Service struct {
	repo *Repository
}

// NewService creates a new trash Service.
func NewService(db *sql.DB) *Service {
	return &Service{
		repo: NewRepository(db),
	}
}

// TrashFileEntry moves a file entry to trash.
func (s *Service) TrashFileEntry(ctx context.Context, userID, fileEntryID string, retentionDays int) (*TrashRecord, error) {
	return s.repo.TrashFileEntry(ctx, userID, fileEntryID, retentionDays)
}

// RestoreFileEntry restores a file entry from trash.
func (s *Service) RestoreFileEntry(ctx context.Context, userID, fileEntryID string) error {
	return s.repo.Restore(ctx, userID, fileEntryID)
}

// ListTrashRecords lists trash records for a user.
func (s *Service) ListTrashRecords(ctx context.Context, userID string, limit int) ([]*TrashRecord, error) {
	return s.repo.ListTrashRecords(ctx, userID, limit)
}

// Purge permanently deletes file entries that are past their purge time.
func (s *Service) Purge(ctx context.Context, limit int) (int64, error) {
	return s.repo.Purge(ctx, limit)
}

// CreateTombstone creates a new tombstone.
func (s *Service) CreateTombstone(ctx context.Context, userID, objectID string, generation int64, retentionDays int) (*Tombstone, error) {
	return s.repo.CreateTombstone(ctx, userID, objectID, generation, retentionDays)
}

// AckTombstone acknowledges a tombstone.
func (s *Service) AckTombstone(ctx context.Context, userID, tombstoneID, deviceID, result string) error {
	return s.repo.AckTombstone(ctx, userID, tombstoneID, deviceID, result)
}
