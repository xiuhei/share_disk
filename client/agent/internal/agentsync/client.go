// Package agentsync delivers the Agent's durable local outbox to the control
// plane using an independently provisioned device session.
package agentsync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/share-disk/share-disk/client/ubuntu/internal/localapi"
	"github.com/share-disk/share-disk/client/ubuntu/internal/storage"
	storagecatalog "github.com/share-disk/share-disk/client/ubuntu/internal/storage/catalog"
)

var errTransferCanceled = errors.New("transfer canceled")

const (
	settingAccessToken  = "control_access_token"
	settingRefreshToken = "control_refresh_token"
	settingDeviceID     = "control_device_id"
	settingUserID       = "control_user_id"
)

// Client owns provisioning credentials and serial outbox delivery.
type Client struct {
	db                *sql.DB
	repo              *localapi.Repository
	http              *http.Client
	transferHTTP      *http.Client
	controlURL        string
	endpoint          string
	peerID            string
	deviceName        string
	syncInterval      time.Duration
	heartbeatInterval time.Duration
	mu                sync.Mutex
	transferStore     *storage.SQLiteObjectStore
	transferScratch   string
}

// SetTransferStore enables execution of server-assigned replica tasks.
func (c *Client) SetTransferStore(store *storage.SQLiteObjectStore, scratchDir string) {
	c.transferStore = store
	c.transferScratch = scratchDir
}

// Status is safe to expose to an authenticated LAN client; it contains no
// credentials or request bodies.
type Status struct {
	Configured  bool  `json:"configured"`
	Provisioned bool  `json:"provisioned"`
	Pending     int64 `json:"pending"`
	Failed      int64 `json:"failed"`
}

// These wire types are intentionally client-owned. The Ubuntu client depends
// on the Control API contract, not on server implementation packages.
type registerDeviceRequest struct {
	Name     string `json:"name"`
	Platform string `json:"platform"`
	PeerID   string `json:"peer_id"`
}

type enrollDeviceRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
	PeerID   string `json:"peer_id"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type authResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	UserID       string `json:"user_id"`
	DeviceID     string `json:"device_id"`
}

type deviceCommand struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type transferAssignment struct {
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

type completeTransferRequest struct {
	ExpectedVersion int64  `json:"expected_version"`
	LocalFileID     string `json:"local_file_id"`
	Endpoint        string `json:"endpoint"`
	Size            int64  `json:"size"`
	SHA256          string `json:"sha256"`
}

func New(db *sql.DB, controlURL, endpoint, peerID, deviceName string, interval, heartbeatInterval time.Duration) *Client {
	return &Client{db: db, repo: localapi.NewRepository(db), http: &http.Client{Timeout: 15 * time.Second}, transferHTTP: &http.Client{Timeout: 9 * time.Minute}, controlURL: strings.TrimRight(controlURL, "/"), endpoint: strings.TrimRight(endpoint, "/"), peerID: peerID, deviceName: deviceName, syncInterval: interval, heartbeatInterval: heartbeatInterval}
}

// Provision registers this physical Agent under the authenticated caller's
// account and persists only the new Agent session.
func (c *Client) Provision(ctx context.Context, callerAuthorization, callerUserID string) error {
	if !strings.HasPrefix(callerAuthorization, "Bearer ") {
		return errors.New("bearer authorization is required")
	}
	var existingUserID string
	if err := c.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, settingUserID).Scan(&existingUserID); err == nil && existingUserID == callerUserID {
		// Do not mistake stale local credentials for successful provisioning.
		// A revoked or reset Agent session must be recreated using the caller's
		// valid account session so its durable outbox can resume.
		if err := c.sendHeartbeat(ctx); err == nil {
			return nil
		}
	} else if err != nil && err != sql.ErrNoRows {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	body := registerDeviceRequest{Name: c.deviceName, Platform: "linux", PeerID: c.peerID}
	var auth authResponse
	status, _, err := c.request(ctx, http.MethodPost, "/v1/devices/register", callerAuthorization, body, &auth)
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return fmt.Errorf("control server rejected device registration with status %d", status)
	}
	return c.persistAuth(ctx, auth)
}

// Enroll authenticates an existing account and provisions this Linux Agent as
// its own stable device. It is intended for the local Unix-socket CLI setup.
func (c *Client) Enroll(ctx context.Context, account, password string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	account = strings.TrimSpace(account)
	if account == "" || password == "" {
		return errors.New("account and password are required")
	}
	request := enrollDeviceRequest{Account: account, Password: password, Name: c.deviceName, Platform: "linux", PeerID: c.peerID}
	var auth authResponse
	status, _, err := c.request(ctx, http.MethodPost, "/v1/auth/enroll", "", request, &auth)
	if err != nil {
		return err
	}
	if status != http.StatusCreated {
		return fmt.Errorf("control server rejected Agent enrollment with status %d", status)
	}
	return c.persistAuth(ctx, auth)
}

func (c *Client) persistAuth(ctx context.Context, auth authResponse) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{settingAccessToken: auth.AccessToken, settingRefreshToken: auth.RefreshToken, settingDeviceID: auth.DeviceID, settingUserID: auth.UserID} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, datetime('now')) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value); err != nil {
			return fmt.Errorf("persist coordinator credentials: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return c.reconcileExisting(ctx, auth.DeviceID)
}

func (c *Client) reconcileExisting(ctx context.Context, serverDeviceID string) error {
	marker := "catalog_reconciled:" + serverDeviceID
	var ignored string
	if err := c.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, marker).Scan(&ignored); err == nil {
		return nil
	} else if err != sql.ErrNoRows {
		return err
	}
	rows, err := c.db.QueryContext(ctx, `SELECT f.id,f.name,f.mime,o.size,lower(hex(o.sha256)),f.status,f.purge_after FROM lan_files f JOIN local_objects o ON o.id=f.object_id WHERE f.status IN ('active','trashed') ORDER BY f.created_at,f.id`)
	if err != nil {
		return err
	}
	type existing struct {
		id, name, mime, sha, status string
		size                        int64
		purge                       sql.NullString
	}
	var files []existing
	for rows.Next() {
		var file existing
		if err := rows.Scan(&file.id, &file.name, &file.mime, &file.size, &file.sha, &file.status, &file.purge); err != nil {
			rows.Close()
			return err
		}
		files = append(files, file)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, file := range files {
		if _, err := c.repo.CreateOutgoingOp(ctx, "catalog.register", "reconcile-register:"+serverDeviceID+":"+file.id, map[string]interface{}{"local_file_id": file.id, "name": file.name, "mime": file.mime, "size": file.size, "sha256": file.sha, "endpoint": c.endpoint}); err != nil {
			return err
		}
		if file.status == "trashed" {
			request := map[string]interface{}{"local_file_id": file.id, "action": "trash"}
			if file.purge.Valid {
				request["purge_after"] = file.purge.String
			}
			op, err := c.repo.CreateOutgoingOp(ctx, "catalog.lifecycle", "reconcile-trash:"+serverDeviceID+":"+file.id, request)
			if err != nil {
				return err
			}
			request["operation_id"] = op.ID
			encoded, _ := json.Marshal(request)
			if _, err := c.db.ExecContext(ctx, `UPDATE outgoing_ops SET request_data=? WHERE id=?`, encoded, op.ID); err != nil {
				return err
			}
		}
	}
	_, err = c.db.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES(?, '1', datetime('now'))`, marker)
	return err
}

func (c *Client) Status(ctx context.Context) (interface{}, error) {
	var deviceID string
	err := c.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, settingDeviceID).Scan(&deviceID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	var pending, failed int64
	if err := c.db.QueryRowContext(ctx, `SELECT count(*) FROM outgoing_ops WHERE status IN ('pending','processing')`).Scan(&pending); err != nil {
		return nil, err
	}
	if err := c.db.QueryRowContext(ctx, `SELECT count(*) FROM outgoing_ops WHERE status = 'failed'`).Scan(&failed); err != nil {
		return nil, err
	}
	return Status{Configured: true, Provisioned: deviceID != "", Pending: pending, Failed: failed}, nil
}

// Recover releases operations whose process died while delivering or while a
// local purge saga was finishing. Call it after storage startup recovery and
// before starting any new local mutations.
func (c *Client) Recover(ctx context.Context) error {
	_, err := c.db.ExecContext(ctx, `UPDATE outgoing_ops SET status = 'pending', updated_at = datetime('now') WHERE status = 'processing'`)
	return err
}

// Run processes operations FIFO. One operation is completed before the next
// begins so rename/trash ordering is deterministic.
func (c *Client) Run(ctx context.Context) {
	ticker := time.NewTicker(c.syncInterval)
	defer ticker.Stop()
	heartbeat := time.NewTicker(c.heartbeatInterval)
	defer heartbeat.Stop()
	_ = c.sendHeartbeat(ctx)
	for {
		c.drain(ctx)
		c.executeOneDeviceCommand(ctx)
		c.processCancellations(ctx)
		c.executeOneTransfer(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-heartbeat.C:
			_ = c.sendHeartbeat(ctx)
		}
	}
}

func (c *Client) executeOneDeviceCommand(ctx context.Context) {
	if c.transferStore == nil {
		return
	}
	var command deviceCommand
	status, _, err := c.authorizedRequest(ctx, http.MethodPost, "/v1/device-commands/claim", struct{}{}, &command)
	if err != nil || status == http.StatusNoContent {
		return
	}
	if status != http.StatusOK {
		return
	}
	applyErr := c.applyDeviceCommand(ctx, command)
	message := ""
	if applyErr != nil {
		message = applyErr.Error()
		if len(message) > 500 {
			message = message[:500]
		}
	}
	_, _, _ = c.authorizedRequest(ctx, http.MethodPost, "/v1/device-commands/"+url.PathEscape(command.ID)+"/finish", map[string]interface{}{"success": applyErr == nil, "error": message}, nil)
}

func (c *Client) applyDeviceCommand(ctx context.Context, command deviceCommand) error {
	if command.Type != "file.lifecycle" {
		return fmt.Errorf("unsupported device command %s", command.Type)
	}
	var payload struct {
		LocalFileID string `json:"local_file_id"`
		Action      string `json:"action"`
		Name        string `json:"name"`
	}
	if err := json.Unmarshal(command.Payload, &payload); err != nil || payload.LocalFileID == "" {
		return fmt.Errorf("invalid lifecycle command")
	}
	owner, err := c.setting(ctx, settingUserID)
	if err != nil {
		return err
	}
	active, err := c.transferStore.GetStore().GetLANFile(payload.LocalFileID, owner)
	if err != nil {
		return err
	}
	trash, err := c.transferStore.GetStore().GetLANTrashFile(payload.LocalFileID, owner)
	if err != nil {
		return err
	}
	switch payload.Action {
	case "rename":
		if active != nil && active.Name == payload.Name {
			return nil
		}
		normalized, err := storagecatalog.NormalizeName(payload.Name)
		if err != nil {
			return err
		}
		_, err = c.transferStore.GetStore().RenameLANFile(payload.LocalFileID, owner, payload.Name, normalized)
		return err
	case "trash":
		if trash != nil {
			return nil
		}
		_, err = c.transferStore.GetStore().TrashLANFile(payload.LocalFileID, owner, 7*24*time.Hour)
		return err
	case "restore":
		if active != nil {
			return nil
		}
		_, err = c.transferStore.GetStore().RestoreLANFile(payload.LocalFileID, owner)
		return err
	case "purge":
		if active == nil && trash == nil {
			return nil
		}
		if trash == nil {
			return fmt.Errorf("file must be trashed before purge")
		}
		return c.transferStore.PurgeLANFile(ctx, payload.LocalFileID, owner)
	default:
		return fmt.Errorf("unsupported lifecycle action %s", payload.Action)
	}
}

func (c *Client) executeOneTransfer(ctx context.Context) {
	if c.transferStore == nil {
		return
	}
	var assignment transferAssignment
	status, _, err := c.authorizedRequest(ctx, http.MethodPost, "/v1/transfers/claim", map[string]int{"lease_seconds": 600}, &assignment)
	if err != nil || status == http.StatusNoContent {
		return
	}
	if status != http.StatusOK {
		return
	}
	c.recordTransfer(assignment)
	version := assignment.Version
	fail := func() {
		_, _, _ = c.authorizedRequest(ctx, http.MethodPost, "/v1/transfers/"+url.PathEscape(assignment.TaskID)+"/fail", map[string]int64{"expected_version": version}, nil)
		c.updateTransfer(assignment.TaskID, "retry_wait", 0, "transfer attempt failed")
	}
	for _, state := range []string{"discovering", "connecting", "transferring"} {
		if c.cancelRequested(assignment.TaskID) {
			c.cancelTransfer(ctx, assignment.TaskID)
			return
		}
		status, _, err = c.authorizedRequest(ctx, http.MethodPost, "/v1/transfers/"+url.PathEscape(assignment.TaskID)+"/state", map[string]interface{}{"expected_version": version, "state": state}, nil)
		if err != nil || status != http.StatusNoContent {
			fail()
			return
		}
		version++
		c.updateTransfer(assignment.TaskID, state, 0, "")
	}
	if err := os.MkdirAll(c.transferScratch, 0700); err != nil {
		fail()
		return
	}
	pathKey := sha256.Sum256([]byte(assignment.TaskID))
	tempPath := filepath.Join(c.transferScratch, fmt.Sprintf("transfer-%x.part", pathKey[:16]))
	temp, err := os.OpenFile(tempPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		fail()
		return
	}
	if err := c.downloadReplica(ctx, assignment, temp); err != nil {
		if info, statErr := temp.Stat(); statErr == nil {
			c.updateTransfer(assignment.TaskID, "transferring", info.Size(), err.Error())
		}
		_ = temp.Close()
		if errors.Is(err, errTransferCanceled) {
			_ = os.Remove(tempPath)
			c.cancelTransfer(ctx, assignment.TaskID)
			return
		}
		fail()
		return
	}
	c.updateTransfer(assignment.TaskID, "transferring", assignment.Size, "")
	if c.cancelRequested(assignment.TaskID) {
		_ = temp.Close()
		_ = os.Remove(tempPath)
		c.cancelTransfer(ctx, assignment.TaskID)
		return
	}
	if err := temp.Close(); err != nil {
		fail()
		return
	}
	status, _, err = c.authorizedRequest(ctx, http.MethodPost, "/v1/transfers/"+url.PathEscape(assignment.TaskID)+"/state", map[string]interface{}{"expected_version": version, "state": "verifying"}, nil)
	if err != nil || status != http.StatusNoContent {
		fail()
		return
	}
	version++
	c.updateTransfer(assignment.TaskID, "verifying", assignment.Size, "")
	actualHash, err := storage.HashFile(tempPath)
	if err != nil || actualHash.String() != assignment.SHA256 {
		_ = os.Truncate(tempPath, 0)
		fail()
		return
	}
	info, err := c.transferStore.Import(ctx, tempPath)
	if err != nil || info.Size != assignment.Size || info.Hash.String() != assignment.SHA256 {
		fail()
		return
	}
	userID, err := c.setting(ctx, settingUserID)
	if err != nil {
		fail()
		return
	}
	localFile, err := c.transferStore.GetStore().GetLANFile(assignment.TargetLocalFileID, userID)
	if err != nil {
		fail()
		return
	}
	if localFile == nil {
		normalized, normalizeErr := storagecatalog.NormalizeName(assignment.Name)
		if normalizeErr != nil {
			fail()
			return
		}
		localFile, err = c.transferStore.GetStore().CreateLANFile(assignment.TargetLocalFileID, userID, info.Hash.String(), assignment.Name, normalized, assignment.MIME)
		if err != nil {
			fail()
			return
		}
	}
	if localFile.SHA256 != assignment.SHA256 {
		fail()
		return
	}
	report := completeTransferRequest{ExpectedVersion: version, LocalFileID: localFile.ID, Endpoint: c.endpoint, Size: info.Size, SHA256: info.Hash.String()}
	status, _, err = c.authorizedRequest(ctx, http.MethodPost, "/v1/transfers/"+url.PathEscape(assignment.TaskID)+"/complete", report, nil)
	if err != nil || status != http.StatusNoContent {
		fail()
		return
	}
	c.updateTransfer(assignment.TaskID, "completed", assignment.Size, "")
	_ = os.Remove(tempPath)
}

func (c *Client) recordTransfer(a transferAssignment) {
	_, _ = c.db.Exec(`INSERT INTO transfer_runs(task_id,object_id,name,state,size,attempt,source_endpoint,updated_at) VALUES(?,?,?,?,?,?,?,datetime('now')) ON CONFLICT(task_id) DO UPDATE SET state=excluded.state,attempt=excluded.attempt,source_endpoint=excluded.source_endpoint,error=NULL,cancel_requested=0,updated_at=datetime('now')`, a.TaskID, a.ObjectID, a.Name, "assigned", a.Size, a.Attempt, a.SourceEndpoint)
}

func (c *Client) updateTransfer(taskID, state string, transferred int64, message string) {
	_, _ = c.db.Exec(`UPDATE transfer_runs SET state=?,transferred=CASE WHEN ?>transferred THEN ? ELSE transferred END,error=NULLIF(?,''),updated_at=datetime('now') WHERE task_id=?`, state, transferred, transferred, message, taskID)
}

func (c *Client) cancelRequested(taskID string) bool {
	var requested bool
	_ = c.db.QueryRow(`SELECT cancel_requested<>0 FROM transfer_runs WHERE task_id=?`, taskID).Scan(&requested)
	return requested
}

func (c *Client) cancelTransfer(ctx context.Context, taskID string) {
	status, _, err := c.authorizedRequest(ctx, http.MethodPost, "/v1/transfers/"+url.PathEscape(taskID)+"/cancel", struct{}{}, nil)
	if err == nil && status == http.StatusNoContent {
		c.updateTransfer(taskID, "canceled", 0, "")
	}
}

func (c *Client) processCancellations(ctx context.Context) {
	rows, err := c.db.QueryContext(ctx, `SELECT task_id FROM transfer_runs WHERE cancel_requested=1 AND state NOT IN ('completed','canceled','failed_permanent') ORDER BY updated_at LIMIT 20`)
	if err != nil {
		return
	}
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		c.cancelTransfer(ctx, id)
	}
}

func (c *Client) downloadReplica(ctx context.Context, assignment transferAssignment, dst *os.File) error {
	info, err := dst.Stat()
	if err != nil {
		return err
	}
	offset := info.Size()
	if offset > assignment.Size {
		if err := dst.Truncate(0); err != nil {
			return err
		}
		offset = 0
	}
	if offset == assignment.Size {
		return dst.Sync()
	}
	endpoint := strings.TrimRight(assignment.SourceEndpoint, "/") + "/v1/lan/files/" + url.PathEscape(assignment.SourceLocalFileID) + "/content"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	token, err := c.setting(ctx, settingAccessToken)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
		req.Header.Set("If-Range", `"`+assignment.SHA256+`"`)
	}
	resp, err := c.transferHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("source returned status %d", resp.StatusCode)
	}
	if advertised := resp.Header.Get("X-Content-SHA256"); advertised != "" && !strings.EqualFold(advertised, assignment.SHA256) {
		return fmt.Errorf("source hash mismatch")
	}
	if offset > 0 && resp.StatusCode == http.StatusPartialContent {
		if !strings.HasPrefix(resp.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-", offset)) {
			return fmt.Errorf("source resume offset mismatch")
		}
	} else {
		if err := dst.Truncate(0); err != nil {
			return err
		}
		offset = 0
	}
	if _, err := dst.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	remaining := assignment.Size - offset
	buffer := make([]byte, 1024*1024)
	written := int64(0)
	for written <= remaining {
		if c.cancelRequested(assignment.TaskID) {
			return errTransferCanceled
		}
		want := len(buffer)
		if left := remaining + 1 - written; int64(want) > left {
			want = int(left)
		}
		if want == 0 {
			break
		}
		n, readErr := resp.Body.Read(buffer[:want])
		if n > 0 {
			if _, err := dst.Write(buffer[:n]); err != nil {
				return err
			}
			written += int64(n)
			c.updateTransfer(assignment.TaskID, "transferring", offset+written, "")
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	if offset+written != assignment.Size {
		return fmt.Errorf("source size mismatch")
	}
	return dst.Sync()
}

func (c *Client) authorizedRequest(ctx context.Context, method, path string, body, dst interface{}) (int, []byte, error) {
	token, err := c.setting(ctx, settingAccessToken)
	if err != nil {
		return 0, nil, err
	}
	status, response, err := c.request(ctx, method, path, "Bearer "+token, body, dst)
	if status != http.StatusUnauthorized {
		return status, response, err
	}
	if err := c.refresh(ctx); err != nil {
		return status, response, err
	}
	token, err = c.setting(ctx, settingAccessToken)
	if err != nil {
		return 0, nil, err
	}
	return c.request(ctx, method, path, "Bearer "+token, body, dst)
}

func (c *Client) sendHeartbeat(ctx context.Context) error {
	token, err := c.setting(ctx, settingAccessToken)
	if err != nil {
		return err
	}
	status, _, err := c.requestRaw(ctx, http.MethodPost, "/v1/devices/heartbeat", "Bearer "+token, nil)
	if err != nil {
		return err
	}
	if status == http.StatusUnauthorized {
		if err := c.refresh(ctx); err != nil {
			return err
		}
		token, err = c.setting(ctx, settingAccessToken)
		if err != nil {
			return err
		}
		status, _, err = c.requestRaw(ctx, http.MethodPost, "/v1/devices/heartbeat", "Bearer "+token, nil)
	}
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("heartbeat returned status %d", status)
	}
	return nil
}

func (c *Client) drain(ctx context.Context) {
	for {
		ops, err := c.repo.GetPendingOps(ctx, 20)
		if err != nil || len(ops) == 0 {
			return
		}
		for _, op := range ops {
			if err := c.deliver(ctx, op); err != nil {
				return
			}
		}
	}
}

func (c *Client) deliver(ctx context.Context, op *localapi.OutgoingOp) error {
	if err := c.repo.MarkProcessing(ctx, op.ID); err != nil {
		return err
	}
	path := ""
	switch op.OpType {
	case "catalog.register":
		path = "/v1/catalog/files/register"
	case "catalog.lifecycle":
		path = "/v1/catalog/lifecycle"
	default:
		return c.repo.MarkOpFailed(ctx, op.ID, 0, map[string]string{"error": "unsupported operation type"})
	}
	token, err := c.setting(ctx, settingAccessToken)
	if err != nil {
		_ = c.repo.MarkOpRetryable(ctx, op.ID, c.syncInterval)
		return err
	}
	status, response, err := c.requestRaw(ctx, http.MethodPost, path, "Bearer "+token, op.RequestData)
	if status == http.StatusUnauthorized {
		if refreshErr := c.refresh(ctx); refreshErr == nil {
			token, err = c.setting(ctx, settingAccessToken)
			if err == nil {
				status, response, err = c.requestRaw(ctx, http.MethodPost, path, "Bearer "+token, op.RequestData)
			}
		} else {
			err = refreshErr
		}
	}
	if err != nil || status == http.StatusRequestTimeout || status == http.StatusTooEarly || status == http.StatusTooManyRequests || status >= 500 {
		backoff := time.Duration(1<<min(op.RetryCount, 6)) * c.syncInterval
		return c.repo.MarkOpRetryable(ctx, op.ID, backoff)
	}
	if status < 200 || status >= 300 {
		return c.repo.MarkOpFailed(ctx, op.ID, status, responsePayload(response, nil))
	}
	var value interface{}
	if len(response) > 0 {
		if json.Unmarshal(response, &value) != nil {
			value = map[string]string{"response": string(response)}
		}
	}
	return c.repo.MarkOpCompleted(ctx, op.ID, status, value)
}

func (c *Client) refresh(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	refreshToken, err := c.setting(ctx, settingRefreshToken)
	if err != nil {
		return err
	}
	var auth authResponse
	status, _, err := c.request(ctx, http.MethodPost, "/v1/auth/refresh", "", refreshRequest{RefreshToken: refreshToken}, &auth)
	if err != nil || status != http.StatusOK {
		return fmt.Errorf("refresh Agent session failed")
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{settingAccessToken: auth.AccessToken, settingRefreshToken: auth.RefreshToken} {
		if _, err := tx.ExecContext(ctx, `UPDATE settings SET value = ?, updated_at = datetime('now') WHERE key = ?`, value, key); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (c *Client) setting(ctx context.Context, key string) (string, error) {
	var value string
	if err := c.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&value); err != nil {
		return "", err
	}
	return value, nil
}

func (c *Client) request(ctx context.Context, method, path, authorization string, body, dst interface{}) (int, []byte, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	status, response, err := c.requestRaw(ctx, method, path, authorization, payload)
	if err == nil && dst != nil && len(response) > 0 {
		err = json.Unmarshal(response, dst)
	}
	return status, response, err
}

func (c *Client) requestRaw(ctx context.Context, method, path, authorization string, payload []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.controlURL+path, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	response, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, response, err
}

func responsePayload(response []byte, err error) interface{} {
	if err != nil {
		return map[string]string{"error": err.Error()}
	}
	var value interface{}
	if json.Unmarshal(response, &value) == nil {
		return value
	}
	return map[string]string{"response": string(response)}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
