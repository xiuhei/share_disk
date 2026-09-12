package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
)

var ErrVersionConflict = errors.New("catalog version conflict")
var ErrIdempotencyConflict = errors.New("operation id reused with different input")

type ActionsRequest struct {
	OperationID      string           `json:"operation_id"`
	FileIDs          []string         `json:"file_ids"`
	Action           string           `json:"action"`
	Name             string           `json:"name,omitempty"`
	ExpectedVersions map[string]int64 `json:"expected_versions,omitempty"`
}

// ApplyActions commits the entire batch, its event records, and every device
// command together. A retry returns the saved result even after versions change.
func (r *Repository) ApplyActions(ctx context.Context, userID string, req ActionsRequest) ([]*UnifiedFile, error) {
	if req.OperationID == "" || len(req.OperationID) > 90 || len(req.FileIDs) == 0 || len(req.FileIDs) > 500 ||
		(req.Action != "rename" && req.Action != "trash" && req.Action != "restore" && req.Action != "purge") || (req.Action == "rename" && len(req.FileIDs) != 1) {
		return nil, fmt.Errorf("%w: invalid file actions", ErrInvalidRequest)
	}
	seen := make(map[string]bool)
	for _, id := range req.FileIDs {
		if _, err := uuid.Parse(id); err != nil || seen[id] {
			return nil, fmt.Errorf("%w: duplicate or invalid file id", ErrInvalidRequest)
		}
		seen[id] = true
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(payload)
	key := "control-actions:" + req.OperationID
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, userID); err != nil {
		return nil, err
	}
	var previousHash, previousBody []byte
	err = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_keys WHERE user_id=$1 AND key=$2 AND status='completed'`, userID, key).Scan(&previousHash, &previousBody)
	if err == nil {
		if !equalBytes(previousHash, digest[:]) {
			return nil, ErrIdempotencyConflict
		}
		var files []*UnifiedFile
		if err := json.Unmarshal(previousBody, &files); err != nil {
			return nil, err
		}
		return files, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	files := make([]*UnifiedFile, 0, len(req.FileIDs))
	for i, id := range req.FileIDs {
		// Row locks fence other metadata writers as well as lifecycle reports.
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT version FROM file_entries WHERE id=$1 AND user_id=$2 FOR UPDATE`, id, userID).Scan(&version); err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrFileNotFound
			}
			return nil, err
		}
		if expected, ok := req.ExpectedVersions[id]; ok && expected != version {
			return nil, ErrVersionConflict
		}
		file, err := getUnifiedFile(ctx, tx, userID, id)
		if err != nil {
			return nil, err
		}
		if file == nil {
			return nil, ErrFileNotFound
		}
		report := LifecycleReport{OperationID: "control:" + req.OperationID + ":" + strconv.Itoa(i), LocalFileID: file.LocalFileID, Action: req.Action, Name: req.Name}
		updated, err := applyLifecycleReportTx(ctx, tx, userID, file.OriginDeviceID, report, true)
		if err != nil {
			return nil, err
		}
		files = append(files, updated)
	}
	response, err := json.Marshal(files)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO idempotency_keys(user_id,key,request_hash,status,response_code,response_body,expires_at) VALUES($1,$2,$3,'completed',200,$4::jsonb,NOW()+INTERVAL '30 days')`, userID, key, digest[:], string(response)); err != nil {
		return nil, err
	}
	return files, tx.Commit()
}
