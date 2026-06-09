package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/authz"
	"github.com/futurebuildai/buildos/internal/connectors"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Audit actions for the connector registry (Phase 3b). Singular-noun.resource.verb.
const (
	auditActionConnectorConfigUpdated = "connector.config.updated"
	auditActionConnectorConfigReset   = "connector.config.reset"
	auditActionConnectorToolsRefresh  = "connector.tools.refreshed"
)

// Connector kinds + effective-config source discriminators.
const (
	connectorKindBuiltin = "builtin"
	connectorKindMCP     = "mcp"

	connectorSourceDefault  = "default"  // no override row (disabled)
	connectorSourceOverride = "override" // an explicit per-org row
)

// Refresh bounds — cached MCP tool metadata is attacker-influenced (rendered into
// the model's tools[]), so it is hard-bounded at refresh time.
const (
	maxRefreshTools    = 64
	maxToolDescBytes   = 4 * 1024
	maxToolSchemaBytes = 16 * 1024
)

// ErrConnectorUnavailable is returned when an MCP refresh can't reach / parse the
// server (unreachable, SSRF-blocked, protocol error). The handler maps it to a
// 502-class response — a connector outage, not a 500.
var ErrConnectorUnavailable = errors.New("connector: upstream unavailable")

// mcpInstanceNameRE constrains an operator-chosen MCP instance name.
var mcpInstanceNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,40}$`)

// ConnectorService is the integration connector registry (Phase 3b). Two faces
// over connectors_config + connector_tools + the in-code built-in catalog:
//
//  1. Admin CRUD (ListEffective / Set / Reset / RefreshTools) behind
//     /api/v1/admin/connectors, one-tx-per-mutation + audit.
//  2. ToolsFor — the per-request merge buildRegistry consults: the ENABLED
//     connectors' tools, namespaced + MinRole-floored at admin.
//
// Connectors are DEFAULT-OFF. Built-ins (kind=builtin) validate against the
// in-code catalog; MCP instances (kind=mcp) are operator-created with an https
// endpoint + a vault credential.
type ConnectorService struct {
	pool       *pgxpool.Pool
	store      *store.ConnectorConfigStore
	toolsStore *store.ConnectorToolsStore
	catalog    map[string]connectors.Connector // built-ins, by Name()
	order      []string                        // built-in names, stable order
	secret     connectors.SecretResolver       // vault adapter (per-org connector creds)
	egress     *http.Client                    // SSRF-guarded outbound client (shared)
	breakers   *connectors.BreakerRegistry     // per-(org,endpoint) circuit breakers
	clientVer  string
	audit      AuditRecorder
	logger     *slog.Logger
}

// NewConnectorService wires the stores + built-in catalog + the per-org vault
// secret resolver + audit. The SSRF-guarded egress client and the breaker
// registry are constructed internally. A nil AuditRecorder/logger get safe
// defaults; a nil SecretResolver leaves MCP calls unauthenticated.
func NewConnectorService(pool *pgxpool.Pool, st *store.ConnectorConfigStore, toolsStore *store.ConnectorToolsStore, secret connectors.SecretResolver, audit AuditRecorder, logger *slog.Logger) *ConnectorService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	if logger == nil {
		logger = slog.Default()
	}
	catalog := make(map[string]connectors.Connector)
	var order []string
	for _, c := range connectors.Builtins() {
		catalog[c.Name()] = c
		order = append(order, c.Name())
	}
	return &ConnectorService{
		pool:       pool,
		store:      st,
		toolsStore: toolsStore,
		catalog:    catalog,
		order:      order,
		secret:     secret,
		egress:     connectors.NewEgressClient(0),
		breakers:   connectors.NewBreakerRegistry(connectors.BreakerConfig{}),
		clientVer:  "1.0",
		audit:      audit,
		logger:     logger,
	}
}

// ---- Face 1: admin CRUD ------------------------------------------------

// EffectiveConnector is one connector's effective config for an org: a built-in
// (catalog default merged with any row) or an MCP instance (from its row).
type EffectiveConnector struct {
	Connector      string          `json:"connector"`
	Kind           string          `json:"kind"`
	Description    string          `json:"description"`
	Enabled        bool            `json:"enabled"`
	Config         json.RawMessage `json:"config"`
	Endpoint       string          `json:"endpoint,omitempty"` // mcp only
	ToolsCount     int             `json:"tools_count"`        // mcp only (cached)
	ToolsFetchedAt *time.Time      `json:"tools_fetched_at,omitempty"`
	Source         string          `json:"source"` // "default" | "override"
	UpdatedBy      string          `json:"updated_by,omitempty"`
	UpdatedAt      *time.Time      `json:"updated_at,omitempty"`
}

// SetConnectorInput is the validated input for Set. OrgID + UserSub come from
// JWT claims (never the request body). Kind is "" (infer) / "builtin" / "mcp".
type SetConnectorInput struct {
	OrgID         uuid.UUID
	ConnectorName string
	Kind          string
	Enabled       bool
	Config        json.RawMessage
	UserSub       string
}

// ListEffective returns every built-in connector + every MCP instance configured
// for the org, with effective config. Built-ins default-OFF; MCP instances come
// from their rows (with cached tool counts).
func (s *ConnectorService) ListEffective(ctx context.Context, orgID uuid.UUID) ([]EffectiveConnector, error) {
	var (
		rows   []models.ConnectorConfig
		counts map[string]store.ConnectorToolStats
	)
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		r, qErr := s.store.ListByOrg(ctx, tx, orgID)
		if qErr != nil {
			return qErr
		}
		c, qErr := s.toolsStore.CountsByOrg(ctx, tx, orgID)
		if qErr != nil {
			return qErr
		}
		rows, counts = r, c
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list effective connectors: %w", err)
	}

	byName := make(map[string]models.ConnectorConfig, len(rows))
	for _, r := range rows {
		byName[r.ConnectorName] = r
	}

	out := make([]EffectiveConnector, 0, len(s.order)+len(rows))
	// Built-ins first (catalog order), default-OFF or overridden.
	for _, name := range s.order {
		c := s.catalog[name]
		eff := EffectiveConnector{
			Connector: name, Kind: connectorKindBuiltin, Description: c.Description(),
			Enabled: false, Config: json.RawMessage("{}"), Source: connectorSourceDefault,
		}
		if row, ok := byName[name]; ok {
			eff.Enabled = row.Enabled
			eff.Config = defaultConfigJSON(row.Config)
			eff.Source = connectorSourceOverride
			eff.UpdatedBy = row.UpdatedBy
			ts := row.UpdatedAt
			eff.UpdatedAt = &ts
		}
		out = append(out, eff)
	}
	// MCP instances (rows whose name is not a built-in), sorted by name.
	var mcpNames []string
	for _, r := range rows {
		if _, isBuiltin := s.catalog[r.ConnectorName]; !isBuiltin && r.Kind == connectorKindMCP {
			mcpNames = append(mcpNames, r.ConnectorName)
		}
	}
	sort.Strings(mcpNames)
	for _, name := range mcpNames {
		row := byName[name]
		ts := row.UpdatedAt
		eff := EffectiveConnector{
			Connector: name, Kind: connectorKindMCP, Description: "MCP server connector",
			Enabled: row.Enabled, Config: defaultConfigJSON(row.Config), Endpoint: mcpEndpoint(row.Config),
			Source: connectorSourceOverride, UpdatedBy: row.UpdatedBy, UpdatedAt: &ts,
		}
		if st, ok := counts[name]; ok {
			eff.ToolsCount = st.Count
			ft := st.FetchedAt
			eff.ToolsFetchedAt = &ft
		}
		out = append(out, eff)
	}
	return out, nil
}

// Set upserts a connector. A built-in name (in the catalog) toggles enable/config;
// any other name is an MCP instance (kind=mcp) — validated for a sane instance
// name + an https endpoint in config — created or updated. Validation runs BEFORE
// any DB write; upsert + audit in one tx.
func (s *ConnectorService) Set(ctx context.Context, in SetConnectorInput) (models.ConnectorConfig, error) {
	if in.OrgID == uuid.Nil {
		return models.ConnectorConfig{}, fmt.Errorf("%w: org_id is required", ErrInvalidInput)
	}

	kind := connectorKindBuiltin
	cfg, err := validateConfigObject(in.Config)
	if err != nil {
		return models.ConnectorConfig{}, err
	}

	_, isBuiltin := s.catalog[in.ConnectorName]
	if isBuiltin && in.Kind == connectorKindMCP {
		return models.ConnectorConfig{}, fmt.Errorf("%w: %q is a built-in connector, not an MCP instance", ErrInvalidInput, in.ConnectorName)
	}
	if !isBuiltin {
		// An MCP instance. Only mcp instances are creatable (no other kinds).
		if in.Kind != "" && in.Kind != connectorKindMCP {
			return models.ConnectorConfig{}, fmt.Errorf("%w: only mcp connector instances can be created", ErrInvalidInput)
		}
		kind = connectorKindMCP
		if !mcpInstanceNameRE.MatchString(in.ConnectorName) {
			return models.ConnectorConfig{}, fmt.Errorf("%w: invalid connector name (use lowercase letters, digits, _ or -)", ErrInvalidInput)
		}
		endpoint, eErr := validateMCPEndpoint(in.Config)
		if eErr != nil {
			return models.ConnectorConfig{}, eErr
		}
		// Re-marshal a normalized config holding only the validated endpoint.
		cfg, _ = json.Marshal(map[string]string{"endpoint": endpoint})
	}

	var out models.ConnectorConfig
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, qErr := s.store.Upsert(ctx, tx, store.UpsertConnectorConfigParams{
			OrgID: in.OrgID, ConnectorName: in.ConnectorName, Kind: kind,
			Enabled: in.Enabled, Config: cfg, UpdatedBy: in.UserSub,
		})
		if qErr != nil {
			return qErr
		}
		out = row
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID: in.OrgID, UserSub: in.UserSub, Action: auditActionConnectorConfigUpdated,
			ResourceType: AuditResourceConnectorConfig, ResourceID: row.ID,
			Metadata: marshalAudit(map[string]any{"connector": in.ConnectorName, "kind": kind, "enabled": in.Enabled, "config": json.RawMessage(cfg)}),
		})
		return nil
	})
	if err != nil {
		return models.ConnectorConfig{}, fmt.Errorf("set connector config: %w", err)
	}
	return out, nil
}

// Reset removes the override row for a connector (and clears any cached MCP
// tools), resetting it to default-OFF. Idempotent; audits only when a row was
// deleted. A name that is neither a built-in nor an existing row is 404.
func (s *ConnectorService) Reset(ctx context.Context, orgID uuid.UUID, connectorName, userSub string) error {
	if orgID == uuid.Nil {
		return fmt.Errorf("%w: org_id is required", ErrInvalidInput)
	}
	_, isBuiltin := s.catalog[connectorName]
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		affected, qErr := s.store.DeleteByName(ctx, tx, orgID, connectorName)
		if qErr != nil {
			return qErr
		}
		if affected == 0 && !isBuiltin {
			return fmt.Errorf("%w: unknown connector %q", ErrNotFound, connectorName)
		}
		// Clear any cached tools for the name (harmless when none / for built-ins).
		if err := s.toolsStore.ReplaceForConnector(ctx, tx, orgID, connectorName, nil); err != nil {
			return err
		}
		if affected > 0 {
			s.audit.Record(ctx, tx, AuditEntry{
				OrgID: orgID, UserSub: userSub, Action: auditActionConnectorConfigReset,
				ResourceType: AuditResourceConnectorConfig, ResourceID: orgID,
				Metadata: marshalAudit(map[string]any{"connector": connectorName}),
			})
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return err
		}
		return fmt.Errorf("reset connector config: %w", err)
	}
	return nil
}

// RefreshTools connects to an MCP instance's server, lists its tools, bounds the
// (attacker-influenced) metadata, and replaces the cached tool set + audits. An
// unreachable/SSRF-blocked/malformed server yields ErrConnectorUnavailable
// (502-class), never a 500. Only valid for kind=mcp.
func (s *ConnectorService) RefreshTools(ctx context.Context, orgID uuid.UUID, connectorName, userSub string) (int, error) {
	if orgID == uuid.Nil {
		return 0, fmt.Errorf("%w: org_id is required", ErrInvalidInput)
	}
	var row models.ConnectorConfig
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		r, qErr := s.store.GetByName(ctx, tx, orgID, connectorName)
		if qErr != nil {
			return qErr
		}
		row = r
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return 0, fmt.Errorf("%w: unknown connector %q", ErrNotFound, connectorName)
		}
		return 0, fmt.Errorf("refresh: load connector: %w", err)
	}
	if row.Kind != connectorKindMCP {
		return 0, fmt.Errorf("%w: %q is not an MCP connector", ErrInvalidInput, connectorName)
	}
	endpoint := mcpEndpoint(row.Config)
	if endpoint == "" {
		return 0, fmt.Errorf("%w: connector has no endpoint", ErrInvalidInput)
	}

	bearer := s.resolveSecret(ctx, orgID, connectorName)
	client := connectors.NewMCPClient(connectors.MCPClientParams{HTTP: s.egress, Endpoint: endpoint, Bearer: bearer, ClientVersion: s.clientVer})
	if err := client.Initialize(ctx); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrConnectorUnavailable, err)
	}
	remote, err := client.ListTools(ctx)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrConnectorUnavailable, err)
	}

	rowsToCache := boundTools(remote)
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if rErr := s.toolsStore.ReplaceForConnector(ctx, tx, orgID, connectorName, rowsToCache); rErr != nil {
			return rErr
		}
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID: orgID, UserSub: userSub, Action: auditActionConnectorToolsRefresh,
			ResourceType: AuditResourceConnectorConfig, ResourceID: row.ID,
			Metadata: marshalAudit(map[string]any{"connector": connectorName, "tools": len(rowsToCache)}),
		})
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("refresh: cache tools: %w", err)
	}
	return len(rowsToCache), nil
}

// ---- Face 2: the assistant merge --------------------------------------

// ToolsFor returns the ENABLED connectors' tools for a caller, namespaced and
// MinRole-floored at admin. A connectors_config read error is returned so
// buildRegistry can fail closed; one bad connector is logged + skipped.
func (s *ConnectorService) ToolsFor(ctx context.Context, c connectors.Caller) ([]agentic.Tool, error) {
	var (
		rows  []models.ConnectorConfig
		cache map[string][]connectors.ToolDef
	)
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		r, qErr := s.store.ListByOrg(ctx, tx, c.OrgID)
		if qErr != nil {
			return qErr
		}
		rows = r
		cache = make(map[string][]connectors.ToolDef)
		for _, row := range rows {
			if row.Enabled && row.Kind == connectorKindMCP {
				tools, tErr := s.toolsStore.ListByConnector(ctx, tx, c.OrgID, row.ConnectorName)
				if tErr != nil {
					return tErr
				}
				cache[row.ConnectorName] = toToolDefs(tools)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("resolve enabled connectors: %w", err)
	}

	var out []agentic.Tool
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		conn := s.connectorForRow(row, cache[row.ConnectorName])
		if conn == nil {
			continue // orphan builtin row / unknown kind
		}
		tools, bErr := conn.BuildTools(ctx, c)
		if bErr != nil {
			s.logger.WarnContext(ctx, "connector BuildTools failed; skipping connector",
				slog.String("connector", row.ConnectorName), slog.Any("error", bErr))
			continue
		}
		out = append(out, s.namespaceAndFloor(ctx, row.ConnectorName, tools)...)
	}
	return out, nil
}

// connectorForRow returns the Connector to invoke for an enabled row: the catalog
// built-in, or a freshly-hydrated mcp connector (endpoint + cached tools + secret
// + per-(org,endpoint) breaker). Returns nil for an orphan/unknown row.
func (s *ConnectorService) connectorForRow(row models.ConnectorConfig, cached []connectors.ToolDef) connectors.Connector {
	if row.Kind == connectorKindMCP {
		endpoint := mcpEndpoint(row.Config)
		if endpoint == "" || len(cached) == 0 {
			return nil // not yet refreshed / no endpoint → nothing to mount
		}
		return connectors.NewMCPConnector(connectors.MCPConnectorParams{
			Name: row.ConnectorName, Endpoint: endpoint, CachedTools: cached,
			Secret: s.secret, HTTP: s.egress, Breaker: s.breakers.Get(breakerKey(row.OrgID, endpoint)),
			ClientVersion: s.clientVer,
		})
	}
	return s.catalog[row.ConnectorName] // built-in (nil if orphan)
}

// namespaceAndFloor applies the connector tool guards: drop nil-executor/empty/
// invalid names, namespace, floor MinRole to admin.
func (s *ConnectorService) namespaceAndFloor(ctx context.Context, name string, tools []agentic.Tool) []agentic.Tool {
	out := make([]agentic.Tool, 0, len(tools))
	for _, t := range tools {
		if t.Executor == nil || t.Spec.Name == "" {
			s.logger.WarnContext(ctx, "connector tool has a nil executor or empty name; dropping",
				slog.String("connector", name), slog.String("tool", t.Spec.Name))
			continue
		}
		namespaced := connectors.NamespaceToolName(name, t.Spec.Name)
		if !connectors.ValidToolName(namespaced) {
			s.logger.WarnContext(ctx, "connector tool name invalid after namespacing; dropping",
				slog.String("connector", name), slog.String("tool", t.Spec.Name))
			continue
		}
		t.Spec.Name = namespaced
		t.MinRole = floorConnectorMinRole(t.MinRole)
		out = append(out, t)
	}
	return out
}

// ---- helpers -----------------------------------------------------------

func (s *ConnectorService) resolveSecret(ctx context.Context, orgID uuid.UUID, name string) string {
	if s.secret == nil {
		return ""
	}
	if v, err := s.secret.ResolveConnectorSecret(ctx, orgID, name); err == nil {
		return v
	}
	return ""
}

func breakerKey(orgID uuid.UUID, endpoint string) string { return orgID.String() + "|" + endpoint }

// floorConnectorMinRole raises a connector tool's MinRole to admin when it
// declares anything lower (or an unknown/empty role).
func floorConnectorMinRole(declared string) string {
	if authz.RoleRank(declared) < authz.RoleRank(authz.RoleAdmin) {
		return authz.RoleAdmin
	}
	return declared
}

// validateConnectorConfig enforces a JSON-object config (built-in path).
func validateConnectorConfig(raw json.RawMessage) ([]byte, error) { return validateConfigObject(raw) }

// validateMCPEndpoint extracts + validates config.endpoint as an https URL whose
// host is not a literal blocked IP (best-effort; the dialer is the real guard).
func validateMCPEndpoint(raw json.RawMessage) (string, error) {
	var probe struct {
		Endpoint string `json:"endpoint"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &probe)
	}
	u, err := url.Parse(probe.Endpoint)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("%w: config.endpoint must be an https URL", ErrInvalidInput)
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && connectors.IsBlockedIP(ip) {
		return "", fmt.Errorf("%w: endpoint host is in a blocked (private/metadata) range", ErrInvalidInput)
	}
	return u.String(), nil
}

// mcpEndpoint reads config.endpoint ("" when absent).
func mcpEndpoint(raw json.RawMessage) string {
	var probe struct {
		Endpoint string `json:"endpoint"`
	}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &probe)
	}
	return probe.Endpoint
}

// boundTools clamps the attacker-influenced refreshed metadata: ≤ N tools, valid
// names, truncated descriptions, object-only bounded schemas.
func boundTools(remote []connectors.ToolDef) []store.ConnectorToolRow {
	out := make([]store.ConnectorToolRow, 0, len(remote))
	for _, t := range remote {
		if len(out) >= maxRefreshTools {
			break
		}
		if t.Name == "" || !connectors.ValidToolName(t.Name) {
			continue // unusable remote name; skip
		}
		desc := t.Description
		if len(desc) > maxToolDescBytes {
			desc = desc[:maxToolDescBytes]
		}
		schema := []byte(t.InputSchema)
		if len(schema) == 0 || len(schema) > maxToolSchemaBytes || !json.Valid(schema) || !isJSONObject(schema) {
			schema = []byte("{}")
		}
		out = append(out, store.ConnectorToolRow{ToolName: t.Name, Description: desc, InputSchema: schema})
	}
	return out
}

func toToolDefs(rows []models.ConnectorTool) []connectors.ToolDef {
	out := make([]connectors.ToolDef, 0, len(rows))
	for _, r := range rows {
		out = append(out, connectors.ToolDef{Name: r.ToolName, Description: r.Description, InputSchema: r.InputSchema})
	}
	return out
}

// isJSONObject reports whether raw is a JSON object (first non-space byte '{').
func isJSONObject(raw []byte) bool {
	for _, b := range raw {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}
