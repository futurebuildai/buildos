package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// importStub / zeroTask are non-nil mock results so the ingress handlers write
// a 201 (a nil result would nil-deref in the handler). Bodies aren't asserted —
// these tests prove RBAC status codes only.
var (
	importStub = service.ImportScheduleResult{DependencyCount: 0}
	zeroTask   = models.ProjectTask{}
)

// rbacOrgID is the org_id baked into the X-Dev-Auth header by authedReq.
// It must match the {orgID} URL param for org-scoped routes (requireOrgIDFromURL
// 403s on a mismatch), so the org-scoped tests below use it in the path too.
const rbacOrgID = "11111111-1111-1111-1111-111111111111"

// authedReq builds a request authenticated via the DEV_AUTH_MODE=header bypass
// for the given role, mirroring authedGet but with an arbitrary method + body so
// the ingress POST routes' RBAC gates can be exercised through the full router.
func authedReq(method, target, role, body string) *http.Request {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("X-Dev-Auth", "router-sub,"+rbacOrgID+","+role)
	return r
}

// ingressRouter builds a router with the ingress services wired to mocks (so the
// handlers exist and the route tree mounts) using the header-auth bypass. The
// mocks return zero values; we only assert on STATUS CODES to prove the RBAC
// gate, never the body.
func ingressRouter() http.Handler {
	return NewRouter(RouterConfig{
		DevAuthMode:     "header",
		ScheduleService: &mockScheduleService{importResult: &importStub, createTaskResult: zeroTask},
		BudgetService:   &mockBudgetService{},
		HRService:       &fakeHRService{},
	})
}

// TestScheduleImport_RBAC proves the min-superintendent gate on the keystone
// import route: field_worker is 403, superintendent reaches the handler (200/201).
func TestScheduleImport_RBAC(t *testing.T) {
	handler := ingressRouter()
	const proj = "33333333-3333-3333-3333-333333333333"
	body := `{"tasks":[{"wbs_code":"01-00","name":"Site","duration_days":3}],"recalculate":false}`

	t.Run("field_worker forbidden", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, authedReq("POST", "/api/v1/projects/"+proj+"/schedule/import", "field_worker", body))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("import as field_worker = %d, want 403 (body=%s)", rec.Code, rec.Body.String())
		}
	})
	t.Run("superintendent allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, authedReq("POST", "/api/v1/projects/"+proj+"/schedule/import", "superintendent", body))
		if rec.Code != http.StatusCreated {
			t.Fatalf("import as superintendent = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
		}
	})
}

// TestCreateTask_RBAC proves the min-superintendent gate on POST /tasks.
func TestCreateTask_RBAC(t *testing.T) {
	handler := ingressRouter()
	const proj = "33333333-3333-3333-3333-333333333333"
	body := `{"wbs_code":"01-00","name":"Site","duration_days":3}`

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq("POST", "/api/v1/projects/"+proj+"/tasks", "field_worker", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create task as field_worker = %d, want 403", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq("POST", "/api/v1/projects/"+proj+"/tasks", "superintendent", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create task as superintendent = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestCreateBudgets_RBAC proves the owner/admin gate: superintendent is 403,
// admin reaches the handler.
func TestCreateBudgets_RBAC(t *testing.T) {
	handler := ingressRouter()
	const proj = "33333333-3333-3333-3333-333333333333"
	body := `{"budgets":[{"wbs_code":"01-00","phase_name":"Site","estimated_cost_cents":1000,"currency_code":"USD"}]}`

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq("POST", "/api/v1/projects/"+proj+"/budgets", "superintendent", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create budgets as superintendent = %d, want 403", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq("POST", "/api/v1/projects/"+proj+"/budgets", "admin", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create budgets as admin = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestCreateEmployee_RBAC proves the owner/admin group gate on the employees
// tree: superintendent is 403, admin reaches the handler.
func TestCreateEmployee_RBAC(t *testing.T) {
	handler := ingressRouter()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq("POST", "/api/v1/org/"+rbacOrgID+"/employees", "superintendent",
		`{"first_name":"Dana","last_name":"Cole","role":"Foreman"}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create employee as superintendent = %d, want 403", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq("POST", "/api/v1/org/"+rbacOrgID+"/employees", "admin",
		`{"first_name":"Dana","last_name":"Cole","role":"Foreman"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create employee as admin = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestCreateCertification_RBAC proves the owner/admin group gate on the cert
// route: field_worker is 403, owner reaches the handler.
func TestCreateCertification_RBAC(t *testing.T) {
	handler := ingressRouter()
	const emp = "55555555-5555-5555-5555-555555555555"
	body := `{"cert_type":"osha_10","expiry_date":"2027-01-15"}`

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq("POST", "/api/v1/org/"+rbacOrgID+"/employees/"+emp+"/certifications", "field_worker", body))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("create cert as field_worker = %d, want 403", rec.Code)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq("POST", "/api/v1/org/"+rbacOrgID+"/employees/"+emp+"/certifications", "owner", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create cert as owner = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
}
