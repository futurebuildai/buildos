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

type mockConnectorService struct {
	listResult []service.EffectiveConnector
	listErr    error
	setResult  models.ConnectorConfig
	setErr     error
	resetErr   error

	refreshCount int
	refreshErr   error

	lastListOrgID   uuid.UUID
	lastSetInput    service.SetConnectorInput
	lastResetOrg    uuid.UUID
	lastResetName   string
	lastResetSub    string
	lastRefreshOrg  uuid.UUID
	lastRefreshName string
}

func (m *mockConnectorService) ListEffective(_ context.Context, orgID uuid.UUID) ([]service.EffectiveConnector, error) {
	m.lastListOrgID = orgID
	return m.listResult, m.listErr
}
func (m *mockConnectorService) Set(_ context.Context, in service.SetConnectorInput) (models.ConnectorConfig, error) {
	m.lastSetInput = in
	return m.setResult, m.setErr
}
func (m *mockConnectorService) Reset(_ context.Context, orgID uuid.UUID, name, userSub string) error {
	m.lastResetOrg, m.lastResetName, m.lastResetSub = orgID, name, userSub
	return m.resetErr
}

func (m *mockConnectorService) RefreshTools(_ context.Context, orgID uuid.UUID, name, userSub string) (int, error) {
	m.lastRefreshOrg, m.lastRefreshName = orgID, name
	return m.refreshCount, m.refreshErr
}

func TestConnector_List_OK(t *testing.T) {
	svc := &mockConnectorService{listResult: []service.EffectiveConnector{{Connector: "reference", Enabled: false, Source: "default"}}}
	h := NewConnectorHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/admin/connectors", testOrgID, nil, nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastListOrgID.String() != testOrgID {
		t.Errorf("List org=%s, want %s", svc.lastListOrgID, testOrgID)
	}
	if !strings.Contains(w.Body.String(), "reference") {
		t.Errorf("body should list reference: %s", w.Body.String())
	}
}

func TestConnector_Set_OK_OrgAndSubFromClaims(t *testing.T) {
	svc := &mockConnectorService{setResult: models.ConnectorConfig{ConnectorName: "reference", Enabled: true}}
	h := NewConnectorHandler(svc)
	r := buildRequest(t, "PUT", "/api/v1/admin/connectors/reference", testOrgID,
		map[string]string{"connector": "reference"}, strings.NewReader(`{"enabled":true}`))
	w := httptest.NewRecorder()
	h.Set(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastSetInput.ConnectorName != "reference" || !svc.lastSetInput.Enabled {
		t.Errorf("Set input = %+v", svc.lastSetInput)
	}
	if svc.lastSetInput.OrgID.String() != testOrgID || svc.lastSetInput.UserSub != "test-sub" {
		t.Errorf("Set identity must come from claims: org=%s sub=%s", svc.lastSetInput.OrgID, svc.lastSetInput.UserSub)
	}
}

func TestConnector_Set_UnknownConnector_404(t *testing.T) {
	svc := &mockConnectorService{setErr: service.ErrNotFound}
	h := NewConnectorHandler(svc)
	r := buildRequest(t, "PUT", "/api/v1/admin/connectors/nope", testOrgID,
		map[string]string{"connector": "nope"}, strings.NewReader(`{"enabled":true}`))
	w := httptest.NewRecorder()
	h.Set(w, r)
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "NOT_FOUND") {
		t.Fatalf("status=%d body=%s, want 404 NOT_FOUND", w.Code, w.Body.String())
	}
}

func TestConnector_Set_BadConfig_400(t *testing.T) {
	svc := &mockConnectorService{setErr: service.ErrInvalidInput}
	h := NewConnectorHandler(svc)
	r := buildRequest(t, "PUT", "/api/v1/admin/connectors/reference", testOrgID,
		map[string]string{"connector": "reference"}, strings.NewReader(`{"enabled":true,"config":["bad"]}`))
	w := httptest.NewRecorder()
	h.Set(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "VALIDATION_ERROR") {
		t.Fatalf("status=%d body=%s, want 400 VALIDATION_ERROR", w.Code, w.Body.String())
	}
}

func TestConnector_Set_BadJSON_400(t *testing.T) {
	h := NewConnectorHandler(&mockConnectorService{})
	r := buildRequest(t, "PUT", "/api/v1/admin/connectors/reference", testOrgID,
		map[string]string{"connector": "reference"}, strings.NewReader(`{not json`))
	w := httptest.NewRecorder()
	h.Set(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestConnector_Reset_Idempotent204(t *testing.T) {
	svc := &mockConnectorService{}
	h := NewConnectorHandler(svc)
	r := buildRequest(t, "DELETE", "/api/v1/admin/connectors/reference", testOrgID,
		map[string]string{"connector": "reference"}, nil)
	w := httptest.NewRecorder()
	h.Reset(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204, body=%s", w.Code, w.Body.String())
	}
	if svc.lastResetName != "reference" || svc.lastResetOrg.String() != testOrgID || svc.lastResetSub != "test-sub" {
		t.Errorf("Reset args = (%s, %s, %s)", svc.lastResetOrg, svc.lastResetName, svc.lastResetSub)
	}
}

func TestConnector_Refresh_OK(t *testing.T) {
	svc := &mockConnectorService{refreshCount: 5}
	h := NewConnectorHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/admin/connectors/acme/refresh", testOrgID,
		map[string]string{"connector": "acme"}, nil)
	w := httptest.NewRecorder()
	h.Refresh(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastRefreshName != "acme" || svc.lastRefreshOrg.String() != testOrgID {
		t.Errorf("Refresh args = (%s, %s)", svc.lastRefreshOrg, svc.lastRefreshName)
	}
	if !strings.Contains(w.Body.String(), "5") {
		t.Errorf("body should report the tools_count: %s", w.Body.String())
	}
}

func TestConnector_Refresh_Unavailable_502(t *testing.T) {
	svc := &mockConnectorService{refreshErr: service.ErrConnectorUnavailable}
	h := NewConnectorHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/admin/connectors/acme/refresh", testOrgID,
		map[string]string{"connector": "acme"}, nil)
	w := httptest.NewRecorder()
	h.Refresh(w, r)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "UPSTREAM_ERROR") {
		t.Fatalf("status=%d body=%s, want 502 UPSTREAM_ERROR", w.Code, w.Body.String())
	}
}

func TestConnector_Refresh_NotMCP_400(t *testing.T) {
	svc := &mockConnectorService{refreshErr: service.ErrInvalidInput}
	h := NewConnectorHandler(svc)
	r := buildRequest(t, "POST", "/api/v1/admin/connectors/reference/refresh", testOrgID,
		map[string]string{"connector": "reference"}, nil)
	w := httptest.NewRecorder()
	h.Refresh(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400, body=%s", w.Code, w.Body.String())
	}
}

func TestConnector_RBAC_SuperintendentForbidden(t *testing.T) {
	svc := &mockConnectorService{}
	router := chi.NewRouter()
	MountConnectorRoutes(router, NewConnectorHandler(svc))

	req := httptest.NewRequest("GET", "/api/v1/admin/connectors", nil)
	req = req.WithContext(mw.ContextWithClaims(req.Context(), mw.Claims{
		Sub: "test-sub", OrgID: testOrgID, Role: mw.RoleSuperintendent,
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

func TestConnector_RBAC_AdminAllowed(t *testing.T) {
	svc := &mockConnectorService{listResult: []service.EffectiveConnector{}}
	router := chi.NewRouter()
	MountConnectorRoutes(router, NewConnectorHandler(svc))

	req := httptest.NewRequest("GET", "/api/v1/admin/connectors", nil)
	req = req.WithContext(mw.ContextWithClaims(req.Context(), mw.Claims{
		Sub: "test-sub", OrgID: testOrgID, Role: mw.RoleAdmin,
	}))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200 for admin, body=%s", w.Code, w.Body.String())
	}
}
