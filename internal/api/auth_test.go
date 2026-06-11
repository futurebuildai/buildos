package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// fakeAuthService implements AuthServicer for handler tests. It records the
// arguments each method received (so tests can prove the handler forwards the
// decoded body verbatim) and returns the configured pair/err.
type fakeAuthService struct {
	pair service.TokenPair
	err  error

	gotToken, gotEmail, gotPassword, gotDisplayName string
	gotRefresh                                      string
	gotResetToken, gotNewPassword                   string
	logoutCalled                                    bool
}

func (f *fakeAuthService) ClaimFirstOwner(_ context.Context, cleartext, email, password, displayName string) (service.TokenPair, error) {
	f.gotToken, f.gotEmail, f.gotPassword, f.gotDisplayName = cleartext, email, password, displayName
	return f.pair, f.err
}

func (f *fakeAuthService) Login(_ context.Context, email, password string) (service.TokenPair, error) {
	f.gotEmail, f.gotPassword = email, password
	return f.pair, f.err
}

func (f *fakeAuthService) Refresh(_ context.Context, cleartext string) (service.TokenPair, error) {
	f.gotRefresh = cleartext
	return f.pair, f.err
}

func (f *fakeAuthService) Logout(_ context.Context, cleartext string) error {
	f.logoutCalled = true
	f.gotRefresh = cleartext
	return f.err
}

func (f *fakeAuthService) RequestPasswordReset(_ context.Context, email string) error {
	f.gotEmail = email
	return f.err
}

func (f *fakeAuthService) ResetPassword(_ context.Context, cleartext, newPassword string) error {
	f.gotResetToken, f.gotNewPassword = cleartext, newPassword
	return f.err
}

func sampleTokenPair() service.TokenPair {
	return service.TokenPair{
		AccessToken:  "access-xyz",
		ExpiresIn:    900,
		RefreshToken: "refresh-abc",
		User:         models.User{ID: uuid.New(), Email: "owner@acme.test", Role: "owner"},
	}
}

// jsonReq builds an unauthenticated POST request with a JSON body. The auth
// surface is pre-authentication, so no claims are installed.
func jsonReq(target, body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

func decodeTokenPair(t *testing.T, w *httptest.ResponseRecorder) tokenPairResponse {
	t.Helper()
	var env struct {
		Data tokenPairResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode token pair: %v (body=%s)", err, w.Body.String())
	}
	return env.Data
}

func decodeErrCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error: %v (body=%s)", err, w.Body.String())
	}
	if env.Error == nil {
		t.Fatalf("expected an error object, got body=%s", w.Body.String())
	}
	return env.Error.Code
}

// ---------- POST /auth/claim ----------

func TestClaimFirstOwner_OK(t *testing.T) {
	svc := &fakeAuthService{pair: sampleTokenPair()}
	h := NewAuthHandler(svc)
	r := jsonReq("/api/v1/auth/claim",
		`{"token":"boot-tok","email":"owner@acme.test","password":"pw12345678","display_name":"Owner"}`)
	w := httptest.NewRecorder()
	h.ClaimFirstOwner(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotToken != "boot-tok" || svc.gotEmail != "owner@acme.test" ||
		svc.gotPassword != "pw12345678" || svc.gotDisplayName != "Owner" {
		t.Errorf("service got token=%q email=%q password=%q display=%q",
			svc.gotToken, svc.gotEmail, svc.gotPassword, svc.gotDisplayName)
	}
	got := decodeTokenPair(t, w)
	if got.AccessToken != "access-xyz" || got.TokenType != "Bearer" ||
		got.ExpiresIn != 900 || got.RefreshToken != "refresh-abc" ||
		got.User.Email != "owner@acme.test" {
		t.Errorf("response = %+v", got)
	}
}

func TestClaimFirstOwner_BadJSON(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{})
	w := httptest.NewRecorder()
	h.ClaimFirstOwner(w, jsonReq("/api/v1/auth/claim", "{not json"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

func TestClaimFirstOwner_FirstOwnerExists(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{err: service.ErrFirstOwnerExists})
	w := httptest.NewRecorder()
	h.ClaimFirstOwner(w, jsonReq("/api/v1/auth/claim", `{"email":"a@b.c","password":"x"}`))
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d, want 409", w.Code)
	}
	if code := decodeErrCode(t, w); code != "FIRST_OWNER_EXISTS" {
		t.Errorf("code=%q, want FIRST_OWNER_EXISTS", code)
	}
}

func TestClaimFirstOwner_InvalidBootstrapToken(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{err: service.ErrInvalidBootstrapToken})
	w := httptest.NewRecorder()
	h.ClaimFirstOwner(w, jsonReq("/api/v1/auth/claim", `{"token":"bad"}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
	if code := decodeErrCode(t, w); code != "INVALID_BOOTSTRAP_TOKEN" {
		t.Errorf("code=%q, want INVALID_BOOTSTRAP_TOKEN", code)
	}
}

// ---------- POST /auth/login ----------

func TestLogin_OK(t *testing.T) {
	svc := &fakeAuthService{pair: sampleTokenPair()}
	h := NewAuthHandler(svc)
	w := httptest.NewRecorder()
	h.Login(w, jsonReq("/api/v1/auth/login", `{"email":"owner@acme.test","password":"pw12345678"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotEmail != "owner@acme.test" || svc.gotPassword != "pw12345678" {
		t.Errorf("service got email=%q password=%q", svc.gotEmail, svc.gotPassword)
	}
	if got := decodeTokenPair(t, w); got.AccessToken != "access-xyz" {
		t.Errorf("response = %+v", got)
	}
}

func TestLogin_BadJSON(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{})
	w := httptest.NewRecorder()
	h.Login(w, jsonReq("/api/v1/auth/login", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{err: service.ErrInvalidCredentials})
	w := httptest.NewRecorder()
	h.Login(w, jsonReq("/api/v1/auth/login", `{"email":"a@b.c","password":"nope"}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
	if code := decodeErrCode(t, w); code != "INVALID_CREDENTIALS" {
		t.Errorf("code=%q, want INVALID_CREDENTIALS", code)
	}
}

// ---------- POST /auth/refresh ----------

func TestRefresh_OK(t *testing.T) {
	svc := &fakeAuthService{pair: sampleTokenPair()}
	h := NewAuthHandler(svc)
	w := httptest.NewRecorder()
	h.Refresh(w, jsonReq("/api/v1/auth/refresh", `{"refresh_token":"refresh-abc"}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotRefresh != "refresh-abc" {
		t.Errorf("service got refresh=%q", svc.gotRefresh)
	}
}

func TestRefresh_BadJSON(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{})
	w := httptest.NewRecorder()
	h.Refresh(w, jsonReq("/api/v1/auth/refresh", "}{"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestRefresh_InvalidRefreshToken(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{err: service.ErrInvalidRefreshToken})
	w := httptest.NewRecorder()
	h.Refresh(w, jsonReq("/api/v1/auth/refresh", `{"refresh_token":"gone"}`))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
	if code := decodeErrCode(t, w); code != "INVALID_REFRESH_TOKEN" {
		t.Errorf("code=%q, want INVALID_REFRESH_TOKEN", code)
	}
}

// ---------- POST /auth/logout ----------

func TestLogout_OK(t *testing.T) {
	svc := &fakeAuthService{}
	h := NewAuthHandler(svc)
	w := httptest.NewRecorder()
	h.Logout(w, jsonReq("/api/v1/auth/logout", `{"refresh_token":"refresh-abc"}`))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("204 body should be empty, got %q", w.Body.String())
	}
	if !svc.logoutCalled || svc.gotRefresh != "refresh-abc" {
		t.Errorf("logout called=%v refresh=%q", svc.logoutCalled, svc.gotRefresh)
	}
}

func TestLogout_BadJSON(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{})
	w := httptest.NewRecorder()
	h.Logout(w, jsonReq("/api/v1/auth/logout", "nope"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestLogout_ServiceErrorMapsTo500(t *testing.T) {
	// A non-sentinel error from Logout must not leak internals — default 500.
	h := NewAuthHandler(&fakeAuthService{err: errors.New("revoke store down")})
	w := httptest.NewRecorder()
	h.Logout(w, jsonReq("/api/v1/auth/logout", `{"refresh_token":"x"}`))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
	if code := decodeErrCode(t, w); code != "INTERNAL_ERROR" {
		t.Errorf("code=%q, want INTERNAL_ERROR", code)
	}
}

// ---------- POST /auth/password-reset/request ----------

func TestRequestPasswordReset_OK(t *testing.T) {
	svc := &fakeAuthService{}
	h := NewAuthHandler(svc)
	w := httptest.NewRecorder()
	h.RequestPasswordReset(w, jsonReq("/api/v1/auth/password-reset/request", `{"email":"owner@acme.test"}`))

	if w.Code != http.StatusAccepted {
		t.Fatalf("status=%d, want 202", w.Code)
	}
	if svc.gotEmail != "owner@acme.test" {
		t.Errorf("service got email=%q", svc.gotEmail)
	}
}

func TestRequestPasswordReset_BadJSON(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{})
	w := httptest.NewRecorder()
	h.RequestPasswordReset(w, jsonReq("/api/v1/auth/password-reset/request", "{"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

// A non-sentinel error from RequestPasswordReset must not leak internals — it
// maps through writeAuthError's default branch to 500. (The handler still never
// reveals whether the email matched a user; this leg is the infra-failure path,
// distinct from the always-202 no-enumeration success path.)
func TestRequestPasswordReset_ServiceErrorMapsTo500(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{err: errors.New("mailer transport down")})
	w := httptest.NewRecorder()
	h.RequestPasswordReset(w, jsonReq("/api/v1/auth/password-reset/request", `{"email":"owner@acme.test"}`))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
	if code := decodeErrCode(t, w); code != "INTERNAL_ERROR" {
		t.Errorf("code=%q, want INTERNAL_ERROR", code)
	}
}

// ---------- POST /auth/password-reset/confirm ----------

func TestResetPassword_OK(t *testing.T) {
	svc := &fakeAuthService{}
	h := NewAuthHandler(svc)
	w := httptest.NewRecorder()
	h.ResetPassword(w, jsonReq("/api/v1/auth/password-reset/confirm",
		`{"token":"reset-tok","password":"newpw12345"}`))

	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", w.Code)
	}
	if svc.gotResetToken != "reset-tok" || svc.gotNewPassword != "newpw12345" {
		t.Errorf("service got token=%q password=%q", svc.gotResetToken, svc.gotNewPassword)
	}
}

func TestResetPassword_BadJSON(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{})
	w := httptest.NewRecorder()
	h.ResetPassword(w, jsonReq("/api/v1/auth/password-reset/confirm", ""))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestResetPassword_InvalidResetToken(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{err: service.ErrInvalidResetToken})
	w := httptest.NewRecorder()
	h.ResetPassword(w, jsonReq("/api/v1/auth/password-reset/confirm", `{"token":"bad","password":"x"}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "INVALID_RESET_TOKEN" {
		t.Errorf("code=%q, want INVALID_RESET_TOKEN", code)
	}
}

// ---------- writeAuthError mapping ----------

func TestWriteAuthError_Mapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"invalid credentials", service.ErrInvalidCredentials, http.StatusUnauthorized, "INVALID_CREDENTIALS"},
		{"invalid refresh", service.ErrInvalidRefreshToken, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN"},
		{"invalid reset", service.ErrInvalidResetToken, http.StatusBadRequest, "INVALID_RESET_TOKEN"},
		{"first owner exists", service.ErrFirstOwnerExists, http.StatusConflict, "FIRST_OWNER_EXISTS"},
		{"invalid bootstrap", service.ErrInvalidBootstrapToken, http.StatusUnauthorized, "INVALID_BOOTSTRAP_TOKEN"},
		{"validation wrapped", fmt.Errorf("%w: bad email", service.ErrInvalidInput), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"default no leak", errors.New("secret internal detail"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/x", nil)
			writeAuthError(w, r, tt.err)
			if w.Code != tt.wantStatus {
				t.Fatalf("status=%d, want %d", w.Code, tt.wantStatus)
			}
			if code := decodeErrCode(t, w); code != tt.wantCode {
				t.Errorf("code=%q, want %q", code, tt.wantCode)
			}
			// The default branch must never echo the raw internal error.
			if tt.wantCode == "INTERNAL_ERROR" && strings.Contains(w.Body.String(), "secret internal detail") {
				t.Errorf("internal error leaked into body: %s", w.Body.String())
			}
		})
	}
}

// ---------- HttpOnly refresh-cookie transport ----------

// findRefreshCookie pulls the buildos_refresh Set-Cookie off the recorder.
func findRefreshCookie(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == refreshCookieName {
			return c
		}
	}
	t.Fatalf("no %q Set-Cookie found; headers=%v", refreshCookieName, w.Header().Values("Set-Cookie"))
	return nil
}

// assertSecureRefreshCookie verifies the security-critical attributes of a
// freshly-set refresh cookie: the secret value, HttpOnly, Secure,
// SameSite=Strict, the /api/v1/auth Path scope, a positive Max-Age, and the
// absence of an explicit Domain (host-only — required for the Cloudflare-Worker
// → Railway origin to bind the cookie to the host the browser sees).
func assertSecureRefreshCookie(t *testing.T, c *http.Cookie, wantValue string) {
	t.Helper()
	if c.Value != wantValue {
		t.Errorf("cookie value=%q, want %q", c.Value, wantValue)
	}
	if !c.HttpOnly {
		t.Error("cookie missing HttpOnly")
	}
	if !c.Secure {
		t.Error("cookie missing Secure")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("cookie SameSite=%v, want Strict", c.SameSite)
	}
	if c.Path != "/api/v1/auth" {
		t.Errorf("cookie Path=%q, want /api/v1/auth", c.Path)
	}
	if c.MaxAge <= 0 {
		t.Errorf("cookie Max-Age=%d, want positive", c.MaxAge)
	}
	if c.Domain != "" {
		t.Errorf("cookie Domain=%q, want host-only (empty)", c.Domain)
	}
}

func TestClaim_SetsSecureRefreshCookie(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{pair: sampleTokenPair()})
	w := httptest.NewRecorder()
	h.ClaimFirstOwner(w, jsonReq("/api/v1/auth/claim",
		`{"token":"boot","email":"a@b.c","password":"pw12345678","display_name":"O"}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d", w.Code)
	}
	assertSecureRefreshCookie(t, findRefreshCookie(t, w), "refresh-abc")
}

func TestLogin_SetsSecureRefreshCookie(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{pair: sampleTokenPair()})
	w := httptest.NewRecorder()
	h.Login(w, jsonReq("/api/v1/auth/login", `{"email":"a@b.c","password":"pw12345678"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	assertSecureRefreshCookie(t, findRefreshCookie(t, w), "refresh-abc")
	// Body fields stay UNCHANGED (additive transport — API clients/tests unbroken).
	if got := decodeTokenPair(t, w); got.RefreshToken != "refresh-abc" || got.AccessToken != "access-xyz" {
		t.Errorf("body token pair = %+v", got)
	}
}

func TestRefresh_SetsRotatedRefreshCookie(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{pair: sampleTokenPair()})
	w := httptest.NewRecorder()
	h.Refresh(w, jsonReq("/api/v1/auth/refresh", `{"refresh_token":"refresh-abc"}`))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	assertSecureRefreshCookie(t, findRefreshCookie(t, w), "refresh-abc")
}

// The reload-survival path: the SPA POSTs /refresh with NO body and the browser
// replays the HttpOnly cookie. The handler must read the token from the cookie.
func TestRefresh_ReadsTokenFromCookieWhenBodyEmpty(t *testing.T) {
	svc := &fakeAuthService{pair: sampleTokenPair()}
	h := NewAuthHandler(svc)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil) // no body
	r.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "cookie-refresh-token"})
	w := httptest.NewRecorder()
	h.Refresh(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotRefresh != "cookie-refresh-token" {
		t.Errorf("service got refresh=%q, want the cookie value", svc.gotRefresh)
	}
}

// An explicit body token must win over the cookie (backward-compat).
func TestRefresh_BodyTokenWinsOverCookie(t *testing.T) {
	svc := &fakeAuthService{pair: sampleTokenPair()}
	h := NewAuthHandler(svc)
	r := jsonReq("/api/v1/auth/refresh", `{"refresh_token":"body-token"}`)
	r.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "cookie-token"})
	w := httptest.NewRecorder()
	h.Refresh(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d", w.Code)
	}
	if svc.gotRefresh != "body-token" {
		t.Errorf("service got refresh=%q, want body-token", svc.gotRefresh)
	}
}

// A genuinely malformed (non-empty, non-JSON) body is still a 400.
func TestRefresh_MalformedBodyStill400(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{pair: sampleTokenPair()})
	w := httptest.NewRecorder()
	h.Refresh(w, jsonReq("/api/v1/auth/refresh", "}{not json"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

// On a dead refresh token the handler clears the cookie so the browser stops
// replaying it, and returns 401.
func TestRefresh_DeadTokenClearsCookie(t *testing.T) {
	h := NewAuthHandler(&fakeAuthService{err: service.ErrInvalidRefreshToken})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	r.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "stale"})
	w := httptest.NewRecorder()
	h.Refresh(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", w.Code)
	}
	c := findRefreshCookie(t, w)
	if c.MaxAge >= 0 {
		t.Errorf("expected cleared cookie (Max-Age<0), got MaxAge=%d value=%q", c.MaxAge, c.Value)
	}
}

func TestLogout_ClearsRefreshCookie(t *testing.T) {
	svc := &fakeAuthService{}
	h := NewAuthHandler(svc)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "to-revoke"})
	w := httptest.NewRecorder()
	h.Logout(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", w.Code)
	}
	// Revokes the cookie's token and expires the cookie.
	if svc.gotRefresh != "to-revoke" {
		t.Errorf("service got refresh=%q, want to-revoke (from cookie)", svc.gotRefresh)
	}
	c := findRefreshCookie(t, w)
	if c.MaxAge >= 0 {
		t.Errorf("expected cleared cookie (Max-Age<0), got MaxAge=%d", c.MaxAge)
	}
	if c.Path != "/api/v1/auth" {
		t.Errorf("clear cookie Path=%q, want /api/v1/auth (must match set Path)", c.Path)
	}
}

// WithRefreshCookie(false, ...) drops Secure for local http rigs; the default
// (no option) keeps it on, and a custom TTL is honored as the Max-Age.
func TestRefreshCookie_SecureGateAndTTL(t *testing.T) {
	// Insecure rig: Secure off, TTL honored.
	h := NewAuthHandler(&fakeAuthService{pair: sampleTokenPair()}, WithRefreshCookie(false, 2*time.Hour))
	w := httptest.NewRecorder()
	h.Login(w, jsonReq("/api/v1/auth/login", `{"email":"a@b.c","password":"pw12345678"}`))
	c := findRefreshCookie(t, w)
	if c.Secure {
		t.Error("expected Secure off for insecure rig")
	}
	if c.MaxAge != int((2 * time.Hour).Seconds()) {
		t.Errorf("Max-Age=%d, want %d", c.MaxAge, int((2 * time.Hour).Seconds()))
	}
	// Default handler: Secure on, default refresh TTL.
	hd := NewAuthHandler(&fakeAuthService{pair: sampleTokenPair()})
	wd := httptest.NewRecorder()
	hd.Login(wd, jsonReq("/api/v1/auth/login", `{"email":"a@b.c","password":"pw12345678"}`))
	cd := findRefreshCookie(t, wd)
	if !cd.Secure {
		t.Error("expected Secure on by default")
	}
	if cd.MaxAge != int(service.DefaultRefreshTTL.Seconds()) {
		t.Errorf("default Max-Age=%d, want %d", cd.MaxAge, int(service.DefaultRefreshTTL.Seconds()))
	}
}
