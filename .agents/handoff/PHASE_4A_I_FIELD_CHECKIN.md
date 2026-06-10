# Phase 4a-i — standalone field Check-in screen + the offline affordance pattern

**Status:** BUILT on `feat/phase-4a-i-field-checkin` (committed, not pushed/merged). **Mobile-only — ZERO backend change** (the `/api/v1/field/checkin` endpoint + `queueCheckin` already existed). All mobile gates green. Awaiting owner review → merge.
Plan: [PHASES_2-4_ULTRALOOP_PLAN.md](./PHASES_2-4_ULTRALOOP_PLAN.md) §"Phase 4 · 4a". Binding UX: [frontend/UX_CORE_SCREENS.md](./frontend/UX_CORE_SCREENS.md) §0.3, [frontend/DESIGN_SYSTEM_COMPONENTS.md](./frontend/DESIGN_SYSTEM_COMPONENTS.md) §8/§8.1/§9.

## Why
Crew check-in shipped as a **crew-less stub** auto-fired inside the Daily Log submit (`queueCheckin(projectId, gps)` with an empty crew list). 4a-i extracts it into a real, standalone screen — actual crew capture (name + role), GPS, notes — and establishes the **field offline affordance** (the amber dashed border the design system mandates on queued actions, which didn't exist as a widget yet).

## What changed (mobile/)
- **`lib/screens/check_in_screen.dart` (new):** crew roster (add/remove rows of name + role), best-effort GPS (geolocator, refreshable), optional notes (≤4096), a project selector when >1 project. Submit builds `crew_members = [{name, role?}]` (rows with a non-empty name) and calls `syncServiceProvider.queueCheckin(...)` → outbox → snackbar → `maybePop`. `crew_members` is stored opaquely server-side (`json.RawMessage`/JSONB), so free-text `{name, role}` is the no-backend-change shape (the API_CONTRACT §9 `{worker_id,…}` example is illustrative, not enforced — the field app has no `worker_id`-enrolled crew).
- **`lib/widgets/fb_dashed_border.dart` (new):** reusable dashed rounded-border (CustomPainter), amber by default — **the offline affordance pattern**. The dash walk is guarded (constructor assert + release-safe runtime guard) so it can never spin. Wraps the submit CTA when offline, paired with a `cloud_off` icon + caption (in a `Semantics(liveRegion)`) + the `FbSyncChip` — status is never colour-alone.
- **`lib/screens/more_screen.dart`:** a "Crew check-in" tile (first entry) pushes the screen.
- **`lib/screens/daily_log_screen.dart`:** removed the crew-less check-in stub + the now-orphaned GPS capture/indicator/import — Daily Log is now purely summary + weather + safety + photos.
- **i18n:** 12 new keys in BOTH `app_en.arb` + `app_es.arb` (regenerated `app_localizations*.dart`); key sets verified identical.
- **Touch targets (field rule ≥56px, glove-friendly):** the remove `IconButton` is a 56px target spaced off the role field; the add-crew + update-location `TextButton`s carry a 56px min-height (the theme floors filled/outlined buttons but not text buttons — see follow-up).

## Tests (`test/`)
- **`check_in_screen_test.dart`** — 9 widget tests: roster add/remove + remove-disabled-on-last-row; submit queues the crew payload + null notes/GPS; notes ride along (trimmed); a role rides along; **multi-project dropdown selects the submitted project**; **successful submit pops back** (mounted on a real navigator); empty-crew shows a hint + no queue; offline shows the dashed affordance + caption; online hides it.
- **`check_in_screen_golden_test.dart`** + `test/goldens/check_in_online.png`, `check_in_offline_queued.png` — RUN_GOLDEN_TESTS-guarded (self-skips in default CI), masters committed.

## Max-effort code review (`/code-review max`, 9 finder angles + verify + sweep)
Correctness came back largely clean (the removed-behavior + cross-file finders found nothing). **7 findings fixed:** (1) **`_projectId` never reconciled** against the live project set — a background sync (HomeShell stays mounted under this pushed route and auto-syncs on reconnect/push) could leave a stale selection → a check-in queued against the wrong project (silent) or a debug-only dropdown assert; replaced the one-time `??=` with a `contains`-reconcile + an empty-id guard, **and applied the same fix to `daily_log_screen` (same latent bug)**. (2) **Notes silent data loss** — `maxLength: 4096` counted chars but the backend caps 4096 **bytes** (Go `len()`), so a long accented/emoji note passed the client then 400'd + parked silently; now a utf8-byte validator blocks it client-side (and removes the unwanted `0/4096` counter chrome). (3) **`FbSyncChip` advertised a dead a11y button** (no `onTap`) → now opens Sync Status (matches the spec). (4) **same-frame double-submit** window → synchronous `_submitting` guard. (5) **`FbDashedBorder` negative-size** guard for a sub-stroke child. (6) **touch-target bandaid → root-cause fix**: a global `textButtonTheme` 56px floor in `buildFieldTheme()` (zero blast radius — `check_in` is the only screen using TextButton), removing the two per-call overrides. (7) a notes-byte regression test. **Deferred (noted follow-ups):** a shared `LocationService` + an `FbProjectSelector` widget (GPS + project-dropdown logic duplicated with daily_log), an `OfflineSubmitButton` composite (so daily_log/tasks get the affordance too), and the `worker_id`-vs-`{name,role}` API_CONTRACT doc note.

## Verification (3-lens adversarial pass)
Dart/Flutter correctness: **sound** (controller disposal exactly-once incl. removed rows, mounted-after-await everywhere, payload shape correct, extraction clean — no orphaned refs, verified vs the pre-extraction file). Spec/UX: offline affordance **complete** (dashed + icon + caption + chip), bilingual exact, AAA contrast, opaque-crew shape acceptable. Test coverage: harness conventions good. **Findings fixed:** (1) the dashed painter could infinite-loop if `dashLength+gapLength≤0` → guarded; (2) secondary touch targets sub-56px (remove IconButton, the two TextButtons) → fixed; (3) `location_off` icon used the muted slate token (sub-contrast) → textSecondary; (4) offline state had no screen-reader announcement → `liveRegion`; (5) the 5 highest-value missing tests (maybePop nav, multi-project dropdown, notes, null-GPS, disabled-remove) → added.

## Surfaced follow-ups
- **Global `textButtonTheme` floor** in `buildFieldTheme()` (so every field TextButton gets the 56px floor at the root, not per-call) — deferred to keep this chunk's blast radius zero; do it as a small theme chunk and re-verify the goldens.
- **Live-region a11y is a codebase-wide gap** — `FbSyncChip` + the other screens lack `liveRegion`; this chunk establishes the pattern, a sweep is a follow-up.
- A geolocator-mock test to cover the populated-GPS branch (today only the null-GPS path runs in tests).
- A note to the API_CONTRACT owner that the shipped crew shape is `{name, role}` (free-text), diverging from the doc's `{worker_id,…}` example (doc drift, not a code defect — the column is opaque).

## Gates (mobile)
`cd mobile && dart format` (clean) · `flutter analyze` "No issues found!" · `flutter test` **+60 passed, ~8 golden-skipped** · goldens regenerated + verified under `RUN_GOLDEN_TESTS=1`. No Go/web touched (mobile-only) → no `make audit` needed.

## Definition of done
- [x] Spec (this file) · [x] feature branch; mobile gates green; adversarial verification triaged + fixed · [ ] `/code-review` (owner) · [x] capability demonstrable (a standalone crew check-in that queues offline with the dashed affordance) · [x] HANDOFF/NEXT_STEPS updated. **Remaining 4a: 4a-ii equipment (CROSS-STACK — needs a field-scoped read endpoint + a product call on field fleet visibility).**
