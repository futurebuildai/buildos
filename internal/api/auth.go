package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// refreshCookieName is the HttpOnly cookie that carries the opaque refresh
// token across reloads / deep-links. It is Path-scoped to the auth surface so
// it rides ONLY the /api/v1/auth calls — every other API request authenticates
// with the Bearer access-token header and never sees this cookie.
const refreshCookieName = "buildos_refresh"

// refreshCookiePath scopes the cookie to the auth surface. /api/v1/auth/refresh
// is the only cookie-authed route; that scoping + SameSite=Strict is the CSRF
// mitigation (see docs/security-posture.md).
const refreshCookiePath = "/api/v1/auth"

// AuthServicer is the subset of *service.AuthService consumed by AuthHandler.
// The handler depends on the interface so unit tests can substitute a fake
// (matches the SetupServicer / PipelineServicer pattern in this package).
type AuthServicer interface {
	ClaimFirstOwner(ctx context.Context, cleartext, email, password, displayName string) (service.TokenPair, error)
	Login(ctx context.Context, email, password string) (service.TokenPair, error)
	Refresh(ctx context.Context, cleartext string) (service.TokenPair, error)
	Logout(ctx context.Context, cleartext string) error
	RequestPasswordReset(ctx context.Context, email string) error
	ResetPassword(ctx context.Context, cleartext, newPassword string) error
}

// AuthHandler exposes the unauthenticated /api/v1/auth/* surface — native
// email/password authentication. BuildOS owns identity now (no external OIDC
// provider): it mints its own RS256 access tokens and server-revocable opaque
// refresh tokens.
//
// The refresh token rides BOTH the JSON body (backward-compat for API clients
// and existing tests) AND an HttpOnly cookie. The cookie is what survives a
// browser reload / deep-link: the SPA holds the access token in memory only, so
// on boot it POSTs /refresh with no body and the browser replays the cookie.
type AuthHandler struct {
	svc AuthServicer
	// cookieSecure stamps the Secure attribute on the refresh cookie. Defaults
	// true (staging/prod are HTTPS via Cloudflare); gated off only for local
	// non-prod http rigs so the browser will actually store/send the cookie.
	cookieSecure bool
	// refreshTTL is the cookie Max-Age. Mirrors the AuthService refresh-token
	// lifetime so the cookie and the server-side token expire together.
	refreshTTL time.Duration
}

// AuthHandlerOption configures an AuthHandler.
type AuthHandlerOption func(*AuthHandler)

// WithRefreshCookie configures the HttpOnly refresh-cookie transport: whether
// the Secure attribute is set, and the cookie Max-Age (the refresh-token TTL).
// A non-positive ttl falls back to service.DefaultRefreshTTL so the cookie
// never outlives — or under-lives — the server-side token.
func WithRefreshCookie(secure bool, ttl time.Duration) AuthHandlerOption {
	return func(h *AuthHandler) {
		h.cookieSecure = secure
		if ttl > 0 {
			h.refreshTTL = ttl
		}
	}
}

// NewAuthHandler binds the handler to an AuthServicer. Without options the
// refresh cookie is Secure (production default) with the default refresh TTL.
func NewAuthHandler(svc AuthServicer, opts ...AuthHandlerOption) *AuthHandler {
	h := &AuthHandler{svc: svc, cookieSecure: true, refreshTTL: service.DefaultRefreshTTL}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// setRefreshCookie writes the refresh token as an HttpOnly; Secure;
// SameSite=Strict cookie scoped to /api/v1/auth. No Domain attribute is set:
// the cookie is host-only, binding to whatever host serves it (so it works
// unchanged through the Cloudflare Worker → Railway origin, scoped to the host
// the browser sees, e.g. staging.futurebuild.ai).
func (h *AuthHandler) setRefreshCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    token,
		Path:     refreshCookiePath,
		MaxAge:   int(h.refreshTTL.Seconds()),
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearRefreshCookie expires the refresh cookie (Max-Age=0) on the same Path
// so the browser drops it. The attributes must match the set call for the
// browser to overwrite the right cookie.
func (h *AuthHandler) clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     refreshCookiePath,
		MaxAge:   -1, // negative MaxAge => "Max-Age=0", delete now
		HttpOnly: true,
		Secure:   h.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
}

// refreshTokenFromRequest resolves the refresh token: the JSON body wins (kept
// for API clients / existing tests), falling back to the HttpOnly cookie that
// the browser replays on a reload/deep-link silent refresh.
func refreshTokenFromRequest(r *http.Request, bodyToken string) string {
	if bodyToken != "" {
		return bodyToken
	}
	if c, err := r.Cookie(refreshCookieName); err == nil {
		return c.Value
	}
	return ""
}

// ---------- wire DTOs ----------

type claimRequest struct {
	Token       string `json:"token"`
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type resetRequestBody struct {
	Email string `json:"email"`
}

type resetConfirmBody struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// tokenPairResponse is the success body for claim / login / refresh.
type tokenPairResponse struct {
	AccessToken  string      `json:"access_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    int         `json:"expires_in"`
	RefreshToken string      `json:"refresh_token"`
	User         models.User `json:"user"`
}

func newTokenPairResponse(p service.TokenPair) tokenPairResponse {
	return tokenPairResponse{
		AccessToken:  p.AccessToken,
		TokenType:    "Bearer",
		ExpiresIn:    p.ExpiresIn,
		RefreshToken: p.RefreshToken,
		User:         p.User,
	}
}

// ---------- POST /auth/claim ----------

// ClaimFirstOwner redeems a bootstrap token to create the fork's first owner
// with a native credential. Unauthenticated — the bootstrap token IS the
// privilege grant. 201 on success.
// POST /api/v1/auth/claim
func (h *AuthHandler) ClaimFirstOwner(w http.ResponseWriter, r *http.Request) {
	var body claimRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	pair, err := h.svc.ClaimFirstOwner(r.Context(), body.Token, body.Email, body.Password, body.DisplayName)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}
	h.setRefreshCookie(w, pair.RefreshToken)
	writeJSON(w, r, http.StatusCreated, newTokenPairResponse(pair))
}

// ---------- POST /auth/login ----------

// Login verifies an email/password credential and returns a token pair.
// POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var body loginRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	pair, err := h.svc.Login(r.Context(), body.Email, body.Password)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}
	h.setRefreshCookie(w, pair.RefreshToken)
	writeJSON(w, r, http.StatusOK, newTokenPairResponse(pair))
}

// ---------- POST /auth/refresh ----------

// Refresh rotates a refresh token and mints a fresh access token. The token is
// read from the JSON body when present, else from the HttpOnly buildos_refresh
// cookie — which is how the SPA silently rehydrates a session on a hard reload
// (it holds the access token in memory only, so it POSTs here with no body and
// the browser replays the cookie). An empty body is therefore valid; only a
// genuinely malformed (non-empty, non-JSON) body is a 400. The rotated token is
// written back as a fresh cookie.
// POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var body refreshRequest
	// Decode leniently: an empty body (cookie-only silent refresh) is valid.
	// io.EOF means no body was sent; anything else non-nil is malformed JSON.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	token := refreshTokenFromRequest(r, body.RefreshToken)
	pair, err := h.svc.Refresh(r.Context(), token)
	if err != nil {
		// A dead/missing token clears any stale cookie so the browser stops
		// replaying it on every boot.
		h.clearRefreshCookie(w)
		writeAuthError(w, r, err)
		return
	}
	h.setRefreshCookie(w, pair.RefreshToken)
	writeJSON(w, r, http.StatusOK, newTokenPairResponse(pair))
}

// ---------- POST /auth/logout ----------

// Logout revokes the presented refresh token. Idempotent — 204 even if the
// token was already gone. The token is read from the body or the cookie; the
// cookie is always cleared (Max-Age=0) so the browser stops replaying it.
// An empty body is valid (cookie-only logout).
// POST /api/v1/auth/logout
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var body logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	token := refreshTokenFromRequest(r, body.RefreshToken)
	if err := h.svc.Logout(r.Context(), token); err != nil {
		writeAuthError(w, r, err)
		return
	}
	// Always clear the cookie, even on the idempotent already-gone path.
	h.clearRefreshCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// ---------- POST /auth/password-reset/request ----------

// RequestPasswordReset issues a single-use reset token and emails it. Always
// 202 (never reveals whether the email matched a user — no enumeration).
// POST /api/v1/auth/password-reset/request
func (h *AuthHandler) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var body resetRequestBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	if err := h.svc.RequestPasswordReset(r.Context(), body.Email); err != nil {
		writeAuthError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// ---------- POST /auth/password-reset/confirm ----------

// ResetPassword consumes a reset token and sets a new password, revoking all
// of the user's active sessions. 204 on success.
// POST /api/v1/auth/password-reset/confirm
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var body resetConfirmBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON body")
		return
	}
	if err := h.svc.ResetPassword(r.Context(), body.Token, body.Password); err != nil {
		writeAuthError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---------- error mapping ----------

// writeAuthError maps AuthService sentinels to HTTP responses.
//
//	ErrInvalidCredentials    → 401 INVALID_CREDENTIALS
//	ErrInvalidRefreshToken   → 401 INVALID_REFRESH_TOKEN
//	ErrInvalidResetToken     → 400 INVALID_RESET_TOKEN
//	ErrFirstOwnerExists      → 409 FIRST_OWNER_EXISTS
//	ErrInvalidBootstrapToken → 401 INVALID_BOOTSTRAP_TOKEN (uniform on any
//	                            bootstrap-token failure — see service/setup.go)
//	ErrInvalidInput          → 400 VALIDATION_ERROR
//	default                  → 500 INTERNAL_ERROR (never leak internals)
func writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		writeErrorResponse(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email or password")
	case errors.Is(err, service.ErrInvalidRefreshToken):
		writeErrorResponse(w, r, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "refresh token invalid or expired")
	case errors.Is(err, service.ErrInvalidResetToken):
		writeErrorResponse(w, r, http.StatusBadRequest, "INVALID_RESET_TOKEN", "reset token invalid or expired")
	case errors.Is(err, service.ErrFirstOwnerExists):
		writeErrorResponse(w, r, http.StatusConflict, "FIRST_OWNER_EXISTS", "an owner already exists for this deployment")
	case errors.Is(err, service.ErrInvalidBootstrapToken):
		writeErrorResponse(w, r, http.StatusUnauthorized, "INVALID_BOOTSTRAP_TOKEN", "bootstrap token invalid or expired")
	case errors.Is(err, service.ErrInvalidInput):
		writeErrorResponse(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		writeErrorResponse(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "internal error")
	}
}

// MountAuthRoutes wires the native-auth surface under /api/v1/auth. These
// routes are UNAUTHENTICATED (they mint the credentials the rest of the API
// requires) and must be mounted OUTSIDE the Auth middleware group. They are
// also exempt from the SetupGate (the first-owner claim must work on a fresh
// fork before onboarding completes).
func MountAuthRoutes(r chi.Router, h *AuthHandler) {
	r.Route("/api/v1/auth", func(r chi.Router) {
		r.Post("/claim", h.ClaimFirstOwner)
		r.Post("/login", h.Login)
		r.Post("/refresh", h.Refresh)
		r.Post("/logout", h.Logout)
		r.Post("/password-reset/request", h.RequestPasswordReset)
		r.Post("/password-reset/confirm", h.ResetPassword)
	})
}
