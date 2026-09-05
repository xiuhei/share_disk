package identity

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Device represents a device
type Device struct {
	ID         string     `json:"id"`
	UserID     string     `json:"user_id"`
	Name       string     `json:"name"`
	Platform   string     `json:"platform"`
	PublicKey  []byte     `json:"public_key"`
	PeerID     string     `json:"peer_id"`
	Status     string     `json:"status"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	Version    int64      `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// DeviceRepository handles device database operations
type DeviceRepository struct {
	db *sql.DB
}

// NewDeviceRepository creates a new DeviceRepository
func NewDeviceRepository(db *sql.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

// Create creates a new device
func (r *DeviceRepository) Create(ctx context.Context, userID, name, platform string, publicKey []byte, peerID string) (*Device, error) {
	device := &Device{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      name,
		Platform:  platform,
		PublicKey: publicKey,
		PeerID:    peerID,
		Status:    "active",
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	query := `
		INSERT INTO devices (id, user_id, name, platform, public_key, peer_id, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := r.db.ExecContext(ctx, query,
		device.ID, device.UserID, device.Name, device.Platform,
		device.PublicKey, device.PeerID, device.Status, device.Version,
		device.CreatedAt, device.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create device: %w", err)
	}

	return device, nil
}

// CreateTx creates a new device within an existing transaction.
func (r *DeviceRepository) CreateTx(ctx context.Context, tx *sql.Tx, userID, name, platform string, publicKey []byte, peerID string) (*Device, error) {
	device := &Device{
		ID:        uuid.New().String(),
		UserID:    userID,
		Name:      name,
		Platform:  platform,
		PublicKey: publicKey,
		PeerID:    peerID,
		Status:    "active",
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	query := `
		INSERT INTO devices (id, user_id, name, platform, public_key, peer_id, status, version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := tx.ExecContext(ctx, query,
		device.ID, device.UserID, device.Name, device.Platform,
		device.PublicKey, device.PeerID, device.Status, device.Version,
		device.CreatedAt, device.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create device: %w", err)
	}

	return device, nil
}

// GetByID retrieves a device by ID
func (r *DeviceRepository) GetByID(ctx context.Context, id string) (*Device, error) {
	device := &Device{}
	query := `
		SELECT id, user_id, name, platform, public_key, peer_id, status, last_seen_at, version, created_at, updated_at
		FROM devices
		WHERE id = $1 AND status = 'active'
	`

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&device.ID, &device.UserID, &device.Name, &device.Platform,
		&device.PublicKey, &device.PeerID, &device.Status, &device.LastSeenAt,
		&device.Version, &device.CreatedAt, &device.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	return device, nil
}

// GetByPeerID retrieves a device by peer ID
func (r *DeviceRepository) GetByPeerID(ctx context.Context, peerID string) (*Device, error) {
	device := &Device{}
	query := `
		SELECT id, user_id, name, platform, public_key, peer_id, status, last_seen_at, version, created_at, updated_at
		FROM devices
		WHERE peer_id = $1 AND status = 'active'
	`

	err := r.db.QueryRowContext(ctx, query, peerID).Scan(
		&device.ID, &device.UserID, &device.Name, &device.Platform,
		&device.PublicKey, &device.PeerID, &device.Status, &device.LastSeenAt,
		&device.Version, &device.CreatedAt, &device.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get device: %w", err)
	}

	return device, nil
}

// GetByPeerIDTx retrieves a device in any lifecycle state within the caller's
// transaction. Registration uses this to safely reactivate a previously
// deregistered physical device without violating the unique peer ID.
func (r *DeviceRepository) GetByPeerIDTx(ctx context.Context, tx *sql.Tx, peerID string) (*Device, error) {
	device := &Device{}
	err := tx.QueryRowContext(ctx, `
		SELECT id, user_id, name, platform, public_key, peer_id, status, last_seen_at, version, created_at, updated_at
		FROM devices
		WHERE peer_id = $1
	`, peerID).Scan(
		&device.ID, &device.UserID, &device.Name, &device.Platform,
		&device.PublicKey, &device.PeerID, &device.Status, &device.LastSeenAt,
		&device.Version, &device.CreatedAt, &device.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get device by peer id: %w", err)
	}
	return device, nil
}

// ReactivateTx restores a same-account physical device and refreshes its
// mutable registration metadata. Existing sessions remain revoked; the caller
// creates a new independent session in the same transaction.
func (r *DeviceRepository) ReactivateTx(ctx context.Context, tx *sql.Tx, device *Device, name, platform string, publicKey []byte) error {
	err := tx.QueryRowContext(ctx, `
		UPDATE devices
		SET name=$1, platform=$2, public_key=$3, status='active',
		    version=version+1, updated_at=NOW()
		WHERE id=$4 AND user_id=$5
		RETURNING status,version,updated_at
	`, name, platform, publicKey, device.ID, device.UserID).Scan(&device.Status, &device.Version, &device.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to reactivate device: %w", err)
	}
	device.Name = name
	device.Platform = platform
	device.PublicKey = publicKey
	return nil
}

// ListByUser lists all devices for a user
func (r *DeviceRepository) ListByUser(ctx context.Context, userID string) ([]*Device, error) {
	query := `
		SELECT id, user_id, name, platform, public_key, peer_id, status, last_seen_at, version, created_at, updated_at
		FROM devices
		WHERE user_id = $1 AND status = 'active'
		ORDER BY created_at DESC
	`

	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list devices: %w", err)
	}
	defer rows.Close()

	var devices []*Device
	for rows.Next() {
		device := &Device{}
		err := rows.Scan(
			&device.ID, &device.UserID, &device.Name, &device.Platform,
			&device.PublicKey, &device.PeerID, &device.Status, &device.LastSeenAt,
			&device.Version, &device.CreatedAt, &device.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan device: %w", err)
		}
		devices = append(devices, device)
	}

	return devices, nil
}

// UpdateLastSeen updates the last seen timestamp
func (r *DeviceRepository) UpdateLastSeen(ctx context.Context, id string) error {
	query := `
		UPDATE devices
		SET last_seen_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status = 'active'
	`

	_, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to update last seen: %w", err)
	}

	return nil
}

// Deregister deregisters a device
func (r *DeviceRepository) Deregister(ctx context.Context, id string) error {
	query := `
		UPDATE devices
		SET status = 'deregistered', updated_at = NOW()
		WHERE id = $1 AND status = 'active'
	`

	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to deregister device: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("device not found")
	}

	return nil
}
