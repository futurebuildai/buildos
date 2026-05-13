//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

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

// TestA2AService_ProcessWebhook_LocalblueLeadCaptured_IdempotencyReplay
// asserts the LocalBlue path inherits the receiver's dedup contract:
// Brain redelivers the same envelope (network blip on the first ACK is
// the canonical case) and the second call short-circuits inside the
// dedup INSERT — no duplicate prospect, no duplicate feed card. The
// generic dedup test exercises CreateFeedCard; this one is the
// load-bearing guarantee for the inbound-lead surface specifically,
// because a duplicate prospect would corrupt the contractor's
// pipeline analytics.
func TestA2AService_ProcessWebhook_LocalblueLeadCaptured_IdempotencyReplay(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	contractorOrg := uuid.New()
	testdb.SeedOrg(t, pool, contractorOrg, "Smith Construction")

	payload := map[string]any{
		"lead_id":           uuid.New().String(),
		"contractor_org_id": contractorOrg.String(),
		"lead_name":         "Bath remodel",
		"contact_name":      "Carlos Diaz",
		"contact_email":     "carlos@example.com",
		"source":            "website",
		"captured_at":       "2026-04-30T09:00:00Z",
	}
	body, _ := json.Marshal(payload)

	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)
	envelope := WebhookEnvelope{
		EventType:      EventLocalblueLeadCaptured,
		IdempotencyKey: uuid.New(), // same key on both calls
		Payload:        body,
		Issuer:         "fb-brain",
	}

	first, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	if first.AlreadyProcessed {
		t.Error("first call: already_processed=true, want false")
	}
	if first.FeedCardID == nil {
		t.Fatal("first call: expected feed card created")
	}

	second, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if !second.AlreadyProcessed {
		t.Error("second call: already_processed=false, want true (dedup must short-circuit)")
	}
	if second.FeedCardID != nil {
		t.Errorf("second call: feed_card_id=%v, want nil (dispatch must be skipped on replay)", second.FeedCardID)
	}

	// Hard guarantee: exactly one prospect + one feed card under the
	// contractor's org. The dedup row gets a unique key on
	// idempotency_key so a second INSERT into a2a_inbound_log is the
	// dedup signal — but the test that matters to the operator is
	// "did my pipeline get a duplicate lead?" → no.
	var prospectCount, feedCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM pre_construction_prospects WHERE org_id = $1`, contractorOrg).Scan(&prospectCount); err != nil {
		t.Fatalf("count prospects: %v", err)
	}
	if prospectCount != 1 {
		t.Errorf("prospect count = %d after replay, want 1", prospectCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM feed_cards WHERE org_id = $1`, contractorOrg).Scan(&feedCount); err != nil {
		t.Fatalf("count feed cards: %v", err)
	}
	if feedCount != 1 {
		t.Errorf("feed card count = %d after replay, want 1", feedCount)
	}
}

// TestA2AService_ProcessWebhook_LocalblueLeadCaptured_RejectsMissingLeadName
// asserts the validation gate trips on empty lead_name AND that the
// surrounding tx rolls back cleanly — no orphan dedup row, no orphan
// prospect. Without atomic rollback the receiver would mark the
// envelope "processed" while having silently dropped the work, and
// Brain's retry would correctly skip it (already_processed=true) →
// the lead would be lost forever. This test guards that contract.
func TestA2AService_ProcessWebhook_LocalblueLeadCaptured_RejectsMissingLeadName(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	contractorOrg := uuid.New()
	testdb.SeedOrg(t, pool, contractorOrg, "Smith Construction")

	body, _ := json.Marshal(map[string]any{
		"lead_id":           uuid.New().String(),
		"contractor_org_id": contractorOrg.String(),
		"lead_name":         "", // <-- the gate
		"contact_name":      "Jane Smith",
		"captured_at":       "2026-04-30T12:00:00Z",
	})

	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)
	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventLocalblueLeadCaptured,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}

	assertNoDedupOrOrphans(t, ctx, pool, contractorOrg)
}

// TestA2AService_ProcessWebhook_LocalblueLeadCaptured_RejectsMissingContactName
// is the symmetric case for contact_name. Same atomicity guarantee:
// validation failure → full rollback → Brain's retry can succeed once
// the upstream payload is corrected.
func TestA2AService_ProcessWebhook_LocalblueLeadCaptured_RejectsMissingContactName(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	contractorOrg := uuid.New()
	testdb.SeedOrg(t, pool, contractorOrg, "Smith Construction")

	body, _ := json.Marshal(map[string]any{
		"lead_id":           uuid.New().String(),
		"contractor_org_id": contractorOrg.String(),
		"lead_name":         "Garage build",
		"contact_name":      "", // <-- the gate
		"captured_at":       "2026-04-30T12:00:00Z",
	})

	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)
	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventLocalblueLeadCaptured,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}

	assertNoDedupOrOrphans(t, ctx, pool, contractorOrg)
}

// TestA2AService_ProcessWebhook_LocalblueLeadCaptured_OptionalFieldsMapToNull
// asserts the stringPtrOrNil contract: empty optional fields
// (contact_email, contact_phone, address) arrive at the prospect row
// as SQL NULL, NOT as the empty string "". Reasoning: downstream
// reporting + email-deliverability checks rely on IS NULL semantics;
// a stored "" would be a false-positive ("we have an email!") and
// would also trip Postgres uniqueness gates if any get added later.
func TestA2AService_ProcessWebhook_LocalblueLeadCaptured_OptionalFieldsMapToNull(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	contractorOrg := uuid.New()
	testdb.SeedOrg(t, pool, contractorOrg, "Smith Construction")

	// Required fields only; everything optional intentionally absent
	// or empty in the JSON.
	body, _ := json.Marshal(map[string]any{
		"lead_id":           uuid.New().String(),
		"contractor_org_id": contractorOrg.String(),
		"lead_name":         "Deck install",
		"contact_name":      "Marcus Lee",
		"contact_email":     "",
		"contact_phone":     "",
		"address":           "",
		"source":            "website",
		"captured_at":       "2026-04-30T12:00:00Z",
	})

	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)
	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventLocalblueLeadCaptured,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}

	var emailNull, phoneNull, addressNull bool
	err = pool.QueryRow(ctx, `
		SELECT client_email IS NULL, client_phone IS NULL, address IS NULL
		FROM pre_construction_prospects WHERE org_id = $1`, contractorOrg).Scan(
		&emailNull, &phoneNull, &addressNull)
	if err != nil {
		t.Fatalf("fetch prospect nullability: %v", err)
	}
	if !emailNull {
		t.Error("client_email should be NULL when payload contact_email is empty")
	}
	if !phoneNull {
		t.Error("client_phone should be NULL when payload contact_phone is empty")
	}
	if !addressNull {
		t.Error("address should be NULL when payload address is empty")
	}
}

// TestA2AService_ProcessWebhook_LocalblueLeadCaptured_DefaultSourceTag
// pins the localblueSourceTag boundary: when LocalBlue's payload omits
// source (or sends ""), the prospect's source column lands as plain
// "localblue" — NOT "localblue:" with a dangling colon. The colon-only
// variant would corrupt analytics buckets that group on `source`.
func TestA2AService_ProcessWebhook_LocalblueLeadCaptured_DefaultSourceTag(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	contractorOrg := uuid.New()
	testdb.SeedOrg(t, pool, contractorOrg, "Smith Construction")

	body, _ := json.Marshal(map[string]any{
		"lead_id":           uuid.New().String(),
		"contractor_org_id": contractorOrg.String(),
		"lead_name":         "Window replacement",
		"contact_name":      "Anil Patel",
		// source intentionally omitted → empty string after Unmarshal
		"captured_at": "2026-04-30T12:00:00Z",
	})

	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)
	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventLocalblueLeadCaptured,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}

	var source string
	if err := pool.QueryRow(ctx, `
		SELECT source FROM pre_construction_prospects WHERE org_id = $1`, contractorOrg).Scan(&source); err != nil {
		t.Fatalf("fetch source: %v", err)
	}
	if source != "localblue" {
		t.Errorf("source = %q, want %q (no dangling colon when upstream source is empty)", source, "localblue")
	}
}

// TestA2AService_ProcessWebhook_LocalblueLeadCaptured_NoPipelineStoreReturnsError
// asserts the defensive guard at the top of the handler: a service
// constructed without a pipelineStore (deployment opted out of the
// LocalBlue inbound surface) refuses the event with ErrInvalidInput
// rather than panicking on a nil deref. The dedup row from the
// surrounding tx must also roll back so Brain's retry semantics
// remain coherent if the operator later wires the pipelineStore in.
func TestA2AService_ProcessWebhook_LocalblueLeadCaptured_NoPipelineStoreReturnsError(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	contractorOrg := uuid.New()
	testdb.SeedOrg(t, pool, contractorOrg, "Smith Construction")

	body, _ := json.Marshal(map[string]any{
		"lead_id":           uuid.New().String(),
		"contractor_org_id": contractorOrg.String(),
		"lead_name":         "Pool install",
		"contact_name":      "Priya Rao",
		"captured_at":       "2026-04-30T12:00:00Z",
	})

	// pipelineStore explicitly nil — opt-out posture.
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), nil, nil)
	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventLocalblueLeadCaptured,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput (handler must reject when pipelineStore is nil)", err)
	}

	// Dedup row + prospect + feed card must all be rolled back.
	assertNoDedupOrOrphans(t, ctx, pool, contractorOrg)
}

// assertNoDedupOrOrphans is the rollback-atomicity assertion shared by
// the validation-failure and missing-pipelineStore tests. After a
// LocalBlue handler error, the tx must roll back fully — no
// a2a_inbound_log row (which would falsely mark the envelope as
// "processed" on Brain's retry), no orphan prospect, no orphan feed
// card.
func assertNoDedupOrOrphans(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgID uuid.UUID) {
	t.Helper()
	var dedupCount, prospectCount, feedCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM a2a_inbound_log`).Scan(&dedupCount); err != nil {
		t.Fatalf("count dedup rows: %v", err)
	}
	if dedupCount != 0 {
		t.Errorf("a2a_inbound_log count = %d, want 0 (dedup row must roll back on dispatch failure)", dedupCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM pre_construction_prospects WHERE org_id = $1`, orgID).Scan(&prospectCount); err != nil {
		t.Fatalf("count prospects: %v", err)
	}
	if prospectCount != 0 {
		t.Errorf("prospect count = %d, want 0 (must roll back with the tx)", prospectCount)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM feed_cards WHERE org_id = $1`, orgID).Scan(&feedCount); err != nil {
		t.Fatalf("count feed cards: %v", err)
	}
	if feedCount != 0 {
		t.Errorf("feed card count = %d, want 0 (must roll back with the tx)", feedCount)
	}
}

// TestA2AService_ProcessWebhook_ReviewMaterialQuote_LineItems exercises
// the receiver-side decoder addition for Brain's MaterialQuoteLineItem
// shape (ADR-003 follow-on). When the envelope carries line_items,
// the feed-card body picks up an "N items" suffix so the operator
// preview surfaces the itemization without needing to drill into the
// raw payload.
func TestA2AService_ProcessWebhook_ReviewMaterialQuote_LineItems(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	payload := map[string]any{
		"vendor":        "Acme Lumber",
		"total_cents":   750000,
		"currency_code": "USD",
		"line_items": []map[string]any{
			{"name": "2x4 stud", "quantity": 100, "unit_price_cents": 250, "currency_code": "USD"},
			{"name": "5/8 OSB sheathing", "quantity": 40, "unit_price_cents": 1850, "currency_code": "USD"},
			{"name": "30# felt", "quantity": 3, "unit_price_cents": 6000, "currency_code": "USD"},
		},
	}
	body, _ := json.Marshal(payload)

	result, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}
	if result.FeedCardID == nil {
		t.Fatal("expected feed card to land")
	}

	var cardBody string
	if err := pool.QueryRow(ctx, `SELECT body FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(&cardBody); err != nil {
		t.Fatalf("fetch feed card body: %v", err)
	}
	// Body should be "7500.00 USD · 3 items"
	wantSuffix := "· 3 items"
	if !strings.HasSuffix(cardBody, wantSuffix) {
		t.Errorf("feed card body = %q, want suffix %q", cardBody, wantSuffix)
	}
	// And the money portion still leads.
	if !strings.HasPrefix(cardBody, "7500.00 USD") {
		t.Errorf("feed card body = %q, want prefix %q", cardBody, "7500.00 USD")
	}
}

// TestA2AService_ProcessWebhook_ReviewMaterialQuote_SingularItem pins
// the pluralize boundary so a future cleanup of the helper can't
// silently regress display copy.
func TestA2AService_ProcessWebhook_ReviewMaterialQuote_SingularItem(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"vendor":        "Acme Lumber",
		"total_cents":   1000,
		"currency_code": "USD",
		"line_items": []map[string]any{
			{"name": "2x4 stud", "quantity": 4, "unit_price_cents": 250, "currency_code": "USD"},
		},
	})

	result, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}

	var cardBody string
	if err := pool.QueryRow(ctx, `SELECT body FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(&cardBody); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.HasSuffix(cardBody, "· 1 item") {
		t.Errorf("singular form regression: feed card body = %q, want suffix %q", cardBody, "· 1 item")
	}
}

// TestA2AService_ProcessWebhook_ReviewMaterialQuote_RejectsMixedCurrency
// guards the composite-currency invariant at the receiver. A line item
// with currency_code different from the envelope's would let a mixed-
// currency quote land as a single feed card whose aggregate has no
// canonical interpretation. Reject + tx rollback so Brain's retry can
// succeed once the upstream payload is corrected.
func TestA2AService_ProcessWebhook_ReviewMaterialQuote_RejectsMixedCurrency(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"vendor":        "Acme Lumber",
		"total_cents":   500000,
		"currency_code": "USD",
		"line_items": []map[string]any{
			{"name": "ok item", "quantity": 10, "unit_price_cents": 100, "currency_code": "USD"},
			{"name": "rogue item", "quantity": 1, "unit_price_cents": 49900, "currency_code": "CAD"},
		},
	})

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}

	// Full rollback: no dedup row, no feed card.
	assertNoDedupOrOrphans(t, ctx, pool, orgID)
}

// TestA2AService_ProcessWebhook_ReviewMaterialQuote_RejectsNegativeQuantity
// guards against a buggy Brain emitter producing a negative quantity —
// silently allowed by JSON decode, but would produce a nonsense feed
// card preview and corrupt any downstream aggregation that summed
// quantities.
func TestA2AService_ProcessWebhook_ReviewMaterialQuote_RejectsNegativeQuantity(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"vendor":        "Acme Lumber",
		"total_cents":   100,
		"currency_code": "USD",
		"line_items": []map[string]any{
			{"name": "bad qty", "quantity": -3, "unit_price_cents": 250, "currency_code": "USD"},
		},
	})

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	assertNoDedupOrOrphans(t, ctx, pool, orgID)
}

// TestA2AService_ProcessWebhook_ReviewMaterialQuote_RejectsNegativeUnitPrice
// is the symmetric guard for unit_price_cents. Negative cents would
// produce a feed card body with a negative money preview and trip
// downstream cost-aggregation.
func TestA2AService_ProcessWebhook_ReviewMaterialQuote_RejectsNegativeUnitPrice(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"vendor":        "Acme Lumber",
		"total_cents":   100,
		"currency_code": "USD",
		"line_items": []map[string]any{
			{"name": "bad price", "quantity": 1, "unit_price_cents": -250, "currency_code": "USD"},
		},
	})

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventReviewMaterialQuote,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	assertNoDedupOrOrphans(t, ctx, pool, orgID)
}

// TestA2AService_ProcessWebhook_ReviewLaborBid_WithAIAnalysis covers
// the receiver-side decoder addition for Brain's ai_analysis field.
// When non-empty, the analysis is appended to the feed-card body
// preview so an operator triaging the inbound bid sees Brain's
// reasoning hint inline rather than needing to open the full bid view.
func TestA2AService_ProcessWebhook_ReviewLaborBid_WithAIAnalysis(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"bidder":        "Hammer & Nail Inc",
		"amount_cents":  450000,
		"currency_code": "USD",
		"timeline":      "3 weeks",
		"ai_analysis":   "Within 8% of peer median; verified license; one OSHA citation 2023.",
	})

	result, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventReviewLaborBid,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}
	if result.FeedCardID == nil {
		t.Fatal("expected feed card")
	}

	var cardBody string
	if err := pool.QueryRow(ctx, `SELECT body FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(&cardBody); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.Contains(cardBody, "analysis: Within 8% of peer median") {
		t.Errorf("feed card body = %q, want to contain ai_analysis preview", cardBody)
	}
	// Timeline must still come through alongside the analysis.
	if !strings.Contains(cardBody, "timeline: 3 weeks") {
		t.Errorf("feed card body = %q, want to contain timeline", cardBody)
	}
}

// TestA2AService_ProcessWebhook_ReviewLaborBid_AnalysisTruncation pins
// the 200-rune cap on the analysis preview. Feed-card body is a
// notification surface; the full text is preserved in
// a2a_inbound_log.payload for any caller that needs it.
func TestA2AService_ProcessWebhook_ReviewLaborBid_AnalysisTruncation(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	// Build a 250-rune analysis from a repeating ASCII pattern (each
	// rune = 1 byte, easy to assert against).
	longAnalysis := strings.Repeat("x", 250)

	body, _ := json.Marshal(map[string]any{
		"bidder":        "Hammer & Nail Inc",
		"amount_cents":  450000,
		"currency_code": "USD",
		"ai_analysis":   longAnalysis,
	})

	result, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventReviewLaborBid,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}

	var cardBody string
	if err := pool.QueryRow(ctx, `SELECT body FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(&cardBody); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// Expected analysis chunk: 200 x's + ellipsis.
	wantAnalysis := strings.Repeat("x", 200) + "…"
	if !strings.Contains(cardBody, "analysis: "+wantAnalysis) {
		t.Errorf("feed card body = %q\nwant to contain truncated analysis %q", cardBody, wantAnalysis)
	}
	// Negative pin: the un-truncated 250-rune form must NOT appear.
	if strings.Contains(cardBody, longAnalysis) {
		t.Errorf("feed card body = %q, full untruncated analysis leaked through", cardBody)
	}
}

// TestA2AService_ProcessWebhook_CreateFeedCard_ValidTargetRoles is a
// table over the four BuildOS RBAC roles, pinning that each one
// passes validation and persists onto the feed card unchanged.
func TestA2AService_ProcessWebhook_CreateFeedCard_ValidTargetRoles(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	for _, role := range []string{"owner", "admin", "superintendent", "field_worker"} {
		t.Run(role, func(t *testing.T) {
			body, _ := json.Marshal(map[string]any{
				"card_type":   "brain.generic",
				"title":       "Test " + role,
				"body":        "hi",
				"priority":    "normal",
				"target_role": role,
			})
			result, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
				EventType:      EventCreateFeedCard,
				IdempotencyKey: uuid.New(),
				Payload:        body,
				Issuer:         "fb-brain",
				OrgID:          orgID,
			})
			if err != nil {
				t.Fatalf("ProcessWebhook: %v", err)
			}
			if result.FeedCardID == nil {
				t.Fatal("expected feed card")
			}

			var got *string
			if err := pool.QueryRow(ctx, `SELECT target_role FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(&got); err != nil {
				t.Fatalf("fetch: %v", err)
			}
			if got == nil || *got != role {
				t.Errorf("target_role = %v, want %s", got, role)
			}
		})
	}
}

// TestA2AService_ProcessWebhook_CreateFeedCard_RejectsInvalidTargetRole
// pins the L8 strict-validation choice: a non-empty target_role that
// isn't in the RBAC vocabulary is a wire-shape violation. Reject with
// ErrInvalidInput so the tx rolls back, no card lands silently muted
// to "owner", and Brain's retry succeeds once the upstream payload is
// corrected. Empty (absent) still defaults to "owner" — that's the
// distinct legacy path covered by other tests.
func TestA2AService_ProcessWebhook_CreateFeedCard_RejectsInvalidTargetRole(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"card_type":   "brain.generic",
		"title":       "Test",
		"body":        "hi",
		"priority":    "normal",
		"target_role": "PROJECT_MANAGER", // not in BuildOS vocabulary
	})

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventCreateFeedCard,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	// Tx rollback: no dedup row, no feed card.
	assertNoDedupOrOrphans(t, ctx, pool, orgID)
}

// TestA2AService_ProcessWebhook_CreateFeedCard_EmptyTargetRoleDefaultsToOwner
// is the legacy-default pin. Empty target_role on the wire is the
// pre-ADR-003 default (Brain didn't emit the field). The receiver
// continues to fall back to "owner" so existing Brain emitters keep
// working unchanged.
func TestA2AService_ProcessWebhook_CreateFeedCard_EmptyTargetRoleDefaultsToOwner(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	// target_role intentionally omitted from the JSON.
	body, _ := json.Marshal(map[string]any{
		"card_type": "brain.generic",
		"title":     "Test",
		"body":      "hi",
		"priority":  "normal",
	})

	result, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventCreateFeedCard,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}

	var got *string
	if err := pool.QueryRow(ctx, `SELECT target_role FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(&got); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if got == nil || *got != "owner" {
		t.Errorf("target_role = %v, want owner (legacy default)", got)
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

// TestA2AService_ProcessWebhook_UpdateSchedule covers the happy path:
// Brain emits a delivery-date + affected WBS codes, BuildOS lands a
// superintendent-targeted normal-priority feed card with both fields
// rendered in the body. The CPM-recalc enqueue is intentionally NOT
// triggered yet (blocked on Brain emitting project_id — see service
// handler comment).
func TestA2AService_ProcessWebhook_UpdateSchedule(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"event_type":    "material_delay",
		"delivery_date": "2026-06-15",
		"constraints":   map[string]any{"wbs_codes": []string{"03-30-00", "06-10-00"}},
	})

	result, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventUpdateSchedule,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}
	if result.FeedCardID == nil {
		t.Fatal("expected feed card")
	}

	var cardType, cardBody, priority string
	var targetRole *string
	if err := pool.QueryRow(ctx, `
		SELECT card_type, body, priority, target_role
		FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(&cardType, &cardBody, &priority, &targetRole); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if cardType != "schedule.update_requested" {
		t.Errorf("card_type = %q", cardType)
	}
	if priority != "normal" {
		t.Errorf("priority = %q, want normal", priority)
	}
	if targetRole == nil || *targetRole != "superintendent" {
		t.Errorf("target_role = %v, want superintendent", targetRole)
	}
	if !strings.Contains(cardBody, "2026-06-15") {
		t.Errorf("body = %q, want delivery_date", cardBody)
	}
	if !strings.Contains(cardBody, "03-30-00") || !strings.Contains(cardBody, "06-10-00") {
		t.Errorf("body = %q, want both WBS codes", cardBody)
	}
}

// TestA2AService_ProcessWebhook_UpdateSchedule_IdempotencyReplay pins
// the dedup contract for update_schedule. Brain's at-least-once
// delivery means a Maestro retry can resubmit the same envelope; the
// receiver must produce exactly one feed card.
func TestA2AService_ProcessWebhook_UpdateSchedule_IdempotencyReplay(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	envelope := WebhookEnvelope{
		EventType:      EventUpdateSchedule,
		IdempotencyKey: uuid.New(), // same key both calls
		Payload:        json.RawMessage(`{"event_type":"material_delay","delivery_date":"2026-06-15","constraints":{"wbs_codes":["03-30-00"]}}`),
		Issuer:         "fb-brain",
		OrgID:          orgID,
	}

	first, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if first.AlreadyProcessed || first.FeedCardID == nil {
		t.Fatalf("first call shape = %+v", first)
	}
	second, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !second.AlreadyProcessed {
		t.Error("replay should be already_processed")
	}
	if second.FeedCardID != nil {
		t.Error("replay should NOT create a second feed card")
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM feed_cards WHERE org_id=$1`, orgID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 feed card after replay, got %d", count)
	}
}

// TestA2AService_ProcessWebhook_UpdateSchedule_RejectsOversizedWBSList
// guards the per-payload WBS-code count cap. A buggy Brain emit could
// otherwise inflate the feed-card body unboundedly and degrade the
// notification UI.
func TestA2AService_ProcessWebhook_UpdateSchedule_RejectsOversizedWBSList(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	codes := make([]string, 257) // one over the 256 cap
	for i := range codes {
		codes[i] = "wbs"
	}
	body, _ := json.Marshal(map[string]any{
		"event_type":    "x",
		"delivery_date": "2026-06-15",
		"constraints":   map[string]any{"wbs_codes": codes},
	})

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventUpdateSchedule,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	assertNoDedupOrOrphans(t, ctx, pool, orgID)
}

// TestA2AService_ProcessWebhook_UpdateSchedule_RejectsOversizedWBSCode
// guards the per-WBS-code length cap. Same rationale as the count cap:
// keep the feed-card body bounded against a malformed upstream emit.
func TestA2AService_ProcessWebhook_UpdateSchedule_RejectsOversizedWBSCode(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	huge := strings.Repeat("x", 65) // one over the 64-byte cap
	body, _ := json.Marshal(map[string]any{
		"event_type":  "x",
		"constraints": map[string]any{"wbs_codes": []string{huge}},
	})

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventUpdateSchedule,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	assertNoDedupOrOrphans(t, ctx, pool, orgID)
}

// TestA2AService_ProcessWebhook_DeliveryConfirmation covers the happy
// path: a convergence_status of "converged" with materials_ordered=true
// and labor_approved=true should land a normal-priority,
// superintendent-targeted feed card whose body interpolates all three
// signals.
func TestA2AService_ProcessWebhook_DeliveryConfirmation(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"materials_ordered":  true,
		"labor_approved":     true,
		"convergence_status": "converged",
	})

	result, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventDeliveryConfirmation,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}
	if result.FeedCardID == nil {
		t.Fatal("expected feed card")
	}

	var cardType, cardBody, priority string
	var targetRole *string
	if err := pool.QueryRow(ctx, `
		SELECT card_type, body, priority, target_role
		FROM feed_cards WHERE id = $1`, *result.FeedCardID).Scan(&cardType, &cardBody, &priority, &targetRole); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if cardType != "procurement.delivery_confirmation" {
		t.Errorf("card_type = %q", cardType)
	}
	if priority != "normal" {
		t.Errorf("priority = %q, want normal", priority)
	}
	if targetRole == nil || *targetRole != "superintendent" {
		t.Errorf("target_role = %v, want superintendent", targetRole)
	}
	if !strings.Contains(cardBody, "Materials ordered") {
		t.Errorf("body = %q, want 'Materials ordered'", cardBody)
	}
	if !strings.Contains(cardBody, "Labor ordered") {
		t.Errorf("body = %q, want 'Labor ordered'", cardBody)
	}
	if !strings.Contains(cardBody, "Convergence: converged") {
		t.Errorf("body = %q, want 'Convergence: converged'", cardBody)
	}
}

// TestA2AService_ProcessWebhook_DeliveryConfirmation_EmptyStatusDefaults
// pins the "" → "in_progress" normalization so an upstream emit that
// omits convergence_status still renders a complete feed-card body.
func TestA2AService_ProcessWebhook_DeliveryConfirmation_EmptyStatusDefaults(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"materials_ordered": false,
		"labor_approved":    false,
		// convergence_status intentionally omitted
	})

	result, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventDeliveryConfirmation,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if err != nil {
		t.Fatalf("ProcessWebhook: %v", err)
	}

	var cardBody string
	if err := pool.QueryRow(ctx, `SELECT body FROM feed_cards WHERE id=$1`, *result.FeedCardID).Scan(&cardBody); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.Contains(cardBody, "Convergence: in_progress") {
		t.Errorf("body = %q, want 'Convergence: in_progress'", cardBody)
	}
	if !strings.Contains(cardBody, "Materials pending") {
		t.Errorf("body = %q, want 'Materials pending'", cardBody)
	}
	if !strings.Contains(cardBody, "Labor pending") {
		t.Errorf("body = %q, want 'Labor pending'", cardBody)
	}
}

// TestA2AService_ProcessWebhook_DeliveryConfirmation_RejectsOversizedStatus
// guards the convergence_status length cap. The body cap keeps a
// pathological upstream emit from inflating the feed-card UI.
func TestA2AService_ProcessWebhook_DeliveryConfirmation_RejectsOversizedStatus(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	body, _ := json.Marshal(map[string]any{
		"materials_ordered":  true,
		"labor_approved":     true,
		"convergence_status": strings.Repeat("x", 65),
	})

	_, err := svc.ProcessWebhook(ctx, WebhookEnvelope{
		EventType:      EventDeliveryConfirmation,
		IdempotencyKey: uuid.New(),
		Payload:        body,
		Issuer:         "fb-brain",
		OrgID:          orgID,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	assertNoDedupOrOrphans(t, ctx, pool, orgID)
}

// TestA2AService_ProcessWebhook_DeliveryConfirmation_IdempotencyReplay
// pins the dedup contract for delivery_confirmation, symmetric with
// the update_schedule replay test.
func TestA2AService_ProcessWebhook_DeliveryConfirmation_IdempotencyReplay(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	svc := NewA2AService(pool, store.NewA2AStore(), store.NewFeedCardsStore(), store.NewPipelineStore(), nil)

	envelope := WebhookEnvelope{
		EventType:      EventDeliveryConfirmation,
		IdempotencyKey: uuid.New(),
		Payload:        json.RawMessage(`{"materials_ordered":true,"labor_approved":true,"convergence_status":"converged"}`),
		Issuer:         "fb-brain",
		OrgID:          orgID,
	}

	if _, err := svc.ProcessWebhook(ctx, envelope); err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := svc.ProcessWebhook(ctx, envelope)
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if !second.AlreadyProcessed {
		t.Error("replay should be already_processed")
	}
	if second.FeedCardID != nil {
		t.Error("replay should NOT create a second feed card")
	}

	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM feed_cards WHERE org_id=$1`, orgID).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 feed card after replay, got %d", count)
	}
}
