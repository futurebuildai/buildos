//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/connectors"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

func newConnectorServiceFixture(t *testing.T) (*ConnectorService, *pgxpool.Pool, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	svc := NewConnectorService(pool, store.NewConnectorConfigStore(), audit, slog.New(slog.NewJSONHandler(io.Discard, nil)))
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Conn Co")
	return svc, pool, orgID
}

func connectorAuditCount(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID, action string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		WHERE org_id = $1 AND action = $2 AND resource_type = $3`,
		orgID, action, AuditResourceConnectorConfig).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

func TestConnectorService_DefaultOff_NoToolsNoEnable(t *testing.T) {
	svc, _, orgID := newConnectorServiceFixture(t)
	ctx := context.Background()

	// ListEffective: the reference connector is present but DISABLED by default.
	list, err := svc.ListEffective(ctx, orgID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("ListEffective must surface the built-in catalog")
	}
	var ref *EffectiveConnector
	for i := range list {
		if list[i].Connector == "reference" {
			ref = &list[i]
		}
	}
	if ref == nil {
		t.Fatal("reference connector missing from catalog")
	}
	if ref.Enabled || ref.Source != "default" {
		t.Errorf("reference default = %+v, want disabled + source=default", ref)
	}

	// ToolsFor: nothing mounts while default-OFF.
	tools, err := svc.ToolsFor(ctx, connectors.Caller{OrgID: orgID, Role: "admin", Sub: "s"})
	if err != nil {
		t.Fatalf("ToolsFor: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("default-OFF must yield no connector tools, got %d", len(tools))
	}
}

func TestConnectorService_EnableThenToolsFor_NamespacedAndAdminFloored(t *testing.T) {
	svc, pool, orgID := newConnectorServiceFixture(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, SetConnectorInput{OrgID: orgID, ConnectorName: "reference", Enabled: true, UserSub: "admin"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if n := connectorAuditCount(t, pool, orgID, auditActionConnectorConfigUpdated); n != 1 {
		t.Errorf("connector.config.updated audit rows = %d, want 1", n)
	}

	tools, err := svc.ToolsFor(ctx, connectors.Caller{OrgID: orgID, Role: "admin", Sub: "s"})
	if err != nil {
		t.Fatalf("ToolsFor: %v", err)
	}
	if len(tools) == 0 {
		t.Fatal("enabled connector must yield tools")
	}
	for _, tl := range tools {
		if !strings.HasPrefix(tl.Spec.Name, "conn__reference__") {
			t.Errorf("tool %q not namespaced", tl.Spec.Name)
		}
		if tl.MinRole != "admin" {
			t.Errorf("tool %q MinRole = %q, want admin (floored)", tl.Spec.Name, tl.MinRole)
		}
	}
}

func TestConnectorService_Set_UnknownConnector_NotFound(t *testing.T) {
	svc, _, orgID := newConnectorServiceFixture(t)
	if _, err := svc.Set(context.Background(), SetConnectorInput{OrgID: orgID, ConnectorName: "nope", Enabled: true, UserSub: "a"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("set of unknown connector = %v, want ErrNotFound", err)
	}
}

func TestConnectorService_Set_BadConfig_InvalidInput(t *testing.T) {
	svc, _, orgID := newConnectorServiceFixture(t)
	for _, c := range []string{`["not","object"]`, `{not json`} {
		_, err := svc.Set(context.Background(), SetConnectorInput{OrgID: orgID, ConnectorName: "reference", Enabled: true, Config: []byte(c), UserSub: "a"})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Set(config=%s) = %v, want ErrInvalidInput", c, err)
		}
	}
}

func TestConnectorService_Reset_RemovesOverride_AndAudits(t *testing.T) {
	svc, pool, orgID := newConnectorServiceFixture(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, SetConnectorInput{OrgID: orgID, ConnectorName: "reference", Enabled: true, UserSub: "admin"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := svc.Reset(ctx, orgID, "reference", "admin"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	// Back to default-OFF: ToolsFor yields nothing again.
	tools, err := svc.ToolsFor(ctx, connectors.Caller{OrgID: orgID, Role: "admin", Sub: "s"})
	if err != nil {
		t.Fatalf("ToolsFor: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("after reset, connector tools = %d, want 0", len(tools))
	}
	if n := connectorAuditCount(t, pool, orgID, auditActionConnectorConfigReset); n != 1 {
		t.Errorf("connector.config.reset audit rows = %d, want 1", n)
	}
}

func TestConnectorService_Reset_Absent_Idempotent_NoAudit(t *testing.T) {
	svc, pool, orgID := newConnectorServiceFixture(t)
	if err := svc.Reset(context.Background(), orgID, "reference", "admin"); err != nil {
		t.Fatalf("idempotent reset returned error: %v", err)
	}
	if n := connectorAuditCount(t, pool, orgID, auditActionConnectorConfigReset); n != 0 {
		t.Errorf("phantom reset audit rows = %d, want 0", n)
	}
}

func TestConnectorService_Reset_UnknownConnector_NotFound(t *testing.T) {
	svc, _, orgID := newConnectorServiceFixture(t)
	if err := svc.Reset(context.Background(), orgID, "nope", "a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reset of unknown connector = %v, want ErrNotFound", err)
	}
}
