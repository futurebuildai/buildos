package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/currency"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// This file holds the Phase 2a invoice-ingestion pipeline: the fuzzy→exact
// transform that turns an unstructured invoice document into a real
// invoices row plus a human-review feed card.
//
// EXACT/FUZZY split (enforced on the tx boundary):
//   - FUZZY (outside any tx): the AI InvoiceExtract call. Untrusted output.
//   - EXACT (deterministic gate, then one write tx): validate EVERY
//     AI-sourced field — currency in {USD,CAD}, total > 0, vendor
//     non-empty, line-sum reconciliation (integer-only) — BEFORE any write.
//     The AI never reaches the database; deterministic code re-derives and
//     validates every value that lands in a NOT-NULL or money column.
//
// The invoice is created through BudgetService.createInvoiceTx so it shares
// the single money-validation chokepoint with the manual entry path (§7.5).
// The invoice lands status='pending' (table DEFAULT) — AI never
// auto-approves money.

// invoiceExtractorAI is the consumer-side seam over *ai.Client — one method
// wide, so tests inject a fake without an HTTP server. Mirrors
// cascadeReasonerAI in agentic.go. Deliberately defined HERE (not promoted
// to an agentic port) because 2a has exactly one consumer; the agentic-port
// promotion is the documented 2b lift.
type invoiceExtractorAI interface {
	InvoiceExtract(ctx context.Context, req ai.InvoiceExtractRequest) (*ai.InvoiceExtractResponse, error)
}

// ErrInvoiceExtractionInvalid is the sentinel for "the fuzzy AI output
// failed the deterministic gate" — bad currency, non-positive total,
// line-sum mismatch beyond tolerance, empty vendor, or a currency-override
// disagreement. The handler maps it to 422.
var ErrInvoiceExtractionInvalid = errors.New("service: invoice extraction failed validation")

// IngestionService orchestrates the fuzzy→exact invoice ingestion pipeline.
// It reuses BudgetService for the validated invoice create so all invoice
// invariants apply, and owns the single write tx that lands the invoice +
// review card + idempotency claim + audit atomically.
type IngestionService struct {
	pool      *pgxpool.Pool
	ai        invoiceExtractorAI // nil-safe (typed-nil guard in constructor) → behaves like ErrUnconfigured
	budget    *BudgetService
	ingStore  *store.InvoiceIngestionStore
	feedStore *store.FeedCardsStore
	audit     AuditRecorder
	// mismatchToleranceCents is the fork-configured slack between the line
	// sum and the declared total before the extraction is rejected. Default
	// 0 (exact). A within-tolerance mismatch persists the declared total and
	// bumps the review card to urgent.
	mismatchToleranceCents int64
}

// NewIngestionService wires the deps. It takes the concrete *ai.Client (not
// the invoiceExtractorAI interface) precisely to dodge the typed-nil
// interface hazard: a nil *ai.Client is left out of the interface field, so
// the s.ai == nil guard in IngestInvoiceFromDocument fires and the pipeline
// soft-fails with ai.ErrUnconfigured. Mirrors NewCascadeReasoner.
//
// A nil AuditRecorder falls back to the no-op recorder.
func NewIngestionService(
	pool *pgxpool.Pool,
	client *ai.Client,
	budget *BudgetService,
	ing *store.InvoiceIngestionStore,
	feed *store.FeedCardsStore,
	audit AuditRecorder,
) *IngestionService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	s := &IngestionService{
		pool:      pool,
		budget:    budget,
		ingStore:  ing,
		feedStore: feed,
		audit:     audit,
	}
	// Assign only a non-nil client. Storing a nil *ai.Client straight into
	// the invoiceExtractorAI interface field would make s.ai a non-nil
	// interface wrapping a nil pointer, defeating the s.ai == nil guard.
	if client != nil {
		s.ai = client
	}
	return s
}

// IngestInvoiceInput is what the handler hands the service. DocumentURL XOR
// Text — the handler enforces exactly one is set.
type IngestInvoiceInput struct {
	ProjectID        uuid.UUID
	IdempotencyKey   uuid.UUID // client-supplied; the dedupe anchor
	DocumentURL      string    // signed URL to a jpeg/png/gif/webp image
	Text             string    // OR raw text; exactly one of the two
	CurrencyOverride *string   // optional operator override; if set, AI currency must match or 422
	WBSCode          *string   // optional cost-code hint; persisted as-is, NEVER fed to AI math
}

// IngestInvoiceResult is the success payload — the persisted invoice and
// the review card the handler renders.
type IngestInvoiceResult struct {
	Invoice    models.Invoice
	ReviewCard models.FeedCard
}

// extractionVerdict is the deterministic gate's output: the validated
// values that will be persisted (never raw AI fields copied through) plus
// the audit/priority signals. Computed by validateExtraction BEFORE any tx
// opens, so the validation matrix is unit-testable with no DB.
type extractionVerdict struct {
	vendorName     string
	invoiceNumber  *string
	amountCents    int64 // always the validated TotalCents — never an AI sum
	currencyCode   string
	lineItemCount  int
	lineSumCents   int64
	totalMismatch  bool // line sum != declared total but within tolerance
	priority       string
	issuedUnparsed bool
}

// validateExtraction runs the deterministic gate over the fuzzy AI output
// (§7). It validates currency, total, vendor, the optional currency
// override, and the line-item reconciliation — all integer-only, no
// float64. Any hard failure returns ErrInvoiceExtractionInvalid. On success
// it returns the validated values to persist (the persisted amount is
// always the validated TotalCents) plus the review-card priority and audit
// flags.
func (s *IngestionService) validateExtraction(resp *ai.InvoiceExtractResponse, override *string) (extractionVerdict, error) {
	var v extractionVerdict
	if resp == nil {
		return extractionVerdict{}, fmt.Errorf("%w: nil extraction response", ErrInvoiceExtractionInvalid)
	}

	// §7.1 Currency — must be USD/CAD. AI field is invoice_currency_code
	// (mapped to Go CurrencyCode). If an operator override is set, the AI
	// value must equal it (no silent coercion, no cross-currency math).
	if err := currency.Validate(resp.CurrencyCode); err != nil {
		return extractionVerdict{}, fmt.Errorf("%w: currency_code %q: %v", ErrInvoiceExtractionInvalid, resp.CurrencyCode, err)
	}
	if override != nil && *override != resp.CurrencyCode {
		return extractionVerdict{}, fmt.Errorf("%w: currency override %q != extracted %q", ErrInvoiceExtractionInvalid, *override, resp.CurrencyCode)
	}
	v.currencyCode = resp.CurrencyCode

	// §7.2 Total — must be strictly positive (pure int64 comparison).
	if resp.TotalCents <= 0 {
		return extractionVerdict{}, fmt.Errorf("%w: total_cents must be positive, got %d", ErrInvoiceExtractionInvalid, resp.TotalCents)
	}
	// The persisted amount is ALWAYS the validated declared total, never an
	// AI sum copied through.
	v.amountCents = resp.TotalCents

	// §7.4 Vendor — must be non-empty (column is TEXT NOT NULL; "" would
	// create garbage). Mirrors the manual path's vendor_name guard.
	if resp.VendorName == "" {
		return extractionVerdict{}, fmt.Errorf("%w: vendor_name is required", ErrInvoiceExtractionInvalid)
	}
	v.vendorName = resp.VendorName

	// §7.4 InvoiceNo — nullable; empty → store NULL.
	if resp.InvoiceNo != "" {
		num := resp.InvoiceNo
		v.invoiceNumber = &num
	}

	// §7.4 IssuedDate — the invoices table has NO issued_date column; 2a
	// discards it (documented data loss). We only note an unparseable value
	// in audit metadata; we NEVER map issued→due.
	if resp.IssuedDate != "" {
		if _, perr := time.Parse(cascadeWireDateLayout, resp.IssuedDate); perr != nil {
			v.issuedUnparsed = true
		}
	}

	// §7.3 Line-item reconciliation (deterministic, integer-only). Fold the
	// line AmountCents with currency.Money.Add (which raises ErrCrossCurrency
	// on a mixed-currency fold) — NOT SumByCurrency. Each line inherits the
	// invoice's validated currency. Only AmountCents is summed (quantity ×
	// unit is AI arithmetic and the schema makes them optional). Empty lines
	// → trust the declared total (real invoices have tax/shipping lines the
	// model may not fold).
	v.lineItemCount = len(resp.LineItems)
	v.priority = models.FeedPriorityNormal
	if v.lineItemCount > 0 {
		sum, err := currency.Zero(v.currencyCode)
		if err != nil {
			// currencyCode already validated above; unreachable in practice.
			return extractionVerdict{}, fmt.Errorf("%w: %v", ErrInvoiceExtractionInvalid, err)
		}
		for _, li := range resp.LineItems {
			next, addErr := sum.Add(currency.Money{Cents: li.AmountCents, CurrencyCode: v.currencyCode})
			if addErr != nil {
				return extractionVerdict{}, fmt.Errorf("%w: line-item reconciliation: %v", ErrInvoiceExtractionInvalid, addErr)
			}
			sum = next
		}
		v.lineSumCents = sum.Cents

		diff := v.lineSumCents - resp.TotalCents
		if diff < 0 {
			diff = -diff
		}
		if diff != 0 {
			if diff > s.mismatchToleranceCents {
				// Beyond tolerance → reject; nothing persisted.
				return extractionVerdict{}, fmt.Errorf("%w: line sum %d != total %d (beyond tolerance %d)", ErrInvoiceExtractionInvalid, v.lineSumCents, resp.TotalCents, s.mismatchToleranceCents)
			}
			// Within tolerance → persist the declared total, flag the
			// mismatch, and bump the review card to urgent.
			v.totalMismatch = true
			v.priority = models.FeedPriorityUrgent
		}
	}

	return v, nil
}

// IngestInvoiceFromDocument runs the full pipeline: fuzzy AI extract
// (outside any tx), the deterministic validation gate (§7), then ONE write
// tx that lands the invoice + review card + idempotency claim + audit
// atomically (§3 steps 6–10).
//
// Soft-fail: a nil AI client or ai.ErrUnconfigured returns ai.ErrUnconfigured
// (handler → 503) with NOTHING written — no tx is even opened.
func (s *IngestionService) IngestInvoiceFromDocument(
	ctx context.Context,
	callerOrgID uuid.UUID,
	callerUserSub string,
	in IngestInvoiceInput,
) (IngestInvoiceResult, error) {
	// ---- FUZZY: AI extract (no tx open, no DB locks held) ----
	if s.ai == nil {
		// No AI wiring → graceful degradation, not an error condition for
		// the deployment. Surface ErrUnconfigured for the handler's 503.
		return IngestInvoiceResult{}, fmt.Errorf("invoice ingest: ai client not configured: %w", ai.ErrUnconfigured)
	}

	aiCtx := ai.ContextWithOrgID(ctx, callerOrgID.String())
	resp, err := s.ai.InvoiceExtract(aiCtx, ai.InvoiceExtractRequest{
		DocumentURL: in.DocumentURL,
		Text:        in.Text,
	})
	if err != nil {
		if errors.Is(err, ai.ErrUnconfigured) {
			// No Anthropic key for this org → soft-fail to 503. Nothing
			// written; no tx to roll back.
			return IngestInvoiceResult{}, fmt.Errorf("invoice ingest: %w", ai.ErrUnconfigured)
		}
		if errors.Is(err, ai.ErrUnsupportedMediaType) || errors.Is(err, ai.ErrImageTooLarge) {
			// Bad input document → 422 (treated as invalid extraction).
			return IngestInvoiceResult{}, fmt.Errorf("invoice ingest: %w: %v", ErrInvoiceExtractionInvalid, err)
		}
		// Transport error (timeout, 5xx, decode) → propagate for 502/500.
		return IngestInvoiceResult{}, fmt.Errorf("invoice ingest: ai invoice_extract: %w", err)
	}

	// ---- EXACT: deterministic validation gate (no DB write yet) ----
	verdict, err := s.validateExtraction(resp, in.CurrencyOverride)
	if err != nil {
		return IngestInvoiceResult{}, err
	}

	// ---- ONE write tx: invoice + review card + idempotency claim + audit ----
	var result IngestInvoiceResult
	txErr := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// 8.2 Create the invoice through the shared validated chokepoint
		// (§7.5(a)). createInvoiceTx re-applies currency/vendor/amount
		// invariants and runs VerifyProjectInOrg (cross-tenant guard). The
		// invoice lands status='pending' via table DEFAULT.
		src := "ai_ingest"
		inv, err := s.budget.createInvoiceTx(ctx, tx, callerOrgID, callerUserSub, CreateInvoiceInput{
			ProjectID:     in.ProjectID,
			VendorName:    verdict.vendorName,
			InvoiceNumber: verdict.invoiceNumber,
			AmountCents:   verdict.amountCents,
			CurrencyCode:  verdict.currencyCode,
			WBSCode:       in.WBSCode,
			Source:        &src,
		})
		if err != nil {
			return err
		}

		// 8.3 Create the review feed card. OrgID is mandatory (non-pointer,
		// NOT NULL). TargetRole admin; approve/reject actions deep-link the
		// invoice id so the existing PUT /invoices/{id} status update actions it.
		pid := in.ProjectID
		targetRole := "admin"
		actions := marshalAudit([]invoiceReviewAction{
			{Label: "Approve invoice", ActionType: "approve_invoice", Payload: invoiceReviewActionPayload{InvoiceID: inv.ID}},
			{Label: "Reject invoice", ActionType: "reject_invoice", Payload: invoiceReviewActionPayload{InvoiceID: inv.ID}},
		})
		card, err := s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
			OrgID:      callerOrgID,
			ProjectID:  &pid,
			CardType:   "invoice_review",
			Title:      fmt.Sprintf("Review invoice from %s%s", verdict.vendorName, invoiceNoSuffix(verdict.invoiceNumber)),
			Body:       invoiceReviewBody(verdict),
			Priority:   verdict.priority,
			TargetRole: &targetRole,
			Actions:    actions,
		})
		if err != nil {
			return fmt.Errorf("create review feed card: %w", err)
		}

		// 8.4 Claim the idempotency slot LAST. This is the dedupe
		// enforcement point: on conflict the whole tx (invoice + card)
		// rolls back. Because the conflicting INSERT is the last statement,
		// the tx is not used again — no 25P02 poisoning, no in-tx read-back.
		var extractedBy uuid.UUID
		if uid, perr := uuid.Parse(callerUserSub); perr == nil {
			extractedBy = uid
		}
		if err := s.ingStore.InsertInvoiceIngestion(ctx, tx, store.InsertInvoiceIngestionParams{
			ProjectID:      in.ProjectID,
			OrgID:          callerOrgID,
			IdempotencyKey: in.IdempotencyKey,
			InvoiceID:      inv.ID,
			FeedCardID:     card.ID,
			ExtractedBy:    extractedBy,
		}); err != nil {
			// store.ErrIdempotencyConflict bubbles up; the tx rolls back.
			return err
		}

		// 8.5 Audit (real inv.ID so audit.Record does not skip it).
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        callerOrgID,
			UserSub:      callerUserSub,
			Action:       "ingestion.invoice.extracted",
			ResourceType: AuditResourceInvoice,
			ResourceID:   inv.ID,
			After:        marshalAudit(inv),
			Metadata: marshalAudit(map[string]any{
				"vendor":                verdict.vendorName,
				"ai_total_cents":        resp.TotalCents,
				"persisted_total_cents": verdict.amountCents,
				"currency":              verdict.currencyCode,
				"line_item_count":       verdict.lineItemCount,
				"line_sum_cents":        verdict.lineSumCents,
				"total_mismatch":        verdict.totalMismatch,
				"issued_date_unparsed":  verdict.issuedUnparsed,
				"source":                "ai_ingest",
				"idempotency_key":       in.IdempotencyKey,
			}),
		})

		result.Invoice = inv
		result.ReviewCard = card
		return nil
	})
	if txErr != nil {
		// ErrIdempotencyConflict surfaces unchanged for the handler's 409.
		if errors.Is(txErr, store.ErrIdempotencyConflict) {
			return IngestInvoiceResult{}, store.ErrIdempotencyConflict
		}
		return IngestInvoiceResult{}, mapStoreError(txErr)
	}
	return result, nil
}

// invoiceReviewAction is one feed-card action carrying a review next step.
// Matches the feed_cards.actions JSONB shape ([{label, action_type,
// payload}]) the existing producers use.
type invoiceReviewAction struct {
	Label      string                     `json:"label"`
	ActionType string                     `json:"action_type"`
	Payload    invoiceReviewActionPayload `json:"payload"`
}

// invoiceReviewActionPayload carries the invoice id so the client can
// deep-link the existing PUT /invoices/{invoiceID} status update.
type invoiceReviewActionPayload struct {
	InvoiceID uuid.UUID `json:"invoice_id"`
}

// invoiceNoSuffix renders ": <invoice_no>" when present, else "".
func invoiceNoSuffix(invoiceNumber *string) string {
	if invoiceNumber == nil || *invoiceNumber == "" {
		return ""
	}
	return ": " + *invoiceNumber
}

// invoiceReviewBody renders the human summary for the review card. Money is
// formatted via formatMoney → the existing integer-only formatCents helper;
// NO float64 anywhere (§7.7).
func invoiceReviewBody(v extractionVerdict) string {
	body := fmt.Sprintf("AI-extracted invoice for %s. Amount: %s. Review and approve or reject.",
		v.vendorName, formatMoney(v.amountCents, v.currencyCode))
	if v.totalMismatch {
		body += fmt.Sprintf(" NOTE: line items sum to %s, which does not match the declared total — verify before approving.",
			formatMoney(v.lineSumCents, v.currencyCode))
	}
	return body
}

// formatMoney renders an integer-cents amount as "<CODE> <dollars>.<cc>"
// (e.g. "USD 1500.00"), reusing the package's integer-only formatCents so
// there is exactly one money-formatting implementation. NEVER float64 (§7.7).
func formatMoney(cents int64, code string) string {
	return code + " " + formatCents(cents)
}
