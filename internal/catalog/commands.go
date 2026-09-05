package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var ErrCommandNotFound = errors.New("device command not found")

type DeviceCommand struct {
	ID          string          `json:"id"`
	OperationID string          `json:"operation_id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Attempt     int             `json:"attempt"`
	LeaseUntil  time.Time       `json:"lease_until"`
}

func enqueueLifecycleCommands(ctx context.Context, tx *sql.Tx, userID, originDeviceID, fileID string, report LifecycleReport) error {
	payload, err := json.Marshal(map[string]interface{}{"action": report.Action, "name": report.Name, "purge_after": report.PurgeAfter})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO device_commands(user_id,device_id,operation_id,type,payload)
		SELECT e.user_id,r.device_id,$1,'file.lifecycle',$2::jsonb || jsonb_build_object('local_file_id',r.client_file_id)
		FROM file_entries e JOIN replicas r ON r.object_id=e.object_id AND r.user_id=e.user_id
		WHERE e.id=$3 AND e.user_id=$4 AND r.device_id<>$5 AND r.state='ready' AND r.client_file_id IS NOT NULL
		ON CONFLICT(device_id,operation_id) DO NOTHING
	`, report.OperationID, string(payload), fileID, userID, originDeviceID)
	return err
}

// QueueOriginLifecycleCommand complements an authoritative server-side
// lifecycle request: ApplyLifecycleReport already queued every other replica,
// while this queues the origin that did not perform the mutation locally.
func (r *Repository) QueueOriginLifecycleCommand(ctx context.Context, userID, fileID string, report LifecycleReport) error {
	payload, err := json.Marshal(map[string]interface{}{"action": report.Action, "name": report.Name, "purge_after": report.PurgeAfter})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO device_commands(user_id,device_id,operation_id,type,payload)
		SELECT e.user_id,e.origin_device_id,$1,'file.lifecycle',$2::jsonb || jsonb_build_object('local_file_id',e.client_file_id)
		FROM file_entries e WHERE e.id=$3 AND e.user_id=$4 AND e.origin_device_id IS NOT NULL AND e.client_file_id IS NOT NULL
		ON CONFLICT(device_id,operation_id) DO NOTHING
	`, report.OperationID, string(payload), fileID, userID)
	return err
}

func (r *Repository) ClaimDeviceCommand(ctx context.Context, userID, deviceID string, lease time.Duration) (*DeviceCommand, error) {
	if lease <= 0 || lease > 10*time.Minute {
		lease = 2 * time.Minute
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id::text FROM device_commands WHERE user_id=$1 AND device_id=$2 AND (state='queued' OR (state='leased' AND lease_until<=NOW())) ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, userID, deviceID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, ErrCommandNotFound
	}
	if err != nil {
		return nil, err
	}
	until := time.Now().UTC().Add(lease)
	cmd := &DeviceCommand{}
	err = tx.QueryRowContext(ctx, `UPDATE device_commands SET state='leased',attempt=attempt+1,lease_until=$1,updated_at=NOW() WHERE id=$2 RETURNING id::text,operation_id,type,payload,attempt,lease_until`, until, id).Scan(&cmd.ID, &cmd.OperationID, &cmd.Type, &cmd.Payload, &cmd.Attempt, &cmd.LeaseUntil)
	if err != nil {
		return nil, err
	}
	return cmd, tx.Commit()
}

func (r *Repository) FinishDeviceCommand(ctx context.Context, userID, deviceID, id string, success bool, message string) error {
	state := "completed"
	if !success {
		state = "queued"
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var payload []byte
	result, err := tx.ExecContext(ctx, `UPDATE device_commands SET state=CASE WHEN $1='queued' AND attempt>=10 THEN 'failed' ELSE $1 END,lease_until=CASE WHEN $1='queued' AND attempt<10 THEN NOW()+LEAST(power(2,attempt),300)*INTERVAL '1 second' ELSE NULL END,last_error=NULLIF($2,''),updated_at=NOW() WHERE id=$3 AND user_id=$4 AND device_id=$5 AND state='leased'`, state, message, id, userID, deviceID)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n != 1 {
		return ErrCommandNotFound
	}
	if success {
		if err := tx.QueryRowContext(ctx, `SELECT payload FROM device_commands WHERE id=$1`, id).Scan(&payload); err != nil {
			return err
		}
		var lifecycle struct {
			Action      string `json:"action"`
			LocalFileID string `json:"local_file_id"`
		}
		if json.Unmarshal(payload, &lifecycle) == nil && lifecycle.Action == "purge" {
			if _, err := tx.ExecContext(ctx, `UPDATE replicas SET state='deleted',version=version+1,updated_at=NOW() WHERE user_id=$1 AND device_id=$2 AND client_file_id=$3`, userID, deviceID, lifecycle.LocalFileID); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
