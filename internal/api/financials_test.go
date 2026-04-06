package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	mw "github.com/futurebuild/futurebuild-os/internal/api/middleware"
)

// ---------------------------------------------------------------------------
// FinancialsHandler — Summary
// ---------------------------------------------------------------------------

func TestFinancialsHandler_Summary_InvalidOrgID(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/not-a-uuid/financials/summary", nil)

	ctx := withChiParam(req.Context(), "orgID", "not-a-uuid")
	req = req.WithContext(ctx)

	h.Summary(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ORG_ID")
}

func TestFinancialsHandler_Summary_EmptyOrgID(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org//financials/summary", nil)

	ctx := withChiParam(req.Context(), "orgID", "")
	req = req.WithContext(ctx)

	h.Summary(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ORG_ID")
}

func TestFinancialsHandler_Summary_ValidOrgID_PassesValidation(t *testing.T) {
	// With nil services, a valid orgID should pass validation and then panic
	// when trying to call the service. Recovery confirms we passed validation.
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/"+orgID+"/financials/summary", nil)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service panic means we passed validation
			}
		}()
		h.Summary(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid orgID should pass validation, got 400: %s", rr.Body.String())
	}
}

func TestFinancialsHandler_Summary_DefaultCurrencyUSD(t *testing.T) {
	// Verify that when no currency query param is set, the handler defaults to USD
	// and passes validation (panics on nil service after validation succeeds)
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/"+orgID+"/financials/summary", nil)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected
			}
		}()
		h.Summary(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("expected no validation error for default currency, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// FinancialsHandler — ARAging
// ---------------------------------------------------------------------------

func TestFinancialsHandler_ARAging_InvalidOrgID(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/bad/financials/ar-aging", nil)

	ctx := withChiParam(req.Context(), "orgID", "bad")
	req = req.WithContext(ctx)

	h.ARAging(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ORG_ID")
}

func TestFinancialsHandler_ARAging_UnsupportedCurrency(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/"+orgID+"/financials/ar-aging?currency=EUR", nil)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	h.ARAging(rr, req)

	assertStatus(t, rr, http.StatusUnprocessableEntity)
	assertErrorCode(t, rr, "INVALID_CURRENCY")
}

func TestFinancialsHandler_ARAging_SupportedCurrencyUSD(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	// No currency param defaults to USD, which should pass validation
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/"+orgID+"/financials/ar-aging", nil)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.ARAging(rr, req)
	}()

	if rr.Code == http.StatusBadRequest || rr.Code == http.StatusUnprocessableEntity {
		t.Errorf("USD currency should pass validation, got status %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFinancialsHandler_ARAging_SupportedCurrencyCAD(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/"+orgID+"/financials/ar-aging?currency=CAD", nil)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.ARAging(rr, req)
	}()

	if rr.Code == http.StatusUnprocessableEntity {
		t.Errorf("CAD currency should pass validation, got 422: %s", rr.Body.String())
	}
}

func TestFinancialsHandler_ARAging_UnsupportedCurrencyGBP(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/"+orgID+"/financials/ar-aging?currency=GBP", nil)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	h.ARAging(rr, req)

	assertStatus(t, rr, http.StatusUnprocessableEntity)
	assertErrorCode(t, rr, "INVALID_CURRENCY")
}

// ---------------------------------------------------------------------------
// FinancialsHandler — ProjectFinancials
// ---------------------------------------------------------------------------

func TestFinancialsHandler_ProjectFinancials_InvalidOrgID(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/invalid/financials/projects", nil)

	ctx := withChiParam(req.Context(), "orgID", "invalid")
	req = req.WithContext(ctx)

	h.ProjectFinancials(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ORG_ID")
}

func TestFinancialsHandler_ProjectFinancials_ValidOrgID_PassesValidation(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/"+orgID+"/financials/projects", nil)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected
			}
		}()
		h.ProjectFinancials(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid orgID should pass validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// FinancialsHandler — ListBudgets
// ---------------------------------------------------------------------------

func TestFinancialsHandler_ListBudgets_InvalidProjectID(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/bad-id/budgets", nil)

	ctx := withChiParam(req.Context(), "projectID", "bad-id")
	req = req.WithContext(ctx)

	h.ListBudgets(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROJECT_ID")
}

func TestFinancialsHandler_ListBudgets_EmptyProjectID(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects//budgets", nil)

	ctx := withChiParam(req.Context(), "projectID", "")
	req = req.WithContext(ctx)

	h.ListBudgets(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROJECT_ID")
}

func TestFinancialsHandler_ListBudgets_ValidProjectID_PassesValidation(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/budgets", nil)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.ListBudgets(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid projectID should pass validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// FinancialsHandler — CreateInvoice
// ---------------------------------------------------------------------------

func TestFinancialsHandler_CreateInvoice_InvalidProjectID(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"vendor_name":"Test","amount_cents":50000,"currency_code":"USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/bad/invoices", body)

	ctx := withChiParam(req.Context(), "projectID", "bad")
	req = req.WithContext(ctx)

	h.CreateInvoice(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROJECT_ID")
}

func TestFinancialsHandler_CreateInvoice_MissingClaims(t *testing.T) {
	// Valid projectID but no claims in context -- MustClaimsFromContext will panic
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{"vendor_name":"Test","amount_cents":50000,"currency_code":"USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/invoices", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		h.CreateInvoice(rr, req)
	}()

	if !panicked {
		t.Error("expected panic from MustClaimsFromContext when no claims in context")
	}
}

func TestFinancialsHandler_CreateInvoice_InvalidOrgIDInClaims(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{"vendor_name":"Test","amount_cents":50000,"currency_code":"USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/invoices", body)

	claims := mw.Claims{
		Sub:   uuid.New().String(),
		OrgID: "not-a-uuid",
		Role:  "owner",
	}
	ctx := withChiParam(req.Context(), "projectID", projectID)
	ctx = mw.ContextWithClaims(ctx, claims)
	req = req.WithContext(ctx)

	h.CreateInvoice(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ORG_ID")
}

func TestFinancialsHandler_CreateInvoice_InvalidJSONBody(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{not valid json}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/invoices", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	ctx = mw.ContextWithClaims(ctx, validClaims())
	req = req.WithContext(ctx)

	h.CreateInvoice(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestFinancialsHandler_CreateInvoice_ValidPayload_PassesValidation(t *testing.T) {
	// Valid payload should pass handler validation and then panic on nil service
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{"vendor_name":"Test Vendor","amount_cents":50000,"currency_code":"USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/invoices", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	ctx = mw.ContextWithClaims(ctx, validClaims())
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil budgetSvc
			}
		}()
		h.CreateInvoice(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid invoice payload should pass handler validation, got 400: %s", rr.Body.String())
	}
}

func TestFinancialsHandler_CreateInvoice_EmptyBody(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(``)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/invoices", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	ctx = mw.ContextWithClaims(ctx, validClaims())
	req = req.WithContext(ctx)

	h.CreateInvoice(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

// ---------------------------------------------------------------------------
// FinancialsHandler — UpdateInvoice
// ---------------------------------------------------------------------------

func TestFinancialsHandler_UpdateInvoice_InvalidInvoiceID(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"action":"approve"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/invoices/bad-id", body)

	ctx := withChiParam(req.Context(), "invoiceID", "bad-id")
	req = req.WithContext(ctx)

	h.UpdateInvoice(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_INVOICE_ID")
}

func TestFinancialsHandler_UpdateInvoice_InvalidJSON(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	invoiceID := uuid.New().String()
	body := bytes.NewBufferString(`{invalid`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/invoices/"+invoiceID, body)

	ctx := withChiParam(req.Context(), "invoiceID", invoiceID)
	req = req.WithContext(ctx)

	h.UpdateInvoice(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestFinancialsHandler_UpdateInvoice_InvalidAction(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	invoiceID := uuid.New().String()
	body := bytes.NewBufferString(`{"action":"invalid_action"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/invoices/"+invoiceID, body)

	ctx := withChiParam(req.Context(), "invoiceID", invoiceID)
	req = req.WithContext(ctx)

	h.UpdateInvoice(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ACTION")
}

func TestFinancialsHandler_UpdateInvoice_ApproveAction_PassesValidation(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	invoiceID := uuid.New().String()
	body := bytes.NewBufferString(`{"action":"approve"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/invoices/"+invoiceID, body)

	ctx := withChiParam(req.Context(), "invoiceID", invoiceID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.UpdateInvoice(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid approve action should pass validation, got 400: %s", rr.Body.String())
	}
}

func TestFinancialsHandler_UpdateInvoice_RejectAction_PassesValidation(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	invoiceID := uuid.New().String()
	body := bytes.NewBufferString(`{"action":"reject"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/invoices/"+invoiceID, body)

	ctx := withChiParam(req.Context(), "invoiceID", invoiceID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.UpdateInvoice(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid reject action should pass validation, got 400: %s", rr.Body.String())
	}
}

func TestFinancialsHandler_UpdateInvoice_PayAction_PassesValidation(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	invoiceID := uuid.New().String()
	body := bytes.NewBufferString(`{"action":"pay"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/invoices/"+invoiceID, body)

	ctx := withChiParam(req.Context(), "invoiceID", invoiceID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.UpdateInvoice(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid pay action should pass validation, got 400: %s", rr.Body.String())
	}
}

func TestFinancialsHandler_UpdateInvoice_EmptyAction(t *testing.T) {
	h := NewFinancialsHandler(nil, nil)
	rr := httptest.NewRecorder()
	invoiceID := uuid.New().String()
	body := bytes.NewBufferString(`{"action":""}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/invoices/"+invoiceID, body)

	ctx := withChiParam(req.Context(), "invoiceID", invoiceID)
	req = req.WithContext(ctx)

	h.UpdateInvoice(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ACTION")
}

// ---------------------------------------------------------------------------
// FinancialsHandler — BIGINT cents validation
// ---------------------------------------------------------------------------

func TestCreateInvoiceRequest_BIGINTCents(t *testing.T) {
	// Verify that amount_cents is int64 (BIGINT) and does not use float
	raw := `{"vendor_name":"V","amount_cents":99999999999,"currency_code":"USD"}`
	var req createInvoiceRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.AmountCents != 99999999999 {
		t.Errorf("expected amount_cents=99999999999, got %d", req.AmountCents)
	}
	if req.CurrencyCode != "USD" {
		t.Errorf("expected currency_code=USD, got %s", req.CurrencyCode)
	}
}

func TestCreateInvoiceRequest_CurrencyCodePresent(t *testing.T) {
	raw := `{"vendor_name":"V","amount_cents":1000,"currency_code":"CAD"}`
	var req createInvoiceRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.CurrencyCode == "" {
		t.Error("currency_code must not be empty in parsed request")
	}
	if req.CurrencyCode != "CAD" {
		t.Errorf("expected currency_code=CAD, got %s", req.CurrencyCode)
	}
}

func TestCreateInvoiceRequest_ZeroCents(t *testing.T) {
	raw := `{"vendor_name":"V","amount_cents":0,"currency_code":"USD"}`
	var req createInvoiceRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.AmountCents != 0 {
		t.Errorf("expected amount_cents=0, got %d", req.AmountCents)
	}
}

func TestCreateInvoiceRequest_NegativeCents(t *testing.T) {
	raw := `{"vendor_name":"V","amount_cents":-500,"currency_code":"USD"}`
	var req createInvoiceRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.AmountCents != -500 {
		t.Errorf("expected amount_cents=-500, got %d", req.AmountCents)
	}
}

func TestUpdateInvoiceRequest_ActionParsing(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		expected string
	}{
		{"approve", `{"action":"approve"}`, "approve"},
		{"reject", `{"action":"reject"}`, "reject"},
		{"pay", `{"action":"pay"}`, "pay"},
		{"empty", `{"action":""}`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req updateInvoiceRequest
			if err := json.Unmarshal([]byte(tc.raw), &req); err != nil {
				t.Fatalf("failed to parse: %v", err)
			}
			if req.Action != tc.expected {
				t.Errorf("expected action=%q, got %q", tc.expected, req.Action)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FinancialsHandler — helper functions
// ---------------------------------------------------------------------------

func TestCurrencyFromQuery_Default(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	cc := currencyFromQuery(req)
	if cc != "USD" {
		t.Errorf("expected default currency USD, got %s", cc)
	}
}

func TestCurrencyFromQuery_Explicit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?currency=CAD", nil)
	cc := currencyFromQuery(req)
	if cc != "CAD" {
		t.Errorf("expected CAD, got %s", cc)
	}
}

func TestCurrencyFromQuery_ExplicitUSD(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?currency=USD", nil)
	cc := currencyFromQuery(req)
	if cc != "USD" {
		t.Errorf("expected USD, got %s", cc)
	}
}

func TestParseOrgID_Valid(t *testing.T) {
	id := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := withChiParam(req.Context(), "orgID", id.String())
	req = req.WithContext(ctx)

	parsed, err := parseOrgID(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed != id {
		t.Errorf("expected %s, got %s", id, parsed)
	}
}

func TestParseOrgID_Invalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := withChiParam(req.Context(), "orgID", "not-valid")
	req = req.WithContext(ctx)

	_, err := parseOrgID(req)
	if err == nil {
		t.Error("expected error for invalid org ID")
	}
}

func TestParseProjectID_Valid(t *testing.T) {
	id := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := withChiParam(req.Context(), "projectID", id.String())
	req = req.WithContext(ctx)

	parsed, err := parseProjectID(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed != id {
		t.Errorf("expected %s, got %s", id, parsed)
	}
}

func TestParseProjectID_Invalid(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	ctx := withChiParam(req.Context(), "projectID", "bad")
	req = req.WithContext(ctx)

	_, err := parseProjectID(req)
	if err == nil {
		t.Error("expected error for invalid project ID")
	}
}
