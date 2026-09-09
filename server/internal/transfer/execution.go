package transfer

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNoSource       = errors.New("no ready transfer source")
	ErrAlreadyReplica = errors.New("target already has a ready replica")
)

// Assignment is the complete, account-scoped instruction required by a target
// Agent. Source credentials are never stored in the task: the target uses its
// own account token against the source Agent.
type Assignment struct {
	TaskID            string    `json:"task_id"`
	ObjectID          string    `json:"object_id"`
	TargetDeviceID    string    `json:"target_device_id"`
	SourceDeviceID    string    `json:"source_device_id"`
	SourceEndpoint    string    `json:"source_endpoint"`
	SourceLocalFileID string    `json:"source_local_file_id"`
	TargetLocalFileID string    `json:"target_local_file_id"`
	Name              string    `json:"name"`
	MIME              string    `json:"mime"`
	Size              int64     `json:"size"`
	SHA256            string    `json:"sha256"`
	Reason            string    `json:"reason"`
	Attempt           int       `json:"attempt"`
	Version           int64     `json:"version"`
	LeaseUntil        time.Time `json:"lease_until"`
}

type CompleteRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	LocalFileID     string `json:"local_file_id"`
	Endpoint        string `json:"endpoint"`
	Size            int64  `json:"size"`
	SHA256          string `json:"sha256"`
}

// CreateReplicaTask creates one live task per object/target and snapshots all
// currently usable sources. Repeating the request returns the existing task.
func (r *Repository) CreateReplicaTask(ctx context.Context, userID, fileID, targetDeviceID string, priority int) (*TransferTask, error) {
	if _, err := uuid.Parse(fileID); err != nil {
		return nil, ErrNotFound
	}
	if _, err := uuid.Parse(targetDeviceID); err != nil {
		return nil, ErrNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var objectID string
	err = tx.QueryRowContext(ctx, `
		SELECT e.object_id::text
		FROM file_entries e
		JOIN devices d ON d.id=$3 AND d.user_id=e.user_id AND d.status='active'
			AND d.last_seen_at > NOW() - INTERVAL '2 minutes'
		WHERE e.id=$1 AND e.user_id=$2 AND e.status='active'
	`, fileID, userID, targetDeviceID).Scan(&objectID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var ready bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM replicas WHERE user_id=$1 AND object_id=$2 AND device_id=$3 AND state='ready')`, userID, objectID, targetDeviceID).Scan(&ready); err != nil {
		return nil, err
	}
	if ready {
		return nil, ErrAlreadyReplica
	}

	task := &TransferTask{ID: uuid.NewString(), UserID: userID, ObjectID: objectID, TargetDeviceID: targetDeviceID, Reason: "replication", State: "queued", Priority: priority, Version: 1, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	insert, err := tx.ExecContext(ctx, `INSERT INTO transfer_tasks(id,user_id,object_id,target_device_id,reason,state,priority,attempt,version,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,0,1,$8,$8) ON CONFLICT DO NOTHING`, task.ID, userID, objectID, targetDeviceID, task.Reason, task.State, priority, task.CreatedAt)
	if err != nil {
		return nil, err
	}
	inserted, err := insert.RowsAffected()
	if err != nil {
		return nil, err
	}
	if inserted == 0 {
		existing, getErr := scanTask(tx.QueryRowContext(ctx, `SELECT `+taskColumns+` FROM transfer_tasks WHERE user_id=$1 AND object_id=$2 AND target_device_id=$3 AND state NOT IN ('completed','canceled','failed_permanent')`, userID, objectID, targetDeviceID))
		if getErr != nil {
			return nil, getErr
		}
		return existing, tx.Commit()
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO transfer_sources(task_id,source_device_id,user_id,rank)
		SELECT $1,r.device_id,r.user_id,
				row_number() OVER (ORDER BY d.last_seen_at DESC NULLS LAST,r.updated_at DESC)::integer
		FROM replicas r
		JOIN devices d ON d.id=r.device_id AND d.user_id=r.user_id AND d.status='active'
			AND d.last_seen_at > NOW() - INTERVAL '2 minutes'
		WHERE r.user_id=$2 AND r.object_id=$3 AND r.device_id<>$4
		  AND r.state='ready' AND r.endpoint IS NOT NULL AND r.client_file_id IS NOT NULL
	`, task.ID, userID, objectID, targetDeviceID)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrNoSource
	}
	return task, tx.Commit()
}

// ClaimNext leases the highest-priority ready task to its target device.
func (r *Repository) ClaimNext(ctx context.Context, userID, deviceID string, lease time.Duration) (*Assignment, error) {
	if lease < minLeaseDuration || lease > maxLeaseDuration {
		return nil, fmt.Errorf("lease duration must be between %s and %s", minLeaseDuration, maxLeaseDuration)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var taskID string
	err = tx.QueryRowContext(ctx, `
		SELECT t.id::text FROM transfer_tasks t
		WHERE t.user_id=$1 AND t.target_device_id=$2
		  AND t.state IN ('queued','assigned','discovering','connecting','transferring','verifying','retry_wait','waiting_source')
		  AND (t.lease_until IS NULL OR t.lease_until<=NOW())
		  AND EXISTS(SELECT 1 FROM replicas r JOIN devices d ON d.id=r.device_id AND d.user_id=r.user_id AND d.status='active' AND d.last_seen_at > NOW() - INTERVAL '2 minutes' WHERE r.user_id=t.user_id AND r.object_id=t.object_id AND r.device_id<>t.target_device_id AND r.state='ready' AND r.endpoint IS NOT NULL AND r.client_file_id IS NOT NULL)
		ORDER BY t.priority DESC,t.created_at,t.id
		FOR UPDATE OF t SKIP LOCKED LIMIT 1
	`, userID, deviceID).Scan(&taskID)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO transfer_sources(task_id,source_device_id,user_id,rank)
		SELECT $1,r.device_id,r.user_id,row_number() OVER (ORDER BY d.last_seen_at DESC NULLS LAST,r.updated_at DESC)::integer
		FROM transfer_tasks t
		JOIN replicas r ON r.user_id=t.user_id AND r.object_id=t.object_id AND r.device_id<>t.target_device_id
		JOIN devices d ON d.id=r.device_id AND d.user_id=r.user_id AND d.status='active'
			AND d.last_seen_at > NOW() - INTERVAL '2 minutes'
		WHERE t.id=$1 AND r.state='ready' AND r.endpoint IS NOT NULL AND r.client_file_id IS NOT NULL
		ON CONFLICT(task_id,source_device_id) DO UPDATE SET rank=EXCLUDED.rank,disabled_at=NULL,updated_at=NOW()
	`, taskID); err != nil {
		return nil, err
	}
	leaseUntil := time.Now().UTC().Add(lease)
	if _, err := tx.ExecContext(ctx, `UPDATE transfer_tasks SET state='assigned',lease_owner=$1,lease_until=$2,attempt=attempt+1,version=version+1,updated_at=NOW() WHERE id=$3`, deviceID, leaseUntil, taskID); err != nil {
		return nil, err
	}
	a := &Assignment{}
	err = tx.QueryRowContext(ctx, `
		SELECT t.id::text,t.object_id::text,t.target_device_id::text,
			s.source_device_id::text,r.endpoint,r.client_file_id,t.id::text,
			e.name,COALESCE(o.mime,'application/octet-stream'),o.size,encode(o.sha256,'hex'),
			t.reason,t.attempt,t.version,t.lease_until
		FROM transfer_tasks t
		JOIN file_objects o ON o.id=t.object_id AND o.user_id=t.user_id
		JOIN file_entries e ON e.object_id=t.object_id AND e.user_id=t.user_id AND e.status='active'
		JOIN LATERAL (
			SELECT ts.source_device_id FROM transfer_sources ts
			JOIN replicas rr ON rr.object_id=t.object_id AND rr.device_id=ts.source_device_id AND rr.user_id=t.user_id
			JOIN devices d ON d.id=ts.source_device_id AND d.status='active'
				AND d.last_seen_at > NOW() - INTERVAL '2 minutes'
			WHERE ts.task_id=t.id AND ts.disabled_at IS NULL AND rr.state='ready' AND rr.endpoint IS NOT NULL AND rr.client_file_id IS NOT NULL
			ORDER BY ts.rank,d.last_seen_at DESC NULLS LAST LIMIT 1
		) s ON TRUE
		JOIN replicas r ON r.object_id=t.object_id AND r.device_id=s.source_device_id AND r.user_id=t.user_id
		WHERE t.id=$1
		ORDER BY e.created_at LIMIT 1
	`, taskID).Scan(&a.TaskID, &a.ObjectID, &a.TargetDeviceID, &a.SourceDeviceID, &a.SourceEndpoint, &a.SourceLocalFileID, &a.TargetLocalFileID, &a.Name, &a.MIME, &a.Size, &a.SHA256, &a.Reason, &a.Attempt, &a.Version, &a.LeaseUntil)
	if err != nil {
		return nil, err
	}
	return a, tx.Commit()
}

func (r *Repository) CompleteReplica(ctx context.Context, userID, deviceID, taskID string, req CompleteRequest) error {
	if req.LocalFileID == "" || len(req.LocalFileID) > 64 || req.Size < 0 || len(req.SHA256) != 64 {
		return fmt.Errorf("invalid completion report")
	}
	if _, err := hex.DecodeString(req.SHA256); err != nil || strings.ToLower(req.SHA256) != req.SHA256 {
		return fmt.Errorf("invalid completion hash")
	}
	endpoint, err := validateReplicaEndpoint(req.Endpoint)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var objectID, state, leaseOwner, sha string
	var size int64
	var version int64
	var leaseUntil time.Time
	err = tx.QueryRowContext(ctx, `SELECT t.object_id::text,t.state,t.lease_owner::text,t.lease_until,t.version,o.size,encode(o.sha256,'hex') FROM transfer_tasks t JOIN file_objects o ON o.id=t.object_id AND o.user_id=t.user_id WHERE t.id=$1 AND t.user_id=$2 AND t.target_device_id=$3 FOR UPDATE OF t`, taskID, userID, deviceID).Scan(&objectID, &state, &leaseOwner, &leaseUntil, &version, &size, &sha)
	if err == sql.ErrNoRows {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if state != "verifying" || leaseOwner != deviceID || !leaseUntil.After(time.Now()) {
		return ErrNotClaimable
	}
	if version != req.ExpectedVersion {
		return ErrVersionConflict
	}
	if size != req.Size || sha != req.SHA256 {
		return fmt.Errorf("completion content identity mismatch")
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO replicas(id,user_id,object_id,device_id,state,size,verified_at,reported_at,endpoint,client_file_id) VALUES($1,$2,$3,$4,'ready',$5,NOW(),NOW(),$6,$7) ON CONFLICT(object_id,device_id) DO UPDATE SET state='ready',size=EXCLUDED.size,verified_at=NOW(),reported_at=NOW(),endpoint=EXCLUDED.endpoint,client_file_id=EXCLUDED.client_file_id,version=replicas.version+1,updated_at=NOW()`, uuid.NewString(), userID, objectID, deviceID, size, endpoint, req.LocalFileID)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE transfer_tasks SET state='completed',lease_owner=NULL,lease_until=NULL,version=version+1,updated_at=NOW() WHERE id=$1 AND version=$2`, taskID, req.ExpectedVersion)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrVersionConflict
	}
	return tx.Commit()
}

// FailAttempt releases a failed lease with bounded exponential retry delay.
func (r *Repository) FailAttempt(ctx context.Context, userID, deviceID, taskID string, expectedVersion int64) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE transfer_tasks SET
			state=CASE WHEN attempt>=10 THEN 'failed_permanent' ELSE 'retry_wait' END,
			lease_owner=NULL,
			lease_until=CASE WHEN attempt>=10 THEN NULL ELSE NOW() + LEAST(power(2,attempt),300) * INTERVAL '1 second' END,
			version=version+1,updated_at=NOW()
		WHERE id=$1 AND user_id=$2 AND target_device_id=$3 AND lease_owner=$3
		  AND lease_until>NOW() AND version=$4
	`, taskID, userID, deviceID, expectedVersion)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrNotClaimable
	}
	return nil
}

// CancelTask cancels any non-terminal task owned by the authenticated account.
func (r *Repository) CancelTask(ctx context.Context, userID, taskID string) error {
	result, err := r.db.ExecContext(ctx, `UPDATE transfer_tasks SET state='canceled',lease_owner=NULL,lease_until=NULL,version=version+1,updated_at=NOW() WHERE id=$1 AND user_id=$2 AND state NOT IN ('completed','canceled','failed_permanent')`, taskID, userID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		var exists bool
		if err := r.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM transfer_tasks WHERE id=$1 AND user_id=$2)`, taskID, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}

func validateReplicaEndpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid replica endpoint")
	}
	return strings.TrimRight(u.String(), "/"), nil
}
