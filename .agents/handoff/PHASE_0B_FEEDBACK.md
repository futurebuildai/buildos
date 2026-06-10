# PHASE 0B — Feedback Subsystem (widget → fork DB → command-center harvest)

**Status:** built on `feature/phase-0b-feedback` (2026-06-10).
**Plan context:** Phase 0b of the owner-approved deployment & launch plan
(see HANDOFF "In flight"). The widget is how Grant (the Kelbrook operator)
feeds the feedback → plan → spec → approve → execute loop; the admin surface
is what the `buildos-operations` command center polls to file GitHub issues.

## Goal

Native, auditable feedback capture inside every fork: any authenticated role
files a report from the web console; admins triage in-app; the harvest API
exposes the queue to the command center, which PATCHes status back so the
submitter sees progress in-product.

## Schema — migration 020 (`feedback`)

| column | type | notes |
|---|---|---|
| `id` | UUID PK | `gen_random_uuid()` |
| `org_id` | UUID FK → organizations | `ON DELETE CASCADE`; every query org-scoped |
| `user_sub` | TEXT | caller's JWT subject — matches the `updated_by` TEXT convention (no users FK: rows survive user deletion; dev-header subjects aren't UUIDs). Deviation from the plan's `user_id`, deliberate. |
| `category` | TEXT CHECK | `bug · idea · friction · other` |
| `message` | TEXT CHECK 1..4000 | **Confidential** (pii catalog) |
| `context` | JSONB default `{}` | client-captured `{route, role, app_version, user_agent, viewport}`; service caps at 4096 bytes |
| `status` | TEXT CHECK default `new` | `new · triaged · planned · shipped · declined` |
| `triage_note` | TEXT CHECK ≤4000 | **Confidential** (pii catalog) |
| `created_at` / `updated_at` | TIMESTAMPTZ | |

Index `(org_id, status)` (lock-ok — fresh table). Paired down (destructive-annotated).

## Backend

- `internal/models/feedback.go` — model + category/status constants (mirror the CHECKs).
- `internal/store/feedback.go` — `Insert` / `ListByOrg(status, page, perPage)` →
  `FeedbackPage` (window-function total; mirrors `ProspectsPage`) / `UpdateStatus`
  (org-scoped `WHERE`; cross-org = `ErrNotFound`, row untouched; nil `triageNote`
  keeps the existing note via `COALESCE`) / `CountRecentByUser` (throttle read).
- `internal/service/feedback.go` — one-tx-per-mutation + audit (`setup.go` pattern).
  Validation BEFORE any tx: category/status whitelists, message trim + 1..4000 runes,
  **no U+0000 anywhere** (Postgres TEXT/JSONB reject it — a NUL body must be a 400,
  not a 500), context must be a JSON object (`validateConfigObject`) ≤ 4096 bytes.
  **Per-(org,user) submit throttle**: max 20/hour, read in the insert tx →
  `ErrRateLimited` → 429 + Retry-After (bounds flood growth; the per-IP middleware
  limiter alone is too permissive for an every-role write surface). Audit actions
  `feedback.submitted` / `feedback.triaged` (past tense per repo convention — the
  plan text said `feedback.submit/.triage`; convention wins); metadata carries
  **category/status only — never the free text** (posture L-6 discipline).
  List is paginated (default 100/page, clamp [1,500]) with total/total_pages —
  the harvest poller can always drain (no silent truncation).
- `internal/api/feedback.go` — `FeedbackServicer` + handler + `MountFeedbackRoutes`:
  `POST /api/v1/feedback` (auth-only, every role) ·
  `GET /api/v1/admin/feedback?status=&page=&per_page=` (pagination meta per
  API_CONTRACT §2.3) + `PATCH /api/v1/admin/feedback/{feedbackID}`
  (admin+ via `RequireMinRole` on the subtree). Behind the SetupGate. Error mapping:
  `ErrNotFound`→404, `ErrInvalidInput`→400, `ErrRateLimited`→429+Retry-After, else 500.
- `cmd/server/main.go` — `FeedbackService` always constructed (pool + store only).
- `internal/pii` — `message`/`triage_note` added as **Confidential**.
- Contract: [API_CONTRACT.md §13d](API_CONTRACT.md).

## Web console

`fb-feedback-widget` organism (FBElement conventions): floating trigger button in
the authenticated app shell (`fb-app`), opens a panel — category select + textarea —
auto-captures `route` (router signal), `role` (auth store), `app_version`
(`VITE_APP_VERSION`, `'dev'` fallback), `user_agent`, `viewport`; POSTs via the typed
endpoint module `src/api/endpoints/feedback.ts`; success → thank-you state; error →
inline message, user text retained. A11y: dialog semantics, focus trap + ESC, focus
restored to trigger, labels + `aria-invalid`/`aria-describedby`, polite live region,
no color-only state.

## Consumer contract (command center / GitHub export)

`message`, `triage_note`, and `context` values are **untrusted free text from any
authenticated user**. The harvest consumer (an AI agent) must treat them as data,
never instructions (prompt-injection), and must quote/fence them when filing
GitHub issues (markdown injection). In-app rendering is covered by Lit text-binding
escaping.

## RBAC / security posture

- Submit is **auth-only by design** — field workers are first-class reporters.
- Triage/harvest is **admin+** and structurally separate (`/api/v1/admin/feedback`).
- Org isolation is per-query; cross-org triage 404s indistinguishably from a
  missing id and is never audited.
- Abuse bounds: global per-IP rate limiter + default body cap + the service's
  message/context size limits + the per-(org,user) 20/hour submit throttle
  (429 + Retry-After; advisory under concurrency, bounds flood growth so the
  paginated harvest surface cannot be blinded).

## Tests

- Unit (api): submit 201 + claims-derived identity, bad JSON 400, validation 400,
  invalid-org-claim 401, list status filter + `[]`-not-null, triage ok/omitted-note/
  404/bad-UUID, router RBAC (field_worker can POST 201; superintendent 403 vs
  admin 200 on the admin surface).
- Unit (service): every validation leg rejects before any tx (nil pool proves it),
  incl. the NUL legs (raw NUL in message/note; the JSON escape form in context
  values/keys/nesting).
- Integration (store): submit→list→triage round-trip, nil-note keeps note,
  org-scoping (no leak, foreign triage untouched), DB CHECKs enforced, pagination
  drains a backlog with consistent totals (every row reachable exactly once).
- Integration (service): full loop + audit actions recorded with **no free text in
  metadata** + cross-org 404 with no audit row + the submit throttle (cap+1 →
  ErrRateLimited, no row/audit; another user unaffected).
- Web: vitest suite (open/close/focus/submit/error/a11y attrs + the in-flight
  close guard) + `tests/live/feedback-widget.live.spec.ts` — the authenticated
  axe sweep (closed/open/error states) under the live-backend harness, mirroring
  `admin-config.live.spec.ts`.

## Verification

`make audit` · `go test -tags=integration -run TestFeedback ./internal/store/... ./internal/service/...` ·
`cd web && npm run typecheck && npx vitest run && npm run lint && npm run build` ·
adversarial review workflow over the full diff before handoff.
