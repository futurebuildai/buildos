# Phase 3b-ii — MCP Connector + Egress Security (file-level implementation spec)

> **Loop:** ultraplan (this doc) → ultracode → local gates → review → merge + handoff.
> **Builds on:** [PHASE_3B_CONNECTOR_FRAMEWORK.md](./PHASE_3B_CONNECTOR_FRAMEWORK.md) (3b-i, merged `608948c`). All 3b-ii design decisions were owner-approved and recorded in that spec §9.
> **Last updated:** 2026-06-09.

---

## 0. Decision summary (read first)

3b-i shipped the connector **framework** (seam, registry, admin API, default-OFF, fail-closed merge, a built-in `reference` connector — zero egress). 3b-ii adds the **first egress connector type: `mcp`** — a hand-rolled, **no-new-dependency** MCP **Streamable-HTTP** client an operator points at an external MCP server to expose its tools to the chat assistant. This is the repo's **first arbitrary-URL outbound path**, so the SSRF egress guard is a primary deliverable, not a footnote.

**Owner-approved (from 3b spec §9):**
- **Transport:** full MCP **Streamable HTTP** (2025-06-18) — `initialize` → `notifications/initialized` → `tools/list` → `tools/call`; `Accept: application/json, text/event-stream` (handle BOTH a single `application/json` body AND an SSE `text/event-stream` body); `Mcp-Session-Id` + `MCP-Protocol-Version` lifecycle. Hand-rolled (~JSON-RPC 2.0 + a minimal SSE reader), no `mcp-go` / SDK dependency.
- **Egress SSRF guard:** https-only + **resolve-and-pin private-IP denylist** via `net.Dialer.Control` (validates the ACTUAL dialed IP at connect time → closes DNS-rebinding) + **no redirects** + bounded body. **No** operator host-allowlist (denylist only).
- **Resilience:** a **connectors-local** circuit breaker keyed **per (org, endpoint)** (the `internal/ai` breaker is unexported + process-global — do NOT share it; copy the proven ~80-LOC pattern). **Per-call timeout ~8s** (< the 30s chat-loop budget; the loop deadline does not interrupt a tool mid-call). **Per-result byte cap 48KiB** (< the 256KiB cumulative loop budget — the loop checks-before but adds-after with no per-result cap).
- **Credential:** vault provider key `connector:<connector_name>`; unseal at `tools/call` time; missing/undecryptable ⇒ unauthenticated call (or clean soft `IsError` if the server 401s) — never a hard error / retry storm.
- **Cache:** `tools/list` cached in a **dedicated `connector_tools` table** (NOT the config blob), with `fetched_at`; bounded tool count + per-tool description/schema bytes; **operator-driven refresh** (`POST .../refresh`) — no auto-refresh/TTL in 3b-ii (document the staleness window).
- **Accepted residual (documented):** a prompt-injected model can pass internal Confidential data as a `tools/call` argument to the *legitimately-configured* endpoint — irreducible for outbound connectors; mitigated by default-OFF + admin opt-in + admin-floored MinRole + per-call audit. The `experienceSystemPrompt` already frames tool data as untrusted; extend it to mark connector tool **metadata** (names/descriptions/schemas) as untrusted too.

**Isolation:** `net/http`, `net`, `bufio`, `crypto/tls`, `encoding/json` are stdlib; `go.opentelemetry.io/.../otelhttp` is external — all allowed in `internal/connectors` (Check 3 forbids only `internal/*` except `internal/agentic`). So the MCP client + SSRF dialer + breaker live in `internal/connectors`. The DB (cache rows, config, vault unseal) stays in `internal/service`, reached through **two new ports the connectors package DECLARES**: `SecretResolver` and (cache is read by service, passed in) — see §2. `agentic ← connectors ← service` is preserved.

---

## 1. Package / file layout

### New files
| File | Purpose |
|---|---|
| `internal/connectors/egress.go` | `NewEgressClient` — an `*http.Client` with the SSRF-guarded dialer (https-only, resolve-and-pin denylist `Control`, no-redirect, timeouts, otelhttp). `isBlockedIP`. |
| `internal/connectors/egress_test.go` | SSRF unit tests (loopback, RFC1918, 169.254 metadata, ULA, CGNAT, redirect-to-private, scheme reject) — table-driven on `isBlockedIP` + an httptest server bound to loopback that the guard must REFUSE. |
| `internal/connectors/mcpclient.go` | The Streamable-HTTP JSON-RPC client: `initialize` / `notifications/initialized` / `tools/list` (cursor-paginated) / `tools/call`. SSE reader (`application/json` OR `text/event-stream`). Session + protocol-version headers. All failures → typed soft errors, never panic. |
| `internal/connectors/mcpclient_test.go` | Drives the client against an httptest stub MCP server in BOTH `application/json` and `text/event-stream` modes; the full soft-fail matrix (timeout, 5xx, malformed JSON-RPC, oversized body, JSON-RPC error, MCP `isError`, session 404→re-init). |
| `internal/connectors/mcp.go` | `mcpConnector` implementing `connectors.Connector`: wraps cached `ToolDef`s into `agentic.Tool`s whose executors call `tools/call` (breaker + per-call timeout + per-result cap + vault secret). `breakerRegistry` (per-key process-lifetime). `SecretResolver` + `ToolDef` types. |
| `internal/connectors/mcp_test.go` | `mcpConnector.BuildTools` → tool exec happy/soft-fail; breaker trips; secret resolution; result-byte cap. |
| `internal/store/connector_tools.go` | `ConnectorToolsStore` — replace-all + list cached tools per (org, connector). |
| `internal/store/connector_tools_integration_test.go` | round-trips. |
| `migrations/018_mcp_connectors.{up,down}.sql` | `connectors_config` += `kind TEXT NOT NULL DEFAULT 'builtin'`; new `connector_tools` table. |

### Edited files
| File | Edit |
|---|---|
| `internal/connectors/connector.go` | Add `SecretResolver` interface + `ToolDef` struct (declared here; impl/data in service). `Builtins()` unchanged. |
| `internal/models/connector_config.go` | `ConnectorConfig` += `Kind string`. New `ConnectorTool` model. |
| `internal/store/connector_config.go` | `Upsert`/`Get`/`List` carry `kind`; new `UpsertConnectorConfigParams.Kind`. |
| `internal/service/connector_config.go` | `ConnectorService` gains the connector-tools store + a `connectors.SecretResolver` (a vault adapter) + a `*connectors.BreakerRegistry` + the SSRF egress `*http.Client`. `Set` accepts MCP instances (kind=mcp, validates `config.endpoint` https-URL, reserves the `reference`/builtin names + a sane instance-name charset). `ToolsFor` builds an `mcpConnector` per enabled MCP instance (hydrated with cached tools + endpoint + secret + breaker). New `RefreshTools(ctx, orgID, name)` (initialize+tools/list → validate/bound → replace cache + audit `connector.tools.refreshed`). `ListEffective` surfaces kind + `tools_count` + `fetched_at`. |
| `internal/service/integrations.go` (VaultService) | A `ConnectorSecret(ctx, orgID, connectorName)` method resolving provider `connector:<name>` → satisfies `connectors.SecretResolver`. Reserve the `connector:` prefix + bare `anthropic`/`resend` from being shadowed (validate on `SetCredential`). |
| `internal/api/connector_config.go` | `PUT` body gains `kind` + `config`; new `POST /api/v1/admin/connectors/{connector}/refresh` (admin). DTO surfaces kind/tools_count/fetched_at. `writeConnectorError` maps the new MCP/egress soft errors. |
| `internal/service/assistant.go` | `experienceSystemPrompt`: add a line marking connector tool METADATA (names/descriptions/schemas), not just results, as untrusted. |
| `cmd/server/main.go` | Wire the egress client + breaker registry + the vault `SecretResolver` adapter + the connector-tools store into `NewConnectorService`. |
| `scripts/check-isolation.sh` | No change (connectors still imports only agentic + stdlib + otelhttp). Re-run to confirm. |
| `.agents/TECH_STACK.md` | Note: hand-rolled MCP-over-HTTP client, **no new dependency** (owner-approved; recorded so the choice is auditable). |
| `.agents/handoff/API_CONTRACT.md` | MCP instance create/config + refresh endpoints; the `connector:<name>` vault provider convention. |

---

## 2. Connectors-package ports + types (`connector.go`)
```go
// SecretResolver resolves a connector's per-org credential from the vault.
// Declared in connectors (so the package needn't import internal/service);
// service.VaultService implements it. Returns "" (no credential) — NEVER an
// error — when none is configured; a transport/decrypt failure also returns ""
// so a tools/call degrades to unauthenticated (and soft-fails on a 401) rather
// than a hard error / retry storm.
type SecretResolver interface {
	ResolveConnectorSecret(ctx context.Context, orgID uuid.UUID, connectorName string) (string, error)
}

// ToolDef is one cached MCP tool (from tools/list). The service reads these from
// the connector_tools store and hands them to NewMCPConnector; the connectors
// package never touches the DB.
type ToolDef struct {
	Name        string          // the REMOTE tool name (un-namespaced)
	Description string
	InputSchema json.RawMessage // a JSON object; validated before caching
}
```

## 3. SSRF egress (`egress.go`) — the load-bearing security deliverable
```go
// NewEgressClient returns an *http.Client safe for connecting to an
// operator-configured, otherwise-untrusted URL. Guarantees:
//   - https only (the caller also rejects non-https at config + call time);
//   - the ACTUAL dialed IP is validated at connect time (DialContext via a
//     net.Dialer with a Control hook), closing DNS-rebinding/TOCTOU — a hostname
//     that resolves to a public IP at check time but a private IP at dial time
//     is rejected on the dial;
//   - redirects are refused (CheckRedirect returns an error) — a 30x to a
//     private host can't bypass the guard;
//   - bounded dial/TLS/response timeouts; otelhttp-wrapped transport.
func NewEgressClient(perCallTimeout time.Duration) *http.Client

// isBlockedIP rejects loopback (127/8, ::1), RFC1918 + ULA fc00::/7
// (net.IP.IsPrivate), link-local 169.254/16 + fe80::/10 (net.IP.IsLinkLocalUnicast
// — this is the cloud-metadata range), CGNAT 100.64/10 (explicit), unspecified,
// and multicast. The single source of egress truth; unit-tested exhaustively.
func isBlockedIP(ip net.IP) bool
```
Dialer: `(&net.Dialer{Timeout: …, Control: func(network, address string, _ syscall.RawConn) error { host,_,_ := net.SplitHostPort(address); ip := net.ParseIP(host); if ip == nil || isBlockedIP(ip) { return errBlockedAddress }; if network != "tcp4" && network != "tcp6" { return errBlockedNetwork }; return nil }}).DialContext`. Set it as `Transport.DialContext` (NOT DialTLSContext, so Control runs on the raw TCP dial before TLS). `CheckRedirect: func(*http.Request, []*http.Request) error { return errNoRedirect }`. `Transport.TLSClientConfig` default (verify on). Cap `ResponseHeaderTimeout` + an overall per-call `context` deadline.

## 4. MCP client (`mcpclient.go`)
- `type MCPClient struct { http *http.Client; endpoint string; bearer string; protocolVersion string; sessionID string; perCall time.Duration; maxResultBytes int }`.
- `Initialize(ctx)` → POST `initialize` (params: protocolVersion `2025-06-18`, capabilities `{}`, clientInfo `{name:"buildos", version}`). Capture `Mcp-Session-Id` response header (if any) into `sessionID`. Validate the result's `protocolVersion` (accept the server's; if grossly incompatible, error). Then POST `notifications/initialized` (a notification → expect 202).
- `ListTools(ctx)` → POST `tools/list` (cursor loop on `nextCursor`, bounded to N pages / M tools). Returns `[]ToolDef`.
- `CallTool(ctx, name, args json.RawMessage)` → POST `tools/call` (`params:{name, arguments}`). Map the result `content[]` (text blocks concatenated; non-text blocks summarized) → a string; honor MCP `isError` → soft error. JSON-RPC `error` object → soft error.
- **Request plumbing** (`do`): set headers (`Content-Type: application/json`, `Accept: application/json, text/event-stream`, `MCP-Protocol-Version`, `Mcp-Session-Id` when set, `Authorization: Bearer <bearer>` when set); per-call `context.WithTimeout`; bounded `io.LimitReader(body, maxResultBytes+1)` (overflow → soft error). **Response demux by `Content-Type`:** `application/json` → unmarshal one JSON-RPC message; `text/event-stream` → SSE reader that accumulates `data:` lines per event, parses each as a JSON-RPC message, and returns the one whose `id` matches the request (ignoring server-initiated requests/notifications); on `202` (notification) → no body expected. A `404` with a session id → clear `sessionID` and signal re-init (the executor re-initializes once). All failures → typed errors the connector turns into soft `IsError`.
- **SSE reader:** stdlib `bufio.Scanner`/`Reader` over the response body; split on blank lines; collect `data:` field(s) (concatenated with `\n`), ignore `event:`/`id:`/comments; stop at the matching JSON-RPC response. Bounded by `maxResultBytes`.

## 5. The `mcp` connector (`mcp.go`)
```go
func NewMCPConnector(p MCPConnectorParams) Connector // {Name, Endpoint, CachedTools []ToolDef, Secret SecretResolver, HTTP *http.Client, Breaker *Breaker, PerCall time.Duration, MaxResultBytes int}
```
`BuildTools(ctx, caller)` → one `agentic.Tool` per cached `ToolDef` (NOT live tools/list — uses the cache the service hydrated). Each executor closure:
1. `breaker.allow()` → if open, soft `IsError` ("connector temporarily unavailable").
2. resolve `bearer := Secret.ResolveConnectorSecret(ctx, caller.OrgID, name)` (best-effort).
3. build a fresh `MCPClient`, `Initialize` (per call — stateless; sessions are short-lived) then `CallTool(remoteName, args)`.
4. record breaker success/failure; map result → `ToolResult`; ANY failure → soft `IsError` (never a Go error / panic — the 3b-i `registryInvoker` recover is the backstop).

`breakerRegistry` (process-lifetime, `map[string]*Breaker` keyed `org+"|"+endpoint`, mutex-guarded) — one instance held by `ConnectorService`. The `Breaker` is the copied `internal/ai` pattern (closed/open/half-open, generation-seeded).

## 6. Data model — migration 018
```sql
ALTER TABLE connectors_config ADD COLUMN kind TEXT NOT NULL DEFAULT 'builtin'; -- 'builtin' | 'mcp'

CREATE TABLE connector_tools (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    connector_name TEXT NOT NULL,
    tool_name      TEXT NOT NULL,                       -- the remote (un-namespaced) name
    description    TEXT NOT NULL DEFAULT '',
    input_schema   JSONB NOT NULL DEFAULT '{}'::jsonb,
    fetched_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (org_id, connector_name, tool_name)
);
CREATE INDEX idx_connector_tools_org_conn ON connector_tools (org_id, connector_name); -- buildos:lock-ok: fresh table created in same migration
```
`down`: `-- buildos:destructive:` DROP TABLE connector_tools + ALTER … DROP COLUMN kind. The `kind` add is NOT NULL DEFAULT (no backfill).

## 7. Admin API
- `PUT /api/v1/admin/connectors/{connector}` — body `{ enabled, kind?, config? }`.
  - `{connector}` ∈ `Builtins()` ⇒ builtin (kind ignored/`builtin`); config ignored (3b-i behavior preserved).
  - else ⇒ **MCP instance** (kind must be `mcp`): validate the instance name charset (`^[a-z0-9][a-z0-9_-]{1,40}$`, not `reference`, not `connector:`-prefixed) and `config.endpoint` (a syntactically-valid `https://` URL whose host does not literally parse to a blocked IP — best-effort; the dialer is the real guard). Upsert with `kind=mcp`. 404 is NOT returned for an unknown name on PUT (PUT creates the instance); 400 on a bad name/endpoint.
- `POST /api/v1/admin/connectors/{connector}/refresh` — (admin) connect + `tools/list` → validate/bound (cap ≤ 64 tools, description ≤ 4KiB, inputSchema ≤ 16KiB + must be a JSON object) → replace `connector_tools` for (org, name) + audit `connector.tools.refreshed` (count). Soft-fails (502-class) if the server is unreachable / SSRF-blocked / malformed — never 500. Only valid for `kind=mcp`.
- `DELETE …/{connector}` — also clears cached tools for an mcp instance. Idempotent 204.
- The **credential** is set via the existing `PUT /api/v1/integrations/connector:<name>` (vault). Documented in API_CONTRACT.
- `GET …` — EffectiveConnector gains `kind`, `endpoint` (mcp only, from config), `tools_count`, `tools_fetched_at`.

## 8. RBAC / security / compliance
- Default-OFF, admin-floored, Experience-only, namespaced, fail-closed — all inherited from 3b-i and unchanged.
- **SSRF** is the headline: https-only + resolve-and-pin denylist + no-redirect + bounded body. The ONE source of egress truth is `isBlockedIP`; exhaustively unit-tested incl. a real loopback httptest server the guard must refuse.
- **Per-call audit**: every `tools/call` writes an audit row (`connector.tool.called`: connector, remote tool, is_error, caller sub/role) so exfil-via-connector is forensically distinct from an internal read. (Audit on the read path → standalone tx, best-effort, like the chat audit.)
- **Secret isolation**: the credential never reaches the leaf or the model; resolved at call time; `connector:<name>` vault provider; reserved from shadowing `anthropic`/`resend`.
- **No floats / determinism / Composite Currency**: untouched (MCP tools return opaque text the model summarizes; the engine still owns all math).

## 9. Verification (definition of done)
- **SSRF unit (no network):** `isBlockedIP` table (loopback/RFC1918/169.254/fe80/fc00/100.64/unspecified/multicast → blocked; public → allowed); `NewEgressClient` REFUSES a real `httptest.NewServer` (loopback) with the block error; refuses `http://` + a redirect to a private host.
- **MCP client (httptest stub):** happy path in BOTH `application/json` and `text/event-stream` modes; tools/list cursor pagination; tools/call text-content mapping + MCP `isError` → soft; JSON-RPC error → soft; 5xx/timeout/oversized-body/malformed → soft (never panic); session id captured + echoed; 404 → re-init.
- **mcp connector unit:** BuildTools → N tools; executor happy + soft-fail; breaker opens after threshold; missing-secret → unauthenticated call (soft-fail on 401); per-result byte cap.
- **service/integration (ephemeral PG):** create mcp instance (kind=mcp, endpoint validated); refresh against a stub server caches tools (bounded); ToolsFor (admin) mounts the namespaced mcp tools when enabled; disabled/default-OFF mounts none; ListEffective surfaces kind+counts; the connector-tools store replace/list/org-isolation; `connector.tools.refreshed` + `connector.tool.called` audits.
- **Gates:** `make audit` (isolation 1+2+3 — connectors still agentic-only) + `govulncheck` (default+prod) + full `make test-integration` exit 0.

## 10. Top risks
1. **SSRF correctness is paramount** — the `Control`-hook resolve-and-pin is the only layer that closes rebinding; test it adversarially (a loopback server the guard must refuse). Do NOT rely on config-time URL parsing alone.
2. **SSE parsing** is the trickiest code — keep it a small bounded reader; demux strictly by `Content-Type`; never block unbounded.
3. **Per-call timeout < loop budget** + **per-result cap < 256KiB cumulative** — the loop won't interrupt a tool mid-call.
4. **Breaker keyed per (org, endpoint)** — a flaky tenant server must not trip the breaker for all.
5. **Everything soft-fails** — any MCP/HTTP/SSRF/JSON failure → `IsError` result; the 3b-i `registryInvoker` panic-recover is the backstop, but the connector must not rely on it.
6. **Cache is attacker-influenced** — bound tool count + description/schema bytes at refresh; the cached metadata is rendered into the model's tools[].
