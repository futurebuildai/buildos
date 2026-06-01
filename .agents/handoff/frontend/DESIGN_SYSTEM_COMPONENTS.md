# Design System — Component Library & IA (Standalone/Native Pivot)

**Document ID:** AG-05-DSC
**System:** BuildOS (System of Execution)
**Created:** 2026-06-01
**Pipeline Stage:** 05 — Design System (component layer)
**Status:** DRAFT — extends, does not replace, `DESIGN_SYSTEM.md` + `INFORMATION_ARCHITECTURE.md`
**Source of truth it builds on:** `DESIGN_SYSTEM.md` (AG-05-DS), `INFORMATION_ARCHITECTURE.md` (AG-05-IA), `PERSONAS.md`, `USER_JOURNEYS.md`, `internal/api/router.go`

---

## 0. Scope & Pivot Reconciliation

This document is the **component library + consolidated IA/app-shell spec**. It extends the two existing AG-05 specs. Where the existing specs are still correct (tokens, glassmorphism, Industrial Dark, JetBrains-Mono-for-data), this document references them and does not restate them in full. Where the **standalone/native pivot** has invalidated an existing spec, the conflict is flagged inline and consolidated in §13.

**Pivot deltas that drive this spec (vs. the AG-05 baseline):**

| # | Pivot change | DS/IA impact |
|---|---|---|
| P1 | **"The Brain" removed.** No external OIDC provider, no Maestro gateway, no Hub credential vault, no Billing engine, no A2A. | `DESIGN_SYSTEM.md` "Brain integration" settings surface and `INFORMATION_ARCHITECTURE.md` `/login → Brain OIDC redirect` are dead. Replaced by native auth screens + in-app BYOK vault. Backend evidence: `internal/ai` (native Anthropic client, `KeyResolver`), `internal/mailer/resend.go`, `internal/cryptobox` (at-rest credential encryption). |
| P2 | **Native email/password auth.** | New shared screens: Sign in, Set password (bootstrap-token owner claim), Forgot/Reset password, MFA (future). New components: auth form, masked password input, bootstrap-token field. Backend evidence: `setup_bootstrap_tokens`, `RedeemBootstrapTokenForSubject` in `internal/service/setup.go`. |
| P3 | **In-app BYOK integrations vault.** Per-org keys for Anthropic, Resend, Gable, LocalBlue stored encrypted in the fork. | New Settings → Integrations surface; new components: masked-secret input, key-status badge, "feature requires a key" empty states, test-connection button. Backend evidence: `cryptobox.Seal/Open`, `ai.KeyResolver`, `mailer.KeyResolver`. |
| P4 | **A2A + billing removed.** | Remove A2A nav/settings; remove `/api/v1/billing/*` usage views from Portfolio. "AI usage" becomes a local meter (token/cost from `internal/ai`), not a Brain-billing proxy. |
| P5 | **File/image upload for invoice extraction** is now first-party (Anthropic vision via `internal/ai/image.go`), not a Brain proxy. | New upload + extraction-review component. |

Everything else in `DESIGN_SYSTEM.md` (color, type, spacing, elevation, glass, motion, dark-only) is **carried forward unchanged** unless §13 says otherwise.

---

## 1. Consolidated Navigation / IA Model

### 1.1 Two surfaces, two shells

The AG-05 "three workspaces" model (Portfolio / Command Center / Field Portal) is **retained as the conceptual grouping** but consolidated into **two deployable shells**:

- **Web Console** (Lit) — hosts the **Portfolio** and **Command Center** workspaces in one app shell. Desktop-primary, tablet-responsive.
- **Field App** (Flutter) — hosts the **Field Portal**. Mobile-only, offline-first.

This is consistent with `INFORMATION_ARCHITECTURE.md §1`; it just makes the shell boundary explicit (one Lit bundle, one Flutter bundle) and drops the cross-shell ambiguity.

### 1.2 Web Console app shell

```
┌──────────────────────────────────────────────────────────────────────┐
│  TOP BAR  ⬡ BuildOS  ·  [workspace switcher]      ⌘K  ◐ density  🔔  👤│  56px, glass-panel
├────────────┬─────────────────────────────────────────────┬───────────┤
│ NAV RAIL   │  CONTENT (breadcrumb + tabs + view)         │ CONTEXT   │
│ 280px      │  flex                                        │ PANEL     │
│ glass-     │                                              │ 320px     │
│ panel      │  ┌ breadcrumb ──────────────────────────┐   │ on-demand │
│            │  │ Portfolio › Sunrise Villa › Phase 7  │   │ (artifact/│
│ workspace  │  └──────────────────────────────────────┘   │  detail/  │
│ sections   │  ┌ tabs ─────────────────────────────────┐   │  audit)   │
│ (role-     │  │ Summary | Budget | Schedule | Team    │   │           │
│  gated)    │  └──────────────────────────────────────┘   │           │
│            │  view body                                   │           │
├────────────┴─────────────────────────────────────────────┴───────────┤
│ STATUS STRIP  (offline/sync/setup-incomplete banners surface here)     │
└──────────────────────────────────────────────────────────────────────┘
```

Responsive collapse follows `DESIGN_SYSTEM.md §10.3` breakpoints (XL 3-panel, Desktop 2-panel, Tablet hamburger, Mobile bottom-nav overlay). The **Context Panel** replaces the AG-05 "artifact panel" and generalizes it: it hosts artifacts (chat), record detail (drill-down), and the **Audit Trail viewer** (§7.16).

### 1.3 Web Console nav model (role-gated)

Derived directly from `internal/api/router.go` RBAC gates. The nav **must not render a section the role cannot reach** (avoids dead-end 403s). Gate column maps to router middleware (`RequireRole`, `RequireMinRole`, `RequirePlanTier`).

| Section | Route | owner | admin | superintendent | field_worker | Router gate |
|---|---|:--:|:--:|:--:|:--:|---|
| **Portfolio** (group) | `/portfolio` | ● | ● | ◐ | — | |
| — Financials Summary | `/portfolio/financials` | ● | ● | ◐ read | — | `RequireMinRole(super)`; ar-aging/projects `RequireRole(owner,admin)` |
| — Projects | `/portfolio/projects` | ● | ● | ● read | ● read | `projects.List` open; create owner/admin |
| — Pipeline | `/portfolio/pipeline` | ● | ● | ◐ read | — | `RequireMinRole(super)`; mutations owner/admin |
| — Fleet | `/portfolio/fleet` | ● | ● | ◐ allocate+read | — | `RequireMinRole(super)`; create owner/admin |
| — HR & Certs | `/portfolio/hr` | ● | ● | — | — | `RequireRole(owner,admin)` |
| **Command Center** (group) | `/command` | ● | ● | ● | — | |
| — Briefing | `/command/briefing` | ● | ● | ● | — | feed: all auth roles |
| — Schedule (Gantt) | `/command/schedule` | ● | ● | ● | ● read | gantt read all; recalc `RequireMinRole(super)` |
| — Procurement | `/command/procurement` | ● | ● | ◐ read+request-review | — | list read; create owner/admin; request-review `RequireMinRole(super)` |
| — AI Assistant | `/command/assistant` | ● pro | ● pro | ● pro | — | agents `RequirePlanTier(pro)` (now local plan flag, not Brain) |
| **Activity / Audit** | `/activity` | ● | ● | ◐ | — | audit log read (owner/admin full) |
| **Settings** (group) | `/settings` | ● | ◐ | — | — | |
| — Organization | `/settings/org` | ● | ● | — | — | setup/company-profile |
| — Integrations (BYOK) | `/settings/integrations` | ● | — | — | — | **owner-only** (holds secret material) |
| — Users & Roles | `/settings/users` | ● | ◐ | — | — | |
| — Notifications | `/settings/notifications` | ● | ● | ● | ● | per-user |
| **Profile** | `/profile` | ● | ● | ● | ● | self |

● = full nav item shown · ◐ = shown, read-only or partial · — = hidden entirely.

**Removed from AG-05 IA:** `/settings/brain` (P1), `/api/v1/billing` views (P4). `/login` is now a native form, not a redirect (P2).

**New since AG-05 IA:** `/settings/integrations` (P3), `/command/assistant` (renamed from `/command/chat`; native Anthropic), `/activity` promoted from "Agent Activity Log" to a full audit-trail surface (backed by `migration 008_audit_log`), the **Setup Wizard** flow (below).

### 1.4 Setup Wizard (first-run, pre-operational)

The `SetupGate` middleware 403s (`SETUP_INCOMPLETE`) every operational route until `onboarding_complete=true`. The console must therefore ship a **full-screen wizard shell** (no nav rail, no context panel) that is the *only* reachable surface for a fresh fork. Steps mirror `internal/service/setup.go`:

```
[1] Claim owner (bootstrap token)  →  [2] Company info  →  [3] Trades
  →  [4] Cost codes (CSI)  →  [5] Working calendar + holidays
  →  [6] Permit jurisdictions  →  [7] Integrations (BYOK, optional)  →  [Finish]
```

Wizard chrome: a `fb-wizard-stepper` (horizontal on desktop, vertical drawer on tablet), per-step `fb-form`, and a persistent "Save & continue / Back" footer. `Complete` enforces prereqs (legal name, ≥1 trade, ≥1 cost code, default calendar) — the Finish button is disabled with an inline checklist of unmet prereqs until satisfied. Step 7 is skippable (keys can be added later from Settings → Integrations).

### 1.5 Field App (Flutter) nav model

Retains `INFORMATION_ARCHITECTURE.md §2.4` bottom-tab model, role-scoped to `field_worker` (and `superintendent` who uses it lightly per `PERSONAS.md`):

```
┌──────────────────────────────────────────────┐
│  ◐ offline · 3 queued        Sunrise Villa  👤│  app bar w/ sync chip
├──────────────────────────────────────────────┤
│  view body (one-thing-per-screen, low density) │
├──────────────────────────────────────────────┤
│  [ Tasks ]  [ Log ]  [ Photos ]  [ More ]      │  bottom nav, 64px targets
└──────────────────────────────────────────────┘
```

Field routes unchanged from IA §5.2 (`/field/tasks|log|photos|checkin|sync|profile`). Field auth is native email/password (P2) with a **persistent session** (refresh token in secure storage) so a field worker is not re-prompted in a cellular dead zone. Login screen must work offline against a cached session; only token refresh requires connectivity.

---

## 2. Design Tokens (reconciliation + gap-fill)

`DESIGN_SYSTEM.md §2–§6` is the **canonical token source** and is carried forward verbatim (Gable Green `#00FFA3`, Deep Space `#0A0B10`, Slate Steel surfaces, Outfit + JetBrains Mono, 4px spacing, 6–24px radii, 3-level elevation + glow). This section only **fills gaps** the component layer needs.

### 2.1 Status & critical-path semantic tokens (NEW — gap)

AG-05 defines base semantic colors but not the schedule/CPM and BYOK semantics this library needs. Add:

```css
@theme {
  /* Critical Path Method (schedule physics) */
  --color-cpm-critical:        #F43F5E;  /* on critical path (zero float) — Safety Red */
  --color-cpm-critical-bar:    rgba(244, 63, 94, 0.85);
  --color-cpm-near-critical:   #F59E0B;  /* float ≤ 2 working days — Amber */
  --color-cpm-slack:           #38BDF8;  /* has float — Blueprint Blue */
  --color-cpm-complete:        #00FFA3;  /* % complete fill — Gable Green */
  --color-cpm-baseline:        rgba(139, 141, 152, 0.4); /* baseline ghost bar */
  --color-cpm-dependency-line: #5A5B66;  /* FS/SS dependency connectors */

  /* Domain status (reconciles INFORMATION_ARCHITECTURE.md §6.3) */
  --color-status-active:   #00FFA3;
  --color-status-warning:  #F59E0B;   /* approaching deadline, cert expiring */
  --color-status-critical: #F43F5E;   /* overdue, over-budget, equipment down */
  --color-status-complete: #005234;   /* dimmed green */
  --color-status-pending:  #38BDF8;   /* awaiting approval / in queue */
  --color-status-offline:  #5A5B66;   /* disconnected / unavailable */

  /* Money variance */
  --color-variance-positive: #00FFA3; /* under budget / credit */
  --color-variance-negative: #F43F5E; /* over budget / debit */

  /* BYOK integration key states (NEW for P3) */
  --color-key-connected:    #00FFA3;  /* key present + last test OK */
  --color-key-untested:     #38BDF8;  /* key present, never tested */
  --color-key-error:        #F43F5E;  /* last test failed / revoked */
  --color-key-missing:      #5A5B66;  /* no key — feature disabled */
}
```

These reuse the existing palette (no new hues) so contrast guarantees from `DESIGN_SYSTEM.md §11` hold.

### 2.2 Density tokens (NEW — gap)

AG-05 mandates "data density" but ships no density switch. The web console needs **comfortable** (default) and **compact** (power-user tables) modes. Field app is fixed at **field** density (largest).

```css
@theme {
  --density-row-comfortable: 44px;  /* table/list row height */
  --density-row-compact:     32px;
  --density-row-field:       56px;
  --density-control-comfortable: 40px;  /* input/button height */
  --density-control-compact:     32px;
  --density-control-field:       56px;  /* glove-friendly */
}
```

Density is a `data-density="comfortable|compact"` attribute on the shell root; field density is implicit in the Flutter theme.

### 2.3 Z-index scale (NEW — gap)

AG-05 has elevation shadows but no z-order contract. Define so overlays compose predictably:

```css
@theme {
  --z-base: 0; --z-sticky: 100; --z-nav: 200; --z-context-panel: 250;
  --z-dropdown: 300; --z-modal-backdrop: 400; --z-modal: 410;
  --z-toast: 500; --z-tooltip: 600; --z-command-palette: 700;
}
```

### 2.4 Token naming reconciliation note

AG-05 uses **both** `--fb-*` and `--md-sys-color-*` aliases for the same values (Material-3 interop). This duplication is a known wart. **Recommendation (open question Q1):** treat `--fb-*` as canonical for new components; keep `--md-sys-*` as compatibility aliases only where Material Web components are embedded. Do not introduce a third naming scheme.

---

## 3. Component Inventory — overview

Two parallel implementations per the stack (`TECH_STACK.md`): **Lit web components** (`fb-` prefix, per `INFORMATION_ARCHITECTURE.md §6.2`) and **Flutter widgets** (`Fb` prefix). Field app only implements the subset it needs (marked **F**). The Lit atoms/molecules/organisms from `DESIGN_SYSTEM.md §9` are the baseline; this section **extends** it with the components the pivot and the full router surface require.

Legend: **W** = web (Lit) · **F** = field (Flutter) · **NEW** = not in AG-05 catalog.

| Component | Lit tag | Flutter widget | Surfaces | Status |
|---|---|---|---|---|
| Button | `fb-button` | `FbButton` | W, F | AG-05 |
| Icon button | `fb-icon-button` | `FbIconButton` | W, F | NEW |
| Text / data text | `fb-text` | `FbText` | W, F | AG-05 |
| Badge / status badge | `fb-badge` | `FbBadge` | W, F | AG-05 (extended §7.15) |
| Role badge | `fb-role-badge` | `FbRoleBadge` | W, F | NEW (§7.17) |
| Input (text/number/date) | `fb-input` | `FbInput` | W, F | AG-05 |
| Masked secret input | `fb-secret-input` | `FbSecretInput` | W | NEW (§7.5) |
| Password input | `fb-password-input` | `FbPasswordInput` | W, F | NEW (§7.4) |
| Select / combobox | `fb-select` | `FbSelect` | W, F | AG-05 |
| Money display | `fb-money` | `FbMoney` | W, F | NEW (§7.6) |
| Money input | `fb-money-input` | `FbMoneyInput` | W | NEW (§7.6) |
| Checkbox / switch / radio | `fb-checkbox`/`fb-switch` | `FbCheckbox`/`FbSwitch` | W, F | NEW |
| Form / form field / validation | `fb-form`/`fb-field` | `FbForm`/`FbField` | W, F | NEW (§7.3) |
| Chip / filter chip | `fb-chip` | `FbChip` | W, F | AG-05 |
| Data table / grid | `fb-data-table` | (list-based) | W | AG-05 (extended §7.7) |
| Gantt / timeline | `fb-gantt-chart` | `FbGanttView` (read) | W, F-read | AG-05 (extended §7.8) |
| File / image upload | `fb-file-upload` | `FbCameraCapture` | W, F | NEW (§7.9) |
| Invoice-extraction review | `fb-extraction-review` | — | W | NEW (§7.9) |
| Empty / error / loading / skeleton | `fb-state` | `FbState` | W, F | AG-05 (extended §7.10–§7.13) |
| Toast | `fb-toast`/`fb-toaster` | `FbToast` | W, F | AG-05 |
| Modal / confirm dialog | `fb-modal`/`fb-confirm` | `FbModal`/`FbConfirm` | W, F | AG-05 (extended §7.14) |
| Audit-trail viewer | `fb-audit-trail` | — | W | NEW (§7.16) |
| Nav rail / nav item | `fb-nav-rail`/`fb-nav-item` | (bottom nav) | W | AG-05 |
| Top bar / context panel | `fb-top-bar`/`fb-context-panel` | `FbAppBar` | W | NEW (shell) |
| Wizard stepper | `fb-wizard-stepper` | — | W | NEW (§1.4) |
| Offline / sync chip | `fb-sync-status` | `FbSyncChip` | W-passive, F | NEW (§8) |
| Command palette | `fb-command-palette` | — | W | NEW |
| Tabs / breadcrumb | `fb-tab-bar`/`fb-breadcrumb` | — | W | AG-05 |
| Feed card / stat card / project card | `fb-feed-card`/`fb-stat-card`/`fb-project-card` | `FbFeedCard` | W, F | AG-05 |

The following sections specify only the **NEW** and **materially-extended** components. AG-05 components used as-is are not re-specified.

---

## 4. App-shell components

### 4.1 `fb-top-bar`
Logo + workspace switcher (Portfolio ⇄ Command Center) + command-palette trigger (⌘K) + density toggle + notifications + profile menu. Glass-panel (`DESIGN_SYSTEM.md §7.2`). Workspace switcher hides workspaces the role can't enter (§1.3).

### 4.2 `fb-nav-rail` / `fb-nav-item`
Renders §1.3 nav model. **Role gating is declarative:** each item carries `min-role` / `roles` / `requires-plan` attributes; the rail reads the current claims (from the auth store) and omits or dims items. Active item uses the `.active-indicator` left-bar from `DESIGN_SYSTEM.md §8.1`. Collapses to icon-only at Desktop breakpoint, to hamburger overlay at Tablet.

### 4.3 `fb-context-panel`
Right-hand on-demand panel (320px). Hosts one of: chat artifacts, record detail, or `fb-audit-trail`. Slide-in uses "Emphasized" motion (`DESIGN_SYSTEM.md §10.2`). Dismissible; remembers last width.

### 4.4 `fb-wizard-stepper`
Setup-wizard progress. Steps reflect `setup` service step order (§1.4). States per step: `done | current | upcoming | blocked` (blocked = unmet prereq). Keyboard: arrow keys move focus, Enter activates a completed step for revisit.

---

## 5. Auth & BYOK components (pivot-critical)

### 5.1 `fb-password-input` (P2)
Single-line password with a show/hide toggle (eye icon button). Toggle is keyboard-reachable and announces state (`aria-pressed`, "Show password / Hide password"). Caps-lock warning inline. Min-length / strength hint as `aria-describedby` text, never color-only. Used in: sign-in, set-password (owner claim), reset-password, change-password.

### 5.2 `fb-secret-input` (P3 — BYOK)
Masked input for API keys/tokens written into the integrations vault.

- **Default state when a key already exists:** never echoes the stored secret. Renders a fixed masked placeholder `••••••••••••  ····last4` (only the last 4 chars, if the backend chooses to expose them; otherwise fully masked). The real value lives only in `cryptobox`-sealed storage server-side.
- **Edit affordance:** a "Replace" button reveals an empty input; submitting overwrites. There is **no "reveal stored secret"** — the fork cannot decrypt to screen by design (write-only from the UI's perspective).
- **Paste-friendly:** accepts paste, trims whitespace, disables autocomplete (`autocomplete="off"`, `data-1p-ignore`), `spellcheck=false`, `inputmode` text.
- **Never logged / never in DOM-as-plaintext after submit.** On submit, the field clears its own value.
- **Test-connection** button next to the field calls the provider's health/echo endpoint and flips the adjacent `fb-badge` to the key-state token (§2.1): connected / untested / error.
- Accessibility: the masked dots have `aria-label="API key set"`; screen readers must not attempt to read the mask glyphs.

### 5.3 Bootstrap-token field (P2)
A specialized `fb-secret-input` variant used once on the owner-claim step. 43-char base64url token. Validates length/charset client-side; on submit, redemption returns the **uniform** `ErrInvalidBootstrapToken` on any failure (per `internal/service/setup.go`) — the UI MUST therefore show a single generic error ("That setup token is invalid or expired") and **must not** distinguish "wrong token" from "expired" from "wrong org" (no probing oracle). See §11 content rules.

### 5.4 Integration card (`fb-integration-card`) (P3)
One card per provider (Anthropic, Resend, Gable, LocalBlue). Shows: provider name + logo, key-state badge, masked secret input, test-connection, last-tested timestamp, and an enable/disable switch. Owner-only surface. When `--color-key-missing`, the dependent feature areas elsewhere in the app render the "feature requires a key" empty state (§7.11, §11).

---

## 6. Money components

### 6.1 `fb-money` (display)
Renders the Composite Currency Pattern (`API_CONTRACT.md §2.4`, `TECH_STACK.md`). Props: `cents` (string/bigint — never JS `number` for values that can exceed 2^53), `currency-code` (`USD|CAD`). Always JetBrains Mono, `font-variant-numeric: tabular-nums`. Uses `Intl.NumberFormat` with the correct currency. Variance variants color positive/negative via §2.1 variance tokens. **Never sums across currencies** — a list of mixed-currency values renders grouped subtotals per `currency_code`, never a single total. The component refuses (throws in dev, renders `—` in prod) if asked to add two different codes — mirrors backend `ErrCrossCurrency`.

### 6.2 `fb-money-input` (entry)
Captures dollars in the UI, emits integer cents + currency code. Masks to 2 decimal places; rejects float drift by parsing to integer cents (string math, never `parseFloat` round-trips). Currency selector limited to USD/CAD. Field name must end in `Cents` to satisfy the ESLint composite-currency rule (`TECH_STACK.md`).

---

## 7. Data, schedule, file, and state components

### 7.7 `fb-data-table` (extended)
AG-05 already specifies virtual scroll + mono numerics + variance coloring. Extensions:

- **Density-aware** row height (§2.2).
- **Column types:** text, mono-number, money (`fb-money`), date, status-badge, role-badge, actions. Numeric/money/date columns right-align; tabular-nums.
- **Sort/filter/paginate** map to the backend pagination contract (`API_CONTRACT.md §2.3`, `?page&per_page`). Server-side sort for >200 rows.
- **Selection + bulk actions** gated by role (e.g. only owner/admin see "Approve" bulk action).
- **Accessibility (critical):** real `<table>` semantics with `role="grid"` only when adding 2-D keyboard nav. `<th scope>` on every header; `aria-sort` reflects current sort. Row selection via roving tabindex; arrow-key cell navigation in grid mode; `aria-rowcount`/`aria-rowindex` for virtualized rows so SR users get true totals despite virtual scroll. Sortable headers are real `<button>`s, not click-only `<th>`.

### 7.8 `fb-gantt-chart` (extended — the hard one)
Canvas/SVG timeline over CPM output (`internal/physics`). Extensions and a11y:

- **Critical-path semantics** via §2.1 tokens: critical bars `--color-cpm-critical-bar`, near-critical amber, slack blue, completion fill green, baseline ghost, dependency connectors. Critical status is **never color-only** — critical bars also get a 2px solid left cap + a "critical" text label in the row header and an icon, satisfying WCAG 1.4.1.
- **Data in JetBrains Mono:** dates, durations, float values (`DESIGN_SYSTEM.md §3.2`).
- **Read vs. edit:** field app renders read-only; web console allows duration edit for `superintendent`+ (router `RequireMinRole(super)` on `/schedule/recalculate` and `/tasks/{taskID}`). Edits are optimistic with a recalc spinner; the server re-runs CPM and the bars re-flow.
- **Accessibility (Gantt is not inherently accessible):** ship a **dual representation**. (1) The visual canvas is `aria-hidden`. (2) A visually-equivalent, screen-reader-first **task table** (`fb-data-table`) is always present (toggle or off-screen) exposing: task ID, name, start, finish, duration, total float, is-critical, predecessors. Keyboard users tab through task rows; pressing Enter on a row focuses its bar and announces "WBS 9.2 Second Floor Framing, critical, starts Apr 5, 6 days, 0 float." Pan/zoom controls are real buttons with labels. Honor `prefers-reduced-motion` for the recalc re-flow animation.

### 7.9 `fb-file-upload` + `fb-extraction-review` (P5)
- **`fb-file-upload`:** drag-drop + click, image/PDF, client-side type/size guard, thumbnail preview, progress bar, retry/cancel. Field app uses `FbCameraCapture` (camera + GPS + timestamp metadata, per `USER_JOURNEYS.md` J4). Multi-file for invoices.
- **`fb-extraction-review`:** after upload, the native Anthropic vision extractor (`internal/ai/image.go`) returns structured invoice fields. This component shows the **document on the left, extracted fields on the right** (vendor, dates, line items, totals as `fb-money`, predicted WBS/cost code) each editable, each with a confidence indicator, and a "looks wrong" affordance per field. Submit confirms the extraction. Empty/disabled when the Anthropic key is missing (§11).

### 7.10–7.13 `fb-state` (loading / skeleton / empty / error — extended)
One component, four modes, reusing AG-05 `.skeleton` shimmer and §10.1 state treatments.
- **Loading/skeleton:** shape-matched skeletons (table → skeleton rows; card → skeleton card). Respect `prefers-reduced-motion` (static placeholder, no shimmer).
- **Empty:** centered icon + headline + body + optional primary action. Voice per §11.
- **Error:** icon + message + retry. Maps backend `error.code` (`API_CONTRACT.md §2.2`) to friendly copy (§11). Never shows raw `message` from server for 5xx; shows a generic line + request_id for support.
- **Feature-gated empty (NEW, P3):** the "this feature requires a key" variant — see §11.

### 7.14 `fb-modal` / `fb-confirm` (extended)
AG-05 modal + a `fb-confirm` convenience for destructive actions. Focus trap, `Esc` to close, restore focus to invoker, `role="dialog"` + `aria-modal`, labelled by title. Destructive confirm uses the `btn-destructive` styling and requires an explicit affirmative (no default-focus on the destructive button).

### 7.15 `fb-badge` / status badge (extended)
Maps to §2.1 status + key-state + cpm tokens. **Always pairs color with text + icon** (WCAG 1.4.1) — e.g. critical = red dot + "Critical" + alert icon. Sizes sm/md.

### 7.16 `fb-audit-trail` (NEW)
Reads the audit log (`migration 008_audit_log`, `store/audit.go`). Reverse-chronological list grouped by day; each entry: timestamp (mono), actor (user_sub → resolved display name where available, else "system"), `action` (humanized — e.g. `setup.trade.created` → "Added trade"), resource type + id, and an expandable before/after diff. **PII note:** the backend already scrubs Restricted-class fields from `before/after/metadata` JSONB (`scrubAuditPayloads`), so the viewer renders what it's given and never attempts to "unmask." Filter by `action_prefix` (e.g. `setup.`) and date range. Surfaces in the Context Panel and in `/activity`. Owner/admin only.

### 7.17 `fb-role-badge` (NEW)
Compact pill for the four RBAC roles with a fixed, accessible color+label+icon mapping. Used in Users & Roles, audit actor column, and profile. Roles are ordered `owner > admin > superintendent > field_worker` (matches router). Color is decorative only — the role name text is always present.

| Role | Label | Accent | Icon |
|---|---|---|---|
| owner | Owner | Gable Green | shield-crown |
| admin | Admin | Blueprint Blue | shield |
| superintendent | Superintendent | Amber | hard-hat |
| field_worker | Field Worker | Slate (neutral) | person |

---

## 8. Theming, dark mode, density, field ergonomics

- **Dark-only is retained** (`DESIGN_SYSTEM.md §14`). No light mode, no theme toggle. Print stylesheet may invert. **No change.**
- **Density** (§2.2): web console exposes comfortable/compact via top-bar toggle, persisted per user. Field app is fixed at field density.
- **Field ergonomics (Flutter)** — codifies `PERSONAS.md` (Carlos) + `DESIGN_SYSTEM.md §11`:
  - **Touch targets ≥ 56px** (AG-05 says 48px min, 64px for field; this spec sets field controls at 56px floor, 64px for primary task actions). Glove-friendly: generous spacing, no tap targets within 8px of each other.
  - **One primary action per screen.** Slider-to-complete + large green checkmark for task completion (`USER_JOURNEYS.md` J4).
  - **Offline indicator is mandatory and always visible** (sync chip §8.1). Queued actions render with a dashed border + pending count (`DESIGN_SYSTEM.md §10.1` Offline state).
  - **Bilingual (ES/EN)** per Carlos persona: all field strings localized; prefer icons + photos over text. Weather/safety alerts available in Spanish.
  - **High outdoor-contrast:** field surfaces avoid low-contrast `--color-text-muted` for primary content; minimum body contrast ≥ 7:1 (AAA) in field app given sunlight glare.

### 8.1 `fb-sync-status` / `FbSyncChip`
Field app: persistent chip in the app bar — green dot "Online" / amber "Offline · N queued". Tapping opens the Sync Status screen (outbox count, last sync, manual retry). Backed by the Drift outbox pattern (`INFORMATION_ARCHITECTURE.md §9`). Web console shows a passive variant only when a write fails to reach the server.

---

## 9. Accessibility standards (WCAG 2.1 AA)

Baseline from `DESIGN_SYSTEM.md §11` is retained and extended:

- **Contrast:** body text AA (≥4.5:1) on web; field app primary content targets AAA (≥7:1) for sunlight. Status is never conveyed by color alone (1.4.1) — color + icon + text everywhere (badges, Gantt, money variance, key states).
- **Focus:** visible `:focus-visible` 2px `#00FFA3` outline, 2px offset, on **every** interactive element; never `outline:none` without replacement. Focus trapped in modals; restored to invoker on close. Logical tab order matches visual order.
- **Keyboard:** full operation without a mouse. Command palette (⌘K) for power navigation. Data grid: roving tabindex + arrow keys (§7.7). Gantt: task-row keyboard nav + dual table representation (§7.8). All custom controls (`fb-switch`, `fb-secret-input` toggle, sort headers) are real buttons/inputs or carry correct ARIA roles + key handlers.
- **Screen reader semantics:**
  - Landmarks: `banner` (top bar), `navigation` (nav rail), `main` (content), `complementary` (context panel), `contentinfo` (status strip).
  - **Data grids:** `<table>` semantics, `<th scope>`, `aria-sort`, `aria-rowcount`/`aria-rowindex` for virtualization, sortable headers as buttons.
  - **Gantt:** canvas `aria-hidden`; equivalent accessible task table is the SR source of truth; bar focus announces a full sentence (§7.8).
  - **Live regions:** toasts `role="status"`/`aria-live="polite"`; errors `aria-live="assertive"`; sync state changes announced politely; recalc completion announced.
  - **Masked secrets:** `aria-label="API key set"`; SRs never read mask glyphs.
- **Reduced motion:** `prefers-reduced-motion` disables transforms/shimmer/Gantt re-flow; opacity-only transitions remain.
- **Touch targets:** web ≥44px, field ≥56px (primary 64px).
- **Forms:** every field has a programmatic `<label>`; errors via `aria-describedby` + `aria-invalid`; error summary at form top links to first invalid field.

---

## 10. Content / voice (errors & empty states)

Voice: **direct, operational, no fluff** — matches the Bloomberg-terminal ethos and the personas ("Show me the numbers", "30 seconds or I'll text instead"). Plain language, action-oriented, never blame the user. Numbers and IDs in mono.

### 11.1 Error copy mapping (from `API_CONTRACT.md §2.2` `error.code`)

| `error.code` | User-facing copy | Action shown |
|---|---|---|
| `VALIDATION_ERROR` | Field-level inline messages from `details[]` ("Name is required"). | Fix inline |
| `UNAUTHORIZED` | "Your session expired. Sign in again." | Sign in |
| `FORBIDDEN` | "You don't have access to this. Ask an owner or admin." | — (and the nav should have hidden it; §1.3) |
| `NOT_FOUND` | "We couldn't find that {resource}." | Back to list |
| `SETUP_INCOMPLETE` | Route to the Setup Wizard. "Finish setup before using BuildOS." | Resume setup |
| `RATE_LIMITED` | "Too many requests. Try again in a moment." | Retry (disabled briefly) |
| `UPSTREAM_ERROR` / `SERVICE_UNAVAILABLE` (5xx) | "Something went wrong on our end. Reference: `{request_id}`." | Retry + copy request_id |
| `PAYLOAD_TOO_LARGE` | "That file is too large. Max {N} MB." | Re-pick file |

Never surface raw 5xx `message` text. Always include `request_id` for support on server errors.

### 11.2 "Feature requires a key" states (P3 — BYOK)

When a feature's provider key is missing (`--color-key-missing`), render a **feature-gated empty state**, not an error. It must:
- Name the capability in user terms, not the vendor SKU: "AI briefings are turned off" (not "Anthropic key missing").
- Explain the one-line why: "Add your Anthropic API key to enable AI features."
- Offer the action **only to owners** ("Go to Integrations →"); non-owners see "Ask your account owner to add an API key" with **no** link (they can't reach `/settings/integrations`).
- Never expose key material, never hint whether a key was *recently revoked* vs *never set* (uniform copy).

Examples:
- AI Assistant / Briefing (no Anthropic key): "AI features are turned off until an Anthropic API key is added."
- Email notifications (no Resend key): "Email notifications are paused. Add a Resend API key to send email."
- Gable / LocalBlue integrations: "{Integration} isn't connected yet."

### 11.3 Empty-state copy (no data yet)

Encouraging + next-action, not dead ends:
- Projects: "No projects yet. Create your first project to start scheduling." + Create button (owner/admin only).
- Briefing: "You're all caught up. New alerts appear here each morning."
- Audit trail: "No activity in this range."
- Field tasks: "No tasks assigned for today." (large, low-density, ES/EN.)

### 11.4 Bootstrap-token + auth errors (security-uniform)
Per §5.3 and `internal/service/setup.go`: a single generic message for any bootstrap-token failure ("That setup token is invalid or expired"). Sign-in failures are likewise uniform ("Email or password is incorrect") — never reveal which was wrong. No probing oracles in copy.

---

## 11. Component → route → RBAC traceability (acceptance aid)

| View | Route | Key components | RBAC source |
|---|---|---|---|
| Sign in / Reset | `/login`, `/reset` | `fb-form`, `fb-password-input` | native (P2) |
| Setup wizard | `/setup/*` | `fb-wizard-stepper`, `fb-secret-input`, `fb-form` | SetupGate-exempt |
| Financials | `/portfolio/financials` | `fb-stat-card`, `fb-data-table`, `fb-money`, AR-aging chart | `RequireMinRole(super)` + owner/admin |
| Projects / detail | `/portfolio/projects[/:id]` | `fb-project-card`, tabs, `fb-gantt-chart` | list all; mutate owner/admin |
| Pipeline | `/portfolio/pipeline` | `fb-data-table`, `fb-money` | `RequireMinRole(super)` |
| Fleet | `/portfolio/fleet` | `fb-data-table`, `fb-badge` | super allocate; owner/admin create |
| HR & Certs | `/portfolio/hr` | `fb-data-table`, expiry `fb-badge` | owner/admin only |
| Schedule | `/command/schedule` | `fb-gantt-chart` (+a11y table) | read all; edit super+ |
| Procurement | `/command/procurement` | `fb-feed-card`, `fb-data-table` | read; create owner/admin; request-review super+ |
| AI Assistant | `/command/assistant` | chat, `fb-extraction-review` | plan-tier pro |
| Activity / Audit | `/activity` | `fb-audit-trail`, `fb-role-badge` | owner/admin |
| Integrations (BYOK) | `/settings/integrations` | `fb-integration-card`, `fb-secret-input` | **owner only** |
| Field tasks/log/photos | `/field/*` | `FbFeedCard`, `FbCameraCapture`, `FbSyncChip` | field roles |

---

## 12. Overlaps, conflicts, and open questions

### 12.1 Direct conflicts with existing specs (must be resolved before build)

| # | Conflict | Existing spec says | This pivot requires | Recommended resolution |
|---|---|---|---|---|
| C1 | **Login** | IA §3.4/§5.1: `/login` = "The Brain OIDC redirect (no local login form)". | Native email/password form. | Mark IA §3.4 login row deprecated; adopt §1.3 + §5.1 here. |
| C2 | **Brain settings surface** | IA §2.1 nav + §5.1 `/settings/brain`; DS §"Settings, Brain integration". | No Brain. | Replace `/settings/brain` with `/settings/integrations` (BYOK). |
| C3 | **Billing views** | router `/api/v1/billing/*`; AG-05 implies Brain billing proxy. | Billing removed; AI cost is a local meter. | Drop billing nav; keep a lightweight local AI-usage stat from `internal/ai` if desired (Q3). |
| C4 | **A2A surfaces** | IA §5.3 `/api/v1/a2a/*`, cross-workspace A2A interactions §8. | A2A removed. | Remove A2A from nav, settings, and cross-workspace interaction table. |
| C5 | **`plan_tier` gate** | router `RequirePlanTier(pro)` assumes Brain-issued claim. | No Brain JWTs. | Plan tier becomes a local org attribute (native auth claim). UI still hides pro features for non-pro; backend gate semantics unchanged. (Q2) |
| C6 | **Auth claims source** | API_CONTRACT §1.1: claims "from The Brain". | Claims minted by the fork's native auth. | Same claim *shape* (`sub`, `org_id`, `role`, `plan_tier`); different issuer. UI auth store reads the same fields. |

### 12.2 Overlaps (intentional reuse, no conflict)
- Tokens/glass/motion/dark-only: reused verbatim from `DESIGN_SYSTEM.md`; this doc only adds status/CPM/key/density/z tokens.
- Status vocabulary: §2.1 reconciles `INFORMATION_ARCHITECTURE.md §6.3` into tokens (same colors).
- Component prefix/event/naming conventions: reused from `INFORMATION_ARCHITECTURE.md §6.2`.

### 12.3 Open questions for the user
- **Q1 (tokens):** Collapse the dual `--fb-*` / `--md-sys-*` naming to `--fb-*` canonical, keeping `--md-sys-*` as aliases only? Or keep full Material-3 interop?
- **Q2 (plan tier):** With Brain gone, is `plan_tier` still a concept (self-hosted forks paying for "pro" AI features), or do all forks get all features and the `RequirePlanTier` gates get neutralized? This decides whether the AI Assistant nav item is ever hidden.
- **Q3 (AI usage meter):** Do you still want an in-app "AI usage / cost" view (now sourced locally from `internal/ai` token counts), or drop the financial framing entirely since there's no markup/billing?
- **Q4 (secret last-4):** Should `fb-secret-input` display the last 4 characters of a stored key for operator recognition, or fully mask? Showing last-4 aids ops but is a (small) confidentiality trade-off. Backend must opt to expose it.
- **Q5 (BYOK RBAC):** Integrations is owner-only here. Confirm admins are excluded from viewing/rotating keys (they can configure most other org settings).
- **Q6 (Gable/LocalBlue UX):** Beyond a key + test-connection, do these integrations need richer config surfaces (mappings, sync schedules)? If so they need their own sub-pages, not just an integration card.
- **Q7 (Field auth offline):** Confirm the field app should allow a fully offline launch against a cached session (refresh only when online). This affects token lifetime + secure-storage design.
- **Q8 (density default):** Comfortable as the web default with opt-in compact — confirm, given AG-05's "data density over decoration" principle might argue for compact-by-default for admin tables.

---

## 13. Summary

This spec **extends** the AG-05 design system and IA without contradicting their still-valid parts (Industrial Dark, dark-only, Gable Green, Outfit + JetBrains Mono, glass, 4px spacing, elevation/glow). It (1) consolidates the workspace model into two shells (Lit web console + Flutter field app) with a fully role-gated nav derived from `internal/api/router.go`; (2) fills token gaps (CPM/status/key/variance semantics, density, z-index); (3) specifies the new and extended components the standalone/native pivot demands — native auth inputs, the BYOK masked-secret input + integration cards, money display/input, the accessible data grid and Gantt (with a screen-reader task-table twin), file upload + invoice-extraction review, the audit-trail viewer, role badges, and the full state/empty/error family; (4) sets WCAG 2.1 AA standards with explicit grid/Gantt SR strategies; and (5) defines content/voice for errors and the pivot-critical "feature requires a key" empty states with security-uniform copy. Six concrete conflicts with the existing specs (login, Brain settings, billing, A2A, plan-tier, claim source) are flagged in §12.1 with recommended resolutions, and eight open questions are raised for the user.
