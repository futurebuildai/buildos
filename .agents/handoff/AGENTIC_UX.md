# AGENTIC_UX — file-level implementation spec

**Batch:** Agentic-UX (branch `feature/agentic-ux`). **Owner-facing thesis:** the
agentic backend is already good and real — make it **VISIBLE** and **ACTIONABLE** in
the web console, per [VISION.md](../../VISION.md) ("the harness IS the product"). A
hands-on staging audit found four surfaces where a working backend capability is
hidden or inert behind the UI:

1. The **AI Assistant** page is two static link cards — yet `POST /api/v1/agents/chat`
   is a working bounded Claude tool-use loop over ~8 read-only ERP tools. The #1
   vision role (the conversational ERP experience) is invisible.
2. **Schedule "Suggest adjustments"** renders as a wall of "Advisory" text you close,
   with no per-item state — and the per-row identity it tries to show
   (`wbs_code · name`, `old → new`) is **not on the wire**, so every row reads
   `undefined · undefined / Advisory`.
3. **Feed cards** have a "Review impact" button that silently marks the card actioned
   (visually a second dismiss); the `project_id` + structured action payload that
   would let it navigate + show detail are dropped on the floor. Briefing renders
   **raw markdown** (literal `**asterisks`).
4. The **Gantt** shows WBS codes only — no task names, no date axis, no dependency
   arrows, no click-to-inspect — though `GetGantt` carries names + all dates (and the
   store already loads deps).

This spec is the binding plan to close all four. It honors the house rules:
design-tokens-only (Vanilla CSS, dark-only), **no new dependencies** (TECH_STACK is
authoritative — the markdown renderer is hand-rolled), **JetBrains Mono**
(`--fb-font-mono`) for numerics/dates, the composite-currency rule (integer-cents +
`currency_code`, never float client-side), and the live-spec ordering (sweep
HANDOFF/NEXT_STEPS/CLAUDE + touched specs before merge).

Maps to backlog tasks **#22** (Chunk 1), **#23** (Chunk 2), **#24** (Chunk 3),
**#25** (Chunk 4), **#26** (verification loop).

---

## 0. Build order & parallelization

The shared markdown renderer (Chunk 4) is a **hard dependency of Chunk 1** (chat
replies) and a **fix for Chunk 2** (briefing literal `**`). Sequence Chunk 4 **first**
(it is small and self-contained), then the rest can run in parallel because they touch
disjoint files.

```
        ┌─ Chunk 4: web/src/lib/markdown.ts + fb-markdown atom  (DO FIRST)
        │     └─ consumed by Chunk 1 (chat) + Chunk 2 (briefing)
        │
  parallel after Chunk 4 lands:
        ├─ Chunk 1: fb-assistant-page.ts + api/endpoints/assistant.ts + models.ts (ChatTurn/…)
        ├─ Chunk 2a (web-only): fb-feed-list.ts + fb-feed-card link affordance + briefing markdown swap
        ├─ Chunk 2b (BACKEND, escalated): enrich ScheduleAdjustmentSet + add dry-run preview
        └─ Chunk 3: internal/service/schedule.go (deps, ~15 lines) + fb-gantt-chart.ts + fb-schedule-page detail drawer
```

**File-conflict map (who touches what):**

| File | Chunk(s) | Conflict risk |
|---|---|---|
| `web/src/lib/markdown.ts` (new) | 4 | none (new) |
| `web/src/components/atoms/fb-markdown.ts` (new) | 4 | none (new) |
| `web/src/api/endpoints/assistant.ts` (new) | 1 | none (new) |
| `web/src/components/pages/fb-assistant-page.ts` | 1 | rewrite, isolated |
| `web/src/components/pages/fb-briefing-page.ts` | 2a (swap renderReply→fb-markdown) | low |
| `web/src/components/molecules/fb-feed-list.ts` | 2a | isolated |
| `web/src/components/molecules/fb-feed-card.ts` | 2a (add link slot) | low |
| `web/src/components/pages/fb-schedule-page.ts` | 2b (adj drawer) + 3 (task detail drawer) | **SHARED — coordinate** |
| `web/src/components/organisms/fb-gantt-chart.ts` | 3 | isolated |
| `web/src/types/models.ts` | 1 (`ChatTurn`/`ChatResponse`) + 2b (`ScheduleAdjustment` already declared, align) + 3 (`TaskDependency`, `GanttView.dependencies`) | **SHARED — additive only, low** |
| `internal/service/schedule.go` | 3 (Gantt deps) | isolated |
| `internal/service/agents.go` + `internal/api/agents.go` | 2b (enrich + dry-run) | isolated from web |

**Only genuinely shared web file is `fb-schedule-page.ts`** (Chunk 2b adjustments
drawer + Chunk 3 task-detail drawer). If both run concurrently, do the Chunk 3
detail-drawer addition and the Chunk 2b drawer rework as two small, well-separated
edits (different methods), or land Chunk 3's page edit first. `models.ts` edits are
all additive (new interfaces / new optional fields) and won't collide.

---

## CHUNK 4 — Shared markdown renderer (DO FIRST)

**Why first:** the chat reply (Chunk 1) and the briefing (Chunk 2a) both render
model-authored markdown. The existing `renderReply` in
`fb-briefing-page.ts:104-110` only splits on `\n{2,}` into `<p>` — it leaves literal
`**bold**` and `- bullets` in the text. That is the audit's "literal asterisks" bug.
TECH_STACK forbids a markdown dependency, so build a tiny, **safe** (escaped) renderer
in-repo and reuse it everywhere.

### Files
- **create** `web/src/lib/markdown.ts` — pure function `markdownToTemplate(src: string): TemplateResult`.
- **create** `web/src/components/atoms/fb-markdown.ts` — `<fb-markdown .source=${...}>` thin wrapper that calls the lib and applies prose styles. (One styled surface reused by briefing + chat + advisories.)

### Scope of markdown supported (deliberately minimal, the model's actual output)
- Paragraphs (blank-line separated) → `<p>`.
- Inline `**bold**` → `<strong>`, `*italic*`/`_italic_` → `<em>`, `` `code` `` → `<code>`.
- Unordered lists (`- ` / `* ` line-leading) → `<ul><li>`; ordered (`1. `) → `<ol><li>`.
- Headings `#`..`###` → `<h3>`/`<h4>` (clamp; never an `<h1>` inside a page).
- Line breaks inside a paragraph preserved.
- **Out of scope:** links/images/HTML passthrough/tables. Any raw `<...>` is **escaped
  and rendered as text** — never injected as HTML. This is the security guarantee:
  build a Lit `TemplateResult` tree from parsed tokens with values bound as text
  (Lit auto-escapes interpolated strings); **never** use `unsafeHTML`.

### Implementation note (no new dep, XSS-safe)
Tokenize into block nodes (paragraph / list / heading), then for inline spans split on
the marker regexes and emit `html`...`` fragments where the literal text segments are
**interpolated values** (escaped by Lit), and only the wrapping `<strong>`/`<em>`/`<code>`
tags are static template structure. Because user/model text only ever lands in a
`${value}` slot, no markup can be injected. No `unsafeHTML`, no `DOMParser` of model
output.

### Styles (fb-markdown, tokens only)
`p` → `--fb-text-primary`, `line-height: 1.6`; `strong` → 600 weight; `code` →
`--fb-font-mono`, `--fb-surface-2` background, `--fb-radius-sm`; `ul/ol` indented;
`h3/h4` → `--fb-text-primary`, `--fb-text-title-sm`. No hardcoded colors.

### Consumers (this chunk wires them)
- `fb-briefing-page.ts`: replace `renderReply(b.reply)` (lines 104-110, 145) with
  `<fb-markdown .source=${b.reply}></fb-markdown>`; delete `.reply p` CSS that is now
  inside the atom. **This alone fixes the briefing literal-`**` bug.**
- `fb-assistant-page.ts`: render each assistant turn's `reply` via `<fb-markdown>`.

### Test plan (vitest, `web/tests/markdown.test.ts`)
- `**bold**` → contains `<strong>bold</strong>`, no literal `*`.
- `- a\n- b` → `<ul>` with two `<li>`.
- **XSS:** `<img src=x onerror=alert(1)>` and `[x](javascript:...)` → output text
  contains the **escaped** string, NOT a live `<img>`/`<a>` element (assert
  `el.querySelector('img')` is null and `el.textContent` contains the literal).
- Paragraph split parity with the old `renderReply` (no regression on plain prose).
- Mount `<fb-markdown>` and run the live axe sweep (see §VERIFICATION) — prose region
  has no violations.

---

## CHUNK 1 — Real conversational assistant (HIGHEST PRIORITY, task #22)

Turn `fb-assistant-page` from a 2-card launcher into a real chat over
`POST /api/v1/agents/chat`. The endpoint is **multi-turn but STATELESS server-side** —
the **client owns the conversation history** and resends it (capped) each call. There
is **no `session_id`** in the response (unlike `DailyBriefing`); do not add session
handling — the client IS the memory.

### Multi-turn-vs-single-shot decision
**Build multi-turn, client-owned.** Each `sendChat` call is single-shot server-side,
but the page keeps the running thread in component state and passes prior turns as
`history`. Trim to the caps **before** every send so the server's hard 400s never trip
in normal use:
- ≤ **10** turns (`chatHistoryMaxTurns`) → slice to the last 10 prior turns.
- ≤ **8000** chars per single message (`chatMessageMaxChars`) → block the send + inline
  hint if the draft exceeds it.
- ≤ **24000** chars total (`chatHistoryMaxTotalChars`, sum of all history text + new
  message) → if exceeded after slicing to 10 turns, drop oldest turns until under
  budget (keep the newest context).

### Request / response (verified against `internal/api/assistant.go`)
```
POST /api/v1/agents/chat
req  { message: string,            // required, ≤8000 chars
       history?: { role: 'user'|'assistant', text: string }[] }  // ≤10 turns, Σchars ≤24000
200  { reply: string,
       tools_used: { name: string, is_error: boolean }[],   // name + is_error ONLY
       iterations: number,
       truncated: boolean }                                  // unwrapped from envelope .data
```
- **Identity is NEVER in the body** — org/role/sub come from JWT claims server-side. Do
  not add any identity field.
- `tools_used` is the wire-safe transparency surface: **name + is_error only**, never
  args/results (Confidential, withheld by design — see `assistant.go:47-53`). Render as
  chips; do not attempt to show more.
- `truncated=true` is a **200**, not an error: the loop hit a bound before `end_turn`.
  Render the reply AND a muted "this answer may be incomplete" note.

### Errors (via `writeAIServiceError`, shared with `/agents/*`)
| Status | code | UI treatment |
|---|---|---|
| 503 | `SERVICE_UNAVAILABLE` | **AI off.** `markAiUnconfigured()` + `aiState='gated'`. Owner-only "Go to Integrations →" deep link. NOTE: the code is literally `SERVICE_UNAVAILABLE`, NOT `AI_UNCONFIGURED`; branch `err.code === ErrorCode.SERVICE_UNAVAILABLE \|\| err.isAiUnconfigured` (exactly `fb-briefing-page.ts:89`). |
| 403 | `CAPABILITY_DISABLED` | **Admin kill-switch** (config state, not outage). `aiState='disabled'`: render the gated panel with `userMessageForCode(CAPABILITY_DISABLED)` ("The AI assistant has been turned off by an admin.", `errors.ts:126`) and **NO owner key-link** (it's not a missing key). |
| 429 / 502 / 503 `AI_CIRCUIT_OPEN` | transient | inline retry on that turn (don't drop the user's typed message). |
| 400 | `VALIDATION_ERROR` | inline error on send (show `err.details`/message); should be unreachable if client caps are respected. |
| 401 | `UNAUTHORIZED` | handled globally by the client refresh interceptor (`client.ts:135-147`). |

### Files
- **create** `web/src/api/endpoints/assistant.ts`:
  ```ts
  import { api } from '../client.js';
  import type { ChatTurn, ChatResponse } from '../../types/models.js';
  /** POST /api/v1/agents/chat. STATELESS server-side: client owns + resends history
   *  (caps: ≤10 turns, ≤24k total chars, ≤8k per message). */
  export function sendChat(message: string, history: ChatTurn[]): Promise<ChatResponse> {
    return api.post<ChatResponse>('/api/v1/agents/chat', { message, history });
  }
  ```
- **modify** `web/src/types/models.ts` (additive):
  ```ts
  export interface ChatTurn { role: 'user' | 'assistant'; text: string; }
  export interface ToolTrace { name: string; is_error: boolean; }
  export interface ChatResponse { reply: string; tools_used: ToolTrace[]; iterations: number; truncated: boolean; }
  ```
- **rewrite** `web/src/components/pages/fb-assistant-page.ts` (keep the page shell + the
  `connectedCallback` snapshot pattern; replace the card grid with the chat).

### Component design (`fb-assistant-page`)
State (`@state`): `messages: { role:'user'|'assistant'; text:string; tools?:ToolTrace[]; truncated?:boolean; error?:string }[]`, `draft: string`, `sending: boolean`, `aiState: 'ok'|'gated'|'disabled'|'transient'`, `errorRequestId: string|null`.

- **Proactive gate (connect):** snapshot `aiConfigured.get()` in `connectedCallback`
  (FBElement is NOT signal-reactive — read imperatively, like the existing page does at
  `:94-100`). If false → `aiState='gated'`, render gated panel, never call the endpoint.
- **Composer:** labelled `<textarea>` (`<label>` or `aria-label="Ask the assistant"`) +
  send `fb-button icon="arrow-up"` (`send` icon does NOT exist in `icons.ts:11-59` — use
  `arrow-up`). `?loading=${this.sending}`. **Enter** sends, **Shift+Enter** newline,
  disabled while `sending` or draft empty/whitespace, blocked >8000 chars with an inline
  hint.
- **Send flow:** push the user turn → build `history` from prior turns (map
  `{role,text}`, drop `tools`/`truncated`/`error` metadata) → slice to last 10 turns →
  drop oldest until Σchars ≤ 24000 → `await sendChat(draft, history)` → push assistant
  turn `{role:'assistant', text:reply, tools:tools_used, truncated}`. On catch, mirror
  `fb-briefing-page.ts:86-100`.
- **Reply rendering:** `<fb-markdown .source=${turn.text}>` (Chunk 4). User turns render
  as plain escaped text in a distinct bubble.
- **Grounding chips:** under each assistant reply, render `tools_used` as plain
  (non-selectable) `fb-chip`s in a labelled group (`role="group" aria-label="Sources used"`),
  mapping tool name → friendly label + icon:
  | tool name | label | icon (`icons.ts`) |
  |---|---|---|
  | `list_projects`, `get_project` | Projects | `folder` |
  | `get_schedule_gantt`, `list_project_tasks` | Schedule | `calendar` |
  | `list_procurement` | Procurement | `package` |
  | `list_feed_cards` | Feed | `inbox` |
  | `get_project_budgets`, `get_org_financials` | Financials | `dollar` |
  | `conn__*` (prefix) | Connector | `command` |
  Dedupe by label. `is_error:true` chips get a muted/red style
  (`color: var(--fb-safety-red-text)`) and an "(failed)" affix. **Never show args/results.**
- **Truncated note:** if `turn.truncated`, append muted line "This answer may be
  incomplete (hit a reasoning limit)." under the reply.
- **Starter prompts:** when `messages.length === 0 && aiState === 'ok'`, render 3–4
  suggestion `fb-chip`s that prefill+send on click:
  "What needs my attention today?", "Which projects have critical-path risk?",
  "Show procurement items at risk", and (gate behind `hasMinRole('admin')`)
  "Summarize budget variance by project".
- **States:** in-flight → a `role="status" aria-live="polite"` "Assistant is thinking…"
  row (typing dots; suppress animation via the FBElement reduced-motion rule). `gated`
  (503) → `fb-state mode="gated"` exactly like briefing (`?can-configure=${hasRole('owner')}`,
  `@configure=${() => navigate('/settings/integrations')}`). `disabled`
  (`CAPABILITY_DISABLED`) → `fb-state mode="gated"` with the admin-off copy and **no**
  configure link.

### RBAC
SPA route `/command/assistant` already gated `minRole: superintendent`, **not**
plan-gated (`router.ts:139-147`, ESC-002). Backend route gate
`RequireMinRole(superintendent)` (`router.go:495`). **No router change needed.**

### a11y (DSC §9 / axe)
- Thread wrapper: `<div role="log" aria-live="polite" aria-label="Conversation">` so SR
  users hear each new assistant reply.
- Composer textarea has a `<label>` (or `aria-label`); send button has a discernible
  name; full keyboard (Enter/Shift+Enter).
- Tool chips inside `role="group" aria-label="Sources used"`.
- Typing indicator is an `aria-live` status, not a bare spinner.

### Test plan (vitest, `web/tests/assistant-page.test.ts`; pattern = `command-pages.test.ts`)
Mock `../src/api/endpoints/assistant.js` (`sendChat`). Cases:
1. Empty thread + AI on → renders composer + starter prompts; the admin-only starter is
   absent for a superintendent claim, present for admin.
2. Send → user turn appears, `sendChat` called with `{message, history}`; on resolve the
   assistant turn renders the reply (via `fb-markdown`) + tool chips mapped to labels.
3. `tools_used` with `is_error:true` → chip carries the failed affix; args/results never
   appear in the DOM.
4. `truncated:true` → incomplete note rendered.
5. 503 → `markAiUnconfigured()` called + gated panel with owner link (assert via
   `clearCapabilities()` reset between tests, like `command-pages.test.ts:44`).
6. 403 `CAPABILITY_DISABLED` → disabled copy, **no** configure link.
7. History cap: after 12 turns, the `history` arg to `sendChat` has ≤10 turns.
8. Multi-turn: second send includes the first exchange in `history`.
- Live axe sweep on the mounted page in both empty and conversation states.

---

## CHUNK 2 — Actionable output (task #23)

Two independent sub-surfaces. **2a is web-only and fully buildable today. 2b needs a
small backend change and one escalation.**

### 2a — Actionable feed cards (WEB-ONLY, ships today)

**What's true (do not "fix" what works):** both `/feed/{id}/action` and
`/feed/{id}/dismiss` already hit real endpoints (`fb-feed-list.ts:103-127`). The gap is
**semantic**: "Review impact" optimistically removes the card and POSTs `/action`, which
the server stub (`service/feed.go:30-32`) only flips status→`actioned` + audits — **no
side effect, no detail, no navigation.** So "Review" is just a second "Dismiss." The
routing data and detail already exist end-to-end and are dropped:
- `card.project_id` (`models.ts:515`) is set by both producers
  (`agentic.go:319`, `foresight.go:435-443`) — never read by `fb-feed-list`.
- `card.actions[i].payload` carries `{module|risk_type, severity, recommended_action}`
  (`agentic.go:288-292`, `foresight.go:357-368`) — enough to render an impact panel with
  **zero new backend calls**.
- `card.card_type` values in the wild: `delay_cascade`, `schedule_slip`, `budget_burn`,
  `procurement_criticality`.
- `action_type` values: `review_cascade_impact`, `review_foresight_risk`.

**Web-only redesign (`fb-feed-list.ts` + small `fb-feed-card.ts` add):**
1. **Distinguish review from dismiss.** For a *review* action (`action_type` starting
   `review_`), do **NOT** optimistically remove and do **NOT** auto-POST `/action`.
   Instead open an `fb-modal` (the schedule page already imports it; add to feed list)
   showing: `card.title`, `card.body` (via `fb-markdown` if it carries markdown),
   `payload.recommended_action`, `payload.severity`, and a **"Go to project"** CTA.
2. **Deep-link.** Map `card_type` → route, using `card.project_id`:
   - `schedule_slip` / `delay_cascade` → `navigate('/command/schedule')` (page leads with
     a project picker; pass project hint where the page accepts it — see note below).
   - `budget_burn` → `navigate('/command/financials')`.
   - `procurement_criticality` → `navigate('/command/procurement')`.
   The detail modal's CTA calls `navigate(...)`.
3. **Only acknowledge after the user acts.** When the user clicks "Mark handled" in the
   modal (or after navigating), THEN POST `/feed/{id}/action` (terminal, irreversible)
   and remove the card. The **X** (dismiss) stays the "not relevant" path (already correct
   — `onDismiss`, `:119-127`). Treat action vs dismiss as distinct outcomes.
4. **Briefing markdown:** swap `fb-briefing-page` `renderReply` → `<fb-markdown>` (Chunk 4)
   so the literal `**` is gone.

> **Project-hint note:** `/command/schedule` currently takes no path/query param (it
> picks the first project). Surfacing `?project=<id>` is a small optional enhancement to
> `fb-schedule-page.loadProjects()` (preselect the matching project if the query param is
> present). If skipped for v1, the deep-link still routes to the right module — flag it
> as a follow-up, not a blocker.

**RBAC:** feed action + dismiss are any-authenticated (`router.go:461-465`). No gate
change.

**a11y:** priority already pairs color + icon + label in `fb-feed-card`
(`PRIORITY_META`, never color alone) — keep it. The new detail modal needs a heading +
labelled close + focus trap (fb-modal provides). The "Go to project" CTA and "Mark
handled" buttons need discernible names.

**Test plan (vitest, extend `command-pages.test.ts` or new `feed-actions.test.ts`):**
- Clicking a `review_*` action opens the modal and does **NOT** call `actionFeedCard`
  and does **NOT** remove the card.
- The modal shows `recommended_action` from the payload.
- "Go to project" calls `navigate` with the mapped route per `card_type`.
- "Mark handled" calls `actionFeedCard({action_type, payload})` then removes the card.
- The X still calls `dismissFeedCard` and removes (unchanged).
- A card with no `project_id` renders the detail but hides "Go to project".
- Live axe on the feed list + open modal.

### 2b — Per-suggestion apply/reject for schedule adjustments (NEEDS BACKEND — see ESCALATION)

**What's true:** `recommend-adjustments` is **NOT advisory** — it auto-applies every
non-nil `new_duration_days` via `UpdateTask` inside the tx and synchronously re-runs CPM
(`agents.go:388-453`) **before the modal opens**. "Applied"=already-written; "Advisory"=
the model omitted a number. There is **no per-suggestion accept/reject** today; the batch
is all-or-nothing.

**Wire-shape bug (must fix before any per-row UI is truthful):** the Go response is
`Adjustments []ai.ScheduleAdjustment`, and `ai.ScheduleAdjustment` (`internal/ai/tasks.go:401-405`)
has **only** `{task_id, new_duration_days, rationale}`. But `models.ts:453-462` declares
`wbs_code`, `name`, `old_duration_days`, and `applied` — **none on the wire.** So the
modal renders `undefined · undefined` + always-"Advisory" (`fb-schedule-page.ts:346-356`).
**Web cannot fix this; the data is not in the payload.**

This requires a backend change and a product decision → **see §ESCALATION ESC-AUX-01.**
The recommended resolution (pending owner sign-off, but low-risk and contract-additive):

- **[Backend, ~15 lines, no migration] Enrich the response.** Build an enriched DTO in
  `service.ScheduleAdjustmentSet` so each adjustment carries `wbs_code`, `name`,
  `old_duration_days`, and a real `applied` bool. The service already has the loaded
  `tasks` slice (`agents.go:331/347-358`) to look up WBS/name/old-duration and already
  knows applied-vs-skipped in the loop (`agents.go:388-412`). The TS type already assumes
  these fields. This makes the modal an honest **outcome ledger** ("AI auto-tuned N task
  durations and recomputed the critical path; M monitor-only; critical path
  {recomputed|unchanged}") instead of fake advice.
- **[Backend] `?dry_run=true` (or a sibling preview handler) for TRUE accept/reject.** Run
  steps 1-2 (load + AI `update_schedule`) and return adjustments with old/new durations
  but **skip** the `UpdateTask` writes + recalc. The UI then renders accept/reject
  checkboxes and applies accepted rows via the **EXISTING** `PUT /tasks/{taskID}
  {duration_days}` (`router.go:363-364`, min-superintendent), then calls
  `POST /schedule/recalculate` once. This makes per-suggestion apply/reject fully
  buildable on top of endpoints that already exist, with only a dry-run flag added
  server-side.

**Web work (after backend lands):**
- Honest header: "N durations changed · M monitor-only · critical path {recomputed|unchanged}".
- Each applied row: `WBS  Name   Xd → Yd` (delta in `--fb-font-mono`) + green Applied
  badge + rationale. Each monitor-only row: neutral badge + rationale. (Fixes the
  always-"Advisory"/`undefined` rendering once the wire carries the fields.)
- If dry-run ships: per-row accept/reject checkboxes; "Apply selected" → loop `updateTask`
  → `recalculateSchedule` once → `loadGantt`.

**Web-only fallback if 2b backend slips:** relabel/restructure the modal as a post-hoc
"what the AI just changed" report driven by `applied_deltas`/`skipped_rationale_only`
counts only (the per-row identity stays unavailable until the enrich lands — do NOT
fabricate it). Optionally offer a single "Undo all" that re-PUTs captured old durations
(only possible once `old_duration_days` is on the wire).

**RBAC:** recommend-adjustments + `PUT /tasks` are min-superintendent; gate any new
apply/reject UI behind `hasMinRole('superintendent')` (already done,
`fb-schedule-page.ts:414`). **Composite-currency:** no money in this surface; if any
`*_cents` field ever appears (`ScheduleAdjustmentSet.cost_cents`, `models.ts:474`), keep
it integer-cents + `currency_code`, never reformat as float.

**Backend test plan (Go, 2b):** enriched DTO round-trip (wbs/name/old populated, `applied`
matches the apply loop); dry-run path makes **zero** `UpdateTask` calls and does **not**
recalc; both gated min-superintendent; audit row still written on the real (non-dry-run)
path. **Web test plan:** modal renders real per-row identity from a fixture with the new
fields; dry-run accept/reject calls `updateTask` per accepted row + one
`recalculateSchedule`.

---

## CHUNK 3 — Gantt as a real planning surface (task #24)

`fb-gantt-chart` draws WBS codes only, has no date ticks, no dependency arrows, and no
click-to-inspect. Names + all dates are already in `GetGantt`; **only dependency edges
require a backend add.**

### Phase A — backend: return dependencies from GetGantt (the ONLY API change here)
The `GanttView` (`service/schedule.go:231-235`) is `{Tasks, CriticalPath, ProjectEnd}` —
no deps. The store HAS the loader (`store.GetProjectDependencies`, used by the recalc
path at `service/agents.go:338`); the Gantt read path just never calls it.

1. Add `Dependencies []models.TaskDependency \`json:"dependencies"\`` to `GanttView`.
2. In `GetGantt` (`:239-258`), inside the existing read-only tx, call
   `s.scheduleStore.GetProjectDependencies(ctx, tx, projectID)` and set
   `view.Dependencies`. **No new store query, no migration.**
3. Update `API_CONTRACT.md:217` response to
   `{ tasks, critical_path, project_end, dependencies }`.
4. Web `models.ts`: add `TaskDependency` (mirrors `internal/models/project.go:59-66`:
   `id, project_id, predecessor_id, successor_id, dependency_type, lag_days`) and
   `dependencies?: TaskDependency[]` on `GanttView` (`models.ts:414-420`). Pass through
   `fb-schedule-page.ts` to the chart as a `.dependencies` property.

> `dependency_type` is `'FS'|'SS'|'FF'|'SF'`. v1 arrows render FS chains; other types may
> render as FS for now (flag SS/FF/SF arrow geometry as a follow-up).

### Phase B — web: render planning affordances in `fb-gantt-chart.ts`
All of B1, B2, B5 are web-only on existing data; B3 needs Phase A.
1. **Task name labels (B1, web-only):** add a fixed-width left label gutter (~220px) at
   x=0: `wbs_code` as a small mono prefix (`--fb-font-mono`) + `name` in body sans
   (`--fb-text-secondary`), truncated with `<title>`. Shift bar `origin`/`PAD` right by
   the gutter so bars start after labels. Keep the a11y table unchanged.
2. **Date axis (B2, web-only):** replace the bare axis rule (`:245`) with dated ticks.
   Pick a tick interval from `maxDay` (daily if small, weekly otherwise); per tick draw a
   faint gridline (`--fb-border` low opacity) + `<text class="axis-tick">` formatted from
   `origin + day*DAY_MS` (e.g. "Jun 11") in **`--fb-font-mono`**. Keep `today`/`end`
   markers. **Determinism:** keep using the server's RFC3339 fields verbatim via the
   existing `dayOf`/`origin` math — never reconstruct dates from `duration_days`.
3. **Dependency arrows (B3, needs Phase A):** index placed tasks by `id`; per dependency
   draw an orthogonal elbow `<path>` from predecessor finish-edge → successor start-edge
   (FS) with an arrowhead `<marker>`. Critical-chain links (both endpoints `is_critical`)
   in `--fb-gable-green`; others muted `--fb-border`/`--fb-blueprint-blue`. Honor
   `lag_days` (gap before the arrow lands). Add a legend entry. Skip arrows whose endpoint
   is hidden by the "Critical path only" filter.
4. **Critical-path styling + slip pulse — KEEP AS IS.** Do not touch `.bar.critical/.normal`,
   `.tail.near`, or the `slipped` pulse (`:50-107`); they implement spec and are covered
   by the `slippedIds` diff (`fb-schedule-page.ts:228-250`). Respect
   `prefers-reduced-motion` (already handled) for any new animated arrows.
5. **Click-to-inspect (B5):** the SVG is `aria-hidden="true"` (`:243`) for the
   dual-representation contract (DSC §7.8) — the parallel table is the canonical AT
   surface. **Keep the SVG decorative.** Make the visible **table rows** keyboard-focusable
   (`tabindex="0"`, `role="button"`, `@click`/`@keydown` Enter/Space) AND let mouse users
   click bars (pointer-only), both dispatching a `task-select` CustomEvent with the task
   id. In `fb-schedule-page.ts`, handle `task-select` by opening an `fb-modal`/drawer
   (already imported, `:11`) showing name, wbs, ES/EF/LS/LF (mono), float (mono), critical
   badge, status, percent_complete, assigned_crew — all on `ProjectTask` (`models.ts:178-198`).

### RBAC
Gantt read is any-authenticated; recalc/AI adjust min-superintendent (unchanged). The
detail drawer is read-only — any authenticated role.

### Test plan
- **Go (`internal/service`):** integration test — `GetGantt` now returns the project's
  dependencies (round-trip a fixture with ≥1 FS edge); cross-org still `ErrNotFound`.
- **vitest (`web/tests`, extend command-pages or new `gantt.test.ts`):** mount
  `fb-gantt-chart` with tasks+deps → assert task **names** appear in the SVG (not just the
  a11y table), axis tick `<text>` present, a dependency `<path>` rendered for the edge,
  critical styling unchanged, `task-select` event fires on row activation, detail drawer
  opens with the task's dates. Live axe on the chart + open drawer (SVG stays
  `aria-hidden`, table remains the AT surface).

---

## SHARED: tokens, currency, fonts, live-spec ordering

- **Design tokens only** (`web/src/styles/variables.css`, pierce shadow DOM): surfaces
  `--fb-surface-1/2`; text `--fb-text-primary/secondary/muted`; brand `--fb-gable-green`,
  `--fb-blueprint-blue`; alerts `--fb-safety-red`/`--fb-safety-red-text`,
  `--fb-amber-warning`; structure `--fb-border`; spacing/radius/text scale tokens. No
  hardcoded hex except as the existing `var(--token, #fallback)` belt-and-suspenders.
- **JetBrains Mono (`--fb-font-mono`)** for all numerics/dates: chat has none; schedule
  deltas (`Xd → Yd`), Gantt axis ticks + ES/EF/LS/LF + float in the detail drawer.
- **Composite-currency:** no float money anywhere; integer-cents + `currency_code`, group
  by currency. Only `ScheduleAdjustmentSet.cost_cents` could appear — keep it a string +
  `currency_code`.
- **No new dependencies** (TECH_STACK authoritative): markdown is hand-rolled (Chunk 4);
  Gantt SVG stays hand-rolled (no charting lib).
- **Live-spec ordering:** before merge, sweep `HANDOFF.md`, `.agents/handoff/NEXT_STEPS.md`,
  `CLAUDE.md`, this file, and `.agents/handoff/API_CONTRACT.md` (Gantt `dependencies` +
  enriched `ScheduleAdjustmentSet`) so the docs match the merged code (per MEMORY:
  "sync all docs before merge").

---

## VERIFICATION — re-run browser E2E shows each surface capable/actionable

Use the live Playwright config (`web/playwright.live.config.ts`) + `@axe-core/playwright`
against staging after deploy (task #26). Per surface, the observable proof:

1. **Assistant (Chunk 1):** `/command/assistant` shows a chat composer + starter prompts
   (not two link cards). Typing "What needs my attention today?" + Enter shows a user
   bubble, a "thinking…" status, then a rendered assistant reply (markdown, no literal
   `**`) with **Sources used** chips (e.g. "Schedule", "Feed"). A second message keeps the
   thread (multi-turn). With AI off (or a forced 503), the page shows the gated panel +
   owner Integrations link; with the admin kill-switch, the "turned off by an admin" copy
   and **no** key link. Network tab shows `POST /agents/chat` with `{message, history}` and
   no identity field. Axe: 0 serious/critical.
2. **Feed (Chunk 2a):** "Review impact" opens a detail modal (title/body/recommended
   action) and a "Go to project" button that routes to the right module — it no longer
   silently disappears. The card only leaves the list after "Mark handled" (POST
   `/action`) or the X (POST `/dismiss`). Briefing hero shows formatted bold/lists, no
   literal `**`.
3. **Schedule adjustments (Chunk 2b, after backend):** the drawer reads as an outcome
   ledger — real `WBS · Name` and `Xd → Yd` per row (no `undefined`), an honest header,
   and (if dry-run shipped) per-row accept/reject that writes via `PUT /tasks` +
   `/recalculate`.
4. **Gantt (Chunk 3):** bars show task **names** + a dated axis; FS **dependency arrows**
   connect bars (critical chain in Gable Green); clicking a bar/row opens a detail drawer
   with ES/EF/LS/LF + float (mono) + critical badge. Network shows `schedule/gantt`
   returning a `dependencies` array.

Re-run the full audit checklist; each "does nothing / wall of text / WBS-only / no chat"
finding must now be visibly resolved. Run `npm run typecheck && npm run test &&
npm run lint` in `web/`, plus `make test`/`make test-integration` for the Go bits, before
the verification loop.

---

## ESCALATION — ESC-AUX-01 (Chunk 2b backend)

**File this to `.agents/handoff/ESCALATION_LOG.md` and pause Chunk 2b for owner sign-off**
(Chunks 1, 2a, 3, 4 are unblocked and proceed in parallel).

- **What's blocked:** truthful per-suggestion schedule apply/reject. Two coupled gaps:
  - **[G1] `ScheduleAdjustmentSet` is under-specified on the wire.** Response carries only
    `{task_id, new_duration_days, rationale}` (`internal/ai/tasks.go:401-405`), but the TS
    type + the existing modal assume `wbs_code, name, old_duration_days, applied`
    (`models.ts:453-462`, `fb-schedule-page.ts:346-356`) → renders `undefined · undefined`
    + always-"Advisory". Web-only cannot fix; the data isn't sent.
  - **[G2] No preview/dry-run mode.** `RecommendScheduleAdjustments` is all-or-nothing
    auto-apply + recalc — there is no way to *propose* changes for the user to accept/reject.
- **Why it's an escalation, not an improvisation:** enriching `ScheduleAdjustmentSet` and
  adding a `?dry_run` flag are **API-contract changes** — `CLAUDE.md` / the dual-agent
  protocol says routes/schemas must match `API_CONTRACT.md` exactly and contract changes
  get flagged, not improvised.
- **Recommended resolution (low-risk, contract-additive):** (a) enrich the DTO using the
  already-loaded `tasks` slice (~15 lines, no migration); (b) add `?dry_run=true` that runs
  load+AI and returns adjustments with old/new durations but skips writes+recalc, so the UI
  applies accepted rows via the existing `PUT /tasks` + `/recalculate`. Both stay
  min-superintendent; the real path keeps its `schedule.maestro_edit` audit row.
- **Also note [G3]:** feed `/action` dispatch is a documented stub (`service/feed.go:30-32`)
  — fine for Chunk 2a (review is a client concern), but any FUTURE "apply"-type feed action
  needs a real server-side dispatcher.
