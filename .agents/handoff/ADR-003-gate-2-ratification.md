# ADR-003: Gate 2 Ratification — LocalBlue auto-flow + partial A2A schema lock

**Status:** ACCEPTED — owner-confirmed 2026-05-06.
**Date:** 2026-05-06
**Author:** Claude (codifying owner direction from Gate 2 review session)
**Closes:** Gate 2 owner-approval request raised in [HANDOFF.md](../../HANDOFF.md) on 2026-05-05.
**Status changes elsewhere:**
- [ADR-001 D7](./ADR-001-vision-alignment.md#d7--a2a-receiver-scope-for-sprint-5) PROPOSED → **ACCEPTED with payload-alignment caveat**
- [ADR-001 D14](./ADR-001-vision-alignment.md#d14--localblue-lead--buildos-prospect) PROPOSED → **ACCEPTED**

---

## Decision

Gate 2 had two ratification asks (HANDOFF.md "In flight" 2026-05-05). Both ratified:

1. **Unconditional ratification** — ADR-001 D14: the LocalBlue → Brain → BuildOS A2A auto-flow is the production behavior. A `localblue.lead_captured` event creates a `pre_construction_prospects` row at `stage='LEAD'` with no human-in-the-loop. Implementation shipped in PRs #22 + #23 (2026-05-05).

2. **Conditional ratification (Option A)** — the inbound A2A wire contract is locked at the *envelope* and *event-type list* level. Per-event payload schemas are **NOT** locked yet. S4 (Maestro `update_schedule`) may proceed using only the two payload-aligned events (`update_schedule`, `delivery_confirmation`); the four drift events stay off-limits to agent-driven flows until Brain payload-alignment lands.

This is the lower-risk choice over Option B (ratify BuildOS-canonical structs as locked) because Brain has not committed in 30 days and ratifying drift would compound asymmetry.

## Why ratification was needed

S4 Session 9.1 (DailyFocusAgent calls Maestro `update_schedule`) round-trips the Maestro response back through A2A. The owner did not want the agent layer compounding on un-ratified envelope shapes. Locking the envelope + event list lets the agent work proceed; deferring per-payload alignment isolates each drift to its own small PR.

## What is locked at this gate

### Envelope shape (BuildOS-canonical; Brain must align)

```jsonc
{
  "event_type": "<one of the 6 strings below>",
  "payload":    { /* per-event */ },
  "trace_id":   "<string>",
  "idempotency_key": "<uuid>",
  "timestamp":  "<RFC3339>",
  "iss":        "<issuer string; legacy literal 'fb-brain' subject to D4 cutover>",
  "org_id":     "<uuid>"
}
```

References:
- BuildOS `WebhookEnvelope` — [`internal/service/a2a.go:35-43`](../../internal/service/a2a.go)
- Brain `WebhookEvent` — `brain/internal/a2a/types.go:22-29`

`org_id` becomes **required** in the locked contract. Brain must populate it on every emitted event. Until Brain ships its emit-side change, BuildOS continues to fall back to `A2AService.defaultOrgID` for single-tenant fork mode (forward-compatible — when Brain starts emitting `org_id`, BuildOS preferentially uses that).

### Event-type identifiers (locked)

The list of 6 strings is final. New event types require an ADR addendum.

| Wire string | Direction | Payload status |
|---|---|---|
| `review_material_quote` | Brain → BuildOS | locked; payload pending alignment |
| `review_labor_bid` | Brain → BuildOS | locked; payload pending alignment |
| `update_schedule` | Brain → BuildOS | locked; **payload aligned** |
| `delivery_confirmation` | Brain → BuildOS | locked; **payload aligned** |
| `create_feed_card` | Brain → BuildOS | locked; payload pending alignment |
| `localblue.lead_captured` | Brain → BuildOS | locked; payload BuildOS-canonical, Brain to re-derive |

## What is NOT locked

Per-event payload schemas. Concrete drift evidence as of this ratification:

### 1. `review_material_quote` — BuildOS drops `LineItems`

| Field | Brain `ReviewMaterialQuotePayload` | BuildOS `reviewMaterialQuotePayload` |
|---|---|---|
| RFQID | ✓ | ✓ |
| **LineItems** | `[]MaterialQuoteLineItem` | **missing** |
| TotalCents | ✓ | ✓ |
| CurrencyCode | ✓ | ✓ |
| Vendor | ✓ | ✓ |

Resolution: BuildOS adds `LineItems` to its decoder; receiver tests gain coverage; ratify in a follow-on PR.

### 2. `review_labor_bid` — BuildOS drops `AIAnalysis`

| Field | Brain `ReviewLaborBidPayload` | BuildOS `reviewLaborBidPayload` |
|---|---|---|
| RFQID | ✓ | ✓ |
| Bidder | ✓ | ✓ |
| AmountCents | ✓ | ✓ |
| CurrencyCode | ✓ | ✓ |
| Timeline | ✓ | ✓ |
| **AIAnalysis** | `string` | **missing** |

Resolution: BuildOS adds `AIAnalysis` to its decoder + feed-card body; ratify in a follow-on PR.

### 3. `create_feed_card` — asymmetric expectations

| Field | Brain `CreateFeedCardPayload` | BuildOS `createFeedCardPayload` |
|---|---|---|
| CardType | ✓ | ✓ |
| Title | ✓ | ✓ |
| Body | ✓ | ✓ |
| Actions | `[]FeedCardAction` (typed) | `json.RawMessage` (lax) |
| Priority | ✓ | ✓ |
| **target_role** | **not emitted** | expected (defaults to `"owner"` if absent) |

Resolution open question: either Brain adds `target_role` to its emit-side payload (recommended; preserves operator-targeting semantics that BuildOS already uses) OR BuildOS removes the expectation. Defer to Brain-side decision.

### 4. `localblue.lead_captured` — Brain has no type definition

Per [HANDOFF.md:121](../../HANDOFF.md), Brain-side type definitions were deleted 2026-05-04 (orphan branch never merged). When Brain emitter wiring resumes, Brain re-derives `LocalblueLeadCapturedPayload` from BuildOS's `localblueLeadCapturedPayload` ([`internal/service/a2a.go:360-371`](../../internal/service/a2a.go)) as the canonical reference. Six receiver-side atomicity tests in PR #23 (`a2a_integration_test.go`) lock the BuildOS-side decoder.

**The four drift items above MUST resolve before any agent-driven flow consumes those events.** Receiver-side tests in PRs #22 + #23 already pin the BuildOS-side decoder shape, so the alignment window is safe — Brain's emit changes won't silently regress BuildOS handling.

## What this unblocks

### S4 Session 9.1 — DailyFocusAgent calls Maestro `update_schedule` (PROCEEDS WITH CONSTRAINTS)

`internal/service/agents.go` adds `RecommendScheduleAdjustments(ctx, projectID)` calling `brain.Maestro.UpdateSchedule` with the current task graph. Recommended deltas apply through existing `ScheduleService.RecalculateSchedule` so CPM physics re-validates. Audit row per Maestro-driven edit: `Action="schedule.maestro_edit"`, `ResourceType=AuditResourceSchedule`, metadata `{run_id, tokens_used, cost_cents, currency_code, recommended_delta_count}`.

**Constraint:** the agent is permitted to consume `update_schedule` and `delivery_confirmation` only. Any agent that needs `review_material_quote` / `review_labor_bid` / `create_feed_card` payloads (e.g., S6 SubLiaisonAgent → labor bid review loop) **must wait** for the matching payload-alignment ratification.

### Cross-repo Brain backlog (created by this ratification)

These items belong to Brain's next session. Tracked in [HANDOFF.md cross-repo coordination table](../../HANDOFF.md#cross-repo-coordination-buildos--the-brain).

| # | Severity | Item |
|---|---|---|
| 1 | P0 | Add `OrgID` field to `WebhookEvent` envelope in `brain/internal/a2a/types.go`. Populate on every emitted event from the source org context. |
| 2 | P0 | Re-add `EventLocalblueLeadCaptured` constant + `LocalblueLeadCapturedPayload` type in `brain/internal/a2a/types.go`, mirroring BuildOS canonical (`internal/service/a2a.go:360-371`). |
| 3 | P1 | Decide `create_feed_card` `target_role` resolution. Recommended: Brain adds `target_role` (`string`, optional) to `CreateFeedCardPayload`. |
| 4 | P2 | Coordinate `Issuer` field cutover with ADR-001 D4 (current literal `"fb-brain"` deprecates to URI form via dual-emit window). |

Once items 1–3 are addressed Brain-side, BuildOS lands the receiver-side `LineItems` / `AIAnalysis` / `target_role` decoder additions in a single follow-on PR. Item 4 is its own coordinated cutover (ADR-001 D4 already specifies the dual-aud + dual-iss window).

## Not in scope of this ratification

- **Outbound A2A schemas (BuildOS → Brain).** Different code path; outbound emitter package shipped in PRs #19 + #20 with its own typed payloads (`EmitReviewMaterialQuote`, `EmitReviewLaborBid`).
- **Maestro task envelopes (D5).** Ratified at Gate 1 (2026-05-05).
- **Hub credential proxy (D6).** Ratified at Gate 1 (2026-05-05).
- **Wire-protocol legacy values (D4).** `iss="fb-brain"` / `aud="fb-os"` cutover plan exists in ADR-001 D4 but hasn't started. Listed as item 4 above for coordination but not part of Gate 2 itself.

## Reversibility

**Low cost.** Each drift item resolves in its own small PR. If owner ratifies a payload schema differently than this ADR anticipates (e.g., decides BuildOS should drop `AIAnalysis` rather than Brain emit it), update the relevant Brain-backlog item with the new decision. Annotate inline; no data migration implications.

The locked envelope + event-list portion is irreversibly committed by both repos' shipped code. Reverting that would require active rework and is not on the table.

## Implements

1. ADR-001 D7 inline status header: PROPOSED → ACCEPTED with payload-alignment caveat referencing this ADR.
2. ADR-001 D14 inline status header: PROPOSED → ACCEPTED.
3. HANDOFF.md "Last shipped" gains a 2026-05-06 entry.
4. HANDOFF.md "In flight" — Gate 2 block removed; replaced with ratification note pointing to this ADR.
5. HANDOFF.md "Next up" — S4 Session 9.1 unblocked with the two-event constraint surfaced.
6. HANDOFF.md "Cross-repo coordination" table gains four rows for the Brain-side TODOs (1–4 above).

No production code changes. Receiver-side decoder updates (LineItems, AIAnalysis) land in a follow-on PR after Brain emits the matching payload changes.
