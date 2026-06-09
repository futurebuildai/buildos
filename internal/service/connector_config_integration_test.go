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
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/connectors"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

func newConnectorServiceFixture(t *testing.T) (*ConnectorService, *pgxpool.Pool, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	svc := NewConnectorService(pool, store.NewConnectorConfigStore(), store.NewConnectorToolsStore(), nil, audit, slog.New(slog.NewJSONHandler(io.Discard, nil)))
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

func TestConnectorService_Set_UnknownNameNoEndpoint_InvalidInput(t *testing.T) {
	// Phase 3b-ii: an unknown name is interpreted as an MCP instance, so without
	// a valid https endpoint it is a 400 (not a 404).
	svc, _, orgID := newConnectorServiceFixture(t)
	if _, err := svc.Set(context.Background(), SetConnectorInput{OrgID: orgID, ConnectorName: "nope", Enabled: true, UserSub: "a"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("set of an unknown name without an endpoint = %v, want ErrInvalidInput", err)
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

// ---- Phase 3b-ii: MCP connector instances ----

func TestConnectorService_SetMCPInstance_AndListEffective(t *testing.T) {
	svc, _, orgID := newConnectorServiceFixture(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, SetConnectorInput{
		OrgID: orgID, ConnectorName: "acme", Kind: "mcp", Enabled: true,
		Config: []byte(`{"endpoint":"https://mcp.example.com/mcp"}`), UserSub: "admin",
	}); err != nil {
		t.Fatalf("set mcp instance: %v", err)
	}

	list, err := svc.ListEffective(ctx, orgID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	var inst *EffectiveConnector
	for i := range list {
		if list[i].Connector == "acme" {
			inst = &list[i]
		}
	}
	if inst == nil {
		t.Fatal("mcp instance missing from ListEffective")
	}
	if inst.Kind != "mcp" || !inst.Enabled || inst.Endpoint != "https://mcp.example.com/mcp" {
		t.Errorf("instance = %+v, want kind=mcp enabled endpoint set", inst)
	}
}

func TestConnectorService_SetMCPInstance_RejectsBadInput(t *testing.T) {
	svc, _, orgID := newConnectorServiceFixture(t)
	ctx := context.Background()
	cases := []struct {
		name, config string
	}{
		{"acme", `{"endpoint":"http://insecure.example.com"}`}, // not https
		{"acme", `{"endpoint":"https://169.254.169.254/mcp"}`}, // metadata IP literal
		{"acme", `{"endpoint":"https://10.0.0.1/mcp"}`},        // private IP literal
		{"acme", `{}`}, // no endpoint
		{"Bad Name", `{"endpoint":"https://ok.example.com"}`}, // invalid instance name
	}
	for _, c := range cases {
		_, err := svc.Set(ctx, SetConnectorInput{OrgID: orgID, ConnectorName: c.name, Kind: "mcp", Enabled: true, Config: []byte(c.config), UserSub: "a"})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Set(%q, %s) = %v, want ErrInvalidInput", c.name, c.config, err)
		}
	}
}

func TestConnectorService_ToolsFor_MCPInstance_FromCache(t *testing.T) {
	svc, pool, orgID := newConnectorServiceFixture(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, SetConnectorInput{OrgID: orgID, ConnectorName: "acme", Kind: "mcp", Enabled: true, Config: []byte(`{"endpoint":"https://mcp.example.com/mcp"}`), UserSub: "admin"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	// Seed the cache directly (a real refresh would require egress, which is
	// SSRF-blocked for the loopback test server; the fetch path is unit-covered
	// in internal/connectors).
	seedConnectorToolsCache(t, pool, orgID, "acme", []string{"search", "fetch"})

	tools, err := svc.ToolsFor(ctx, connectors.Caller{OrgID: orgID, Role: "admin", Sub: "s"})
	if err != nil {
		t.Fatalf("ToolsFor: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2 from the cached mcp instance", len(tools))
	}
	for _, tl := range tools {
		if !strings.HasPrefix(tl.Spec.Name, "conn__acme__") {
			t.Errorf("tool %q not namespaced under the instance", tl.Spec.Name)
		}
		if tl.MinRole != "admin" {
			t.Errorf("tool %q MinRole = %q, want admin", tl.Spec.Name, tl.MinRole)
		}
	}
}

func TestConnectorService_RefreshTools_Validation(t *testing.T) {
	svc, _, orgID := newConnectorServiceFixture(t)
	ctx := context.Background()

	// Unknown connector → ErrNotFound.
	if _, err := svc.RefreshTools(ctx, orgID, "nope", "a"); !errors.Is(err, ErrNotFound) {
		t.Errorf("refresh unknown = %v, want ErrNotFound", err)
	}
	// A built-in (reference) is not an MCP connector → ErrInvalidInput.
	if _, err := svc.Set(ctx, SetConnectorInput{OrgID: orgID, ConnectorName: "reference", Enabled: true, UserSub: "a"}); err != nil {
		t.Fatalf("enable reference: %v", err)
	}
	if _, err := svc.RefreshTools(ctx, orgID, "reference", "a"); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("refresh reference = %v, want ErrInvalidInput", err)
	}
}

// RefreshTools against an endpoint that resolves to loopback must be blocked by
// the SSRF egress guard end-to-end through the service (→ ErrConnectorUnavailable).
func TestConnectorService_RefreshTools_SSRFBlocked(t *testing.T) {
	svc, _, orgID := newConnectorServiceFixture(t)
	ctx := context.Background()

	if _, err := svc.Set(ctx, SetConnectorInput{OrgID: orgID, ConnectorName: "loopy", Kind: "mcp", Enabled: true, Config: []byte(`{"endpoint":"https://localhost:1/mcp"}`), UserSub: "admin"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	if _, err := svc.RefreshTools(ctx, orgID, "loopy", "admin"); !errors.Is(err, ErrConnectorUnavailable) {
		t.Errorf("refresh of a loopback endpoint = %v, want ErrConnectorUnavailable (SSRF blocked)", err)
	}
}

// seedConnectorToolsCache inserts cached MCP tool rows directly (bypassing the
// egress-blocked refresh) so ToolsFor's mcp dispatch can be exercised.
func seedConnectorToolsCache(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID, name string, toolNames []string) {
	t.Helper()
	rows := make([]store.ConnectorToolRow, 0, len(toolNames))
	for _, tn := range toolNames {
		rows = append(rows, store.ConnectorToolRow{ToolName: tn, Description: tn + " tool", InputSchema: []byte(`{"type":"object"}`)})
	}
	err := pgx.BeginTxFunc(context.Background(), pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return store.NewConnectorToolsStore().ReplaceForConnector(context.Background(), tx, orgID, name, rows)
	})
	if err != nil {
		t.Fatalf("seed connector tools cache: %v", err)
	}
}
