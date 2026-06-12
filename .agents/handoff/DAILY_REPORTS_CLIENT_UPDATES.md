# Daily Reports → Client Updates — File-Level Implementation Spec (v2: photos + public link)

**Domain:** Close the field → office → homeowner loop, *with real jobsite photos and a public homeowner progress page*.
**Pipeline stage:** Owner-chosen first operational workflow (post agentic-UX batch).
**Status:** SPEC (revised for two owner decisions) — to be built on a fresh branch in **five ordered chunks (A→E)**, each independently buildable, reviewable, and deployable.
**North star:** [VISION.md](../../VISION.md) — BuildOS as an agentic OS whose harness does the GC's coordination busywork. This domain is the first concrete "harness composes a stakeholder communication" surface, and Chunk E is the **first surface outside the everything-behind-auth model**.
**Author note:** This spec references *behavior and contracts*, not current line numbers (they shift on the post-merge branch). File paths and symbol names are stable; verify exact signatures against the branch before coding.

---

## 0. What changed in v2 (two owner decisions)

Two owner decisions expand the original text-only / email-only v1:

1. **Photos are in scope now.** Jobsite photos are captured by the field app (`mobile/lib/screens/photos_screen.dart`: *"Captures live in session memory for now; wiring uploads to a field asset endpoint is a follow-up"*) but **there is no object store anywhere** — `daily_logs.photo_asset_ids UUID[]` and `task_progress.photo_asset_id` are **dangling UUIDs pointing at nothing**. v2 builds **real object storage** (Chunk A) so daily reports and client updates can **embed photos**.
2. **Public shareable link is in scope now.** A homeowner gets the emailed update **and** an unauthenticated, **token-gated, read-only** progress page (Chunk E). This is the **first route outside the everything-behind-auth invariant** (today the only unauth routes are `MountAuthRoutes` and the SPA catch-all in `internal/api/spa.go`). Its security model is designed below with the bootstrap-token pattern as the template.

The previous escalations **OQ-3 (sharing)** and **OQ-6 (photo storage)** are now **resolved by the owner as BUILD**, not defer. Their security/architecture details move from "escalation" into the chunk specs. Everything else from v1 (derived read model, two AI tasks, deterministic redaction, human-in-the-loop send gate, `client_updates` lifecycle, client contact on projects) **stays** and is folded into Chunks C/D.

---

## 0.1 The loop, in one breath

The field app already captures daily logs (`daily_logs`), crew check-ins (`crew_checkins`), per-task progress (`task_progress`), **and photos (on-device only, not uploaded)**. Today that data is write-only with dangling photo IDs. This domain adds:

1. **Object storage** for photos (`internal/storage` port + R2 adapter + `assets` table) so photo IDs resolve to real blobs.
2. **Operator read** of field captures aggregated per `(project, date)`, **with photo thumbnails** (signed GET URLs).
3. **AI composition** of (a) an internal **office digest** and (b) a redacted, client-safe **homeowner progress update**.
4. **A human-in-the-loop composer** — AI drafts → operator edits → operator previews → operator **sends**. Never auto-send.
5. **Delivery** via the existing Resend mailer **and** a **public token-gated progress page** with short-lived signed photo URLs.

---

## 0.2 Grounding (verified against the branch at spec time — cite file:line on the build branch)

- **No storage abstraction exists.** Grep of `internal/` for s3/blob/storage/upload/r2/aws/presign returns only unrelated hits (cryptobox, integration_credential, bodysize). There is **no** `internal/storage`, no S3 SDK in `go.mod` (grep `aws|s3|minio|smithy` in `go.mod` → empty), and `.agents/TECH_STACK.md` lists **no S3/object-store dependency**.
- **R2 is already the org-wide object store for DB backups.** `.github/workflows/backup-nightly.yml` and `promote-production.yml` upload via `BACKUP_UPLOAD_CMD="aws s3 cp {file} s3://${R2_BUCKET}/... --endpoint-url ${R2_ENDPOINT}"` using GitHub secrets `R2_ENDPOINT / R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY / R2_BUCKET` (Cloudflare R2 is S3-compatible). `scripts/backup-db.sh` keeps the upload step storage-agnostic behind `BACKUP_UPLOAD_CMD`. The **photo store reuses the same R2 account/credential shape** (endpoint + bucket + access key + secret), but resolved **per-fork** (see Chunk A) rather than from CI secrets.
- **The per-fork credential pattern is the encrypted vault.** `migrations/012_integration_credentials.up.sql` + `internal/service/integrations.go` (`VaultService`) seal provider keys (anthropic, resend, gable, localblue) AES-256-GCM under `VAULT_MASTER_KEY` (`internal/cryptobox`), one active row per `(org_id, provider)`, resolved at call time. **`VaultService` already implements `ai.KeyResolver` and `mailer.KeyResolver`** — the exact port-resolver shape the storage adapter copies.
- **Body-size ceiling for uploads is reserved.** `internal/api/middleware/bodysize.go` defines `FileUploadMaxBodyBytes = 25 << 20` (25 MiB) *"reserved for endpoints that accept binary uploads (none today, but the next sprint's proof-of-progress photo upload will land here)."* Direct-to-R2 presigned PUT keeps blobs **off** the Go server, so this cap only guards the small JSON presign-request bodies; it is the fallback cap if a proxy-through-server path is ever added.
- **The token pattern for the public link is the bootstrap token.** `internal/service/setup.go`: 32-byte CSPRNG cleartext, base64url (43 chars), **only the sha256 hash is stored** (`setup_bootstrap_tokens`), `GetActiveBootstrapTokenByHash` filters `expires_at > now`, redemption returns a **uniform** `ErrInvalidBootstrapToken` on any failure (missing/expired/redeemed/mismatch) to avoid leaking probe info, default TTL 7 days. The public-link token copies this verbatim (hash-at-rest, uniform error, TTL) and adds **revoke**.
- **Router placement for an unauth route.** `internal/api/router.go`: the global stack (`RequestID → RealIP → otelhttp → SecurityHeaders → RateLimiter → Sentry → Recoverer`) is mounted at **router root**, *then* `MountAuthRoutes` (unauth), *then* a single `r.Group` applies `authMiddleware` + `SetupGate`. A public route mounted as a **sibling of `MountAuthRoutes` (outside the auth group)** inherits RealIP + rate-limiting + security headers but **bypasses Auth and SetupGate cleanly** — no change to either for the rest of the API. `internal/api/middleware/setup_gate.go` `DefaultSetupGateExemptPrefixes` is `/api/v1/setup, /health, /ready, /metrics`; we do **not** add the public prefix there because it never enters the auth group at all.
- **There is no Cloudflare JS Worker.** Cloudflare in front of staging/prod is **DNS + TLS + HSTS + WAF on a proxied CNAME** (`deploy/railway/README.md`), not a request-rewriting Worker. So the public route is just a normal same-origin Go route behind the proxy — **no Worker code to write**; it works through Cloudflare as-is. (The proxy already forwards `/api/*` and the SPA catch-all; `/share/*` and `/p/*` are the same.)
- **CSP constraint for an embedded public page.** `internal/api/spa.go` sets a strict per-response CSP with `img-src 'self' data:` and `frame-ancestors 'none'`. The global `SecurityHeaders` middleware sets `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `nosniff`. The public progress **HTML page must set its own CSP per-response** (like spa.go does) — either proxy photos **same-origin** (`img-src 'self'`) or extend `img-src` to the R2 public hostname. **Default: proxy photo bytes same-origin** through a signed-URL-backed handler so the page CSP stays `img-src 'self'` and the R2 hostname/bucket never appears in client HTML.
- **Mobile photo capture exists, upload does not.** `mobile/lib/screens/photos_screen.dart` captures with `image_picker` into session memory (`_photos: List<String>` of local file paths). `mobile/lib/services/sync_service.dart` already passes `photo_asset_ids` on the daily-log outbox action — but those IDs are invented client-side and resolve to nothing. Chunk B wires capture → presigned PUT → asset row → real IDs.
- **Audit recorder.** `internal/service/audit.go` `AuditRecorder.Record(ctx, tx, AuditEntry{OrgID, UserSub, Action, ResourceType, ResourceID, Metadata json.RawMessage})` — used by every mutation in one tx. `NoopAuditRecorder` for tests.

---

## 0.3 Hard decisions (defaults picked; flagged for owner confirm in §9)

| # | Decision | Default chosen |
|---|---|---|
| D-1 | Max photo size | **15 MiB/object** (enforced in the presign request: server signs a PUT only for a declared `content_length` ≤ cap; R2 `Content-Length` is signed so an oversized body is rejected by R2). 25 MiB body cap (`FileUploadMaxBodyBytes`) is the server-proxy fallback only. |
| D-2 | Allowed content types | **`image/jpeg`, `image/png`, `image/webp`, `image/heic`** (field photos). Declared in the presign request and **signed into the PUT** (Content-Type is part of the v4 signature) so R2 rejects a mismatched upload. |
| D-3 | Photos per daily log | **≤ 20** (count check at daily-log persist; UI caps capture). |
| D-4 | EXIF stripping | **Strip on first signed GET, cache the stripped derivative.** Raw upload preserved in R2 (forensics); the served/derivative copy is EXIF-scrubbed (GPS lat/lng in EXIF is **Restricted** PII and must never reach a homeowner page). v1 of the stripper: server-side decode→re-encode dropping metadata on the operator/public read path. Flagged D-1/D-4 in §9 if owner wants strip-on-upload instead. |
| D-5 | Presigned URL TTL (GET) | **operator surface 15 min; public page 5 min** short-lived, re-minted per page render. PUT presign TTL **5 min**. |
| D-6 | Public-link token TTL | **default 30 days, operator-overridable, revocable any time.** (Bootstrap default is 7 days; a homeowner link wants a longer but bounded life.) |
| D-7 | Public page content: photos or text-only first? | **Photos included from the start** (the owner asked for embedded photos). Text-only is the graceful-degrade fallback if storage is unconfigured for the fork (page renders without the photo strip). |
| D-8 | Antivirus scan | **Out of scope v1; flagged.** R2 has no native AV. Mitigations: type/size allowlist signed into the PUT, EXIF strip + re-encode on serve (which neutralizes most polyglot/embedded-payload images), `Content-Disposition: inline` only for known image types, served behind same-origin proxy with `nosniff`. A ClamAV/Cloudflare-scan River job is a v2 follow-up. |
| D-9 | Storage adapter dependency | **Hand-rolled minimal AWS SigV4 signer over `net/http`** — NO AWS SDK (see §A.1). Mirrors the owner-approved "hand-rolled, no new SDK" precedent for the MCP client (`.agents/TECH_STACK.md`: *"the MCP Streamable-HTTP client is hand-rolled … NO new dependency … Owner-approved 2026-06-09"*). |

---

## CHUNK A — STORAGE SUBSTRATE (backend only)

**Goal:** a per-fork-configurable S3-compatible (R2) object store behind an isolated port, an org/project-scoped `assets` table, and presigned PUT (upload) + signed GET (serve) endpoints with limits, RBAC, and audit. **No photos flow yet — this chunk stands alone and is testable with a curl/operator upload.**

### A.1 Dependency decision — hand-rolled SigV4, NO AWS SDK

`.agents/TECH_STACK.md` lists no S3 SDK and the repo has none. Two robust options:

- **Recommended: a minimal AWS Signature V4 signer + presigner over `net/http`** (~250 LOC, stdlib `crypto/hmac`+`crypto/sha256`). This is exactly the precedent set by the **hand-rolled MCP client** (owner-approved, no new SDK). R2 is S3-compatible; presigned PUT/GET need only SigV4 query-param presigning, which is well-specified and stable. **Zero new dependency.**
- Rejected: `aws-sdk-go-v2` — a large transitive tree for what is two signed-URL shapes; not in TECH_STACK; would need an owner dep-approval. Flag in §9 if owner prefers the SDK over hand-rolling.

> **Escalation note:** introducing even the minimal signer is a TECH_STACK addition (a new `internal/storage` package, no external dep). Per dual-agent protocol, record it in `.agents/handoff/ESCALATION_LOG.md` as "no new module dep; hand-rolled SigV4 mirroring the MCP-client precedent" and proceed unless owner objects.

### A.2 The port (`internal/storage`, leaf-isolated)

New package `internal/storage`, **leaf** (stdlib + `crypto/*` only; no imports from `service`/`store`/`ai`). Declares the port the rest of BuildOS consumes:

```go
// internal/storage/storage.go (NEW) — the ObjectStore port.
type ObjectStore interface {
    // PresignPut returns a time-limited URL the client PUTs bytes to,
    // with Content-Type and Content-Length signed in (R2 rejects a
    // mismatched upload). key is the opaque object key the caller chose.
    PresignPut(ctx context.Context, key, contentType string, contentLength int64, ttl time.Duration) (url string, signedHeaders map[string]string, err error)
    // PresignGet returns a time-limited read URL for key.
    PresignGet(ctx context.Context, key string, ttl time.Duration) (url string, err error)
    // Get streams object bytes (used by the same-origin photo proxy so
    // the R2 host never appears in client HTML/CSP). EXIF strip happens
    // in the service layer on top of this, not here.
    Get(ctx context.Context, key string) (io.ReadCloser, contentType string, err error)
    // Delete removes an object (asset hard-delete / GC).
    Delete(ctx context.Context, key string) error
}
```

- **R2 adapter:** `internal/storage/r2.go` — `R2Store` implements `ObjectStore` via the minimal SigV4 signer against the configured endpoint/bucket. Constructed with **per-fork config** (endpoint, bucket, region=`auto`, access key, secret). **NOT hardcoded** (ADR-002: storage is per-fork). The SSRF-guard discipline used for connectors (`net.Dialer.Control` https-only) applies to the `Get`/proxy path, but presign URLs are handed to the *client*, so the endpoint host is operator-configured trust.
- **Config source for R2 creds — two layers, mirror the existing patterns:**
  - **Endpoint + bucket** (non-secret) via `internal/config` env (`R2_ENDPOINT`, `R2_BUCKET`, `STORAGE_REGION` default `auto`) routed through `config.SecretSource` like `DATABASE_URL` etc. — additive entries in `internal/config/config.go`.
  - **Access key + secret** (secret) → **the encrypted vault**, new provider `ProviderObjectStore = "object_store"` (`internal/service/integrations.go`), sealed AES-256-GCM exactly like the anthropic/resend keys, resolved at call time. A new `VaultService.ObjectStoreCreds(ctx, orgID) (accessKey, secret string, err error)` resolver (same shape as `AnthropicKey`/`ResendKey`). Adapter soft-fails when unconfigured (no creds → upload endpoints 503 `STORAGE_UNAVAILABLE`, mirroring AI's soft-fail), so a fork without R2 still boots and runs text-only.
  - Per-fork-init: `make fork-init` / `docs/fork-onboarding.md` gain an optional R2 section (operator pastes endpoint+bucket into env, access key/secret into the vault via `/api/v1/integrations`). Storage is **opt-in per fork**.

### A.3 Schema — `assets` table

```sql
-- migration NNN_assets.up.sql
CREATE TABLE assets (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    project_id     UUID REFERENCES projects(id) ON DELETE SET NULL, -- nullable: org-level assets allowed
    -- Opaque object key in the bucket. Convention: org/<org>/project/<proj>/<uuid>.<ext>
    storage_key    TEXT NOT NULL,
    content_type   TEXT NOT NULL,
    byte_size      BIGINT NOT NULL,             -- NOT a _cents column; size, not money. Linter rule 1 N/A.
    -- 'pending' (presigned, not yet confirmed uploaded) -> 'ready' (HEAD confirmed) -> 'failed'.
    status         TEXT NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','ready','failed')),
    uploaded_by    UUID NOT NULL REFERENCES users(id),
    -- sha256 of the bytes, set on confirm (dedup/integrity, optional v1).
    checksum_sha256 TEXT,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    confirmed_at   TIMESTAMPTZ
);
CREATE INDEX CONCURRENTLY idx_assets_org_project ON assets(org_id, project_id, created_at DESC);
CREATE UNIQUE INDEX CONCURRENTLY idx_assets_storage_key ON assets(storage_key);
```

- `byte_size BIGINT` — **the migration linter rule 1 regexes `cost|price|amount|total|budget|cents|fee|payment|invoice|balance|revenue|expense`; `byte_size`/`content_type` do NOT match**, so `BIGINT` is fine and no `currency_code` is required (no `_cents` column). Confirm against `scripts/lint-migrations.sh`.
- Both indexes `CONCURRENTLY` (lint rule 5). `.down.sql`: `DROP TABLE assets;` with `-- buildos:destructive: drop assets (v2 object-storage domain)`.
- **PII:** `storage_key` is **Internal** (opaque, contains org/project UUIDs only — those are Internal). The *image content* may contain GPS EXIF (Restricted) — handled by EXIF strip on serve (D-4), not by the row. Add no new PII field-name entries for `assets` (keys/sizes/types are Internal/Public).

### A.4 Models / Store / Service

- **Model** `internal/models/asset.go` (NEW): `Asset` mirroring the table. `StorageKey` `json:"-"` (never serialized to clients — they get signed URLs, never raw keys).
- **Store** `internal/store/assets.go` (NEW): `AssetStore` — `Create(tx, params)` (status `pending`), `Confirm(tx, id, checksum, size)` (→ `ready`), `MarkFailed(tx, id)`, `Get(ctx, orgID, id)`, `ListByProject(ctx, orgID, projectID)`, `ListByIDs(ctx, orgID, ids []uuid.UUID)` (for daily-report photo resolution). **All org-scoped**; mutations take `pgx.Tx`.
- **Service** `internal/service/assets.go` (NEW): `AssetService`. Constructor takes `pool`, `AssetStore`, the `storage.ObjectStore` port (injected — the service-layer adapter wires the R2Store from vault creds), `AuditRecorder`, clock. Methods (one-tx + audit each):
  - `RequestUpload(ctx, orgID, userSub, in RequestUploadInput) (Asset, presignedURL, signedHeaders, error)` — validate content-type allowlist (D-2) + size ≤ cap (D-1), verify project in org, generate `storage_key`, INSERT `assets{status:'pending'}` in one tx + audit `asset.upload_requested`, then `PresignPut`. Returns the URL + the headers the client must echo on PUT.
  - `ConfirmUpload(ctx, orgID, userSub, assetID, checksum) (Asset, error)` — optional HEAD-confirm via `ObjectStore` (or trust client), UPDATE `status='ready'` + `confirmed_at` in one tx + audit `asset.uploaded`. **Daily-log linking (Chunk B) requires `status='ready'`.**
  - `SignedGetURL(ctx, orgID, assetID, ttl) (url, error)` — verify asset in org + `ready`, return short-lived `PresignGet`. (Operator surface uses this directly; public surface uses the same-origin proxy in Chunk E.)
  - `ServeAsset(ctx, orgID, assetID) (io.ReadCloser, contentType, error)` — same-origin proxy path: `ObjectStore.Get` + **EXIF strip/re-encode** (D-4) for image types. Used by both the operator thumbnail proxy and (re-validated) the public proxy.
  - AI/storage soft-fail: nil/unconfigured `ObjectStore` → `ErrStorageUnavailable` → 503 `STORAGE_UNAVAILABLE`.

### A.5 Endpoints (authenticated, this chunk)

Mount under the project subtree (where photos belong) + a flat `/assets/{id}` for serve. RBAC: **upload = `RequireMinRole(field_worker)`** (the field captures photos — but field endpoints are caller-scoped under `/api/v1/field`; see Chunk B for the field-facing variant). The generic operator upload here is **`RequireMinRole(superintendent)`**; serve is `RequireMinRole(superintendent)`.

| Method | Path | RBAC | Body / Response |
|---|---|---|---|
| `POST` | `/api/v1/projects/{projectID}/assets/presign` | minRole superintendent | `{ content_type, byte_size, filename? }` → `201 { data: { asset_id, upload_url, signed_headers, expires_at } }`. `400` on type/size; `503 STORAGE_UNAVAILABLE`. |
| `POST` | `/api/v1/assets/{id}/confirm` | minRole superintendent | `{ checksum_sha256? }` → `200 { data: Asset }` (`status: ready`). |
| `GET` | `/api/v1/assets/{id}` | minRole superintendent | `302` to a short-lived signed GET URL **or** `200 image/*` via same-origin proxy (`ServeAsset`, EXIF-stripped). Default: proxy (keeps R2 host out of the client). |
| `GET` | `/api/v1/projects/{projectID}/assets` | minRole superintendent | `200 { data: Asset[] }` (project gallery; `ready` only by default). |

- `MaxBodySize(FileUploadMaxBodyBytes)` is **not** needed on `/presign` (tiny JSON); it *is* applied if a server-proxy PUT fallback is ever added. The actual bytes go **direct to R2**, never through the Go server, on the happy path.
- Mount block guarded `if cfg.AssetService != nil` (matches `agents != nil`); `cmd/server` wires it, `cmd/worker` passes nil.
- Add to `.agents/handoff/API_CONTRACT.md` (net-new "Assets" section).

### A.6 Chunk A tests

- **Go unit** `internal/storage/r2_test.go`: SigV4 presign produces the canonical signature for known fixtures (compare against a published AWS SigV4 test vector / a recorded R2 presign); Content-Type + Content-Length are in `SignedHeaders`; URL expires per TTL. `service/assets_test.go`: type/size allowlist rejects (D-1/D-2/D-3), `ErrStorageUnavailable` when port nil, audit actions recorded, org-scope enforced, `storage_key` never serialized.
- **Go integration** (`//go:build integration`, Testcontainers) `store/assets_integration_test.go`: round-trip create→confirm→list; cross-org `Get` not-found; unique `storage_key`. ObjectStore is a **fake in-memory port** (no live R2 in CI) — the real R2 adapter is exercised by the signer unit test + a manual operator smoke against a sandbox bucket.
- **EXIF test** `service/assets_exif_test.go`: feed a JPEG with embedded GPS EXIF, assert `ServeAsset` output has **no GPS metadata** (D-4 is a redaction-leak guard for photos).
- **lint-migrations:** the `assets` migration passes all 5 rules (no forbidden numeric types on `byte_size`, paired up/down, destructive down header, CONCURRENTLY indexes).

**Ordering:** Chunk A has **no dependency** on B–E. It ships first.

---

## CHUNK B — FIELD PHOTO UPLOAD (backend + mobile)

**Goal:** wire the field app's daily-log photos to request a presigned PUT, upload to R2, and persist `assets` rows linked to the daily log so `daily_logs.photo_asset_ids` resolves to real blobs.

### B.1 Backend — field-facing presign + linking

The field app is caller-scoped under `/api/v1/field/*` (no URL project param; project comes in the body). Add field-facing variants:

| Method | Path | RBAC | Body / Response |
|---|---|---|---|
| `POST` | `/api/v1/field/assets/presign` | authenticated (all roles, incl. `field_worker`) | `{ project_id, content_type, byte_size }` → `201 { data: { asset_id, upload_url, signed_headers, expires_at } }`. Verifies project in caller's org. |
| `POST` | `/api/v1/field/assets/{id}/confirm` | authenticated | `{ checksum_sha256? }` → `200 { data: Asset }`. |

- These reuse `AssetService.RequestUpload`/`ConfirmUpload` (the field handler passes `claims.Sub` + body `project_id`; same org-scope discipline as `field.go`'s `claimsAndOrg`). `field_worker` **can** upload (it owns capture) — this is the one asset path open to `field_worker`; the operator path (Chunk A) is `superintendent+`.
- **Daily-log linking:** `service.FieldService.DailyLog` (and the daily-log store insert) already accept `PhotoAssetIDs []uuid.UUID`. Add a validation step: **every ID must be an `assets` row that is `ready`, in the caller's org, and (if set) for the same project** — reject unknown/foreign/pending IDs with `400 INVALID_PHOTO_ASSET`. This closes the dangling-ID gap: a daily log can only reference confirmed, org-owned blobs. The link itself stays as the existing `UUID[]` column (no new join table v1) — flagged §9 if a `daily_log_assets` join is preferred for richer per-photo metadata.

### B.2 Mobile (Flutter) — `mobile/`

Current state: `photos_screen.dart` captures to session-memory file paths; `sync_service.dart` already sends `photo_asset_ids` (invented client-side). Wire the real flow:

1. On capture (`image_picker`), for each photo: `POST /api/v1/field/assets/presign` (project + content-type + size) → receive `upload_url` + `signed_headers` + `asset_id`.
2. `PUT` the file bytes directly to `upload_url` with the signed headers (Content-Type/Content-Length). **Offline-first:** queue the presign+PUT+confirm as outbox actions (`mobile/lib/models/outbox_action.dart` / `sync_service.dart`) so capture works offline and uploads drain on reconnect; the daily-log outbox action references the real `asset_id`s only after confirm.
3. `POST /api/v1/field/assets/{id}/confirm` then attach the confirmed `asset_id`s to the daily-log submission.

- **If mobile is out of scope for the first pass:** ship B.1 (the backend API) + an **operator/test upload path** (Chunk A's `/projects/{id}/assets/presign` works from the web console / curl), and leave the Flutter wiring as a tracked follow-up. The backend contract is what unblocks Chunks C–E; the Flutter change is additive. **Default: ship B.1 now, Flutter wiring as a fast-follow in the same chunk if time allows.**

### B.3 Chunk B tests

- **Go unit/integration** `service/field_test.go` + `field_integration_test.go`: `DailyLog` rejects unknown/foreign/pending `photo_asset_ids` (`INVALID_PHOTO_ASSET`); accepts `ready` org-owned IDs; cross-org asset never links.
- **Mobile** (if wired): widget/golden test that capture enqueues a presign→PUT→confirm outbox chain; unit test that the daily-log payload carries confirmed IDs only. `mobile/test/live` smoke if a sandbox bucket is available.

**Ordering:** Chunk B depends on **A** (needs `assets` + presign). C–E depend on B only for *real* photos (they degrade to count/text without it).

---

## CHUNK C — DAILY REPORTS SURFACE + AI (backend + web)

**Goal:** operator read surface aggregating daily logs **with photo thumbnails (signed GET)**; the two AI tasks (office digest + client-safe draft) with **deterministic redaction**.

### C.1 Daily-report read model (derived, not a table)

A "daily report" for `(project, date)` is **computed on read** from `daily_logs` + `crew_checkins` + `task_progress` — no `daily_reports` table. Correlate by `(project_id, calendar_date)`: `daily_logs.log_date == crew_checkins.reported_at::date == task_progress.reported_at::date`. `task_progress` joins to a project via `project_tasks.project_id` and **has no `org_id`** — its query MUST join `project_tasks → projects` and assert `projects.org_id = $orgID`.

```go
// internal/models/dailyreport.go (NEW)
type DailyReport struct {
    ProjectID         uuid.UUID          `json:"project_id"`
    ProjectName       string             `json:"project_name"`
    LogDate           time.Time          `json:"log_date"`
    WeatherConditions string             `json:"weather_conditions,omitempty"`
    WorkSummary       string             `json:"work_summary"`
    SafetyIncidents   string             `json:"safety_incidents,omitempty"` // INTERNAL — never to client
    Photos            []PhotoRef         `json:"photos,omitempty"`           // resolved assets (signed GET), not raw IDs
    PhotoCount        int                `json:"photo_count"`
    ReportedBy        Attribution        `json:"reported_by"`                // display_name (Restricted) + id
    CrewCount         int                `json:"crew_count"`                 // crew_checkins JSONB length (opaque, count only)
    TaskProgress      []TaskProgressLine `json:"task_progress,omitempty"`
    ReportedAt        time.Time          `json:"reported_at"`
}
type PhotoRef struct {
    AssetID   uuid.UUID `json:"asset_id"`
    ThumbURL  string    `json:"thumb_url"`  // short-lived signed GET (operator surface, 15 min)
    CreatedAt time.Time `json:"created_at"`
}
```

`crew_members JSONB` is **opaque** (`json.RawMessage`) — read **count only**, tolerate arbitrary shape (OQ-8 default: fold in count + task-progress).

### C.2 Stores / Service

- **Store** `internal/store/field.go` (new read methods, org-scoped, mirror `VerifyProjectInOrg`): `ListDailyLogsByProject(ctx, orgID, projectID, since, until)`, `ListDailyLogsByOrgDate(ctx, orgID, day)`, `CrewCountByProjectDate(ctx, orgID, projectID, day)`, `TaskProgressByProjectDate(ctx, orgID, projectID, day)` (joins through `project_tasks→projects`). Uses `idx_daily_logs_project_date`.
- **Service** `internal/service/reports.go` (NEW) `ReportsService` — **read-only, no audit** (house style: reads aren't audited). `ListProjectReports` / `GetProjectReport` fold crew count + task-progress into `(project, date)` buckets and **resolve `photo_asset_ids` → `[]PhotoRef`** via `AssetService.SignedGetURL` (skip silently if storage unconfigured → `PhotoCount` still set from `len(photo_asset_ids)`). `VerifyProjectInOrg` before any read.

### C.3 AI composition — two typed `*ai.Client` tasks (NOT the agentic harness)

`internal/ai/tasks.go` (the `DailyBriefing` precedent — typed single-shot, `c.callText` + `FastModel`, per-org key from `ai.ContextWithOrgID`, soft-fails `ErrUnconfigured`):

- `DailyReportDigest(ctx, DailyReportDigestRequest) (*DailyReportDigestResponse, error)` — **office/internal**. Terse; INCLUDES safety, crew, schedule deltas. kind `daily_report_digest`.
- `ClientProgressUpdate(ctx, ClientProgressUpdateRequest) (*ClientProgressUpdateResponse, error)` — **client-safe homeowner draft**. Warm tone; EXCLUDES financials, safety-liability, crew identities, GPS. kind `client_progress_update`.

### C.4 Deterministic redaction at the boundary (security-critical, unchanged from v1)

**The AI is NOT the redaction gate.** The service builds the client-task request from an **allowlist of fields**:

- **Excluded from the client prompt entirely:** `safety_incidents`, `crew_members`/crew identities, GPS coords, `reported_by` identity, any `*_cents`/budget, internal notes, **raw photo EXIF (incl. GPS)**.
- **Included:** project name, date range, weather, sanitized work-summary, high-level % complete, photo count, **EXIF-stripped photo derivatives** (for the public page in E; the AI text task still gets count only — no image input v1).
- `pii.FieldClass` classifies the excluded fields (emails/GPS/names = Restricted). The request builder **asserts no Restricted-classed field leaks** into the client payload. The system prompt's "write for a homeowner, omit liability/financial" is belt-and-suspenders only.

### C.5 Endpoints (operator daily-reports read) + web

| Method | Path | RBAC | Response |
|---|---|---|---|
| `GET` | `/api/v1/projects/{projectID}/daily-reports?since=&until=` | minRole superintendent | `200 { data: DailyReport[] }` newest-first |
| `GET` | `/api/v1/projects/{projectID}/daily-reports/{date}` | minRole superintendent | `200 { data: DailyReport }` (`date`=`YYYY-MM-DD`) |

- `safety_incidents` **IS** returned here (internal operator surface); stripped only on the client path. Default window: last 14 days. 404 `PROJECT_NOT_FOUND` uniform on cross-org.
- **Web:** Daily Reports → Command Center group. Route `/command/reports` (list) + `/command/reports/:id` (detail), gate `minRole superintendent`; register in **both** `web/src/router.ts` `RouteGate` and `fb-nav-rail.ts` `NAV_MODEL` (+ `DESIGN_SYSTEM_COMPONENTS.md §1.3`). Add a "Daily Reports" tab to `fb-project-detail-page.ts` `TABS`. Reuse `fb-data-table`, `fb-state`, `fb-markdown`, `fb-breadcrumb`, `fb-tab-bar`, `fb-chip`; model on `fb-briefing-page.ts`. **Photo thumbnails** render from `PhotoRef.thumb_url` (signed, same-origin proxy or signed R2 GET) in a `fb-photo-strip` (new small component or reuse a grid). New web API module `web/src/api/endpoints/reports.ts`.

### C.6 Chunk C tests

- **Go unit** `ai/tasks_test.go`: both tasks marshal the right request, dispatch on the right `task_kind`, `ErrUnconfigured` soft-fails. **Redaction-leak test:** feed a `DailyReport` carrying `safety_incidents`, crew identities, GPS, `*_cents`; assert the **client-task marshaled prompt contains none of them**. `service/reports_test.go`: aggregation buckets correctly; org-scope; photo resolution degrades to count when storage nil.
- **Go integration** `store/field_integration_test.go`: insert logs/checkins/progress → reads return correct org-scoped, date-bucketed results; cross-org isolation; `task_progress` org-scope via join holds; `photo_asset_ids` resolve to signed PhotoRefs (fake ObjectStore).
- **Web** vitest+axe: `fb-reports-page`/`fb-report-detail-page` render list, loading/empty/error states, photo strip, lazy tab. axe clean.

**Ordering:** C depends on A (signed GET for thumbnails) + the field source tables (exist). Degrades to text/count without B's real photos.

---

## CHUNK D — CLIENT UPDATE COMPOSE + EMAIL (backend + web)

**Goal:** the human-in-the-loop composer (AI draft → edit → preview → send), the `client_updates` lifecycle table, send via Resend, and the client contact on projects. **Email delivery only in this chunk; the public link is Chunk E.**

### D.1 Client contact on `projects`

```sql
-- migration NNN_project_client_contact.up.sql
ALTER TABLE projects ADD COLUMN client_name  TEXT;
ALTER TABLE projects ADD COLUMN client_email TEXT;
ALTER TABLE projects ADD COLUMN client_phone TEXT;
UPDATE projects p SET client_name=pcp.client_name, client_email=pcp.client_email, client_phone=pcp.client_phone
  FROM pre_construction_prospects pcp
 WHERE pcp.project_id=p.id AND p.client_email IS NULL;
```

- All nullable. `.down.sql` drops the three with `-- buildos:destructive: revert client-contact columns on projects`.
- **PII:** add `client_email`/`client_name`/`client_phone` → **Restricted** in `pii.FieldClass`. Never serialize `client_email` to `field_worker` responses (gate at handler/role). `models.Project` gains `ClientName/ClientEmail/ClientPhone *string` (`omitempty`).
- Fix `CreateProjectFromProspect` (`internal/store` + `internal/service/pipeline.go`) to carry the three fields forward (close the leak at source, not just backfill).
- **Decision (v1):** columns on `projects` (single homeowner) — `project_contacts` table is the v2 multi-stakeholder path (§9).

### D.2 `client_updates` lifecycle table

```sql
-- migration NNN_client_updates.up.sql
CREATE TABLE client_updates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    project_id      UUID NOT NULL REFERENCES projects(id),
    period_start    DATE NOT NULL,
    period_end      DATE NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','sent','failed')),
    ai_draft        TEXT,
    edited_body     TEXT NOT NULL DEFAULT '',
    subject         TEXT NOT NULL DEFAULT '',
    recipient_email TEXT,                        -- snapshot at send; Restricted; never logged
    -- Photo assets the operator chose to include in this update (subset of the
    -- period's daily-log photos). Resolved to signed/proxied URLs at render.
    photo_asset_ids UUID[] NOT NULL DEFAULT '{}',
    created_by      UUID NOT NULL REFERENCES users(id),
    sent_by         UUID REFERENCES users(id),
    sent_at         TIMESTAMPTZ,
    send_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX CONCURRENTLY idx_client_updates_project ON client_updates(project_id, created_at DESC);
CREATE INDEX CONCURRENTLY idx_client_updates_org_status ON client_updates(org_id, status, created_at DESC);
```

- `photo_asset_ids` added vs v1 (operator curates which photos the homeowner sees — a redaction control). No `_cents`. `.down.sql`: `DROP TABLE client_updates;` + `-- buildos:destructive: drop client_updates (client-update domain)`.
- **PII:** `recipient_email` Restricted (never in audit metadata — reference `client_update.id`+`project_id` only); `ai_draft`/`edited_body`/`subject` Confidential.

### D.3 Service (`internal/service/client_update.go`, NEW)

Constructor: `pool`, `ReportsService` (or field/schedule stores), the two AI task interfaces (consumer-side, like `DailyBriefer`), `clientUpdateStore`, `AssetService` (for photo validation/resolution), `mailer.Mailer` (nil → `NewNoopMailer`), `AuditRecorder`. Nil AI → 503 sentinel (mirror `ErrAgentsAIUnavailable`).

| Method | Tx + Audit |
|---|---|
| `CreateDraft(ctx, orgID, projectID, period)` | Call Chunk C `DraftClientUpdate` (the **redacted** AI draft — itself audited `client_update.drafted`, resource = project), then INSERT `{status:'draft', ai_draft, edited_body=ai_draft}` one tx, audit **`client_update.created`** (resource = the new row). AI soft-fail → 503, no empty draft. |
| `UpdateDraft(ctx, orgID, id, edited_body, subject, photo_asset_ids)` | Operator edit incl. **curating which photos** (validate IDs are `ready` + org + project-matched). One-tx UPDATE (status `draft` **or** `failed` — editing a failed row resets it to `draft` + clears `send_error`), audit **`client_update.updated`**. |
| `SendClientUpdate(ctx, orgID, id)` | Load draft + `client_email`. Reject empty (`ErrNoClientContact` → 422). One tx: snapshot `recipient_email`, set `sent_by`/`sent_at`/`status='sent'`, audit `client_update.sent`. **Then** `mailer.Send` **after commit** (see ordering). |
| `GenerateOfficeDigest(ctx, orgID, projectID, period)` | Call `DailyReportDigest`, `CreateFeedCard` (`card_type` e.g. `daily_digest`) to operators, audit `report.digest.generated`. |

**Send ordering:** persist `status='sent'` + audit **inside the tx**, then `mailer.Send` **after commit**. On `ErrMailerUnconfigured` → 422 `MAILER_UNCONFIGURED`, roll status to `draft` or write `status='failed'`+`send_error` (operator expects this email to go out — do not swallow). Never log `recipient_email`. Email composition copies `auth.go sendResetEmail` (build `mailer.Message`, render markdown→HTML; the homeowner email **may embed photos** as `<img>` pointing at the **public same-origin proxy** or signed URLs — see E for the public-link case; for plain email, use signed R2 GET URLs with a longer TTL or inline CID attachments — **default: signed GET URLs, 7-day TTL for email-embedded images**, flagged §9).

### D.4 Endpoints + web

**RBAC: `RequireRole(owner, admin)`** (external comms = owner/admin trust; OQ-1 default).

| Method | Path | RBAC | Body / Response |
|---|---|---|---|
| `POST` | `/api/v1/projects/{projectID}/client-updates` | owner,admin | `{ period_start, period_end }` → `201 { data: ClientUpdate }`. `503 AI_UNAVAILABLE`. |
| `GET` | `/api/v1/projects/{projectID}/client-updates` | owner,admin | `200 { data: ClientUpdate[] }` newest-first. |
| `GET` | `/api/v1/client-updates/{id}` | owner,admin | `200 { data: ClientUpdate }`. |
| `PATCH` | `/api/v1/client-updates/{id}` | owner,admin | `{ edited_body, subject, photo_asset_ids? }` → `200` (draft only; `409 ALREADY_SENT`). |
| `POST` | `/api/v1/client-updates/{id}/send` | owner,admin | → `200` (`sent`). `422 NO_CLIENT_CONTACT`, `422 MAILER_UNCONFIGURED`, `409 ALREADY_SENT`. |

- `ClientUpdate` JSON **omits `recipient_email`** from list responses; org-scope every query; 404 uniform on cross-org.
- **Web:** Client Updates → Portfolio group. Route `/portfolio/client-updates` (list/history) + composer (deep-linked `?project=<id>`), gate `roles owner,admin`. Flow: select project+range → POST draft → AI-draft step (model on `fb-briefing-page.ts` AI-hero gated/transient/ok; 503 → Integrations deep-link) → edit step (textarea/`fb-markdown`, **photo picker** to curate `photo_asset_ids` from the period gallery) → preview (`fb-markdown` + photo strip) → send (`fb-confirm`/`fb-modal` confirm before external send) → POST `/send`. New web API module `web/src/api/endpoints/client-updates.ts`. "Send update"/"Draft client update" actions on project detail + daily-report detail.

### D.5 Chunk D tests

- **Go unit** `service/client_update_test.go`: draft→edit→send happy path (fake mailer/AI/audit); `SendClientUpdate` rejects empty `client_email`; `MAILER_UNCONFIGURED` rolls back / records `failed`; PATCH on `sent` → `ErrAlreadySent`; **`recipient_email` never in any logged field** (capture slog); photo-curation rejects non-ready/foreign IDs. `pii_test.go`: `client_email`/`recipient_email` Restricted; `ScrubMap` redacts in nested `client_update` metadata.
- **Go integration** `store/client_update_integration_test.go`: round-trip + status CHECK + cross-org not-found; full `DraftClientUpdate`→`UpdateDraft`→`SendClientUpdate` against real DB + fake mailer (row transitions + audit in same tx). `pipeline` integration: `CreateProjectFromProspect` carries `client_*`; backfill UPDATE populates.
- **Web** vitest+axe: composer states, AI-gated 503, send-confirm modal fires before POST, photo picker. axe clean.

**Ordering:** D depends on A (photo resolution) + C (reports + AI tasks). Email works without E.

---

## CHUNK E — PUBLIC SHAREABLE LINK (backend + minimal web/HTML)

**Goal:** an unauthenticated, **token-gated, read-only** homeowner progress page that **bypasses Auth + SetupGate cleanly** (router placement), is **rate-limited**, serves **only client-safe redacted content + short-lived proxied photos**, and **never exposes the raw ERP**. This is the first surface outside everything-behind-auth — security is the point.

### E.1 Token model (template: `setup_bootstrap_tokens`)

```sql
-- migration NNN_client_update_share_links.up.sql
CREATE TABLE client_update_share_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    client_update_id UUID NOT NULL REFERENCES client_updates(id) ON DELETE CASCADE,
    token_hash      BYTEA NOT NULL,              -- sha256 of the 32-byte CSPRNG cleartext; cleartext NEVER stored
    expires_at      TIMESTAMPTZ NOT NULL,
    revoked_at      TIMESTAMPTZ,                 -- operator revoke (NULL = active)
    created_by      UUID NOT NULL REFERENCES users(id),
    last_viewed_at  TIMESTAMPTZ,                 -- best-effort, no PII
    view_count      INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX CONCURRENTLY idx_share_links_token_hash ON client_update_share_links(token_hash);
CREATE INDEX CONCURRENTLY idx_share_links_update ON client_update_share_links(client_update_id, created_at DESC);
```

- **Mirror the bootstrap token exactly:** 32-byte CSPRNG cleartext, base64url (43 chars), **only the sha256 hash stored**, `GetActiveByHash` filters `expires_at > now AND revoked_at IS NULL`, resolution returns a **uniform not-found** on any failure (missing/expired/revoked/mismatch) — never distinguish reasons (enumeration defense). Cleartext shown **once** to the operator at create (then it's the URL they email). Default TTL **30 days** (D-6), operator-overridable, **revocable**.
- `.down.sql`: `DROP TABLE client_update_share_links;` + `-- buildos:destructive: drop client_update_share_links (public share domain)`.
- **PII:** `token_hash` is the secret's hash (treat like a credential — never log). No emails on this table.

### E.2 Share-link lifecycle service (authenticated, owner/admin)

`internal/service/share_link.go` (NEW), or fold into `ClientUpdateService`. One-tx + audit:

- `CreateShareLink(ctx, orgID, clientUpdateID, userSub, ttl) (cleartext, ShareLink, error)` — only for a **`sent`** (or explicitly `draft`-preview-allowed?) update; INSERT hashed token; audit `client_update.share_link.created`. Returns cleartext once. Default: link allowed once the update is `sent` (the homeowner sees what was emailed). Flag §9 if pre-send preview links are wanted.
- `RevokeShareLink(ctx, orgID, linkID, userSub)` — set `revoked_at`; audit `client_update.share_link.revoked`.
- `ListShareLinks(ctx, orgID, clientUpdateID)` — operator sees active/expired/revoked (never cleartext).

Authenticated endpoints (owner,admin), under the existing auth group:

| Method | Path | RBAC | Response |
|---|---|---|---|
| `POST` | `/api/v1/client-updates/{id}/share-links` | owner,admin | `{ ttl_days? }` → `201 { data: { url, expires_at } }` (url = `https://<host>/p/<cleartext>`; shown once). |
| `GET` | `/api/v1/client-updates/{id}/share-links` | owner,admin | `200 { data: ShareLink[] }` (no cleartext). |
| `DELETE` | `/api/v1/share-links/{linkID}` | owner,admin | `204` (revoke). |

### E.3 The PUBLIC route (UNAUTHENTICATED — the first one)

**Router placement (the critical bit):** mount a `MountPublicShareRoutes(r, publicHandler)` as a **sibling of `MountAuthRoutes`** in `internal/api/router.go` — **outside** the `r.Group` that applies `authMiddleware` + `SetupGate`. It therefore:
- **inherits** the root global stack (RequestID, RealIP, otelhttp, SecurityHeaders, **RateLimiter**, Sentry, Recoverer) — so it is rate-limited and security-headed automatically;
- **bypasses** Auth + SetupGate **without touching either** — no entry added to `DefaultSetupGateExemptPrefixes` (the gate only runs inside the auth group, which this route never enters);
- **works through Cloudflare** unchanged (proxied CNAME, no JS Worker) — it's just another same-origin path the proxy forwards.

Two public, unauth routes under a dedicated prefix (`/p/` chosen to not collide with `/api/`, `/share/` reserved for SPA if needed):

| Method | Path | Auth | Response |
|---|---|---|---|
| `GET` | `/p/{token}` | **NONE** (token-gated) | `200 text/html` — the rendered progress page (server-rendered minimal HTML, **not** the SPA) OR `200 application/json { data: PublicUpdate }` if a JSON content path is preferred for a tiny JS render. **404** (uniform) on any invalid/expired/revoked token. |
| `GET` | `/p/{token}/photos/{assetID}` | **NONE** (token-gated) | `200 image/*` — **same-origin proxy** of an EXIF-stripped photo, but ONLY if `assetID` is in the linked update's curated `photo_asset_ids`. 404 otherwise. |

- **What the public payload contains (allowlist — the redaction gate):** project name, address city/region only (NOT full street — flag §9), period range, the operator-edited `edited_body` (already client-safe prose), the curated photos. **NEVER:** `safety_incidents`, crew identities, GPS/EXIF, any `*_cents`/budget, `recipient_email`, internal notes, schedule internals, other projects, any org-wide data. The public handler builds the payload from a **dedicated `PublicUpdate` projection** that physically cannot carry ERP fields (it only has the allowlisted columns) — same discipline as the AI client-task allowlist.
- **Photos on the public page:** served via `/p/{token}/photos/{assetID}` (same-origin proxy → `AssetService.ServeAsset`, **EXIF-stripped**, 5-min effective freshness) so the page CSP stays `img-src 'self'`, the R2 host never appears in client HTML, and a leaked token grants only the curated photos for that one update (re-validated against the link's `photo_asset_ids` on every request). No raw R2 presigned URL ever reaches the homeowner.
- **Page security headers:** the public HTML handler sets its **own CSP per-response** (like `spa.go`): `default-src 'none'; img-src 'self'; style-src 'self' 'unsafe-inline'; base-uri 'self'; form-action 'none'; frame-ancestors 'none'`. Inherits `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `nosniff` from `SecurityHeaders`. **No `Strict-Transport-Security` weakening, no cookies, no JS that calls `/api/*`** (the page is self-contained server-rendered HTML; if any JS, it only calls `/p/*`).
- **Rate limiting:** inherited per-IP limiter already covers it; consider a **tighter dedicated limiter** for `/p/*` (e.g. 10 rps/IP) since legit homeowner traffic is low and this is the brute-force surface — construct a second `IPRateLimiter` and apply only on the public group. **Default: dedicated stricter limiter on `/p/*`.**
- **No SetupGate concerns:** because the route is outside the auth group, an incomplete-onboarding fork still serves a valid share link — acceptable (links are only mintable post-`sent`, which requires a configured fork). Confirm §9.

### E.4 Web / HTML for the public page

- **Server-rendered minimal HTML** (a new `internal/api/public_share.go` template, modeled on `spa.go`'s self-contained response), **NOT** the Lit SPA (the SPA assumes auth + `/api` connectivity; the public page must be standalone, no auth bundle, no ERP API surface). Dark, branded, read-only: project name, period, prose (markdown→sanitized HTML), photo grid (`<img src="/p/{token}/photos/{id}">`). axe-clean static template.
- Operator side (in the Chunk D composer): a **"Create public link"** action (owner/admin) that calls `POST .../share-links`, shows the URL once with copy-to-clipboard + a revoke control listing active links. New web bits in `client-updates.ts` + the composer page.

### E.5 Chunk E tests (security-focused)

- **Go unit** `service/share_link_test.go`: token is 32-byte CSPRNG base64url; only hash stored; **uniform not-found** on missing/expired/revoked/mismatch (assert error is identical across all four); revoke flips `revoked_at`; audit recorded.
- **Public-link-cannot-see-raw-ERP test** (the headline security test) `api/public_share_test.go`: given a `client_update` whose source `DailyReport`s carry `safety_incidents`, crew identities, GPS, `*_cents`, full street address, and a sibling project's data — assert the rendered `/p/{token}` HTML/JSON contains **NONE** of them (grep the response body for each forbidden value → must be absent); assert a photo NOT in the curated set 404s at `/p/{token}/photos/{id}`; assert an expired/revoked/garbage token 404s uniformly; assert no `Set-Cookie`, no `/api/*` reference in the page; assert CSP header present and strict.
- **EXIF test** (reuses A's): public photo proxy output has no GPS metadata.
- **Router test**: `/p/{token}` is reachable with NO auth header and with SetupGate active (onboarding incomplete) — proves bypass; and an authenticated `/api/*` route with a bad/no token still 401s (proves the bypass didn't weaken auth elsewhere).
- **Rate-limit test**: `/p/*` returns 429 past the dedicated limiter.
- **Web** axe on the static public template.

**Ordering:** E depends on A (photo proxy), C (the report/redaction allowlist), D (`client_updates` + curated photos). **Ships last.**

---

## WIRING SUMMARY (cross-chunk)

### Migrations (4 new, each in its chunk)
- A: `NNN_assets.{up,down}.sql` — `assets` table + 2 CONCURRENTLY indexes; destructive down header.
- D: `NNN_project_client_contact.{up,down}.sql` (3 nullable cols + backfill, destructive down) + `NNN_client_updates.{up,down}.sql` (table + `photo_asset_ids`, 2 CONCURRENTLY indexes, destructive down).
- E: `NNN_client_update_share_links.{up,down}.sql` — token table + 2 CONCURRENTLY indexes; destructive down.

### New packages / files
- `internal/storage/` (NEW, leaf): `storage.go` (port), `r2.go` (SigV4 adapter), `sigv4.go` (signer), tests. **Add to `make lint-isolation` allowlist as leaf** (stdlib + crypto only).
- `internal/models/`: `asset.go`, `dailyreport.go`, `client_update.go`, `share_link.go`; `project.go` +client fields.
- `internal/store/`: `assets.go`, `client_update.go`, `share_link.go`; `field.go` +reads; `projects.go` +client cols; `pipeline` fix.
- `internal/service/`: `assets.go`, `reports.go`, `client_update.go`, `share_link.go`; `integrations.go` +`ProviderObjectStore`+`ObjectStoreCreds`; `pipeline.go` +backfill; `audit.go` +resource consts (`asset`, `daily_report`, `client_update`, `share_link`).
- `internal/ai/tasks.go`: `DailyReportDigest` + `ClientProgressUpdate`.
- `internal/api/`: `assets.go`, `reports.go`, `client_update.go`, `share_link.go` (auth), `public_share.go` (UNAUTH); `router.go` mounts (guarded `if cfg.XService != nil`; public route sibling of `MountAuthRoutes`).
- `internal/config/config.go`: +`R2Endpoint`, `R2Bucket`, `StorageRegion` (via `SecretSource`).
- `internal/pii/pii.go`: +`client_email`/`client_name`/`client_phone`/`recipient_email` → Restricted.

### Audit actions (new)
`asset.upload_requested`, `asset.uploaded`, `report.digest.generated`, `client_update.drafted` (Chunk C AI-draft generation, resource = project), `client_update.created/updated/sent/send_failed` (the persisted-row lifecycle — `.created`/`.updated` match the house `project.created/updated` convention; `.drafted` is the distinct transient-draft event), `client_update.share_link.created/revoked`. (Reads unaudited.)

### Web (lockstep `router.ts` + `fb-nav-rail.ts` + `DESIGN_SYSTEM_COMPONENTS §1.3`)
- `/command/reports` (Command Center, minRole superintendent) — C.
- `/portfolio/client-updates` (Portfolio, owner/admin) — D/E.
- Project-detail `TABS`: "Daily Reports".
- New pages: `fb-reports-page`, `fb-report-detail-page`, `fb-client-update-page` (+ photo strip/picker, share-link controls). API modules `reports.ts`, `client-updates.ts`. Public page is **server-rendered, not in the SPA**.

### Mobile (Chunk B)
`photos_screen.dart` + `sync_service.dart` + `outbox_action.dart`: capture → presign → PUT → confirm → attach real `asset_id`s. (Or backend-only first pass + Flutter fast-follow.)

### Spec docs (dual-agent protocol)
Author net-new sections in `.agents/handoff/API_CONTRACT.md` (Assets, Daily Reports, Client Updates, Share Links, Public Share) and `UX_CORE_SCREENS.md`. Record in `ESCALATION_LOG.md`: the hand-rolled-SigV4 dep decision (D-9), and any §9 items the owner hasn't pre-approved.

---

## AGENTIC FRAMING

**How this advances VISION:** "daily reports → client updates" is the textbook **GC-coordination chore** — every evening a GC reads the day's field logs, recaps to the office, and updates the homeowner. The harness composes both communications with the deterministic engine (schedule snapshot) as ground truth and the operator as the send gate. The public link extends the homeowner touchpoint without weakening the auth model.

**RECOMMENDATION — no new harness role for v1; plain services.** v1 is a directed compose-and-send (not cross-module judgment that applies deltas in one tx), and the human-in-the-loop send gate is antithetical to the harness's autonomous-tick model. Keeping it `service.ClientUpdateService`/`AssetService` avoids the leaf-isolation constraint (which would force `mailer.Send`/`ObjectStore` into Workspace adapters) for no benefit yet. **`internal/storage` is leaf-isolated (stdlib+crypto only)** and must stay out of `internal/agentic`'s import graph — `make lint-isolation` stays green.

**v2 harness path (record, don't build):** a per-org "draft a client update every Friday" capability that lands a **draft** (never auto-sent) + a "review & send" feed card. At that point declare an `agentic` port (`DraftClientUpdate`), implement the adapter in `internal/service` (it owns the tx, the INSERT, and any future `mailer.Send`/storage), register the capability. The office digest is the more natural first harness candidate (internal, surfaces passively, no external-send risk).

---

## TEST PLAN ROLL-UP + VERIFICATION

Per-chunk tests are in §A.6/B.3/C.6/D.5/E.5. The **two non-negotiable security tests**:
1. **Redaction-leak test (C.6):** the client AI-task request builder NEVER includes `safety_incidents`, crew identities, GPS, or `*_cents`.
2. **Public-link-cannot-see-raw-ERP test (E.5):** the rendered `/p/{token}` page contains NONE of safety/crew/GPS/cents/full-address/sibling-project data; uncurated photos 404; bad/expired/revoked tokens 404 uniformly; no cookies, no `/api/*` reference, strict CSP.

Plus the **EXIF strip** test (photos never leak GPS to client/public surfaces) and the **router bypass** test (public route reachable unauth with SetupGate on; auth elsewhere unweakened).

### Browser verification walkthrough (proves the full loop)
With `DEV_AUTH_MODE=header`, a seeded project with `client_email`, a sandbox Resend key, and a sandbox R2 bucket configured in the vault:
1. **Field log + photo:** request a field presign, PUT a photo to R2, confirm, POST `/api/v1/field/daily-log` with the real `asset_id`.
2. **Office sees it:** as superintendent, `/command/reports` shows the log with weather/work-summary/crew-count and a **photo thumbnail** (signed GET); `safety_incidents` visible (internal).
3. **AI drafts:** as owner, open the composer, generate → client-safe draft (assert safety/crew/GPS absent), curate which photos to include.
4. **Edit + preview:** tweak body, preview renders markdown + photo strip.
5. **Send:** confirm dialog → `client_updates` flips `sent`, captured Resend request shows homeowner address + edited body + photo `<img>` URLs; audit `client_update.sent`; `recipient_email` absent from logs.
6. **Public link:** create a share link → open `/p/{token}` in an **unauthenticated** browser → see the redacted page + EXIF-stripped photos; confirm safety/crew/GPS/cents absent, no `/api/*` calls, strict CSP; revoke → `/p/{token}` now 404s.
7. **History:** the update + active links show in `/portfolio/client-updates`.

`make audit` (+ `make lint-isolation`, + `make lint-migrations` on the 4 new migrations) must stay green; confirm `internal/agentic` and `internal/storage` gain no forbidden imports.

---

## 9. REMAINING HARD DECISIONS (defaults picked; confirm before/while building)

These belong in `.agents/handoff/ESCALATION_LOG.md`; the chosen default is what gets built unless the owner objects.

- **§9-1 (was OQ-1) Client Updates / share-link RBAC:** `{owner,admin}` (default — external comms = owner/admin trust) vs allow `superintendent`.
- **§9-2 (was OQ-2) Contact storage:** columns on `projects` (default) vs `project_contacts` table (multi-stakeholder).
- **§9-3 Storage dependency (D-9):** hand-rolled SigV4, no SDK (default, mirrors MCP-client precedent) vs `aws-sdk-go-v2` (needs dep approval).
- **§9-4 EXIF (D-4):** strip-on-serve, keep raw in R2 (default) vs strip-on-upload (no raw retained).
- **§9-5 Max photo size / count (D-1/D-3):** 15 MiB, ≤20/log (defaults).
- **§9-6 Token TTL (D-6):** 30-day default, revocable (default) vs shorter.
- **§9-7 Public page address granularity:** city/region only (default) vs full street vs none.
- **§9-8 Public photo path (D-7):** same-origin EXIF-stripped proxy (default) vs raw presigned R2 GET (rejected — leaks R2 host + bypasses EXIF strip + CSP widening).
- **§9-9 Email-embedded image delivery (D-3.x):** signed GET URLs 7-day TTL (default) vs CID inline attachments vs the same `/p/*` proxy.
- **§9-10 Share link before send:** link mintable only after `sent` (default) vs allow draft-preview links.
- **§9-11 Dedicated `/p/*` rate limiter:** stricter second limiter (default) vs reuse the global one.
- **§9-12 `daily_log_assets` join vs `UUID[]`:** keep `UUID[]` column (default) vs join table for per-photo metadata.
- **§9-13 Antivirus (D-8):** out of scope v1, mitigations only (default) vs block on a scan job.
- **§9-14 Aggregation window:** operator-chosen `period_start`/`period_end`, default since-last-sent else last 7 days (default).
- **§9-15 Office digest delivery:** feed card to operators (default) and/or office email.
- **§9-16 Redaction trust:** deterministic allowlist is the gate, AI prompt belt-and-suspenders (default; trusting the model rejected).
