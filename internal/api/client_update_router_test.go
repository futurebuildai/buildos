package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// fakeClientUpdateService is a router-level fake ClientUpdateServicer. Records
// the org id (org-scope assertion) and returns canned results/errors.
type fakeClientUpdateService struct {
	createErr error
	createRes models.ClientUpdate
	updateErr error
	updateRes models.ClientUpdate
	sendErr   error
	sendRes   models.ClientUpdate
	listErr   error
	listRes   []models.ClientUpdate
	getErr    error
	getRes    models.ClientUpdate

	gotOrgID uuid.UUID
}

func (f *fakeClientUpdateService) CreateDraft(_ context.Context, orgID uuid.UUID, _ string, _ service.CreateDraftInput) (models.ClientUpdate, error) {
	f.gotOrgID = orgID
	return f.createRes, f.createErr
}
func (f *fakeClientUpdateService) UpdateDraft(_ context.Context, orgID uuid.UUID, _ string, _ service.UpdateDraftInput) (models.ClientUpdate, error) {
	f.gotOrgID = orgID
	return f.updateRes, f.updateErr
}
func (f *fakeClientUpdateService) SendClientUpdate(_ context.Context, orgID uuid.UUID, _ string, _ uuid.UUID) (models.ClientUpdate, error) {
	f.gotOrgID = orgID
	return f.sendRes, f.sendErr
}
func (f *fakeClientUpdateService) ListByProject(_ context.Context, orgID, _ uuid.UUID) ([]models.ClientUpdate, error) {
	f.gotOrgID = orgID
	return f.listRes, f.listErr
}
func (f *fakeClientUpdateService) Get(_ context.Context, orgID, _ uuid.UUID) (models.ClientUpdate, error) {
	f.gotOrgID = orgID
	return f.getRes, f.getErr
}

const cuProjID = "44444444-4444-4444-4444-444444444444"
const cuID = "55555555-5555-5555-5555-555555555555"

func cuRouter(svc ClientUpdateServicer) http.Handler {
	return NewRouter(RouterConfig{DevAuthMode: "header", ClientUpdateService: svc})
}

// ---- RBAC: owner/admin only (§9-1) -------------------------------------

func TestClientUpdateRoutes_Create_RBAC(t *testing.T) {
	body := `{"report_date":"2026-06-09"}`
	cases := []struct {
		role string
		want int
	}{
		{"field_worker", http.StatusForbidden},
		{"superintendent", http.StatusForbidden},
		{"admin", http.StatusCreated},
		{"owner", http.StatusCreated},
	}
	for _, tc := range cases {
		t.Run(tc.role, func(t *testing.T) {
			h := cuRouter(&fakeClientUpdateService{createRes: models.ClientUpdate{ID: uuid.New(), Status: "draft"}})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/projects/"+cuProjID+"/client-updates", tc.role, body))
			if rec.Code != tc.want {
				t.Fatalf("create as %s = %d, want %d (body=%s)", tc.role, rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}

func TestClientUpdateRoutes_Send_RBAC(t *testing.T) {
	for _, role := range []string{"field_worker", "superintendent"} {
		t.Run(role+" forbidden", func(t *testing.T) {
			h := cuRouter(&fakeClientUpdateService{})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/client-updates/"+cuID+"/send", role, ""))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("send as %s = %d, want 403", role, rec.Code)
			}
		})
	}
}

// ---- org-scope: handler passes the caller's claim org ------------------

func TestClientUpdateRoutes_List_OrgScoped(t *testing.T) {
	fake := &fakeClientUpdateService{listRes: []models.ClientUpdate{{ID: uuid.New(), Status: "sent"}}}
	h := cuRouter(fake)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/projects/"+cuProjID+"/client-updates", "owner", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if fake.gotOrgID.String() != rbacOrgID {
		t.Fatalf("handler passed org %s, want caller's claim org", fake.gotOrgID)
	}
}

// ---- send error mappings (the operator MUST know it did not send) ------

func TestClientUpdateRoutes_Send_NoClientContact422(t *testing.T) {
	h := cuRouter(&fakeClientUpdateService{sendErr: service.ErrNoClientContact})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/client-updates/"+cuID+"/send", "owner", ""))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("send no-contact = %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "NO_CLIENT_CONTACT") {
		t.Fatalf("body missing NO_CLIENT_CONTACT: %s", rec.Body.String())
	}
}

func TestClientUpdateRoutes_Send_MailerUnconfigured422(t *testing.T) {
	h := cuRouter(&fakeClientUpdateService{sendErr: service.ErrMailerUnconfigured})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/client-updates/"+cuID+"/send", "owner", ""))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("send mailer-unconfigured = %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MAILER_UNCONFIGURED") {
		t.Fatalf("body missing MAILER_UNCONFIGURED: %s", rec.Body.String())
	}
}

func TestClientUpdateRoutes_Send_AlreadySent409(t *testing.T) {
	h := cuRouter(&fakeClientUpdateService{sendErr: service.ErrAlreadySent})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/client-updates/"+cuID+"/send", "admin", ""))
	if rec.Code != http.StatusConflict {
		t.Fatalf("re-send = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "ALREADY_SENT") {
		t.Fatalf("body missing ALREADY_SENT: %s", rec.Body.String())
	}
}

func TestClientUpdateRoutes_Create_AIUnavailable503(t *testing.T) {
	h := cuRouter(&fakeClientUpdateService{createErr: service.ErrClientUpdateAIUnavailable})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/projects/"+cuProjID+"/client-updates", "owner", `{"report_date":"2026-06-09"}`))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("create AI-unavailable = %d, want 503 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestClientUpdateRoutes_Create_BadDate400(t *testing.T) {
	h := cuRouter(&fakeClientUpdateService{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPost, "/api/v1/projects/"+cuProjID+"/client-updates", "owner", `{"report_date":"06-09-2026"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create bad date = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestClientUpdateRoutes_Get_CrossOrg404(t *testing.T) {
	h := cuRouter(&fakeClientUpdateService{getErr: service.ErrNotFound})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/client-updates/"+cuID, "owner", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET cross-org = %d, want 404", rec.Code)
	}
}

func TestClientUpdateRoutes_Update_AlreadySent409(t *testing.T) {
	h := cuRouter(&fakeClientUpdateService{updateErr: service.ErrAlreadySent})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodPatch, "/api/v1/client-updates/"+cuID, "owner", `{"subject":"x","edited_body":"y"}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("PATCH sent = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
}

// recipient_email must never appear in a list/get response body.
func TestClientUpdateRoutes_Get_NoRecipientEmailInBody(t *testing.T) {
	addr := "home@owner.example"
	h := cuRouter(&fakeClientUpdateService{getRes: models.ClientUpdate{ID: uuid.New(), Status: "sent", RecipientEmail: &addr}})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/client-updates/"+cuID, "owner", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("get = %d, want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), addr) {
		t.Fatalf("recipient_email leaked into response: %s", rec.Body.String())
	}
}

func TestClientUpdateRoutes_SkippedWhenNil(t *testing.T) {
	h := NewRouter(RouterConfig{DevAuthMode: "header"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, authedReq(http.MethodGet, "/api/v1/projects/"+cuProjID+"/client-updates", "owner", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("client-updates with ClientUpdateService nil = %d, want 404", rec.Code)
	}
}
