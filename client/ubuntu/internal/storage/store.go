package storage

import (
	"context"
	"time"
)

// ObjectStore defines the interface for object storage operations.
type ObjectStore interface {
	// Import imports a file into the store.
	Import(ctx context.Context, sourcePath string) (*ObjectInfo, error)

	// Verify verifies an existing object by recomputing its hash.
	Verify(ctx context.Context, hash Hash) error

	// GetObject returns information about an object.
	GetObject(hash Hash) (*ObjectInfo, error)

	// DeleteObject deletes an object.
	DeleteObject(hash Hash) error

	// ListObjects lists all objects in the store.
	ListObjects() ([]ObjectInfo, error)

	// ScanAndRepair scans the storage and repairs any inconsistencies.
	ScanAndRepair(ctx context.Context) error

	// GetObjectsNeedingVerification returns objects that need verification.
	GetObjectsNeedingVerification(maxAge time.Duration) ([]ObjectInfo, error)

	// GetStaleObjects returns objects in non-ready state for too long.
	GetStaleObjects(maxAge time.Duration) ([]ObjectInfo, error)
}

// PersistentObjectStore extends ObjectStore with persistent state management.
type PersistentObjectStore interface {
	ObjectStore

	// Init initializes the store and performs startup recovery.
	Init(ctx context.Context) error

	// Close closes the store and releases resources.
	Close() error

	// GetStore returns the underlying SQLite store for advanced operations.
	GetStore() *SQLiteStore
}
