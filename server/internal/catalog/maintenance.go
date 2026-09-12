package catalog

import (
	"context"
	"database/sql"
	"fmt"
)

// PurgeExpiredTrash records logical deletion and durable Agent commands. Physical
// replica deletion remains pending until the owning Agent acknowledges it.
// Each item has a bounded transaction; an interrupted cycle can safely repeat.
func (r *Repository) PurgeExpiredTrash(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `SELECT e.user_id::text,e.id::text
		FROM trash_records t JOIN file_entries e ON e.id=t.file_entry_id AND e.user_id=t.user_id
		WHERE t.status='trashed' AND e.status='trashed' AND t.purge_after<=NOW()
		ORDER BY t.purge_after,e.id LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type candidate struct{ userID, fileID string }
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.userID, &item.fileID); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, item := range candidates {
		changed, err := r.purgeExpiredTrashItem(ctx, item.userID, item.fileID)
		if err != nil {
			return count, err
		}
		if changed {
			count++
		}
	}
	return count, nil
}

func (r *Repository) purgeExpiredTrashItem(ctx context.Context, userID, fileID string) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, userID); err != nil {
		return false, err
	}
	var deviceID, localID string
	var version int64
	// Recheck after taking the same account lock used by restore/trash. A restored
	// or newly trashed entry must not be deleted from an earlier scan result.
	err = tx.QueryRowContext(ctx, `SELECT e.origin_device_id::text,e.client_file_id,e.version
		FROM file_entries e JOIN trash_records t ON t.file_entry_id=e.id AND t.user_id=e.user_id
		WHERE e.id=$1 AND e.user_id=$2 AND e.status='trashed' AND t.status='trashed' AND t.purge_after<=NOW()
		FOR UPDATE OF e,t`, fileID, userID).Scan(&deviceID, &localID, &version)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	report := LifecycleReport{OperationID: fmt.Sprintf("retention:%s:%d", fileID, version), LocalFileID: localID, Action: "purge"}
	if _, err := applyLifecycleReportTx(ctx, tx, userID, deviceID, report, true); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}
