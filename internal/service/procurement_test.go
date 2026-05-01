package service

import (
	"context"
	"errors"
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
	return NewProcurementService(nil, nil, nil)
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
