package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/agents"
)

// ---------------------------------------------------------------------------
// BidLevelingHandler — construction
// ---------------------------------------------------------------------------

func TestNewBidLevelingHandler_NilAgent_ReturnsNil(t *testing.T) {
	h := NewBidLevelingHandler(nil)
	if h != nil {
		t.Error("expected nil handler when agent is nil")
	}
}

// ---------------------------------------------------------------------------
// BidLevelingHandler — AnalyzeBids validation
// ---------------------------------------------------------------------------

func TestBidLevelingHandler_AnalyzeBids_InvalidJSON(t *testing.T) {
	// Use a non-nil agent via unsafe pointer trick won't work since agent needs
	// real setup. Instead we test that the handler returns 400 for invalid JSON
	// before hitting the agent at all.
	// Since NewBidLevelingHandler(nil) returns nil, we need a non-nil agent.
	// We'll construct a BidLevelingHandler directly with a nil agent field
	// since the validation check happens before agent.AnalyzeBids is called.
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{broken}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestBidLevelingHandler_AnalyzeBids_EmptyBody(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(``)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_BODY")
}

func TestBidLevelingHandler_AnalyzeBids_InvalidOrgID(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"not-a-uuid",
		"project_id":"` + uuid.New().String() + `",
		"bids":[
			{"vendor":"A","line_items":[{"description":"x","quantity":1,"unit_price_cents":100,"currency_code":"USD"}]},
			{"vendor":"B","line_items":[{"description":"x","quantity":1,"unit_price_cents":200,"currency_code":"USD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ID")
}

func TestBidLevelingHandler_AnalyzeBids_InvalidProjectID(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"not-a-uuid",
		"bids":[
			{"vendor":"A","line_items":[{"description":"x","quantity":1,"unit_price_cents":100,"currency_code":"USD"}]},
			{"vendor":"B","line_items":[{"description":"x","quantity":1,"unit_price_cents":200,"currency_code":"USD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ID")
}

func TestBidLevelingHandler_AnalyzeBids_InvalidItemID(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	itemID := "bad-item-id"
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"item_id":"` + itemID + `",
		"bids":[
			{"vendor":"A","line_items":[{"description":"x","quantity":1,"unit_price_cents":100,"currency_code":"USD"}]},
			{"vendor":"B","line_items":[{"description":"x","quantity":1,"unit_price_cents":200,"currency_code":"USD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "INVALID_ID")
}

func TestBidLevelingHandler_AnalyzeBids_TooFewBids(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"bids":[
			{"vendor":"A","line_items":[{"description":"x","quantity":1,"unit_price_cents":100,"currency_code":"USD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestBidLevelingHandler_AnalyzeBids_ZeroBids(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"bids":[]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestBidLevelingHandler_AnalyzeBids_MissingVendorName(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"bids":[
			{"vendor":"","line_items":[{"description":"x","quantity":1,"unit_price_cents":100,"currency_code":"USD"}]},
			{"vendor":"B","line_items":[{"description":"x","quantity":1,"unit_price_cents":200,"currency_code":"USD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestBidLevelingHandler_AnalyzeBids_EmptyLineItems(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"bids":[
			{"vendor":"A","line_items":[]},
			{"vendor":"B","line_items":[{"description":"x","quantity":1,"unit_price_cents":200,"currency_code":"USD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestBidLevelingHandler_AnalyzeBids_MissingCurrencyCode(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"bids":[
			{"vendor":"A","line_items":[{"description":"x","quantity":1,"unit_price_cents":100,"currency_code":""}]},
			{"vendor":"B","line_items":[{"description":"x","quantity":1,"unit_price_cents":200,"currency_code":"USD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestBidLevelingHandler_AnalyzeBids_NegativeUnitPrice(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"bids":[
			{"vendor":"A","line_items":[{"description":"x","quantity":1,"unit_price_cents":-100,"currency_code":"USD"}]},
			{"vendor":"B","line_items":[{"description":"x","quantity":1,"unit_price_cents":200,"currency_code":"USD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	h.AnalyzeBids(rr, req)

	assertStatus(t, rr, http.StatusBadRequest)
	assertErrorCode(t, rr, "VALIDATION_ERROR")
}

func TestBidLevelingHandler_AnalyzeBids_ValidPayload_PassesValidation(t *testing.T) {
	// Valid payload should pass all handler validation, then panic on nil agent
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"bids":[
			{"vendor":"Vendor A","line_items":[
				{"description":"Lumber","quantity":100,"unit_price_cents":500,"currency_code":"USD"},
				{"description":"Nails","quantity":1000,"unit_price_cents":10,"currency_code":"USD"}
			]},
			{"vendor":"Vendor B","line_items":[
				{"description":"Lumber","quantity":100,"unit_price_cents":450,"currency_code":"USD"},
				{"description":"Nails","quantity":1000,"unit_price_cents":12,"currency_code":"USD"}
			]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil agent
			}
		}()
		h.AnalyzeBids(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid bid analysis payload should pass all handler validation, got 400: %s", rr.Body.String())
	}
}

func TestBidLevelingHandler_AnalyzeBids_ValidWithOptionalItemID(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	itemID := uuid.New().String()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"item_id":"` + itemID + `",
		"bids":[
			{"vendor":"A","line_items":[{"description":"x","quantity":1,"unit_price_cents":100,"currency_code":"USD"}]},
			{"vendor":"B","line_items":[{"description":"x","quantity":1,"unit_price_cents":200,"currency_code":"USD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil agent
			}
		}()
		h.AnalyzeBids(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("valid payload with item_id should pass validation, got 400: %s", rr.Body.String())
	}
}

func TestBidLevelingHandler_AnalyzeBids_ThreeBids_PassesValidation(t *testing.T) {
	h := &BidLevelingHandler{agent: nil}
	rr := httptest.NewRecorder()
	body := bytes.NewBufferString(`{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"bids":[
			{"vendor":"A","line_items":[{"description":"x","quantity":1,"unit_price_cents":100,"currency_code":"CAD"}]},
			{"vendor":"B","line_items":[{"description":"x","quantity":1,"unit_price_cents":200,"currency_code":"CAD"}]},
			{"vendor":"C","line_items":[{"description":"x","quantity":1,"unit_price_cents":150,"currency_code":"CAD"}]}
		]
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/procurement/bids/analyze", body)

	func() {
		defer func() {
			if r := recover(); r != nil {
				// Expected: nil agent
			}
		}()
		h.AnalyzeBids(rr, req)
	}()

	if rr.Code == http.StatusBadRequest {
		t.Errorf("three bids should pass validation, got 400: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Request body parsing tests — bid analysis BIGINT cents
// ---------------------------------------------------------------------------

func TestAnalyzeBidsRequest_Parsing(t *testing.T) {
	raw := `{
		"org_id":"` + uuid.New().String() + `",
		"project_id":"` + uuid.New().String() + `",
		"bids":[
			{"vendor":"Vendor A","line_items":[
				{"description":"Lumber","quantity":500,"unit_price_cents":99999999999,"currency_code":"USD"}
			]}
		]
	}`
	var req analyzeBidsRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(req.Bids) != 1 {
		t.Fatalf("expected 1 bid, got %d", len(req.Bids))
	}
	if req.Bids[0].Vendor != "Vendor A" {
		t.Errorf("expected vendor=Vendor A, got %s", req.Bids[0].Vendor)
	}
	if req.Bids[0].LineItems[0].UnitPriceCents != 99999999999 {
		t.Errorf("expected unit_price_cents=99999999999, got %d", req.Bids[0].LineItems[0].UnitPriceCents)
	}
	if req.Bids[0].LineItems[0].CurrencyCode != "USD" {
		t.Errorf("expected currency_code=USD, got %s", req.Bids[0].LineItems[0].CurrencyCode)
	}
}

func TestBidInput_CurrencyCodeInEveryLineItem(t *testing.T) {
	bid := agents.BidInput{
		Vendor: "Test",
		LineItems: []agents.BidLineItem{
			{Description: "A", Quantity: 1, UnitPriceCents: 100, CurrencyCode: "USD"},
			{Description: "B", Quantity: 2, UnitPriceCents: 200, CurrencyCode: "USD"},
		},
	}
	for i, li := range bid.LineItems {
		if li.CurrencyCode == "" {
			t.Errorf("line item %d missing currency_code", i)
		}
	}
}

func TestBidAnalysis_BIGINT_RankedBidTotalCents(t *testing.T) {
	raw := `{
		"ranked_bids":[{"vendor":"A","rank":1,"total_cents":99999999999,"currency_code":"USD","score":0.95,"notes":"best"}],
		"missing_scope":[],
		"outlier_flags":[],
		"recommendation":"Go with A",
		"confidence":0.90
	}`
	var analysis agents.BidAnalysis
	if err := json.Unmarshal([]byte(raw), &analysis); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if len(analysis.RankedBids) != 1 {
		t.Fatalf("expected 1 ranked bid, got %d", len(analysis.RankedBids))
	}
	if analysis.RankedBids[0].TotalCents != 99999999999 {
		t.Errorf("expected total_cents=99999999999, got %d", analysis.RankedBids[0].TotalCents)
	}
	if analysis.RankedBids[0].CurrencyCode != "USD" {
		t.Errorf("expected currency_code=USD, got %s", analysis.RankedBids[0].CurrencyCode)
	}
}

func TestOutlierFlag_BIGINT_Cents(t *testing.T) {
	raw := `{
		"vendor":"A",
		"description":"Steel Beams",
		"unit_price_cents":5000000,
		"currency_code":"CAD",
		"avg_price_cents":3000000,
		"deviation_pct":66.7,
		"direction":"high"
	}`
	var flag agents.OutlierFlag
	if err := json.Unmarshal([]byte(raw), &flag); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	if flag.UnitPriceCents != 5000000 {
		t.Errorf("expected unit_price_cents=5000000, got %d", flag.UnitPriceCents)
	}
	if flag.CurrencyCode != "CAD" {
		t.Errorf("expected currency_code=CAD, got %s", flag.CurrencyCode)
	}
	if flag.AvgPriceCents != 3000000 {
		t.Errorf("expected avg_price_cents=3000000, got %d", flag.AvgPriceCents)
	}
}

// ---------------------------------------------------------------------------
// itoa helper tests
// ---------------------------------------------------------------------------

func TestItoa(t *testing.T) {
	tests := []struct {
		input    int
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{10, "10"},
		{123, "123"},
		{-1, "-1"},
		{-42, "-42"},
		{999999, "999999"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			result := itoa(tc.input)
			if result != tc.expected {
				t.Errorf("itoa(%d) = %q, expected %q", tc.input, result, tc.expected)
			}
		})
	}
}
