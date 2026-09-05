package identity

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// User represents a user account
type User struct {
	ID           string    `json:"id"`
	Account      string    `json:"account"`
	PasswordHash string    `json:"-"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserRepository handles user database operations
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository creates a new UserRepository
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create creates a new user
func (r *UserRepository) Create(ctx context.Context, account, passwordHash string) (*User, error) {
	user := &User{
		ID:           uuid.New().String(),
		Account:      account,
		PasswordHash: passwordHash,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	query := `
		INSERT INTO users (id, account, password_hash, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := r.db.ExecContext(ctx, query,
		user.ID, user.Account, user.PasswordHash, user.Status, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// CreateTx creates a new user within an existing transaction.
func (r *UserRepository) CreateTx(ctx context.Context, tx *sql.Tx, account, passwordHash string) (*User, error) {
	user := &User{
		ID:           uuid.New().String(),
		Account:      account,
		PasswordHash: passwordHash,
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	query := `
		INSERT INTO users (id, account, password_hash, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := tx.ExecContext(ctx, query,
		user.ID, user.Account, user.PasswordHash, user.Status, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetByAccount retrieves a user by account name
func (r *UserRepository) GetByAccount(ctx context.Context, account string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, account, password_hash, status, created_at, updated_at
		FROM users
		WHERE LOWER(account) = LOWER($1) AND status = 'active'
	`

	err := r.db.QueryRowContext(ctx, query, account).Scan(
		&user.ID, &user.Account, &user.PasswordHash, &user.Status,
		&user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// GetByID retrieves a user by ID
func (r *UserRepository) GetByID(ctx context.Context, id string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, account, password_hash, status, created_at, updated_at
		FROM users
		WHERE id = $1 AND status = 'active'
	`

	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&user.ID, &user.Account, &user.PasswordHash, &user.Status,
		&user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// HasAnyUsers checks if any users have ever existed. It counts users in any
// status (including suspended/deleted) so that bootstrap remains one-time even
// if every account is later deactivated: the installation is already claimed.
func (r *UserRepository) HasAnyUsers(ctx context.Context) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM users`

	err := r.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check users: %w", err)
	}

	return count > 0, nil
}

// HasAnyUsersTx checks if any users have ever existed within a transaction.
func (r *UserRepository) HasAnyUsersTx(ctx context.Context, tx *sql.Tx) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM users`

	err := tx.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check users: %w", err)
	}

	return count > 0, nil
}
