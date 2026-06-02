package api

import (
	"context"
	"encoding/json"
	"io"
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

// mockBudgetService implements BudgetServicer for handler tests. Each method
// returns whatever was last assigned to its corresponding *Result/Err field.
type mockBudgetService struct {
	getProjectBudgetsResult []models.ProjectBudget
	getProjectBudgetsErr    error

	getOrgFinancialsSummaryResult service.FinancialsSummary
	getOrgFinancialsSummaryErr    error

	getARAgingResult []models.ARAgingSnapshot
	getARAgingErr    error

	getProjectFinancialsResult []models.ProjectFinancial
	getProjectFinancialsErr    error

	createInvoiceResult models.Invoice
	createInvoiceErr    error

	updateInvoiceResult models.Invoice
	updateInvoiceErr    error

	// Captured call args for assertions.
	lastCallerOrgID  uuid.UUID
	lastCurrencyCode string
	lastProjectID    uuid.UUID
}

func (m *mockBudgetService) GetProjectBudgets(_ context.Context, projectID, callerOrgID uuid.UUID) ([]models.ProjectBudget, error) {
	m.lastProjectID = projectID
	m.lastCallerOrgID = callerOrgID
	return m.getProjectBudgetsResult, m.getProjectBudgetsErr
}

func (m *mockBudgetService) GetOrgFinancialsSummary(_ context.Context, orgID uuid.UUID, currencyCode string) (service.FinancialsSummary, error) {
	m.lastCallerOrgID = orgID
	m.lastCurrencyCode = currencyCode
	return m.getOrgFinancialsSummaryResult, m.getOrgFinancialsSummaryErr
}

func (m *mockBudgetService) GetARAging(_ context.Context, orgID uuid.UUID, currencyCode string) ([]models.ARAgingSnapshot, error) {
	m.lastCallerOrgID = orgID
	m.lastCurrencyCode = currencyCode
	return m.getARAgingResult, m.getARAgingErr
}

func (m *mockBudgetService) GetProjectFinancials(_ context.Context, orgID uuid.UUID, currencyCode string) ([]models.ProjectFinancial, error) {
	m.lastCallerOrgID = orgID
	m.lastCurrencyCode = currencyCode
	return m.getProjectFinancialsResult, m.getProjectFinancialsErr
}

func (m *mockBudgetService) CreateInvoice(_ context.Context, callerOrgID uuid.UUID, _ string, _ service.CreateInvoiceInput) (models.Invoice, error) {
	m.lastCallerOrgID = callerOrgID
	return m.createInvoiceResult, m.createInvoiceErr
}

func (m *mockBudgetService) UpdateInvoice(_ context.Context, callerOrgID uuid.UUID, _ string, _ service.UpdateInvoiceInput) (models.Invoice, error) {
	m.lastCallerOrgID = callerOrgID
	return m.updateInvoiceResult, m.updateInvoiceErr
}

const (
	testOrgID  = "11111111-1111-1111-1111-111111111111"
	otherOrgID = "22222222-2222-2222-2222-222222222222"
	testProjID = "33333333-3333-3333-3333-333333333333"
	testInvID  = "44444444-4444-4444-4444-444444444444"
)

// buildRequest constructs a request whose context carries:
//   - JWT claims for the caller (via middleware.ContextWithClaims)
//   - Chi URL params (via chi.RouteCtxKey)
//
// Both layers are required because the handler reads from each.
func buildRequest(t *testing.T, method, target, callerOrgID string, params map[string]string, body io.Reader) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, target, body)
	ctx := mw.ContextWithClaims(r.Context(), mw.Claims{
		Sub:   "test-sub",
		OrgID: callerOrgID,
		Role:  "owner",
	})
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	r = r.WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	return r
}

// ---------- /financials/summary ----------

func TestSummary_OK(t *testing.T) {
	svc := &mockBudgetService{
		getOrgFinancialsSummaryResult: service.FinancialsSummary{
			CorporateBudgets: []models.CorporateBudget{{CurrencyCode: "USD", TotalEstimatedCents: 10000}},
		},
	}
	h := NewFinancialsHandler(svc)

	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/financials/summary?currency=USD",
		testOrgID, map[string]string{"orgID": testOrgID}, nil)
	w := httptest.NewRecorder()
	h.Summary(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCurrencyCode != "USD" {
		t.Errorf("currency passed to service = %q, want USD", svc.lastCurrencyCode)
	}
	if svc.lastCallerOrgID.String() != testOrgID {
		t.Errorf("orgID passed to service = %s, want %s", svc.lastCallerOrgID, testOrgID)
	}
}

func TestSummary_OrgMismatchReturns403(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{})
	r := buildRequest(t, "GET", "/api/v1/org/"+otherOrgID+"/financials/summary",
		testOrgID, map[string]string{"orgID": otherOrgID}, nil)
	w := httptest.NewRecorder()
	h.Summary(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "FORBIDDEN") {
		t.Errorf("body should contain FORBIDDEN: %s", w.Body.String())
	}
}

func TestSummary_InvalidOrgIDReturns400(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{})
	r := buildRequest(t, "GET", "/api/v1/org/not-a-uuid/financials/summary",
		testOrgID, map[string]string{"orgID": "not-a-uuid"}, nil)
	w := httptest.NewRecorder()
	h.Summary(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestSummary_ServiceInvalidInputMaps400(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{
		getOrgFinancialsSummaryErr: service.ErrInvalidInput,
	})
	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/financials/summary?currency=EUR",
		testOrgID, map[string]string{"orgID": testOrgID}, nil)
	w := httptest.NewRecorder()
	h.Summary(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "VALIDATION_ERROR") {
		t.Errorf("body should contain VALIDATION_ERROR: %s", w.Body.String())
	}
}

// ---------- /projects/{id}/budgets ----------

func TestListBudgets_NotFoundReturns404(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{
		getProjectBudgetsErr: service.ErrNotFound,
	})
	r := buildRequest(t, "GET", "/api/v1/projects/"+testProjID+"/budgets",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.ListBudgets(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestListBudgets_OK(t *testing.T) {
	svc := &mockBudgetService{
		getProjectBudgetsResult: []models.ProjectBudget{{
			WBSCode:                   "9.0",
			PhaseName:                 "Roofing",
			EstimatedCostCents:        450000,
			EstimatedCostCurrencyCode: "USD",
		}},
	}
	h := NewFinancialsHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/projects/"+testProjID+"/budgets",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.ListBudgets(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCallerOrgID.String() != testOrgID {
		t.Errorf("service got caller_org=%s, want %s", svc.lastCallerOrgID, testOrgID)
	}
	if svc.lastProjectID.String() != testProjID {
		t.Errorf("service got project_id=%s, want %s", svc.lastProjectID, testProjID)
	}
}

// ---------- /projects/{id}/invoices ----------

func TestCreateInvoice_HappyPath(t *testing.T) {
	projectID := uuid.MustParse(testProjID)
	svc := &mockBudgetService{
		createInvoiceResult: models.Invoice{
			ProjectID:    projectID,
			VendorName:   "Acme Lumber",
			AmountCents:  150000,
			CurrencyCode: "USD",
			Status:       models.InvoiceStatusPending,
		},
	}
	h := NewFinancialsHandler(svc)

	body := strings.NewReader(`{"vendor_name":"Acme Lumber","amount_cents":150000,"currency_code":"USD"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/invoices",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.CreateInvoice(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Invoice models.Invoice `json:"invoice"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, w.Body.String())
	}
	if resp.Data.Invoice.VendorName != "Acme Lumber" {
		t.Errorf("vendor=%q", resp.Data.Invoice.VendorName)
	}
}

func TestCreateInvoice_CrossCurrencyMaps422(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{
		createInvoiceErr: service.ErrCrossCurrency,
	})
	body := strings.NewReader(`{"vendor_name":"X","amount_cents":1,"currency_code":"USD"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/invoices",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.CreateInvoice(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status=%d, want 422; body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "CROSS_CURRENCY_ERROR") {
		t.Errorf("body should contain CROSS_CURRENCY_ERROR: %s", w.Body.String())
	}
}

func TestCreateInvoice_BadJSONReturns400(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{})
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/invoices",
		testOrgID, map[string]string{"projectID": testProjID}, strings.NewReader("not-json"))
	w := httptest.NewRecorder()
	h.CreateInvoice(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestUpdateInvoice_NotFoundReturns404(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{
		updateInvoiceErr: service.ErrNotFound,
	})
	body := strings.NewReader(`{"status":"paid"}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/invoices/"+testInvID,
		testOrgID, map[string]string{"projectID": testProjID, "invoiceID": testInvID}, body)
	w := httptest.NewRecorder()
	h.UpdateInvoice(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
}

func TestUpdateInvoice_BadStatusMaps400(t *testing.T) {
	// The service is the validator; the mock returns ErrInvalidInput to
	// simulate that pathway. Real validation lives in BudgetService.UpdateInvoice.
	h := NewFinancialsHandler(&mockBudgetService{
		updateInvoiceErr: service.ErrInvalidInput,
	})
	body := strings.NewReader(`{"status":"bogus"}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/invoices/"+testInvID,
		testOrgID, map[string]string{"projectID": testProjID, "invoiceID": testInvID}, body)
	w := httptest.NewRecorder()
	h.UpdateInvoice(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400; body=%s", w.Code, w.Body.String())
	}
}

// ---------- /org/{orgID}/financials/ar-aging ----------

func TestARAging_OK(t *testing.T) {
	svc := &mockBudgetService{getARAgingResult: []models.ARAgingSnapshot{{CurrencyCode: "USD"}}}
	h := NewFinancialsHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/financials/ar-aging?currency=USD",
		testOrgID, map[string]string{"orgID": testOrgID}, nil)
	w := httptest.NewRecorder()
	h.ARAging(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCallerOrgID.String() != testOrgID || svc.lastCurrencyCode != "USD" {
		t.Errorf("service got org=%s currency=%q", svc.lastCallerOrgID, svc.lastCurrencyCode)
	}
	if !strings.Contains(w.Body.String(), `"snapshots"`) {
		t.Errorf("body should wrap snapshots: %s", w.Body.String())
	}
}

func TestARAging_OrgMismatch403(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{})
	r := buildRequest(t, "GET", "/api/v1/org/"+otherOrgID+"/financials/ar-aging",
		testOrgID, map[string]string{"orgID": otherOrgID}, nil)
	w := httptest.NewRecorder()
	h.ARAging(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403", w.Code)
	}
}

func TestARAging_ServiceErr500(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{getARAgingErr: errInternal()})
	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/financials/ar-aging",
		testOrgID, map[string]string{"orgID": testOrgID}, nil)
	w := httptest.NewRecorder()
	h.ARAging(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
}

// ---------- /org/{orgID}/financials/projects ----------

func TestProjectFinancials_OK(t *testing.T) {
	svc := &mockBudgetService{getProjectFinancialsResult: []models.ProjectFinancial{{CurrencyCode: "CAD"}}}
	h := NewFinancialsHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/financials/projects?currency=CAD",
		testOrgID, map[string]string{"orgID": testOrgID}, nil)
	w := httptest.NewRecorder()
	h.ProjectFinancials(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastCallerOrgID.String() != testOrgID || svc.lastCurrencyCode != "CAD" {
		t.Errorf("service got org=%s currency=%q", svc.lastCallerOrgID, svc.lastCurrencyCode)
	}
	if !strings.Contains(w.Body.String(), `"projects"`) {
		t.Errorf("body should wrap projects: %s", w.Body.String())
	}
}

func TestProjectFinancials_ServiceInvalidInput400(t *testing.T) {
	h := NewFinancialsHandler(&mockBudgetService{getProjectFinancialsErr: service.ErrInvalidInput})
	r := buildRequest(t, "GET", "/api/v1/org/"+testOrgID+"/financials/projects?currency=EUR",
		testOrgID, map[string]string{"orgID": testOrgID}, nil)
	w := httptest.NewRecorder()
	h.ProjectFinancials(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}
