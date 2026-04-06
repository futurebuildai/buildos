package api

import (
	"encoding/json"
	"log/slog"
	"testing"
)

func TestA2AEventTypes(t *testing.T) {
	events := []string{
		EventReviewMaterialQuote,
		EventReviewLaborBid,
		EventUpdateSchedule,
		EventDeliveryConfirmation,
		EventCreateFeedCard,
	}
	for _, e := range events {
		if e == "" {
			t.Error("event type constant must not be empty")
		}
	}
	if len(events) != 5 {
		t.Errorf("expected 5 event types, got %d", len(events))
	}
}

func TestA2AWebhookPayloadParsing(t *testing.T) {
	raw := `{
		"event_type": "review_material_quote",
		"payload": {"rfq_id": "abc-123", "total_cents": 50000, "currency_code": "USD"},
		"trace_id": "trace-001",
		"idempotency_key": "key-001",
		"timestamp": "2026-04-02T14:30:00Z",
		"iss": "fb-brain",
		"org_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	}`

	var p a2aWebhookPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to parse payload: %v", err)
	}

	if p.EventType != "review_material_quote" {
		t.Errorf("expected event_type=review_material_quote, got %s", p.EventType)
	}
	if p.TraceID != "trace-001" {
		t.Errorf("expected trace_id=trace-001, got %s", p.TraceID)
	}
	if p.IdempotencyKey != "key-001" {
		t.Errorf("expected idempotency_key=key-001, got %s", p.IdempotencyKey)
	}
	if p.Issuer != "fb-brain" {
		t.Errorf("expected iss=fb-brain, got %s", p.Issuer)
	}
	if p.OrgID != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("expected org_id=a1b2c3d4-e5f6-7890-abcd-ef1234567890, got %s", p.OrgID)
	}
	if p.Payload == nil {
		t.Error("expected payload to be non-nil")
	}
}

func TestMaterialQuotePayloadParsing(t *testing.T) {
	raw := `{
		"rfq_id": "rfq-001",
		"line_items": [
			{"name": "2x4 Lumber", "quantity": 500, "unit_price_cents": 450, "currency_code": "USD"},
			{"name": "Drywall 4x8", "quantity": 200, "unit_price_cents": 1200, "currency_code": "USD"}
		],
		"total_cents": 465000,
		"currency_code": "USD",
		"vendor": "GableERP"
	}`

	var p materialQuotePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if p.RFQID != "rfq-001" {
		t.Errorf("expected rfq_id=rfq-001, got %s", p.RFQID)
	}
	if len(p.LineItems) != 2 {
		t.Fatalf("expected 2 line items, got %d", len(p.LineItems))
	}
	if p.LineItems[0].UnitPriceCents != 450 {
		t.Errorf("expected unit_price_cents=450, got %d", p.LineItems[0].UnitPriceCents)
	}
	if p.LineItems[0].CurrencyCode != "USD" {
		t.Errorf("expected currency_code=USD, got %s", p.LineItems[0].CurrencyCode)
	}
	if p.TotalCents != 465000 {
		t.Errorf("expected total_cents=465000, got %d", p.TotalCents)
	}
	if p.CurrencyCode != "USD" {
		t.Errorf("expected currency_code=USD, got %s", p.CurrencyCode)
	}
	if p.Vendor != "GableERP" {
		t.Errorf("expected vendor=GableERP, got %s", p.Vendor)
	}
}

func TestLaborBidPayloadParsing(t *testing.T) {
	raw := `{
		"rfq_id": "rfq-002",
		"bidder": "Apex Roofing",
		"amount_cents": 1200000,
		"currency_code": "CAD",
		"timeline": "14 days",
		"ai_analysis": "Competitive bid"
	}`

	var p laborBidPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if p.AmountCents != 1200000 {
		t.Errorf("expected amount_cents=1200000, got %d", p.AmountCents)
	}
	if p.CurrencyCode != "CAD" {
		t.Errorf("expected currency_code=CAD, got %s", p.CurrencyCode)
	}
	if p.Bidder != "Apex Roofing" {
		t.Errorf("expected bidder=Apex Roofing, got %s", p.Bidder)
	}
}

func TestUpdateSchedulePayloadParsing(t *testing.T) {
	raw := `{
		"event_type": "material_delivery",
		"delivery_date": "2026-05-15",
		"constraints": {"wbs_codes": ["9.1", "9.2"]}
	}`

	var p updateSchedulePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if p.EventType != "material_delivery" {
		t.Errorf("expected event_type=material_delivery, got %s", p.EventType)
	}
	if len(p.Constraints.WBSCodes) != 2 {
		t.Fatalf("expected 2 WBS codes, got %d", len(p.Constraints.WBSCodes))
	}
}

func TestDeliveryConfirmationPayloadParsing(t *testing.T) {
	raw := `{
		"materials_ordered": true,
		"labor_approved": false,
		"convergence_status": "partial"
	}`

	var p deliveryConfirmationPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if !p.MaterialsOrdered {
		t.Error("expected materials_ordered=true")
	}
	if p.LaborApproved {
		t.Error("expected labor_approved=false")
	}
	if p.ConvergenceStatus != "partial" {
		t.Errorf("expected convergence_status=partial, got %s", p.ConvergenceStatus)
	}
}

func TestCreateFeedCardPayloadParsing(t *testing.T) {
	raw := `{
		"card_type": "procurement",
		"title": "Quote Ready for Review",
		"body": "GableERP quote #Q-2026-0042 is ready",
		"actions": [{"label": "Review", "action_type": "open_quote"}],
		"priority": "urgent"
	}`

	var p createFeedCardPayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if p.CardType != "procurement" {
		t.Errorf("expected card_type=procurement, got %s", p.CardType)
	}
	if p.Priority != "urgent" {
		t.Errorf("expected priority=urgent, got %s", p.Priority)
	}
	if p.Actions == nil {
		t.Error("expected actions to be non-nil")
	}
}

func TestDevFallbackOrgID(t *testing.T) {
	id := devFallbackOrgID()
	if id.String() != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected dev placeholder org ID, got %s", id.String())
	}
}

func TestResolveOrgID(t *testing.T) {
	h := &A2AHandler{devMode: true, logger: slog.Default()}

	// Valid org_id in payload
	payload := &a2aWebhookPayload{OrgID: "a1b2c3d4-e5f6-7890-abcd-ef1234567890"}
	orgID, err := h.resolveOrgID(payload)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if orgID.String() != "a1b2c3d4-e5f6-7890-abcd-ef1234567890" {
		t.Errorf("expected org_id from payload, got %s", orgID.String())
	}

	// Empty org_id falls back in dev mode
	payload = &a2aWebhookPayload{}
	orgID, err = h.resolveOrgID(payload)
	if err != nil {
		t.Fatalf("expected dev fallback, got error: %v", err)
	}
	if orgID.String() != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected dev fallback org ID, got %s", orgID.String())
	}

	// Empty org_id fails in production mode
	hProd := &A2AHandler{devMode: false}
	_, err = hProd.resolveOrgID(payload)
	if err == nil {
		t.Error("expected error for missing org_id in production mode")
	}

	// Invalid UUID format
	payload = &a2aWebhookPayload{OrgID: "not-a-uuid"}
	_, err = h.resolveOrgID(payload)
	if err == nil {
		t.Error("expected error for invalid UUID format")
	}
}

func TestUpdateSchedulePayloadProjectID(t *testing.T) {
	raw := `{
		"event_type": "material_delivery",
		"project_id": "11111111-2222-3333-4444-555555555555",
		"delivery_date": "2026-05-15",
		"constraints": {"wbs_codes": ["9.1", "9.2"]}
	}`

	var p updateSchedulePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if p.ProjectID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("expected project_id=11111111-2222-3333-4444-555555555555, got %s", p.ProjectID)
	}
}

func TestCurrencyCodePropagation(t *testing.T) {
	// Verify that material quote payloads carry currency_code at both
	// the root level and line-item level (Composite Currency Pattern)
	raw := `{
		"rfq_id": "rfq-100",
		"line_items": [
			{"name": "Lumber", "quantity": 100, "unit_price_cents": 500, "currency_code": "CAD"}
		],
		"total_cents": 50000,
		"currency_code": "CAD",
		"vendor": "TestVendor"
	}`

	var p materialQuotePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if p.CurrencyCode != "CAD" {
		t.Errorf("root currency_code should be CAD, got %s", p.CurrencyCode)
	}
	if p.LineItems[0].CurrencyCode != "CAD" {
		t.Errorf("line item currency_code should be CAD, got %s", p.LineItems[0].CurrencyCode)
	}
}
