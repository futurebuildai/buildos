package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	mw "github.com/futurebuildai/buildos/internal/api/middleware"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// mockAgentConfigService implements AgentConfigServicer for handler tests.
type mockAgentConfigService struct {
	listResult []service.EffectiveAgentConfig
	listErr    error
	setResult  models.AgentConfig
	setErr     error
	resetErr   error

	lastListOrgID uuid.UUID
	lastSetInput  service.SetAgentConfigInput
	lastResetOrg  uuid.UUID
	lastResetCap  string
	lastResetSub  string
}

func (m *mockAgentConfigService) ListEffective(_ context.Context, orgID uuid.UUID) ([]service.EffectiveAgentConfig, error) {
	m.lastListOrgID = orgID
	return m.listResult, m.listErr
}
func (m *mockAgentConfigService) Set(_ context.Context, in service.SetAgentConfigInput) (models.AgentConfig, error) {
	m.lastSetInput = in
	return m.setResult, m.setErr
}
func (m *mockAgentConfigService) Reset(_ context.Context, orgID uuid.UUID, capability, userSub string) error {
	m.lastResetOrg, m.lastResetCap, m.lastResetSub = orgID, capability, userSub
	return m.resetErr
}

// ---------- GET /api/v1/admin/agents ----------

func TestAgentConfig_List_OK(t *testing.T) {
	svc := &mockAgentConfigService{listResult: []service.EffectiveAgentConfig{
		{Capability: "foresight", Enabled: true, Source: "default"},
	}}
	h := NewAgentConfigHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/admin/agents", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastListOrgID.String() != testOrgID {
		t.Errorf("List got org=%s, want %s", svc.lastListOrgID, testOrgID)
	}
	if !strings.Contains(w.Body.String(), "foresight") {
		t.Errorf("body should list foresight: %s", w.Body.String())
	}
}

// ---------- PUT /api/v1/admin/agents/{capability} ----------

func TestAgentConfig_Set_OK(t *testing.T) {
	svc := &mockAgentConfigService{setResult: models.AgentConfig{Capability: "foresight", Enabled: false}}
	h := NewAgentConfigHandler(svc)
	r := buildRequest(t, "PUT", "/api/v1/admin/agents/foresight", testOrgID,
		map[string]string{"capability": "foresight"},
		strings.NewReader(`{"enabled":false,"config":{"budget_burn_percent":50}}`))
	w := httptest.NewRecorder()
	h.Set(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastSetInput.Capability != "foresight" || svc.lastSetInput.Enabled {
		t.Errorf("Set input = %+v, want capability=foresight enabled=false", svc.lastSetInput)
	}
	if svc.lastSetInput.OrgID.String() != testOrgID {
		t.Errorf("Set org = %s, want %s (from claims, not body)", svc.lastSetInput.OrgID, testOrgID)
	}
	if svc.lastSetInput.UserSub != "test-sub" {
		t.Errorf("Set UserSub = %q, want test-sub (from claims)", svc.lastSetInput.UserSub)
	}
}

func TestAgentConfig_Set_UnknownCapability_404(t *testing.T) {
	svc := &mockAgentConfigService{setErr: service.ErrNotFound}
	h := NewAgentConfigHandler(svc)
	r := buildRequest(t, "PUT", "/api/v1/admin/agents/nope", testOrgID,
		map[string]string{"capability": "nope"}, strings.NewReader(`{"enabled":true}`))
	w := httptest.NewRecorder()
	h.Set(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Errorf("body missing NOT_FOUND: %s", w.Body.String())
	}
}

func TestAgentConfig_Set_MalformedConfig_400(t *testing.T) {
	svc := &mockAgentConfigService{setErr: service.ErrInvalidInput}
	h := NewAgentConfigHandler(svc)
	r := buildRequest(t, "PUT", "/api/v1/admin/agents/foresight", testOrgID,
		map[string]string{"capability": "foresight"}, strings.NewReader(`{"enabled":true,"config":{"budget_burn_percent":-9}}`))
	w := httptest.NewRecorder()
	h.Set(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "VALIDATION_ERROR") {
		t.Errorf("body missing VALIDATION_ERROR: %s", w.Body.String())
	}
}

func TestAgentConfig_Set_BadJSON_400(t *testing.T) {
	h := NewAgentConfigHandler(&mockAgentConfigService{})
	r := buildRequest(t, "PUT", "/api/v1/admin/agents/foresight", testOrgID,
		map[string]string{"capability": "foresight"}, strings.NewReader(`{not json`))
	w := httptest.NewRecorder()
	h.Set(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", w.Code, w.Body.String())
	}
}

// ---------- DELETE /api/v1/admin/agents/{capability} ----------

func TestAgentConfig_Reset_Idempotent204(t *testing.T) {
	svc := &mockAgentConfigService{} // resetErr nil — reset succeeds (whether or not a row existed)
	h := NewAgentConfigHandler(svc)
	r := buildRequest(t, "DELETE", "/api/v1/admin/agents/foresight", testOrgID,
		map[string]string{"capability": "foresight"}, nil)
	w := httptest.NewRecorder()
	h.Reset(w, r)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204, body=%s", w.Code, w.Body.String())
	}
	if svc.lastResetCap != "foresight" || svc.lastResetOrg.String() != testOrgID || svc.lastResetSub != "test-sub" {
		t.Errorf("Reset args = (%s, %s, %s), want (%s, foresight, test-sub)", svc.lastResetOrg, svc.lastResetCap, svc.lastResetSub, testOrgID)
	}
}

func TestAgentConfig_Reset_UnknownCapability_404(t *testing.T) {
	svc := &mockAgentConfigService{resetErr: service.ErrNotFound}
	h := NewAgentConfigHandler(svc)
	r := buildRequest(t, "DELETE", "/api/v1/admin/agents/nope", testOrgID,
		map[string]string{"capability": "nope"}, nil)
	w := httptest.NewRecorder()
	h.Reset(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404, body=%s", w.Code, w.Body.String())
	}
}

// ---------- RBAC (route-level) ----------

// TestAgentConfig_RBAC_SuperintendentForbidden drives a request through the
// MOUNTED routes (so RequireMinRole(admin) actually runs) with a superintendent
// caller, asserting the admin gate returns 403 before the handler runs.
func TestAgentConfig_RBAC_SuperintendentForbidden(t *testing.T) {
	svc := &mockAgentConfigService{}
	router := chi.NewRouter()
	MountAgentConfigRoutes(router, NewAgentConfigHandler(svc))

	req := httptest.NewRequest("GET", "/api/v1/admin/agents", nil)
	req = req.WithContext(mw.ContextWithClaims(req.Context(), mw.Claims{
		Sub:   "test-sub",
		OrgID: testOrgID,
		Role:  mw.RoleSuperintendent, // below admin
	}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403 (admin gate), body=%s", w.Code, w.Body.String())
	}
	if svc.lastListOrgID != uuid.Nil {
		t.Error("handler must not run when the admin gate rejects the caller")
	}
}

func TestAgentConfig_RBAC_AdminAllowed(t *testing.T) {
	svc := &mockAgentConfigService{listResult: []service.EffectiveAgentConfig{}}
	router := chi.NewRouter()
	MountAgentConfigRoutes(router, NewAgentConfigHandler(svc))

	req := httptest.NewRequest("GET", "/api/v1/admin/agents", nil)
	req = req.WithContext(mw.ContextWithClaims(req.Context(), mw.Claims{
		Sub:   "test-sub",
		OrgID: testOrgID,
		Role:  mw.RoleAdmin,
	}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 for admin, body=%s", w.Code, w.Body.String())
	}
}
