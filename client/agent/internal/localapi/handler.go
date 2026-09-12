package localapi

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	sharediskv1 "github.com/share-disk/share-disk/contracts/proto/sharedisk/v1"

	"github.com/share-disk/share-disk/client/agent/internal/storage"
	storagecatalog "github.com/share-disk/share-disk/client/agent/internal/storage/catalog"
)

// LocalHandler implements the local IPC service by delegating to the agent's
// SQLite-backed object store, which is the authoritative local source of truth.
type LocalHandler struct {
	store          *storage.SQLiteObjectStore
	st             *storage.Storage
	deviceID       string
	version        string
	db             *sql.DB
	endpoint       string
	trashRetention time.Duration
	coordinator    interface {
		Enroll(context.Context, string, string) error
	}
}

// SetCoordinator enables local CLI enrollment without exposing credentials on
// the LAN API.
func (h *LocalHandler) SetCoordinator(coordinator interface {
	Enroll(context.Context, string, string) error
}) {
	h.coordinator = coordinator
}

// NewLocalHandler creates a LocalHandler.
func NewLocalHandler(store *storage.SQLiteObjectStore, st *storage.Storage, deviceID, version string, db *sql.DB, endpoint string, trashRetention time.Duration) *LocalHandler {
	return &LocalHandler{store: store, st: st, deviceID: deviceID, version: version, db: db, endpoint: endpoint, trashRetention: trashRetention}
}

// Handle dispatches a request by its oneof payload type.
func (h *LocalHandler) Handle(ctx context.Context, req *sharediskv1.LocalRequest) *sharediskv1.LocalResponse {
	switch p := req.GetPayload().(type) {
	case *sharediskv1.LocalRequest_Import:
		return h.handleImport(ctx, p.Import)
	case *sharediskv1.LocalRequest_Stat:
		return h.handleStat(ctx, p.Stat)
	case *sharediskv1.LocalRequest_Verify:
		return h.handleVerify(ctx, p.Verify)
	case *sharediskv1.LocalRequest_Status:
		return h.handleStatus(ctx, p.Status)
	case *sharediskv1.LocalRequest_ListTransfers:
		return h.handleListTransfers(ctx, p.ListTransfers)
	case *sharediskv1.LocalRequest_CancelTransfer:
		return h.handleCancelTransfer(ctx, p.CancelTransfer)
	case *sharediskv1.LocalRequest_ListLocalFiles:
		return h.handleListLocalFiles(ctx, p.ListLocalFiles)
	case *sharediskv1.LocalRequest_MutateLocalFile:
		return h.handleMutateLocalFile(ctx, p.MutateLocalFile)
	case *sharediskv1.LocalRequest_Export:
		return h.handleExport(ctx, p.Export)
	case *sharediskv1.LocalRequest_CoordinatorEnroll:
		return h.handleCoordinatorEnroll(ctx, p.CoordinatorEnroll)
	default:
		return errResponse("INVALID_REQUEST", "unknown or missing request payload")
	}
}

func (h *LocalHandler) handleCoordinatorEnroll(ctx context.Context, req *sharediskv1.CoordinatorEnrollRequest) *sharediskv1.LocalResponse {
	if h.coordinator == nil {
		return errResponse("COORDINATOR_DISABLED", "control server is not configured")
	}
	if err := h.coordinator.Enroll(ctx, req.GetAccount(), req.GetPassword()); err != nil {
		return errResponse("ENROLL_FAILED", err.Error())
	}
	return &sharediskv1.LocalResponse{Payload: &sharediskv1.LocalResponse_CoordinatorEnroll{CoordinatorEnroll: &sharediskv1.CoordinatorEnrollResponse{Provisioned: true}}}
}

func (h *LocalHandler) handleExport(ctx context.Context, req *sharediskv1.ExportRequest) *sharediskv1.LocalResponse {
	if req.GetFileId() == "" || !filepath.IsAbs(req.GetDestinationPath()) {
		return errResponse("INVALID_REQUEST", "file_id and absolute destination_path are required")
	}
	owner, err := h.ownerUserID(ctx)
	if err != nil {
		return errResponse("COORDINATOR_NOT_PROVISIONED", "Agent is not provisioned")
	}
	file, err := h.store.GetStore().GetLANFile(req.GetFileId(), owner)
	if err != nil || file == nil {
		return errResponse("NOT_FOUND", "file not found")
	}
	hash, err := storage.ParseHash(file.SHA256)
	if err != nil {
		return errResponse("EXPORT_FAILED", "invalid stored object")
	}
	info, err := h.store.GetObject(hash)
	if err != nil {
		return errResponse("EXPORT_FAILED", err.Error())
	}
	destination := filepath.Clean(req.GetDestinationPath())
	if _, err := os.Lstat(destination); err == nil {
		return errResponse("DESTINATION_EXISTS", "destination already exists")
	} else if !os.IsNotExist(err) {
		return errResponse("EXPORT_FAILED", err.Error())
	}
	source, err := os.Open(info.Path)
	if err != nil {
		return errResponse("EXPORT_FAILED", err.Error())
	}
	defer source.Close()
	temp, err := os.CreateTemp(filepath.Dir(destination), ".share-disk-export-*")
	if err != nil {
		return errResponse("EXPORT_FAILED", err.Error())
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	written, copyErr := io.Copy(temp, source)
	if copyErr == nil {
		copyErr = temp.Sync()
	}
	if closeErr := temp.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil || written != file.Size {
		return errResponse("EXPORT_FAILED", "failed to write complete destination")
	}
	if err := os.Link(tempPath, destination); err != nil {
		return errResponse("EXPORT_FAILED", err.Error())
	}
	return &sharediskv1.LocalResponse{Payload: &sharediskv1.LocalResponse_Export{Export: &sharediskv1.ExportResponse{DestinationPath: destination, Size: uint64(written), Sha256: file.SHA256}}}
}

func (h *LocalHandler) handleImport(ctx context.Context, req *sharediskv1.ImportRequest) *sharediskv1.LocalResponse {
	if req.GetSourcePath() == "" {
		return errResponse("INVALID_REQUEST", "source_path is required")
	}

	info, err := h.store.Import(ctx, req.GetSourcePath())
	if err != nil {
		return errResponse("IMPORT_FAILED", err.Error())
	}

	owner, ownerErr := h.ownerUserID(ctx)
	if ownerErr != nil {
		return errResponse("COORDINATOR_NOT_PROVISIONED", "provision the Agent before importing account files")
	}
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		name = filepath.Base(req.GetSourcePath())
	}
	normalized, normalizeErr := storagecatalog.NormalizeName(name)
	if normalizeErr != nil {
		return errResponse("INVALID_NAME", normalizeErr.Error())
	}
	entryID := uuid.NewString()
	if _, err := h.store.GetStore().CreateLANFileCoordinated(ctx, entryID, owner, info.Hash.String(), name, normalized, "application/octet-stream", h.endpoint); err != nil {
		return localLifecycleError(err)
	}
	return &sharediskv1.LocalResponse{
		Payload: &sharediskv1.LocalResponse_Import{
			Import: &sharediskv1.ImportResponse{
				FileEntryId:  entryID,
				FileObjectId: info.Hash.String(),
				Sha256:       info.Hash[:],
				Size:         uint64(info.Size),
			},
		},
	}
}

func (h *LocalHandler) ownerUserID(ctx context.Context) (string, error) {
	if h.db == nil {
		return "", sql.ErrNoRows
	}
	var owner string
	err := h.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key='control_user_id'`).Scan(&owner)
	if err == sql.ErrNoRows && h.endpoint == "" {
		return "local-" + h.deviceID, nil
	}
	return owner, err
}

func (h *LocalHandler) handleListLocalFiles(ctx context.Context, req *sharediskv1.ListLocalFilesRequest) *sharediskv1.LocalResponse {
	owner, err := h.ownerUserID(ctx)
	if err != nil {
		return errResponse("COORDINATOR_NOT_PROVISIONED", "Agent is not provisioned")
	}
	var files []storage.LANFile
	if req.GetTrash() {
		files, err = h.store.GetStore().ListLANTrash(owner)
	} else {
		files, err = h.store.GetStore().ListLANFiles(owner)
	}
	if err != nil {
		return errResponse("INTERNAL", "failed to list local files")
	}
	output := make([]*sharediskv1.LocalFileRecord, 0, len(files))
	for i := range files {
		output = append(output, localFileToProto(&files[i]))
	}
	return &sharediskv1.LocalResponse{Payload: &sharediskv1.LocalResponse_ListLocalFiles{ListLocalFiles: &sharediskv1.ListLocalFilesResponse{Files: output}}}
}

func (h *LocalHandler) handleMutateLocalFile(ctx context.Context, req *sharediskv1.MutateLocalFileRequest) *sharediskv1.LocalResponse {
	owner, err := h.ownerUserID(ctx)
	if err != nil {
		return errResponse("COORDINATOR_NOT_PROVISIONED", "Agent is not provisioned")
	}
	coordinated := h.endpoint != ""
	var file *storage.LANFile
	switch req.GetAction() {
	case "rename":
		name := strings.TrimSpace(req.GetName())
		normalized, normalizeErr := storagecatalog.NormalizeName(name)
		if normalizeErr != nil {
			return errResponse("INVALID_NAME", normalizeErr.Error())
		}
		file, err = h.store.GetStore().RenameLANFileCoordinated(ctx, req.GetFileId(), owner, name, normalized, coordinated)
	case "trash":
		file, err = h.store.GetStore().TrashLANFileCoordinated(ctx, req.GetFileId(), owner, h.trashRetention, coordinated)
	case "restore":
		file, err = h.store.GetStore().RestoreLANFileCoordinated(ctx, req.GetFileId(), owner, coordinated)
	case "purge":
		if coordinated {
			err = h.store.PurgeLANFileCoordinated(ctx, req.GetFileId(), owner)
		} else {
			err = h.store.PurgeLANFile(ctx, req.GetFileId(), owner)
		}
	default:
		return errResponse("INVALID_REQUEST", "action must be rename, trash, restore, or purge")
	}
	if err != nil {
		return localLifecycleError(err)
	}
	return &sharediskv1.LocalResponse{Payload: &sharediskv1.LocalResponse_MutateLocalFile{MutateLocalFile: &sharediskv1.MutateLocalFileResponse{File: localFileToProto(file)}}}
}

func localFileToProto(file *storage.LANFile) *sharediskv1.LocalFileRecord {
	if file == nil {
		return nil
	}
	record := &sharediskv1.LocalFileRecord{Id: file.ID, Name: file.Name, Mime: file.MIME, Size: uint64(file.Size), Sha256: file.SHA256, Status: file.Status}
	if file.DeletedAt != nil {
		record.DeletedAt = file.DeletedAt.UTC().Format(time.RFC3339Nano)
	}
	if file.PurgeAfter != nil {
		record.PurgeAfter = file.PurgeAfter.UTC().Format(time.RFC3339Nano)
	}
	return record
}

func localLifecycleError(err error) *sharediskv1.LocalResponse {
	switch {
	case errors.Is(err, storage.ErrLANFileNotFound):
		return errResponse("NOT_FOUND", "file not found")
	case errors.Is(err, storage.ErrLANNameConflict):
		return errResponse("NAME_CONFLICT", "an active file already uses this name")
	case errors.Is(err, storage.ErrLANInvalidState):
		return errResponse("INVALID_STATE", "file state does not allow this operation")
	default:
		return errResponse("FILE_OPERATION_FAILED", err.Error())
	}
}

func (h *LocalHandler) handleStat(ctx context.Context, req *sharediskv1.StatRequest) *sharediskv1.LocalResponse {
	hash, err := storage.ParseHash(req.GetIdentifier())
	if err != nil {
		return errResponse("INVALID_REQUEST", "invalid object identifier")
	}

	info, err := h.store.GetObject(hash)
	if err != nil {
		return errResponse("NOT_FOUND", err.Error())
	}

	return &sharediskv1.LocalResponse{
		Payload: &sharediskv1.LocalResponse_Stat{
			Stat: &sharediskv1.StatResponse{
				Object: objectToProto(info),
			},
		},
	}
}

func (h *LocalHandler) handleVerify(ctx context.Context, req *sharediskv1.VerifyRequest) *sharediskv1.LocalResponse {
	hash, err := storage.ParseHash(req.GetObjectId())
	if err != nil {
		return errResponse("INVALID_REQUEST", "invalid object id")
	}

	if err := h.store.Verify(ctx, hash); err != nil {
		return &sharediskv1.LocalResponse{
			Payload: &sharediskv1.LocalResponse_Verify{
				Verify: &sharediskv1.VerifyResponse{Valid: false, Error: err.Error()},
			},
		}
	}
	return &sharediskv1.LocalResponse{
		Payload: &sharediskv1.LocalResponse_Verify{
			Verify: &sharediskv1.VerifyResponse{Valid: true},
		},
	}
}

func (h *LocalHandler) handleStatus(ctx context.Context, req *sharediskv1.StatusRequest) *sharediskv1.LocalResponse {
	objects, err := h.store.ListObjects()
	if err != nil {
		return errResponse("INTERNAL", err.Error())
	}

	var used int64
	for i := range objects {
		used += objects[i].Size
	}

	available, err := storage.GetDiskSpace(h.st.RootDir())
	if err != nil {
		available = 0
	}

	return &sharediskv1.LocalResponse{
		Payload: &sharediskv1.LocalResponse_Status{
			Status: &sharediskv1.StatusResponse{
				Version:          h.version,
				DeviceId:         h.deviceID,
				StorageRoot:      h.st.RootDir(),
				ObjectCount:      int64(len(objects)),
				StorageUsed:      uint64(used),
				StorageAvailable: uint64(available),
			},
		},
	}
}

func (h *LocalHandler) handleListTransfers(ctx context.Context, req *sharediskv1.ListTransfersRequest) *sharediskv1.LocalResponse {
	limit := int(req.GetLimit())
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := h.db.QueryContext(ctx, `SELECT task_id,object_id,state,attempt,created_at,updated_at FROM transfer_runs WHERE (?='' OR state=?) ORDER BY updated_at DESC LIMIT ?`, req.GetStatus(), req.GetStatus(), limit)
	if err != nil {
		return errResponse("INTERNAL", "failed to list transfers")
	}
	defer rows.Close()
	var transfers []*sharediskv1.Transfer
	for rows.Next() {
		item := &sharediskv1.Transfer{}
		if err := rows.Scan(&item.Id, &item.ObjectId, &item.State, &item.Attempt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return errResponse("INTERNAL", "failed to read transfers")
		}
		transfers = append(transfers, item)
	}
	return &sharediskv1.LocalResponse{Payload: &sharediskv1.LocalResponse_ListTransfers{ListTransfers: &sharediskv1.ListTransfersResponse{Transfers: transfers}}}
}

func (h *LocalHandler) handleCancelTransfer(ctx context.Context, req *sharediskv1.CancelTransferRequest) *sharediskv1.LocalResponse {
	if req.GetTransferId() == "" {
		return errResponse("INVALID_REQUEST", "transfer_id is required")
	}
	result, err := h.db.ExecContext(ctx, `UPDATE transfer_runs SET cancel_requested=1,updated_at=datetime('now') WHERE task_id=? AND state NOT IN ('completed','canceled','failed_permanent')`, req.GetTransferId())
	if err != nil {
		return errResponse("INTERNAL", "failed to request transfer cancellation")
	}
	count, _ := result.RowsAffected()
	return &sharediskv1.LocalResponse{Payload: &sharediskv1.LocalResponse_CancelTransfer{CancelTransfer: &sharediskv1.CancelTransferResponse{Canceled: count == 1}}}
}

func objectToProto(info *storage.ObjectInfo) *sharediskv1.FileObject {
	return &sharediskv1.FileObject{
		Id:         info.Hash.String(),
		Sha256:     info.Hash[:],
		Size:       uint64(info.Size),
		ChunkSize:  uint32(info.ChunkSize),
		ChunkCount: uint32(info.ChunkCount),
		Status:     string(info.Status),
	}
}

func errResponse(code, message string) *sharediskv1.LocalResponse {
	return &sharediskv1.LocalResponse{
		Payload: &sharediskv1.LocalResponse_Error{
			Error: &sharediskv1.LocalError{Code: code, Message: message},
		},
	}
}
