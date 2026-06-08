//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/currency"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// These tests exercise the FULL Phase 2a ingestion write tx end-to-end
// against an ephemeral, freshly migrated Postgres (testdb.NewPool): the
// fuzzy AI extract is a deterministic fake, but the deterministic gate, the
// invoice create (through BudgetService.createInvoiceTx), the review feed
// card, the idempotency outbox claim, and the audit write all hit real SQL
// in one tx. The unit tests (invoice_ingest_test.go) cover the validation
// matrix with a nil pool; these prove the persistence + idempotency + the
// soft-fail-writes-nothing invariant.

// fakeIngestExtractor is the integration test double for invoiceExtractorAI:
// it replays a scripted response/error with no HTTP server, so the write tx
// can be driven deterministically. (fakeExtractor in the unit test file is
// shadowed by the !integration build constraint; this one lives behind the
// integration tag.)
type fakeIngestExtractor struct {
	resp  *ai.InvoiceExtractResponse
	err   error
	calls int
}

func (f *fakeIngestExtractor) InvoiceExtract(_ context.Context, _ ai.InvoiceExtractRequest) (*ai.InvoiceExtractResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// ingestFixture bundles the seeded ids + the real IngestionService under
// test plus its sibling stores (so assertions can read rows back).
type ingestFixture struct {
	pool      *pgxpool.Pool
	svc       *IngestionService
	orgID     uuid.UUID
	projectID uuid.UUID
	callerSub string // a parseable UUID so extracted_by lands non-nil
}

// newIngestFixture wires a REAL IngestionService over a fresh migrated pool
// with a REAL BudgetService + AuditService (so the in-tx invoice + audit
// writes actually hit invoices/audit_log and the assertions can count them).
// The AI seam is the supplied extractor (set ai=nil on the returned service
// to exercise the soft-fail leg). It seeds an org + project.
func newIngestFixture(t *testing.T, ext invoiceExtractorAI) *ingestFixture {
	t.Helper()
	pool := testdb.NewPool(t)

	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	budget := NewBudgetService(pool, store.NewFinancialsStore(), audit)

	svc := &IngestionService{
		pool:      pool,
		ai:        ext, // may be nil to drive the soft-fail leg
		budget:    budget,
		ingStore:  store.NewInvoiceIngestionStore(),
		feedStore: store.NewFeedCardsStore(),
		audit:     audit,
	}

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Cedar Ridge Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Birchwood Custom")

	return &ingestFixture{
		pool:      pool,
		svc:       svc,
		orgID:     orgID,
		projectID: projectID,
		callerSub: uuid.New().String(),
	}
}

// validIngestResp is the canonical good extraction: vendor + invoice_no, USD,
// a positive total, and two line items that sum exactly to the total.
func validIngestResp() *ai.InvoiceExtractResponse {
	return &ai.InvoiceExtractResponse{
		VendorName:   "Acme Lumber",
		InvoiceNo:    "INV-2001",
		IssuedDate:   "2026-06-01",
		TotalCents:   150000,
		CurrencyCode: currency.USD,
		LineItems: []ai.InvoiceExtractLineItem{
			{Description: "2x4 studs", Quantity: 100, UnitCents: 1000, AmountCents: 100000},
			{Description: "plywood", Quantity: 25, UnitCents: 2000, AmountCents: 50000},
		},
	}
}

// --- row-count / row-read helpers --------------------------------------

func countRows(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count query %q: %v", sql, err)
	}
	return n
}

func ingestInvoiceCount(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID) int {
	return countRows(t, pool, `SELECT count(*) FROM invoices WHERE project_id = $1`, projectID)
}

func ingestFeedCardCount(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID) int {
	return countRows(t, pool, `SELECT count(*) FROM feed_cards WHERE org_id = $1`, orgID)
}

func ingestOutboxCount(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID) int {
	return countRows(t, pool, `SELECT count(*) FROM invoice_ingestions WHERE project_id = $1`, projectID)
}

func ingestAuditCount(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID) int {
	return countRows(t, pool, `SELECT count(*) FROM audit_log WHERE org_id = $1 AND action = $2`,
		orgID, "ingestion.invoice.extracted")
}

// assertZeroPersisted is the soft-fail / reject invariant: NOTHING landed.
func assertZeroPersisted(t *testing.T, fx *ingestFixture) {
	t.Helper()
	if n := ingestInvoiceCount(t, fx.pool, fx.projectID); n != 0 {
		t.Errorf("invoices = %d, want 0 (nothing persisted)", n)
	}
	if n := ingestFeedCardCount(t, fx.pool, fx.orgID); n != 0 {
		t.Errorf("feed_cards = %d, want 0 (nothing persisted)", n)
	}
	if n := ingestOutboxCount(t, fx.pool, fx.projectID); n != 0 {
		t.Errorf("invoice_ingestions = %d, want 0 (nothing persisted)", n)
	}
	if n := ingestAuditCount(t, fx.pool, fx.orgID); n != 0 {
		t.Errorf("ingestion audit rows = %d, want 0 (nothing persisted)", n)
	}
}

// TestIngestInvoice_DocumentToInvoiceCardAudit is the end-to-end happy path:
// a real org + project, a FAKE extractor returning a valid response → the
// write tx lands exactly one pending ai_ingest invoice, one invoice_review
// feed card targeted at admin (actions carrying the invoice id), one
// invoice_ingestions outbox row linking both, and one
// ingestion.invoice.extracted audit row on the real invoice id — all in one
// committed tx.
func TestIngestInvoice_DocumentToInvoiceCardAudit(t *testing.T) {
	fx := newIngestFixture(t, &fakeIngestExtractor{resp: validIngestResp()})
	ctx := context.Background()

	res, err := fx.svc.IngestInvoiceFromDocument(ctx, fx.orgID, fx.callerSub, IngestInvoiceInput{
		ProjectID:      fx.projectID,
		IdempotencyKey: uuid.New(),
		Text:           "raw invoice text",
	})
	if err != nil {
		t.Fatalf("IngestInvoiceFromDocument: %v", err)
	}

	// --- the invoice: exactly one, pending, ai_ingest, validated total ---
	if n := ingestInvoiceCount(t, fx.pool, fx.projectID); n != 1 {
		t.Fatalf("invoices = %d, want 1", n)
	}
	var (
		gotStatus   string
		gotSource   string
		gotAmount   int64
		gotCurrency string
		gotVendor   string
	)
	if err := fx.pool.QueryRow(ctx, `
		SELECT status, source, amount_cents, currency_code, vendor_name
		FROM invoices WHERE id = $1`, res.Invoice.ID).
		Scan(&gotStatus, &gotSource, &gotAmount, &gotCurrency, &gotVendor); err != nil {
		t.Fatalf("read invoice: %v", err)
	}
	if gotStatus != "pending" {
		t.Errorf("status = %q, want pending (AI never auto-approves money)", gotStatus)
	}
	if gotSource != "ai_ingest" {
		t.Errorf("source = %q, want ai_ingest", gotSource)
	}
	if gotAmount != 150000 {
		t.Errorf("amount_cents = %d, want 150000 (validated total)", gotAmount)
	}
	if gotCurrency != currency.USD {
		t.Errorf("currency_code = %q, want USD", gotCurrency)
	}
	if gotVendor != "Acme Lumber" {
		t.Errorf("vendor_name = %q, want Acme Lumber", gotVendor)
	}

	// --- the review card: exactly one, invoice_review, admin, actions ----
	if n := ingestFeedCardCount(t, fx.pool, fx.orgID); n != 1 {
		t.Fatalf("feed_cards = %d, want 1", n)
	}
	var (
		cardType   string
		targetRole *string
		actionsRaw []byte
	)
	if err := fx.pool.QueryRow(ctx, `
		SELECT card_type, target_role, actions
		FROM feed_cards WHERE id = $1`, res.ReviewCard.ID).
		Scan(&cardType, &targetRole, &actionsRaw); err != nil {
		t.Fatalf("read feed_card: %v", err)
	}
	if cardType != "invoice_review" {
		t.Errorf("card_type = %q, want invoice_review", cardType)
	}
	if targetRole == nil || *targetRole != "admin" {
		t.Errorf("target_role = %v, want admin", targetRole)
	}
	// The actions JSONB must carry the created invoice id so the client can
	// deep-link the existing PUT /invoices/{id} status update.
	if !strings.Contains(string(actionsRaw), res.Invoice.ID.String()) {
		t.Errorf("card actions %s do not carry invoice id %s", actionsRaw, res.Invoice.ID)
	}

	// --- the outbox row: exactly one, links invoice + card ---------------
	if n := ingestOutboxCount(t, fx.pool, fx.projectID); n != 1 {
		t.Fatalf("invoice_ingestions = %d, want 1", n)
	}
	var (
		obInvoiceID uuid.UUID
		obCardID    uuid.UUID
	)
	if err := fx.pool.QueryRow(ctx, `
		SELECT invoice_id, feed_card_id
		FROM invoice_ingestions WHERE project_id = $1`, fx.projectID).
		Scan(&obInvoiceID, &obCardID); err != nil {
		t.Fatalf("read invoice_ingestions: %v", err)
	}
	if obInvoiceID != res.Invoice.ID {
		t.Errorf("outbox invoice_id = %s, want %s", obInvoiceID, res.Invoice.ID)
	}
	if obCardID != res.ReviewCard.ID {
		t.Errorf("outbox feed_card_id = %s, want %s", obCardID, res.ReviewCard.ID)
	}

	// --- the audit row: exactly one ingestion action on the real id ------
	if n := ingestAuditCount(t, fx.pool, fx.orgID); n != 1 {
		t.Fatalf("ingestion.invoice.extracted audit rows = %d, want 1", n)
	}
	var (
		auResource string
		auResID    uuid.UUID
	)
	if err := fx.pool.QueryRow(ctx, `
		SELECT resource_type, resource_id
		FROM audit_log WHERE org_id = $1 AND action = $2`,
		fx.orgID, "ingestion.invoice.extracted").
		Scan(&auResource, &auResID); err != nil {
		t.Fatalf("read audit_log: %v", err)
	}
	if auResource != string(AuditResourceInvoice) {
		t.Errorf("audit resource_type = %q, want %q", auResource, AuditResourceInvoice)
	}
	if auResID != res.Invoice.ID {
		t.Errorf("audit resource_id = %s, want invoice id %s", auResID, res.Invoice.ID)
	}
}

// TestIngestInvoice_Idempotent proves the dedupe anchor: two calls with the
// SAME idempotency_key — the second hits the UNIQUE (project_id,
// idempotency_key) and the whole tx rolls back (invoice + card never commit
// the second time), so exactly one of each row exists and the service
// surfaces store.ErrIdempotencyConflict.
func TestIngestInvoice_Idempotent(t *testing.T) {
	fx := newIngestFixture(t, &fakeIngestExtractor{resp: validIngestResp()})
	ctx := context.Background()
	key := uuid.New()

	if _, err := fx.svc.IngestInvoiceFromDocument(ctx, fx.orgID, fx.callerSub, IngestInvoiceInput{
		ProjectID:      fx.projectID,
		IdempotencyKey: key,
		Text:           "raw invoice text",
	}); err != nil {
		t.Fatalf("first ingest: %v", err)
	}

	_, err := fx.svc.IngestInvoiceFromDocument(ctx, fx.orgID, fx.callerSub, IngestInvoiceInput{
		ProjectID:      fx.projectID,
		IdempotencyKey: key,
		Text:           "raw invoice text",
	})
	if !errors.Is(err, store.ErrIdempotencyConflict) {
		t.Fatalf("second ingest (same key): err = %v, want store.ErrIdempotencyConflict", err)
	}

	// Exactly one of each — the second tx rolled back wholesale.
	if n := ingestInvoiceCount(t, fx.pool, fx.projectID); n != 1 {
		t.Errorf("invoices = %d, want 1 (replay must not duplicate)", n)
	}
	if n := ingestFeedCardCount(t, fx.pool, fx.orgID); n != 1 {
		t.Errorf("feed_cards = %d, want 1 (replay must not duplicate)", n)
	}
	if n := ingestOutboxCount(t, fx.pool, fx.projectID); n != 1 {
		t.Errorf("invoice_ingestions = %d, want 1 (replay must not duplicate)", n)
	}
	if n := ingestAuditCount(t, fx.pool, fx.orgID); n != 1 {
		t.Errorf("ingestion audit rows = %d, want 1 (replay must not duplicate)", n)
	}
}

// TestIngestInvoice_SoftFailNoKey proves graceful degradation: an
// IngestionService whose AI seam is nil (no Anthropic key wired) returns an
// error that errors.Is(ai.ErrUnconfigured) and writes ZERO rows — the
// soft-fail returns BEFORE any tx opens, so there is no partial state.
func TestIngestInvoice_SoftFailNoKey(t *testing.T) {
	// nil extractor → s.ai == nil → soft-fail leg, no tx opened.
	fx := newIngestFixture(t, nil)
	ctx := context.Background()

	_, err := fx.svc.IngestInvoiceFromDocument(ctx, fx.orgID, fx.callerSub, IngestInvoiceInput{
		ProjectID:      fx.projectID,
		IdempotencyKey: uuid.New(),
		Text:           "raw invoice text",
	})
	if !errors.Is(err, ai.ErrUnconfigured) {
		t.Fatalf("nil AI client: err = %v, want ai.ErrUnconfigured", err)
	}
	assertZeroPersisted(t, fx)
}

// TestIngestInvoice_RejectsBadFuzzyOutput proves the deterministic gate
// rejects untrusted AI output before any write: an empty vendor and a bad
// currency each return ErrInvoiceExtractionInvalid with ZERO rows persisted.
func TestIngestInvoice_RejectsBadFuzzyOutput(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*ai.InvoiceExtractResponse)
	}{
		{"empty vendor", func(r *ai.InvoiceExtractResponse) { r.VendorName = "" }},
		{"bad currency", func(r *ai.InvoiceExtractResponse) { r.CurrencyCode = "EUR" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := validIngestResp()
			c.mut(resp)
			fx := newIngestFixture(t, &fakeIngestExtractor{resp: resp})
			ctx := context.Background()

			_, err := fx.svc.IngestInvoiceFromDocument(ctx, fx.orgID, fx.callerSub, IngestInvoiceInput{
				ProjectID:      fx.projectID,
				IdempotencyKey: uuid.New(),
				Text:           "raw invoice text",
			})
			if !errors.Is(err, ErrInvoiceExtractionInvalid) {
				t.Fatalf("%s: err = %v, want ErrInvoiceExtractionInvalid", c.name, err)
			}
			assertZeroPersisted(t, fx)
		})
	}
}
