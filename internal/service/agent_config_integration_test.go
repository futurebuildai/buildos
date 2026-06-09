//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

func newAgentConfigServiceFixture(t *testing.T) (*AgentConfigService, *pgxpool.Pool, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	svc := NewAgentConfigService(pool, store.NewAgentConfigStore(), audit, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Config Co")
	return svc, pool, orgID
}

func agentConfigAuditCount(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID, action string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		WHERE org_id = $1 AND action = $2 AND resource_type = $3`,
		orgID, action, AuditResourceAgentConfig).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

func TestAgentConfigService_Resolve_NoRow_EnabledWithDefault(t *testing.T) {
	svc, _, orgID := newAgentConfigServiceFixture(t)
	ctx := context.Background()

	cc, err := svc.Resolve(ctx, orgID, agentic.Foresight)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !cc.Enabled {
		t.Error("no-row resolve must be enabled-with-default")
	}
	if got := agentic.ParseForesightTuning(cc.Config); got != agentic.DefaultForesightTuning() {
		t.Errorf("no-row foresight config = %+v, want defaults", got)
	}
}

func TestAgentConfigService_SetThenResolve_Disabled(t *testing.T) {
	svc, _, orgID := newAgentConfigServiceFixture(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, SetAgentConfigInput{OrgID: orgID, Capability: agentic.Foresight.String(), Enabled: false, UserSub: "admin"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	cc, err := svc.Resolve(ctx, orgID, agentic.Foresight)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cc.Enabled {
		t.Error("after Set(enabled=false), Resolve must report disabled")
	}
}

func TestAgentConfigService_SetTuning_FlowsThroughResolve(t *testing.T) {
	svc, _, orgID := newAgentConfigServiceFixture(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, SetAgentConfigInput{
		OrgID:      orgID,
		Capability: agentic.Foresight.String(),
		Enabled:    true,
		Config:     []byte(`{"budget_burn_percent":50}`),
		UserSub:    "admin",
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	cc, err := svc.Resolve(ctx, orgID, agentic.Foresight)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	tuning := agentic.ParseForesightTuning(cc.Config)
	if tuning.BudgetBurnPercent != 50 {
		t.Errorf("burn = %d, want 50 (override)", tuning.BudgetBurnPercent)
	}
	if tuning.ScheduleFloatDays != agentic.DefaultForesightTuning().ScheduleFloatDays {
		t.Errorf("schedule_float_days = %d, want default (un-overridden)", tuning.ScheduleFloatDays)
	}
}

func TestAgentConfigService_Set_UnknownCapability_NotFound(t *testing.T) {
	svc, _, orgID := newAgentConfigServiceFixture(t)
	_, err := svc.Set(context.Background(), SetAgentConfigInput{OrgID: orgID, Capability: "nope", Enabled: true, UserSub: "a"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("set of unknown capability = %v, want ErrNotFound", err)
	}
}

func TestAgentConfigService_Set_InvalidForesightTuning_InvalidInput(t *testing.T) {
	svc, _, orgID := newAgentConfigServiceFixture(t)
	cases := []string{
		`{"budget_burn_percent":-9}`, // negative
		`{"schedule_float_days":-1}`, // negative
		`["not","an","object"]`,      // not an object
		`{not valid json`,            // malformed
	}
	for _, c := range cases {
		_, err := svc.Set(context.Background(), SetAgentConfigInput{
			OrgID: orgID, Capability: agentic.Foresight.String(), Enabled: true, Config: []byte(c), UserSub: "a",
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Set(config=%s) = %v, want ErrInvalidInput", c, err)
		}
	}
}

func TestAgentConfigService_Set_WritesAudit(t *testing.T) {
	svc, pool, orgID := newAgentConfigServiceFixture(t)
	if _, err := svc.Set(context.Background(), SetAgentConfigInput{OrgID: orgID, Capability: agentic.Experience.String(), Enabled: false, UserSub: "admin"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if n := agentConfigAuditCount(t, pool, orgID, auditActionAgentConfigUpdated); n != 1 {
		t.Errorf("agent.config.updated audit rows = %d, want 1", n)
	}
}

func TestAgentConfigService_Reset_RemovesOverride_AndAudits(t *testing.T) {
	svc, pool, orgID := newAgentConfigServiceFixture(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, SetAgentConfigInput{OrgID: orgID, Capability: agentic.Foresight.String(), Enabled: false, UserSub: "admin"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := svc.Reset(ctx, orgID, agentic.Foresight.String(), "admin"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	// Back to default (enabled).
	cc, err := svc.Resolve(ctx, orgID, agentic.Foresight)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !cc.Enabled {
		t.Error("after Reset, Resolve must return the enabled default")
	}
	if n := agentConfigAuditCount(t, pool, orgID, auditActionAgentConfigReset); n != 1 {
		t.Errorf("agent.config.reset audit rows = %d, want 1", n)
	}
}

func TestAgentConfigService_Reset_Absent_Idempotent_NoAudit(t *testing.T) {
	svc, pool, orgID := newAgentConfigServiceFixture(t)
	// No override exists. Reset must be a clean no-op (no error, no audit row).
	if err := svc.Reset(context.Background(), orgID, agentic.Foresight.String(), "admin"); err != nil {
		t.Fatalf("idempotent reset returned error: %v", err)
	}
	if n := agentConfigAuditCount(t, pool, orgID, auditActionAgentConfigReset); n != 0 {
		t.Errorf("phantom reset audit rows = %d, want 0", n)
	}
}

func TestAgentConfigService_Reset_UnknownCapability_NotFound(t *testing.T) {
	svc, _, orgID := newAgentConfigServiceFixture(t)
	if err := svc.Reset(context.Background(), orgID, "nope", "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reset of unknown capability = %v, want ErrNotFound", err)
	}
}

func TestAgentConfigService_ListEffective_MergesCatalogAndOverride(t *testing.T) {
	svc, _, orgID := newAgentConfigServiceFixture(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, SetAgentConfigInput{OrgID: orgID, Capability: agentic.Foresight.String(), Enabled: false, UserSub: "admin"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	list, err := svc.ListEffective(ctx, orgID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Every catalog capability is present.
	if len(list) != len(agentic.NewRegistry().Capabilities()) {
		t.Fatalf("list len = %d, want %d (full catalog)", len(list), len(agentic.NewRegistry().Capabilities()))
	}
	var sawForesightOverride, sawDefault bool
	for _, e := range list {
		switch e.Capability {
		case agentic.Foresight.String():
			if e.Source != agentConfigSourceOverride || e.Enabled {
				t.Errorf("foresight effective = %+v, want override+disabled", e)
			}
			if e.UpdatedBy != "admin" || e.UpdatedAt == nil {
				t.Errorf("override row must carry updated_by/at: %+v", e)
			}
			sawForesightOverride = true
		default:
			if e.Source == agentConfigSourceDefault {
				sawDefault = true
				if !e.Enabled {
					t.Errorf("default capability %s should be enabled", e.Capability)
				}
			}
		}
	}
	if !sawForesightOverride || !sawDefault {
		t.Errorf("expected both an override (foresight) and at least one default row; override=%v default=%v", sawForesightOverride, sawDefault)
	}
}
