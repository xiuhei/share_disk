package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrFolderNotFound = errors.New("folder not found")
	ErrFolderNotEmpty = errors.New("folder not empty")
)

type FileQuery struct {
	Trash    bool
	FolderID string
	Search   string
	Sort     string
}

// QueryUnifiedFiles provides folder filtering, case-insensitive search and a
// fixed allowlist of sort orders for all clients.
func (r *Repository) QueryUnifiedFiles(ctx context.Context, userID string, query FileQuery) ([]UnifiedFile, error) {
	status := "active"
	if query.Trash {
		status = "trashed"
	}
	search := strings.TrimSpace(query.Search)
	order := "e.normalized_name ASC,e.id"
	switch query.Sort {
	case "name_desc":
		order = "e.normalized_name DESC,e.id"
	case "newest":
		order = "e.created_at DESC,e.id"
	case "oldest":
		order = "e.created_at ASC,e.id"
	case "size_desc":
		order = "o.size DESC,e.normalized_name,e.id"
	case "size_asc":
		order = "o.size ASC,e.normalized_name,e.id"
	}
	rows, err := r.db.QueryContext(ctx, unifiedFileSelect+`
		WHERE e.user_id=$1 AND e.status=$2
		  AND ($3='' OR e.folder_id::text=$3)
		  AND ($4='' OR e.name ILIKE '%' || $4 || '%')
		ORDER BY `+order, userID, status, query.FolderID, search)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := make([]UnifiedFile, 0)
	for rows.Next() {
		file, err := scanUnifiedFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, *file)
	}
	return files, rows.Err()
}

func (r *Repository) GetUnifiedFile(ctx context.Context, userID, fileID string) (*UnifiedFile, error) {
	file, err := getUnifiedFile(ctx, r.db, userID, fileID)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, ErrFileNotFound
	}
	return file, nil
}

func (r *Repository) ListAllFolders(ctx context.Context, userID string) ([]Folder, error) {
	if _, err := ensureRootFolderDB(ctx, r.db, userID); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id::text,parent_id::text,name,status,version,created_at,updated_at FROM folders WHERE user_id=$1 AND status='active' ORDER BY parent_id NULLS FIRST,normalized_name,id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Folder
	for rows.Next() {
		var folder Folder
		if err := rows.Scan(&folder.ID, &folder.ParentID, &folder.Name, &folder.Status, &folder.Version, &folder.CreatedAt, &folder.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, folder)
	}
	return result, rows.Err()
}

func (r *Repository) CreateManagedFolder(ctx context.Context, userID, parentID, name string) (*Folder, error) {
	name = strings.TrimSpace(name)
	normalized, err := normalizeName(name)
	if err != nil || len(name) > 255 {
		return nil, fmt.Errorf("%w: folder name", ErrInvalidRequest)
	}
	if parentID == "" {
		parentID, err = ensureRootFolderDB(ctx, r.db, userID)
		if err != nil {
			return nil, err
		}
	}
	var parentOK bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM folders WHERE id=$1 AND user_id=$2 AND status='active')`, parentID, userID).Scan(&parentOK); err != nil || !parentOK {
		return nil, ErrFolderNotFound
	}
	folder := &Folder{ID: uuid.NewString(), UserID: userID, ParentID: &parentID, Name: name, NormalizedName: normalized, Status: "active", Version: 1}
	err = r.db.QueryRowContext(ctx, `INSERT INTO folders(id,user_id,parent_id,name,normalized_name,status) VALUES($1,$2,$3,$4,$5,'active') RETURNING created_at,updated_at`, folder.ID, userID, parentID, name, normalized).Scan(&folder.CreatedAt, &folder.UpdatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrNameConflict
		}
		return nil, err
	}
	return folder, nil
}

func (r *Repository) RenameManagedFolder(ctx context.Context, userID, folderID, name string) (*Folder, error) {
	name = strings.TrimSpace(name)
	normalized, err := normalizeName(name)
	if err != nil || len(name) > 255 {
		return nil, fmt.Errorf("%w: folder name", ErrInvalidRequest)
	}
	folder := &Folder{}
	err = r.db.QueryRowContext(ctx, `UPDATE folders SET name=$1,normalized_name=$2,version=version+1,updated_at=NOW() WHERE id=$3 AND user_id=$4 AND status='active' AND parent_id IS NOT NULL RETURNING id::text,user_id::text,parent_id::text,name,normalized_name,status,version,created_at,updated_at`, name, normalized, folderID, userID).Scan(&folder.ID, &folder.UserID, &folder.ParentID, &folder.Name, &folder.NormalizedName, &folder.Status, &folder.Version, &folder.CreatedAt, &folder.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrFolderNotFound
	}
	if isUniqueViolation(err) {
		return nil, ErrNameConflict
	}
	return folder, err
}

func (r *Repository) DeleteManagedFolder(ctx context.Context, userID, folderID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE folders f SET status='deleted',version=version+1,updated_at=NOW() WHERE f.id=$1 AND f.user_id=$2 AND f.status='active' AND f.parent_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM folders c WHERE c.parent_id=f.id AND c.status='active') AND NOT EXISTS(SELECT 1 FROM file_entries e WHERE e.folder_id=f.id AND e.status<>'purged')`, folderID, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 1 {
		return nil
	}
	var exists bool
	if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM folders WHERE id=$1 AND user_id=$2 AND status='active')`, folderID, userID).Scan(&exists); err != nil {
		return err
	}
	if exists {
		return ErrFolderNotEmpty
	}
	return ErrFolderNotFound
}

func (r *Repository) MoveFiles(ctx context.Context, userID, folderID string, fileIDs []string, expectedVersions ...map[string]int64) error {
	if len(fileIDs) == 0 || len(fileIDs) > 500 {
		return fmt.Errorf("%w: file_ids", ErrInvalidRequest)
	}
	if _, err := uuid.Parse(folderID); err != nil {
		return fmt.Errorf("%w: folder_id", ErrInvalidRequest)
	}
	for _, id := range fileIDs {
		if _, err := uuid.Parse(id); err != nil {
			return fmt.Errorf("%w: file_ids", ErrInvalidRequest)
		}
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, userID); err != nil {
		return err
	}
	var target string
	if err := tx.QueryRowContext(ctx, `SELECT id FROM folders WHERE id=$1 AND user_id=$2 AND status='active' FOR SHARE`, folderID, userID).Scan(&target); err != nil {
		if err == sql.ErrNoRows {
			return ErrFolderNotFound
		}
		return err
	}
	seen := make(map[string]bool)
	for _, id := range fileIDs {
		if seen[id] {
			return fmt.Errorf("%w: duplicate file id", ErrInvalidRequest)
		}
		seen[id] = true
		var version int64
		var oldFolder string
		if err := tx.QueryRowContext(ctx, `SELECT version,folder_id FROM file_entries WHERE id=$1 AND user_id=$2 AND status='active' FOR UPDATE`, id, userID).Scan(&version, &oldFolder); err != nil {
			if err == sql.ErrNoRows {
				return ErrFileNotFound
			}
			return err
		}
		if len(expectedVersions) > 0 {
			if expected, ok := expectedVersions[0][id]; ok && expected != version {
				return ErrVersionConflict
			}
		}
		if oldFolder == folderID {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE file_entries SET folder_id=$1,version=version+1,updated_at=NOW() WHERE id=$2 AND user_id=$3`, folderID, id, userID); err != nil {
			if isUniqueViolation(err) {
				return ErrNameConflict
			}
			return err
		}
		if err := appendEvent(ctx, tx, userID, "file.move", id, version+1); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func ensureRootFolderDB(ctx context.Context, db *sql.DB, userID string) (string, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	id, err := ensureRootFolder(ctx, tx, userID)
	if err != nil {
		return "", err
	}
	return id, tx.Commit()
}
