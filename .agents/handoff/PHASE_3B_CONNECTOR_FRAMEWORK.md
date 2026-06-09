# Phase 3b-i — Connector Framework + Tool Seam (file-level implementation spec)

> **Loop:** ultraplan (this doc) → ultracode → local gates → review → merge + handoff.
> **North star:** [VISION.md](../../VISION.md) Phase 3 — "3p integration + MCP layer (isolated, configurable, vault-backed) — connectors a builder enables and configures post-deploy."
> **Plan row:** [PHASES_2-4_ULTRALOOP_PLAN.md](./PHASES_2-4_ULTRALOOP_PLAN.md) §"Phase 3 · 3b". **Last updated:** 2026-06-08.

---

## 0. Decision summary (read this first)

Phase 3b is **split** (owner-approved, after a 4-lens design critique found the single-chunk framing under-secured and under-scoped):

- **3b-i (THIS spec):** the connector **framework** + tool seam + config registry + admin API + a trivial **in-process** built-in connector. **Zero network I/O. Default-OFF.** Proves the entire seam end-to-end (a connector's tools appear in chat when an admin enables it, are namespaced, role-gated, audited, and soft-fail — and vanish when disabled) before any egress exists.
- **3b-ii (NEXT chunk):** the real **MCP-over-HTTP client** + SSRF egress hardening + resilience, on this proven foundation. **Owner decisions already recorded** (see §9): full Streamable-HTTP (SSE + sessions); private-IP **denylist-only** egress; hand-rolled, **no new dependency**.

**The seam generalizes for free.** `agentic.Tool{Spec, MinRole, Executor}` / `ToolExecutor.Execute(ctx, json.RawMessage)` carry no I/O assumptions — a connector tool is just a `Tool` whose executor does connector work. So **no leaf type change.** The MCP client does `net/http`, so connector code **cannot** live in the leaf (the isolation gate walks `.Imports` + `.TestImports`). New package `internal/connectors` sits **above** the leaf and **below** service: `agentic ← connectors ← service` (one direction, no cycle).

**Dependency direction (load-bearing):** `internal/connectors` imports **only** stdlib + `uuid` + `internal/agentic`. It does **NOT** import `internal/service`/`internal/store`/`internal/ai`. `internal/service` imports `internal/connectors`. Enforced by a new isolation gate **Check 3**.

**Security posture (committed, from the critique):**
- Connectors are **default-OFF per org** — no `connectors_config` row ⇒ disabled; an admin must explicitly enable one. (Opposite of 3a's default-ON capabilities.)
- Connector tools are **read-advisory by admin attestation** (BuildOS cannot enforce read-only on a remote server — a 3b-ii concern, but the framework policy starts here) and **MinRole-floored at `admin`** uniformly. Field workers/superintendents cannot trigger connector tools by chatting.
- **Experience-only.** Connector tools mount **only** in `AssistantService.buildRegistry`. The worker flows (cascade/foresight) **never** touch connectors. Enforced structurally (only `buildRegistry` consults the connector service).
- **Tool-name namespacing** (`conn__<connector>__<tool>`) makes collisions with internal ERP tools and across connectors **structurally impossible**; `AssistantRegistry.Add` **panics** on a duplicate/empty name, so the connector merge path uses a new non-panicking `TryAdd` (skip + log) as belt-and-suspenders.
- The connector leg of `buildRegistry` is **fallible + isolated**: a `connectors_config` read error or one broken connector **soft-fails to "internal tools only"** (fail-closed: mount zero connector tools) — it never breaks chat.

---

## 1. Package / file layout

### New files
| File | Purpose | Imports |
|---|---|---|
| `internal/connectors/connector.go` | `Connector` interface + `Caller` + `NamespaceToolName` / `validToolName` helpers + `Builtins()` catalog. | stdlib + uuid + `internal/agentic` |
| `internal/connectors/reference.go` | `referenceConnector` — the in-process, read-only, no-network built-in (proves the seam). | stdlib + uuid + `internal/agentic` |
| `internal/connectors/connector_test.go`, `reference_test.go` | leaf-ish unit tests (no `internal/service`). | — |
| `internal/models/connector_config.go` | `ConnectorConfig` row struct. | — |
| `internal/store/connector_config.go` | `ConnectorConfigStore` (stateless, `pgx.Tx`, org-scoped). | — |
| `internal/store/connector_config_integration_test.go` | `//go:build integration`. | — |
| `internal/service/connector_config.go` | `ConnectorService` — two faces: admin CRUD (`ListEffective`/`Set`/`Reset`) + `ToolsFor(ctx, caller)` (the per-request enabled+namespaced+floored connector tools for `buildRegistry`). | imports `internal/connectors` |
| `internal/service/connector_config_integration_test.go` | `//go:build integration`. | — |
| `internal/api/connector_config.go` | `ConnectorHandler` (`List`/`Set`/`Reset`) + `writeConnectorError` + `MountConnectorRoutes`. | — |
| `internal/api/connector_config_test.go` | handler unit tests. | — |
| `migrations/017_connectors_config.{up,down}.sql` | additive `connectors_config` table. | — |

### Edited files
| File | Edit |
|---|---|
| `internal/agentic/assistant_tool.go` | Add `AssistantRegistry.TryAdd(t Tool) bool` (non-panicking: returns false on empty/duplicate name) + `Has(name string) bool`. `Add` unchanged (still panics — programmer-error guard for internal tools). |
| `internal/service/assistant.go` | `AssistantService` gains a `connectors *ConnectorService` field; `NewAssistantService` gains the param; `buildRegistry` gains `ctx` and, **after** the internal tools, merges the connector leg (fallible/isolated). `Converse` passes `ctx`. |
| `internal/api/router.go` | `RouterConfig.ConnectorService ConnectorServicer`; conditionally `MountConnectorRoutes` under Auth + SetupGate + `RequireMinRole(admin)`, **off** the pro tree. |
| `cmd/server/main.go` | Build `store.NewConnectorConfigStore()` + `service.NewConnectorService(...)`; pass to `RouterConfig.ConnectorService` **and** into `NewAssistantService(..., connectorService)`. |
| `cmd/worker/main.go` | **No connector wiring** (Experience-only; the worker never builds `AssistantService`). |
| `scripts/check-isolation.sh` | New **Check 3**: `internal/connectors` imports no `internal/*` except `internal/agentic`. Also assert `internal/connectors` absent from the `internal/physics`/`internal/currency` dep closure. |
| `.agents/handoff/API_CONTRACT.md` | Add `/api/v1/admin/connectors` (3 endpoints + status codes) + RBAC matrix row + SetupGate note. |

### Explicitly NOT touched (isolation proof)
`internal/physics`, `internal/currency` — no edits, no new imports. `internal/agentic` gains only the `TryAdd`/`Has` methods (stdlib only — still a leaf). The worker binary gains nothing connector-related.

---

## 2. Key Go types / interfaces

### 2a. `internal/connectors/connector.go`
```go
package connectors

import (
	"context"
	"github.com/google/uuid"
	"github.com/futurebuildai/buildos/internal/agentic"
)

// Caller is the sealed identity a connector binds into its tool executors —
// mirrors the experience flow's closure-binding (the model never sees identity).
type Caller struct {
	OrgID uuid.UUID
	Role  string
	Sub   string
}

// Connector is a named provider of agentic tools. Built-in connectors (3b-i) are
// in-process; the MCP connector (3b-ii) calls an external server. A connector
// PRODUCES tools (with identity-sealed executors); per-org enable + the admin
// MinRole floor are the SERVICE's job (ConnectorService), not the connector's.
type Connector interface {
	// Name is the stable connector id (matches connectors_config.connector_name).
	Name() string
	// Description is admin-facing prose for the GET catalog.
	Description() string
	// BuildTools returns the tools this connector contributes for the caller.
	// Executors are identity-sealed closures. Returns an error only on an
	// internal failure; the registry soft-fails the whole connector leg on error.
	BuildTools(ctx context.Context, c Caller) ([]agentic.Tool, error)
}

// Builtins returns the built-in connector catalog the binary ships. 3b-i: just
// the reference connector. (3b-ii adds the MCP connector type.)
func Builtins() []Connector { return []Connector{newReferenceConnector()} }

// NamespaceToolName deterministically prefixes a connector tool so it can never
// collide with an internal ERP tool (bare names) or another connector. Result is
// validated against Anthropic's tool-name charset by validToolName.
func NamespaceToolName(connector, tool string) string // "conn__"+connector+"__"+tool

// validToolName reports whether s matches ^[a-zA-Z0-9_-]{1,128}$ (Anthropic's
// tool-name constraint — an out-of-charset name 400s the whole Messages call).
func validToolName(s string) bool
```

### 2b. `internal/connectors/reference.go` (the no-network proof)
`referenceConnector` (`Name()="reference"`) exposes **read-only, in-process, deterministic** tools — no secret, no egress:
- `reference_glossary` — optional `{ "term": string }` arg; returns construction/CPM/ERP term definitions (WBS, critical path, total float, GSF, etc.) from a static in-code map (all terms when `term` omitted).
- `reference_supported_currencies` — no args; returns `["USD","CAD"]` + the composite-currency rule (cross-currency forbidden).

Each tool's executor is an `agentic.ToolExecutor` returning `ToolResult{Content: <json>, IsError: bool}` — soft-fails a bad `term` lookup as `IsError`, never a hard error. The connector declares each tool's natural `MinRole` (e.g. `superintendent`); the **service floors it to `admin`** (see 2d).

### 2c. `internal/models` + `internal/store`
```go
type ConnectorConfig struct {
	ID            uuid.UUID       `json:"id"`
	OrgID         uuid.UUID       `json:"org_id"`
	ConnectorName string          `json:"connector_name"` // matches a Builtins() Name()
	Enabled       bool            `json:"enabled"`
	Config        json.RawMessage `json:"config"`         // forward-compat (3b-ii: endpoint, etc.); 3b-i: validated object, otherwise unused
	UpdatedBy     string          `json:"updated_by"`
	CreatedAt, UpdatedAt time.Time
}
```
`ConnectorConfigStore` mirrors `AgentConfigStore` verbatim: `Upsert` (`ON CONFLICT (org_id, connector_name) DO UPDATE`), `GetByName`, `ListByOrg`, `DeleteByName` — all `pgx.Tx`, `WHERE org_id=$1` first.

### 2d. `internal/service/connector_config.go` — `ConnectorService` (two faces)
```go
type ConnectorService struct {
	pool    *pgxpool.Pool
	store   *store.ConnectorConfigStore
	catalog map[string]connectors.Connector // from connectors.Builtins(), by Name()
	audit   AuditRecorder
	logger  *slog.Logger
}
```
**Face 1 — admin CRUD** (mirrors `AgentConfigService`, but **default-OFF**):
- `ListEffective(ctx, orgID) ([]EffectiveConnector, error)` — every built-in connector merged with its per-org row; `Enabled` defaults **false** (no row), `Source` = `"default"`|`"override"`.
- `Set(ctx, SetConnectorInput{OrgID, ConnectorName, Enabled, Config, UserSub})` — validate `ConnectorName ∈ catalog` (`ErrNotFound` → 404) + `Config` is a JSON object (`ErrInvalidInput` → 400); upsert + audit `connector.config.updated` in one tx.
- `Reset(ctx, orgID, name, userSub)` — validate `name ∈ catalog`; delete; idempotent (no row ⇒ nil, no audit); audit `connector.config.reset` only when a row was deleted.

**Face 2 — the assistant merge** (the seam):
```go
// ToolsFor returns the ENABLED connectors' tools for a caller, namespaced and
// MinRole-floored at admin, ready to merge into the per-request registry. A
// connectors_config read error is returned so buildRegistry can soft-fail the
// whole connector leg (fail-closed → mount zero connector tools). NEVER returns
// internal/agentic tools — only connector tools.
func (s *ConnectorService) ToolsFor(ctx context.Context, c connectors.Caller) ([]agentic.Tool, error)
```
`ToolsFor` logic: one read-only tx lists the org's enabled rows; for each built-in connector whose row is `enabled`, call `conn.BuildTools(ctx, c)`; for each returned tool — **namespace** the spec name (`NamespaceToolName`), **drop** it if `!validToolName`, **floor** `MinRole = higher(tool.MinRole, RoleAdmin)` — and collect. A single connector's `BuildTools` error is logged + that connector skipped (one bad connector ≠ whole leg down); a DB error is returned (buildRegistry fail-closes).

### 2e. `internal/agentic/assistant_tool.go` — `TryAdd`
```go
// TryAdd registers a tool if its name is non-empty and unused, returning true.
// On an empty or duplicate name it returns false WITHOUT panicking — the
// connector-merge path uses this so a remote/connector name can never crash
// buildRegistry (Add still panics, as the internal-tool programmer-error guard).
func (r *AssistantRegistry) TryAdd(t Tool) bool

func (r *AssistantRegistry) Has(name string) bool
```

### 2f. `internal/service/assistant.go` — `buildRegistry` merge
`buildRegistry(ctx, orgID, role, sub)` (ctx added). **After** the existing internal-tool `addIfAllowed` block, **before** `return reg`:
```go
// --- connector tools (Phase 3b) — merged AFTER internal tools so internal
// names win; the whole leg is fail-closed: any error mounts ZERO connector
// tools and never breaks chat. ---
if s.connectors != nil {
	connTools, err := s.connectors.ToolsFor(ctx, connectors.Caller{OrgID: orgID, Role: role, Sub: sub})
	if err != nil {
		s.logger.WarnContext(ctx, "connector tools unavailable; serving internal tools only",
			slog.Any("error", err))
	} else {
		for _, t := range connTools {
			if !authz.RoleAtLeast(role, t.MinRole) {
				continue // layer-4 role filter (floored at admin in ToolsFor)
			}
			if !reg.TryAdd(t) { // namespaced, so a collision is a connector bug — skip+log
				s.logger.WarnContext(ctx, "skipped duplicate/invalid connector tool",
					slog.String("tool", t.Spec.Name))
			}
		}
	}
}
```

---

## 3. Migration — `migrations/017_connectors_config.{up,down}.sql`
`up`:
```sql
CREATE TABLE connectors_config (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    connector_name TEXT NOT NULL,                       -- matches a built-in connectors.Connector Name()
    enabled        BOOLEAN NOT NULL DEFAULT false,      -- DEFAULT-OFF (opposite of agents_config)
    config         JSONB NOT NULL DEFAULT '{}'::jsonb,  -- forward-compat (3b-ii: endpoint, etc.); NEVER secrets (vault)
    updated_by     TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, connector_name)
);
CREATE INDEX idx_connectors_config_org ON connectors_config (org_id); -- buildos:lock-ok: fresh table created in same migration
```
`down`: `-- buildos:destructive: rollback of the connectors_config registry (per-org connector enable/config only; no operational data).` + `DROP TABLE IF EXISTS connectors_config;`

No seeding (default-OFF = absence of a row). Reuses the migrate-runner tx-wrap convention (plain `CREATE INDEX` + lock-ok).

---

## 4. Admin API — `/api/v1/admin/connectors`
New operator namespace sibling of `/api/v1/admin/agents`. Behind Auth + **SetupGate** + `RequireMinRole(admin)`, **off** the pro tree. Conditional mount on `cfg.ConnectorService != nil`.
- `GET /api/v1/admin/connectors` → `200 {data:{connectors:[]EffectiveConnector}}` (catalog ⟕ rows; `enabled` defaults false; `source`).
- `PUT /api/v1/admin/connectors/{connector}` → upsert `{enabled, config?}`. `200 {data:{connector}}`; `404` (unknown connector), `400` (config not an object).
- `DELETE /api/v1/admin/connectors/{connector}` → idempotent `204` (reset to default-OFF); `404` (unknown connector).
Audit `connector.config.updated` / `connector.config.reset` (singular-noun convention; `AuditResourceConnectorConfig="connector_config"`). Reads not audited.

---

## 5. Isolation gate — `check-isolation.sh` Check 3
Add after Check 2:
- **Check 3:** `go list -f '{{.Imports}}{{.TestImports}}'` over `./internal/connectors/...`, filter to `MODULE/internal/`, fail if anything other than `MODULE/internal/agentic` appears (in particular `internal/service`, `internal/store`, `internal/ai`). Wording mirrors Check 2.
- Extend **Check 1**'s forbidden set to also assert `internal/connectors` is absent from the `internal/physics`/`internal/currency` dep closure (defensive — they don't import it).

Wire into `make lint-isolation` / `make audit` (already runs the script).

---

## 6. RBAC / security / compliance
- **Default-OFF + admin-floored + Experience-only** (see §0). The reference connector's tools are admin-only even though they make no egress — uniform framework policy (the framework can't know a connector is in-process vs network; 3b-ii connectors ARE egress).
- **Namespacing** guarantees no collision; **TryAdd** is the non-panic backstop; internal tools merge first.
- **Fail-closed** connector leg: any error ⇒ zero connector tools, chat unaffected.
- **No secrets** in `connectors_config` (the reference connector needs none; 3b-ii credentials go in the vault). Config JSONB is validated as an object; PII scrub at the audit layer is a no-op on it.
- **Composite Currency / determinism:** untouched. Connector tools never do money/schedule math; the reference currency tool only *reports* the supported codes.

---

## 7. Ordered task breakdown (bottom-up; each step compiles)
1. `internal/connectors` (connector.go + reference.go + helpers) + unit tests. (Leaf-ish; compiles standalone.)
2. `check-isolation.sh` Check 3 + run it (proves the package boundary before wiring).
3. `agentic.AssistantRegistry.TryAdd`/`Has` + tests.
4. Migration 017 + `make lint-migrations`.
5. model + `ConnectorConfigStore` + integration test.
6. `ConnectorService` (both faces) + integration test.
7. `buildRegistry` ctx + connector merge; `AssistantService` field + `NewAssistantService` param; update its call sites/tests.
8. Admin API handler + `writeConnectorError` + `MountConnectorRoutes` + handler tests; `RouterConfig.ConnectorService`.
9. `cmd/server` wiring.
10. `API_CONTRACT.md`.
11. Gates: `make audit` (incl. isolation 1+2+**3**, migration-017 lint, test, test-prod, bench) + `govulncheck` (both) + `make test-integration`.

---

## 8. Verification (definition of done)
- **Unit:** reference connector tools (glossary lookup hit/miss soft-fail, currencies); namespacing + charset validation; `TryAdd` dup/empty → false (no panic); `buildRegistry` merge — disabled connector ⇒ internal tools only; enabled ⇒ internal + namespaced connector tools; **fail-closed** (a fake `ConnectorService.ToolsFor` returning an error ⇒ internal tools only, no chat break); admin-floor (a superintendent caller never sees connector tools).
- **Integration (ephemeral PG):** store upsert/get/list/delete + org isolation; service Resolve/CRUD — default-OFF (no row ⇒ ListEffective shows disabled, ToolsFor yields nothing), enable ⇒ ToolsFor yields the namespaced admin-floored tools, unknown connector Set/Reset ⇒ 404-class `ErrNotFound`, audit rows for updated/reset (and none for a phantom reset).
- **API:** handler RBAC (route-level admin gate; superintendent → 403), 404 unknown connector, 400 bad config, idempotent 204 reset.
- **Gates green:** `make audit` (isolation **Check 3** included) + `govulncheck` (default+prod) + `make test-integration` exit 0. `make lint-isolation` proves `internal/connectors` imports only `internal/agentic`.
- **Capability demonstrable:** an admin `PUT /api/v1/admin/connectors/reference {"enabled":true}` makes the reference tools appear in the assistant for an admin caller (and only admin) with **no redeploy**; disabling hides them — covered by tests.

---

## 9. Phase 3b-ii (NEXT chunk) — owner decisions RECORDED
The MCP connector + egress security, to build on this framework. **Owner-approved 2026-06-08:**
- **Transport:** **full MCP Streamable HTTP** — `Accept: application/json, text/event-stream`, SSE event-stream parsing, `Mcp-Session-Id` + `MCP-Protocol-Version` lifecycle (`initialize` → `notifications/initialized` → `tools/list` → `tools/call`). Real-server interop (not the sessionless JSON-only cut). Hand-rolled, **no new dependency**.
- **Egress SSRF guard:** **private-IP denylist only** (no operator host allowlist) — https-only + resolve-and-pin via `net.Dialer.Control` rejecting loopback/RFC1918/link-local-169.254+fe80/ULA-fc00/CGNAT-100.64/unspecified/multicast on the **resolved IP at connect time** (closes DNS-rebinding), `CheckRedirect` re-running the guard per hop, bounded body read.
- **Resilience:** ship a **connectors-local circuit breaker keyed per (org, endpoint)** (the `internal/ai` breaker is unexported + process-global; do NOT share it). Per-call timeout ~8–10s (< the 30s loop budget; the loop deadline does not interrupt a tool mid-call). **Per-result byte cap 32–64KiB** (< the 256KiB cumulative loop budget — the loop checks-before but adds-after with no per-result cap).
- **Credential:** vault provider key `connector:<connector_id>`; reserve/reject the `connector:` prefix + bare `anthropic`/`resend` from generic `/integrations` writes; unseal at `tools/call` time; missing-cred ⇒ clean soft `IsError`.
- **Cache:** tools/list cached in a **dedicated `connector_tools` store** (not `config`), with `fetched_at` + a content hash; bounded tool count + per-tool description/schema bytes; operator-driven refresh (`POST .../refresh`); document the staleness window.
- **Accepted residual (documented):** a prompt-injected model can pass internal Confidential data as a connector `tools/call` argument to the *legitimately-configured* endpoint — irreducible for outbound connectors; mitigated by default-OFF + admin opt-in + read-advisory attestation + per-call audit. Extend `experienceSystemPrompt` to mark connector tool **metadata** (names/descriptions/schemas), not just results, as untrusted.

## 10. Top risks (carry into ultracode/review)
1. **Import cycle / isolation:** `internal/connectors` must not import `internal/service` — Check 3 enforces. The merge + config live in service; connectors is a pure library producing `agentic.Tool`.
2. **buildRegistry now does I/O** (the config read) — keep the connector leg fail-closed; `buildRegistry` stays effectively infallible to its caller (Converse).
3. **`TryAdd` vs `Add`:** only the connector path uses `TryAdd`; internal tools keep `Add` (panic guard). Don't weaken `Add`.
4. **Default-OFF** is the opposite of 3a — the resolver/ListEffective must default `enabled=false`, and `ToolsFor` must yield nothing without an explicit enabled row.
5. **Worker isolation:** the worker never constructs `AssistantService`; confirm no connector code is reachable from cascade/foresight.
