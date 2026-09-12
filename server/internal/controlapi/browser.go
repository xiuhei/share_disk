package controlapi

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/share-disk/share-disk/server/internal/identity"
)

const browserCookie = "share_disk_session"
const browserContextKey ctxKey = 1

func (h *Handler) BrowserDownloadTicket(w http.ResponseWriter, r *http.Request) {
	if h.catalogRepo == nil || h.tokenManager == nil {
		writeError(w, r, 503, "UNAVAILABLE", "Download is not configured")
		return
	}
	if !sameBrowserOrigin(r) {
		writeError(w, r, 403, "FORBIDDEN", "Invalid download origin")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	file, err := h.catalogRepo.GetUnifiedFile(r.Context(), claims.UserID, chi.URLParam(r, "fileID"))
	if err != nil {
		writeCatalogError(w, r, err)
		return
	}
	if file.Status != "active" || !file.Available {
		writeError(w, r, 409, "NO_SOURCE", "文件没有在线副本")
		return
	}
	token, err := h.tokenManager.GenerateBrowserDownloadToken(claims.UserID, file.ContentLocalFileID, claims.SessionID, r.Header.Get("Origin"))
	if err != nil {
		writeError(w, r, 500, "INTERNAL_ERROR", "Failed to issue download ticket")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]interface{}{"url": strings.TrimRight(file.ReplicaEndpoint, "/") + "/v1/lan/browser-download", "ticket": token, "expires_in": 60})
}

func (h *Handler) RenameDevice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if decodeJSON(w, r, &req) != nil {
		writeError(w, r, 400, "INVALID_REQUEST", "Invalid device name")
		return
	}
	claims, _ := ClaimsFromContext(r.Context())
	if err := h.identityService.RenameDevice(r.Context(), claims.UserID, chi.URLParam(r, "deviceID"), req.Name); err != nil {
		if errors.Is(err, identity.ErrDeviceNotFound) {
			writeError(w, r, 404, "NOT_FOUND", "Device not found")
		} else if isValidationError(err) {
			writeError(w, r, 400, "INVALID_REQUEST", err.Error())
		} else {
			writeError(w, r, 500, "INTERNAL_ERROR", "Failed to rename device")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Exact Origin validation deliberately does not trust forwarded host headers.
// TLS-terminating proxies must preserve Host. HTTP is supported for isolated LAN development.
func sameBrowserOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	return err == nil && (origin.Scheme == "http" || origin.Scheme == "https") &&
		origin.Host == r.Host && origin.User == nil && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == "" &&
		(r.TLS == nil || origin.Scheme == "https") && r.Header.Get("Sec-Fetch-Site") != "cross-site"
}

func (h *Handler) BrowserLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !sameBrowserOrigin(r) {
		writeError(w, r, 403, "FORBIDDEN", "Cross-origin login rejected")
		return
	}
	var req struct {
		Account  string `json:"account"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if decodeJSON(w, r, &req) != nil {
		writeError(w, r, 400, "INVALID_REQUEST", "Invalid login request")
		return
	}
	session, token, err := h.identityService.LoginBrowser(r.Context(), strings.TrimSpace(req.Account), req.Password, req.Remember)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			writeError(w, r, 401, "INVALID_CREDENTIALS", "账号或密码错误")
		} else if isValidationError(err) {
			writeError(w, r, 400, "INVALID_REQUEST", err.Error())
		} else {
			writeError(w, r, 500, "INTERNAL_ERROR", "登录失败")
		}
		return
	}
	cookie := &http.Cookie{Name: browserCookie, Value: token, Path: "/v1", HttpOnly: true, Secure: r.TLS != nil || strings.HasPrefix(r.Header.Get("Origin"), "https://"), SameSite: http.SameSiteStrictMode}
	if req.Remember {
		cookie.Expires = session.ExpiresAt
		cookie.MaxAge = int(time.Until(session.ExpiresAt).Seconds())
	}
	http.SetCookie(w, cookie)
	h.writeBrowserSession(w, r, session)
}

func (h *Handler) writeBrowserSession(w http.ResponseWriter, r *http.Request, session *identity.BrowserSession) {
	account, err := h.identityService.GetAccount(r.Context(), session.UserID)
	if err != nil {
		writeError(w, r, 500, "INTERNAL_ERROR", "Failed to read account")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]interface{}{"account": account, "csrf_token": session.CSRFToken, "expires_at": session.ExpiresAt})
}

func (h *Handler) BrowserSession(w http.ResponseWriter, r *http.Request) {
	session, ok := r.Context().Value(browserContextKey).(*identity.BrowserSession)
	if !ok {
		writeError(w, r, 401, "UNAUTHORIZED", "Browser session required")
		return
	}
	h.writeBrowserSession(w, r, session)
}

func (h *Handler) BrowserLogout(w http.ResponseWriter, r *http.Request) {
	session, ok := r.Context().Value(browserContextKey).(*identity.BrowserSession)
	if !ok {
		writeError(w, r, 401, "UNAUTHORIZED", "Browser session required")
		return
	}
	if err := h.identityService.LogoutBrowser(r.Context(), session.UserID, session.ID); err != nil {
		writeError(w, r, 500, "INTERNAL_ERROR", "Logout failed")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: browserCookie, Value: "", Path: "/v1", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) authenticateBrowserRequest(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	cookie, err := r.Cookie(browserCookie)
	if err != nil {
		writeError(w, r, 401, "UNAUTHORIZED", "请先登录")
		return r, false
	}
	session, err := h.identityService.AuthenticateBrowser(r.Context(), cookie.Value)
	if err != nil {
		if errors.Is(err, identity.ErrInvalidCredentials) {
			writeError(w, r, 401, "UNAUTHORIZED", "登录已过期，请重新登录")
		} else {
			writeError(w, r, 503, "UNAVAILABLE", "暂时无法验证会话，请稍后重试")
		}
		return r, false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if !sameBrowserOrigin(r) || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRFToken)) != 1 {
			writeError(w, r, 403, "CSRF_REJECTED", "Request origin or CSRF token is invalid")
			return r, false
		}
	}
	// Browser sessions may operate account resources but cannot impersonate storage Agents.
	if strings.HasPrefix(r.URL.Path, "/v1/device-commands") ||
		r.URL.Path == "/v1/devices/register" || r.URL.Path == "/v1/devices/heartbeat" ||
		r.URL.Path == "/v1/catalog/files/register" || r.URL.Path == "/v1/catalog/lifecycle" ||
		(strings.HasPrefix(r.URL.Path, "/v1/transfers/") && r.Method == http.MethodPost &&
			r.URL.Path != "/v1/transfers/" && !strings.HasSuffix(r.URL.Path, "/cancel")) {
		writeError(w, r, 403, "DEVICE_REQUIRED", "A registered storage device is required")
		return r, false
	}
	ctx := context.WithValue(r.Context(), browserContextKey, session)
	ctx = context.WithValue(ctx, ctxKeyClaims, &identity.TokenClaims{UserID: session.UserID, SessionID: session.ID})
	w.Header().Set("Cache-Control", "no-store")
	return r.WithContext(ctx), true
}
