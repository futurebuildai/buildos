package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

// ---------- Budget batch create handler ----------

// TestCreateBudgets_PassesClaimsAndLines proves the handler maps the batch
// body to service lines and derives org+sub from claims.
func TestCreateBudgets_PassesClaimsAndLines(t *testing.T) {
	svc := &mockBudgetService{createBudgetsResult: []models.ProjectBudget{{WBSCode: "01-00"}}}
	h := NewFinancialsHandler(svc)
	body := strings.NewReader(`{"budgets":[
		{"wbs_code":"01-00","phase_name":"Site","estimated_cost_cents":4500000,"currency_code":"USD"},
		{"wbs_code":"03-30","phase_name":"Foundation","estimated_cost_cents":12000000,"currency_code":"USD"}]}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/budgets",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.CreateBudgets(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCallerOrgID.String() != testOrgID {
		t.Errorf("org = %s, want %s (from claims)", svc.lastCallerOrgID, testOrgID)
	}
	if svc.lastBudgetUserSub != "test-sub" {
		t.Errorf("user_sub = %q, want test-sub", svc.lastBudgetUserSub)
	}
	if len(svc.lastBudgetLines) != 2 {
		t.Fatalf("lines = %d, want 2", len(svc.lastBudgetLines))
	}
	if svc.lastBudgetLines[0].EstimatedCostCents != 4500000 || svc.lastBudgetLines[0].CurrencyCode != "USD" {
		t.Errorf("line[0] = %+v, want 4500000/USD", svc.lastBudgetLines[0])
	}
}

// TestCreateBudgets_BadJSON400 proves a malformed body is rejected pre-service.
func TestCreateBudgets_BadJSON400(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{})
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/budgets",
		testOrgID, map[string]string{"projectID": testProjID}, strings.NewReader(`{bad`))
	w := httptest.NewRecorder()
	h.CreateBudgets(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// TestCreateBudgets_CrossCurrencyMaps422 proves the budget handler maps
// ErrCrossCurrency to 422 (the existing writeServiceError already handles it).
func TestCreateBudgets_CrossCurrencyMaps422(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{createBudgetsErr: service.ErrCrossCurrency})
	body := strings.NewReader(`{"budgets":[{"wbs_code":"01-00","phase_name":"Site","estimated_cost_cents":1,"currency_code":"USD"}]}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/budgets",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.CreateBudgets(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status=%d, want 422", w.Code)
	}
}

// TestCreateBudgets_InvalidInputMaps400 proves a validation error → 400.
func TestCreateBudgets_InvalidInputMaps400(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{createBudgetsErr: service.ErrInvalidInput})
	body := strings.NewReader(`{"budgets":[{"wbs_code":"01-00","phase_name":"Site","estimated_cost_cents":1,"currency_code":"EUR"}]}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/budgets",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.CreateBudgets(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// ---------- HR create handlers ----------

// TestCreateEmployee_PassesClaims proves org+sub come from claims (URL org must
// match the claim — requireOrgIDFromURL) and the body fields are forwarded.
func TestCreateEmployee_PassesClaims(t *testing.T) {
	svc := &fakeHRService{createEmpResult: models.Employee{FirstName: "Dana"}}
	h := NewHRHandler(svc)
	body := strings.NewReader(`{"first_name":"Dana","last_name":"Cole","role":"Foreman","hire_date":"2024-03-01"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/employees",
		testOrgID, map[string]string{"orgID": testOrgID}, body)
	w := httptest.NewRecorder()
	h.CreateEmployee(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotCreateEmpInput.OrgID.String() != testOrgID {
		t.Errorf("org = %s, want %s (from claims)", svc.gotCreateEmpInput.OrgID, testOrgID)
	}
	if svc.gotCreateEmpInput.CallerUserSub != "test-sub" {
		t.Errorf("user_sub = %q, want test-sub", svc.gotCreateEmpInput.CallerUserSub)
	}
	if svc.gotCreateEmpInput.Role != "Foreman" || svc.gotCreateEmpInput.HireDate == nil {
		t.Errorf("body not forwarded: %+v", svc.gotCreateEmpInput)
	}
}

// TestCreateEmployee_OrgMismatch403 proves the URL-vs-claim org guard.
func TestCreateEmployee_OrgMismatch403(t *testing.T) {
	h := NewHRHandler(&fakeHRService{})
	body := strings.NewReader(`{"first_name":"Dana","last_name":"Cole","role":"Foreman"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+otherOrgID+"/employees",
		testOrgID, map[string]string{"orgID": otherOrgID}, body)
	w := httptest.NewRecorder()
	h.CreateEmployee(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("status=%d, want 403", w.Code)
	}
}

// TestCreateEmployee_InvalidInputMaps400 proves a validation error → 400.
func TestCreateEmployee_InvalidInputMaps400(t *testing.T) {
	h := NewHRHandler(&fakeHRService{createEmpErr: service.ErrInvalidInput})
	body := strings.NewReader(`{"first_name":"","last_name":"","role":""}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/employees",
		testOrgID, map[string]string{"orgID": testOrgID}, body)
	w := httptest.NewRecorder()
	h.CreateEmployee(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// TestCreateCertification_PassesClaimsAndDates proves the cert handler parses
// the required expiry date, forwards the optional issued date, and scopes by
// employee + claim org.
func TestCreateCertification_PassesClaimsAndDates(t *testing.T) {
	svc := &fakeHRService{createCertResult: models.Certification{CertType: "osha_10"}}
	h := NewHRHandler(svc)
	const emp = "55555555-5555-5555-5555-555555555555"
	body := strings.NewReader(`{"cert_type":"osha_10","cert_number":"X-1","issued_date":"2024-01-15","expiry_date":"2027-01-15","status":"active"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/employees/"+emp+"/certifications",
		testOrgID, map[string]string{"orgID": testOrgID, "employeeID": emp}, body)
	w := httptest.NewRecorder()
	h.CreateCertification(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.gotCreateCertInput.EmployeeID.String() != emp {
		t.Errorf("employee = %s, want %s", svc.gotCreateCertInput.EmployeeID, emp)
	}
	if svc.gotCreateCertInput.OrgID.String() != testOrgID {
		t.Errorf("org = %s, want %s (from claims)", svc.gotCreateCertInput.OrgID, testOrgID)
	}
	if svc.gotCreateCertInput.ExpiryDate.IsZero() {
		t.Error("expiry_date not parsed")
	}
	if svc.gotCreateCertInput.IssuedDate == nil {
		t.Error("issued_date not forwarded")
	}
}

// TestCreateCertification_BadExpiry400 proves a missing/invalid expiry is 400.
func TestCreateCertification_BadExpiry400(t *testing.T) {
	h := NewHRHandler(&fakeHRService{})
	const emp = "55555555-5555-5555-5555-555555555555"
	body := strings.NewReader(`{"cert_type":"osha_10"}`) // no expiry_date
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/employees/"+emp+"/certifications",
		testOrgID, map[string]string{"orgID": testOrgID, "employeeID": emp}, body)
	w := httptest.NewRecorder()
	h.CreateCertification(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

// TestCreateCertification_EmployeeNotFound404 proves a cross-org employee
// surfaces as 404 (never leak existence).
func TestCreateCertification_EmployeeNotFound404(t *testing.T) {
	h := NewHRHandler(&fakeHRService{createCertErr: service.ErrEmployeeNotFound})
	const emp = "55555555-5555-5555-5555-555555555555"
	body := strings.NewReader(`{"cert_type":"osha_10","expiry_date":"2027-01-15"}`)
	r := buildRequest(t, "POST", "/api/v1/org/"+testOrgID+"/employees/"+emp+"/certifications",
		testOrgID, map[string]string{"orgID": testOrgID, "employeeID": emp}, body)
	w := httptest.NewRecorder()
	h.CreateCertification(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}
