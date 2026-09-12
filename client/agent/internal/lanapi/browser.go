package lanapi

import (
	"context"
	"net/http"
	"strings"

	"github.com/share-disk/share-disk/client/agent/internal/storage"
	"github.com/share-disk/share-disk/internal/auth"
)

// A browser-native attachment download streams directly from the Agent.
// The credential travels in a bounded POST body, never a URL or redirect.
func browserDownload(manager *auth.TokenManager, store *storage.SQLiteObjectStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodPost {
			writeError(w, 405, "METHOD_NOT_ALLOWED", "Use POST with a download ticket")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8192)
		if err := r.ParseForm(); err != nil {
			writeError(w, 400, "INVALID_REQUEST", "Invalid download request")
			return
		}
		// PostForm deliberately excludes query parameters.
		claims, err := manager.ValidateAccessToken(r.PostForm.Get("ticket"))
		if err != nil {
			writeError(w, 401, "UNAUTHORIZED", "Download ticket is invalid or expired")
			return
		}
		if (claims.Scope != "browser_download" && claims.Scope != "share_download") || claims.FileID == "" || strings.ContainsAny(claims.FileID, "/\\") {
			writeError(w, 403, "TOKEN_SCOPE_DENIED", "Ticket is not valid for browser download")
			return
		}
		if claims.Scope == "browser_download" && (claims.Origin == "" || claims.Origin != r.Header.Get("Origin")) {
			writeError(w, 403, "ORIGIN_DENIED", "Ticket belongs to another browser origin")
			return
		}
		request := r.Clone(context.WithValue(r.Context(), claimsContextKey{}, claims))
		request.Method = http.MethodGet
		downloadFile(store, claims.FileID, w, request)
	}
}
