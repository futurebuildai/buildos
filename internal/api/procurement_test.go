package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// ---------------------------------------------------------------------------
// ProcurementHandler — List
// ---------------------------------------------------------------------------

func TestProcurementHandler_List_InvalidProjectID(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/bad-id/procurement", nil)

	ctx := withChiParam(req.Context(), "projectID", "bad-id")
	req = req.WithContext(ctx)

	h.List(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ID")
}

func TestProcurementHandler_List_EmptyProjectID(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects//procurement", nil)

	ctx := withChiParam(req.Context(), "projectID", "")
	req = req.WithContext(ctx)

	h.List(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ID")
}

func TestProcurementHandler_List_ValidProjectID_PassesValidation(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+projectID+"/procurement", nil)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.List(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid projectID should pass validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ProcurementHandler — Create
// ---------------------------------------------------------------------------

func TestProcurementHandler_Create_InvalidProjectID(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"description":"Lumber","estimated_cost_cents":50000,"estimated_cost_currency_code":"USD"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/bad/procurement", body)

	ctx := withChiParam(req.Context(), "projectID", "bad")
	req = req.WithContext(ctx)

	h.Create(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ID")
}

func TestProcurementHandler_Create_InvalidJSON(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{broken}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/procurement", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	h.Create(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestProcurementHandler_Create_EmptyBody(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(``)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/procurement", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	h.Create(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestProcurementHandler_Create_ValidPayload_PassesValidation(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{
		"description":"2x4 Lumber",
		"estimated_cost_cents":150000,
		"estimated_cost_currency_code":"USD",
		"supplier_name":"Lumber Depot"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/procurement", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.Create(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid procurement create payload should pass handler validation, got 400: %s", rr.Body.String())
	}
}

func TestProcurementHandler_Create_InvalidMustOrderDate(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{
		"description":"Lumber",
		"estimated_cost_cents":50000,
		"estimated_cost_currency_code":"USD",
		"must_order_date":"not-a-date"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/procurement", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	h.Create(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_DATE")
}

func TestProcurementHandler_Create_InvalidExpectedDeliveryDate(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{
		"description":"Lumber",
		"estimated_cost_cents":50000,
		"estimated_cost_currency_code":"USD",
		"expected_delivery_date":"not-a-date"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/procurement", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	h.Create(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_DATE")
}

func TestProcurementHandler_Create_ValidRFC3339Dates(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{
		"description":"Lumber",
		"estimated_cost_cents":50000,
		"estimated_cost_currency_code":"USD",
		"must_order_date":"2026-05-01T00:00:00Z",
		"expected_delivery_date":"2026-05-15T00:00:00Z"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/procurement", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.Create(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid RFC3339 dates should pass validation, got 400: %s", rr.Body.String())
	}
}

func TestProcurementHandler_Create_ValidISO8601Dates(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	projectID := uuid.New().String()
	body := bytes.NewBufferString(`{
		"description":"Lumber",
		"estimated_cost_cents":50000,
		"estimated_cost_currency_code":"USD",
		"must_order_date":"2026-05-01",
		"expected_delivery_date":"2026-05-15"
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/procurement", body)

	ctx := withChiParam(req.Context(), "projectID", projectID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.Create(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid ISO 8601 dates should pass validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ProcurementHandler — Update
// ---------------------------------------------------------------------------

func TestProcurementHandler_Update_InvalidItemID(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"status":"DELIVERED"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/procurement/bad-id", body)

	ctx := withChiParam(req.Context(), "itemID", "bad-id")
	req = req.WithContext(ctx)

	h.Update(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ID")
}

func TestProcurementHandler_Update_InvalidJSON(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	itemID := uuid.New().String()
	body := bytes.NewBufferString(`{broken}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/procurement/"+itemID, body)

	ctx := withChiParam(req.Context(), "itemID", itemID)
	req = req.WithContext(ctx)

	h.Update(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestProcurementHandler_Update_ValidStatus_PassesValidation(t *testing.T) {
	validStatuses := []string{"PENDING", "WARNING", "CRITICAL", "DELIVERED", "CANCELLED"}
	for _, status := range validStatuses {
		t.Run(status, func(t *testing.T) {
			h := NewProcurementHandler(nil)
			rr := httptest.NewRecorder()
			itemID := uuid.New().String()
			body := bytes.NewBufferString(`{"status":"` + status + `"}`)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/procurement/"+itemID, body)

			ctx := withChiParam(req.Context(), "itemID", itemID)
			req = req.WithContext(ctx)

			func() {
				defer func() {
					if r := recover(); r != nil {
						// Expected: nil service
					}
				}()
				h.Update(rr, req)
			}()

			if rr.Code == http.StatusBadRequest {
				t.Errorf("valid status %s should pass handler validation, got 400: %s", status, rr.Body.String())
			}
		})
	}
}

func TestProcurementHandler_Update_WithSupplierInfo(t *testing.T) {
	h := NewProcurementHandler(nil)
	rr := httptest.NewRecorder()
	itemID := uuid.New().String()
	body := bytes.NewBufferString(`{"status":"DELIVERED","supplier_name":"Acme Corp","supplier_contact":"john@acme.com"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/projects/xxx/procurement/"+itemID, body)

	ctx := withChiParam(req.Context(), "itemID", itemID)
	req = req.WithContext(ctx)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil service
			}
		}()
		h.Update(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid update with supplier info should pass handler validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Request body parsing tests — procurement BIGINT cents validation
// ---------------------------------------------------------------------------

func TestProcurementCreateBody_BIGINTCents(t *testing.T) {
	type createBody struct {
		Description               string `json:"description"`
		EstimatedCostCents        int64  `json:"estimated_cost_cents"`
		EstimatedCostCurrencyCode string `json:"estimated_cost_currency_code"`
	}

	raw := `{"description":"Heavy Equipment","estimated_cost_cents":88888888888,"estimated_cost_currency_code":"CAD"}`
	var body createBody
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if body.EstimatedCostCents != 88888888888 {
		t.Errorf("expected estimated_cost_cents=88888888888, got %d", body.EstimatedCostCents)
	}
	if body.EstimatedCostCurrencyCode != "CAD" {
		t.Errorf("expected estimated_cost_currency_code=CAD, got %s", body.EstimatedCostCurrencyCode)
	}
}

func TestProcurementCostSummary_CurrencyCodePresent(t *testing.T) {
	summary := models.ProcurementCostSummary{
		CurrencyCode: "USD",
		TotalCents:   100000,
		ItemCount:    5,
	}
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := parsed["currency_code"]; !ok {
		t.Error("currency_code field missing from serialized ProcurementCostSummary")
	}
	if _, ok := parsed["total_cents"]; !ok {
		t.Error("total_cents field missing from serialized ProcurementCostSummary")
	}
}

func TestProcurementItem_CurrencyCodeInJSON(t *testing.T) {
	item := models.ProcurementItem{
		ID:                        uuid.New(),
		OrgID:                     uuid.New(),
		ProjectID:                 uuid.New(),
		Description:               "Test Item",
		EstimatedCostCents:        50000,
		EstimatedCostCurrencyCode: "USD",
		Status:                    models.ProcurementPending,
	}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := parsed["estimated_cost_currency_code"]; !ok {
		t.Error("estimated_cost_currency_code field missing from serialized ProcurementItem")
	}
	if _, ok := parsed["estimated_cost_cents"]; !ok {
		t.Error("estimated_cost_cents field missing from serialized ProcurementItem")
	}

	// Verify no float in cents value
	var centsValue json.Number
	if err := json.Unmarshal(parsed["estimated_cost_cents"], &centsValue); err != nil {
		t.Fatalf("failed to parse cents value: %v", err)
	}
	centsInt, err := centsValue.Int64()
	if err != nil {
		t.Errorf("estimated_cost_cents should be parseable as int64, got error: %v", err)
	}
	if centsInt != 50000 {
		t.Errorf("expected 50000 cents, got %d", centsInt)
	}
}
