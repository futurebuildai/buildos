//go:build integration

package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// End-to-end integration tests for the A2A service: real Postgres,
// idempotency dedup actually goes through the table, feed cards land.

func TestA2AService_ProcessWebhook_ReviewMaterialQuote(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	envelope := WebhookEnvelope{
		EventType:      EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{"vendor":"Acme Lumber","total_cents":250000,"currency_code":"USD"}`),
		Issuer:         "fb-brain",
		OrgID:          orgID,
	}

	result, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}
	if result.AlreadyProcessed {
		t.Error("first call should not be already_processed")
	}
	if result.FeedCardID == nil {
		t.Fatal("first call should have created a feed card")
	}

	// Verify the feed card landed with the expected shape.
	var (
		cardType, title, body, priority, status string
		targetRole                              *string
	)
	err = pool.QueryRow(ctx, `
		SELECT card_type, title, body, priority, status, target_role
		FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(
		&cardType, &title, &body, &priority, &status, &targetRole,
	)
	if err != nil {
		t.Fatalf("fetch feed card: %v", err)
	}
	if cardType != "procurement.material_quote" {
		t.Errorf("card_type = %q", cardType)
	}
	if title != "Review material quote from Acme Lumber" {
		t.Errorf("title = %q", title)
	}
	if priority != "urgent" {
		t.Errorf("priority = %q, want urgent", priority)
	}
	if status != "active" {
		t.Errorf("status = %q, want active", status)
	}
	if targetRole == nil || *targetRole != "owner" {
		t.Errorf("target_role = %v, want owner", targetRole)
	}
}

func TestA2AService_ProcessWebhook_IdempotencyDedup(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	envelope := WebhookEnvelope{
		EventType:      EventCreateFeedCard,
		IdempotencyKey: uuid.New(), // same key both calls
		Payload:        json.RawMessage(`{"card_type":"test","title":"hello","body":"world","priority":"normal"}`),
		Issuer:         "fb-brain",
		OrgID:          orgID,
	}

	first, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if first.AlreadyProcessed {
		t.Error("first call should not be already_processed")
	}
	if first.FeedCardID == nil {
		t.Fatal("first call should have created a feed card")
	}

	second, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !second.AlreadyProcessed {
		t.Error("duplicate call should be already_processed")
	}
	if second.FeedCardID != nil {
		t.Error("duplicate call should NOT create a second feed card")
	}

	// Confirm only one feed card exists in the org.
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM feed_cards WHERE org_id = $1`, orgID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 feed card after dedup, got %d", count)
	}
}

func TestA2AService_ProcessWebhook_DefaultOrgID_Fallback(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	defaultOrg := uuid.New()
	testdb.SeedOrg(t, pool, defaultOrg, "Default Org")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), &defaultOrg)

	// Envelope WITHOUT OrgID — should fall back to defaultOrg.
	envelope := WebhookEnvelope{
		EventType:      EventCreateFeedCard,
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{"card_type":"test","title":"fallback","body":"","priority":"normal"}`),
		Issuer:         "fb-brain",
		// OrgID intentionally zero
	}

	result, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}
	if result.FeedCardID == nil {
		t.Fatal("expected feed card created via default org fallback")
	}

	// Verify it landed under the default org.
	var orgID uuid.UUID
	if err := pool.QueryRow(ctx, `SELECT org_id FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(&orgID); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if orgID != defaultOrg {
		t.Errorf("feed card org = %s, want default %s", orgID, defaultOrg)
	}
}

func TestA2AService_ProcessWebhook_UnknownEventType(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      "bogus_event_type",
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{}`),
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if err == nil {
		t.Fatal("unknown event type should error")
	}
	// Implementation detail: dedup row is rolled back when dispatch fails,
	// so no log row remains. Verify.
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM a2a_inbound_log`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 dedup rows after dispatch failure (tx rollback); got %d", count)
	}
}

func TestA2AService_ProcessWebhook_LocalblueLeadCaptured(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	contractorOrg := uuid.New()
	testdb.SeedOrg(t, pool, contractorOrg, "Smith Construction")

	// LocalBlue's chatbot captured a kitchen-remodel lead at the
	// contractor's website. Brain forwards it; BuildOS receiver lands it
	// as a prospect at stage='LEAD' + emits a feed card to the owner.
	// Note: envelope.OrgID is intentionally zero — the receiver must
	// pull contractor_org_id from the payload (LocalBlue knows it via
	// the chatbot's site_id binding).
	gsf := 2400
	payload := map[string]any{
		"lead_id":           uuid.New().String(),
		"contractor_org_id": contractorOrg.String(),
		"lead_name":         "Kitchen remodel",
		"contact_name":      "Jane Smith",
		"contact_email":     "jane@example.com",
		"contact_phone":     "+1-555-0100",
		"address":           "123 Main St, Austin TX",
		"estimated_gsf":     gsf,
		"source":            "website",
		"captured_at":       "2026-04-30T12:00:00Z",
	}
	body, _ := json.Marshal(payload)

	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)
	envelope := WebhookEnvelope{
		EventType:      EventLocalblueLeadCaptured,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		// OrgID intentionally zero — must resolve via payload.
	}

	result, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}
	if result.FeedCardID == nil {
		t.Fatal("expected feed card created from localblue lead")
	}

	// Verify the prospect landed in the contractor's org at stage=LEAD.
	var prospectName, clientName, source, stage string
	var probabilityPct int
	err = pool.QueryRow(ctx, `
		SELECT name, client_name, COALESCE(source, ''), pipeline_stage, probability_pct
		FROM pre_construction_prospects
		WHERE org_id = $1`, contractorOrg).Scan(&prospectName, &clientName, &source, &stage, &probabilityPct)
	if err != nil {
		t.Fatalf("fetch prospect: %v", err)
	}
	if prospectName != "Kitchen remodel" {
		t.Errorf("prospect name = %q", prospectName)
	}
	if clientName != "Jane Smith" {
		t.Errorf("client name = %q", clientName)
	}
	if source != "localblue:website" {
		t.Errorf("source tag = %q, want localblue:website", source)
	}
	if stage != "LEAD" {
		t.Errorf("stage = %q, want LEAD", stage)
	}
	if probabilityPct != 10 {
		t.Errorf("probability_pct = %d, want 10", probabilityPct)
	}

	// Verify the feed card landed under the contractor's org with
	// urgent priority + owner targeting.
	var cardOrgID uuid.UUID
	var cardType, title, priority string
	var targetRole *string
	err = pool.QueryRow(ctx, `
		SELECT org_id, card_type, title, priority, target_role
		FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(
		&cardOrgID, &cardType, &title, &priority, &targetRole)
	if err != nil {
		t.Fatalf("fetch feed card: %v", err)
	}
	if cardOrgID != contractorOrg {
		t.Errorf("feed card org = %s, want %s", cardOrgID, contractorOrg)
	}
	if cardType != "pipeline.lead_captured" {
		t.Errorf("card_type = %q", cardType)
	}
	if title != "New lead from LocalBlue" {
		t.Errorf("title = %q", title)
	}
	if priority != "urgent" {
		t.Errorf("priority = %q, want urgent", priority)
	}
	if targetRole == nil || *targetRole != "owner" {
		t.Errorf("target_role = %v, want owner", targetRole)
	}
}

func TestA2AService_ProcessWebhook_LocalblueLeadCaptured_RejectsMissingContractorOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	// Payload missing contractor_org_id — should fail validation
	// because the envelope has no OrgID either and there's no default.
	body := []byte(`{"lead_id":"` + uuid.New().String() + `","lead_name":"x","contact_name":"y","captured_at":"2026-04-30T00:00:00Z"}`)
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventLocalblueLeadCaptured,
		IdempotencyKey: uuid.New(),
		Payload:        body,
	})
	if err == nil {
		t.Fatal("expected error: missing contractor_org_id")
	}
}

func TestA2AService_ProcessWebhook_RequiresOrgID(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil) // no defaultOrgID

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventCreateFeedCard,
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{"title":"x","priority":"normal"}`),
		// OrgID missing
	})
	if err == nil {
		t.Fatal("missing org_id with no default should error")
	}
}
