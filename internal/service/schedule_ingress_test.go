package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// newImportSvcForValidationTests returns a ScheduleService with a nil pool.
// Every test here exercises ONLY the pre-tx validation pass; reaching
// BeginTxFunc would nil-deref, which itself proves validation rejected before
// any DB work.
func newImportSvcForValidationTests() *ScheduleService {
	return NewScheduleService(nil, nil, nil, nil)
}

func importInput(tasks []ImportTaskInput, deps []ImportDependencyInput) ImportScheduleInput {
	return ImportScheduleInput{Tasks: tasks, Dependencies: deps, Recalculate: false}
}

func TestImportSchedule_EmptyTasksRejected(t *testing.T) {
	svc := newImportSvcForValidationTests()
	_, err := svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub", importInput(nil, nil))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestImportSchedule_DurationBounds(t *testing.T) {
	svc := newImportSvcForValidationTests()
	// 0 is rejected (lower bound is 1, not 0 — physics.getTaskDuration rejects 0).
	_, err := svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub",
		importInput([]ImportTaskInput{{WBSCode: "01-00", Name: "Site", DurationDays: 0}}, nil))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duration 0: err = %v, want ErrInvalidInput", err)
	}
	// Above the migration-019 cap is rejected.
	_, err = svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub",
		importInput([]ImportTaskInput{{WBSCode: "01-00", Name: "Site", DurationDays: 36501}}, nil))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("duration 36501: err = %v, want ErrInvalidInput", err)
	}
}

func TestImportSchedule_BadStatusRejected(t *testing.T) {
	svc := newImportSvcForValidationTests()
	_, err := svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub",
		importInput([]ImportTaskInput{{WBSCode: "01-00", Name: "Site", DurationDays: 3, Status: "bogus"}}, nil))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestImportSchedule_DuplicateWBSRejected(t *testing.T) {
	svc := newImportSvcForValidationTests()
	_, err := svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub",
		importInput([]ImportTaskInput{
			{WBSCode: "01-00", Name: "A", DurationDays: 3},
			{WBSCode: "01-00", Name: "B", DurationDays: 3},
		}, nil))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestImportSchedule_UnknownDepRefRejected(t *testing.T) {
	svc := newImportSvcForValidationTests()
	_, err := svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub",
		importInput(
			[]ImportTaskInput{{WBSCode: "01-00", Name: "Site", DurationDays: 3}},
			[]ImportDependencyInput{{PredecessorCode: "01-00", SuccessorCode: "99-99"}},
		))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "unknown wbs_code") {
		t.Errorf("err = %v, want it to name the unknown wbs_code", err)
	}
}

func TestImportSchedule_BadDependencyTypeRejected(t *testing.T) {
	svc := newImportSvcForValidationTests()
	_, err := svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub",
		importInput(
			[]ImportTaskInput{
				{WBSCode: "01-00", Name: "A", DurationDays: 3},
				{WBSCode: "02-00", Name: "B", DurationDays: 3},
			},
			[]ImportDependencyInput{{PredecessorCode: "01-00", SuccessorCode: "02-00", DependencyType: "ZZ"}},
		))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// TestImportSchedule_SelfLoopRejectedNoPanic is the critical guard: a self-loop
// (predecessor_code == successor_code) MUST be rejected in the validation pass
// BEFORE graph construction, because gonum's SetEdge panics on a self-edge. The
// test asserts ErrInvalidInput and — via the deferred recover — that no panic
// escaped.
func TestImportSchedule_SelfLoopRejectedNoPanic(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("self-loop import panicked (gonum SetEdge?): %v", rec)
		}
	}()
	svc := newImportSvcForValidationTests()
	_, err := svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub",
		importInput(
			[]ImportTaskInput{{WBSCode: "01-00", Name: "Site", DurationDays: 3}},
			[]ImportDependencyInput{{PredecessorCode: "01-00", SuccessorCode: "01-00"}},
		))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "self-loop") {
		t.Errorf("err = %v, want it to mention self-loop", err)
	}
}

// TestImportSchedule_CycleRejected proves a true A→B→A cycle is caught by
// physics.DetectCycle in the pre-tx pass and named in the error.
func TestImportSchedule_CycleRejected(t *testing.T) {
	svc := newImportSvcForValidationTests()
	_, err := svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub",
		importInput(
			[]ImportTaskInput{
				{WBSCode: "A", Name: "A", DurationDays: 3},
				{WBSCode: "B", Name: "B", DurationDays: 3},
			},
			[]ImportDependencyInput{
				{PredecessorCode: "A", SuccessorCode: "B"},
				{PredecessorCode: "B", SuccessorCode: "A"},
			},
		))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("err = %v, want it to mention a cycle", err)
	}
}

func TestImportSchedule_LagBoundsRejected(t *testing.T) {
	svc := newImportSvcForValidationTests()
	_, err := svc.ImportSchedule(context.Background(), uuid.New(), uuid.New(), "sub",
		importInput(
			[]ImportTaskInput{
				{WBSCode: "A", Name: "A", DurationDays: 3},
				{WBSCode: "B", Name: "B", DurationDays: 3},
			},
			[]ImportDependencyInput{{PredecessorCode: "A", SuccessorCode: "B", LagDays: 99999}},
		))
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// TestCreateTask_ValidationBeforeTx proves CreateTask rejects bad input before
// touching the (nil) pool.
func TestCreateTask_ValidationBeforeTx(t *testing.T) {
	svc := newImportSvcForValidationTests()
	cases := []CreateTaskInput{
		{OrgID: uuid.Nil}, // nil org
		{OrgID: uuid.New(), WBSCode: "", Name: "x", DurationDays: 3},   // empty wbs
		{OrgID: uuid.New(), WBSCode: "01", Name: "", DurationDays: 3},  // empty name
		{OrgID: uuid.New(), WBSCode: "01", Name: "x", DurationDays: 0}, // duration 0
		{OrgID: uuid.New(), WBSCode: "01", Name: "x", DurationDays: 3, Status: "bogus"},
		{OrgID: uuid.New(), WBSCode: "01", Name: "x", DurationDays: 3, PercentComplete: 101},
	}
	for i, in := range cases {
		if _, err := svc.CreateTask(context.Background(), in); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("case %d: err = %v, want ErrInvalidInput", i, err)
		}
	}
}

// TestCreateProjectBudgets_ValidationBeforeTx covers the budget batch
// validation legs incl. the cross-currency-equivalent rejections (unsupported
// code, negative cents, empty/dup wbs).
func TestCreateProjectBudgets_ValidationBeforeTx(t *testing.T) {
	svc := NewBudgetService(nil, nil, nil)
	org, proj := uuid.New(), uuid.New()

	// empty batch
	if _, err := svc.CreateProjectBudgets(context.Background(), org, "sub", proj, nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty batch: err = %v, want ErrInvalidInput", err)
	}
	// unsupported currency
	if _, err := svc.CreateProjectBudgets(context.Background(), org, "sub", proj,
		[]CreateProjectBudgetLine{{WBSCode: "01", PhaseName: "p", CurrencyCode: "EUR", EstimatedCostCents: 1}}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("EUR: err = %v, want ErrInvalidInput", err)
	}
	// empty currency (rejected — must default explicitly)
	if _, err := svc.CreateProjectBudgets(context.Background(), org, "sub", proj,
		[]CreateProjectBudgetLine{{WBSCode: "01", PhaseName: "p", CurrencyCode: "", EstimatedCostCents: 1}}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty currency: err = %v, want ErrInvalidInput", err)
	}
	// negative cents
	if _, err := svc.CreateProjectBudgets(context.Background(), org, "sub", proj,
		[]CreateProjectBudgetLine{{WBSCode: "01", PhaseName: "p", CurrencyCode: "USD", EstimatedCostCents: -1}}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("negative cents: err = %v, want ErrInvalidInput", err)
	}
	// duplicate wbs in batch
	if _, err := svc.CreateProjectBudgets(context.Background(), org, "sub", proj,
		[]CreateProjectBudgetLine{
			{WBSCode: "01", PhaseName: "p", CurrencyCode: "USD", EstimatedCostCents: 1},
			{WBSCode: "01", PhaseName: "q", CurrencyCode: "USD", EstimatedCostCents: 2},
		}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("dup wbs: err = %v, want ErrInvalidInput", err)
	}
}

// TestCreateEmployee_ValidationBeforeTx and cert covers the HR validation legs.
func TestCreateEmployee_ValidationBeforeTx(t *testing.T) {
	svc := NewHRService(nil, nil, nil)
	if _, err := svc.CreateEmployee(context.Background(), CreateEmployeeInput{OrgID: uuid.Nil}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateEmployee(context.Background(), CreateEmployeeInput{OrgID: uuid.New(), FirstName: "", LastName: "x", Role: "r"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty first_name: err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateCertification_ValidationBeforeTx(t *testing.T) {
	svc := NewHRService(nil, nil, nil)
	org, emp := uuid.New(), uuid.New()
	// missing expiry
	if _, err := svc.CreateCertification(context.Background(), CreateCertificationInput{OrgID: org, EmployeeID: emp, CertType: "osha_10"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("missing expiry: err = %v, want ErrInvalidInput", err)
	}
	// bad status
	if _, err := svc.CreateCertification(context.Background(), CreateCertificationInput{
		OrgID: org, EmployeeID: emp, CertType: "osha_10", ExpiryDate: time.Now(), Status: "bogus",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad status: err = %v, want ErrInvalidInput", err)
	}
}
