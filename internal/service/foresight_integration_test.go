//go:build integration

package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// fakeForesightReasoner is a deterministic agentic.ForesightReasoner used in CI:
// no live AI. plan is returned verbatim when err is nil; otherwise err is
// returned (used to script the ErrReasonerUnavailable soft-fail leg). It records
// the context it was handed (so a test can prove the workspace projected only
// the breached, not-already-carded snapshot) and counts its calls (so the
// dedup-before-AI guarantee is verifiable: a second sweep over an
// already-carded standing risk must NOT re-invoke the reasoner).
type fakeForesightReasoner struct {
	plan    agentic.ForesightPlan
	err     error
	lastCtx agentic.ForesightContext
	calls   int
}

func (r *fakeForesightReasoner) JudgeRisks(_ context.Context, c agentic.ForesightContext) (agentic.ForesightPlan, error) {
	r.calls++
	r.lastCtx = c
	if r.err != nil {
		return agentic.ForesightPlan{}, r.err
	}
	return r.plan, nil
}

// foresightFixture bundles the seeded ids + the real workspace under test.
type foresightFixture struct {
	pool      *pgxpool.Pool
	workspace *ForesightWorkspace
	orgID     uuid.UUID
	projectID uuid.UUID
}

// newForesightWorkspaceFixture wires a REAL service.ForesightWorkspace over a
// fresh migrated pool with all five stores plus a REAL AuditService (so the
// in-tx audit writes actually hit audit_log and the assertions can count them).
// A zero-value ForesightThresholds is passed so the documented defaults apply
// (schedule float <=2 days, burn >=80%). It seeds an org + active project but no
// schedule/procurement/budget rows; callers seed the graph they need.
func newForesightWorkspaceFixture(t *testing.T) *foresightFixture {
	t.Helper()
	pool := testdb.NewPool(t)

	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ws := NewForesightWorkspace(
		pool,
		store.NewScheduleStore(),
		store.NewProcurementStore(),
		store.NewFinancialsStore(),
		store.NewProjectStore(),
		store.NewFeedCardsStore(),
		audit,
		ForesightThresholds{},
	)

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Stoneridge Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Maple Ridge Custom")

	return &foresightFixture{pool: pool, workspace: ws, orgID: orgID, projectID: projectID}
}

// newForesightOrchestrator builds a ForesightOrchestrator over the fixture's
// real workspace and the supplied reasoner (a fake in CI). Mirrors the cascade
// tests' agentic.NewOrchestrator usage.
func (fx *foresightFixture) orchestrator(r agentic.ForesightReasoner) *agentic.ForesightOrchestrator {
	return agentic.NewForesightOrchestrator(r, fx.workspace, slog.New(slog.NewJSONHandler(io.Discard, nil)))
}

// seedForesightTask inserts a project_tasks row with explicit is_critical /
// total_float / percent_complete so a fixture can construct a real
// schedule-slip breach (critical-and-active, or float <= the threshold).
func seedForesightTask(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, wbs, name string, totalFloat, percentComplete int, isCritical bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO project_tasks (project_id, wbs_code, name, duration_days, status, percent_complete, total_float, is_critical)
		VALUES ($1, $2, $3, 5, 'pending', $4, $5, $6)`,
		projectID, wbs, name, percentComplete, totalFloat, isCritical); err != nil {
		t.Fatalf("seed foresight task %s: %v", wbs, err)
	}
}

// seedForesightProcurement inserts a procurement_items row with an explicit,
// already-computed status (foresight READS procurement_items.status; it never
// re-derives it). WARNING/CRITICAL = breached.
func seedForesightProcurement(t *testing.T, pool *pgxpool.Pool, projectID, orgID uuid.UUID, name, wbs, status string, leadTimeDays int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO procurement_items (project_id, org_id, name, wbs_code, lead_time_days, status)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		projectID, orgID, name, wbs, leadTimeDays, status); err != nil {
		t.Fatalf("seed foresight procurement %s: %v", name, err)
	}
}

// seedForesightBudget inserts a single project_budgets row. All three monetary
// pairs share currency to satisfy chk_budget_currency_match. burn =
// actual*100/estimated (integer); est=0 yields the -1 sentinel (never breaches).
func seedForesightBudget(t *testing.T, pool *pgxpool.Pool, projectID uuid.UUID, wbs, currency string, est, com, act int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO project_budgets (
			project_id, wbs_code, phase_name,
			estimated_cost_cents, estimated_cost_currency_code,
			committed_cost_cents, committed_cost_currency_code,
			actual_cost_cents, actual_cost_currency_code
		) VALUES ($1, $2, $3, $4, $5, $6, $5, $7, $5)`,
		projectID, wbs, "phase "+wbs, est, currency, com, act); err != nil {
		t.Fatalf("seed project_budget %s: %v", wbs, err)
	}
}

// foresightCardRow is the slice of a feed_cards row a foresight assertion reads
// back, including the dedup subject_code + status.
type foresightCardRow struct {
	cardType    string
	priority    string
	subjectCode string
	status      string
	targetRole  *string
}

// readForesightFeedCards returns every feed card the foresight run wrote for the
// org, newest first, so assertions can verify count + card_type / priority /
// subject_code / status / target_role.
func readForesightFeedCards(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID) []foresightCardRow {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT card_type, priority, subject_code, status, target_role
		FROM feed_cards
		WHERE org_id = $1
		ORDER BY created_at DESC`, orgID)
	if err != nil {
		t.Fatalf("query feed_cards: %v", err)
	}
	defer rows.Close()

	var out []foresightCardRow
	for rows.Next() {
		var fc foresightCardRow
		if err := rows.Scan(&fc.cardType, &fc.priority, &fc.subjectCode, &fc.status, &fc.targetRole); err != nil {
			t.Fatalf("scan feed_card: %v", err)
		}
		out = append(out, fc)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate feed_cards: %v", err)
	}
	return out
}

// foresightRiskAuditCount counts the audit_log rows the foresight run wrote
// (action agentic.foresight.risk_surfaced, resource_type foresight,
// resource_id = projectID) for the org — one per surfaced risk card, committed
// in the same tx as the feed card.
func foresightRiskAuditCount(t *testing.T, pool *pgxpool.Pool, orgID, projectID uuid.UUID) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		WHERE org_id = $1
		  AND action = $2
		  AND resource_type = $3
		  AND resource_id = $4`,
		orgID, "agentic.foresight.risk_surfaced", AuditResourceForesight, projectID).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

// dismissForesightCard flips the single active foresight card of the given
// card_type/subject to status='dismissed' (modeling an operator acknowledging
// the standing risk). Used to prove dismissed-state suppression (§5).
func dismissForesightCard(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID, cardType, subject string) {
	t.Helper()
	feedStore := store.NewFeedCardsStore()
	err := pgx.BeginTxFunc(context.Background(), pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var cardID uuid.UUID
		if err := tx.QueryRow(context.Background(), `
			SELECT id FROM feed_cards
			WHERE org_id = $1 AND card_type = $2 AND subject_code = $3 AND status = 'active'
			ORDER BY created_at DESC
			LIMIT 1`, orgID, cardType, subject).Scan(&cardID); err != nil {
			return fmt.Errorf("find active card to dismiss: %w", err)
		}
		if _, err := feedStore.DismissFeedCard(context.Background(), tx, cardID, orgID); err != nil {
			return fmt.Errorf("dismiss card: %w", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("dismiss foresight card %s/%s: %v", cardType, subject, err)
	}
}

// budgetBurnPlan returns a one-risk plan for a budget_burn breach on the given
// WBS — the canonical deterministic foresight plan used by the dedup tests.
func budgetBurnPlan(wbs string) agentic.ForesightPlan {
	return agentic.ForesightPlan{Risks: []agentic.ForesightRisk{{
		RiskType:          foresightCardBudget,
		SubjectCode:       wbs,
		Severity:          "high",
		Title:             "Cost code over 80% burn",
		Body:              "Actuals have consumed more than 80% of the estimate for this phase.",
		RecommendedAction: "Owner to review the phase contingency before the next draw.",
	}}}
}

// TestForesight_MetricCrossesThreshold_SurfacesOneCard is the end-to-end happy
// path: a project with a single breached budget line (burn >= 80%) is loaded by
// the REAL ForesightWorkspace, a FAKE reasoner returns one budget_burn risk for
// that WBS, and the orchestrator applies it in one tx. Asserts exactly one
// feed_cards row of card_type budget_burn, status='active', target_role='owner',
// subject_code = the WBS; and exactly one agentic.foresight.risk_surfaced audit
// row against resource foresight / resource_id = projectID.
func TestForesight_MetricCrossesThreshold_SurfacesOneCard(t *testing.T) {
	fx := newForesightWorkspaceFixture(t)
	ctx := context.Background()

	// One budget line at ~92% burn (breaches the 80% default); one healthy line
	// well under threshold so we prove only the breached subject is carded.
	seedForesightBudget(t, fx.pool, fx.projectID, "1.0", "USD", 500000, 480000, 460000) // 92%
	seedForesightBudget(t, fx.pool, fx.projectID, "2.0", "USD", 500000, 100000, 100000) // 20%

	reasoner := &fakeForesightReasoner{plan: budgetBurnPlan("1.0")}
	res, err := fx.orchestrator(reasoner).RunForesight(ctx, agentic.ForesightInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunForesight: %v", err)
	}

	// The reasoner ran exactly once over a context carrying ONLY the breached
	// line (the healthy 20% line is filtered out by the deterministic gate).
	if reasoner.calls != 1 {
		t.Fatalf("reasoner calls = %d, want 1", reasoner.calls)
	}
	if got := len(reasoner.lastCtx.Budget); got != 1 {
		t.Errorf("reasoner saw %d budget lines, want 1 (only breached)", got)
	} else if reasoner.lastCtx.Budget[0].WBS != "1.0" {
		t.Errorf("reasoner saw budget WBS %q, want 1.0", reasoner.lastCtx.Budget[0].WBS)
	}
	// The breached line crossed the threshold deterministically (92% >= 80%) and
	// the burn is integer-percent (no float64 anywhere on the money path).
	if reasoner.lastCtx.Budget[0].BurnPercent != 92 {
		t.Errorf("burn_percent = %d, want 92 (460000*100/500000, integer)", reasoner.lastCtx.Budget[0].BurnPercent)
	}

	if res.CardsCreated != 1 || res.Risks != 1 || res.CardsSkipped != 0 {
		t.Errorf("result = %+v, want risks 1 / created 1 / skipped 0", res)
	}

	cards := readForesightFeedCards(t, fx.pool, fx.orgID)
	if len(cards) != 1 {
		t.Fatalf("persisted feed cards = %d, want 1", len(cards))
	}
	c := cards[0]
	if c.cardType != foresightCardBudget {
		t.Errorf("card_type = %q, want %q", c.cardType, foresightCardBudget)
	}
	if c.status != "active" {
		t.Errorf("status = %q, want active", c.status)
	}
	if c.subjectCode != "1.0" {
		t.Errorf("subject_code = %q, want 1.0", c.subjectCode)
	}
	if c.priority != "urgent" { // high severity -> urgent priority
		t.Errorf("priority = %q, want urgent", c.priority)
	}
	if c.targetRole == nil || *c.targetRole != "owner" { // budget risk -> owner
		t.Errorf("target_role = %v, want owner", c.targetRole)
	}

	if got := foresightRiskAuditCount(t, fx.pool, fx.orgID, fx.projectID); got != 1 {
		t.Errorf("agentic.foresight.risk_surfaced audit rows = %d, want 1", got)
	}
}

// TestForesight_SecondSweepSameStandingRisk_NoDuplicate proves the full dedup
// lifecycle (§5, §8):
//   - Run #1 surfaces one card + one audit row.
//   - Run #2 (same breach, same fake plan) writes NO new card and NO new audit
//     row; result.CardsSkipped >= 1; AND the fake reasoner is NOT re-invoked
//     (call-count stays 1) — proving dedup happens BEFORE the AI call (the
//     dedup-before-AI cost gate: the carded subject is dropped while loading the
//     context, so HasMaterialSignal is false and the orchestrator never reasons).
//   - DismissFeedCard the card; Run #3 STILL surfaces nothing — dismissed-state
//     suppression (a dismissed card still occupies the dedup slot, so an
//     acknowledged standing risk is not re-spammed daily).
func TestForesight_SecondSweepSameStandingRisk_NoDuplicate(t *testing.T) {
	fx := newForesightWorkspaceFixture(t)
	ctx := context.Background()

	seedForesightBudget(t, fx.pool, fx.projectID, "1.0", "USD", 500000, 480000, 460000) // 92% burn

	reasoner := &fakeForesightReasoner{plan: budgetBurnPlan("1.0")}
	orch := fx.orchestrator(reasoner)

	// Run #1 — surfaces the card.
	res1, err := orch.RunForesight(ctx, agentic.ForesightInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunForesight #1: %v", err)
	}
	if res1.CardsCreated != 1 {
		t.Fatalf("run #1 cards created = %d, want 1", res1.CardsCreated)
	}
	if reasoner.calls != 1 {
		t.Fatalf("run #1 reasoner calls = %d, want 1", reasoner.calls)
	}

	// Run #2 — same standing risk. The carded subject is dropped pre-AI, so the
	// material-signal gate trips and the reasoner is NOT called again.
	res2, err := orch.RunForesight(ctx, agentic.ForesightInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunForesight #2: %v", err)
	}
	if reasoner.calls != 1 {
		t.Errorf("run #2 reasoner calls = %d, want 1 (dedup-before-AI: carded subject dropped before reasoning)", reasoner.calls)
	}
	if res2.CardsCreated != 0 {
		t.Errorf("run #2 cards created = %d, want 0 (dedup)", res2.CardsCreated)
	}

	// Still exactly one card and one audit row across both runs.
	if cards := readForesightFeedCards(t, fx.pool, fx.orgID); len(cards) != 1 {
		t.Fatalf("after run #2, feed cards = %d, want 1 (no duplicate)", len(cards))
	}
	if got := foresightRiskAuditCount(t, fx.pool, fx.orgID, fx.projectID); got != 1 {
		t.Errorf("after run #2, audit rows = %d, want 1 (no duplicate)", got)
	}

	// Dismiss the card, then sweep again. A dismissed card still occupies the
	// dedup slot, so the standing risk is suppressed (not re-surfaced).
	dismissForesightCard(t, fx.pool, fx.orgID, foresightCardBudget, "1.0")

	res3, err := orch.RunForesight(ctx, agentic.ForesightInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunForesight #3: %v", err)
	}
	if reasoner.calls != 1 {
		t.Errorf("run #3 reasoner calls = %d, want 1 (dismissed card still suppresses)", reasoner.calls)
	}
	if res3.CardsCreated != 0 {
		t.Errorf("run #3 cards created = %d, want 0 (dismissed-state suppression)", res3.CardsCreated)
	}

	// Exactly one feed card total (now dismissed), no new active card.
	cards := readForesightFeedCards(t, fx.pool, fx.orgID)
	if len(cards) != 1 {
		t.Fatalf("after run #3, feed cards = %d, want 1 (dismissed, no new card)", len(cards))
	}
	if cards[0].status != "dismissed" {
		t.Errorf("after run #3, card status = %q, want dismissed", cards[0].status)
	}
	if got := foresightRiskAuditCount(t, fx.pool, fx.orgID, fx.projectID); got != 1 {
		t.Errorf("after run #3, audit rows = %d, want 1 (no new surface)", got)
	}
}

// TestForesight_SoftFailNoKey proves the soft-fail leg with the REAL
// ForesightReasoner built over a NIL *ai.Client (modeling a fork with no
// Anthropic key). The deterministic load + material-signal gate still run, but
// JudgeRisks returns agentic.ErrReasonerUnavailable, which the orchestrator
// swallows: zero result + nil error, and zero feed_cards / audit_log rows
// written. A missing AI key must never fail the job.
func TestForesight_SoftFailNoKey(t *testing.T) {
	fx := newForesightWorkspaceFixture(t)
	ctx := context.Background()

	// A real breach so the gate passes and reasoning is genuinely attempted.
	seedForesightBudget(t, fx.pool, fx.projectID, "1.0", "USD", 500000, 480000, 460000) // 92% burn

	// Real reasoner, nil *ai.Client -> typed-nil-safe -> ErrReasonerUnavailable.
	reasoner := NewForesightReasoner(nil, fx.orgID)
	res, err := fx.orchestrator(reasoner).RunForesight(ctx, agentic.ForesightInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunForesight soft-fail returned err = %v, want nil", err)
	}
	if res.CardsCreated != 0 || res.Risks != 0 {
		t.Errorf("result = %+v, want zero cards / risks on soft-fail", res)
	}
	if cards := readForesightFeedCards(t, fx.pool, fx.orgID); len(cards) != 0 {
		t.Errorf("persisted feed cards = %d, want 0 on soft-fail", len(cards))
	}
	if got := foresightRiskAuditCount(t, fx.pool, fx.orgID, fx.projectID); got != 0 {
		t.Errorf("audit rows = %d, want 0 on soft-fail", got)
	}
}

// TestForesight_NoBreach_NoAICall_NoCard proves the material-signal gate: a
// healthy project (no procurement WARNING/CRITICAL, no low-float/critical task,
// no >=80% budget burn) yields a context with no breached metric, so the
// orchestrator skips reasoning entirely (fake reasoner never called) and writes
// zero feed cards / zero audit rows.
func TestForesight_NoBreach_NoAICall_NoCard(t *testing.T) {
	fx := newForesightWorkspaceFixture(t)
	ctx := context.Background()

	// Healthy across all three dimensions:
	//   - procurement OK (not breached).
	//   - task with comfortable float (> 2) and not critical.
	//   - budget line at 20% burn (< 80%).
	seedForesightProcurement(t, fx.pool, fx.projectID, fx.orgID, "Windows", "1.0", "OK", 14)
	seedForesightTask(t, fx.pool, fx.projectID, "1.0", "Landscaping", 10, 0, false) // float 10, non-critical
	seedForesightBudget(t, fx.pool, fx.projectID, "1.0", "USD", 500000, 100000, 100000) // 20% burn

	reasoner := &fakeForesightReasoner{plan: budgetBurnPlan("1.0")}
	res, err := fx.orchestrator(reasoner).RunForesight(ctx, agentic.ForesightInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunForesight: %v", err)
	}
	if reasoner.calls != 0 {
		t.Errorf("reasoner calls = %d, want 0 (no breach -> gate skips reasoning)", reasoner.calls)
	}
	if res.CardsCreated != 0 || res.Risks != 0 {
		t.Errorf("result = %+v, want zero cards / risks", res)
	}
	if cards := readForesightFeedCards(t, fx.pool, fx.orgID); len(cards) != 0 {
		t.Errorf("persisted feed cards = %d, want 0", len(cards))
	}
	if got := foresightRiskAuditCount(t, fx.pool, fx.orgID, fx.projectID); got != 0 {
		t.Errorf("audit rows = %d, want 0", got)
	}
}

// TestForesight_BudgetZeroEstimate_NeverBreaches proves the divide-by-zero
// sentinel: a budget line with EstimatedCents=0 computes BurnPercent=-1, which
// is never >= the threshold, so it never breaches, never enters the context, and
// is never rendered as a card. The fake reasoner is never called.
func TestForesight_BudgetZeroEstimate_NeverBreaches(t *testing.T) {
	fx := newForesightWorkspaceFixture(t)
	ctx := context.Background()

	// Zero estimate but non-zero actuals: a naive ratio would be +Inf, but the
	// integer sentinel makes BurnPercent -1, which never breaches.
	seedForesightBudget(t, fx.pool, fx.projectID, "1.0", "USD", 0, 250000, 250000)

	reasoner := &fakeForesightReasoner{plan: budgetBurnPlan("1.0")}
	res, err := fx.orchestrator(reasoner).RunForesight(ctx, agentic.ForesightInput{OrgID: fx.orgID, ProjectID: fx.projectID})
	if err != nil {
		t.Fatalf("RunForesight: %v", err)
	}
	if reasoner.calls != 0 {
		t.Errorf("reasoner calls = %d, want 0 (zero-estimate -> -1 sentinel -> never breaches)", reasoner.calls)
	}
	if res.CardsCreated != 0 || res.Risks != 0 {
		t.Errorf("result = %+v, want zero cards / risks", res)
	}
	if cards := readForesightFeedCards(t, fx.pool, fx.orgID); len(cards) != 0 {
		t.Errorf("persisted feed cards = %d, want 0", len(cards))
	}
}
