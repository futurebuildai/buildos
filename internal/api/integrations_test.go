package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// mockIntegrationsService implements IntegrationsServicer for handler tests.
// Only the methods exercised by the test under construction need non-zero
// returns; the rest satisfy the interface. Matches the mockSetupService pattern.
type mockIntegrationsService struct {
	setResult  models.IntegrationCredential
	setErr     error
	deleteErr  error
	listResult []models.IntegrationCredential
	listErr    error
	capsResult service.CapabilitiesResult
	capsErr    error

	lastCapsOrgID  uuid.UUID
	lastSetInput   service.SetCredentialInput
	lastListOrgID  uuid.UUID
	lastDelOrgID   uuid.UUID
	lastDelProv    string
	lastDelUserSub string
}

func (m *mockIntegrationsService) SetCredential(_ context.Context, in service.SetCredentialInput) (models.IntegrationCredential, error) {
	m.lastSetInput = in
	return m.setResult, m.setErr
}
func (m *mockIntegrationsService) DeleteCredential(_ context.Context, orgID uuid.UUID, provider, userSub string) error {
	m.lastDelOrgID, m.lastDelProv, m.lastDelUserSub = orgID, provider, userSub
	return m.deleteErr
}
func (m *mockIntegrationsService) ListCredentials(_ context.Context, orgID uuid.UUID) ([]models.IntegrationCredential, error) {
	m.lastListOrgID = orgID
	return m.listResult, m.listErr
}
func (m *mockIntegrationsService) Capabilities(_ context.Context, orgID uuid.UUID) (service.CapabilitiesResult, error) {
	m.lastCapsOrgID = orgID
	return m.capsResult, m.capsErr
}

// decodeCapabilities unwraps the {data: ...} envelope into the capabilities DTO.
func decodeCapabilities(t *testing.T, w *httptest.ResponseRecorder) capabilitiesDTO {
	t.Helper()
	var env struct {
		Data capabilitiesDTO `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return env.Data
}

// ---------- GET /capabilities ----------

func TestCapabilities_BothOff(t *testing.T) {
	svc := &mockIntegrationsService{
		capsResult: service.CapabilitiesResult{
			AIConfigured:    false,
			EmailConfigured: false,
			Providers: []service.ProviderCapability{
				{Provider: service.ProviderAnthropic, Configured: false},
				{Provider: service.ProviderResend, Configured: false},
			},
		},
	}
	h := NewIntegrationsHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/capabilities", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.Capabilities(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCapsOrgID.String() != testOrgID {
		t.Errorf("Capabilities got org=%s, want %s", svc.lastCapsOrgID, testOrgID)
	}
	caps := decodeCapabilities(t, w)
	if caps.AIConfigured || caps.EmailConfigured {
		t.Errorf("expected both off, got %+v", caps)
	}
	if len(caps.Providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(caps.Providers))
	}
	// Unconfigured providers must omit fingerprint/created_* (omitempty).
	for _, p := range caps.Providers {
		if p.Configured {
			t.Errorf("provider %s should be unconfigured", p.Provider)
		}
		if p.Fingerprint != "" || p.CreatedAt != nil || p.CreatedBy != "" {
			t.Errorf("unconfigured provider leaked metadata: %+v", p)
		}
	}
}

func TestCapabilities_AnthropicOn(t *testing.T) {
	created := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	svc := &mockIntegrationsService{
		capsResult: service.CapabilitiesResult{
			AIConfigured:    true,
			EmailConfigured: false,
			Providers: []service.ProviderCapability{
				{
					Provider:    service.ProviderAnthropic,
					Configured:  true,
					Fingerprint: "ab12",
					CreatedAt:   created,
					CreatedBy:   "owner-sub",
				},
				{Provider: service.ProviderResend, Configured: false},
			},
		},
	}
	h := NewIntegrationsHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/capabilities", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.Capabilities(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	caps := decodeCapabilities(t, w)
	if !caps.AIConfigured {
		t.Errorf("expected ai_configured=true, got %+v", caps)
	}
	if caps.EmailConfigured {
		t.Errorf("expected email_configured=false, got %+v", caps)
	}
	var anthropic *capabilityProviderDTO
	for i := range caps.Providers {
		if caps.Providers[i].Provider == service.ProviderAnthropic {
			anthropic = &caps.Providers[i]
		}
	}
	if anthropic == nil {
		t.Fatalf("anthropic provider missing: %+v", caps.Providers)
	}
	if !anthropic.Configured || anthropic.Fingerprint != "ab12" || anthropic.CreatedBy != "owner-sub" {
		t.Errorf("anthropic metadata not surfaced: %+v", anthropic)
	}
	if anthropic.CreatedAt == nil || !anthropic.CreatedAt.Equal(created) {
		t.Errorf("anthropic created_at wrong: %+v", anthropic.CreatedAt)
	}
}

func TestCapabilities_BothOn_ProvidersShape(t *testing.T) {
	svc := &mockIntegrationsService{
		capsResult: service.CapabilitiesResult{
			AIConfigured:    true,
			EmailConfigured: true,
			Providers: []service.ProviderCapability{
				{Provider: service.ProviderAnthropic, Configured: true, Fingerprint: "aaaa", CreatedBy: "o"},
				{Provider: service.ProviderResend, Configured: true, Fingerprint: "bbbb", CreatedBy: "o"},
			},
		},
	}
	h := NewIntegrationsHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/capabilities", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.Capabilities(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	caps := decodeCapabilities(t, w)
	if !caps.AIConfigured || !caps.EmailConfigured {
		t.Errorf("expected both on, got %+v", caps)
	}
	seen := map[string]bool{}
	for _, p := range caps.Providers {
		seen[p.Provider] = p.Configured
	}
	if !seen[service.ProviderAnthropic] || !seen[service.ProviderResend] {
		t.Errorf("both providers should be configured: %+v", caps.Providers)
	}
}

func TestCapabilities_InvalidOrgIDClaim_401(t *testing.T) {
	svc := &mockIntegrationsService{}
	h := NewIntegrationsHandler(svc)
	// Malformed org claim — callerOrgIDFromClaims should 401 before the
	// service is consulted.
	r := buildRequest(t, "GET", "/api/v1/capabilities", "not-a-uuid", nil, nil)
	w := httptest.NewRecorder()
	h.Capabilities(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status=%d, want 401", w.Code)
	}
}

func TestCapabilities_ServiceError_500(t *testing.T) {
	svc := &mockIntegrationsService{capsErr: assertErr("vault on fire")}
	h := NewIntegrationsHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/capabilities", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.Capabilities(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status=%d, want 500", w.Code)
	}
	// Underlying error must not leak.
	if w.Body.Len() > 0 && contains(w.Body.String(), "fire") {
		t.Errorf("internal error leaked: %s", w.Body.String())
	}
}

// contains is a tiny strings.Contains wrapper kept local to avoid an import
// just for one assertion (mirrors the lightweight style of the setup test).
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// decodeIntegrationsList unwraps {data:{integrations:[...]}}.
func decodeIntegrationsList(t *testing.T, w *httptest.ResponseRecorder) []integrationCredentialDTO {
	t.Helper()
	var env struct {
		Data struct {
			Integrations []integrationCredentialDTO `json:"integrations"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return env.Data.Integrations
}

// decodeIntegration unwraps {data:{integration:{...}}}.
func decodeIntegration(t *testing.T, w *httptest.ResponseRecorder) integrationCredentialDTO {
	t.Helper()
	var env struct {
		Data struct {
			Integration integrationCredentialDTO `json:"integration"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, w.Body.String())
	}
	return env.Data.Integration
}

// ---------- GET /integrations ----------

func TestIntegrationsList_OK(t *testing.T) {
	svc := &mockIntegrationsService{listResult: []models.IntegrationCredential{
		{ID: uuid.New(), Provider: "anthropic", Label: "prod", Last4: "1234", IsActive: true,
			Ciphertext: []byte("secret"), Nonce: []byte("nonce"), KeyVersion: 1},
	}}
	h := NewIntegrationsHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/integrations", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastListOrgID.String() != testOrgID {
		t.Errorf("org = %s, want %s", svc.lastListOrgID, testOrgID)
	}
	got := decodeIntegrationsList(t, w)
	if len(got) != 1 || got[0].Provider != "anthropic" || got[0].Last4 != "1234" {
		t.Fatalf("integrations = %+v", got)
	}
	// Secret bytes must never appear in the JSON body.
	if contains(w.Body.String(), "secret") || contains(w.Body.String(), "nonce") ||
		contains(w.Body.String(), "key_version") || contains(w.Body.String(), "ciphertext") {
		t.Errorf("secret material leaked: %s", w.Body.String())
	}
}

func TestIntegrationsList_EmptyIsArrayNotNull(t *testing.T) {
	h := NewIntegrationsHandler(&mockIntegrationsService{listResult: nil})
	r := buildRequest(t, "GET", "/api/v1/integrations", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	// The DTO slice is make([]..., 0) so it serializes as [] not null.
	if !contains(w.Body.String(), `"integrations":[]`) {
		t.Errorf("empty list should be [], got %s", w.Body.String())
	}
}

func TestIntegrationsList_ServiceErr500(t *testing.T) {
	h := NewIntegrationsHandler(&mockIntegrationsService{listErr: errInternal()})
	r := buildRequest(t, "GET", "/api/v1/integrations", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
}

// ---------- PUT /integrations/{provider} ----------

func TestIntegrationsSet_OK(t *testing.T) {
	svc := &mockIntegrationsService{setResult: models.IntegrationCredential{
		ID: uuid.New(), Provider: "anthropic", Label: "prod", Last4: "wxyz", IsActive: true,
	}}
	h := NewIntegrationsHandler(svc)
	// Provider param is mixed-case + padded — handler lowercases/trims it.
	r := buildRequest(t, "PUT", "/api/v1/integrations/Anthropic", testOrgID,
		map[string]string{"provider": "  Anthropic "}, strings.NewReader(`{"label":"prod","key":"sk-ant-secret"}`))
	w := httptest.NewRecorder()
	h.Set(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastSetInput.Provider != "anthropic" {
		t.Errorf("provider = %q, want lowercased/trimmed anthropic", svc.lastSetInput.Provider)
	}
	if svc.lastSetInput.Key != "sk-ant-secret" || svc.lastSetInput.Label != "prod" {
		t.Errorf("set input = %+v", svc.lastSetInput)
	}
	if svc.lastSetInput.OrgID.String() != testOrgID || svc.lastSetInput.UserSub != "test-sub" {
		t.Errorf("set input org/sub = %s/%q", svc.lastSetInput.OrgID, svc.lastSetInput.UserSub)
	}
	got := decodeIntegration(t, w)
	if got.Last4 != "wxyz" {
		t.Errorf("returned dto = %+v", got)
	}
}

func TestIntegrationsSet_EmptyProvider400(t *testing.T) {
	h := NewIntegrationsHandler(&mockIntegrationsService{})
	// No provider URL param → chi.URLParam returns "".
	r := buildRequest(t, "PUT", "/api/v1/integrations/", testOrgID, nil,
		strings.NewReader(`{"key":"k"}`))
	w := httptest.NewRecorder()
	h.Set(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestIntegrationsSet_BadJSON400(t *testing.T) {
	h := NewIntegrationsHandler(&mockIntegrationsService{})
	r := buildRequest(t, "PUT", "/api/v1/integrations/anthropic", testOrgID,
		map[string]string{"provider": "anthropic"}, strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.Set(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestIntegrationsSet_MissingKey400(t *testing.T) {
	h := NewIntegrationsHandler(&mockIntegrationsService{})
	r := buildRequest(t, "PUT", "/api/v1/integrations/anthropic", testOrgID,
		map[string]string{"provider": "anthropic"}, strings.NewReader(`{"label":"x"}`))
	w := httptest.NewRecorder()
	h.Set(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

func TestIntegrationsSet_ServiceValidation400(t *testing.T) {
	h := NewIntegrationsHandler(&mockIntegrationsService{setErr: wrapInvalid("unsupported provider")})
	r := buildRequest(t, "PUT", "/api/v1/integrations/bogus", testOrgID,
		map[string]string{"provider": "bogus"}, strings.NewReader(`{"key":"k"}`))
	w := httptest.NewRecorder()
	h.Set(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

// ---------- DELETE /integrations/{provider} ----------

func TestIntegrationsDelete_OK(t *testing.T) {
	svc := &mockIntegrationsService{}
	h := NewIntegrationsHandler(svc)
	r := buildRequest(t, "DELETE", "/api/v1/integrations/Resend", testOrgID,
		map[string]string{"provider": " Resend "}, nil)
	w := httptest.NewRecorder()
	h.Delete(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("204 should have empty body, got %s", w.Body.String())
	}
	if svc.lastDelProv != "resend" {
		t.Errorf("provider = %q, want lowercased/trimmed resend", svc.lastDelProv)
	}
	if svc.lastDelOrgID.String() != testOrgID || svc.lastDelUserSub != "test-sub" {
		t.Errorf("delete org/sub = %s/%q", svc.lastDelOrgID, svc.lastDelUserSub)
	}
}

func TestIntegrationsDelete_EmptyProvider400(t *testing.T) {
	h := NewIntegrationsHandler(&mockIntegrationsService{})
	r := buildRequest(t, "DELETE", "/api/v1/integrations/", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.Delete(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestIntegrationsDelete_NotFound404(t *testing.T) {
	h := NewIntegrationsHandler(&mockIntegrationsService{deleteErr: service.ErrNotFound})
	r := buildRequest(t, "DELETE", "/api/v1/integrations/anthropic", testOrgID,
		map[string]string{"provider": "anthropic"}, nil)
	w := httptest.NewRecorder()
	h.Delete(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
	if code := decodeErrCode(t, w); code != "NOT_FOUND" {
		t.Errorf("code=%q, want NOT_FOUND", code)
	}
}

// ---------- shared 401 guard leg (List/Set/Delete) ----------

type integrationsHandlerFn func(*IntegrationsHandler, http.ResponseWriter, *http.Request)

// TestIntegrations_AllHandlers_InvalidOrgIDClaim_401 covers the
// callerOrgIDFromClaims 401 short-circuit shared by List/Set/Delete: a
// malformed org claim is rejected before the provider param or body is
// touched, so the service is never consulted.
func TestIntegrations_AllHandlers_InvalidOrgIDClaim_401(t *testing.T) {
	cases := []struct {
		name   string
		method string
		fn     integrationsHandlerFn
	}{
		{"list", "GET", (*IntegrationsHandler).List},
		{"set", "PUT", (*IntegrationsHandler).Set},
		{"delete", "DELETE", (*IntegrationsHandler).Delete},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			svc := &mockIntegrationsService{}
			h := NewIntegrationsHandler(svc)
			r := buildRequest(t, c.method, "/api/v1/integrations/anthropic", "not-a-uuid",
				map[string]string{"provider": "anthropic"}, strings.NewReader(`{"key":"k"}`))
			w := httptest.NewRecorder()
			c.fn(h, w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status=%d, want 401", w.Code)
			}
		})
	}
}

// ---------- writeIntegrationError mapping ----------

func TestWriteIntegrationError_Mapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"not found", service.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"validation", wrapInvalid("bad"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"default", errInternal(), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			writeIntegrationError(w, r, tt.err)
			if w.Code != tt.wantStatus {
				t.Fatalf("status=%d, want %d", w.Code, tt.wantStatus)
			}
			if code := decodeErrCode(t, w); code != tt.wantCode {
				t.Errorf("code=%q, want %q", code, tt.wantCode)
			}
		})
	}
}
