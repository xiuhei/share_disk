package controlapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/share-disk/share-disk/internal/version"
	"github.com/share-disk/share-disk/server/internal/catalog"
	"github.com/share-disk/share-disk/server/internal/health"
	"github.com/share-disk/share-disk/server/internal/identity"
	"github.com/share-disk/share-disk/server/internal/transfer"
)

// maxBodyBytes bounds request bodies. Authentication payloads are small; a
// larger limit would allow unbounded memory allocation.
const maxBodyBytes = 1 << 20 // 1 MiB

// Handler handles HTTP requests
type Handler struct {
	identityService *identity.Service
	healthManager   *health.Manager
	tokenManager    *identity.TokenManager
	catalogRepo     *catalog.Repository
	transferService *transfer.Service
}

// SetTransferService enables the transfer scheduling endpoints. It is kept
// separate from construction so small auth-only handler tests need no database.
func (h *Handler) SetTransferService(service *transfer.Service) { h.transferService = service }

// NewHandler creates a new Handler
func NewHandler(identityService *identity.Service, healthManager *health.Manager, tokenManager *identity.TokenManager, catalogRepos ...*catalog.Repository) *Handler {
	h := &Handler{
		identityService: identityService,
		healthManager:   healthManager,
		tokenManager:    tokenManager,
	}
	if len(catalogRepos) > 0 {
		h.catalogRepo = catalogRepos[0]
	}
	return h
}

type ctxKey int

const (
	ctxKeyClaims ctxKey = iota
)

// ClaimsFromContext returns the authenticated token claims.
func ClaimsFromContext(ctx context.Context) (*identity.TokenClaims, bool) {
	claims, ok := ctx.Value(ctxKeyClaims).(*identity.TokenClaims)
	return claims, ok
}

// NewRouter creates a new router with all routes
func NewRouter(h *Handler) *chi.Mux {
	return newRouter(h, false)
}

// NewMonitoringRouter creates the separately hosted operations console. API
// routes are included because the browser shell uses same-origin requests.
func NewMonitoringRouter(h *Handler) *chi.Mux {
	return newRouter(h, true)
}

func newRouter(h *Handler, monitoring bool) *chi.Mux {
	r := chi.NewRouter()

	// Middleware. Note: middleware.RealIP is intentionally NOT used because it
	// trusts forwarded headers unconditionally and is vulnerable to IP
	// spoofing. Forwarded headers are only honored behind an explicitly
	// configured trusted proxy boundary.
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(middleware.AllowContentType("application/json"))

	// Health endpoints
	r.Get("/livez", h.Livez)
	r.Get("/readyz", h.Readyz)
	r.Get("/version", h.Version)
	if monitoring {
		r.Get("/admin", h.AdminConsole)
		r.Get("/admin/", h.AdminConsole)
	}
	r.Get("/v1/public/shares/{token}", h.ResolveShare)

	// Auth endpoints
	r.Route("/v1/auth", func(r chi.Router) {
		r.Post("/bootstrap", h.Bootstrap)
		r.Post("/login", h.Login)
		r.Post("/enroll", h.EnrollDevice)
		r.Post("/refresh", h.Refresh)
		r.Get("/jwks", h.JWKS)
		r.Group(func(r chi.Router) {
			r.Use(h.requireAuth)
			r.Post("/logout", h.Logout)
		})
	})

	r.Route("/v1/catalog", func(r chi.Router) {
		r.Use(h.requireAuth)
		r.Get("/files", h.ListCatalogFiles)
		r.Get("/files/{fileID}", h.GetCatalogFile)
		r.Post("/files/register", h.RegisterCatalogFile)
		r.Post("/files/move", h.MoveCatalogFiles)
		r.Post("/files/actions", h.ApplyCatalogActions)
		r.Get("/trash", h.ListCatalogTrash)
		r.Post("/lifecycle", h.ReportCatalogLifecycle)
		r.Get("/folders", h.ListFolders)
		r.Post("/folders", h.CreateFolder)
		r.Patch("/folders/{folderID}", h.RenameFolder)
		r.Delete("/folders/{folderID}", h.DeleteFolder)
	})
	r.With(h.requireAuth).Get("/v1/shares", h.ListShares)
	r.With(h.requireAuth).Post("/v1/shares", h.CreateShare)
	r.With(h.requireAuth).Delete("/v1/shares/{shareID}", h.RevokeShare)

	r.With(h.requireAuth).Post("/v1/devices/register", h.RegisterDevice)
	r.With(h.requireAuth).Post("/v1/devices/heartbeat", h.DeviceHeartbeat)
	r.With(h.requireAuth).Get("/v1/devices", h.ListDevices)
	r.With(h.requireAuth).Delete("/v1/devices/{deviceID}", h.DeregisterDevice)
	r.With(h.requireAuth).Get("/v1/account", h.GetAccount)
	r.With(h.requireAuth).Patch("/v1/account/password", h.ChangePassword)
	r.Route("/v1/device-commands", func(r chi.Router) {
		r.Use(h.requireAuth)
		r.Post("/claim", h.ClaimDeviceCommand)
		r.Post("/{commandID}/finish", h.FinishDeviceCommand)
	})
	r.Route("/v1/transfers", func(r chi.Router) {
		r.Use(h.requireAuth)
		r.Get("/", h.ListTransfers)
		r.Post("/", h.CreateTransfer)
		r.Post("/claim", h.ClaimTransfer)
		r.Post("/{taskID}/state", h.UpdateTransferState)
		r.Post("/{taskID}/complete", h.CompleteTransfer)
		r.Post("/{taskID}/fail", h.FailTransfer)
		r.Post("/{taskID}/cancel", h.CancelTransfer)
	})

	return r
}

func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	account, err := h.identityService.GetAccount(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to read account")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"account": account})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	var req identity.ChangePasswordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}
	if err := h.identityService.ChangePassword(r.Context(), claims.UserID, claims.SessionID, req); err != nil {
		switch {
		case isValidationError(err):
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, identity.ErrInvalidCredentials):
			writeError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Current password is incorrect")
		default:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to change password")
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password_changed"})
}

func (h *Handler) ListShares(w http.ResponseWriter, r *http.Request) {
	if h.catalogRepo == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Catalog is not configured")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	shares, err := h.catalogRepo.ListShares(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list shares")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"shares": shares})
}

func (h *Handler) CreateShare(w http.ResponseWriter, r *http.Request) {
	if h.catalogRepo == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Catalog is not configured")
		return
	}
	var req struct {
		FileID           string `json:"file_id"`
		ExpiresInSeconds int64  `json:"expires_in_seconds"`
		MaxDownloads     int    `json:"max_downloads"`
	}
	if decodeJSON(w, r, &req) != nil || req.FileID == "" {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid share request")
		return
	}
	if req.ExpiresInSeconds == 0 {
		req.ExpiresInSeconds = 24 * 60 * 60
	}
	claims, _ := ClaimsFromContext(r.Context())
	share, err := h.catalogRepo.CreateShare(r.Context(), claims.UserID, req.FileID, time.Duration(req.ExpiresInSeconds)*time.Second, req.MaxDownloads)
	if err != nil {
		writeShareError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, share)
}

func (h *Handler) ResolveShare(w http.ResponseWriter, r *http.Request) {
	if h.catalogRepo == nil || h.tokenManager == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Sharing is not configured")
		return
	}
	resolved, err := h.catalogRepo.ResolveShare(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		writeShareError(w, r, err)
		return
	}
	ttl := time.Until(resolved.Share.ExpiresAt)
	accessToken, err := h.tokenManager.GenerateShareAccessToken(resolved.UserID, resolved.File.ContentLocalFileID, resolved.Share.ID, ttl)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create download token")
		return
	}
	contentURL := strings.TrimRight(resolved.File.ReplicaEndpoint, "/") + "/v1/lan/files/" + url.PathEscape(resolved.File.ContentLocalFileID) + "/content"
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"share": resolved.Share, "file": resolved.File, "content_url": contentURL,
		"access_token": accessToken, "access_token_expires_in": h.tokenManager.AccessTokenTTL(),
	})
}

func (h *Handler) RevokeShare(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.catalogRepo.RevokeShare(r.Context(), claims.UserID, chi.URLParam(r, "shareID")); err != nil {
		writeShareError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeShareError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrShareNotFound):
		writeError(w, r, http.StatusNotFound, "SHARE_NOT_FOUND", "Share not found")
	case errors.Is(err, catalog.ErrShareExpired):
		writeError(w, r, http.StatusGone, "SHARE_EXPIRED", "Share expired or download limit reached")
	default:
		writeCatalogError(w, r, err)
	}
}

func (h *Handler) ApplyCatalogActions(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OperationID string   `json:"operation_id"`
		FileIDs     []string `json:"file_ids"`
		Action      string   `json:"action"`
		Name        string   `json:"name"`
	}
	if decodeJSON(w, r, &req) != nil || req.OperationID == "" || len(req.FileIDs) == 0 || len(req.FileIDs) > 500 || (req.Action == "rename" && len(req.FileIDs) != 1) {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid file action request")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	files := make([]*catalog.UnifiedFile, 0, len(req.FileIDs))
	for index, id := range req.FileIDs {
		file, err := h.catalogRepo.GetUnifiedFile(r.Context(), claims.UserID, id)
		if err != nil {
			writeCatalogError(w, r, err)
			return
		}
		report := catalog.LifecycleReport{OperationID: req.OperationID + ":" + strconv.Itoa(index), LocalFileID: file.LocalFileID, Action: req.Action, Name: req.Name}
		updated, err := h.catalogRepo.ApplyLifecycleReport(r.Context(), claims.UserID, file.OriginDeviceID, report)
		if err != nil {
			writeCatalogError(w, r, err)
			return
		}
		if err := h.catalogRepo.QueueOriginLifecycleCommand(r.Context(), claims.UserID, file.ID, report); err != nil {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to queue file action")
			return
		}
		files = append(files, updated)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"files": files})
}

func (h *Handler) DeregisterDevice(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.identityService.DeregisterDevice(r.Context(), claims.UserID, chi.URLParam(r, "deviceID")); err != nil {
		if errors.Is(err, identity.ErrDeviceNotFound) {
			writeError(w, r, http.StatusNotFound, "DEVICE_NOT_FOUND", "Device not found")
		} else {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to deregister device")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ClaimDeviceCommand(w http.ResponseWriter, r *http.Request) {
	if h.catalogRepo == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Device coordination is not configured")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	command, err := h.catalogRepo.ClaimDeviceCommand(r.Context(), claims.UserID, claims.DeviceID, 2*time.Minute)
	if errors.Is(err, catalog.ErrCommandNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to claim device command")
		return
	}
	writeJSON(w, http.StatusOK, command)
}

func (h *Handler) FinishDeviceCommand(w http.ResponseWriter, r *http.Request) {
	if h.catalogRepo == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Device coordination is not configured")
		return
	}
	var req struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid command result")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.catalogRepo.FinishDeviceCommand(r.Context(), claims.UserID, claims.DeviceID, chi.URLParam(r, "commandID"), req.Success, req.Error); err != nil {
		if errors.Is(err, catalog.ErrCommandNotFound) {
			writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Device command not found")
		} else {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to finish device command")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) ListTransfers(w http.ResponseWriter, r *http.Request) {
	if h.transferService == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Transfer scheduling is not configured")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	claims, _ := ClaimsFromContext(r.Context())
	tasks, err := h.transferService.ListTasks(r.Context(), claims.UserID, r.URL.Query().Get("state"), limit)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list transfers")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"transfers": tasks})
}

func (h *Handler) CancelTransfer(w http.ResponseWriter, r *http.Request) {
	if h.transferService == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Transfer scheduling is not configured")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.transferService.CancelTask(r.Context(), claims.UserID, chi.URLParam(r, "taskID")); err != nil {
		writeTransferError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) EnrollDevice(w http.ResponseWriter, r *http.Request) {
	var req identity.EnrollDeviceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}
	resp, err := h.identityService.EnrollDevice(r.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, identity.ErrInvalidCredentials):
			writeError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials")
		case errors.Is(err, identity.ErrDeviceConflict):
			writeError(w, r, http.StatusConflict, "DEVICE_CONFLICT", "Device is already registered to another account")
		case isValidationError(err):
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		default:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) ListDevices(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	devices, err := h.identityService.ListDevices(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list devices")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"devices": devices})
}

func (h *Handler) CreateTransfer(w http.ResponseWriter, r *http.Request) {
	if h.transferService == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Transfer scheduling is not configured")
		return
	}
	var req struct {
		FileID         string `json:"file_id"`
		TargetDeviceID string `json:"target_device_id"`
		Priority       int    `json:"priority"`
	}
	if err := decodeJSON(w, r, &req); err != nil || req.FileID == "" || req.TargetDeviceID == "" || req.Priority < -100 || req.Priority > 100 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid transfer request")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	task, err := h.transferService.CreateReplicaTask(r.Context(), claims.UserID, req.FileID, req.TargetDeviceID, req.Priority)
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, task)
}

func (h *Handler) ClaimTransfer(w http.ResponseWriter, r *http.Request) {
	if h.transferService == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Transfer scheduling is not configured")
		return
	}
	var req struct {
		LeaseSeconds int `json:"lease_seconds"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid claim request")
		return
	}
	if req.LeaseSeconds == 0 {
		req.LeaseSeconds = 120
	}
	claims, _ := ClaimsFromContext(r.Context())
	a, err := h.transferService.ClaimNext(r.Context(), claims.UserID, claims.DeviceID, time.Duration(req.LeaseSeconds)*time.Second)
	if errors.Is(err, transfer.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (h *Handler) UpdateTransferState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedVersion int64  `json:"expected_version"`
		State           string `json:"state"`
	}
	if h.transferService == nil || decodeJSON(w, r, &req) != nil || req.ExpectedVersion <= 0 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid transfer state request")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	err := h.transferService.UpdateState(r.Context(), claims.UserID, chi.URLParam(r, "taskID"), claims.DeviceID, req.ExpectedVersion, req.State)
	if err != nil {
		writeTransferError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) CompleteTransfer(w http.ResponseWriter, r *http.Request) {
	var req transfer.CompleteRequest
	if h.transferService == nil || decodeJSON(w, r, &req) != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid completion report")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.transferService.CompleteReplica(r.Context(), claims.UserID, claims.DeviceID, chi.URLParam(r, "taskID"), req); err != nil {
		writeTransferError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) FailTransfer(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExpectedVersion int64 `json:"expected_version"`
	}
	if h.transferService == nil || decodeJSON(w, r, &req) != nil || req.ExpectedVersion <= 0 {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid failure report")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.transferService.FailAttempt(r.Context(), claims.UserID, claims.DeviceID, chi.URLParam(r, "taskID"), req.ExpectedVersion); err != nil {
		writeTransferError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeTransferError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, transfer.ErrNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Transfer resource not found")
	case errors.Is(err, transfer.ErrNoSource):
		writeError(w, r, http.StatusConflict, "NO_SOURCE", "No ready source replica is available")
	case errors.Is(err, transfer.ErrAlreadyReplica):
		writeError(w, r, http.StatusConflict, "ALREADY_REPLICATED", "Target already has a ready replica")
	case errors.Is(err, transfer.ErrNotClaimable), errors.Is(err, transfer.ErrInvalidTransition):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", "Transfer state does not allow this operation")
	case errors.Is(err, transfer.ErrVersionConflict):
		writeError(w, r, http.StatusConflict, "VERSION_CONFLICT", "Transfer version changed")
	default:
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid transfer request")
	}
}

func (h *Handler) DeviceHeartbeat(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.identityService.Heartbeat(r.Context(), claims.UserID, claims.DeviceID); err != nil {
		if errors.Is(err, identity.ErrDeviceNotFound) {
			writeError(w, r, http.StatusNotFound, "DEVICE_NOT_FOUND", "Device not found")
		} else {
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RegisterDevice exchanges an account grant for a separate Linux Agent
// session, avoiding long-term reuse of a phone session.
func (h *Handler) RegisterDevice(w http.ResponseWriter, r *http.Request) {
	var req identity.RegisterDeviceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	resp, err := h.identityService.RegisterDevice(r.Context(), claims.UserID, &req)
	if err != nil {
		switch {
		case isValidationError(err):
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, identity.ErrDeviceConflict):
			writeError(w, r, http.StatusConflict, "DEVICE_CONFLICT", "Device is already registered to another account")
		default:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		}
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (h *Handler) RegisterCatalogFile(w http.ResponseWriter, r *http.Request) {
	if h.catalogRepo == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Catalog is not configured")
		return
	}
	var req catalog.RegisterFileRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	file, err := h.catalogRepo.RegisterVerifiedFile(r.Context(), claims.UserID, claims.DeviceID, req)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, file)
}

func (h *Handler) ListCatalogFiles(w http.ResponseWriter, r *http.Request) {
	h.listCatalog(w, r, false)
}

func (h *Handler) ListCatalogTrash(w http.ResponseWriter, r *http.Request) {
	h.listCatalog(w, r, true)
}

func (h *Handler) listCatalog(w http.ResponseWriter, r *http.Request, trash bool) {
	if h.catalogRepo == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Catalog is not configured")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	files, err := h.catalogRepo.QueryUnifiedFiles(r.Context(), claims.UserID, catalog.FileQuery{Trash: trash, FolderID: r.URL.Query().Get("folder_id"), Search: r.URL.Query().Get("q"), Sort: r.URL.Query().Get("sort")})
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list catalog")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"files": files})
}

func (h *Handler) GetCatalogFile(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	file, err := h.catalogRepo.GetUnifiedFile(r.Context(), claims.UserID, chi.URLParam(r, "fileID"))
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, file)
}

func (h *Handler) ListFolders(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	folders, err := h.catalogRepo.ListAllFolders(r.Context(), claims.UserID)
	if err != nil {
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list folders")
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"folders": folders})
}

func (h *Handler) CreateFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ParentID string `json:"parent_id"`
		Name     string `json:"name"`
	}
	if decodeJSON(w, r, &req) != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid folder request")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	folder, err := h.catalogRepo.CreateManagedFolder(r.Context(), claims.UserID, req.ParentID, req.Name)
	if err != nil {
		writeFolderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, folder)
}

func (h *Handler) RenameFolder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if decodeJSON(w, r, &req) != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid folder request")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	folder, err := h.catalogRepo.RenameManagedFolder(r.Context(), claims.UserID, chi.URLParam(r, "folderID"), req.Name)
	if err != nil {
		writeFolderError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, folder)
}

func (h *Handler) DeleteFolder(w http.ResponseWriter, r *http.Request) {
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.catalogRepo.DeleteManagedFolder(r.Context(), claims.UserID, chi.URLParam(r, "folderID")); err != nil {
		writeFolderError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) MoveCatalogFiles(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FileIDs  []string `json:"file_ids"`
		FolderID string   `json:"folder_id"`
	}
	if decodeJSON(w, r, &req) != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid move request")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.catalogRepo.MoveFiles(r.Context(), claims.UserID, req.FolderID, req.FileIDs); err != nil {
		writeFolderError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeFolderError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrFolderNotFound), errors.Is(err, catalog.ErrFileNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Folder or file not found")
	case errors.Is(err, catalog.ErrFolderNotEmpty), errors.Is(err, catalog.ErrNameConflict):
		writeError(w, r, http.StatusConflict, "CONFLICT", "Folder is not empty or name already exists")
	case errors.Is(err, catalog.ErrInvalidRequest):
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid folder request")
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Folder operation failed")
	}
}

func (h *Handler) ReportCatalogLifecycle(w http.ResponseWriter, r *http.Request) {
	if h.catalogRepo == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "Catalog is not configured")
		return
	}
	var report catalog.LifecycleReport
	if err := decodeJSON(w, r, &report); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	file, err := h.catalogRepo.ApplyLifecycleReport(r.Context(), claims.UserID, claims.DeviceID, report)
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, file)
}

func writeCatalogError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, catalog.ErrFileNotFound):
		writeError(w, r, http.StatusNotFound, "NOT_FOUND", "Catalog file not found")
	case errors.Is(err, catalog.ErrNameConflict):
		writeError(w, r, http.StatusConflict, "NAME_CONFLICT", "An active file already uses this name")
	case errors.Is(err, catalog.ErrInvalidState):
		writeError(w, r, http.StatusConflict, "INVALID_STATE", "File state does not allow this operation")
	case errors.Is(err, catalog.ErrInvalidRequest):
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid catalog request")
	default:
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
	}
}

// errorBody matches the OpenAPI Error schema.
type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// errorResponse wraps the error payload in the OpenAPI shape.
type errorResponse struct {
	Error errorBody `json:"error"`
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes an error response in the OpenAPI shape.
func writeError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	writeJSON(w, status, errorResponse{
		Error: errorBody{
			Code:      code,
			Message:   message,
			RequestID: middleware.GetReqID(r.Context()),
		},
	})
}

// decodeJSON decodes a single JSON object from the request body with a size
// limit, unknown-field rejection, and trailing-data detection.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst interface{}) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		return err
	}

	// Reject trailing data so callers cannot smuggle a second JSON value.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}

// Livez handles liveness probe
func (h *Handler) Livez(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}

// Readyz handles readiness probe using the registered health checks.
func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
	if h.healthManager == nil {
		writeJSON(w, http.StatusOK, map[string]string{"status": "UP"})
		return
	}

	status := http.StatusOK
	result := h.healthManager.RunChecks()
	if result.Status == health.StatusDown {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, result)
}

// Version handles version request
func (h *Handler) Version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, version.Get())
}

// JWKS exposes the access-token verification public key so Agents can fetch
// trusted key material without ever receiving the server's signing private key.
func (h *Handler) JWKS(w http.ResponseWriter, r *http.Request) {
	if h.tokenManager == nil {
		writeError(w, r, http.StatusServiceUnavailable, "UNAVAILABLE", "No token signing key configured")
		return
	}
	writeJSON(w, http.StatusOK, h.tokenManager.JWKS())
}

// Bootstrap handles first account creation
func (h *Handler) Bootstrap(w http.ResponseWriter, r *http.Request) {
	var req identity.BootstrapRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	resp, err := h.identityService.Bootstrap(r.Context(), &req)
	if err != nil {
		switch {
		case isValidationError(err):
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, identity.ErrInvalidBootstrapToken):
			writeError(w, r, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid bootstrap token")
		case errors.Is(err, identity.ErrAlreadyBootstrapped):
			writeError(w, r, http.StatusGone, "ALREADY_BOOTSTRAPPED", "Bootstrap already completed")
		default:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		}
		return
	}

	writeJSON(w, http.StatusCreated, resp)
}

// Login handles user login
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req identity.LoginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	resp, err := h.identityService.Login(r.Context(), &req)
	if err != nil {
		switch {
		case isValidationError(err):
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, identity.ErrInvalidCredentials):
			writeError(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid credentials")
		case errors.Is(err, identity.ErrDeviceNotFound):
			writeError(w, r, http.StatusUnauthorized, "DEVICE_NOT_FOUND", "Device not found")
		default:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// Refresh handles token refresh
func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req identity.RefreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	resp, err := h.identityService.Refresh(r.Context(), &req)
	if err != nil {
		switch {
		case isValidationError(err):
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, identity.ErrInvalidRefreshToken):
			writeError(w, r, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid refresh token")
		default:
			writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		}
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

// Logout handles user logout using the authenticated session.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Not authenticated")
		return
	}

	if err := h.identityService.Logout(r.Context(), claims.UserID, claims.SessionID); err != nil {
		if isValidationError(err) {
			writeError(w, r, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		if errors.Is(err, identity.ErrSessionNotFound) {
			writeError(w, r, http.StatusNotFound, "SESSION_NOT_FOUND", "Session not found")
			return
		}
		writeError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// requireAuth is an HTTP middleware that requires a valid Bearer access token
// and re-checks server-side session, device, and user state.
func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Missing or malformed Authorization header")
			return
		}

		claims, err := h.identityService.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired token")
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyClaims, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

// isValidationError reports whether err is (or wraps) an identity.ValidationError.
func isValidationError(err error) bool {
	var ve *identity.ValidationError
	return errors.As(err, &ve)
}
