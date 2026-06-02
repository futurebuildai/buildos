package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	lastCapsOrgID uuid.UUID
}

func (m *mockIntegrationsService) SetCredential(_ context.Context, _ service.SetCredentialInput) (models.IntegrationCredential, error) {
	return m.setResult, m.setErr
}
func (m *mockIntegrationsService) DeleteCredential(_ context.Context, _ uuid.UUID, _, _ string) error {
	return m.deleteErr
}
func (m *mockIntegrationsService) ListCredentials(_ context.Context, _ uuid.UUID) ([]models.IntegrationCredential, error) {
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
