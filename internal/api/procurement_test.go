package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/service"
)

const testItemID = "55555555-5555-5555-5555-555555555555"

// mockProcurementService implements ProcurementServicer for handler
// tests. List/Create/Update aren't exercised by the RequestVendorReview
// suite below — they're stubbed to satisfy the interface so this file
// can host its own table tests without dragging in handler tests for
// the CRUD methods (covered separately by the integration suite).
type mockProcurementService struct {
	rvrResult uuid.UUID
	rvrErr    error

	listResult   []models.ProcurementItem
	listErr      error
	createResult models.ProcurementItem
	createErr    error
	updateResult models.ProcurementItem
	updateErr    error

	// Captured args for happy-path assertions.
	lastOrg      uuid.UUID
	lastUserSub  string
	lastInput    service.RequestVendorReviewInput
	rvrCallCount int

	lastListProjID uuid.UUID
	lastListOrg    uuid.UUID
	lastListStatus []string
	lastCreateOrg  uuid.UUID
	lastCreateSub  string
	lastCreateIn   service.CreateProcurementItemInput
	lastUpdateOrg  uuid.UUID
	lastUpdateSub  string
	lastUpdateIn   service.UpdateProcurementItemInput
}

func (m *mockProcurementService) ListProcurement(_ context.Context, projectID, callerOrgID uuid.UUID, statusFilter []string) ([]models.ProcurementItem, error) {
	m.lastListProjID, m.lastListOrg, m.lastListStatus = projectID, callerOrgID, statusFilter
	return m.listResult, m.listErr
}

func (m *mockProcurementService) CreateProcurementItem(_ context.Context, orgID uuid.UUID, sub string, in service.CreateProcurementItemInput) (models.ProcurementItem, error) {
	m.lastCreateOrg, m.lastCreateSub, m.lastCreateIn = orgID, sub, in
	return m.createResult, m.createErr
}

func (m *mockProcurementService) UpdateProcurementItem(_ context.Context, orgID uuid.UUID, sub string, in service.UpdateProcurementItemInput) (models.ProcurementItem, error) {
	m.lastUpdateOrg, m.lastUpdateSub, m.lastUpdateIn = orgID, sub, in
	return m.updateResult, m.updateErr
}

func (m *mockProcurementService) RequestVendorReview(_ context.Context, callerOrgID uuid.UUID, callerUserSub string, in service.RequestVendorReviewInput) (uuid.UUID, error) {
	m.rvrCallCount++
	m.lastOrg = callerOrgID
	m.lastUserSub = callerUserSub
	m.lastInput = in
	return m.rvrResult, m.rvrErr
}

func TestRequestVendorReview_HappyPathReturns201(t *testing.T) {
	feedCardID := uuid.MustParse("cccccccc-cccc-cccc-cccc-cccccccccccc")
	rfq := uuid.MustParse("dddddddd-dddd-dddd-dddd-dddddddddddd")
	svc := &mockProcurementService{rvrResult: feedCardID}
	h := NewProcurementHandler(svc)

	body := strings.NewReader(`{
        "vendor": "Acme Lumber",
        "total_cents": 50000,
        "currency_code": "USD",
        "rfq_id": "dddddddd-dddd-dddd-dddd-dddddddddddd",
        "reasoning": "lowest bid by 8%"
    }`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID+"/request-review",
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.RequestVendorReview(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201; body=%s", w.Code, w.Body.String())
	}
	// Service args propagate.
	if svc.lastOrg.String() != testOrgID {
		t.Errorf("service got org=%s, want %s", svc.lastOrg, testOrgID)
	}
	if svc.lastUserSub != "test-sub" {
		t.Errorf("service got user_sub=%q, want test-sub", svc.lastUserSub)
	}
	if svc.lastInput.ProcurementItemID.String() != testItemID {
		t.Errorf("service got item_id=%s, want %s", svc.lastInput.ProcurementItemID, testItemID)
	}
	if svc.lastInput.RFQID != rfq {
		t.Errorf("service got rfq_id=%s, want %s", svc.lastInput.RFQID, rfq)
	}
	if svc.lastInput.Vendor != "Acme Lumber" || svc.lastInput.TotalCents != 50000 || svc.lastInput.CurrencyCode != "USD" || svc.lastInput.Reasoning != "lowest bid by 8%" {
		t.Errorf("service got input=%+v, want vendor/total/currency/reasoning to round-trip", svc.lastInput)
	}
	// Response body carries the created feed-card id.
	if !strings.Contains(w.Body.String(), `"feed_card_id":"cccccccc-cccc-cccc-cccc-cccccccccccc"`) {
		t.Errorf("response missing feed_card_id: %s", w.Body.String())
	}
}

func TestRequestVendorReview_OmitRFQIDStaysNil(t *testing.T) {
	// AI-driven flow: caller doesn't supply rfq_id, so service
	// must see uuid.Nil (the wire-omit marker). Pin this contract so
	// future schema changes can't accidentally promote uuid.Nil to a
	// non-zero default.
	feedCardID := uuid.MustParse("eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee")
	svc := &mockProcurementService{rvrResult: feedCardID}
	h := NewProcurementHandler(svc)

	body := strings.NewReader(`{"vendor":"Acme","total_cents":1000,"currency_code":"USD"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID+"/request-review",
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.RequestVendorReview(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201; body=%s", w.Code, w.Body.String())
	}
	if svc.lastInput.RFQID != uuid.Nil {
		t.Errorf("rfq_id=%s, want uuid.Nil when JSON field omitted", svc.lastInput.RFQID)
	}
}

func TestRequestVendorReview_BadProjectIDReturns400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{})
	body := strings.NewReader(`{"vendor":"Acme","total_cents":1,"currency_code":"USD"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/not-a-uuid/procurement/"+testItemID+"/request-review",
		testOrgID, map[string]string{"projectID": "not-a-uuid", "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.RequestVendorReview(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestRequestVendorReview_BadItemIDReturns400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{})
	body := strings.NewReader(`{"vendor":"Acme","total_cents":1,"currency_code":"USD"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement/not-a-uuid/request-review",
		testOrgID, map[string]string{"projectID": testProjID, "itemID": "not-a-uuid"}, body)
	w := httptest.NewRecorder()
	h.RequestVendorReview(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestRequestVendorReview_InvalidJSONReturns400(t *testing.T) {
	svc := &mockProcurementService{}
	h := NewProcurementHandler(svc)
	body := strings.NewReader(`{not-json`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID+"/request-review",
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.RequestVendorReview(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
	if svc.rvrCallCount != 0 {
		t.Errorf("service called %d times on bad JSON; want 0 (handler must reject before service)", svc.rvrCallCount)
	}
}

func TestRequestVendorReview_InvalidInputReturns400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{rvrErr: service.ErrInvalidInput})
	body := strings.NewReader(`{"vendor":"","total_cents":-1,"currency_code":"XYZ"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID+"/request-review",
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.RequestVendorReview(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status=%d, want 400", w.Code)
	}
}

func TestRequestVendorReview_ItemNotFoundReturns404(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{rvrErr: service.ErrProcurementItemNotFound})
	body := strings.NewReader(`{"vendor":"Acme","total_cents":1,"currency_code":"USD"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID+"/request-review",
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.RequestVendorReview(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("status=%d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"NOT_FOUND"`) {
		t.Errorf("body missing NOT_FOUND code: %s", w.Body.String())
	}
}

func TestRequestVendorReview_VendorReviewUnavailableReturns503(t *testing.T) {
	// The worker binary constructs ProcurementService without a
	// feed-card store; if it ever reaches this handler (it shouldn't —
	// worker doesn't mount the router), 503 is the right answer because
	// the caller should retry against a server binary, not treat it as a
	// permanent input error.
	h := NewProcurementHandler(&mockProcurementService{rvrErr: service.ErrVendorReviewUnavailable})
	body := strings.NewReader(`{"vendor":"Acme","total_cents":1,"currency_code":"USD"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID+"/request-review",
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.RequestVendorReview(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status=%d, want 503", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"SERVICE_UNAVAILABLE"`) {
		t.Errorf("body missing SERVICE_UNAVAILABLE code: %s", w.Body.String())
	}
}

// ---------- GET /projects/{projectID}/procurement ----------

func TestProcurementList_OK(t *testing.T) {
	svc := &mockProcurementService{listResult: []models.ProcurementItem{{ID: uuid.New(), Name: "Rebar"}}}
	h := NewProcurementHandler(svc)
	r := buildRequest(t, "GET", "/api/v1/projects/"+testProjID+"/procurement?status=WARNING,CRITICAL",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	if svc.lastListProjID.String() != testProjID || svc.lastListOrg.String() != testOrgID {
		t.Errorf("list got proj=%s org=%s", svc.lastListProjID, svc.lastListOrg)
	}
	if got := svc.lastListStatus; len(got) != 2 || got[0] != "WARNING" || got[1] != "CRITICAL" {
		t.Errorf("status filter = %v, want [WARNING CRITICAL]", got)
	}
}

func TestProcurementList_BadProjectID400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{})
	r := buildRequest(t, "GET", "/api/v1/projects/not-a-uuid/procurement",
		testOrgID, map[string]string{"projectID": "not-a-uuid"}, nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestProcurementList_ServiceErr500(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{listErr: errInternal()})
	r := buildRequest(t, "GET", "/api/v1/projects/"+testProjID+"/procurement",
		testOrgID, map[string]string{"projectID": testProjID}, nil)
	w := httptest.NewRecorder()
	h.List(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d, want 500", w.Code)
	}
}

// ---------- POST /projects/{projectID}/procurement ----------

func TestProcurementCreate_OK(t *testing.T) {
	svc := &mockProcurementService{createResult: models.ProcurementItem{ID: uuid.New(), Name: "Lumber"}}
	h := NewProcurementHandler(svc)
	body := strings.NewReader(`{"name":"Lumber","wbs_code":"03-100","estimated_cost_cents":250000,"estimated_cost_currency_code":"USD","lead_time_days":14,"weather_buffer_days":2,"need_by_date":"2026-04-01"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Create(w, r)

	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	in := svc.lastCreateIn
	if in.ProjectID.String() != testProjID || in.Name != "Lumber" || in.WBSCode != "03-100" ||
		in.EstimatedCostCents != 250000 || in.EstimatedCostCurrencyCode != "USD" ||
		in.LeadTimeDays != 14 || in.WeatherBufferDays != 2 {
		t.Errorf("create input = %+v", in)
	}
	if in.NeedByDate == nil || !in.NeedByDate.Equal(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("need_by_date = %v, want 2026-04-01", in.NeedByDate)
	}
	if svc.lastCreateSub != "test-sub" {
		t.Errorf("sub = %q, want test-sub", svc.lastCreateSub)
	}
}

func TestProcurementCreate_BadProjectID400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{})
	r := buildRequest(t, "POST", "/api/v1/projects/not-a-uuid/procurement",
		testOrgID, map[string]string{"projectID": "not-a-uuid"}, strings.NewReader(`{"name":"x"}`))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestProcurementCreate_BadJSON400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{})
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement",
		testOrgID, map[string]string{"projectID": testProjID}, strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestProcurementCreate_BadNeedByDate400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{})
	body := strings.NewReader(`{"name":"x","need_by_date":"nope"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

func TestProcurementCreate_ServiceValidation400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{createErr: service.ErrInvalidInput})
	body := strings.NewReader(`{"name":"","estimated_cost_currency_code":"XYZ"}`)
	r := buildRequest(t, "POST", "/api/v1/projects/"+testProjID+"/procurement",
		testOrgID, map[string]string{"projectID": testProjID}, body)
	w := httptest.NewRecorder()
	h.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

// ---------- PUT /projects/{projectID}/procurement/{itemID} ----------

func TestProcurementUpdate_OK(t *testing.T) {
	svc := &mockProcurementService{updateResult: models.ProcurementItem{ID: uuid.MustParse(testItemID)}}
	h := NewProcurementHandler(svc)
	body := strings.NewReader(`{"status":"ORDERED","po_number":"PO-42","ordered_at":"2026-04-02T09:00:00Z"}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID,
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.Update(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status=%d, body=%s", w.Code, w.Body.String())
	}
	in := svc.lastUpdateIn
	if in.ItemID.String() != testItemID || in.ProjectID.String() != testProjID {
		t.Errorf("update ids = item %s proj %s", in.ItemID, in.ProjectID)
	}
	if in.Status == nil || *in.Status != "ORDERED" || in.PONumber == nil || *in.PONumber != "PO-42" {
		t.Errorf("update input = %+v", in)
	}
	if in.OrderedAt == nil || !in.OrderedAt.Equal(time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("ordered_at = %v, want 2026-04-02T09:00:00Z", in.OrderedAt)
	}
}

func TestProcurementUpdate_BadItemID400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{})
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/procurement/not-a-uuid",
		testOrgID, map[string]string{"projectID": testProjID, "itemID": "not-a-uuid"}, strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestProcurementUpdate_BadJSON400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{})
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID,
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, strings.NewReader("{bad"))
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

func TestProcurementUpdate_BadOrderedAt400(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{})
	// ordered_at only accepts full RFC3339 — a date-only value is rejected.
	body := strings.NewReader(`{"ordered_at":"2026-04-02"}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID,
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
	if code := decodeErrCode(t, w); code != "VALIDATION_ERROR" {
		t.Errorf("code=%q, want VALIDATION_ERROR", code)
	}
}

func TestProcurementUpdate_NotFound404(t *testing.T) {
	h := NewProcurementHandler(&mockProcurementService{updateErr: service.ErrProcurementItemNotFound})
	body := strings.NewReader(`{"status":"ORDERED"}`)
	r := buildRequest(t, "PUT", "/api/v1/projects/"+testProjID+"/procurement/"+testItemID,
		testOrgID, map[string]string{"projectID": testProjID, "itemID": testItemID}, body)
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", w.Code)
	}
}

func TestProcurementUpdate_BadProjectID400(t *testing.T) {
	// Update parses projectID FIRST (before itemID), so a bad projectID
	// short-circuits at the leading guard — distinct from the bad-itemID
	// leg covered above.
	h := NewProcurementHandler(&mockProcurementService{})
	r := buildRequest(t, "PUT", "/api/v1/projects/not-a-uuid/procurement/"+testItemID,
		testOrgID, map[string]string{"projectID": "not-a-uuid", "itemID": testItemID}, strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	h.Update(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", w.Code)
	}
}

// ---------- shared 401 guard leg (List/Create/Update/RequestVendorReview) ----------

type procurementHandlerFn func(*ProcurementHandler, http.ResponseWriter, *http.Request)

// TestProcurement_AllHandlers_InvalidOrgIDClaim_401 covers the
// callerOrgIDFromClaims 401 short-circuit shared by all four handlers:
// each parses the URL UUID(s) FIRST then the org claim, so a malformed
// org claim is rejected before the body is decoded or the service is
// consulted.
func TestProcurement_AllHandlers_InvalidOrgIDClaim_401(t *testing.T) {
	cases := []struct {
		name   string
		method string
		target string
		fn     procurementHandlerFn
	}{
		{"list", "GET", "/api/v1/projects/" + testProjID + "/procurement", (*ProcurementHandler).List},
		{"create", "POST", "/api/v1/projects/" + testProjID + "/procurement", (*ProcurementHandler).Create},
		{"update", "PUT", "/api/v1/projects/" + testProjID + "/procurement/" + testItemID, (*ProcurementHandler).Update},
		{"request-review", "POST", "/api/v1/projects/" + testProjID + "/procurement/" + testItemID + "/request-review", (*ProcurementHandler).RequestVendorReview},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := NewProcurementHandler(&mockProcurementService{})
			r := buildRequest(t, c.method, c.target, "not-a-uuid",
				map[string]string{"projectID": testProjID, "itemID": testItemID}, strings.NewReader(`{}`))
			w := httptest.NewRecorder()
			c.fn(h, w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("status=%d, want 401", w.Code)
			}
		})
	}
}

// ---------- procurement writeServiceError mapping ----------

func TestProcurementWriteServiceError_Mapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"vendor review unavailable", service.ErrVendorReviewUnavailable, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
		{"item not found", service.ErrProcurementItemNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"project not found", service.ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"validation", wrapInvalid("bad"), http.StatusBadRequest, "VALIDATION_ERROR"},
		{"default", errInternal(), http.StatusInternalServerError, "INTERNAL_ERROR"},
	}
	h := NewProcurementHandler(&mockProcurementService{})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, "/x", nil)
			h.writeServiceError(w, r, tt.err)
			if w.Code != tt.wantStatus {
				t.Fatalf("status=%d, want %d", w.Code, tt.wantStatus)
			}
			if code := decodeErrCode(t, w); code != tt.wantCode {
				t.Errorf("code=%q, want %q", code, tt.wantCode)
			}
		})
	}
}

// ---------- parseOptionalRFC3339 ----------

func TestParseOptionalRFC3339(t *testing.T) {
	if got, err := parseOptionalRFC3339(nil); err != nil || got != nil {
		t.Errorf("nil → %v, %v; want nil, nil", got, err)
	}
	empty := ""
	if got, err := parseOptionalRFC3339(&empty); err != nil || got != nil {
		t.Errorf("empty → %v, %v; want nil, nil", got, err)
	}
	dateOnly := "2026-04-02"
	if _, err := parseOptionalRFC3339(&dateOnly); err == nil {
		t.Error("date-only should error (RFC3339 only)")
	}
	ts := "2026-04-02T09:00:00Z"
	got, err := parseOptionalRFC3339(&ts)
	if err != nil {
		t.Fatalf("RFC3339: %v", err)
	}
	if got == nil || !got.Equal(time.Date(2026, 4, 2, 9, 0, 0, 0, time.UTC)) {
		t.Errorf("parsed = %v", got)
	}
}
