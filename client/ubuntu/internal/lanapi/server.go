// Package lanapi exposes the explicitly enabled, authenticated data plane used
// by Android clients on a trusted local network.
package lanapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tus/tusd/v2/pkg/filelocker"
	"github.com/tus/tusd/v2/pkg/filestore"
	tusd "github.com/tus/tusd/v2/pkg/handler"

	"github.com/share-disk/share-disk/client/ubuntu/internal/storage"
	storagecatalog "github.com/share-disk/share-disk/client/ubuntu/internal/storage/catalog"
	"github.com/share-disk/share-disk/internal/auth"
	"github.com/share-disk/share-disk/internal/version"
)

const uploadsPath = "/v1/lan/uploads/"

type claimsContextKey struct{}

// Server is the Agent's independently stoppable LAN HTTP server.
type Server struct {
	address     string
	http        *http.Server
	listener    net.Listener
	coordinator Coordinator
}

// Coordinator provisions the Agent and exposes durable sync status.
type Coordinator interface {
	Provision(context.Context, string, string) error
	Status(context.Context) (interface{}, error)
}

// New builds the LAN API without opening a socket.
func New(address, publicKeyPEM, incomingDir string, maxUploadSize int64, trashRetention time.Duration, replicaEndpoint string, store *storage.SQLiteObjectStore) (*Server, error) {
	if err := os.MkdirAll(incomingDir, 0700); err != nil {
		return nil, fmt.Errorf("create LAN incoming directory: %w", err)
	}

	tokenManager, err := auth.NewTokenVerifier(publicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse access public key: %w", err)
	}
	tusHandler, err := newTusHandler(incomingDir, maxUploadSize, replicaEndpoint, store)
	if err != nil {
		return nil, err
	}

	server := &Server{address: address}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	mux.HandleFunc("GET /version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, version.Get())
	})
	mux.Handle("/v1/lan/files", authenticate(tokenManager, http.HandlerFunc(listFiles(store))))
	mux.Handle("/v1/lan/files/", authenticate(tokenManager, http.HandlerFunc(fileRoute(store, trashRetention, replicaEndpoint != ""))))
	mux.Handle("/v1/lan/trash", authenticate(tokenManager, http.HandlerFunc(listTrash(store))))
	mux.Handle("/v1/lan/trash/", authenticate(tokenManager, http.HandlerFunc(trashRoute(store, replicaEndpoint != ""))))
	mux.Handle("/v1/lan/coordinator/provision", authenticate(tokenManager, http.HandlerFunc(server.provision)))
	mux.Handle("/v1/lan/coordinator/status", authenticate(tokenManager, http.HandlerFunc(server.coordinatorStatus)))
	mux.Handle(uploadsPath, authenticate(tokenManager, http.StripPrefix(strings.TrimSuffix(uploadsPath, "/"), tusHandler)))

	server.http = &http.Server{
		Addr:              address,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	return server, nil
}

// SetCoordinator attaches the control-plane client. Nil keeps standalone LAN
// mode available for an intentionally uncoordinated deployment.
func (s *Server) SetCoordinator(coordinator Coordinator) { s.coordinator = coordinator }

func (s *Server) provision(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if s.coordinator == nil {
		writeError(w, http.StatusServiceUnavailable, "COORDINATOR_DISABLED", "control-plane synchronization is not configured")
		return
	}
	claims, _ := claimsFromContext(r.Context())
	if err := s.coordinator.Provision(r.Context(), r.Header.Get("Authorization"), claims.UserID); err != nil {
		writeError(w, http.StatusBadGateway, "PROVISION_FAILED", "control-plane provisioning failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) coordinatorStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}
	if s.coordinator == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"configured": false, "provisioned": false})
		return
	}
	status, err := s.coordinator.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load coordinator status")
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func newTusHandler(incomingDir string, maxUploadSize int64, replicaEndpoint string, store *storage.SQLiteObjectStore) (http.Handler, error) {
	uploadDir := filepath.Join(incomingDir, "tus")
	lockDir := filepath.Join(incomingDir, "locks")
	if err := os.MkdirAll(uploadDir, 0700); err != nil {
		return nil, fmt.Errorf("create tus directory: %w", err)
	}
	if err := os.MkdirAll(lockDir, 0700); err != nil {
		return nil, fmt.Errorf("create tus lock directory: %w", err)
	}

	fileStore := filestore.New(uploadDir)
	fileStore.DirModePerm = 0700
	fileStore.FileModePerm = 0600
	locker := filelocker.New(lockDir)
	composer := tusd.NewStoreComposer()
	fileStore.UseIn(composer)
	locker.UseIn(composer)

	config := tusd.Config{
		StoreComposer:        composer,
		BasePath:             uploadsPath,
		MaxSize:              maxUploadSize,
		DisableDownload:      true,
		DisableConcatenation: true,
		PreUploadCreateCallback: func(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error) {
			claims, ok := claimsFromContext(hook.Context)
			if !ok {
				return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, tusd.NewError("UNAUTHORIZED", "authentication required", http.StatusUnauthorized)
			}
			name, normalized, mediaType, err := validateMetadata(hook.Upload.MetaData)
			if err != nil {
				return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, tusd.NewError("INVALID_METADATA", err.Error(), http.StatusBadRequest)
			}
			exists, err := store.GetStore().ActiveLANNameExists(claims.UserID, normalized)
			if err != nil {
				return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, fmt.Errorf("check active filename: %w", err)
			}
			if exists {
				return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, tusd.NewError("NAME_CONFLICT", "an active file already uses this name", http.StatusConflict)
			}
			metadata := tusd.MetaData{
				"filename":      name,
				"normalized":    normalized,
				"mime":          mediaType,
				"owner_user_id": claims.UserID,
			}
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{MetaData: metadata}, nil
		},
		PreFinishResponseCallback: func(hook tusd.HookEvent) (tusd.HTTPResponse, error) {
			claims, ok := claimsFromContext(hook.Context)
			if !ok || claims.UserID != hook.Upload.MetaData["owner_user_id"] {
				return tusd.HTTPResponse{}, tusd.NewError("FORBIDDEN", "upload owner mismatch", http.StatusForbidden)
			}

			existing, err := store.GetStore().GetLANFile(hook.Upload.ID, claims.UserID)
			if err != nil {
				return tusd.HTTPResponse{}, err
			}
			if existing != nil {
				return completionResponse(existing), nil
			}

			path := hook.Upload.Storage[filestore.StorageKeyPath]
			if path == "" {
				return tusd.HTTPResponse{}, errors.New("tus storage path is missing")
			}
			info, err := store.Import(hook.Context, path)
			if err != nil {
				return tusd.HTTPResponse{}, fmt.Errorf("publish completed upload: %w", err)
			}
			file, err := store.GetStore().CreateLANFileCoordinated(hook.Context,
				hook.Upload.ID,
				claims.UserID,
				info.Hash.String(),
				hook.Upload.MetaData["filename"],
				hook.Upload.MetaData["normalized"],
				hook.Upload.MetaData["mime"],
				replicaEndpoint,
			)
			if err != nil {
				if errors.Is(err, storage.ErrLANNameConflict) {
					return tusd.HTTPResponse{}, tusd.NewError("NAME_CONFLICT", "an active file already uses this name", http.StatusConflict)
				}
				return tusd.HTTPResponse{}, fmt.Errorf("publish file metadata: %w", err)
			}
			return completionResponse(file), nil
		},
	}

	return tusd.NewHandler(config)
}

func validateMetadata(metadata tusd.MetaData) (string, string, string, error) {
	name := strings.TrimSpace(metadata["filename"])
	if len(name) == 0 || len(name) > 255 {
		return "", "", "", fmt.Errorf("filename must contain 1 to 255 bytes")
	}
	normalized, err := storagecatalog.NormalizeName(name)
	if err != nil {
		return "", "", "", err
	}
	mediaType := strings.TrimSpace(metadata["mime"])
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	parsed, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		return "", "", "", fmt.Errorf("invalid MIME type")
	}
	return name, normalized, parsed, nil
}

func completionResponse(file *storage.LANFile) tusd.HTTPResponse {
	return tusd.HTTPResponse{Header: tusd.HTTPHeader{
		"X-Share-Disk-File-ID": file.ID,
		"X-Share-Disk-SHA256":  file.SHA256,
	}}
}

// Listen binds the configured address so startup failures are reported before
// the Agent announces readiness.
func (s *Server) Listen() error {
	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		return err
	}
	s.listener = listener
	return nil
}

// Address returns the bound address, including the selected port when port 0
// is used by tests.
func (s *Server) Address() string {
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return s.address
}

// Serve runs until ctx is cancelled or the server fails.
func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		return fmt.Errorf("LAN server is not listening")
	}
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.http.Serve(s.listener)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Close releases the listener.
func (s *Server) Close() error {
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func authenticate(manager *auth.TokenManager, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "missing or malformed Authorization header")
			return
		}
		claims, err := manager.ValidateAccessToken(parts[1])
		if err != nil || claims.UserID == "" || claims.DeviceID == "" || claims.SessionID == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or expired token")
			return
		}
		if claims.Scope != "" {
			expected := "/v1/lan/files/" + claims.FileID + "/content"
			if claims.Scope != "share_download" || claims.FileID == "" || r.URL.Path != expected || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
				writeError(w, http.StatusForbidden, "TOKEN_SCOPE_DENIED", "token is not valid for this operation")
				return
			}
		}
		ctx := context.WithValue(r.Context(), claimsContextKey{}, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func claimsFromContext(ctx context.Context) (*auth.TokenClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey{}).(*auth.TokenClaims)
	return claims, ok
}

func listFiles(store *storage.SQLiteObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		claims, _ := claimsFromContext(r.Context())
		files, err := store.GetStore().ListLANFiles(claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list files")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"files": files})
	}
}

func fileRoute(store *storage.SQLiteObjectStore, trashRetention time.Duration, coordinated bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1/lan/files/")
		if path == "" {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "file not found")
			return
		}
		if strings.HasSuffix(path, "/content") {
			id := strings.TrimSuffix(path, "/content")
			if id == "" || strings.Contains(id, "/") {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "file not found")
				return
			}
			downloadFile(store, id, w, r)
			return
		}
		if strings.Contains(path, "/") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "file not found")
			return
		}
		switch r.Method {
		case http.MethodPatch:
			renameFile(store, coordinated, path, w, r)
		case http.MethodDelete:
			trashFile(store, trashRetention, coordinated, path, w, r)
		default:
			w.Header().Set("Allow", "PATCH, DELETE")
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		}
	}
}

func downloadFile(store *storage.SQLiteObjectStore, id string, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
		return
	}

	claims, _ := claimsFromContext(r.Context())
	file, err := store.GetStore().GetLANFile(id, claims.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to load file")
		return
	}
	if file == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "file not found")
		return
	}
	hash, err := storage.ParseHash(file.ObjectID)
	if err != nil || store.Verify(r.Context(), hash) != nil {
		writeError(w, http.StatusConflict, "OBJECT_NOT_READY", "object failed integrity verification")
		return
	}
	info, err := store.GetObject(hash)
	if err != nil {
		writeError(w, http.StatusConflict, "OBJECT_NOT_READY", "object is unavailable")
		return
	}
	object, err := os.Open(info.Path)
	if err != nil {
		writeError(w, http.StatusConflict, "OBJECT_NOT_READY", "object is unavailable")
		return
	}
	defer object.Close()

	w.Header().Set("ETag", `"`+file.SHA256+`"`)
	w.Header().Set("X-Content-SHA256", file.SHA256)
	w.Header().Set("Content-Type", file.MIME)
	if disposition := mime.FormatMediaType("attachment", map[string]string{"filename": file.Name}); disposition != "" {
		w.Header().Set("Content-Disposition", disposition)
	}
	http.ServeContent(w, r, file.Name, file.CreatedAt, object)
}

func renameFile(store *storage.SQLiteObjectStore, coordinated bool, id string, w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request must contain only a valid name")
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "request must contain only a valid name")
		return
	}
	name := strings.TrimSpace(body.Name)
	if len(name) == 0 || len(name) > 255 {
		writeError(w, http.StatusBadRequest, "INVALID_NAME", "filename must contain 1 to 255 bytes")
		return
	}
	normalized, err := storagecatalog.NormalizeName(name)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_NAME", err.Error())
		return
	}
	claims, _ := claimsFromContext(r.Context())
	file, err := store.GetStore().RenameLANFileCoordinated(r.Context(), id, claims.UserID, name, normalized, coordinated)
	if writeLifecycleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, file)
}

func trashFile(store *storage.SQLiteObjectStore, retention time.Duration, coordinated bool, id string, w http.ResponseWriter, r *http.Request) {
	claims, _ := claimsFromContext(r.Context())
	file, err := store.GetStore().TrashLANFileCoordinated(r.Context(), id, claims.UserID, retention, coordinated)
	if writeLifecycleError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, file)
}

func listTrash(store *storage.SQLiteObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		claims, _ := claimsFromContext(r.Context())
		files, err := store.GetStore().ListLANTrash(claims.UserID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "INTERNAL", "failed to list trash")
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{"files": files})
	}
}

func trashRoute(store *storage.SQLiteObjectStore, coordinated bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/v1/lan/trash/")
		claims, _ := claimsFromContext(r.Context())
		if strings.HasSuffix(path, "/restore") {
			id := strings.TrimSuffix(path, "/restore")
			if r.Method != http.MethodPost || id == "" || strings.Contains(id, "/") {
				writeError(w, http.StatusNotFound, "NOT_FOUND", "trash record not found")
				return
			}
			file, err := store.GetStore().RestoreLANFileCoordinated(r.Context(), id, claims.UserID, coordinated)
			if writeLifecycleError(w, err) {
				return
			}
			writeJSON(w, http.StatusOK, file)
			return
		}
		if path == "" || strings.Contains(path, "/") {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "trash record not found")
			return
		}
		if r.Method != http.MethodDelete {
			w.Header().Set("Allow", http.MethodDelete)
			writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		var err error
		if coordinated {
			err = store.PurgeLANFileCoordinated(r.Context(), path, claims.UserID)
		} else {
			err = store.PurgeLANFile(r.Context(), path, claims.UserID)
		}
		if writeLifecycleError(w, err) {
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeLifecycleError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, storage.ErrLANFileNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "file not found")
	case errors.Is(err, storage.ErrLANNameConflict):
		writeError(w, http.StatusConflict, "NAME_CONFLICT", "an active file already uses this name")
	case errors.Is(err, storage.ErrLANInvalidState):
		writeError(w, http.StatusConflict, "INVALID_STATE", "file state does not allow this operation")
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL", "file operation failed")
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}
