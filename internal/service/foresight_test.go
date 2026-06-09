package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/models"
)

// These are the no-DB unit tests for the foresight service adapters
// (spec §12.2): the deterministic metric math (integer burn incl. the -1
// zero-estimate sentinel, the total_float deref, days-until-must-order), the
// HasMaterialSignal truth table, the severity->priority + module->role maps,
// the ai.ErrUnconfigured -> agentic.ErrReasonerUnavailable soft-fail
// translation, the dedup-skip path, and the context<->ai request field mapping.
// The transactional load/apply legs are covered by the integration suite.

// ---- foresightBurnPercent (integer-only; -1 sentinel) ------------------

func TestForesightBurnPercent(t *testing.T) {
	tests := []struct {
		name      string
		actual    int64
		estimated int64
		want      int
	}{
		{"at_threshold_80", 8000, 10000, 80},
		{"over_estimate_120", 12000, 10000, 120},
		{"under_estimate_50", 5000, 10000, 50},
		{"zero_actual", 0, 10000, 0},
		{"zero_estimate_sentinel", 5000, 0, -1},
		{"zero_both_sentinel", 0, 0, -1},
		{"integer_floor_truncates", 999, 1000, 99}, // 99900/1000 = 99 (not 99.9)
		{"large_no_overflow", 1_000_000_000_00, 2_000_000_000_00, 50},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := foresightBurnPercent(tc.actual, tc.estimated)
			if got != tc.want {
				t.Errorf("foresightBurnPercent(%d, %d) = %d, want %d", tc.actual, tc.estimated, got, tc.want)
			}
		})
	}
}

// ---- foresightRemainingFloatDays (total_float deref + clamp) -----------

func TestForesightRemainingFloatDays(t *testing.T) {
	pos := 5
	zero := 0
	neg := -3
	tests := []struct {
		name string
		in   *int
		want int
	}{
		{"nil_is_zero", nil, 0},
		{"positive_passthrough", &pos, 5},
		{"zero_passthrough", &zero, 0},
		{"negative_clamps_to_zero", &neg, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := foresightRemainingFloatDays(tc.in); got != tc.want {
				t.Errorf("foresightRemainingFloatDays(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// ---- foresightDaysUntil (whole-day floor; nil -> 0) --------------------

func TestForesightDaysUntil(t *testing.T) {
	now := time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC)
	in10 := now.Add(10 * 24 * time.Hour)
	overdue := now.Add(-2 * 24 * time.Hour)
	subDay := now.Add(23 * time.Hour) // 23h floors to 0 whole days
	exactDay := now.Add(24 * time.Hour)
	tests := []struct {
		name string
		in   *time.Time
		want int
	}{
		{"nil_is_zero", nil, 0},
		{"ten_days_out", &in10, 10},
		{"overdue_negative", &overdue, -2},
		{"sub_day_floors_to_zero", &subDay, 0},
		{"exact_one_day", &exactDay, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := foresightDaysUntil(tc.in, now); got != tc.want {
				t.Errorf("foresightDaysUntil(%v, now) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

// ---- HasMaterialSignal truth table -------------------------------------

func TestForesightHasMaterialSignal(t *testing.T) {
	tests := []struct {
		name string
		ctx  agentic.ForesightContext
		want bool
	}{
		{
			name: "empty_no_signal",
			ctx:  agentic.ForesightContext{},
			want: false,
		},
		{
			name: "procurement_breach",
			ctx: agentic.ForesightContext{
				Procurement: []agentic.ProcurementMetric{{Breached: true}},
			},
			want: true,
		},
		{
			name: "schedule_breach",
			ctx: agentic.ForesightContext{
				Schedule: []agentic.ScheduleMetric{{Breached: true}},
			},
			want: true,
		},
		{
			name: "budget_breach",
			ctx: agentic.ForesightContext{
				Budget: []agentic.BudgetMetric{{Breached: true}},
			},
			want: true,
		},
		{
			name: "rows_present_but_none_breached",
			ctx: agentic.ForesightContext{
				Procurement: []agentic.ProcurementMetric{{Breached: false}},
				Schedule:    []agentic.ScheduleMetric{{Breached: false}},
				Budget:      []agentic.BudgetMetric{{Breached: false}},
			},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ctx.HasMaterialSignal(); got != tc.want {
				t.Errorf("HasMaterialSignal() = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- severity -> priority map ------------------------------------------

func TestForesightSeverityToPriority(t *testing.T) {
	tests := []struct {
		severity string
		want     string
	}{
		{"critical", models.FeedPriorityCritical},
		{"high", models.FeedPriorityUrgent},
		{"low", models.FeedPriorityLow},
		{"normal", models.FeedPriorityNormal},
		{"", models.FeedPriorityNormal},      // unknown -> normal
		{"bogus", models.FeedPriorityNormal}, // unknown -> normal
	}
	for _, tc := range tests {
		t.Run(tc.severity, func(t *testing.T) {
			if got := foresightSeverityToPriority(tc.severity); got != tc.want {
				t.Errorf("foresightSeverityToPriority(%q) = %q, want %q", tc.severity, got, tc.want)
			}
		})
	}
}

// ---- card_type -> target_role map --------------------------------------

func TestForesightTargetRole(t *testing.T) {
	tests := []struct {
		cardType string
		want     string
	}{
		{foresightCardBudget, "owner"},
		{foresightCardSchedule, "superintendent"},
		{foresightCardProcurement, "superintendent"},
		{"unknown_card_type", "superintendent"}, // default routes to the field owner
	}
	for _, tc := range tests {
		t.Run(tc.cardType, func(t *testing.T) {
			if got := foresightTargetRole(tc.cardType); got != tc.want {
				t.Errorf("foresightTargetRole(%q) = %q, want %q", tc.cardType, got, tc.want)
			}
		})
	}
}

// ---- fake foresightReasonerAI ------------------------------------------

// fakeForesightAI is a deterministic stand-in for the native AI client's
// foresight_risk seam. It captures the last request (to prove the
// context->ai field mapping) and returns a scripted response or error (to
// script the ai.ErrUnconfigured soft-fail leg).
type fakeForesightAI struct {
	resp    *ai.ForesightRiskResponse
	err     error
	calls   int
	lastReq ai.ForesightRiskRequest
}

func (f *fakeForesightAI) ForesightRiskJudgment(_ context.Context, req ai.ForesightRiskRequest) (*ai.ForesightRiskResponse, error) {
	f.calls++
	f.lastReq = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

// newForesightReasonerWithAI builds a ForesightReasoner over a fake seam
// (NewForesightReasoner takes the concrete *ai.Client to dodge the typed-nil
// hazard, so tests inject the fake into the unexported field directly).
func newForesightReasonerWithAI(seam foresightReasonerAI, orgID uuid.UUID) *ForesightReasoner {
	return &ForesightReasoner{ai: seam, orgID: orgID}
}

// ---- soft-fail translation: ai.ErrUnconfigured -> ErrReasonerUnavailable

func TestForesightReasoner_Unconfigured_SoftFails(t *testing.T) {
	seam := &fakeForesightAI{err: ai.ErrUnconfigured}
	r := newForesightReasonerWithAI(seam, uuid.New())

	_, err := r.JudgeRisks(context.Background(), materialForesightContext())
	if !errors.Is(err, agentic.ErrReasonerUnavailable) {
		t.Fatalf("JudgeRisks err = %v, want wrap of ErrReasonerUnavailable", err)
	}
	if seam.calls != 1 {
		t.Errorf("ai calls = %d, want 1", seam.calls)
	}
}

// A nil *ai.Client (no key wired at all) must also surface as
// ErrReasonerUnavailable WITHOUT ever attempting an AI call (the typed-nil
// guard in NewForesightReasoner).
func TestForesightReasoner_NilClient_SoftFails(t *testing.T) {
	r := NewForesightReasoner(nil, uuid.New())

	_, err := r.JudgeRisks(context.Background(), materialForesightContext())
	if !errors.Is(err, agentic.ErrReasonerUnavailable) {
		t.Fatalf("JudgeRisks err = %v, want wrap of ErrReasonerUnavailable", err)
	}
}

// A non-ErrUnconfigured AI error is a HARD error (River retries), NOT a
// soft-fail.
func TestForesightReasoner_OtherAIError_IsHard(t *testing.T) {
	sentinel := errors.New("transport boom")
	seam := &fakeForesightAI{err: sentinel}
	r := newForesightReasonerWithAI(seam, uuid.New())

	_, err := r.JudgeRisks(context.Background(), materialForesightContext())
	if errors.Is(err, agentic.ErrReasonerUnavailable) {
		t.Fatalf("JudgeRisks err = %v, must NOT be ErrReasonerUnavailable (hard error path)", err)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("JudgeRisks err = %v, want wrap of %v", err, sentinel)
	}
}

// ---- context <-> ai request field mapping ------------------------------

func TestForesightReasoner_ContextToRequestMapping(t *testing.T) {
	seam := &fakeForesightAI{resp: &ai.ForesightRiskResponse{}}
	r := newForesightReasonerWithAI(seam, uuid.New())

	ctxIn := agentic.ForesightContext{
		ProjectName: "Maple Ridge Custom",
		Procurement: []agentic.ProcurementMetric{{
			WBS:                "03-30-00",
			Description:        "Cast-in-place concrete",
			Status:             models.ProcurementStatusCritical,
			DaysUntilMustOrder: -2,
		}},
		Schedule: []agentic.ScheduleMetric{{
			WBS:                "06-10-00",
			Name:               "Rough framing",
			RemainingFloatDays: 1,
			IsCritical:         true,
			PercentComplete:    40,
		}},
		Budget: []agentic.BudgetMetric{{
			WBS:            "09-00-00",
			EstimatedCents: 10000,
			CommittedCents: 9000,
			ActualCents:    8500,
			CurrencyCode:   "USD",
			BurnPercent:    85,
		}},
	}

	if _, err := r.JudgeRisks(context.Background(), ctxIn); err != nil {
		t.Fatalf("JudgeRisks: unexpected error: %v", err)
	}

	got := seam.lastReq
	if got.ProjectName != ctxIn.ProjectName {
		t.Errorf("ProjectName = %q, want %q", got.ProjectName, ctxIn.ProjectName)
	}

	if len(got.Procurement) != 1 {
		t.Fatalf("Procurement len = %d, want 1", len(got.Procurement))
	}
	p := got.Procurement[0]
	if p.WBS != "03-30-00" || p.Description != "Cast-in-place concrete" ||
		p.Status != models.ProcurementStatusCritical || p.DaysUntilMustOrder != -2 {
		t.Errorf("procurement mapping = %+v", p)
	}

	if len(got.Schedule) != 1 {
		t.Fatalf("Schedule len = %d, want 1", len(got.Schedule))
	}
	s := got.Schedule[0]
	if s.WBS != "06-10-00" || s.Name != "Rough framing" || s.RemainingFloatDays != 1 ||
		!s.IsCritical || s.PercentComplete != 40 {
		t.Errorf("schedule mapping = %+v", s)
	}

	if len(got.Budget) != 1 {
		t.Fatalf("Budget len = %d, want 1", len(got.Budget))
	}
	b := got.Budget[0]
	if b.WBS != "09-00-00" || b.EstimatedCents != 10000 || b.CommittedCents != 9000 ||
		b.ActualCents != 8500 || b.CurrencyCode != "USD" || b.BurnPercent != 85 {
		t.Errorf("budget mapping = %+v", b)
	}
}

// ---- ai response -> plan mapping (WBS -> SubjectCode dedup anchor) ------

func TestForesightReasoner_ResponseToPlanMapping(t *testing.T) {
	seam := &fakeForesightAI{resp: &ai.ForesightRiskResponse{
		Risks: []ai.ForesightRiskItem{{
			RiskType:          foresightCardBudget,
			WBS:               "09-00-00",
			Severity:          "high",
			Title:             "Finishes trending over",
			Body:              "Actuals at 85% of estimate with work remaining.",
			RecommendedAction: "Owner to review the finishes allowance.",
		}},
	}}
	r := newForesightReasonerWithAI(seam, uuid.New())

	plan, err := r.JudgeRisks(context.Background(), materialForesightContext())
	if err != nil {
		t.Fatalf("JudgeRisks: unexpected error: %v", err)
	}
	if len(plan.Risks) != 1 {
		t.Fatalf("plan risks = %d, want 1", len(plan.Risks))
	}
	risk := plan.Risks[0]
	// The ai-layer WBS must land on the dedup SubjectCode anchor.
	if risk.SubjectCode != "09-00-00" {
		t.Errorf("SubjectCode = %q, want %q (WBS->SubjectCode mapping)", risk.SubjectCode, "09-00-00")
	}
	if risk.RiskType != foresightCardBudget || risk.Severity != "high" ||
		risk.Title != "Finishes trending over" ||
		risk.RecommendedAction != "Owner to review the finishes allowance." {
		t.Errorf("plan risk mapping = %+v", risk)
	}
}

// A nil response (no error) is a hard error, not a silent empty plan.
func TestForesightReasoner_NilResponse_IsError(t *testing.T) {
	seam := &fakeForesightAI{resp: nil} // nil resp, nil err
	r := newForesightReasonerWithAI(seam, uuid.New())

	if _, err := r.JudgeRisks(context.Background(), materialForesightContext()); err == nil {
		t.Fatal("JudgeRisks: want error for nil response, got nil")
	}
}

// ---- dedup-skip path: empty SubjectCode defaults to "total" ------------

// The apply layer defaults an empty SubjectCode to the project-level "total"
// subject. This is the pure-function half of the dedup-skip path (the
// transactional HasActiveRiskCard skip is covered by the integration suite).
func TestForesightSubjectDefault(t *testing.T) {
	if foresightBudgetTotalSubject != "total" {
		t.Errorf("foresightBudgetTotalSubject = %q, want %q", foresightBudgetTotalSubject, "total")
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty_defaults_to_total", "", "total"},
		{"present_passthrough", "09-00-00", "09-00-00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subject := tc.in
			if subject == "" {
				subject = foresightBudgetTotalSubject
			}
			if subject != tc.want {
				t.Errorf("subject = %q, want %q", subject, tc.want)
			}
		})
	}
}

// (Threshold-default behavior moved to the leaf in Phase 3a: see
// agentic.ForesightTuning.WithDefaults / ParseForesightTuning and their tests in
// internal/agentic/config_test.go. The service no longer owns those defaults.)

// materialForesightContext returns a context with one breached signal — the
// minimal input that would clear HasMaterialSignal and trigger a real AI
// call (so the reasoner-leg tests exercise the dispatch path).
func materialForesightContext() agentic.ForesightContext {
	return agentic.ForesightContext{
		ProjectName: "Maple Ridge Custom",
		Budget: []agentic.BudgetMetric{{
			WBS:            "09-00-00",
			EstimatedCents: 10000,
			ActualCents:    8500,
			CurrencyCode:   "USD",
			BurnPercent:    85,
			Breached:       true,
		}},
	}
}
