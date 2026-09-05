package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

var (
	ErrFileNotFound   = errors.New("catalog file not found")
	ErrNameConflict   = errors.New("catalog name conflict")
	ErrInvalidState   = errors.New("catalog file state does not allow this operation")
	ErrInvalidRequest = errors.New("invalid catalog request")
)

// UnifiedFile is the account-wide logical file view returned to every client.
// The control plane stores metadata and a verified replica endpoint only; it
// never stores file bytes.
type UnifiedFile struct {
	ID                 string     `json:"id"`
	FolderID           string     `json:"folder_id"`
	LocalFileID        string     `json:"local_file_id"`
	Name               string     `json:"name"`
	MIME               string     `json:"mime"`
	Size               int64      `json:"size"`
	SHA256             string     `json:"sha256"`
	Status             string     `json:"status"`
	Version            int64      `json:"version"`
	OriginDeviceID     string     `json:"origin_device_id"`
	OriginDeviceName   string     `json:"origin_device_name"`
	OriginLastSeenAt   *time.Time `json:"origin_last_seen_at,omitempty"`
	OriginEndpoint     string     `json:"origin_endpoint"`
	ReplicaEndpoint    string     `json:"replica_endpoint"`
	ContentLocalFileID string     `json:"content_local_file_id"`
	DeletedAt          *time.Time `json:"deleted_at,omitempty"`
	PurgeAfter         *time.Time `json:"purge_after,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type RegisterFileRequest struct {
	LocalFileID string `json:"local_file_id"`
	Name        string `json:"name"`
	MIME        string `json:"mime"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	Endpoint    string `json:"endpoint"`
}

type LifecycleReport struct {
	OperationID string     `json:"operation_id"`
	LocalFileID string     `json:"local_file_id"`
	Action      string     `json:"action"`
	Name        string     `json:"name,omitempty"`
	PurgeAfter  *time.Time `json:"purge_after,omitempty"`
}

// RegisterVerifiedFile atomically creates or reuses the content object,
// logical entry and READY replica. Repeating the same local file id from the
// same device returns the existing entry, making outbox retries idempotent.
func (r *Repository) RegisterVerifiedFile(ctx context.Context, userID, deviceID string, req RegisterFileRequest) (*UnifiedFile, error) {
	if req.LocalFileID == "" || len(req.LocalFileID) > 64 {
		return nil, fmt.Errorf("%w: local_file_id", ErrInvalidRequest)
	}
	name := strings.TrimSpace(req.Name)
	normalized, err := normalizeName(name)
	if err != nil || len(name) > 255 {
		return nil, fmt.Errorf("%w: name", ErrInvalidRequest)
	}
	if req.Size < 0 || len(req.SHA256) != 64 || !isLowerHex(req.SHA256) {
		return nil, fmt.Errorf("%w: content identity", ErrInvalidRequest)
	}
	endpoint, err := validateEndpoint(req.Endpoint)
	if err != nil {
		return nil, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		return nil, err
	}

	existing, err := getUnifiedFileByLocalID(ctx, tx, userID, deviceID, req.LocalFileID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		if existing.SHA256 != req.SHA256 || existing.Size != req.Size {
			return nil, fmt.Errorf("%w: local_file_id refers to different content", ErrInvalidRequest)
		}
		return existing, tx.Commit()
	}

	folderID, err := ensureRootFolder(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	objectID := uuid.NewString()
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO file_objects (id, user_id, sha256, size, mime, chunk_size, chunk_count, status)
		VALUES ($1, $2, decode($3, 'hex'), $4, $5, 4194304,
			GREATEST(1, ((($4::bigint) + 4194303) / 4194304)::integer), 'ready')
		ON CONFLICT (user_id, sha256, size) DO UPDATE
		SET status = 'ready', mime = COALESCE(NULLIF(EXCLUDED.mime, ''), file_objects.mime), updated_at = NOW()
		RETURNING id::text
	`, objectID, userID, req.SHA256, req.Size, req.MIME).Scan(&objectID); err != nil {
		return nil, fmt.Errorf("register file object: %w", err)
	}

	entryID := uuid.NewString()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO file_entries
			(id, user_id, folder_id, object_id, name, normalized_name, origin_device_id, client_file_id, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'active')
	`, entryID, userID, folderID, objectID, name, normalized, deviceID, req.LocalFileID)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrNameConflict
		}
		return nil, fmt.Errorf("register file entry: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO replicas (id, user_id, object_id, device_id, state, size, verified_at, reported_at, endpoint, client_file_id)
		VALUES ($1, $2, $3, $4, 'ready', $5, NOW(), NOW(), $6, $7)
		ON CONFLICT (object_id, device_id) DO UPDATE
		SET state = 'ready', size = EXCLUDED.size, verified_at = NOW(), reported_at = NOW(),
			endpoint = EXCLUDED.endpoint, client_file_id = EXCLUDED.client_file_id,
			version = replicas.version + 1, updated_at = NOW()
	`, uuid.NewString(), userID, objectID, deviceID, req.Size, endpoint, req.LocalFileID)
	if err != nil {
		return nil, fmt.Errorf("register replica: %w", err)
	}
	if err := appendEvent(ctx, tx, userID, "file.registered", entryID, 1); err != nil {
		return nil, err
	}
	file, err := getUnifiedFile(ctx, tx, userID, entryID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return file, nil
}

func (r *Repository) ListUnifiedFiles(ctx context.Context, userID string, trash bool) ([]UnifiedFile, error) {
	return r.QueryUnifiedFiles(ctx, userID, FileQuery{Trash: trash})
}

// ApplyLifecycleReport applies an operation already committed by the owning
// Agent. The origin-device predicate prevents one device from reporting state
// for another device's local file.
func (r *Repository) ApplyLifecycleReport(ctx context.Context, userID, deviceID string, report LifecycleReport) (*UnifiedFile, error) {
	if report.LocalFileID == "" || report.OperationID == "" || len(report.OperationID) > 128 {
		return nil, fmt.Errorf("%w: operation_id and local_file_id are required", ErrInvalidRequest)
	}
	requestJSON, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	requestHash := sha256.Sum256(requestJSON)
	idempotencyKey := deviceID + ":" + report.OperationID
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, userID); err != nil {
		return nil, err
	}
	var storedHash []byte
	var storedBody []byte
	err = tx.QueryRowContext(ctx, `SELECT request_hash,response_body FROM idempotency_keys WHERE user_id=$1 AND key=$2 AND status='completed'`, userID, idempotencyKey).Scan(&storedHash, &storedBody)
	if err == nil {
		if !equalBytes(storedHash, requestHash[:]) {
			return nil, fmt.Errorf("%w: operation_id was reused with different input", ErrInvalidRequest)
		}
		var stored UnifiedFile
		if err := json.Unmarshal(storedBody, &stored); err != nil {
			return nil, err
		}
		return &stored, tx.Commit()
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO idempotency_keys (user_id,key,request_hash,status,expires_at) VALUES ($1,$2,$3,'processing',NOW()+INTERVAL '30 days')`, userID, idempotencyKey, requestHash[:]); err != nil {
		return nil, err
	}
	file, err := getUnifiedFileByLocalID(ctx, tx, userID, deviceID, report.LocalFileID)
	if err != nil {
		return nil, err
	}
	if file == nil {
		return nil, ErrFileNotFound
	}

	changed := false
	switch report.Action {
	case "rename":
		if file.Status != "active" {
			return nil, ErrInvalidState
		}
		name := strings.TrimSpace(report.Name)
		normalized, nameErr := normalizeName(name)
		if nameErr != nil || len(name) > 255 {
			return nil, fmt.Errorf("%w: name", ErrInvalidRequest)
		}
		if _, err = tx.ExecContext(ctx, `UPDATE file_entries SET name=$1, normalized_name=$2, version=version+1, updated_at=NOW() WHERE id=$3`, name, normalized, file.ID); err != nil {
			if isUniqueViolation(err) {
				return nil, ErrNameConflict
			}
			return nil, err
		}
		changed = true
	case "trash":
		if file.Status == "active" {
			purgeAfter := time.Now().UTC().Add(7 * 24 * time.Hour)
			if report.PurgeAfter != nil {
				purgeAfter = report.PurgeAfter.UTC()
			}
			if _, err = tx.ExecContext(ctx, `UPDATE file_entries SET status='trashed', version=version+1, updated_at=NOW() WHERE id=$1`, file.ID); err != nil {
				return nil, err
			}
			_, err = tx.ExecContext(ctx, `INSERT INTO trash_records (id,user_id,file_entry_id,original_folder_id,original_name,purge_after,status) SELECT $1,user_id,id,folder_id,name,$2,'trashed' FROM file_entries WHERE id=$3 ON CONFLICT (file_entry_id) DO UPDATE SET deleted_at=NOW(),purge_after=EXCLUDED.purge_after,status='trashed',updated_at=NOW()`, uuid.NewString(), purgeAfter, file.ID)
			if err != nil {
				return nil, err
			}
			changed = true
		} else if file.Status != "trashed" {
			return nil, ErrInvalidState
		}
	case "restore":
		if file.Status == "trashed" {
			if _, err = tx.ExecContext(ctx, `UPDATE file_entries SET status='active', version=version+1, updated_at=NOW() WHERE id=$1`, file.ID); err != nil {
				if isUniqueViolation(err) {
					return nil, ErrNameConflict
				}
				return nil, err
			}
			if _, err = tx.ExecContext(ctx, `DELETE FROM trash_records WHERE file_entry_id=$1`, file.ID); err != nil {
				return nil, err
			}
			changed = true
		} else if file.Status != "active" {
			return nil, ErrInvalidState
		}
	case "purge":
		if file.Status == "purged" {
			break
		}
		if file.Status != "trashed" {
			return nil, ErrInvalidState
		}
		if _, err = tx.ExecContext(ctx, `UPDATE file_entries SET status='purged', version=version+1, updated_at=NOW() WHERE id=$1`, file.ID); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE trash_records SET status='purged', updated_at=NOW() WHERE file_entry_id=$1`, file.ID); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE replicas SET state='deleted', version=version+1, updated_at=NOW() WHERE user_id=$1 AND device_id=$2 AND object_id=(SELECT object_id FROM file_entries WHERE id=$3)`, userID, deviceID, file.ID); err != nil {
			return nil, err
		}
		changed = true
	default:
		return nil, fmt.Errorf("%w: lifecycle action", ErrInvalidRequest)
	}

	if changed {
		if err := enqueueLifecycleCommands(ctx, tx, userID, deviceID, file.ID, report); err != nil {
			return nil, err
		}
		if err := appendEvent(ctx, tx, userID, "file."+report.Action, file.ID, file.Version+1); err != nil {
			return nil, err
		}
	}
	file, err = getUnifiedFile(ctx, tx, userID, file.ID)
	if err != nil {
		return nil, err
	}
	responseJSON, err := json.Marshal(file)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE idempotency_keys SET status='completed',response_code=200,response_body=$1::jsonb,updated_at=NOW() WHERE user_id=$2 AND key=$3`, string(responseJSON), userID, idempotencyKey); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return file, nil
}

const unifiedFileSelect = `
	SELECT e.id::text, e.folder_id::text, COALESCE(e.client_file_id,''), e.name, COALESCE(o.mime,''), o.size,
		encode(o.sha256,'hex'), e.status, e.version, e.origin_device_id::text,
		COALESCE(d.name,''), d.last_seen_at, COALESCE(origin_replica.endpoint,''),
		COALESCE(content_replica.endpoint,''), COALESCE(content_replica.client_file_id,''), tr.deleted_at, tr.purge_after,
		e.created_at, e.updated_at
	FROM file_entries e
	JOIN file_objects o ON o.id=e.object_id AND o.user_id=e.user_id
	LEFT JOIN devices d ON d.id=e.origin_device_id AND d.user_id=e.user_id
	LEFT JOIN replicas origin_replica ON origin_replica.object_id=e.object_id AND origin_replica.device_id=e.origin_device_id AND origin_replica.user_id=e.user_id
	LEFT JOIN LATERAL (
		SELECT rr.endpoint,rr.client_file_id
		FROM replicas rr JOIN devices rd ON rd.id=rr.device_id AND rd.user_id=rr.user_id AND rd.status='active'
		WHERE rr.object_id=e.object_id AND rr.user_id=e.user_id AND rr.state='ready'
		  AND rr.endpoint IS NOT NULL AND rr.client_file_id IS NOT NULL
		ORDER BY rd.last_seen_at DESC NULLS LAST,rr.verified_at DESC NULLS LAST,rr.id
		LIMIT 1
	) content_replica ON TRUE
	LEFT JOIN trash_records tr ON tr.file_entry_id=e.id AND tr.user_id=e.user_id`

func getUnifiedFile(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, userID, id string) (*UnifiedFile, error) {
	return scanUnifiedFileRow(q.QueryRowContext(ctx, unifiedFileSelect+` WHERE e.user_id=$1 AND e.id=$2`, userID, id))
}

func getUnifiedFileByLocalID(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...interface{}) *sql.Row
}, userID, deviceID, localID string) (*UnifiedFile, error) {
	return scanUnifiedFileRow(q.QueryRowContext(ctx, unifiedFileSelect+` WHERE e.user_id=$1 AND e.origin_device_id=$2 AND e.client_file_id=$3`, userID, deviceID, localID))
}

type scanner interface{ Scan(...interface{}) error }

func scanUnifiedFileRow(row scanner) (*UnifiedFile, error) {
	file, err := scanUnifiedFile(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return file, err
}

func scanUnifiedFile(row scanner) (*UnifiedFile, error) {
	var file UnifiedFile
	var lastSeenAt, deletedAt, purgeAfter sql.NullTime
	if err := row.Scan(&file.ID, &file.FolderID, &file.LocalFileID, &file.Name, &file.MIME, &file.Size,
		&file.SHA256, &file.Status, &file.Version, &file.OriginDeviceID,
		&file.OriginDeviceName, &lastSeenAt, &file.OriginEndpoint, &file.ReplicaEndpoint,
		&file.ContentLocalFileID, &deletedAt, &purgeAfter,
		&file.CreatedAt, &file.UpdatedAt); err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		file.DeletedAt = &deletedAt.Time
	}
	if lastSeenAt.Valid {
		file.OriginLastSeenAt = &lastSeenAt.Time
	}
	if purgeAfter.Valid {
		file.PurgeAfter = &purgeAfter.Time
	}
	return &file, nil
}

func ensureRootFolder(ctx context.Context, tx *sql.Tx, userID string) (string, error) {
	var id string
	err := tx.QueryRowContext(ctx, `SELECT id::text FROM folders WHERE user_id=$1 AND parent_id IS NULL AND normalized_name='Files' AND status='active'`, userID).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	id = uuid.NewString()
	_, err = tx.ExecContext(ctx, `INSERT INTO folders (id,user_id,parent_id,name,normalized_name,status) VALUES ($1,$2,NULL,'Files','Files','active')`, id, userID)
	return id, err
}

func appendEvent(ctx context.Context, tx *sql.Tx, userID, eventType, entityID string, version int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO account_events (user_id,seq,type,entity_id,entity_version,payload) SELECT $1,COALESCE(MAX(seq),0)+1,$2,$3,$4,'{}'::jsonb FROM account_events WHERE user_id=$1`, userID, eventType, entityID, version)
	return err
}

func validateEndpoint(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("%w: replica endpoint", ErrInvalidRequest)
	}
	return strings.TrimRight(u.String(), "/"), nil
}

func isLowerHex(value string) bool {
	for _, r := range value {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func isUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}

func equalBytes(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var diff byte
	for i := range a {
		diff |= a[i] ^ b[i]
	}
	return diff == 0
}
