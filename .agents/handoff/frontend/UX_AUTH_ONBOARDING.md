# UX Spec — Auth, Onboarding & Integrations (BYOK)

**Document ID:** FE-UX-AUTH-ONBOARDING
**System:** BuildOS (System of Execution) — web shell (Lit) + Flutter field surfaces
**Created:** 2026-06-01
**Pipeline Stage:** Frontend UX — flows reshaped by the standalone/native pivot
**Status:** DRAFT — RESEARCH + SPEC ONLY (no application code)
**Design System:** GableLBM Industrial Dark (`DESIGN_SYSTEM.md`)
**Depends on backend:** `internal/service/setup.go`, `internal/api/setup.go`, `internal/api/middleware/setup_gate.go`, `internal/api/router.go`, `internal/ai/*`, `internal/mailer/*`, `internal/cryptobox/*`

---

## 0. Context — what the native pivot changes

BuildOS is dropping the external **"The Brain"** dependency and going fully standalone/native. The flows in this spec are the ones most reshaped by that pivot. What was previously delegated to The Brain is now owned in-app:

| Surface | Old (Brain) | New (native) |
|---------|-------------|--------------|
| Identity / login | Brain OIDC redirect; app only validated JWTs | App-minted email/password JWTs (`/auth/*`) |
| First-owner provisioning | Brain JIT-provisions the user, then bootstrap token redeemed | One-time bootstrap token redeemed **with** email+password at `/setup/bootstrap`; creates first owner directly |
| AI key | Brain held Anthropic key, metered + marked up | Per-org Anthropic key in the in-app BYOK vault (`internal/ai`, `KeyResolver.AnthropicKey`) |
| Email delivery | Brain / 3rd-party | Per-org Resend key in the BYOK vault (`internal/mailer`, `KeyResolver.ResendKey`) |
| 3rd-party (Gable, LocalBlue) | Brain Hub vault | Per-org keys in the BYOK vault |

**Grounding facts confirmed from code (do not re-derive):**

- The onboarding wizard, `SetupGate`, and bootstrap-token machinery **already exist** (`internal/service/setup.go`, `internal/api/setup.go`, `internal/api/middleware/setup_gate.go`).
- Native AI (`internal/ai/client.go`) and native mailer (`internal/mailer/resend.go`) **already exist** and resolve a per-org secret through a `KeyResolver` interface. A missing key returns a typed soft error: `ai.ErrUnconfigured` / `mailer.ErrMailerUnconfigured`. **This is the backbone of every "AI disabled" / "mailer unconfigured" empty state in this spec.**
- Secret encryption uses `internal/cryptobox` (AES-256-GCM). Secrets are **never echoed back** over the wire.
- The wizard's `SETUP_INCOMPLETE` gate exempts `/api/v1/setup`, `/health`, `/ready`, `/metrics`, `/api/v1/a2a/webhook` (`DefaultSetupGateExemptPrefixes`).

**Backend endpoints referenced by this spec.** Native auth + BYOK endpoints are *being built*; the wizard endpoints are *shipped*. Where shipped, the exact request/response/error mapping is cited from code. Where being built, the contract is proposed here and flagged in **Open Questions** for backend ratification.

| Area | Method + Path | State |
|------|---------------|-------|
| Auth | `POST /api/v1/auth/login` | being built |
| Auth | `POST /api/v1/auth/refresh` | being built |
| Auth | `POST /api/v1/auth/logout` | being built |
| Auth | `POST /api/v1/auth/password-reset/request` | being built |
| Auth | `POST /api/v1/auth/password-reset/confirm` | being built |
| Bootstrap | `POST /api/v1/setup/bootstrap` | shipped (token-only today; email+password addition flagged — see §1 + Open Questions Q1) |
| Wizard | `GET /api/v1/setup/state` | shipped |
| Wizard | `POST /api/v1/setup/company-info` | shipped |
| Wizard | `POST /api/v1/setup/trades` | shipped |
| Wizard | `POST /api/v1/setup/cost-codes` | shipped |
| Wizard | `POST /api/v1/setup/calendars` | shipped |
| Wizard | `POST /api/v1/setup/calendars/{calendarID}/holidays` | shipped |
| Wizard | `POST /api/v1/setup/jurisdictions` | shipped |
| Wizard | `POST /api/v1/setup/complete` | shipped |
| BYOK | `POST /api/v1/integrations/{provider}/credentials` | being built |
| BYOK | `GET /api/v1/integrations/credentials` | being built |
| BYOK | `DELETE /api/v1/integrations/{provider}/credentials/{scope}` | being built |

**Global conventions applied to every screen below**

- Dark-only (`DESIGN_SYSTEM §14`). No theme toggle. Deep Space `#0A0B10` canvas, Slate Steel `#161821` surfaces, glass cards/panels.
- Fonts: Outfit for labels/headings/body; **JetBrains Mono for all secret/key material, token strings, masked values, timestamps, IDs, durations** (`DESIGN_SYSTEM §3.2`).
- Errors are surfaced by **color + text + icon** (never color alone — `DESIGN_SYSTEM §11`, WCAG 3.3.1).
- Standard interaction states from `DESIGN_SYSTEM §10.1`: Default / Hover / Active / Focus (2px `#00FFA3` `:focus-visible`) / Disabled (40% opacity) / Loading (`.skeleton`) / Empty / Error (3px `--fb-error` left border) / Success (300ms glow flash).
- Toasts per `INFORMATION_ARCHITECTURE §4.3`: success green 5s, error red persists, warning amber 10s, info blue 8s.
- Touch targets ≥48px (web), ≥64px on Field Portal.
- Error envelope is the standard `{ error: { code, message, details? }, meta }` (`API_CONTRACT §2.2`).

**Auth-screen workspace note:** Login / reset / bootstrap render **outside** the `<fb-org-shell>` (no sidebar) on a centered glass-panel over the Deep Space canvas — these screens exist before a session and before workspace context. The wizard renders inside a **dedicated minimal shell** (progress rail, no Portfolio/Command nav, since `SETUP_INCOMPLETE` blocks those routes). Integrations render **inside** the full org shell under Settings, post-onboarding.

---

## 1. First-Owner Bootstrap Claim

**Who:** Tom (Owner) — the very first human on a fresh fork. No account exists yet.
**Trigger:** Tom receives, out-of-band from the fork operator, a one-time **bootstrap token** (43-char base64url, shown once) + the deployment URL.
**Goal:** Exchange the token + chosen email/password for the first owner account and a live session.

### 1.1 Backend reality vs. the pivot target

Today `POST /api/v1/setup/bootstrap` (shipped) is **JWT-authenticated but not admin-gated** — it expects the caller to already have a JWT (`claims.Sub`, `claims.OrgID`) and calls `RedeemBootstrapTokenForSubject(token, subject, callerOrgID)`. That model assumed Brain had already provisioned the user (hence the `BOOTSTRAP_USER_NOT_PROVISIONED` / 412 path).

In the **native** model there is no pre-existing JWT and no pre-provisioned user at first-owner time. The pivot target endpoint accepts `{ token, email, password }` **unauthenticated**, creates the owner user, redeems the token, and returns an access+refresh token pair. The UI below is specced against the **target** shape and notes the divergence. See **Open Questions Q1**.

### 1.2 Screen B1 — "Claim your BuildOS"

Centered glass-panel (`§7.2 heavy glass`), max-width 480px. Logo lockup top. Headline "Claim your BuildOS" (Outfit Headline Small). Subtext: "Paste the setup token your operator gave you to create the owner account."

Fields:
1. **Bootstrap token** — `<fb-input type="text">`, JetBrains Mono, monospace, `autocomplete="off"`, `spellcheck=false`. Placeholder shows the 43-char shape masked. Show a char-count helper (`43 / 43`) since the token is fixed-length (`SeedBootstrapTokenIfNeeded` enforces exactly 43 base64url chars). A paste button is offered (operators usually copy/paste).
2. **Email** — `<fb-input type="email">`, `autocomplete="username"`.
3. **Password** — `<fb-input type="password">`, `autocomplete="new-password"`, with a show/hide toggle and a live strength meter. Client policy mirror (server is source of truth): ≥12 chars; surface server policy errors verbatim. See Open Questions Q2.
4. **Confirm password** — must match (client-side, instant inline error).

Primary CTA: **"Create owner account"** (`<fb-button variant=primary>`, full-width, glow on hover). Disabled until: token length == 43, email is RFC-valid shape, password meets client policy, confirm matches.

### 1.3 States

- **Default:** as above.
- **Loading:** CTA enters `loading` (spinner), inputs disabled, panel unchanged. No skeleton (form already rendered).
- **Client-validation error:** inline under the offending field, red 3px left border + icon + message. CTA stays disabled.
- **Success:** brief green glow on the panel (300ms), then route to the wizard entry (`/setup` → §6). Tokens are persisted (see §3.1). Toast: "Owner account created. Let's set up your company."
- **Token field empty on submit:** the shipped handler returns `400 VALIDATION_ERROR "token is required"`; we pre-empt this client-side but still map it.

### 1.4 Error mapping (from `writeSetupError` + target additions)

| Backend | HTTP / code | UI treatment |
|---------|-------------|--------------|
| `ErrInvalidBootstrapToken` (missing / expired / already redeemed / hash mismatch — **intentionally uniform**) | `401 INVALID_BOOTSTRAP_TOKEN` | Form-level error banner: "This setup token is invalid or has expired. Ask your operator to issue a new one." **Never** distinguish "expired" vs "wrong" vs "used" — the backend deliberately collapses these (security: avoids probe leakage). Keep token field populated so the user can re-copy. |
| `ErrSetupAlreadyComplete` | `409 SETUP_ALREADY_COMPLETE` | Full-panel terminal state: "This BuildOS is already set up. Sign in instead." with a **"Go to sign in"** button → `/login`. Hide the form. |
| `BOOTSTRAP_USER_NOT_PROVISIONED` (legacy path only) | `412 PRECONDITION_FAILED` | Should not occur in native mode (we create the user inline). If seen during the transition, show: "Account provisioning is not ready yet — contact your operator." Log to Sentry. Flagged Q1. |
| Email already in use (target) | proposed `409 EMAIL_TAKEN` | Inline on email field: "An account with this email already exists." |
| Weak password (target) | proposed `400 VALIDATION_ERROR` w/ `details[].field=password` | Inline on password field, surface `reason` verbatim. |
| `VALIDATION_ERROR` (bad JSON / generic) | `400` | Field-level if `details[].field` present; else form-level banner. |
| `500 INTERNAL_ERROR` | `500` | Form-level: "Something went wrong creating your account. Try again." CTA re-enabled. Capture to Sentry. |
| Rate limited | `429 RATE_LIMITED` | Form-level: "Too many attempts. Wait a moment and try again." Honor `X-RateLimit-Reset` to show a countdown. |

### 1.5 Edge cases

- **Token already redeemed by a faster claimant** → uniform `401 INVALID_BOOTSTRAP_TOKEN` (the backend's redeem race resolves to the same code). Same banner as invalid.
- **Cross-org token** (legacy path): backend refuses **without consuming** the token and returns uniform 401. UI identical.
- **Network failure mid-submit:** treat as retryable; show form-level "Couldn't reach the server" + retry. Do **not** assume the account was/ wasn't created — on retry, an already-redeemed token yields the uniform 401, at which point we route the user to `/login` with a hint ("If you already created your account, sign in").

---

## 2. Login

**Who:** All roles (Tom/owner, Sarah/admin, Mike/super, Carlos/field). Carlos logs in from Flutter Field Portal; the same contract applies, larger touch targets.
**Trigger:** Visiting any authenticated route without a valid session → redirect to `/login` with `?next=` preserved.

### 2.1 Screen L1 — "Sign in"

Centered glass-panel, 480px. Logo. Headline "Sign in". Fields:
1. **Email** — `type="email"`, `autocomplete="username"`.
2. **Password** — `type="password"`, `autocomplete="current-password"`, show/hide toggle.
3. **"Remember this device"** checkbox (controls refresh-token persistence lifetime — see Open Questions Q5).

Primary CTA **"Sign in"**. Secondary link **"Forgot password?"** → `/auth/password-reset` (§5). No "Sign up" link — accounts are operator/owner-provisioned (single-tenant fork; ADR-002).

### 2.2 States

- **Default / Loading / Success** per global conventions. On success: store tokens (§3.1), route to `?next=` or workspace default by role (owner/admin → `/portfolio/financials`; super → `/command/briefing`; field → Flutter task list). If the org is **not yet onboarded**, the first authenticated call returns `403 SETUP_INCOMPLETE` → route to `/setup` (§6) instead.
- **Empty:** N/A (form always present).

### 2.3 Error mapping

| Condition | HTTP / code | UI treatment |
|-----------|-------------|--------------|
| Bad email or password | `401 UNAUTHORIZED` (uniform — do not reveal which) | Form-level banner: "Email or password is incorrect." Keep email populated, clear password. |
| Account locked / too many failures | proposed `429 RATE_LIMITED` or `403` w/ `ACCOUNT_LOCKED` | Banner: "Too many attempts. Try again in N minutes." countdown from `X-RateLimit-Reset`. See Q4. |
| Org onboarding incomplete (post-auth call) | `403 SETUP_INCOMPLETE` | Not an error toast — silently route to `/setup`. Only owners/admins can complete it; a super/field who logs in pre-onboarding sees a "Setup in progress — check back soon" interstitial (they cannot complete the wizard; routes are admin-gated). See Q7. |
| `VALIDATION_ERROR` (malformed) | `400` | Inline field errors. |
| `500` | `500` | Banner: "Sign-in is temporarily unavailable." Retry. |

### 2.4 Edge cases

- **Already authenticated user hits `/login`:** redirect to workspace default (don't show the form).
- **`next` points at a route the role can't access:** after login, if the target 403s on RBAC, fall back to the role's default workspace + info toast.
- **Field Portal offline:** login requires connectivity (token mint is server-side). Show the offline banner (amber, `§10.1 Offline`) and disable the CTA with helper text "You're offline — sign in needs a connection." Cached session (if any, unexpired) keeps the user in.

---

## 3. Session Refresh & Expiry Handling

Access token ~15 min; refresh token longer-lived. This is plumbing, not a screen — but it has visible states.

### 3.1 Token storage & lifecycle (web)

- Access token held **in memory** (JS variable / signal), never localStorage, to limit XSS blast radius.
- Refresh token: **httpOnly, Secure, SameSite=Strict cookie** set by the server is strongly preferred (UI never reads it). If the backend instead returns the refresh token in the JSON body, store it in memory + a short-lived persisted store only when "Remember this device" is checked. **Flagged Q5** — backend must declare the transport.
- Flutter: access token in memory; refresh token in platform secure storage (Keychain / Keystore).

### 3.2 Proactive refresh

- A scheduler refreshes the access token at ~T-60s before `exp` (decode `exp` client-side; clock-skew tolerant). On success, swap the in-memory access token transparently. No UI.
- All API calls go through one client wrapper that, on `401`, attempts a **single** silent refresh then retries the original request once. Concurrent 401s share one in-flight refresh (single-flight) to avoid a refresh stampede.

### 3.3 States

| Situation | UX |
|-----------|-----|
| Silent refresh success | Invisible. No toast. |
| Refresh fails (refresh token expired/revoked) | Stop retries. Capture the current route into `?next=`. Show a **non-blocking "Session expired" toast** (info, 8s) then route to `/login`. For unsaved-form routes, instead show a **blocking modal**: "Your session expired. Sign in to continue — your work is preserved." with a sign-in CTA that returns to the exact route (see Q6 on form preservation). |
| Mid-request expiry on a mutation | The wrapper's retry covers transient `401`. If refresh also fails, the mutation is **not** silently dropped: surface error toast "Couldn't save — your session expired. Sign in and try again." Do not auto-resubmit after re-login (avoid double-write); re-present the form with values intact. |
| User idle past refresh-token TTL | On next interaction, refresh 401s → session-expired flow above. |
| Logout elsewhere / token revoked | Same as refresh failure. |

### 3.4 Error mapping

| Backend | code | UI |
|---------|------|-----|
| Refresh accepted | `200` new access (+ rotated refresh) | silent |
| Refresh rejected | `401 UNAUTHORIZED` | session-expired flow |
| Refresh malformed | `400 VALIDATION_ERROR` | treat as rejected → re-login |
| `500` on refresh | `500` | one retry w/ backoff; if still failing, error toast + keep user on page (don't eject on a transient 5xx), banner "Reconnecting…" |

**Refresh-token rotation:** if the backend rotates refresh tokens on every use, the client must atomically replace the stored token; a failed swap must not strand the user. Flagged Q5.

---

## 4. Logout

**Who:** All roles. **Trigger:** "Sign out" in the profile menu (`INFORMATION_ARCHITECTURE §2.1` Profile).

### 4.1 Flow

1. User clicks "Sign out".
2. If there is unsaved work on the current route, show confirm modal: "Sign out? Unsaved changes will be lost." (Cancel / Sign out).
3. Call `POST /api/v1/auth/logout` (revokes the refresh token server-side / clears the httpOnly cookie). Show a brief blocking spinner on the menu item.
4. Clear in-memory access token + any persisted refresh material. Clear per-user cached query state.
5. Route to `/login` with a success toast "You've been signed out."

### 4.2 States & errors

| Situation | UX |
|-----------|-----|
| Logout success | toast + route to `/login`. |
| `auth/logout` returns `500` / network error | **Still** clear client tokens and route to `/login` (local logout must always succeed from the user's POV). Fire-and-forget a background retry of the revoke; if it permanently fails, that's a server-side cleanup concern, not a user blocker. Toast: "Signed out." (do not surface the server error to the user). |
| Already logged out (no token) | route to `/login` directly. |
| Multi-tab | broadcast logout via `BroadcastChannel`/storage event so other tabs also drop to `/login`. |
| Flutter | clear secure-storage refresh token; if offline, perform local logout and queue the revoke for next connectivity. |

---

## 5. Password Reset (Request + Confirm) — incl. unconfigured-mailer state

**Who:** Any user who forgot their password. **Critical pivot dependency:** reset emails ship via **Resend**, which only works once the org's `resend` BYOK key is set (`mailer.ErrMailerUnconfigured` otherwise). The unconfigured state is a first-class design concern.

### 5.1 Screen P1 — "Reset your password" (request)

Centered glass-panel, 480px. Headline "Reset your password". Body: "Enter your email and we'll send a reset link." Single field **Email** + CTA **"Send reset link"**. Link "Back to sign in".

#### States

- **Default / Loading.**
- **Success (enumeration-safe):** Always show the **same** confirmation regardless of whether the email exists: "If an account exists for that email, a reset link is on its way. Check your inbox." (Prevents account enumeration.) Replace the form with this confirmation panel + a "Back to sign in" link and a "Resend" affordance (rate-limited, disabled for 30s with countdown).
- **Mailer unconfigured (the pivot state):** The backend's `mailer.Send` returns `ErrMailerUnconfigured` when no Resend key exists. **Two sub-cases — see Open Questions Q3 for which the backend adopts:**
  - **(Preferred) Enumeration-safe + silent drop:** API still returns the generic `200` "if an account exists…" success even when the mail was dropped (the email simply never arrives). The *operator/owner* is alerted out-of-band (e.g. an admin banner — see below). Rationale: never leak mailer-config state to an anonymous requester.
  - **(Alternative) Explicit signal:** API returns `503 MAILER_UNCONFIGURED`. Only acceptable if product decides the trade-off is fine for a single-tenant fork. UI: "Password reset email can't be sent yet — this BuildOS hasn't connected an email provider. Ask your administrator to add a Resend key under Settings → Integrations." Do **not** show this to anonymous users if enumeration safety is required.
- **Owner/admin-facing nudge:** Independently of the reset screen, when `resend` is unconfigured the **Integrations** screen (§7) and a global admin banner surface: "Email delivery is off. Password resets and notifications won't send until you add a Resend key." This is where the unconfigured state is *honestly* exposed (to a privileged, authenticated user) without leaking to anonymous requesters.

#### Error mapping

| Condition | code | UI |
|-----------|------|-----|
| Accepted (exists or not) | `200` (or `202`) | generic enumeration-safe confirmation |
| Mailer unconfigured | `ErrMailerUnconfigured` → see Q3 | per chosen strategy above |
| Rate limited | `429 RATE_LIMITED` | "Too many requests. Try again in N." countdown |
| `VALIDATION_ERROR` (bad email shape) | `400` | inline on email field |
| `500` | `500` | form-level "Couldn't start the reset. Try again." |

### 5.2 Screen P2 — "Set a new password" (confirm)

Reached via the emailed link carrying a single-use reset token (`?token=…`). Centered glass-panel. Headline "Set a new password". Token is read from the URL (not shown). Fields: **New password** (`autocomplete="new-password"` + strength meter), **Confirm new password**. CTA **"Update password"**.

#### States

- **Loading-on-mount:** Optionally validate the token on mount (lightweight) to fail fast on expired links; otherwise validate on submit. If validating on mount, show a centered spinner then the form.
- **Default / Submitting / Success:** On success, glow flash + route to `/login` with toast "Password updated — sign in with your new password." Do **not** auto-login (forces a fresh credential entry; cleaner audit trail). See Q8.
- **Invalid/expired/used token:** terminal panel "This reset link is invalid or has expired. Request a new one." + button "Request a new link" → P1. Like bootstrap, keep failure reasons **uniform** to avoid probing.

#### Error mapping

| Condition | code | UI |
|-----------|------|-----|
| Token bad/expired/used | `401`/`410` uniform (proposed `INVALID_RESET_TOKEN`) | terminal "invalid or expired link" + re-request |
| Weak password | `400 VALIDATION_ERROR` `details[].field=password` | inline, surface `reason` |
| Confirm mismatch | client-side | inline, instant |
| `500` | `500` | form-level retry |
| Token missing from URL | client-side | terminal "This link is incomplete" + re-request |

### 5.3 Edge cases

- **User clicks an old link after already resetting:** uniform invalid/expired terminal state.
- **Reset while the mailer was unconfigured, then a key is added:** the original request never produced an email; user must re-request. The §7 admin banner is the mechanism that gets the key added.

---

## 6. Onboarding Wizard (6 steps) — UI over the shipped service

**Who:** Owner (and admins — wizard routes are `RequireMinRole(RoleAdmin)`; bootstrap-redeem is the only non-admin setup route). Super/field cannot drive the wizard (Q7).
**Gate:** Until `onboarding_complete=true`, `SetupGate` 403s every non-exempt route with `SETUP_INCOMPLETE`. So the wizard runs in a **dedicated minimal shell** (progress rail + step content), not the full org shell.
**Source of truth for resume:** `GET /api/v1/setup/state` returns each section as present/absent; the UI lands the user on the first incomplete step.

The six product steps map onto the backend as follows (backend numbering in `setup.go` is non-contiguous — steps 1,3,4,5,6,8 — but the user sees six steps):

| UI Step | Title | Endpoint(s) | Required to Complete? |
|---------|-------|-------------|------------------------|
| 1 | Company info | `POST /setup/company-info` | **Yes** (legal_name) |
| 2 | Trades | `POST /setup/trades` (one row per call) | **Yes** (≥1 trade) |
| 3 | Cost codes | `POST /setup/cost-codes` (one row per call) | **Yes** (≥1 cost code) |
| 4 | Working calendar (+ holidays) | `POST /setup/calendars`, then `POST /setup/calendars/{id}/holidays` | **Yes** (a default calendar) |
| 5 | Permit jurisdictions | `POST /setup/jurisdictions` | No (optional) |
| 6 | Review & complete | `POST /setup/complete` | n/a — the finish line |

### 6.0 Shell, navigation & shared states

- **Progress rail** (left, glass-panel): six numbered steps with state dots — done (Gable Green check), current (green ring + glow), upcoming (muted), and **blocked** (amber, for steps with unmet prereqs the user tries to skip past). Steps 1–4 show a small "required" tag.
- **Resume:** on entry, `GET /setup/state`; compute the first incomplete required step and land there. A returning user who already did steps 1–3 lands on step 4.
- **Per-step Save behavior:** each step's "Save & continue" calls its endpoint(s). Because trades/cost-codes/holidays are **one-row-per-call**, the UI batches the user's list into sequential calls and surfaces **per-row** success/failure (the service is built so per-row UNIQUE collisions diagnose individually).
- **Loading:** `.skeleton` for the state fetch; CTA `loading` spinner on save.
- **Empty:** each list step (trades/codes/holidays/jurisdictions) shows an empty illustration + "Add your first …" primary action.
- **Global error states:**
  - `409 SETUP_ALREADY_COMPLETE` on any step → the org finished onboarding (e.g. another admin completed it in parallel). Show a modal "Setup is already complete" → route into the app. The service `guardNotComplete` enforces this on every mutation.
  - `403 SETUP_INCOMPLETE` should never hit the wizard itself (routes are exempt); if it does, treat as a routing bug, log to Sentry.
  - `401` → session-expired flow (§3).

### 6.1 Step 1 — Company info

`POST /api/v1/setup/company-info`. Body fields (all optional pointers server-side, but **legal_name is required to complete** — see §6.6): `legal_name`, `address`, `ein`, `company_type`, `region`.

Form:
- **Legal company name** (`legal_name`) — required (enforced at Complete; we require it here for a clean flow). Outfit.
- **Address** (`address`) — multiline.
- **EIN** (`ein`) — JetBrains Mono input (it's an identifier); optional. (Treat as Confidential PII in any logging — never echo into telemetry.)
- **Company type** (`company_type`) — `<fb-select>` (e.g. LLC, S-Corp, Sole Prop, …; exact list Q10).
- **Region** (`region`) — input with **ISO-3166-2 shape hint** ("e.g. US-CT"). The service validates a relaxed shape via `looksLikeRegion`.

Validation surfaced from service rules (`UpdateCompanyInfo`):

| Rule (server) | Client mirror | Error message on `400 VALIDATION_ERROR` |
|---------------|---------------|------------------------------------------|
| `at least one field required` | disable Save until ≥1 field changed | "Enter at least one detail to save." |
| `region must look like ISO-3166-2 (e.g. US-CT)` | live shape check | inline on region: surface server message verbatim |

CTA **"Save & continue"** → step 2. On `200`, store returned `company_profile`, advance.

### 6.2 Step 2 — Trades

`POST /api/v1/setup/trades`, **one call per trade**. Body: `code`, `name`, `description?`, `is_default`.

UI: an **editable list/table**. Each row: **Code** (JetBrains Mono, auto-uppercased to mirror server normalization `strings.ToUpper(TrimSpace)`), **Name** (Outfit), **Description** (optional), **Default** toggle. "Add trade" adds a row; "Save & continue" persists all rows.

Validation surfaced from `CreateTrade`:

| Rule (server) | Client mirror | Error |
|---------------|---------------|-------|
| `code must be 1..16 ASCII chars (A-Z, 0-9, _, -)` | mask input to allowed chars, 16-char cap | per-row inline: "Code must be 1–16 letters, digits, _ or -." |
| `name required` | disable row save when blank | per-row inline: "Name is required." |
| UNIQUE collision on code (`mapSetupStoreError` → 23505 → `400 VALIDATION_ERROR`) | — | per-row inline: "That trade code already exists." Keep other rows' successes. |

**Per-row result handling:** show a green check per persisted row, red row-level error per failure. "Save & continue" is allowed once ≥1 trade persisted successfully (Complete requires ≥1). Offer a **"Suggest common trades"** seed list (Q11) to reduce typing.

### 6.3 Step 3 — Cost codes

`POST /api/v1/setup/cost-codes`, **one call per code**. Body: `code`, `name`, `division`, `parent_code?`, `is_default`.

UI: editable table. **Code** (JetBrains Mono, masked to the **CSI MasterFormat** shape `NN-NN` or `NN-NN-NN`), **Name**, **Division** (e.g. "03 Concrete"), optional **Parent code**, **Default** toggle.

Validation from `CreateCostCode`:

| Rule (server) | Client mirror | Error |
|---------------|---------------|-------|
| `code must look like NN-NN-NN (CSI MasterFormat)` (`looksLikeCSICode`: 2 or 3 two-digit segments) | input mask `NN-NN[-NN]` | per-row: "Code must look like 03-30 or 03-30-00." |
| `name required` | disable blank | per-row: "Name is required." |
| `division required` | disable blank | per-row: "Division is required." |
| UNIQUE collision | 23505 → 400 | per-row: "That cost code already exists." |

Offer a **"Load CSI starter set"** (Q11). "Save & continue" enabled after ≥1 code persists.

### 6.4 Step 4 — Working calendar (+ holidays)

Two-phase within one step. First create the calendar (`POST /setup/calendars`), then add holidays against the returned `calendar.id` (`POST /setup/calendars/{calendarID}/holidays`).

**Calendar form** (`CreateCalendar`):
- **Name** (required).
- **Timezone** — `<fb-select>` of IANA zones (validated server-side via `time.LoadLocation`); default **America/New_York** (schema default). Catch typos before save.
- **Working days** — seven day-of-week toggles (Mon–Sun) → bitmap `working_days_mask` (Mon=bit0..Sun=bit6, `models.WorkingDayBit`). Default preset **Mon–Fri** (`WorkingDaysMonFri` = 31). Range 0..127.
- **Daily work minutes** — numeric (JetBrains Mono); default **480** (8h); must be 1..1440. Offer an "8h / 10h" quick-pick.
- **Set as default** — toggle; **must be the default** since Complete requires a *default* calendar; pre-check it and explain "This is your project default."

Validation from `CreateCalendar`:

| Rule | Client mirror | Error on 400 |
|------|---------------|--------------|
| `name required` | disable Save | "Calendar name is required." |
| `timezone … not a valid IANA TZ` | select from valid list only | "Pick a valid timezone." |
| `working_days_mask must be 0..127` | toggle UI can't exceed | (defensive) "Select your working days." |
| `daily_work_minutes must be 1..1440` | clamp input | "Daily work minutes must be between 1 and 1440." |

**Holidays sub-list** (`AddHoliday`, one call per holiday): each row **Date** (date picker → `YYYY-MM-DD`, the canonical wire shape; RFC3339 also accepted) + **Name**. Validation: `name required`, `holiday_date required`; UNIQUE collision per (calendar, date) → per-row "A holiday already exists on that date." Holidays are optional. Offer a **"Add US federal holidays"** seed (Q11).

**Sequencing UX:** holidays can only be added after the calendar is saved. Show holidays as disabled with helper "Save the calendar first" until `calendar.id` exists. "Save & continue" → step 5.

### 6.5 Step 5 — Permit jurisdictions (optional)

`POST /setup/jurisdictions`. Body: `name`, `region?`, `permit_types?` (JSON), `inspection_checklist?` (JSON), `notes?`.

UI: editable list. **Name** (required), **Region** (ISO-3166-2 hint), **Permit types** (tag/chip editor → serialized to a JSON array), **Inspection checklist** (repeatable text items → JSON array), **Notes**. The chip/list editors keep operators out of raw JSON; the UI serializes to the `json.RawMessage` fields the handler expects.

Validation from `AddJurisdiction`:

| Rule | Client mirror | Error |
|------|---------------|-------|
| `name required` | disable Save | "Jurisdiction name is required." |
| `permit_types must be valid JSON` | UI builds valid JSON from chips | (defensive) "Couldn't save permit types." |
| `inspection_checklist must be valid JSON` | UI builds valid JSON | (defensive) "Couldn't save the checklist." |

This step is **skippable**: secondary button **"Skip for now"** → step 6. Make clear it can be added later under Settings.

### 6.6 Step 6 — Review & complete

`POST /api/v1/setup/complete`. Idempotent; re-call returns the existing snapshot.

UI: a **read-only review** of everything captured (company info, trades count, cost-codes count, default calendar + holiday count, jurisdictions count), each section with an **"Edit"** link back to its step. Primary CTA **"Finish setup"**.

**Pre-flight prereq check (mirror of server `Complete`):** before enabling "Finish setup", confirm client-side that: legal_name present, ≥1 trade, ≥1 cost code, a default calendar exists. If any are missing, show an amber checklist with jump links and disable the CTA. This makes the server's 400s nearly impossible to hit, but we still map them:

| Server `Complete` failure | code | UI |
|---------------------------|------|-----|
| `company info incomplete (legal_name missing)` | `400 VALIDATION_ERROR` | jump to Step 1, highlight legal name |
| `at least one trade required` | `400` | jump to Step 2 |
| `at least one cost code required` | `400` | jump to Step 3 |
| `a default working calendar is required` | `400` | jump to Step 4 |
| already complete | `200` (idempotent) | treat as success → route into app |

**Success:** big green glow, confetti-free (Industrial ethos), toast "Setup complete — welcome to BuildOS." Route to the owner default workspace (`/portfolio/financials`). The `SetupGate` now passes for all routes.

**Note on AI/email during onboarding:** the wizard does **not** require any BYOK key. But the review step should surface a gentle nudge card: "Want AI briefings and email alerts? Add your Anthropic and Resend keys in Settings → Integrations." linking to §7. (Do not block completion on it.)

---

## 7. Integrations / BYOK Key Management

**Who:** **Owner and admin only.** Super/field never see Settings → Integrations. The screen lives **inside the full org shell** under `Settings → Integrations` (`INFORMATION_ARCHITECTURE §2.1` Settings; replaces the old "Brain integration" entry, which the pivot removes — Q12).
**Backend:** keys are encrypted via `internal/cryptobox` (AES-GCM) and resolved at call time by the `ai` / `mailer` `KeyResolver`s. **Secrets are never echoed back** — `GET …/credentials` returns metadata + a masked preview only.

**Providers & what each unlocks** (drives the empty-state copy):

| Provider | Unlocks | Empty-state consequence |
|----------|---------|--------------------------|
| `anthropic` | **All AI** (daily briefings, schedule recommendations, invoice extraction…) — `ai.ErrUnconfigured` when absent | "AI features are off" |
| `resend` | Transactional email incl. **password-reset delivery** + notifications — `mailer.ErrMailerUnconfigured` when absent | "Email delivery is off" |
| `gable` | Gable 3rd-party integration | provider-specific feature off |
| `localblue` | LocalBlue 3rd-party integration | provider-specific feature off |

**Scopes:** the DELETE path is `…/credentials/{scope}`, implying a provider may hold more than one credential keyed by scope (e.g. an org-level key vs. a project-scoped key, or `default`). The list endpoint returns one entry per (provider, scope). The UI groups by provider and lists scopes within. If the product only ever uses a single `default` scope per provider initially, the UI still renders the scope (as `default`) for forward-compat. **Flagged Q9.**

### 7.1 Screen I1 — Integrations overview (list)

`GET /api/v1/integrations/credentials`. Header: "Integrations" + subhead "Connect your own keys. BuildOS stores them encrypted and never shows them again."

A **provider card grid** (`<fb-card>`, glass), one card per provider (anthropic, resend, gable, localblue), each showing:
- Provider name + logo + one-line "what it unlocks".
- **Status badge** (`INFORMATION_ARCHITECTURE §6.3`):
  - **Connected** (Gable Green "Active") — key set; show masked preview + scope(s) + "last updated" timestamp (JetBrains Mono).
  - **Not connected** (Blueprint Blue "Pending" or gray) — empty state.
  - **Error / invalid** (Safety Red "Critical") — last test/use failed auth (see test affordance §7.4).
- Primary action: **"Connect"** (empty) or **"Manage"** (connected).

**Proposed list response shape (Q13):**
```json
{ "data": { "credentials": [
  { "provider": "anthropic", "scope": "default", "masked": "sk-ant-...****7f3a",
    "last_updated": "2026-05-30T12:00:00Z", "status": "connected" }
] } }
```
`masked` is server-rendered (UI never possesses the secret). Render `masked` in JetBrains Mono.

#### States

- **Loading:** skeleton cards.
- **Empty (no keys at all):** each provider card in its empty state; a top banner for the two consequential ones (anthropic, resend) — see §7.5.
- **Error fetching list:** full-section error panel + "Retry".
- **Role-gated:** if a non-owner/admin somehow reaches the route (deep link), show `403 FORBIDDEN` → "You don't have access to integrations" with a back link. Nav never renders the entry for them.

### 7.2 Screen I2 — Set / update a key (per provider)

`POST /api/v1/integrations/{provider}/credentials`. Opened as an `<fb-modal>` (glass-panel) from a provider card's Connect/Manage.

Fields:
- **API key / secret** — `<fb-input type="password">`, JetBrains Mono, `autocomplete="off"`, show/hide toggle, paste button. Helper text per provider (e.g. anthropic: "Starts with `sk-ant-`. Find it in the Anthropic Console."). Client-side **format hint** only (e.g. prefix check) — server is source of truth.
- **Scope** — if scopes are exposed (Q9): `<fb-select>` or text, defaulting to `default`. Hidden/locked to `default` if the product is single-scope initially.
- (Optional, provider-dependent) additional non-secret config (e.g. Resend "from" identity is **not** in the vault per `resend.go` — it's mailer config; if surfaced at all it belongs to a separate mailer-settings form, **not** here — Q14).

Actions: **"Save key"** (primary), **"Test key"** (secondary — see §7.4), **Cancel**.

#### States

- **Default / Loading (saving).**
- **Success:** modal closes, provider card flips to **Connected** with masked preview + glow flash; success toast "Anthropic key saved. AI features are now on." For `resend`: "Resend key saved. Email delivery is now on." (and the global "email off" banner clears).
- **Update existing:** same modal pre-fills nothing (secret never returned) — it shows the masked current value as read-only context with copy disabled, and a fresh empty secret field labeled "Replace key". Saving overwrites; warn "This replaces the current key."
- **Validation error:** inline on the secret field.

#### Error mapping

| Condition | code (proposed) | UI |
|-----------|-----------------|-----|
| Empty/blank secret | client + `400 VALIDATION_ERROR` | inline "Enter the API key." |
| Malformed for provider (server-validated) | `400 VALIDATION_ERROR` | inline, surface `reason` |
| Encryption/storage failure | `500 INTERNAL_ERROR` | modal-level "Couldn't save the key. Try again." (never log the key) |
| Unknown provider in path | `404 NOT_FOUND` | shouldn't happen from UI; toast "Unknown integration." |
| Not owner/admin | `403 FORBIDDEN` | modal blocks; "You don't have access." |
| Rate limited | `429` | "Too many attempts. Try again shortly." |

### 7.3 Delete a key

`DELETE /api/v1/integrations/{provider}/credentials/{scope}`. From the provider's Manage view, a **"Remove key"** destructive action (`btn-destructive`, Safety Red border).

- **Confirm modal** (required — destructive): "Remove the {provider} key? {Consequence}. You can add a new one anytime." Consequence text is provider-aware: anthropic → "AI features will turn off"; resend → "Password-reset and notification emails will stop sending". Type-to-confirm not required (single fork, owner-trusted), but the destructive styling + explicit consequence is mandatory.
- **States:** Loading (button spinner) → on success the card returns to **Not connected** empty state + toast "{Provider} key removed. {Feature} is now off."; the relevant global banner (§7.5) reappears.

#### Error mapping

| Condition | code | UI |
|-----------|------|-----|
| Removed | `200`/`204` | success toast + card → empty |
| Scope/provider not found (already gone) | `404 NOT_FOUND` | treat as success (idempotent UX): card → empty, info toast "Already removed." |
| Not owner/admin | `403 FORBIDDEN` | block |
| `500` | `500` | "Couldn't remove the key. Try again." card unchanged |

### 7.4 "Test key" affordance

Validating a key without waiting for a real feature to fail is high-value (especially during onboarding). Two implementation options — **Flagged Q15**:

- **(Preferred) Dedicated test endpoint** (proposed `POST /api/v1/integrations/{provider}/test`): server makes a cheap authenticated probe (e.g. Anthropic models list; Resend domains/whoami) using the just-entered or stored key and returns `{ ok, detail? }`. UI shows inline result: green "Key works." / red "Key was rejected by {provider} ({status})." Available both in the Set modal (test before save) and on the Manage view (test stored key).
- **(Fallback) No test endpoint:** omit the button; instead, the provider card surfaces a **derived health state** from the last real call (e.g. if the most recent AI call returned an auth `4xx`, mark the card "Error — key may be invalid"). Less immediate but no new endpoint.

**States for test:** idle → testing (spinner) → success (green inline + "Key works") / failure (red inline, show provider status code, never the key) / unreachable (amber "Couldn't reach {provider} — try again").

### 7.5 Empty / "feature disabled until key set" states (the pivot's signature states)

These appear **outside** Integrations too — wherever a feature would call the missing provider. They are driven by the backend soft errors `ai.ErrUnconfigured` / `mailer.ErrMailerUnconfigured`.

| Surface | Trigger | Empty state |
|---------|---------|-------------|
| Morning Briefing / any AI feed card region | AI call → `ai.ErrUnconfigured` (or briefing absent because the job no-ops without a key) | Inline empty card: hard-hat-style icon, "AI briefings are off", body "Add your Anthropic key to turn on daily briefings, schedule suggestions, and invoice reading.", **owner/admin** see a primary "Add Anthropic key" → I2; **super/field** see "Ask your owner to connect AI." (no link). |
| Schedule "Recommend adjustments" (Pro + AI) | `ai.ErrUnconfigured` | disabled button with tooltip "Connect an Anthropic key to enable AI recommendations" (+ link for owner/admin). |
| Password reset (§5) | `mailer.ErrMailerUnconfigured` | per §5.1 strategy (enumeration-safe). |
| Global admin banner (owner/admin only) | `resend` and/or `anthropic` unconfigured | Dismissible-per-session amber banner at top of org shell: "Email delivery is off — password resets won't send. Connect Resend." and/or "AI features are off — connect Anthropic." each with a "Set up" link to Integrations. **Never shown to super/field.** |
| Settings → Integrations cards | per provider | the per-card empty states (§7.1). |

**Consistency rule:** every "feature off" state names the **specific provider** to connect, states the **concrete consequence**, and (for owner/admin only) offers a **one-click path to I2**. For non-privileged roles it states who to ask, with no actionable link (they can't set keys).

### 7.6 Role-gating summary (Integrations)

| Capability | owner | admin | superintendent | field_worker |
|------------|:-----:|:-----:|:--------------:|:------------:|
| See Settings → Integrations nav entry | Yes | Yes | No | No |
| List credentials | Yes | Yes | — (403) | — (403) |
| Set/update key | Yes | Yes | — | — |
| Test key | Yes | Yes | — | — |
| Delete key | Yes | Yes | — | — |
| See "feature off" empty states | Yes (+ link) | Yes (+ link) | Yes (no link, "ask owner") | Yes (no link) |
| See global admin "email/AI off" banner | Yes | Yes | No | No |

(Aligns with `API_CONTRACT §1.2` where financial/sensitive surfaces are owner/admin; integrations hold secret material so they're the most restricted.)

---

## 8. Cross-cutting requirements

- **No secret ever round-trips to the client after save.** List shows server-rendered masked previews only; the secret field is write-only.
- **No secret in telemetry.** EIN, API keys, tokens, emails, passwords are Confidential/Restricted PII (`CLAUDE.md` PII taxonomy). The UI must not put them in URLs (except the necessarily-URL reset/bootstrap tokens, which must be treated as single-use and scrubbed from analytics/referrers — set `referrerpolicy=no-referrer` on the reset screen and strip the token from the address bar after read).
- **Accessibility:** all forms keyboard-navigable; `:focus-visible` 2px `#00FFA3`; errors via color+text+icon; show/hide password toggles are real buttons with aria labels; modals trap focus and restore it on close.
- **Reduced motion:** glow/skeleton respect `prefers-reduced-motion` (opacity-only fallback).
- **Field Portal (Flutter):** login + reset reuse the same contracts; 64px targets; offline banner disables auth CTAs; the wizard and Integrations are **web-only** (owner/admin surfaces) — Carlos never sees them.

---

## 9. Open Questions (for the user / backend ratification)

1. **Bootstrap shape pivot (Q1).** The shipped `POST /api/v1/setup/bootstrap` is JWT-authenticated, takes `{token}` only, and assumes a pre-provisioned user (`BOOTSTRAP_USER_NOT_PROVISIONED`/412). The native target is **unauthenticated** `{token, email, password}` that creates the owner inline and returns tokens. Confirm the target contract and whether the legacy JWT path is removed or kept during transition. This determines whether §1 shows a 412 path at all.
2. **Password policy (Q2).** What is the server password policy (min length, complexity, breach check)? The UI mirrors it client-side. Default assumption: ≥12 chars.
3. **Mailer-unconfigured response (Q3).** For `POST /auth/password-reset/request` when `resend` is unset: enumeration-safe silent `200` (preferred) or explicit `503 MAILER_UNCONFIGURED`? Drives §5.1.
4. **Login lockout (Q4).** Is there account lockout / throttling on failed logins, and what code (`429` vs `403 ACCOUNT_LOCKED`)? Needed for the countdown UX.
5. **Refresh-token transport & rotation (Q5).** Is the refresh token an httpOnly Secure cookie (preferred) or returned in the JSON body? Does it rotate on every refresh? Drives §3 storage + single-flight logic.
6. **Form preservation on expiry (Q6).** Confirm desired behavior when a session expires mid-edit: blocking modal + preserve form (proposed) vs. eject to login. Any drafts persisted server-side?
7. **Pre-onboarding non-admin login (Q7).** A super/field who logs in before onboarding completes can't drive the wizard (admin-gated). What should they see — a "setup in progress" interstitial (proposed) or be blocked from login entirely until onboarding is done?
8. **Post-reset auto-login (Q8).** After `password-reset/confirm`, route to `/login` (proposed, cleaner audit) or auto-issue a session?
9. **BYOK scope semantics (Q9).** What does `{scope}` mean for credentials — `default` only, or per-project / per-environment scopes? Can one provider hold multiple scoped keys? Drives the I1/I2 scope UI.
10. **Company type enum (Q10).** Exact allowed `company_type` values for the Step 1 select.
11. **Wizard seed lists (Q11).** Do we ship starter sets for trades, CSI cost codes, US federal holidays to reduce typing? If so, where do they live (frontend constant vs. a seed endpoint)?
12. **Settings nav cleanup (Q12).** The IA still lists `Settings → Brain integration` (`/settings/brain`). The pivot removes Brain; confirm it's replaced by `Settings → Integrations` and the `/settings/brain` route is retired.
13. **List response shape (Q13).** Confirm the `GET /integrations/credentials` response (provider, scope, server-rendered `masked`, `last_updated`, `status`). Confirm masking is server-side.
14. **Resend "from" identity (Q14).** `from`/`fromName` are mailer config, not vault secrets (`resend.go`). Where does the operator set the sender identity — env/operator config only, or a separate (non-BYOK) Settings form? Should it appear near the Resend card?
15. **Test-key endpoint (Q15).** Approve a dedicated `POST /integrations/{provider}/test` probe endpoint (preferred) vs. deriving health from last real call? Drives §7.4.
16. **A2A / agent-card under native (Q16).** `internal/api/a2a.go` + `/api/v1/a2a/webhook` reference Brain as the JWS signer/issuer (`iss: fb-brain`). Out of scope here, but the pivot likely reshapes A2A too — flagging so it's not forgotten.
