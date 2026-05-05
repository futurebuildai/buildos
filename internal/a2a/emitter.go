// Package a2a provides the typed-payload emitter for outbound A2A
// (App-to-App) events sent from BuildOS to The Brain. Each Emit
// method builds a typed JSON payload, wraps it in a
// worker.A2AWebhookDispatchArgs, and InsertTx-es it on the supplied
// pgx.Tx so the enqueue commits or rolls back with the surrounding
// domain mutation.
//
// The actual HTTP delivery + JWS signing happens later in the River
// worker via service.A2AOutboundService.DeliverA2AWebhook. The
// emitter's only job is to produce the right envelope shape and
// queue it durably.
//
// Design choice: this package depends on `worker` (for the dispatch
// args struct) and `river` (for the InsertTx interface) but
// intentionally NOT on `internal/service`. Domain services
// (procurement, fleet, schedule) import this package to enqueue
// outbound events without forming an import cycle through service.
package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/futurebuildai/buildos/internal/worker"
)

// Event type constants for outbound A2A events. These are the
// wire-protocol identifiers Brain expects on its receiver, mirroring
// the inbound dispatch constants in service/a2a.go. Duplicated here
// (rather than imported) so the emitter doesn't take a service-layer
// dependency — emitter is meant to be importable by every domain
// service without tangling the import graph.
const (
	EventReviewMaterialQuote = "review_material_quote"
	EventReviewLaborBid      = "review_labor_bid"
)

// ErrInvalidArgs is returned when an Emit method is called with
// invalid input. Wrapped with a descriptive message so callers can
// surface a precise cause; errors.Is(err, ErrInvalidArgs) tests for
// the class.
var ErrInvalidArgs = errors.New("a2a: invalid args")

// Enqueuer is the consumer-side interface Emitter needs from a
// River client. *river.Client[pgx.Tx] satisfies this in production;
// tests substitute a fake that captures the dispatched args. Defined
// here rather than in worker so this package doesn't pull worker's
// full registry surface.
type Enqueuer interface {
	InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Emitter is the typed outbound-event surface. One per process is
// fine — the struct holds only the Enqueuer reference and is
// goroutine-safe to the same extent the underlying River client is.
type Emitter struct {
	enq Enqueuer
}

// NewEmitter wires an Emitter to a River client (or test fake).
// Panics on nil so wiring bugs surface at startup, not at the first
// queued event.
func NewEmitter(enq Enqueuer) *Emitter {
	if enq == nil {
		panic("a2a: NewEmitter requires non-nil Enqueuer")
	}
	return &Emitter{enq: enq}
}

// ReviewMaterialQuoteArgs is the input shape for
// EmitReviewMaterialQuote. ProcurementItemID ties the quote back to
// the procurement_items row this quote responds to; RFQID is
// optional (Maestro recommendations may produce unsolicited quotes
// not tied to a formal RFQ). Reasoning is the optional human-readable
// explanation Maestro emitted alongside the recommendation.
type ReviewMaterialQuoteArgs struct {
	OrgID             uuid.UUID
	ProcurementItemID uuid.UUID
	RFQID             uuid.UUID // optional — uuid.Nil when no formal RFQ
	Vendor            string
	TotalCents        int64
	CurrencyCode      string // USD | CAD
	Reasoning         string // optional Maestro narrative
	TraceID           string // optional — propagates the inbound request's trace
}

// reviewMaterialQuotePayload is the JSON shape sent to Brain. The
// json tag layout is the wire-protocol contract — renames here are
// breaking changes coordinated cross-repo. RFQID + Reasoning are
// pointer / string omitempty so absent values don't show on the
// wire. (uuid.UUID itself doesn't honor omitempty at the
// encoding/json layer because [16]byte is never "empty"; using
// *uuid.UUID is the canonical fix.)
type reviewMaterialQuotePayload struct {
	OrgID             uuid.UUID  `json:"org_id"`
	ProcurementItemID uuid.UUID  `json:"procurement_item_id"`
	RFQID             *uuid.UUID `json:"rfq_id,omitempty"`
	Vendor            string     `json:"vendor"`
	TotalCents        int64      `json:"total_cents"`
	CurrencyCode      string     `json:"currency_code"`
	Reasoning         string     `json:"reasoning,omitempty"`
}

// EmitReviewMaterialQuote validates the args, marshals the typed
// payload, and InsertTx-es a worker.A2AWebhookDispatchArgs on the
// supplied tx. A fresh idempotency_key is minted per call —
// duplicate-suppression is Brain's responsibility on its receiver
// (matching the inbound contract from BuildOS's side; see
// store.A2AStore.InsertInboundLog).
//
// Validation is up-front: org_id, procurement_item_id, vendor must
// be non-empty; total_cents must be non-negative; currency_code
// must be USD or CAD. Validation errors wrap ErrInvalidArgs and do
// NOT touch the tx, so callers can rely on "validation fail = no
// queue mutation".
func (e *Emitter) EmitReviewMaterialQuote(ctx context.Context, tx pgx.Tx, args ReviewMaterialQuoteArgs) (uuid.UUID, error) {
	if args.OrgID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: org_id is required", ErrInvalidArgs)
	}
	if args.ProcurementItemID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: procurement_item_id is required", ErrInvalidArgs)
	}
	if strings.TrimSpace(args.Vendor) == "" {
		return uuid.Nil, fmt.Errorf("%w: vendor is required", ErrInvalidArgs)
	}
	if args.TotalCents < 0 {
		return uuid.Nil, fmt.Errorf("%w: total_cents must be non-negative", ErrInvalidArgs)
	}
	if !isSupportedCurrency(args.CurrencyCode) {
		return uuid.Nil, fmt.Errorf("%w: unsupported currency_code %q", ErrInvalidArgs, args.CurrencyCode)
	}

	var rfqIDPtr *uuid.UUID
	if args.RFQID != uuid.Nil {
		v := args.RFQID
		rfqIDPtr = &v
	}
	payload, err := json.Marshal(reviewMaterialQuotePayload{
		OrgID:             args.OrgID,
		ProcurementItemID: args.ProcurementItemID,
		RFQID:             rfqIDPtr,
		Vendor:            strings.TrimSpace(args.Vendor),
		TotalCents:        args.TotalCents,
		CurrencyCode:      args.CurrencyCode,
		Reasoning:         strings.TrimSpace(args.Reasoning),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("a2a: marshal review_material_quote payload: %w", err)
	}

	idempotencyKey := uuid.New()
	if _, err := e.enq.InsertTx(ctx, tx, worker.A2AWebhookDispatchArgs{
		OrgID:          args.OrgID,
		EventType:      EventReviewMaterialQuote,
		Payload:        payload,
		TraceID:        args.TraceID,
		IdempotencyKey: idempotencyKey,
	}, nil); err != nil {
		return uuid.Nil, fmt.Errorf("a2a: enqueue review_material_quote: %w", err)
	}
	return idempotencyKey, nil
}

// ReviewLaborBidArgs is the input shape for EmitReviewLaborBid.
// Timeline is the bidder's stated start/finish window as free-form
// text (e.g. "starts 2026-06-01, 4 weeks"); Brain's review task
// parses it into structured constraints if needed.
type ReviewLaborBidArgs struct {
	OrgID        uuid.UUID
	ProjectID    uuid.UUID
	RFQID        uuid.UUID // optional — uuid.Nil when no formal RFQ
	Bidder       string
	AmountCents  int64
	CurrencyCode string // USD | CAD
	Timeline     string // optional — free-form schedule
	TraceID      string // optional
}

// reviewLaborBidPayload is the JSON shape sent to Brain. RFQID is
// *uuid.UUID so omitempty actually drops the field — uuid.UUID is
// [16]byte and never satisfies json's "empty" check.
type reviewLaborBidPayload struct {
	OrgID        uuid.UUID  `json:"org_id"`
	ProjectID    uuid.UUID  `json:"project_id"`
	RFQID        *uuid.UUID `json:"rfq_id,omitempty"`
	Bidder       string     `json:"bidder"`
	AmountCents  int64      `json:"amount_cents"`
	CurrencyCode string     `json:"currency_code"`
	Timeline     string     `json:"timeline,omitempty"`
}

// EmitReviewLaborBid mirrors EmitReviewMaterialQuote: validate,
// marshal typed payload, enqueue.
func (e *Emitter) EmitReviewLaborBid(ctx context.Context, tx pgx.Tx, args ReviewLaborBidArgs) (uuid.UUID, error) {
	if args.OrgID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: org_id is required", ErrInvalidArgs)
	}
	if args.ProjectID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: project_id is required", ErrInvalidArgs)
	}
	if strings.TrimSpace(args.Bidder) == "" {
		return uuid.Nil, fmt.Errorf("%w: bidder is required", ErrInvalidArgs)
	}
	if args.AmountCents < 0 {
		return uuid.Nil, fmt.Errorf("%w: amount_cents must be non-negative", ErrInvalidArgs)
	}
	if !isSupportedCurrency(args.CurrencyCode) {
		return uuid.Nil, fmt.Errorf("%w: unsupported currency_code %q", ErrInvalidArgs, args.CurrencyCode)
	}

	var rfqIDPtr *uuid.UUID
	if args.RFQID != uuid.Nil {
		v := args.RFQID
		rfqIDPtr = &v
	}
	payload, err := json.Marshal(reviewLaborBidPayload{
		OrgID:        args.OrgID,
		ProjectID:    args.ProjectID,
		RFQID:        rfqIDPtr,
		Bidder:       strings.TrimSpace(args.Bidder),
		AmountCents:  args.AmountCents,
		CurrencyCode: args.CurrencyCode,
		Timeline:     strings.TrimSpace(args.Timeline),
	})
	if err != nil {
		return uuid.Nil, fmt.Errorf("a2a: marshal review_labor_bid payload: %w", err)
	}

	idempotencyKey := uuid.New()
	if _, err := e.enq.InsertTx(ctx, tx, worker.A2AWebhookDispatchArgs{
		OrgID:          args.OrgID,
		EventType:      EventReviewLaborBid,
		Payload:        payload,
		TraceID:        args.TraceID,
		IdempotencyKey: idempotencyKey,
	}, nil); err != nil {
		return uuid.Nil, fmt.Errorf("a2a: enqueue review_labor_bid: %w", err)
	}
	return idempotencyKey, nil
}

// isSupportedCurrency mirrors the codebase-wide USD/CAD-only policy.
// Kept package-private to avoid drift with internal/currency; the
// emitter package is small enough that a one-liner check beats
// taking a dependency for two values.
func isSupportedCurrency(c string) bool {
	return c == "USD" || c == "CAD"
}
