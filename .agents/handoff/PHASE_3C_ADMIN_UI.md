# Phase 3c — Admin Config Web UI (agents + connectors)

**Status:** BUILT on `feat/phase-3c-admin-ui` — all local web gates green (typecheck · 225 vitest · build · eslint+prettier · Playwright a11y 9/9). Awaiting owner review (`/code-review max` ± `ultra`) → merge.
**Owner of this doc:** Claude Code. **North star:** [VISION.md](../../VISION.md) ("agents enabled and tuned post-deploy via admin config" — the UI half). **Contracts:** [API_CONTRACT.md](API_CONTRACT.md) §13b (agents), §13c (connectors), §13 (vault). **Frontend bindings:** [frontend/](frontend/) FRONTEND_ARCHITECTURE / DESIGN_SYSTEM_COMPONENTS / UX_AUTH_ONBOARDING / UX_CORE_SCREENS.

This spec was hardened by a 9-agent design-critique workflow (2 alternative IAs + 6 adversarial lenses → synthesis). Every wire shape below was read from the live Go structs, not the doc examples.

---

## 1. Scope

Operator (**admin+**) web-console screens to manage the two backend admin surfaces so the agentic harness is configurable from the UI, not curl — *"Claude for Small Business inside the ERP."*

- **AI Agents** (`/settings/agents`) — enable/disable + tune the 3 catalog capabilities (`delay_cascade`, `foresight`, `experience`).
- **Connectors** (`/settings/connectors`) — enable the built-in `reference` connector; create/configure/refresh/credential/delete MCP server instances.

**NO backend changes.** Stack: Vite + Lit 3 + TS-strict, Vanilla CSS, dark-only, the existing `fb-*` library, the typed fetch client (single-flight 401→refresh), the History-API router + RBAC, Lit Signals. Reuse the existing screens' patterns (`fb-integrations-page` is the canonical template).

**Hard rules:** WCAG 2.1 AA zero-violation + keyboard-reachable; status never by color alone; never render secret material (write-only, `last4` only); JetBrains Mono for numerics; dark-only. Composite-currency ESLint rule is N/A (no money; `schedule_float_days` / `budget_burn_percent` / `tools_count` are plain ints and do not trip it).

**ESC-002 aside (do not block 3c):** self-minted tokens carry `plan_tier=""`, so `RequirePlanTier(pro)` 402-walls `/api/v1/agents/chat`. The `/admin/*` surfaces 3c wires are **admin-gated, not plan-gated**, so reachable today. Only the chat experience the `experience` capability governs is 402-walled. The screens must **not** be `requiresPro` (the kill-switch must reach admins regardless of tier).

---

## 2. Verified backend wire shapes (authoritative — read from Go structs)

> `config` is `json.RawMessage` → serializes as an **embedded JSON object** (`"config": {...}`), never a quoted string. Send it as an object; never `JSON.stringify` it (server 400s "config not an object").

### 2.1 Agents — `/api/v1/admin/agents` (RequireMinRole=admin; SetupGate; NOT plan-gated)
```jsonc
// GET → { agents: EffectiveAgentConfig[] }
EffectiveAgentConfig = {
  capability: "delay_cascade" | "foresight" | "experience",
  description: string,                 // catalog sentence
  enabled: boolean,
  config: object,                      // embedded object, always ≥ {}
  source: "default" | "override",
  updated_by?: string,                 // omitempty (override only)
  updated_at?: string                  // omitempty ISO (override only)
}
// PUT /admin/agents/{capability}  body { enabled: boolean, config?: object }
//   FULL-DOCUMENT: enabled authoritative; omitted/null config ⇒ RESET tuning to catalog default (NOT a merge).
//   → { agent: AgentConfig{ id, org_id, capability, enabled, config, updated_by, created_at, updated_at } }
//   400 VALIDATION_ERROR (config not object; foresight threshold ≤0); 404 NOT_FOUND (unknown capability).
// DELETE /admin/agents/{capability} → 204 (idempotent reset); 404 unknown capability.
```
Catalog (all `DefaultEnabled=true`):
- **delay_cascade** — `config {}`, no tunable keys. enable/disable only.
- **foresight** — `config { schedule_float_days:int, budget_burn_percent:int }`, defaults `{2, 80}`. Both must be **positive integers (≥1)** on write or 400.
- **experience** — `config {}`, no tunable keys. enable/disable only. (Disabling ⇒ `403 CAPABILITY_DISABLED` on `/agents/chat`.)

### 2.2 Connectors — `/api/v1/admin/connectors` (RequireMinRole=admin; SetupGate; NOT plan-gated; DEFAULT-OFF)
```jsonc
// GET → { connectors: EffectiveConnector[] }
EffectiveConnector = {
  connector: string,                   // name
  kind: "builtin" | "mcp",             // ← UI branches on THIS, never the name string
  description: string,
  enabled: boolean,
  config: object,                      // embedded object, ≥ {}
  endpoint?: string,                   // omitempty — MCP only (https URL)
  tools_count: number,                 // always present int; 0 for builtin / un-refreshed mcp
  tools_fetched_at?: string,           // omitempty ISO — MCP only, present only AFTER a refresh
  source: "default" | "override",
  updated_by?: string, updated_at?: string
}
// Built-in catalog = exactly ONE today: "reference".
// PUT /admin/connectors/{connector}  body { enabled, kind?: "mcp", config?: object }
//   built-in name (reference): toggle enable/config; kind ignored; config must be an object.
//   any other name = MCP INSTANCE: kind "mcp" (or omitted), name ^[a-z0-9][a-z0-9_-]{1,40}$,
//     config { endpoint:"https://…" } (https only; private/metadata IP rejected dial-time). Create-or-update.
//   → { connector: ConnectorConfig{ id, org_id, connector_name, kind, enabled, config, updated_by, created_at, updated_at } }
//   400 VALIDATION_ERROR (bad config/endpoint/name, or kind=mcp on a built-in).
// POST /admin/connectors/{connector}/refresh  (MCP only) → { connector: string, tools_count: number }
//   404 unknown; 400 not-an-mcp; 502 UPSTREAM_ERROR (unreachable / SSRF-blocked / malformed).
// DELETE /admin/connectors/{connector} → 204 (idempotent reset to default-OFF; clears cached tools);
//   404 if neither built-in nor existing instance.
```

### 2.3 Connector credential = the EXISTING vault (NO new endpoint)
- Optional bearer for an MCP instance is set via `/api/v1/integrations` under provider `connector:<name>`.
- The vault is **admin+** (verified — NOT owner-only on the backend; the owner-only gate is only on the existing `/settings/integrations` *page*). So an admin manages connector creds **from within the connectors screen**, never by linking to the owner-only integrations page.
- `GET /api/v1/integrations` returns **all** credentials including `connector:<name>` rows (not filtered). `IntegrationCredential{ id, provider, label, last4, is_active, created_by, created_at, updated_at }`; secret bytes never returned.
- The provider param accepts a colon. Existing TS client: `listIntegrations()`, `setCredential(provider,{label,key})`, `deleteCredential(provider)`.

---

## 3. Information architecture (decision: two routes)

Two new routes in the existing **"Manage"** nav group, both `shell:'org'`, gate `{ roles: ['owner','admin'] }` (admin+), **NOT** `requiresPro`:

| Route | Tag | Nav label | Icon |
|---|---|---|---|
| `/settings/agents` | `fb-agents-page` | AI Agents | `sliders` |
| `/settings/connectors` | `fb-connectors-page` | Connectors | `command` |

> **Rejected — Alt A (single `/settings/automation` with tabs):** the router forwards no query params and the nav highlight is exact-equality, so tab state would need hand-parsed `?tab=` + hand-written `history.replaceState` glue, and tabbed panels strictly increase a11y surface (role=tabpanel + `aria-labelledby` + focusable container + `hidden`-toggling the inactive panel so its `fb-secret-input`/`fb-field` ids leave the tab order). Two flat documents are what the axe tooling + live harness already scan, and "Connectors" stays discoverable.
> **Rejected — Alt B (dedicated "Admin" nav group + overview hub):** over-sectioning for two links; an overview page is net-new scope duplicating status logic; an "Admin" label over `/settings/*` URLs is a namespace mismatch (Organization/Users/Integrations already live under `/settings` in "Manage").

Icons: `sliders`/`command` are free; `sparkles` (AI Assistant), `package` (Procurement), `key` (Integrations) are already in the rail — reusing them is ambiguous. (`hexagon` is the fallback for Connectors if `command` reads wrong in review.)

---

## 4. Screen — `fb-agents-page` (`/settings/agents`)

Loads `GET /admin/agents` → 3 capability cards rendered **inline** (no `fb-agent-card` molecule — one consumer, trivial content; extraction is over-engineering). Page pattern mirrors `fb-integrations-page` (`@state loading/loadError/notice`; `connectedCallback → load()`; `try/catch ApiError`).

### 4.1 Per-card anatomy (hand-authored plain-language copy)
Each card: friendly **title** + a "what it does for you" sentence (not the raw slug/backend description) + `fb-switch` (`label="Enable {friendlyTitle}"`) + a **source badge** via `fb-badge` (`source==='default'` → neutral "Standard settings"; `'override'` → active "Your settings") + a **Reset** affordance shown only when `source==='override'`.

Suggested copy (refine in build):
- **delay_cascade → "Schedule-delay ripple"**: "When a task slips, automatically work out what else is affected — procurement, crews, budget — and post it to your feed."
- **foresight → "Risk early-warning"**: "Keep an eye on every project and warn you about budget overruns, tight schedules, and at-risk material orders before they bite." + tuning form (§4.2).
- **experience → "AI assistant (chat)"**: "Let your team ask BuildOS questions in plain English and get answers grounded in your live data." Cross-surface note: *"Turning this off disables the AI Assistant chat for everyone in your company."* Note chat may be unreachable on this deployment today (ESC-002 `plan_tier=""` 402-wall).

### 4.2 foresight tuning form
A per-card `fb-form` with **two separate `fb-field`** wrappers — **one `fb-field` per `fb-input`** (`fb-field` only wires `controls[0]`; a shared field leaves the 2nd input unnamed & unwireable). Each `fb-input type="number"` `min="1" step="1" inputmode="numeric"`, `name="schedule_float_days"` / `name="budget_burn_percent"`, prefilled with the effective config, with hand-authored **outcome** hints:
- schedule_float_days: *"Warn me when a task has this many days or fewer of slack left — lower warns sooner."*
- budget_burn_percent: *"Warn me once a project has spent this share of its budget (e.g. 80 = at 80% spent)."*

Optional live preview line: *"You'll be warned at {m}% budget and {n} days of slack."* `Save` button writes `PUT { enabled: current, config: { schedule_float_days, budget_burn_percent } }`.

### 4.3 Correctness rules (the load-bearing ones)
- **String coercion (CRITICAL):** `fb-form` serializes control `.value` as **strings**. On Save, `Number()`-coerce and client-validate `Number.isInteger(n) && n >= 1` **before** the PUT. Empty/NaN ⇒ `fb-form.setErrors` keyed by the **human label** ("Schedule float (days)"), since the summary renders `` `${key}: msg` ``.
- **Toggle never resets tuning (CRITICAL):** the enable toggle and Save share one **full-document** PUT where omitted/`{}` config **resets** to catalog default. So the toggle **always** sends `config: savedConfig` — a snapshot of the **last server-confirmed** config, never live inputs. `delay_cascade`/`experience` send `{ enabled }` (their config is `{}`).
- **config is an object:** send the object, never a stringified JSON.
- **Switch truth resync (CRITICAL):** after **any** toggle (success **or** failure) `await this.load()` in `finally` to re-derive `enabled` from server truth. `fb-switch` self-mutates `checked` before the page handler and a same-value re-render is a Lit no-op — `fb-integrations-page`'s always-reload is the only proven resync pattern.
- **400 VALIDATION_ERROR:** consume `err.details[]` directly (translate wire field → human label → the right `fb-field.error` so `aria-invalid` lands on the correct input). Do **not** route VALIDATION_ERROR through `userMessageForCode` (it falls through to generic "our end").
- **Reset:** `source==='override'` → ghost "Reset to default" `fb-button`. For `delay_cascade`/`experience` → `DELETE` directly; for `foresight` (tuned thresholds) → open `fb-confirm` first, then `DELETE`. Refetch after.
- **Serialize writes per capability; explicit `if (busy) return` double-submit gate** (the `fb-form` microtask guard doesn't cover async; `fb-confirm` has no busy state). Disable that card's controls + `aria-busy` "Saving…" while in flight.
- **Focus:** after each refetch re-render, restore focus to the acted-on control (or move it to the result notice) — inline card actions drop focus to `<body>` on full re-render (WCAG 2.4.3).
- **Type safety:** never index `config.schedule_float_days` on a non-foresight card (`noUncheckedIndexedAccess` forces a typed `ForesightConfig` guard).

### 4.4 AI-dependency state (agents need an Anthropic key)
Read the `aiConfigured` signal from `capabilityStore`.
- **Persistent neutral dependency row** on each enabled card (not an error): *"Agents only run when an Anthropic key is set."* Owner sees a real focusable `<a href="/settings/integrations">` rendered **only** when `hasRole('owner')`; admin sees plain text *"Ask your owner to add an Anthropic key."* (no link → no dead-end at the owner-only route). Shown even in assume-on/unknown so the dependency is never hidden.
- When `aiConfigured === false`: escalate to a prominent banner above the cards (icon + text, `role="status"`). (Prefer the persistent row + banner over `fb-state mode=gated` for the whole page, because the kill-switches must stay operable even with AI off.)

### 4.5 States
loading: `fb-state` skeleton card · error (GET failed): `fb-state mode=error retryable` with `error-code`+`request-id` → `@retry=load` · loaded: 3 cards from server truth · per-capability in-flight: that card disabled + `aria-busy` · save success: outcome-stating notice tied to the **response body** (e.g. *"Foresight will now warn you at 80% budget and 2 days of slack."*), not optimistic text · notice region `role` conditional (`alert` for err, `status` for ok). No empty state (always 3 cards).

---

## 5. Screen — `fb-connectors-page` (`/settings/connectors`)

Loads `GET /admin/connectors` **and** `GET /api/v1/integrations` (both admin+). Page intro (plain language): *"Connectors let BuildOS's AI use tools from another service you run. You'll need the server's web address (https) from whoever set it up."*

### 5.1 Add MCP connector
Primary `fb-button` "Connect an external tool server" → **`fb-modal`** (no new dialog component) containing an `fb-form`:
- **Name** `fb-input` (label "A short name, e.g. my-estimator") — client-validate `^[a-z0-9][a-z0-9_-]{1,40}$` **before** submit; error copy is plain *"Use lowercase letters, numbers, dashes; 2–41 characters"* (never the raw regex).
- **Endpoint** `fb-input type="url"` (hint "Must start with https://; private/local addresses are rejected").

Flow: `PUT { enabled:true, kind:'mcp', config:{ endpoint } }`. On **400** → stay in modal, inline `err.details[]` → `fb-form.setErrors` (label-keyed). On **success** → close modal + `await load()` **FIRST** (so the new card renders), **THEN** fire a card-scoped `POST refresh`. On open, programmatically focus the Name field (`fb-modal` otherwise focuses Close).

### 5.2 `fb-connector-card` — the ONE new molecule
Extracted for genuine complexity + isolated a11y/test surface (mirrors how `fb-integrations-page` composes `fb-integration-card`). **Branches strictly on `connector.kind`**, never the name string, and renders all MCP affordances **regardless of `enabled`/`source`**.

- **Built-in `reference` (`kind==='builtin'`):** description (*"A read-only glossary and currency helper that runs inside BuildOS — no external calls."*) + `fb-switch` (`label="Enable reference connector"`) + source badge. Toggling **off** a default-OFF built-in = **DELETE** (drop the override row), not `PUT {enabled:false}`.
- **MCP (`kind==='mcp'`):** `fb-switch` (`label="Enable {name}"`) + endpoint in `fb-text` mono + **tools status badge** + **Refresh** `fb-button` (`icon=refresh`, `aria-label="Refresh tools for {name}"`) + **credential sub-row** + **Edit endpoint** affordance (re-PUT create-or-update — the typo-recovery path for a 502) + **Delete** `fb-button` (`icon=trash`, `aria-label="Delete connector {name}"`) → `fb-confirm` (message names the connector).

**Credential sub-row** — composed from **atoms directly** inside the card (NOT `fb-integration-card`, whose key-state badge / Test button / 2nd Enable switch are all wrong for a bearer): `fb-secret-input` (write-only; `label="{name} bearer token"`; `has-value`/`last4` from the **active** integrations row only) + Save `fb-button`. Helper: *"Access token — only if the server requires sign-in. Leave blank if open."*

**Tools status** as a first-class `fb-badge` (icon+text, never color-only):
- refreshed with N tools → neutral *"{tools_count} tools · refreshed {tools_fetched_at}"*
- enabled && never refreshed (no `tools_fetched_at`) → **warning** *"No tools loaded"* + adjacent Refresh
- after a failed refresh → persistent *"Last refresh failed"* card status (not a vanishing toast)

### 5.3 Correctness rules
- **kind branch, not name:** an MCP instance could be named anything; only `kind` distinguishes.
- **Affordances persist when `enabled=false`:** a disabled MCP instance (`enabled=false, kind='mcp', source='override'`) still renders endpoint/refresh/cred/delete — never suppress, or you strand the instance.
- **Credential presence filters on `is_active`:** `integrations.find(c => c.provider === 'connector:'+name && c.is_active)`. A deactivated/rotated bearer (`is_active=false`, `last4` still present) renders as **"no credential"** — orthogonal to `connector.enabled`.
- **`.submit()` clears the DOM:** after `setCredential`, call `fb-secret-input.submit()` in a `finally` (success **and** error) to wipe plaintext, then refetch `/api/v1/integrations` to refresh `last4`.
- **Name = join key** across the registry path `/admin/connectors/{connector}` and the vault provider `connector:<name>`. Validate the regex before any PUT and `encodeURIComponent(connector)` on the path in `admin.ts` (mirror `integrations.ts`); use the **same** validated string for both writes.
- **Enable auto-refreshes:** enabling an MCP via PUT does **not** run `tools/list`; auto-trigger `POST refresh` after a successful enable-PUT (mirror Add) so "enabled" doesn't strand a toolless no-op. Surface 502 on the card.
- **Error mapping:** refresh **502 UPSTREAM_ERROR** → persistent card copy *"Last refresh failed — couldn't reach {endpoint}. Check the address and try again."* (branch on `err.code === UPSTREAM_ERROR` explicitly; never `userMessageForCode`, which says "our end"). PUT **400** → inline `details[]`.
- **DELETE:** 204 idempotent; **404** (name neither built-in nor instance) treat as success + refetch, not an error. The delete `fb-confirm` copy notes the orphaned vault credential: *"This won't remove any saved access token"* — and offers to delete it too (`deleteCredential('connector:'+name)`).
- **"never refreshed" vs "refreshed, genuinely zero tools":** distinct — only the former (no `tools_fetched_at`) nags; a working empty server isn't warned forever.
- **Double-submit** guards on Add / Refresh / Delete / cred Save (explicit per-action busy flag). **Focus** after Delete moves to a stable target (Add button / page heading).

### 5.4 States
loading: `fb-state` skeleton · error (GET /admin/connectors failed): `fb-state mode=error retryable` · **partial load** (connectors OK, integrations failed): render cards with credential state **"unknown"** (distinct from "no key"); don't block the page on the secondary fetch · MCP list empty: `fb-state mode=empty` "No external tool servers yet." (the built-in `reference` always renders above) · per-card status for refresh/save/delete so card A's success isn't attributed to card B · notice region `role` conditional.

---

## 6. Resolved design decisions (from the critique synthesis)

| # | Decision | Choice |
|---|---|---|
| 1 | IA shape | Two routes under "Manage", admin+, not plan-gated |
| 2 | Molecule extraction | Extract exactly **one**: `fb-connector-card`. Agent cards inline. |
| 3 | Connector cred row | Compose atoms (`fb-secret-input` + Save) inside the card; do NOT reuse `fb-integration-card`. |
| 4 | "AI needs a key" | Persistent neutral per-card dependency row + owner-only link + `aiConfigured=false` banner. Don't dead-end admins at the owner-only integrations route. |
| 5 | foresight write | `Number()`-coerce + validate `≥1`; toggle sends a `savedConfig` snapshot; serialize writes; disable toggle while form dirty. |
| 6 | error→UI mapping | Branch explicitly: VALIDATION_ERROR → `details[]`; UPSTREAM_ERROR → connector copy; add `CAPABILITY_DISABLED` to `errors.ts`. Never via `userMessageForCode`. |
| 7 | switch resync | `await load()` in `finally` after every toggle. |
| 8 | cred presence | Filter `is_active`; `.submit()` clears DOM; enable vs cred presence orthogonal. |
| 9 | icons | Agents `sliders`, Connectors `command`. |
| 10 | Add-MCP failure | PUT 400 → stay in modal inline; PUT success → close + reload FIRST then refresh; 502 → persistent card error + Edit-endpoint. |
| 11 | copy | Hand-authored friendly titles/descriptions/hints; reframe MCP vocab. No backend change. |

**Escalations to owner:** none — the design is unambiguous and all wire shapes are verified.

---

## 7. Files (create / edit)

**Create:**
- `web/src/api/endpoints/admin.ts` — `listAgents/setAgent/resetAgent` + `listConnectors/setConnector/deleteConnector/refreshConnector`. `encodeURIComponent` on `{capability}`/`{connector}` path segments; `config` sent as an object.
- `web/src/components/pages/fb-agents-page.ts` — 3 inline cards + foresight `fb-form`.
- `web/src/components/pages/fb-connectors-page.ts` — built-in + MCP cards via `fb-connector-card`; Add-MCP `fb-modal`+`fb-form` inline.
- `web/src/components/molecules/fb-connector-card.ts` — the only new molecule.
- `web/tests/agents-page.test.ts`, `web/tests/connectors-page.test.ts` — vitest suites.
- `web/tests/live/admin-config.live.spec.ts` — authenticated axe sweep + journeys (signs in as admin; runs under the live backend harness).

**Edit:**
- `web/src/types/models.ts` — add `EffectiveAgentConfig`, `AgentConfig`, `ForesightConfig`, `EffectiveConnector`, `ConnectorConfig` (`config` typed `Record<string, unknown>` + a `ForesightConfig` guard).
- `web/src/router.ts` — two route rows (gate `{roles:['owner','admin']}`, `shell:'org'`, **no** `requiresPro`; comment "NOT plan-gated — ESC-002").
- `web/src/components/shell/fb-nav-rail.ts` — two items in "Manage" (`AI Agents`/`sliders`, `Connectors`/`command`; gate `{roles:['owner','admin']}`).
- `web/src/components/pages/index.ts` — import the two new page modules.
- `web/src/api/errors.ts` — add `CAPABILITY_DISABLED` to `ErrorCode` + `userMessageForCode` copy *"The AI assistant has been turned off by an admin."*
- `web/tests/*` (router/nav) — assert both routes are admin+ AND not pro-gated; nav shows for owner+admin, hides for superintendent/field_worker.

---

## 8. A11y plan

- Every `fb-switch` gets an explicit per-item `label` (agents "Enable {friendlyTitle}", built-in "Enable reference connector", MCP "Enable {name}"). Icon-only buttons get `aria-label` ("Refresh tools for {name}", "Delete connector {name}", "Reset {capability} to default").
- `fb-secret-input` gets a distinct `label` folded into the masked-state accessible name.
- One `fb-field` per `fb-input` (number fields), each with its own label/hint/`min=1`/`step=1`/`inputmode=numeric` and an `error` prop bound to that field's translated server error.
- Notice regions: `role=${kind==='err' ? 'alert' : 'status'}`; always icon+text (never color-only). Tools-count update region `aria-live=polite`.
- Add-MCP `fb-modal`: focus the Name field on open; `fb-modal` already traps focus + Esc + restores. Foresight form errors surface via `fb-form` summary (`role=alert`).
- Focus management: restore focus to the acted-on control after each refetch; after Delete move focus to a stable target.
- **Coverage:** `web/tests/e2e/a11y.spec.ts` sweeps only `PUBLIC_ROUTES` (unauthenticated) — the new routes are authenticated. Add `web/tests/live/admin-config.live.spec.ts` (admin sign-in journey) running `AxeBuilder.withTags(['wcag2a','wcag2aa'])` on each route **including the open Add-MCP modal and a populated foresight-error state**, plus the Tab-reaches-a-named-control check. (Runs under the live backend harness, like `settings-integrations.live.spec.ts`.) Keep the default `npm run test:e2e` green (no PUBLIC_ROUTES regression).

---

## 9. Test plan

1. **agents-page (vitest):** mock `endpoints/admin.js` — renders 3 cards; switches resolve via `getByRole('switch',{name:'Enable …'})`; foresight Save coerces strings→ints and rejects empty/NaN client-side **before** any PUT; foresight 400 `details[]` maps to the correct `fb-field` error; toggling foresight sends `config:savedConfig` (both ints preserved), `delay_cascade`/`experience` send `{enabled}`; a rejected toggle reloads and resets the switch; reset (override) DELETEs+refetches; foresight reset goes through `fb-confirm`; PUT body `config` is an **object** (typeof check).
2. **agents-page dependency states:** `aiConfigured=false` → banner; assume-on/unknown → persistent neutral per-card row; the integrations link renders ONLY for `hasRole('owner')` (a real focusable `<a>`), admins get plain text.
3. **connectors-page:** built-in renders via kind branch (not name); toggling off the built-in DELETEs; MCP card renders endpoint/refresh/cred/delete even when `enabled=false`; credential derives from active-only integrations row (inactive `connector:<name>` → "no credential"); after cred Save, `fb-secret-input.submit()` leaves `value=''`; cred Save refetches integrations; partial-load (integrations rejects) → "unknown" not "no key".
4. **connectors-page Add-MCP:** invalid name blocked client-side (plain copy) before PUT; PUT 400 keeps modal open with `details[]` inline; PUT success closes + reloads, THEN refresh fires; refresh 502 shows persistent card-level UPSTREAM_ERROR copy (not generic); enabling an MCP auto-fires refresh; enabled+never-refreshed → "No tools loaded"; refreshed-zero-tools does not warn; `admin.ts` path uses `encodeURIComponent`.
5. **double-submit/focus:** rapid double-activate of Delete/Refresh/Save/Add fires once (busy gate); focus restored after refetch; focus moves to a stable target after Delete.
6. **router/nav:** `/settings/agents` and `/settings/connectors` have `gate.requiresPro !== true` and `gate.roles` deep-equals `['owner','admin']`; nav "Manage" shows both for owner+admin, hides for superintendent/field_worker; icons are `sliders`/`command`.
7. **errors.test:** `CAPABILITY_DISABLED` maps to its copy; UPSTREAM_ERROR / VALIDATION_ERROR handled by the page's explicit branches, not the generic table.
8. **live/admin-config.live.spec:** admin sign-in → client-side nav → AxeBuilder on each route (incl. open modal + populated foresight error) → zero violations + keyboard reachability.

**Gate run (non-negotiable, all green):** `cd web && npm run typecheck && npm test && npm run build && npm run lint && npm run test:e2e`. (No Go changes ⇒ no `make audit`.)

---

## 10. Risk register (top risks → mitigations)

| Risk | Mitigation |
|---|---|
| foresight tuning silently reset/clobbered/written as 0\|NaN (fb-form yields strings; toggle+Save share one full-document PUT) | coerce+validate `≥1`; toggle sends `savedConfig` snapshot; disable toggle while dirty; serialize writes; tests |
| VALIDATION_ERROR & UPSTREAM_ERROR fall through to generic copy | branch on `err.code`; `details[]` for 400; connector copy for 502; tests for both |
| switch stuck wrong after a failed PUT (self-mutate + Lit no-op) | `await load()` in `finally` after every toggle; test a rejected toggle resets it |
| credential state misrepresented (inactive bearer reads as "set") | filter `is_active`; enable vs cred orthogonal; test |
| plaintext bearer lingers in DOM | `.submit()` in `finally` (success+error); test `value===''` |
| ESC-002 regression (someone adds `requiresPro`) | router/nav test asserts `requiresPro!==true`; code comment |
| created-but-unrefreshed MCP invisible on 502 | reload-first then refresh; persistent card error + Edit-endpoint |
| enabled MCP with zero tools = silent no-op | auto-refresh after enable; persistent "No tools loaded" badge |
| name join-key path-traversal / wrong vault key | validate regex; `encodeURIComponent`; same string for both writes |
| new authenticated routes ship with ZERO axe coverage | `admin-config.live.spec.ts` axe sweep incl. modal + error states |
| 5 switches + masked field unnamed to SR | explicit labels everywhere; tests resolve via `getByRole` |
| error notices announced politely behind other speech | `role` conditional (alert/status) |
| two number inputs in one `fb-field` (2nd unnamed) | one `fb-field` per input |
| double-submit (microtask guard doesn't cover async) | explicit per-action busy flags |
| focus lost to `<body>` after refetch / Delete | restore focus to acted-on control / stable target |
| orphaned `connector:<name>` vault cred survives DELETE | delete-confirm copy + offer to delete the credential |
| partial load shows "No key set" as authoritative | render credential "unknown" when integrations fetch fails |

---

## 11. Verification criteria (definition of done)

- Both screens build + render against the live native backend; an admin can enable/disable/tune all 3 agents and create/enable/configure/refresh/credential/delete an MCP connector + toggle the built-in `reference` — all from the UI, no curl.
- foresight tuning round-trips correctly (no silent reset); the experience kill-switch is reachable for admins regardless of plan tier.
- All §9 gates green; new authenticated routes axe-clean + keyboard-reachable under the live harness; no PUBLIC_ROUTES a11y regression.
- No secret material ever rendered; connector creds write-only with `last4`-only display.
- Pause for owner review (`/code-review max` local, optionally `/code-review ultra` cloud) → triage → owner merge decision (Claude does not merge/push).
