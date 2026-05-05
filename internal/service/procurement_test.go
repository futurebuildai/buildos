package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/brain"
	"github.com/futurebuildai/buildos/internal/models"
)

// fakeProcurementRecommender is the test double for
// MaestroProcurementRecommender. Captures the last request and
// replays a scripted response — same pattern as fakeBriefer in
// agents_test.go. Lets the validation tests assert the gate without
// spinning up an HTTP server.
type fakeProcurementRecommender struct {
	lastReq brain.ProcurementRecommendRequest
	resp    *brain.ProcurementRecommendResponse
	err     error
}

func (f *fakeProcurementRecommender) ProcurementRecommend(_ context.Context, req brain.ProcurementRecommendRequest) (*brain.ProcurementRecommendResponse, error) {
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// These tests cover the input-validation gates that run BEFORE
// ProcurementService touches the database pool. Passing nil pool/store
// proves the gates are effective — any post-validation code path would
// panic.

func newProcurementSvcForValidationTests() *ProcurementService {
	// nil audit falls back to a no-op recorder; nil pool/store
	// causes any post-validation path to panic, which proves the
	// validation gates short-circuit before touching either.
	// nil maestro is intentional — only RecommendVendors uses it,
	// and it short-circuits with ErrMaestroUnavailable rather than
	// panicking, so we get a clean error in those tests too.
	return NewProcurementService(nil, nil, nil, nil)
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

func TestProcurementService_RecommendVendors_NoMaestroReturnsUnavailable(t *testing.T) {
	// Constructed with nil Maestro: a worker-style binary that only
	// recomputes statuses. RecommendVendors must surface a sentinel
	// rather than panicking on the nil call. Validation has already
	// passed here (non-nil ids), so we know the path reached the
	// nil-Maestro gate.
	svc := NewProcurementService(nil, nil, nil, nil)
	_, err := svc.RecommendVendors(context.Background(), uuid.New(), "sub-1", uuid.New())
	if !errors.Is(err, ErrMaestroUnavailable) {
		t.Errorf("err = %v, want ErrMaestroUnavailable", err)
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
		{0.123, 12},   // rounds half-down
		{0.125, 13},   // half-up at .5
		{0.999, 100},  // rounds to 100
		{-0.5, 0},     // clamps below 0
		{1.5, 100},    // clamps above 1
		{0.005, 1},    // very small
	}
	for _, c := range cases {
		got := confidenceToPct(c.in)
		if got != c.want {
			t.Errorf("confidenceToPct(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
