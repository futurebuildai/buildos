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

const testItemID = "55555555-5555-5555-5555-555555555555"

// mockProcurementService implements ProcurementServicer for handler
// tests. List/Create/Update aren't exercised by the RequestVendorReview
// suite below — they're stubbed to satisfy the interface so this file
// can host its own table tests without dragging in handler tests for
// the CRUD methods (covered separately by the integration suite).
type mockProcurementService struct {
	rvrResult uuid.UUID
	rvrErr    error

	// Captured args for happy-path assertions.
	lastOrg      uuid.UUID
	lastUserSub  string
	lastInput    service.RequestVendorReviewInput
	rvrCallCount int
}

func (m *mockProcurementService) ListProcurement(_ context.Context, _, _ uuid.UUID, _ []string) ([]models.ProcurementItem, error) {
	return nil, nil
}

func (m *mockProcurementService) CreateProcurementItem(_ context.Context, _ uuid.UUID, _ string, _ service.CreateProcurementItemInput) (models.ProcurementItem, error) {
	return models.ProcurementItem{}, nil
}

func (m *mockProcurementService) UpdateProcurementItem(_ context.Context, _ uuid.UUID, _ string, _ service.UpdateProcurementItemInput) (models.ProcurementItem, error) {
	return models.ProcurementItem{}, nil
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
