package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
)

// These tests cover the input-validation gates that run BEFORE
// ProcurementService touches the database pool. Passing nil pool/store
// proves the gates are effective — any post-validation code path would
// panic.

func newProcurementSvcForValidationTests() *ProcurementService {
	// nil audit falls back to a no-op recorder; nil pool/store
	// causes any post-validation path to panic, which proves the
	// validation gates short-circuit before touching either.
	// nil recommender / nil feed store are intentional — RecommendVendors
	// and RequestVendorReview short-circuit with their respective
	// "unavailable" sentinels rather than panicking.
	return NewProcurementService(nil, nil, nil, nil, nil)
}

func ptrString(s string) *string { return &s }

func TestProcurementService_List_RejectsNilIDs(t *testing.T) {
	svc := newProcurementSvcForValidationTests()
	if _, err := svc.ListProcurement(context.Background(), uuid.Nil, uuid.New(), nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil project: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.ListProcurement(context.Background(), uuid.New(), uuid.Nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
}

func TestProcurementService_List_RejectsBadStatus(t *testing.T) {
	svc := newProcurementSvcForValidationTests()
	_, err := svc.ListProcurement(context.Background(), uuid.New(), uuid.New(), []string{"bogus"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestProcurementService_Create_RejectsBadInput(t *testing.T) {
	svc := newProcurementSvcForValidationTests()
	cases := []struct {
		name string
		org  uuid.UUID
		in   CreateProcurementItemInput
	}{
		{"nil org", uuid.Nil, CreateProcurementItemInput{ProjectID: uuid.New(), Name: "x", WBSCode: "1.1", EstimatedCostCurrencyCode: "USD"}},
		{"nil project", uuid.New(), CreateProcurementItemInput{Name: "x", WBSCode: "1.1", EstimatedCostCurrencyCode: "USD"}},
		{"empty name", uuid.New(), CreateProcurementItemInput{ProjectID: uuid.New(), Name: "  ", WBSCode: "1.1", EstimatedCostCurrencyCode: "USD"}},
		{"empty wbs", uuid.New(), CreateProcurementItemInput{ProjectID: uuid.New(), Name: "x", WBSCode: "", EstimatedCostCurrencyCode: "USD"}},
		{"negative cost", uuid.New(), CreateProcurementItemInput{ProjectID: uuid.New(), Name: "x", WBSCode: "1.1", EstimatedCostCents: -1, EstimatedCostCurrencyCode: "USD"}},
		{"negative lead", uuid.New(), CreateProcurementItemInput{ProjectID: uuid.New(), Name: "x", WBSCode: "1.1", LeadTimeDays: -1, EstimatedCostCurrencyCode: "USD"}},
		{"negative buffer", uuid.New(), CreateProcurementItemInput{ProjectID: uuid.New(), Name: "x", WBSCode: "1.1", WeatherBufferDays: -1, EstimatedCostCurrencyCode: "USD"}},
		{"empty currency", uuid.New(), CreateProcurementItemInput{ProjectID: uuid.New(), Name: "x", WBSCode: "1.1", EstimatedCostCurrencyCode: ""}},
		{"unsupported currency", uuid.New(), CreateProcurementItemInput{ProjectID: uuid.New(), Name: "x", WBSCode: "1.1", EstimatedCostCurrencyCode: "EUR"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.CreateProcurementItem(context.Background(), c.org, "sub-1", c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestProcurementService_Update_RejectsBadInput(t *testing.T) {
	svc := newProcurementSvcForValidationTests()
	bogus := "bogus-status"
	emptyPO := ""
	ordered := models.ProcurementStatusOrdered
	cases := []struct {
		name string
		org  uuid.UUID
		in   UpdateProcurementItemInput
	}{
		{"nil org", uuid.Nil, UpdateProcurementItemInput{ItemID: uuid.New(), ProjectID: uuid.New(), Status: ptrString("OK")}},
		{"nil ids", uuid.New(), UpdateProcurementItemInput{Status: ptrString("OK")}},
		{"no fields", uuid.New(), UpdateProcurementItemInput{ItemID: uuid.New(), ProjectID: uuid.New()}},
		{"bogus status", uuid.New(), UpdateProcurementItemInput{ItemID: uuid.New(), ProjectID: uuid.New(), Status: &bogus}},
		{"ordered without po", uuid.New(), UpdateProcurementItemInput{ItemID: uuid.New(), ProjectID: uuid.New(), Status: &ordered}},
		{"ordered with empty po", uuid.New(), UpdateProcurementItemInput{ItemID: uuid.New(), ProjectID: uuid.New(), Status: &ordered, PONumber: &emptyPO}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.UpdateProcurementItem(context.Background(), c.org, "sub-1", c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestProcurementService_RecommendVendors_RejectsBadInput(t *testing.T) {
	svc := newProcurementSvcForValidationTests()
	if _, err := svc.RecommendVendors(context.Background(), uuid.Nil, "sub-1", uuid.New()); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.RecommendVendors(context.Background(), uuid.New(), "sub-1", uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil item: err = %v, want ErrInvalidInput", err)
	}
}

func TestProcurementService_RecommendVendors_NoAIReturnsUnavailable(t *testing.T) {
	// Constructed with nil recommender: a worker-style binary that only
	// recomputes statuses. RecommendVendors must surface a sentinel
	// rather than panicking on the nil call. Validation has already
	// passed here (non-nil ids), so we know the path reached the
	// nil-recommender gate.
	svc := NewProcurementService(nil, nil, nil, nil, nil)
	_, err := svc.RecommendVendors(context.Background(), uuid.New(), "sub-1", uuid.New())
	if !errors.Is(err, ErrAIUnavailable) {
		t.Errorf("err = %v, want ErrAIUnavailable", err)
	}
}

func TestProcurementService_RequestVendorReview_RejectsBadInput(t *testing.T) {
	svc := newProcurementSvcForValidationTests()
	good := RequestVendorReviewInput{
		ProcurementItemID: uuid.New(),
		Vendor:            "Acme Materials",
		TotalCents:        125000,
		CurrencyCode:      "USD",
	}
	cases := []struct {
		name string
		org  uuid.UUID
		mut  func(*RequestVendorReviewInput)
	}{
		{"nil org", uuid.Nil, func(*RequestVendorReviewInput) {}},
		{"nil item", uuid.New(), func(in *RequestVendorReviewInput) { in.ProcurementItemID = uuid.Nil }},
		{"blank vendor", uuid.New(), func(in *RequestVendorReviewInput) { in.Vendor = "   " }},
		{"negative total", uuid.New(), func(in *RequestVendorReviewInput) { in.TotalCents = -1 }},
		{"bad currency", uuid.New(), func(in *RequestVendorReviewInput) { in.CurrencyCode = "EUR" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := good
			c.mut(&in)
			_, err := svc.RequestVendorReview(context.Background(), c.org, "sub-1", in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestProcurementService_RequestVendorReview_NoFeedStoreReturnsUnavailable(t *testing.T) {
	// Constructed with nil feed-card store: worker binary or any path
	// that never wires the feed store. RequestVendorReview must surface
	// a sentinel rather than panicking on the nil call. Validation
	// has passed (non-nil ids), so we know the path reached the
	// nil-feed-store gate.
	svc := NewProcurementService(nil, nil, nil, nil, nil)
	_, err := svc.RequestVendorReview(context.Background(), uuid.New(), "sub-1", RequestVendorReviewInput{
		ProcurementItemID: uuid.New(),
		Vendor:            "Acme",
		TotalCents:        1,
		CurrencyCode:      "USD",
	})
	if !errors.Is(err, ErrVendorReviewUnavailable) {
		t.Errorf("err = %v, want ErrVendorReviewUnavailable", err)
	}
}

func TestConfidenceToPct(t *testing.T) {
	cases := []struct {
		in   float64
		want int
	}{
		{0.0, 0},
		{0.5, 50},
		{1.0, 100},
		{0.123, 12},  // rounds half-down
		{0.125, 13},  // half-up at .5
		{0.999, 100}, // rounds to 100
		{-0.5, 0},    // clamps below 0
		{1.5, 100},   // clamps above 1
		{0.005, 1},   // very small
	}
	for _, c := range cases {
		got := confidenceToPct(c.in)
		if got != c.want {
			t.Errorf("confidenceToPct(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestFormatCents covers the integer-cents → fixed-2-decimal renderer used in
// vendor-review feed bodies. Per the Composite Currency Pattern it must never
// touch a float: sub-dollar amounts zero-pad, the cents remainder is always two
// digits, and negatives carry the sign on the whole value (not the cents).
func TestFormatCents(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0.00"},
		{5, "0.05"},
		{50, "0.50"},
		{100, "1.00"},
		{50000, "500.00"},
		{123456, "1234.56"},
		{-2500, "-25.00"},
		{-7, "-0.07"},
	}
	for _, c := range cases {
		if got := formatCents(c.in); got != c.want {
			t.Errorf("formatCents(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestVendorReviewBody covers the human-readable feed-card body. It must embed
// the vendor, the formatCents-rendered amount, the currency code, and the
// quoted item name verbatim — this is the operator-facing summary, so a
// regression in any field is user-visible.
func TestVendorReviewBody(t *testing.T) {
	got := vendorReviewBody("Framing lumber", "Acme Supply", 50000, "USD")
	for _, want := range []string{"Acme Supply", "500.00", "USD", `"Framing lumber"`} {
		if !strings.Contains(got, want) {
			t.Errorf("vendorReviewBody() = %q, missing %q", got, want)
		}
	}
}
