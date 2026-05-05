package a2a

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/futurebuildai/buildos/internal/worker"
)

// fakeEnqueuer captures the last InsertTx call without touching River.
// Lets the emitter tests assert envelope shape and event_type without
// spinning up Postgres or a real River client.
type fakeEnqueuer struct {
	calls    int
	lastCtx  context.Context
	lastTx   pgx.Tx
	lastArgs river.JobArgs
	lastOpts *river.InsertOpts
	err      error
}

func (f *fakeEnqueuer) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	f.calls++
	f.lastCtx = ctx
	f.lastTx = tx
	f.lastArgs = args
	f.lastOpts = opts
	if f.err != nil {
		return nil, f.err
	}
	return &rivertype.JobInsertResult{}, nil
}

func TestNewEmitter_PanicsOnNilEnqueuer(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil enqueuer, got none")
		}
	}()
	_ = NewEmitter(nil)
}

func TestEmitReviewMaterialQuote_RejectsBadInput(t *testing.T) {
	good := ReviewMaterialQuoteArgs{
		OrgID:             uuid.New(),
		ProcurementItemID: uuid.New(),
		Vendor:            "Acme Materials",
		TotalCents:        125000,
		CurrencyCode:      "USD",
	}
	cases := []struct {
		name string
		mut  func(*ReviewMaterialQuoteArgs)
	}{
		{"nil org", func(a *ReviewMaterialQuoteArgs) { a.OrgID = uuid.Nil }},
		{"nil item", func(a *ReviewMaterialQuoteArgs) { a.ProcurementItemID = uuid.Nil }},
		{"empty vendor", func(a *ReviewMaterialQuoteArgs) { a.Vendor = "  " }},
		{"negative total", func(a *ReviewMaterialQuoteArgs) { a.TotalCents = -1 }},
		{"empty currency", func(a *ReviewMaterialQuoteArgs) { a.CurrencyCode = "" }},
		{"unsupported currency", func(a *ReviewMaterialQuoteArgs) { a.CurrencyCode = "EUR" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeEnqueuer{}
			emitter := NewEmitter(fake)
			args := good
			c.mut(&args)
			_, err := emitter.EmitReviewMaterialQuote(context.Background(), nil, args)
			if !errors.Is(err, ErrInvalidArgs) {
				t.Errorf("err = %v, want ErrInvalidArgs", err)
			}
			if fake.calls != 0 {
				t.Errorf("validation failure must not enqueue (calls = %d)", fake.calls)
			}
		})
	}
}

func TestEmitReviewMaterialQuote_BuildsCorrectEnvelope(t *testing.T) {
	fake := &fakeEnqueuer{}
	emitter := NewEmitter(fake)

	orgID := uuid.New()
	itemID := uuid.New()
	rfqID := uuid.New()

	idem, err := emitter.EmitReviewMaterialQuote(context.Background(), nil, ReviewMaterialQuoteArgs{
		OrgID:             orgID,
		ProcurementItemID: itemID,
		RFQID:             rfqID,
		Vendor:            "Acme Materials",
		TotalCents:        125000,
		CurrencyCode:      "USD",
		Reasoning:         "lowest predicted spend",
		TraceID:           "trace-123",
	})
	if err != nil {
		t.Fatalf("EmitReviewMaterialQuote: %v", err)
	}
	if idem == uuid.Nil {
		t.Fatal("idempotency key must be non-nil")
	}
	if fake.calls != 1 {
		t.Fatalf("calls = %d, want 1", fake.calls)
	}

	dispatch, ok := fake.lastArgs.(worker.A2AWebhookDispatchArgs)
	if !ok {
		t.Fatalf("InsertTx args = %T, want worker.A2AWebhookDispatchArgs", fake.lastArgs)
	}
	if dispatch.EventType != EventReviewMaterialQuote {
		t.Errorf("event_type = %q, want %q", dispatch.EventType, EventReviewMaterialQuote)
	}
	if dispatch.OrgID != orgID {
		t.Errorf("org_id = %v, want %v", dispatch.OrgID, orgID)
	}
	if dispatch.IdempotencyKey != idem {
		t.Errorf("dispatch idem = %v, want %v", dispatch.IdempotencyKey, idem)
	}
	if dispatch.TraceID != "trace-123" {
		t.Errorf("trace_id = %q, want %q", dispatch.TraceID, "trace-123")
	}

	var got reviewMaterialQuotePayload
	if err := json.Unmarshal(dispatch.Payload, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	want := reviewMaterialQuotePayload{
		OrgID:             orgID,
		ProcurementItemID: itemID,
		RFQID:             &rfqID,
		Vendor:            "Acme Materials",
		TotalCents:        125000,
		CurrencyCode:      "USD",
		Reasoning:         "lowest predicted spend",
	}
	// reflect.DeepEqual rather than == because RFQID is *uuid.UUID and
	// struct equality compares pointer addresses, not pointed-to values.
	if !reflect.DeepEqual(got, want) {
		t.Errorf("payload mismatch:\n got = %+v\n want = %+v", got, want)
	}
}

func TestEmitReviewMaterialQuote_OmitsEmptyOptionals(t *testing.T) {
	// rfq_id (uuid.Nil) and reasoning (empty) must be omitted from
	// the wire JSON via the omitempty tag — Brain's dedup/ETL needs
	// to distinguish "absent" from "nil-uuid".
	fake := &fakeEnqueuer{}
	emitter := NewEmitter(fake)
	_, err := emitter.EmitReviewMaterialQuote(context.Background(), nil, ReviewMaterialQuoteArgs{
		OrgID:             uuid.New(),
		ProcurementItemID: uuid.New(),
		Vendor:            "Acme Materials",
		TotalCents:        50000,
		CurrencyCode:      "CAD",
	})
	if err != nil {
		t.Fatalf("EmitReviewMaterialQuote: %v", err)
	}
	dispatch := fake.lastArgs.(worker.A2AWebhookDispatchArgs)
	var raw map[string]any
	if err := json.Unmarshal(dispatch.Payload, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := raw["rfq_id"]; present {
		t.Errorf("rfq_id must be omitted when uuid.Nil; payload = %s", string(dispatch.Payload))
	}
	if _, present := raw["reasoning"]; present {
		t.Errorf("reasoning must be omitted when empty; payload = %s", string(dispatch.Payload))
	}
	if got := raw["currency_code"]; got != "CAD" {
		t.Errorf("currency_code = %v, want CAD", got)
	}
}

func TestEmitReviewMaterialQuote_PropagatesEnqueueError(t *testing.T) {
	fake := &fakeEnqueuer{err: errors.New("river closed")}
	emitter := NewEmitter(fake)
	_, err := emitter.EmitReviewMaterialQuote(context.Background(), nil, ReviewMaterialQuoteArgs{
		OrgID:             uuid.New(),
		ProcurementItemID: uuid.New(),
		Vendor:            "Acme",
		TotalCents:        1,
		CurrencyCode:      "USD",
	})
	if err == nil {
		t.Fatal("expected enqueue error to propagate")
	}
	if errors.Is(err, ErrInvalidArgs) {
		t.Errorf("enqueue error must not be wrapped as ErrInvalidArgs: %v", err)
	}
}

func TestEmitReviewLaborBid_RejectsBadInput(t *testing.T) {
	good := ReviewLaborBidArgs{
		OrgID:        uuid.New(),
		ProjectID:    uuid.New(),
		Bidder:       "Acme Framing",
		AmountCents:  500000,
		CurrencyCode: "USD",
	}
	cases := []struct {
		name string
		mut  func(*ReviewLaborBidArgs)
	}{
		{"nil org", func(a *ReviewLaborBidArgs) { a.OrgID = uuid.Nil }},
		{"nil project", func(a *ReviewLaborBidArgs) { a.ProjectID = uuid.Nil }},
		{"empty bidder", func(a *ReviewLaborBidArgs) { a.Bidder = "" }},
		{"negative amount", func(a *ReviewLaborBidArgs) { a.AmountCents = -1 }},
		{"unsupported currency", func(a *ReviewLaborBidArgs) { a.CurrencyCode = "GBP" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fake := &fakeEnqueuer{}
			emitter := NewEmitter(fake)
			args := good
			c.mut(&args)
			_, err := emitter.EmitReviewLaborBid(context.Background(), nil, args)
			if !errors.Is(err, ErrInvalidArgs) {
				t.Errorf("err = %v, want ErrInvalidArgs", err)
			}
			if fake.calls != 0 {
				t.Errorf("validation failure must not enqueue (calls = %d)", fake.calls)
			}
		})
	}
}

func TestEmitReviewLaborBid_BuildsCorrectEnvelope(t *testing.T) {
	fake := &fakeEnqueuer{}
	emitter := NewEmitter(fake)

	orgID := uuid.New()
	projectID := uuid.New()

	idem, err := emitter.EmitReviewLaborBid(context.Background(), nil, ReviewLaborBidArgs{
		OrgID:        orgID,
		ProjectID:    projectID,
		Bidder:       "Acme Framing",
		AmountCents:  500000,
		CurrencyCode: "USD",
		Timeline:     "starts 2026-06-01, 4 weeks",
	})
	if err != nil {
		t.Fatalf("EmitReviewLaborBid: %v", err)
	}

	dispatch := fake.lastArgs.(worker.A2AWebhookDispatchArgs)
	if dispatch.EventType != EventReviewLaborBid {
		t.Errorf("event_type = %q, want %q", dispatch.EventType, EventReviewLaborBid)
	}
	if dispatch.IdempotencyKey != idem {
		t.Errorf("dispatch idem = %v, want %v", dispatch.IdempotencyKey, idem)
	}

	var got reviewLaborBidPayload
	if err := json.Unmarshal(dispatch.Payload, &got); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	want := reviewLaborBidPayload{
		OrgID:        orgID,
		ProjectID:    projectID,
		Bidder:       "Acme Framing",
		AmountCents:  500000,
		CurrencyCode: "USD",
		Timeline:     "starts 2026-06-01, 4 weeks",
	}
	if got != want {
		t.Errorf("payload mismatch:\n got = %+v\n want = %+v", got, want)
	}
}

func TestEmitReviewLaborBid_OmitsEmptyTimeline(t *testing.T) {
	fake := &fakeEnqueuer{}
	emitter := NewEmitter(fake)
	_, err := emitter.EmitReviewLaborBid(context.Background(), nil, ReviewLaborBidArgs{
		OrgID:        uuid.New(),
		ProjectID:    uuid.New(),
		Bidder:       "Acme",
		AmountCents:  1,
		CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("EmitReviewLaborBid: %v", err)
	}
	dispatch := fake.lastArgs.(worker.A2AWebhookDispatchArgs)
	var raw map[string]any
	if err := json.Unmarshal(dispatch.Payload, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := raw["timeline"]; present {
		t.Errorf("timeline must be omitted when empty; payload = %s", string(dispatch.Payload))
	}
}

// Compile-time assertion that *river.Client[pgx.Tx] satisfies the
// Enqueuer interface. If River's signature drifts this test file
// stops compiling — louder than a runtime nil dereference at the
// first queued event.
var _ Enqueuer = (*river.Client[pgx.Tx])(nil)
