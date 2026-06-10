# Phase 4a-ii — read-only "equipment on my projects" (field)

**Status:** BUILT on `feat/phase-4a-ii-field-equipment` (committed, not pushed/merged). CROSS-STACK (Go + Flutter). All gates green. Awaiting owner review → merge.
**Gated decision resolved:** owner chose the read-only option (Option B) — see [ESCALATION_LOG.md](./ESCALATION_LOG.md) ESC-003. This file is the **working spec** (the screen was unspecified by any binding doc; Antigravity owes the spec backfill listed in ESC-003).
Planned by the 4a-ii ultraplan (8-agent ground → design → critique → plan).

## Why
Crew check-in shipped (4a-i); equipment was the last 4a gap. A field worker could not see which excavator/crane is on their site without asking a superintendent. This adds a read-only equipment view, scoped to the worker's own active sites, riding the existing field-sync seam — **no new endpoint, no new RBAC gate, no migration.**

## What changed (Go backend)
- **`internal/models/field.go`:** new field-safe `FieldEquipment` DTO (id, name, asset_type, serial_number?, status, allocation start/end) — deliberately **not** `models.FleetAsset` (no `org_id` / future cost column leak). Added `Equipment []FieldEquipment` to `FieldSyncResponse` (with a doc note that it is a FULL-SET collection, not a delta).
- **`internal/store/field.go`:** `ListAllocatedEquipment(ctx, tx, {UserID, OrgID, Today})` — returns assets whose `equipment_allocations` window is active today (`$today ∈ [start, end)`) on a project where the caller has a **non-completed assigned task** (derived `SELECT … project_tasks WHERE $user = ANY(assigned_crew) AND status <> 'completed'`, mirroring `ListAssignedTasks`). Org isolation via the `projects` join on **both** the allocation and the subquery (defense in depth — `equipment_allocations` has no `org_id`). **Full-set: no `Since`/delta** (neither fleet table has `updated_at`; relevance pivots on the window + status). Date bound as an unambiguous `YYYY-MM-DD` string so the window is TZ-independent.
- **`internal/service/field.go`:** `Sync` calls `ListAllocatedEquipment` inside the existing read-only tx (scoped to the looked-up `userID` + `Today: serverTime`) and sets `resp.Equipment`. No new handler/route — rides `GET /api/v1/field/sync` (which `field_worker` already reaches; `/fleet` is `superintendent+`).
- **API handler:** unchanged — `Sync` serializes the whole response, so `equipment` rides along.

## What changed (Flutter mobile)
- **`models/equipment_asset.dart` (new)** + `models/field_sync.dart` parses the `equipment` array.
- **`database/app_database.dart`:** new `CachedEquipment` Drift table; **schemaVersion 1 → 2** with the app's **first-ever `MigrationStrategy`** (`onCreate: createAll`, `onUpgrade` from <2 creates the equipment cache — a server-refilled pull cache, so creating it empty loses no local data). `replaceEquipment` is **FULL-REPLACE (delete-then-fill in a tx)** — unlike `replaceTasks`'s upsert — so equipment that left a site disappears. `allCachedEquipment` reader.
- **`services/sync_service.dart`:** `pull()` calls `replaceEquipment(...)` (full-replace) after `replaceTasks`.
- **`providers/app_providers.dart`:** `equipmentProvider` (FutureProvider over the cache, sorted by name), invalidated in `syncNow`.
- **`screens/equipment_screen.dart` (new):** read-only list — name, type, a status badge (available/maintenance/unavailable, **never colour-only** — dot + colour + localized label), serial (mono), the allocation window; empty + loading + error states; pull-to-refresh → `syncNow`; the `FbSyncChip` (now with `onTap` → Sync Status). Wired into the **More tab** (a "precision_manufacturing" tile). 7 new EN/ES i18n keys.
- **`pubspec.yaml`:** `sqlite3` added to **dev_dependencies** (already a transitive dep of drift — made explicit only so the v1→v2 migration test can build a raw v1 schema). No new runtime dependency.

## Tests
- **Go (integration):** `TestFieldStore_ListAllocatedEquipment` — assignment-scoping (userA sees only their site's asset; userB sees only theirs), org-scoping incl. a **stale cross-org assigned_crew no-leak** case, the active-today window (past + future excluded, end-exclusive), completed-task-site exclusion, and the field-safe projection (id/type/status/window, no org_id). `TestFieldService_Sync` extended to assert `resp.Equipment` end-to-end.
- **Mobile:** `app_database_test.dart` — `replaceEquipment` is full-replace (2→1→0, the delete-then-fill correctness the critique flagged) **and** the **v1→v2 migration** (raw v1 schema via sqlite3 → open at v2 → equipment cache created, pre-existing task preserved). `equipment_screen_test.dart` — renders name/type/status/serial/window, the three status labels, the empty state. `sync_service_test.dart` extended — `pull` full-replaces the equipment cache (drops what left).

## Verification
Built grounded (read the exact `ListAssignedTasks` + `pull`/Drift patterns to mirror) off the ultraplan, whose 2-critic pass had already caught + fixed the **build-breaking delta bug** (created_at delta → permanent staleness; corrected to full-replace) before any code.

**`/code-review max` (9 finder angles + verify + sweep):** the tenant-isolation finder confirmed all invariants hold (org + assignment scoping, field-safe projection, additive JSON). **6 findings fixed:** (1) **date day-shift (HIGH)** — `df.format(date.toLocal())` rolled every allocation date back a day in the Americas (calendar DATEs are midnight-UTC); now formats the UTC value. (2) **exclusive end shown as inclusive** — now renders `end − 1` as the last on-site day. (3) **`DateFormat` ignored the app locale** (ES saw English months) — now passes `Localizations.localeOf(context)`. (4) **`unavailable` slate label sub-AA contrast** → `textSecondary` (≥7:1). (5) **malformed/partial 200 missing the `equipment` key wiped the cache** (full-replace, unlike tasks' upsert) — `pull` now guards on `json.containsKey('equipment')`. (6) **defense-in-depth** — added `AND fa.org_id = $1` (the asset's own org) + a serial-number assertion in the store test + a missing-key regression test. Deferred (noted): extract `FbStatusBadge` / a shared refreshable-empty widget (status-pill + `_Centered` duplicate existing patterns); the 4-site field-safe-DTO duplication; the single-`?since`-cursor coupling.

## Gates
`make audit` ALL PASSED · `make lint-isolation` PASSED · build/vet (default/prod) · field integration tests (store + service) green · `dart format` + `flutter analyze` clean · `flutter test` (+67, ~8 golden-skipped) · NO migration added (Option B is migration-free).

## Definition of done
- [x] Ultraplan + owner decision (ESC-003) · [x] feature branch; Go + mobile gates green · [ ] `/code-review` (owner) · [x] capability demonstrable (a field worker sees equipment on their active sites, offline) · [x] ESCALATION_LOG + HANDOFF/NEXT_STEPS updated. **This closes Phase 4a.** Spec backfill owed to Antigravity (ESC-003).
