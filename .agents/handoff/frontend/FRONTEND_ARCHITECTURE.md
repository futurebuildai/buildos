# Frontend Architecture — BuildOS (Standalone / Native pivot)

**Document ID:** FE-01-ARCH
**System:** BuildOS (System of Execution)
**Status:** DRAFT — for owner ratification
**Author role:** Frontend architect
**Date:** 2026-06-01

> **Scope note.** This document is a spec only. No application code is produced here. It extends — and must not contradict — the existing handoff specs:
> - [DESIGN_SYSTEM.md](../DESIGN_SYSTEM.md) (GableLBM Industrial Dark, Lit 3.0, dark-only)
> - [INFORMATION_ARCHITECTURE.md](../INFORMATION_ARCHITECTURE.md) (three-workspace model)
> - [ARCHITECTURE.md](../ARCHITECTURE.md) (Go backend package layout)
> - [PRD.md](../PRD.md) / [PERSONAS.md](../PERSONAS.md)
> - [.agents/TECH_STACK.md](../../TECH_STACK.md) (single source of truth for tech choices)
>
> Where this document and an older spec conflict (the architectural pivot below changed several assumptions), the conflict is called out explicitly in **§10 Open questions / decisions** and the newer position wins **only after owner ratification**. Until then, treat the older spec's wording as the default and this doc's wording as the proposed delta.

---

## 0. The pivot, in one paragraph

BuildOS is dropping its dependency on the external **"The Brain"** service and becoming a fully standalone, native application per deployment (still single-tenant per customer fork, per ADR-002). For the frontend this changes five things:

1. **Auth is native.** The app mints its own JWTs. Login is native email + password; there is **no external OIDC/IdP redirect**. Sessions use a short-lived access token + a long-lived refresh token. Password reset is via email (Resend). First-run uses a one-time **bootstrap token** to create the first owner.
2. **Integrations are BYOK** (bring-your-own-key), configured **in-app** by owner/admin, stored in an encrypted credential vault. Providers: `anthropic`, `resend`, `gable`, `localblue`. New UI: set / list / delete keys.
3. **A2A webhooks and billing UI are removed.** No screens for either.
4. **AI features are native** (direct Anthropic Messages API) and only function once an Anthropic key is configured. The client must detect this and degrade gracefully (the **soft-fail 503** pattern, §4.3).
5. **No external identity, no plan-tier metering as a hard product gate.** RBAC is local (`owner > admin > superintendent > field_worker`); any plan-tier gating that survives the pivot is treated by the client as a soft, server-driven capability flag, not an OIDC scope.

Backend reality check (already in code at the time of writing): the native primitives exist — `internal/ai` (Anthropic client, returns `ErrUnconfigured` when no key), `internal/mailer` (Resend, returns `ErrMailerUnconfigured`), `internal/cryptobox` (AES-256-GCM vault for at-rest BYOK secrets), and the setup wizard + bootstrap-token flow (`internal/api/setup.go`). The legacy JWKS/Brain auth middleware (`internal/api/middleware/auth.go`) is the surface being replaced by native token minting/validation.

---

## 1. Surfaces & target platforms

BuildOS ships **two frontend codebases**, each in its own repo (this repo is backend-only). They map onto the three workspaces from INFORMATION_ARCHITECTURE.md §1.

| Surface | Codebase | Platform | Workspaces served | Primary personas / roles |
|---|---|---|---|---|
| **Office Console** | `buildos-web` (Vite + Lit) | Web, desktop-first (≥1200px), tablet-capable | Portfolio Dashboard, Agent Command Center, **Setup wizard**, **Settings → Integrations (BYOK)** | Tom (owner), Sarah (admin), Mike (superintendent, web) |
| **Field App** | `buildos-mobile` (Flutter) | iOS + Android, offline-first | Field Portal | Carlos (field_worker), subcontractor crew leads; Mike uses it secondarily in the field |

### 1.1 Role → surface → workspace matrix

Derived from PERSONAS.md "Persona Access Matrix" and `internal/api/router.go` RBAC gates.

| Role | Office Console | Field App | Highest-value workspace |
|---|---|---|---|
| `owner` | Full (Portfolio + Command Center + Settings + Integrations + Setup) | Light/read | Portfolio Dashboard |
| `admin` | Full (same as owner except some owner-only approvals) | Occasional | Portfolio (financials) |
| `superintendent` | Command Center (read most financials, write schedule/tasks, request vendor review) | Heavy | Agent Command Center |
| `field_worker` | **No console access** (login lands them on a "use the mobile app" screen) | **Primary** | Field Portal |

**RBAC is enforced server-side** (router.go is the source of truth). The client mirrors the same matrix purely for UX (hide/disable controls the user can't use) and must **never** treat client-side hiding as a security control. A `403 FORBIDDEN` from the API is always handled gracefully (§4.3).

### 1.2 Authoritative backend route inventory (from `internal/api/router.go`)

The client is built against these routes and their RBAC gates. New native routes (auth, integrations) are proposed in §3.5 / §10 and are **not yet in router.go** — they're flagged as backend prerequisites.

| Area | Route | Methods | RBAC gate (server) |
|---|---|---|---|
| Probes | `/health`, `/ready` | GET | none |
| Setup wizard | `/api/v1/setup/bootstrap` | POST | auth only (token IS the grant) |
| Setup wizard | `/api/v1/setup/state` | GET | admin+ |
| Setup wizard | `/api/v1/setup/{company-info,trades,cost-codes,calendars,calendars/{id}/holidays,jurisdictions,complete}` | POST | admin+ |
| Projects | `/api/v1/projects` | GET (all), POST (owner/admin) | mixed |
| Projects | `/api/v1/projects/{projectID}` | GET (all), PUT (owner/admin) | mixed |
| Schedule | `/api/v1/projects/{projectID}/schedule/recalculate` | POST | superintendent+ |
| Schedule | `/api/v1/projects/{projectID}/schedule/gantt` | GET | all |
| Schedule (AI) | `/api/v1/projects/{projectID}/schedule/recommend-adjustments` | POST | superintendent+ (+ AI-configured) |
| Tasks | `/api/v1/projects/{projectID}/tasks` | GET (all), PUT task (superintendent+) | mixed |
| Budgets | `/api/v1/projects/{projectID}/budgets` | GET | owner/admin |
| Invoices | `/api/v1/projects/{projectID}/invoices` | POST, PUT | owner/admin |
| Procurement | `/api/v1/projects/{projectID}/procurement` | GET (all), POST/PUT (owner/admin), `{itemID}/request-review` (superintendent+) | mixed |
| Financials | `/api/v1/org/{orgID}/financials/{summary,ar-aging,projects}` | GET | superintendent+ (ar-aging/projects owner/admin) |
| Pipeline | `/api/v1/org/{orgID}/pipeline/prospects…` | GET/POST/PUT | superintendent+ (writes owner/admin) |
| Fleet | `/api/v1/org/{orgID}/fleet` | GET/POST/`{id}/allocate` | superintendent+ (create owner/admin) |
| HR | `/api/v1/org/{orgID}/employees…` | GET | owner/admin |
| Feed | `/api/v1/feed`, `/{cardID}/action`, `/{cardID}/dismiss` | GET/POST | all |
| AI agents | `/api/v1/agents/daily-briefing` | POST | (AI-configured; see §4.3) |
| Field | `/api/v1/field/{sync,progress,checkin,daily-log}` | GET/POST | all (primarily field_worker) |

> **Removed by the pivot:** `/api/v1/billing/*` and the inbound `/api/v1/a2a/webhook` are **not** frontend surfaces. The webhook receiver may remain server-side for legacy reasons but has zero UI. Billing has zero UI.
>
> **`plan_tier` gates** (`RequirePlanTier(PlanTierPro)` on AI routes) survive only as long as the backend keeps them; the client treats a `402`/`403` on AI routes identically to "AI not available — see §4.3" rather than rendering an upsell, because there is no billing surface to upsell into.

---

## 2. Per-surface tech architecture

### 2.1 Office Console (Lit web)

Aligned with TECH_STACK.md (Vite + Lit, TypeScript strict, Vanilla CSS tokens) and DESIGN_SYSTEM.md (Lit 3.0, `FBElement` base, dark-only).

| Concern | Choice | Notes |
|---|---|---|
| **Language** | TypeScript (strict mode) | `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes` on |
| **Framework** | Lit 3.x Web Components | `FBElement` base class per DESIGN_SYSTEM.md §8 |
| **Bundler / dev** | Vite | per TECH_STACK.md |
| **Styling** | Vanilla CSS with CSS custom-property design tokens | `styles/variables.css` holds the GableLBM tokens. **See §10 OQ-1: DESIGN_SYSTEM.md / ARCHITECTURE.md mention Tailwind CSS 4 — this conflicts with TECH_STACK.md "Vanilla CSS". Defaulting to Vanilla CSS per the single source of truth.** |
| **Routing** | Lightweight client router (`@lit-labs/router` or a thin custom router over the History API) | Routes per IA §5.1, adjusted in §3 below |
| **State** | Lit Signals (`@lit-labs/signals` / `@preact/signals-core`) | Matches ARCHITECTURE.md's "Signals-based state management". One global `authStore`, one `capabilityStore`, per-workspace stores. No Redux. |
| **Data fetching** | Single typed API client module (§4) wrapping `fetch` | No axios; native `fetch` keeps the dep tree minimal per the "no deps not in TECH_STACK" rule |
| **Charts** | D3.js (AR aging, Gantt) | Already named in DESIGN_SYSTEM.md component catalog |
| **Forms / validation** | Native + a tiny schema validator (e.g. `zod` if added to TECH_STACK; otherwise hand-rolled) | **OQ-5**: `zod` is not in TECH_STACK.md — flag before adding |
| **i18n** | English first; string-table abstraction from day one | Spanish is required for Field (Carlos); console is English-only for v1 |
| **Testing** | Vitest (unit), Playwright (E2E, per TECH_STACK.md), `@open-wc/testing` for component tests | c8 for coverage (TECH_STACK.md) |

**Package layout (`buildos-web/`):**

```
buildos-web/
├── src/
│   ├── api/                 # typed client: client.ts, endpoints/*.ts, errors.ts, tokens.ts
│   ├── auth/                # authStore, login/refresh/logout flows, route guards
│   ├── capability/          # capabilityStore — feature availability (AI/email configured?)
│   ├── components/
│   │   ├── base/fb-element.ts
│   │   ├── atoms/ molecules/ organisms/ pages/   # per DESIGN_SYSTEM.md §9
│   ├── state/               # signals stores per workspace
│   ├── router.ts
│   ├── styles/variables.css # GableLBM tokens
│   └── main.ts
├── tests/ (vitest + playwright)
├── index.html
├── vite.config.ts
└── package.json
```

### 2.2 Field App (Flutter)

Aligned with TECH_STACK.md (Flutter, field-only, offline-first) and IA §9 (Drift outbox).

| Concern | Choice | Notes |
|---|---|---|
| **Language / SDK** | Dart / Flutter (stable channel) | Field surfaces only |
| **Routing** | `go_router` | Declarative, deep-link friendly; matches IA §5.2 routes |
| **State** | Riverpod (or `flutter_bloc`) | **OQ-6**: pick one; Riverpod recommended for testability. ARCHITECTURE.md names no specific state lib for Flutter. |
| **Local DB** | Drift (SQLite) | Per ARCHITECTURE.md mobile tree + IA §9 |
| **Offline sync** | Outbox table + `workmanager` background drain + `connectivity_plus` | Per IA §9 architecture diagram |
| **API client** | `dio` or `http` with an interceptor mirroring the web client's 401→refresh→retry (§4.2) | Token storage via `flutter_secure_storage` (Keychain/Keystore) |
| **Push** | FCM (`firebase_messaging`) for feed/notification wake-ups | ARCHITECTURE.md `push_service.dart` |
| **Camera/photo** | `image_picker` + on-device compression before upload | Photo progress reporting |
| **i18n** | `flutter_localizations` + ARB files; **Spanish + English required** | Carlos persona: bilingual, low text density |
| **Testing** | `flutter_test` (unit/widget), `integration_test`, golden tests for offline states | |

**Package layout** follows ARCHITECTURE.md §2 `mobile/lib/` tree (`database/`, `services/{sync,auth,push}_service.dart`, `screens/`, `widgets/`). Add `services/credential_aware.dart` is **not** needed — BYOK config is web-only (owner/admin); the field app only consumes whatever capability flags `/field/sync` exposes.

---

## 3. Routing & screen deltas from the pivot

Base routes are in IA §5.1–§5.2. The pivot **changes the auth/settings family** and **adds the integrations family**. Below are only the deltas; everything else in IA §5 stands.

### 3.1 Auth routes (replace the OIDC redirect)

IA §5.1 currently says `/login → The Brain OIDC redirect`. Replace with native screens:

```
/login                  → native email + password form (no redirect)
/forgot-password        → request reset email (Resend) by email address
/reset-password?token=  → set new password using emailed token
/first-run              → bootstrap-token entry → create first owner (email+password)
                          → on success, app holds the minted access token and routes to /setup
/logout                 → clears tokens, revokes refresh token server-side, → /login
```

- `/first-run` is reachable only when the deployment has no owner yet. The client probes a public, unauthenticated endpoint (proposed: `GET /api/v1/auth/state` → `{ "needs_bootstrap": true|false }`, **OQ-2**) to decide whether `/login` or `/first-run` is the cold-start landing.
- After bootstrap or login, **SetupGate** still applies: an org with `onboarding_complete=false` gets `403 SETUP_INCOMPLETE` on operational routes (see `internal/api/middleware/setup_gate.go`). The client treats `SETUP_INCOMPLETE` as "redirect to `/setup`" regardless of which screen the user was trying to reach (§4.3).

### 3.2 Setup wizard routes (already backed by `internal/api/setup.go`)

```
/setup                  → wizard shell (admin+); redirect target for SETUP_INCOMPLETE
/setup/company-info     → step 1  (POST /api/v1/setup/company-info)
/setup/trades           → step 2  (POST /api/v1/setup/trades)
/setup/cost-codes       → step 3  (POST /api/v1/setup/cost-codes)
/setup/calendar         → step 4  (POST /api/v1/setup/calendars + .../holidays)
/setup/jurisdictions    → step 5  (POST /api/v1/setup/jurisdictions)
/setup/integrations     → step 6 (NEW) — prompt to add at least an Anthropic key (skippable)
/setup/review           → GET /api/v1/setup/state → POST /api/v1/setup/complete
```

The wizard reads `GET /api/v1/setup/state` to render progress and gate the **Complete** button on the server-enforced prereqs (legal name, ≥1 trade, ≥1 cost code, a default calendar). Integration setup is **encouraged but not a hard prereq** for `complete` (the backend's `Complete` does not require keys), so AI simply stays "unconfigured" until a key is added later.

### 3.3 Settings → Integrations (BYOK) — NEW family

Replaces IA §5.1 `/settings/brain`. There is no "Brain integration" anymore.

```
/settings/integrations            → list configured providers + status (owner/admin only)
/settings/integrations/:provider  → set / rotate / delete key for one provider
```

Providers and their UX semantics:

| Provider | Used for | If unset, what breaks | UI affordance |
|---|---|---|---|
| `anthropic` | All AI features (briefing, invoice extraction, procurement recs, schedule adjust, intent, tribunal) | AI features show "not configured" (soft-fail) | Required-to-unlock-AI banner |
| `resend` | Transactional email (password reset, notifications) | Password-reset emails silently no-op; in-app shows a warning to owner | Warning if unset |
| `gable` | Gable integration (LBM supplier) | Gable-dependent features disabled | Optional |
| `localblue` | LocalBlue lead capture | LocalBlue lead inflow disabled | Optional |

**Security UX rules for the key vault (the keys are PII-Restricted secret material, per `internal/cryptobox`):**
- Keys are **write-only from the UI**. The list view shows provider, status (`configured` / `not configured`), a non-reversible fingerprint or last-4 (server-provided; never the key), `created_at`, and `created_by`. The raw key is **never** returned by the API and never rendered.
- Setting a key is a single POST; the field is masked, paste-friendly, with an explicit "Save" (no autosave). A successful save shows only "saved" — never echoes the value.
- Delete requires a confirm dialog (destructive styling per DESIGN_SYSTEM.md `.btn-destructive`).
- Only `owner`/`admin` can reach this family. `superintendent`/`field_worker` get `403` → "ask your owner/admin."
- These screens require backend routes that **do not yet exist** in router.go — see §3.5 / OQ-3.

### 3.4 Removed routes

- `/settings/brain` — deleted.
- Any billing route/screen — none ever to be added.
- A2A configuration UI — none.

### 3.5 Backend route prerequisites (NOT yet in router.go — backend must add)

The frontend is blocked on these. They are proposed contracts; the **API_CONTRACT.md is the binding source** once updated:

| Proposed route | Purpose | Auth |
|---|---|---|
| `POST /api/v1/auth/login` | email+password → `{access_token, refresh_token, expires_in}` | public |
| `POST /api/v1/auth/refresh` | rotate refresh → new access (+ rotated refresh) | refresh token |
| `POST /api/v1/auth/logout` | revoke refresh token | access token |
| `POST /api/v1/auth/password/forgot` | trigger Resend reset email | public |
| `POST /api/v1/auth/password/reset` | token + new password | public (token) |
| `GET  /api/v1/auth/state` | `{needs_bootstrap, setup_complete}` for cold-start routing | public |
| `GET  /api/v1/capabilities` | `{ai_configured, email_configured, providers:[…]}` | access token |
| `GET/POST/DELETE /api/v1/integrations/{provider}` | BYOK vault CRUD (write-only key) | owner/admin |

---

## 4. Auth, session & API client model (client side)

### 4.1 Token storage

Native JWTs: a short-lived **access token** (recommend 15 min) and a long-lived **refresh token** (recommend 7–30 days, rotating).

**Office Console (web):**
- **Access token:** held **in memory only** (a signal in `authStore`). Never in `localStorage` (XSS exfiltration risk).
- **Refresh token:** the **strongly preferred** design is an **`HttpOnly; Secure; SameSite=Strict` cookie** set by the backend on login/refresh, so JS never touches it. The `/auth/refresh` call then needs no body — the cookie rides along. **OQ-4**: if the backend cannot set cookies (pure bearer model), fall back to refresh token in memory + a "remember me" that re-prompts on reload; do **not** put refresh tokens in `localStorage`.
- On hard reload, the in-memory access token is gone; the app calls `/auth/refresh` once at boot (cookie-backed) to silently re-establish the session, else routes to `/login`.

**Field App (Flutter):**
- Both tokens in `flutter_secure_storage` (OS Keychain/Keystore). Offline-first means the field app must survive long offline periods; the access token will expire, so the refresh token is the durable credential. If both are expired and the device is offline, the app stays in **read-only cached mode** and queues writes in the Drift outbox until a refresh succeeds online.

### 4.2 The 401 → refresh → retry interceptor (both surfaces)

Single choke-point in the API client. Pseudocode contract (not implementation):

```
async function request(req):
    attachAccessToken(req)
    res = await fetch(req)
    if res.status != 401: return res
    # 401: try exactly one refresh, single-flight (concurrent 401s share one refresh promise)
    ok = await refreshOnce()          # POST /auth/refresh; rotates refresh token
    if not ok:
        authStore.clear(); route('/login'); throw SessionExpired
    attachAccessToken(req)            # new token
    return await fetch(req)           # retry once; a second 401 → logout
```

- **Single-flight refresh:** concurrent requests that all 401 must await one shared refresh, not stampede the refresh endpoint.
- **Refresh rotation:** the backend returns a new refresh token on every refresh; the client replaces the stored one. A refresh that itself 401s ⇒ full logout.
- **No infinite loops:** retry the original request at most once.

### 4.3 Soft-fail / unconfigured (the 503 / capability pattern)

This is the central pivot-specific UX pattern. Because AI and email are BYOK, a perfectly healthy deployment may simply have no Anthropic key. The backend signals this distinctly:

- `internal/ai` returns `ErrUnconfigured` when no Anthropic key is set → handlers surface a **`503 SERVICE_UNAVAILABLE`** with an error code the client can recognize (proposed code `AI_UNCONFIGURED`; **OQ-7** to confirm in the error taxonomy).
- `internal/mailer` returns `ErrMailerUnconfigured` (soft — best-effort; the originating request does **not** fail).

**Client rules:**
1. **Proactive capability gating.** On login, the client fetches `GET /api/v1/capabilities` and caches it in `capabilityStore`. AI-driven affordances (Generate Briefing, Extract Invoice, Recommend Adjustments, Recommend Vendors, Tribunal Review) render in a **disabled "AI not configured"** state with a one-click deep link to `/settings/integrations/anthropic` (owner/admin) or an explanatory tooltip (other roles) when `ai_configured == false`. This avoids letting users trigger an action that can only 503.
2. **Reactive soft-fail.** If a capability flips between fetch and call (race), an AI call returns `503 AI_UNCONFIGURED`. The client shows a non-destructive inline notice ("AI isn't set up yet — add an Anthropic key in Settings → Integrations"), **not** a red error toast, and refreshes `capabilityStore`. Distinguish this clearly from `502/503` transient upstream errors (`ai: upstream transient` / circuit-open), which DO get a retry affordance and a normal error toast.
3. **Email is invisible-soft.** A flow that triggers email (e.g. password reset request) always shows the same neutral confirmation regardless of whether `resend` is configured (avoids account enumeration + matches `ErrMailerUnconfigured` being non-fatal). Owner/admin separately see a Settings warning if `email_configured == false`.
4. **Error taxonomy mapping** (client must branch on the machine code, not the HTTP status alone):

| Server code | HTTP | Client treatment |
|---|---|---|
| `AI_UNCONFIGURED` | 503 | Soft notice + deep link to integrations; refresh capabilities. Not an error toast. |
| `SERVICE_UNAVAILABLE` (AI transient/circuit) | 502/503 | Error toast + Retry. |
| `RATE_LIMITED` | 429 | Toast with backoff hint; auto-retry honoring `Retry-After`. |
| `SETUP_INCOMPLETE` | 403 | Redirect to `/setup`. |
| `UNAUTHORIZED` | 401 | 401-interceptor (§4.2). |
| `FORBIDDEN` | 403 | "You don't have access" — hide/disable the control going forward. |
| `VALIDATION_ERROR` | 400 | Inline field errors. |
| `NOT_FOUND` | 404 | Empty state. |

### 4.4 Logout

`/auth/logout` posts the refresh token (or relies on the HttpOnly cookie) so the server can revoke it; the client then clears `authStore`, `capabilityStore`, any per-workspace caches, and routes to `/login`. On the field app, logout also warns if the outbox is non-empty (unsynced field work would be lost) and blocks until drained or explicitly discarded.

---

## 5. AI feature surfaces (native)

All AI runs through `internal/ai` → Anthropic directly. There is **no Maestro/Brain proxy**, no token/cost metadata surfaced to the user (those were Brain concerns; `internal/ai` carries none). Client-relevant AI surfaces:

| Feature | Where (workspace/route) | Endpoint | Special UI |
|---|---|---|---|
| **Daily briefing** | Command Center → Briefing | `POST /api/v1/agents/daily-briefing` | Called on workspace open; skeleton while generating; soft-fail if unconfigured |
| **Invoice extraction (+ image upload)** | Portfolio → project invoices | (invoice create flow → AI extract) | **Image upload**: client uploads/links a document image (jpeg/png/gif/webp, ≤5 MB per `internal/ai` `defaultMaxImageBytes`); show progress, then a review/confirm form pre-filled with extracted fields before persist. Map `ErrImageTooLarge`/`ErrUnsupportedMediaType` to inline 400s. |
| **Procurement recommendations** | Command Center → Procurement | `…/procurement/{itemID}/request-review` + recs | Recommendation cards; confidence as integer % (no floats); amounts as `*_cents` + `currency_code` |
| **Schedule adjustment** | Command Center → Schedule | `…/schedule/recommend-adjustments` | Shows applied deltas + a "recalc may lag" hint; re-fetch gantt after |
| **Intent classification / tribunal review** | Background / chat-adjacent | (server-side) | Surfaced as feed cards / activity; no dedicated input screen needed for v1 |

Every AI surface respects §4.3. Invoice image upload must enforce the same size/type limits client-side **before** upload for instant feedback, but the server limit is authoritative.

---

## 6. Build tooling, env config & feature discovery

### 6.1 Environment config

The only required runtime config the client needs is the **API base URL**. Everything else (which features are on) is **discovered at runtime** from the server, not baked at build time — this keeps a single web build deployable across forks.

| Var (web, Vite) | Example | Notes |
|---|---|---|
| `VITE_API_BASE_URL` | `https://acme.buildos.example` or `/` (same-origin) | If the console is served same-origin behind the Go server, prefer relative `/api/v1`. |
| `VITE_SENTRY_DSN` | (optional) | Frontend error tracking; mirrors backend Sentry posture (empty = disabled). PII scrubbing rules from CLAUDE.md apply on the client too. |

| Var (Flutter) | Notes |
|---|---|
| `--dart-define=API_BASE_URL=…` | Per-flavor (dev/staging/prod) build config |
| `--dart-define=FCM_*` | Push config |

**No** `anthropic`/`resend`/`gable`/`localblue` keys ever live in frontend config or build args — they are server-side vault material set via the Integrations UI.

### 6.2 Feature availability discovery

Two layers:
1. **Capability flags** — `GET /api/v1/capabilities` returns `{ ai_configured, email_configured, providers }`. Cached in `capabilityStore`, refreshed on login, after any Integrations mutation, and after any `AI_UNCONFIGURED` soft-fail. Drives proactive gating (§4.3.1).
2. **Role flags** — from the access token claims (`role`, `org_id`, `sub`) the client mirrors the RBAC matrix for UX only.

The client must function (read-only where applicable) even if `/capabilities` fails — treat unknown capabilities as "assume on, fall back to reactive soft-fail," so a capabilities outage never hard-bricks the UI.

### 6.3 CI / quality gates (frontend repos)

Mirror backend rigor (TECH_STACK.md): ESLint + Prettier + `tsc --noEmit` on web; `flutter analyze` + `dart format` on mobile; Vitest/`flutter test` unit; Playwright/`integration_test` E2E; the **`eslint-plugin-fb` composite-currency rule** (TECH_STACK.md §Constraints) — `number`-typed monetary props are flagged unless named `*Cents` with a sibling `*CurrencyCode`. Accessibility checks (axe via Playwright) run in CI per §7.

---

## 7. Accessibility (WCAG 2.1 AA) & field ergonomics

Extends DESIGN_SYSTEM.md §11. These are baselines, enforced in CI where automatable.

**Console (WCAG 2.1 AA):**
- Contrast: AA (4.5:1 body / 3:1 large) — the GableLBM palette already meets this (DESIGN_SYSTEM.md §11); CI runs axe on every page route.
- Keyboard: full operability; tab order = visual order; `:focus-visible` 2px Gable Green outline; no `outline:none` without a replacement; modals/command palettes trap focus and restore it on close.
- Screen readers: ARIA landmarks (`navigation`/`main`/`complementary`); live regions for toasts and for AI "generating…" status; data tables use proper `<th scope>`/row headers; Gantt and AR-aging D3 charts ship an accessible table fallback (charts are not keyboard-accessible alone).
- Status never by color alone (WCAG 1.4.1): pair color with icon + text (DESIGN_SYSTEM.md §11 error rule) — critical for the green/red financial variance coloring.
- `prefers-reduced-motion`: disable transforms/shimmer, keep opacity (DESIGN_SYSTEM.md §10.2).
- Forms: programmatic label association, inline error text + `aria-invalid`, error summary on submit.

**Field App (AA + field ergonomics — Carlos persona):**
- Touch targets **≥48px**, Field Portal uses **64px** (DESIGN_SYSTEM.md §11).
- One-handed, glove-friendly: primary actions reachable in the thumb zone; large hit areas; minimal typing (sliders, one-tap, voice-to-text for daily logs per PERSONAS.md).
- **Bilingual (Spanish/English)** with a language toggle that does not require reading English to find.
- High outdoor-sunlight legibility: rely on the dark theme's high-contrast tokens; avoid low-contrast glass effects on critical field text (glassmorphism is a console aesthetic; field uses solid surfaces).
- Offline indicators are mandatory and non-color-only: amber badge + count + text (IA §9, DESIGN_SYSTEM.md §10.1 Offline).
- Respect OS font-scaling / dynamic type.

**Responsive (console)** per DESIGN_SYSTEM.md §10.3 breakpoints: 3-panel ≥1440, 2-panel ≥1200, collapsible sidebar ≥768, bottom-nav overlay <768. `field_worker` who somehow opens the console on mobile gets the "use the app" screen, not a degraded console.

---

## 8. Offline architecture (Field App) — affirming IA §9

No change from IA §9; restated as binding for the build:
- **Outbox table** (`id, action, payload, idempotency_key, status`) in Drift. Every mutating field action (progress, checkin, daily-log, photo) writes the outbox first, renders optimistic UI, then drains.
- **Drain**: `workmanager` background task + `connectivity_plus` trigger; POSTs carry `idempotency_key` so server-side dedup (`internal/store` field sync) makes retries safe.
- **Pull**: `GET /api/v1/field/sync?since=<ts>` returns missed updates; **server-wins** conflict resolution; client re-fetches after a successful drain.
- **Visibility**: Sync Status screen shows outbox count + last-sync time + connectivity dot (green online / amber offline-with-pending).
- Auth interaction: outbox drain runs through the same 401→refresh→retry interceptor; if refresh fails offline, items stay queued (not dropped).

---

## 9. Cross-cutting conventions

- **Currency**: all money arrives as `*_cents` (BIGINT) + `currency_code`. Format on display only (DESIGN_SYSTEM.md §15); never do float math; never sum across `currency_code` (group by it). Render in JetBrains Mono via `data-currency`.
- **Numbers/IDs/dates/durations/percentages**: JetBrains Mono (DESIGN_SYSTEM.md §3.2).
- **Loading**: skeleton shimmer (`.skeleton`) for content; spinner only for in-button/inline waits; AI generation uses skeleton + an ARIA live "generating…".
- **Errors**: branch on machine error code (§4.3 table), not raw status; destructive actions always confirm.
- **PII on the client**: never log access tokens, refresh tokens, emails, or any BYOK key to console/Sentry; apply the backend's Restricted-class redaction posture (CLAUDE.md "PII handling") to frontend Sentry `beforeSend`.
- **Empty states**: every list/table/chart has a designed empty state (DESIGN_SYSTEM.md §10.1).

---

## 10. Open questions / decisions for the owner

| # | Question | Default if unanswered | Why it matters |
|---|---|---|---|
| **OQ-1** | **Styling stack**: TECH_STACK.md says **Vanilla CSS**; DESIGN_SYSTEM.md §2.4 + ARCHITECTURE.md §2 reference **Tailwind CSS 4**. Which is binding? | **Vanilla CSS + CSS custom properties** (TECH_STACK.md is the single source of truth) | Affects every component, the build, and the design-token plumbing. Needs one answer before component work starts. |
| **OQ-2** | Cold-start routing endpoint — confirm `GET /api/v1/auth/state` (`needs_bootstrap`, `setup_complete`) as a public, unauth probe. | Build against this shape; backend to confirm | Without it the client can't decide `/login` vs `/first-run`. |
| **OQ-3** | Confirm the BYOK integrations API contract (`GET/POST/DELETE /api/v1/integrations/{provider}`), incl. that GET never returns the key (only status + fingerprint/last-4 + metadata). | Write-only, status+fingerprint only | Security-critical; the vault holds Restricted secret material. |
| **OQ-4** | Refresh-token transport on web: **HttpOnly cookie** (preferred) vs bearer-in-memory. Can the backend set `HttpOnly; Secure; SameSite=Strict` cookies on the same origin? | HttpOnly cookie | Drives XSS/CSRF posture and the silent-reauth-on-reload flow. If cookie, also confirm CSRF strategy (double-submit token or SameSite-only). |
| **OQ-5** | May we add `zod` (web) for runtime schema validation of API responses? Not in TECH_STACK.md. | Hand-rolled validators until approved | Dependency-policy gate. |
| **OQ-6** | Flutter state management: **Riverpod** vs Bloc — ARCHITECTURE.md doesn't specify. | Riverpod | Team familiarity / testability tradeoff. |
| **OQ-7** | Confirm the canonical **error codes** for `AI_UNCONFIGURED` vs transient AI failures in API_CONTRACT.md / error taxonomy, so the client can branch soft-fail vs retry correctly. | `AI_UNCONFIGURED` (503) for unset key; `SERVICE_UNAVAILABLE` (502/503) for transient | The whole soft-fail UX (§4.3) hinges on distinguishing these. |
| **OQ-8** | Access/refresh token TTLs and whether refresh rotation is mandatory server-side. | 15 min access / 7–30 day rotating refresh | Drives the interceptor and the field app's offline tolerance window. |
| **OQ-9** | ~~Does any `plan_tier` gating survive the standalone pivot?~~ **RESOLVED (ESC-002, 2026-06-09):** removed entirely — AI endpoints are role-gated only. Drop `requiresPro` from web routes; map any residual tier 402/403 to the same "AI unavailable" soft notice (no upsell). | Treat as removed | There is no billing UI to upsell into. |
| **OQ-10** | Should the legacy `/api/v1/a2a/webhook` and `/api/v1/billing/*` be formally removed from API_CONTRACT.md so the frontend never references them? | Treat as non-frontend; no UI | Avoids dead references in the client. |
| **OQ-11** | Console for `field_worker`: hard-block to a "use the mobile app" screen, or allow read-only? | Hard-block landing screen | UX + scope. |

---

## 11. Summary

- **Two surfaces**: Lit web **Office Console** (Portfolio + Command Center + Setup + Integrations) and Flutter **Field App** (offline-first Field Portal). Role→surface mapping is RBAC-mirrored client-side, server-enforced.
- **Native auth replaces OIDC**: in-memory access token + (preferably) HttpOnly-cookie rotating refresh token; single-flight **401→refresh→retry** interceptor; native login / forgot / reset / first-run-bootstrap screens.
- **BYOK integrations** (`anthropic`, `resend`, `gable`, `localblue`) via a write-only, owner/admin-only vault UI; keys never returned to the client.
- **Soft-fail capability pattern** is the headline pivot UX: `GET /api/v1/capabilities` drives proactive gating of AI affordances; `AI_UNCONFIGURED` (503) is a soft notice with a deep link, distinct from transient `502/503` (retry). Email is invisible-soft.
- **No billing UI, no A2A UI.**
- Build-time config is just the **API base URL**; all feature availability is **runtime-discovered**.
- WCAG 2.1 AA across both surfaces; 48/64px touch targets, bilingual, mandatory offline indicators on field.
- Aligned with TECH_STACK.md (Lit/Vite/TS/Vanilla-CSS, Flutter/Drift, Anthropic-only AI, composite-currency lint) and DESIGN_SYSTEM.md (GableLBM Industrial Dark, dark-only).
- **11 open questions** in §10 need owner answers before component-level work; the styling-stack conflict (OQ-1), the auth/integration API contracts (OQ-2/3/4), and the AI error taxonomy (OQ-7) are the blockers.
