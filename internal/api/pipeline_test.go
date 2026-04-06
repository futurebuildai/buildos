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
// PipelineHandler — ListProspects
// ---------------------------------------------------------------------------

func TestPipelineHandler_ListProspects_InvalidOrgID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/bad/pipeline/prospects", nil)

	ctx := withChiParam(req.Context(), "orgID", "bad")
	req = req.WithContext(ctx)

	h.ListProspects(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ORG_ID")
}

func TestPipelineHandler_ListProspects_EmptyOrgID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org//pipeline/prospects", nil)

	ctx := withChiParam(req.Context(), "orgID", "")
	req = req.WithContext(ctx)

	h.ListProspects(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ORG_ID")
}

func TestPipelineHandler_ListProspects_ValidOrgID_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/"+orgID+"/pipeline/prospects", nil)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.ListProspects(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid orgID should pass validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — CreateProspect
// ---------------------------------------------------------------------------

func TestPipelineHandler_CreateProspect_InvalidOrgID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"name":"Test","client_name":"Client"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/invalid/pipeline/prospects", body)

	ctx := withChiParam(req.Context(), "orgID", "invalid")
	req = req.WithContext(ctx)

	h.CreateProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ORG_ID")
}

func TestPipelineHandler_CreateProspect_InvalidJSON(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	body := bytes.NewBufferString(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/"+orgID+"/pipeline/prospects", body)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	h.CreateProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestPipelineHandler_CreateProspect_EmptyBody(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	body := bytes.NewBufferString(``)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/"+orgID+"/pipeline/prospects", body)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	h.CreateProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestPipelineHandler_CreateProspect_ValidPayload_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	body := bytes.NewBufferString(`{"name":"New Home Build","client_name":"John Doe","client_email":"john@example.com"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/"+orgID+"/pipeline/prospects", body)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.CreateProspect(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid prospect payload should pass handler validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — GetProspect
// ---------------------------------------------------------------------------

func TestPipelineHandler_GetProspect_InvalidProspectID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/xxx/pipeline/prospects/bad-id", nil)

	ctx := withChiParam(req.Context(), "prospectID", "bad-id")
	req = req.WithContext(ctx)

	h.GetProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROSPECT_ID")
}

func TestPipelineHandler_GetProspect_EmptyProspectID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/xxx/pipeline/prospects/", nil)

	ctx := withChiParam(req.Context(), "prospectID", "")
	req = req.WithContext(ctx)

	h.GetProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROSPECT_ID")
}

func TestPipelineHandler_GetProspect_ValidID_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/xxx/pipeline/prospects/"+prospectID, nil)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.GetProspect(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid prospectID should pass validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — UpdateProspect
// ---------------------------------------------------------------------------

func TestPipelineHandler_UpdateProspect_InvalidProspectID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"name":"Updated","client_name":"Client"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/prospects/bad", body)

	ctx := withChiParam(req.Context(), "prospectID", "bad")
	req = req.WithContext(ctx)

	h.UpdateProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROSPECT_ID")
}

func TestPipelineHandler_UpdateProspect_InvalidJSON(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	body := bytes.NewBufferString(`{broken}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/prospects/"+prospectID, body)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	req = req.WithContext(ctx)

	h.UpdateProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestPipelineHandler_UpdateProspect_ValidPayload_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	body := bytes.NewBufferString(`{"name":"Updated Build","client_name":"Jane Doe"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/prospects/"+prospectID, body)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.UpdateProspect(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid update payload should pass handler validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — AdvanceProspect
// ---------------------------------------------------------------------------

func TestPipelineHandler_AdvanceProspect_InvalidProspectID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/bad/advance", nil)

	ctx := withChiParam(req.Context(), "prospectID", "bad")
	req = req.WithContext(ctx)

	h.AdvanceProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROSPECT_ID")
}

func TestPipelineHandler_AdvanceProspect_ValidID_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/"+prospectID+"/advance", nil)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.AdvanceProspect(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid prospectID should pass validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — LoseProspect
// ---------------------------------------------------------------------------

func TestPipelineHandler_LoseProspect_InvalidProspectID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"reason":"Budget constraints"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/bad/lose", body)

	ctx := withChiParam(req.Context(), "prospectID", "bad")
	req = req.WithContext(ctx)

	h.LoseProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROSPECT_ID")
}

func TestPipelineHandler_LoseProspect_InvalidJSON(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	body := bytes.NewBufferString(`{broken}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/"+prospectID+"/lose", body)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	req = req.WithContext(ctx)

	h.LoseProspect(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestPipelineHandler_LoseProspect_ValidPayload_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	body := bytes.NewBufferString(`{"reason":"Client went with competitor"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/"+prospectID+"/lose", body)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.LoseProspect(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid lose payload should pass handler validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — CreateEstimate
// ---------------------------------------------------------------------------

func TestPipelineHandler_CreateEstimate_InvalidProspectID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"total_estimated_cents":500000,"currency_code":"USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/bad/estimates", body)

	ctx := withChiParam(req.Context(), "prospectID", "bad")
	req = req.WithContext(ctx)

	h.CreateEstimate(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROSPECT_ID")
}

func TestPipelineHandler_CreateEstimate_InvalidJSON(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	body := bytes.NewBufferString(`{broken}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/"+prospectID+"/estimates", body)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	req = req.WithContext(ctx)

	h.CreateEstimate(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestPipelineHandler_CreateEstimate_ValidPayload_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	body := bytes.NewBufferString(`{"total_estimated_cents":500000,"currency_code":"USD","margin_pct":15}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/"+prospectID+"/estimates", body)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.CreateEstimate(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid estimate payload should pass handler validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — UpdateEstimate
// ---------------------------------------------------------------------------

func TestPipelineHandler_UpdateEstimate_InvalidEstimateID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"total_estimated_cents":600000,"currency_code":"USD","margin_pct":18,"status":"draft"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/estimates/bad-id", body)

	ctx := withChiParam(req.Context(), "estimateID", "bad-id")
	req = req.WithContext(ctx)

	h.UpdateEstimate(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ESTIMATE_ID")
}

func TestPipelineHandler_UpdateEstimate_InvalidJSON(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	estimateID := uuid.New().String()
	body := bytes.NewBufferString(`{broken}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/estimates/"+estimateID, body)

	ctx := withChiParam(req.Context(), "estimateID", estimateID)
	req = req.WithContext(ctx)

	h.UpdateEstimate(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestPipelineHandler_UpdateEstimate_ValidPayload_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	estimateID := uuid.New().String()
	body := bytes.NewBufferString(`{"total_estimated_cents":600000,"currency_code":"USD","margin_pct":18,"status":"draft"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/estimates/"+estimateID, body)

	ctx := withChiParam(req.Context(), "estimateID", estimateID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.UpdateEstimate(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid estimate update should pass handler validation, got 400: %s", rr.Body.String())
	}
}

func TestPipelineHandler_UpdateEstimate_SentStatus_SetsSentAt(t *testing.T) {
	// When status is "sent", handler sets sentAt before calling service.
	// Verify the payload passes validation with sent status.
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	estimateID := uuid.New().String()
	body := bytes.NewBufferString(`{"total_estimated_cents":600000,"currency_code":"USD","margin_pct":18,"status":"sent"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/estimates/"+estimateID, body)

	ctx := withChiParam(req.Context(), "estimateID", estimateID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.UpdateEstimate(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("sent status should pass handler validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — CreatePermit
// ---------------------------------------------------------------------------

func TestPipelineHandler_CreatePermit_InvalidProspectID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"permit_type":"Building","jurisdiction":"County","fee_cents":50000,"fee_currency_code":"USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/bad/permits", body)

	ctx := withChiParam(req.Context(), "prospectID", "bad")
	req = req.WithContext(ctx)

	h.CreatePermit(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PROSPECT_ID")
}

func TestPipelineHandler_CreatePermit_MissingClaims(t *testing.T) {
	// MustClaimsFromContext will panic without claims
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	body := bytes.NewBufferString(`{"permit_type":"Building","jurisdiction":"County","fee_cents":50000,"fee_currency_code":"USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/"+prospectID+"/permits", body)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	req = req.WithContext(ctx)

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		h.CreatePermit(rr, req)
	}()

	if !panicked {
		t.Error("expected panic from MustClaimsFromContext when no claims in context")
	}
}

func TestPipelineHandler_CreatePermit_InvalidJSON(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	body := bytes.NewBufferString(`{broken}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/"+prospectID+"/permits", body)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	ctx = mw.ContextWithClaims(ctx, validClaims())
	req = req.WithContext(ctx)

	h.CreatePermit(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestPipelineHandler_CreatePermit_ValidPayload_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	prospectID := uuid.New().String()
	body := bytes.NewBufferString(`{
		"permit_type":"Building",
		"jurisdiction":"City of Portland",
		"fee_cents":50000,
		"fee_currency_code":"USD",
		"submitted_date":"2026-03-15",
		"expected_issue_date":"2026-04-15"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/org/xxx/pipeline/prospects/"+prospectID+"/permits", body)

	ctx := withChiParam(req.Context(), "prospectID", prospectID)
	ctx = mw.ContextWithClaims(ctx, validClaims())
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.CreatePermit(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid permit payload should pass handler validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — UpdatePermit
// ---------------------------------------------------------------------------

func TestPipelineHandler_UpdatePermit_InvalidPermitID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"permit_type":"Building","jurisdiction":"County","fee_cents":50000,"fee_currency_code":"USD","status":"submitted"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/permits/bad-id", body)

	ctx := withChiParam(req.Context(), "permitID", "bad-id")
	req = req.WithContext(ctx)

	h.UpdatePermit(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_PERMIT_ID")
}

func TestPipelineHandler_UpdatePermit_InvalidJSON(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	permitID := uuid.New().String()
	body := bytes.NewBufferString(`{broken}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/permits/"+permitID, body)

	ctx := withChiParam(req.Context(), "permitID", permitID)
	req = req.WithContext(ctx)

	h.UpdatePermit(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestPipelineHandler_UpdatePermit_ValidPayload_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	permitID := uuid.New().String()
	body := bytes.NewBufferString(`{
		"permit_type":"Building",
		"jurisdiction":"County",
		"fee_cents":75000,
		"fee_currency_code":"CAD",
		"status":"submitted",
		"submitted_date":"2026-03-01",
		"expected_issue_date":"2026-04-01",
		"actual_issue_date":"2026-04-05"
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/org/xxx/pipeline/permits/"+permitID, body)

	ctx := withChiParam(req.Context(), "permitID", permitID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.UpdatePermit(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid permit update should pass handler validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// PipelineHandler — Analytics
// ---------------------------------------------------------------------------

func TestPipelineHandler_Analytics_InvalidOrgID(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/bad/pipeline/analytics", nil)

	ctx := withChiParam(req.Context(), "orgID", "bad")
	req = req.WithContext(ctx)

	h.Analytics(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ORG_ID")
}

func TestPipelineHandler_Analytics_ValidOrgID_PassesValidation(t *testing.T) {
	h := NewPipelineHandler(nil)
	rr := httptest.NewRecorder()
	orgID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/org/"+orgID+"/pipeline/analytics", nil)

	ctx := withChiParam(req.Context(), "orgID", orgID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.Analytics(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid orgID should pass validation for analytics, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Request body parsing tests — BIGINT cents validation
// ---------------------------------------------------------------------------

func TestCreateEstimateRequest_BIGINTCents(t *testing.T) {
	raw := `{"total_estimated_cents":99999999999,"currency_code":"USD","margin_pct":15}`
	var req createEstimateRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.TotalEstimatedCents != 99999999999 {
		t.Errorf("expected total_estimated_cents=99999999999, got %d", req.TotalEstimatedCents)
	}
	if req.CurrencyCode != "USD" {
		t.Errorf("expected currency_code=USD, got %s", req.CurrencyCode)
	}
}

func TestCreatePermitRequest_BIGINTFeeCents(t *testing.T) {
	raw := `{"permit_type":"Building","jurisdiction":"County","fee_cents":12345678,"fee_currency_code":"CAD"}`
	var req createPermitRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.FeeCents != 12345678 {
		t.Errorf("expected fee_cents=12345678, got %d", req.FeeCents)
	}
	if req.FeeCurrencyCode != "CAD" {
		t.Errorf("expected fee_currency_code=CAD, got %s", req.FeeCurrencyCode)
	}
}

func TestUpdateEstimateRequest_CurrencyCodePresent(t *testing.T) {
	raw := `{"total_estimated_cents":100000,"currency_code":"CAD","margin_pct":20,"status":"draft"}`
	var req updateEstimateRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.CurrencyCode == "" {
		t.Error("currency_code must not be empty")
	}
	if req.CurrencyCode != "CAD" {
		t.Errorf("expected CAD, got %s", req.CurrencyCode)
	}
}

func TestCreateProspectRequest_Parsing(t *testing.T) {
	email := "john@example.com"
	phone := "555-1234"
	raw := `{"name":"Test Build","client_name":"John","client_email":"john@example.com","client_phone":"555-1234","gsf":2500}`
	var req createProspectRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.Name != "Test Build" {
		t.Errorf("expected name=Test Build, got %s", req.Name)
	}
	if req.ClientName != "John" {
		t.Errorf("expected client_name=John, got %s", req.ClientName)
	}
	if req.ClientEmail == nil || *req.ClientEmail != email {
		t.Errorf("expected client_email=%s, got %v", email, req.ClientEmail)
	}
	if req.ClientPhone == nil || *req.ClientPhone != phone {
		t.Errorf("expected client_phone=%s, got %v", phone, req.ClientPhone)
	}
	if req.GSF == nil || *req.GSF != 2500 {
		t.Errorf("expected gsf=2500, got %v", req.GSF)
	}
}

func TestLoseProspectRequest_Parsing(t *testing.T) {
	raw := `{"reason":"Budget cuts"}`
	var req loseProspectRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.Reason != "Budget cuts" {
		t.Errorf("expected reason=Budget cuts, got %s", req.Reason)
	}
}

func TestUpdatePermitRequest_AllDateFields(t *testing.T) {
	raw := `{
		"permit_type":"Building",
		"jurisdiction":"County",
		"submitted_date":"2026-03-01",
		"expected_issue_date":"2026-04-01",
		"actual_issue_date":"2026-04-05",
		"fee_cents":10000,
		"fee_currency_code":"USD",
		"status":"approved"
	}`
	var req updatePermitRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if req.SubmittedDate == nil || *req.SubmittedDate != "2026-03-01" {
		t.Errorf("expected submitted_date=2026-03-01, got %v", req.SubmittedDate)
	}
	if req.ExpectedIssueDate == nil || *req.ExpectedIssueDate != "2026-04-01" {
		t.Errorf("expected expected_issue_date=2026-04-01, got %v", req.ExpectedIssueDate)
	}
	if req.ActualIssueDate == nil || *req.ActualIssueDate != "2026-04-05" {
		t.Errorf("expected actual_issue_date=2026-04-05, got %v", req.ActualIssueDate)
	}
}
