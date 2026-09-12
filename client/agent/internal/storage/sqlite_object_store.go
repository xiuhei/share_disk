package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SQLiteObjectStore implements PersistentObjectStore using SQLite for state management.
type SQLiteObjectStore struct {
	storage   *Storage
	store     *SQLiteStore
	chunkSize int64
}

// NewSQLiteObjectStore creates a new SQLiteObjectStore with the given chunk size.
// A chunkSize of 0 or negative uses DefaultChunkSize.
func NewSQLiteObjectStore(storage *Storage, db *sql.DB, chunkSize int64) *SQLiteObjectStore {
	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}
	return &SQLiteObjectStore{
		storage:   storage,
		store:     NewSQLiteStore(db),
		chunkSize: chunkSize,
	}
}

// Init initializes the store and performs startup recovery.
func (s *SQLiteObjectStore) Init(ctx context.Context) error {
	if err := s.storage.EnsureDirectories(); err != nil {
		return fmt.Errorf("failed to ensure directories: %w", err)
	}
	return s.startupRecovery(ctx)
}

// Close closes the store and releases resources.
func (s *SQLiteObjectStore) Close() error {
	return nil
}

// GetStore returns the underlying SQLite store.
func (s *SQLiteObjectStore) GetStore() *SQLiteStore {
	return s.store
}

// startupRecovery reconciles SQLite state with the filesystem after a crash.
// It scans forward (files -> records) and then backward (records -> files) so
// that a READY record whose backing file disappeared is downgraded rather than
// left permanently claiming READY.
func (s *SQLiteObjectStore) startupRecovery(ctx context.Context) error {
	storedObjects, err := s.store.ListObjects()
	if err != nil {
		return fmt.Errorf("failed to list stored objects: %w", err)
	}

	storedMap := make(map[string]*ObjectState)
	for i := range storedObjects {
		storedMap[storedObjects[i].ID] = &storedObjects[i]
	}

	// seen records the object IDs that have a matching file on disk.
	seen := make(map[string]bool)

	objectsDir := s.storage.ObjectsDir()
	err = filepath.Walk(objectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		hashStr := filepath.Base(path)
		hash, err := ParseHash(hashStr)
		if err != nil {
			return nil
		}

		storedObj, exists := storedMap[hash.String()]
		if !exists {
			// File without a record: leftover from a crashed import before the
			// SQLite commit. Quarantine it; never auto-mark it READY.
			if err := s.store.UpsertObject(hash, info.Size(), path, ObjectStatusQuarantined); err != nil {
				return fmt.Errorf("failed to quarantine orphan object %s: %w", hashStr, err)
			}
			return nil
		}

		seen[hash.String()] = true

		// Record exists and file exists. If the record was mid-import (not
		// READY), verify the content and promote it to READY, since the file
		// was already fully streamed and hashed before the rename.
		if storedObj.Status == ObjectStatusReady {
			if err := s.Verify(ctx, hash); err != nil {
				if uerr := s.store.UpdateObjectStatus(hash, ObjectStatusCorrupt); uerr != nil {
					return fmt.Errorf("failed to mark corrupt object %s: %w", hashStr, uerr)
				}
			}
			return nil
		}

		if err := s.Verify(ctx, hash); err != nil {
			if uerr := s.store.UpdateObjectStatus(hash, ObjectStatusCorrupt); uerr != nil {
				return fmt.Errorf("failed to mark corrupt object %s: %w", hashStr, uerr)
			}
			return nil
		}
		if err := s.store.UpdateObjectStatus(hash, ObjectStatusReady); err != nil {
			return fmt.Errorf("failed to promote object %s to ready: %w", hashStr, err)
		}
		if err := s.store.UpdateObjectVerification(hash); err != nil {
			return fmt.Errorf("failed to update verification for %s: %w", hashStr, err)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan objects directory: %w", err)
	}

	// Reverse check: any stored record whose file is missing must be downgraded
	// so SQLite never claims READY for an object that no longer exists on disk.
	for i := range storedObjects {
		obj := &storedObjects[i]
		if seen[obj.ID] {
			continue
		}
		hash, err := ParseHash(obj.ID)
		if err != nil {
			continue
		}
		if _, err := os.Stat(s.storage.ObjectPath(hash)); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat object %s: %w", obj.ID, err)
		}

		if obj.Status == ObjectStatusReady {
			if err := s.store.UpdateObjectStatus(hash, ObjectStatusMissing); err != nil {
				return fmt.Errorf("failed to mark missing object %s: %w", obj.ID, err)
			}
		}
	}

	// Remove stale non-ready records whose file no longer exists.
	return s.cleanupStaleObjects(ctx)
}

// cleanupStaleObjects removes non-ready records that have no backing file.
func (s *SQLiteObjectStore) cleanupStaleObjects(ctx context.Context) error {
	staleObjects, err := s.store.GetStaleObjects(1 * time.Hour)
	if err != nil {
		return fmt.Errorf("failed to get stale objects: %w", err)
	}

	for _, obj := range staleObjects {
		hash, err := ParseHash(obj.ID)
		if err != nil {
			continue
		}

		objectPath := s.storage.ObjectPath(hash)
		if _, err := os.Stat(objectPath); os.IsNotExist(err) {
			if err := s.store.DeleteObject(hash); err != nil {
				return fmt.Errorf("failed to delete stale object %s: %w", obj.ID, err)
			}
		}
	}

	return nil
}

// Import imports a file into the store using the persistent state machine:
//
//	stage -> IMPORTING record -> rename -> READY record
//
// It is idempotent and never deletes an already-valid object.
func (s *SQLiteObjectStore) Import(ctx context.Context, sourcePath string) (*ObjectInfo, error) {
	om := NewObjectManagerWithChunkSize(s.storage, s.chunkSize)

	staged, err := om.Stage(ctx, sourcePath)
	if err != nil {
		return nil, err
	}
	defer staged.Discard()

	hash := staged.Hash()

	// Idempotency: if the object already exists and is READY, verify the
	// persisted file (existence, regularity, hash) before trusting the record.
	// A missing or corrupt object falls through and is re-imported.
	existing, err := s.store.LookupObject(hash)
	if err != nil {
		return nil, err
	}
	if existing != nil && existing.Status == ObjectStatusReady && existing.Size == staged.Size() {
		if err := s.verifyPersistedObject(hash); err == nil {
			if err := s.finalizeStaged(staged); err != nil {
				return nil, err
			}
			return s.objectInfo(staged), nil
		}
	}

	// Record IMPORTING before the rename so a crash in the window between the
	// rename and the READY commit is recoverable.
	if err := s.store.UpsertObject(hash, staged.Size(), staged.ObjectPath(), ObjectStatusImporting); err != nil {
		return nil, fmt.Errorf("failed to record object in SQLite: %w", err)
	}

	if err := staged.Commit(); err != nil {
		return nil, err
	}

	if err := s.finalizeStaged(staged); err != nil {
		return nil, err
	}

	return s.objectInfo(staged), nil
}

func (s *SQLiteObjectStore) finalizeStaged(staged *StagedImport) error {
	manifest, err := staged.Manifest()
	if err != nil {
		return fmt.Errorf("failed to create canonical manifest: %w", err)
	}
	digest, err := manifest.Digest()
	if err != nil {
		return fmt.Errorf("failed to digest canonical manifest: %w", err)
	}
	if err := s.store.FinalizeObject(staged.Hash(), staged.Chunks(), digest); err != nil {
		return fmt.Errorf("failed to persist canonical manifest: %w", err)
	}
	return nil
}

func (s *SQLiteObjectStore) objectInfo(staged *StagedImport) *ObjectInfo {
	return &ObjectInfo{
		Hash:       staged.Hash(),
		Size:       staged.Size(),
		ChunkSize:  staged.ChunkSize(),
		ChunkCount: len(staged.Chunks()),
		Chunks:     staged.Chunks(),
		Status:     ObjectStatusReady,
		Path:       staged.ObjectPath(),
	}
}

// objectInfoFromState builds an ObjectInfo from a SQLite record, using the
// persisted status and the canonical object path derived from the hash.
// Chunk manifest is loaded from SQLite if available; objects imported before
// chunks were tracked will have an empty manifest.
func (s *SQLiteObjectStore) objectInfoFromState(obj *ObjectState) (*ObjectInfo, error) {
	hash, err := ParseHash(obj.ID)
	if err != nil {
		return nil, fmt.Errorf("invalid object id %q: %w", obj.ID, err)
	}

	chunks, err := s.store.LoadChunks(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to load chunks for %s: %w", obj.ID, err)
	}

	info := &ObjectInfo{
		Hash:   hash,
		Size:   obj.Size,
		Status: obj.Status,
		Path:   s.storage.ObjectPath(hash),
	}
	if len(chunks) > 0 {
		info.ChunkSize = s.chunkSize
		info.ChunkCount = len(chunks)
		info.Chunks = chunks
	}
	return info, nil
}

// verifyPersistedObject checks that the persisted file backing a READY record
// still exists, is a regular file, and hashes correctly. On failure it updates
// the SQLite status (MISSING for a missing file, CORRUPT for a hash mismatch)
// and returns an error.
func (s *SQLiteObjectStore) verifyPersistedObject(hash Hash) error {
	path := s.storage.ObjectPath(hash)
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			_ = s.store.UpdateObjectStatus(hash, ObjectStatusMissing)
			return fmt.Errorf("object file missing: %s", hash.String())
		}
		return fmt.Errorf("failed to stat object: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("object is a symlink: %s", hash.String())
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("object is not a regular file: %s", hash.String())
	}

	actualHash, err := HashFile(path)
	if err != nil {
		return fmt.Errorf("failed to hash object: %w", err)
	}
	if actualHash != hash {
		_ = s.store.UpdateObjectStatus(hash, ObjectStatusCorrupt)
		return fmt.Errorf("hash mismatch: expected %s, got %s", hash.String(), actualHash.String())
	}

	return nil
}

// Verify verifies an existing object by recomputing its hash.
func (s *SQLiteObjectStore) Verify(ctx context.Context, hash Hash) error {
	om := NewObjectManagerWithChunkSize(s.storage, s.chunkSize)

	if err := om.Verify(ctx, hash); err != nil {
		if uerr := s.store.UpdateObjectStatus(hash, ObjectStatusCorrupt); uerr != nil {
			return fmt.Errorf("verification failed (%v) and status update failed: %w", err, uerr)
		}
		return err
	}

	return s.store.UpdateObjectVerification(hash)
}

// GetObject returns information about an object from SQLite, which is the
// authoritative local source of truth. It never infers READY from file
// existence alone.
func (s *SQLiteObjectStore) GetObject(hash Hash) (*ObjectInfo, error) {
	obj, err := s.store.LookupObject(hash)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("object not found: %s", hash.String())
	}
	return s.objectInfoFromState(obj)
}

// DeleteObject deletes an object, transitioning through DELETING so that a
// crash between file removal and record removal is recoverable.
func (s *SQLiteObjectStore) DeleteObject(hash Hash) error {
	if err := s.store.UpdateObjectStatus(hash, ObjectStatusDeleting); err != nil {
		return err
	}

	om := NewObjectManagerWithChunkSize(s.storage, s.chunkSize)
	if err := om.DeleteObject(hash); err != nil {
		return err
	}

	return s.store.DeleteObject(hash)
}

// PurgeLANFile permanently removes a trashed logical file. Content bytes are
// reclaimed only when this is the final logical reference to the object.
func (s *SQLiteObjectStore) PurgeLANFile(ctx context.Context, id, ownerUserID string) error {
	return s.purgeLANFile(ctx, id, ownerUserID, false)
}

// PurgeLANFileCoordinated keeps the server report blocked until the local
// filesystem/SQLite purge saga has completed.
func (s *SQLiteObjectStore) PurgeLANFileCoordinated(ctx context.Context, id, ownerUserID string) error {
	return s.purgeLANFile(ctx, id, ownerUserID, true)
}

func (s *SQLiteObjectStore) purgeLANFile(ctx context.Context, id, ownerUserID string, coordinated bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var file *LANFile
	var lastReference bool
	var opID string
	var err error
	if coordinated {
		file, lastReference, opID, err = s.store.BeginPurgeLANFileCoordinated(ctx, id, ownerUserID)
	} else {
		file, lastReference, err = s.store.BeginPurgeLANFile(id, ownerUserID)
	}
	if err != nil {
		return err
	}
	if !lastReference {
		err = s.store.FinalizePurgeLANFile(id, ownerUserID)
	} else {
		hash, parseErr := ParseHash(file.ObjectID)
		if parseErr != nil {
			return fmt.Errorf("invalid object id for purge: %w", parseErr)
		}
		err = s.DeleteObject(hash)
	}
	if err != nil {
		return err
	}
	if coordinated {
		return s.store.ActivateOutgoing(ctx, opID)
	}
	return nil
}

// PurgeExpiredLANFiles applies the same crash-recoverable permanent-delete path
// to every record whose retention deadline elapsed.
func (s *SQLiteObjectStore) PurgeExpiredLANFiles(ctx context.Context, now time.Time) error {
	return s.purgeExpiredLANFiles(ctx, now, false)
}

func (s *SQLiteObjectStore) PurgeExpiredLANFilesCoordinated(ctx context.Context, now time.Time) error {
	return s.purgeExpiredLANFiles(ctx, now, true)
}

func (s *SQLiteObjectStore) purgeExpiredLANFiles(ctx context.Context, now time.Time, coordinated bool) error {
	targets, err := s.store.ListExpiredLANTrash(now)
	if err != nil {
		return err
	}
	var purgeErrors []error
	for _, target := range targets {
		var err error
		if coordinated {
			err = s.PurgeLANFileCoordinated(ctx, target.ID, target.OwnerUserID)
		} else {
			err = s.PurgeLANFile(ctx, target.ID, target.OwnerUserID)
		}
		if err != nil {
			purgeErrors = append(purgeErrors, fmt.Errorf("purge %s: %w", target.ID, err))
		}
	}
	return errors.Join(purgeErrors...)
}

// ListObjects lists all objects in the store from SQLite state.
func (s *SQLiteObjectStore) ListObjects() ([]ObjectInfo, error) {
	stored, err := s.store.ListObjects()
	if err != nil {
		return nil, err
	}

	objects := make([]ObjectInfo, 0, len(stored))
	for i := range stored {
		info, err := s.objectInfoFromState(&stored[i])
		if err != nil {
			continue
		}
		objects = append(objects, *info)
	}

	return objects, nil
}

// ScanAndRepair scans the storage and repairs any inconsistencies.
func (s *SQLiteObjectStore) ScanAndRepair(ctx context.Context) error {
	return s.startupRecovery(ctx)
}

// GetObjectsNeedingVerification returns objects that need verification.
func (s *SQLiteObjectStore) GetObjectsNeedingVerification(maxAge time.Duration) ([]ObjectInfo, error) {
	storedObjects, err := s.store.GetObjectsNeedingVerification(maxAge)
	if err != nil {
		return nil, err
	}

	var objects []ObjectInfo
	for _, obj := range storedObjects {
		hash, err := ParseHash(obj.ID)
		if err != nil {
			continue
		}
		objects = append(objects, ObjectInfo{
			Hash:   hash,
			Size:   obj.Size,
			Status: obj.Status,
			Path:   obj.RelativePath,
		})
	}

	return objects, nil
}

// GetStaleObjects returns objects in non-ready state for too long.
func (s *SQLiteObjectStore) GetStaleObjects(maxAge time.Duration) ([]ObjectInfo, error) {
	storedObjects, err := s.store.GetStaleObjects(maxAge)
	if err != nil {
		return nil, err
	}

	var objects []ObjectInfo
	for _, obj := range storedObjects {
		hash, err := ParseHash(obj.ID)
		if err != nil {
			continue
		}
		objects = append(objects, ObjectInfo{
			Hash:   hash,
			Size:   obj.Size,
			Status: obj.Status,
			Path:   obj.RelativePath,
		})
	}

	return objects, nil
}
