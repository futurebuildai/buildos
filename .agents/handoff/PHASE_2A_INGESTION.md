# Phase 2a — Invoice Ingestion Pipeline (file-level implementation spec)

> **Status:** Spec ready for ultracode · **Author:** Lead architect (ultraplan) · **Date:** 2026-06-08
> **Companion:** [VISION.md](../../VISION.md) (north star) · [PHASES_2-4_ULTRALOOP_PLAN.md](PHASES_2-4_ULTRALOOP_PLAN.md) (chunk 2a row)
> **Chunk goal (verbatim from the plan):** turn the orphaned `invoice_extract` AI task into a real
> ingestion pipeline — an invoice document is extracted by AI into structured fields, then persisted
> as a real `invoices` row and surfaced as a review feed card for human approval. The harness
> ingestion role: *unstructured reality → structured ERP records.*

---

## 0. Decision summary (read this first)

This spec **scopes Phase 2a as a service-layer ingestion pipeline (no new `internal/agentic` port)**
and lands the agentic-port version as an **explicit, mechanical deferred seam (§11)** that Phase 2b
will promote when a second trigger (River/webhook) appears.

That decision was contested in planning (it appears to soften HARD PRINCIPLE #4, "mirror Phase 1 for
any new port/adapter," and the VISION "harness ingestion role"). It is resolved here, not improvised
away — see §8.4 (Escalation resolution) for why the service-layer home is correct *and isolation-clean*
for this specific shape, and §11 for the exact promotion path. **If the reviewing owner mandates the
agentic-port home for all ingestion, this spec's §1–§3 change shape (see §11) but §6/§7/§9/§10 do not.**

### Why minimal-slice / service-direct wins (and the runner-up tradeoff)

| | **Chosen: service-direct (no new port)** | **Runner-up: agentic-port (mirror Phase 1)** |
|---|---|---|
| Shape | one AI call → one write tx, in `internal/service` | new `DocExtractor`/`IngestWorkspace` ports in `internal/agentic` + 2 adapters + Orchestrator method + Registry capability |
| Fit | Ingestion 2a is a **linear** unstructured→structured transform with **one** fuzzy step and **one** persistence target. Phase 1's Orchestrator/Registry earns its cost when AI judgment **fans out across modules** (schedule→procurement→budget→crew). 2a has no fan-out. | Speculative generality: invents a capability + two ports + an orchestrator method before a second consumer exists. |
| Isolation | Clean: adds **nothing** to `internal/agentic`; `make lint-isolation` has nothing new to police. `internal/service` already legitimately imports `ai`/`store`/`currency` (e.g. `agentic.go`, `budget.go`). | Also clean, but at the cost of the machinery above. |
| Generalize | The `invoiceExtractorAI` narrow interface + the outbox table shape + the validation chokepoint are exactly what 2b promotes — a **mechanical lift** (§11). | Would already be in the harness, but 2a doesn't need it yet. |

**The runner-up's fatal flaw (caught in adversarial review, verified against the code):** the
agentic-port design proposed an *in-transaction* "detect UNIQUE conflict → then read the prior
invoice/card refs and return a deduped result in the same tx." **This is impossible in Postgres** —
once a statement raises SQLSTATE `23505`, the transaction is poisoned (`25P02`, "current transaction
is aborted") and every subsequent statement, including the dedupe SELECT, fails until ROLLBACK. The
existing field-sync pattern (`internal/store/field.go`) works *precisely because the conflict is the
last act* and the tx rolls back. This spec adopts the field-sync semantics exactly: **conflict →
bare 409, no in-tx read-back** (§6).

We also graft three best-of ideas from the other approaches:
- From agentic-port: a dedicated **outbox table** (isolates ingestion dedupe from the unconstrained
  manual `invoices` path) + a **document `source` provenance column**.
- From the critiques: **deterministic validation of *all* AI-sourced fields** (not just money) by
  routing through `BudgetService.CreateInvoice` so vendor/amount/currency invariants can't be
  bypassed (§7.5), and an explicit **PDF/multipart deferral** to 2b with a JSON `document_url`/`text`
  transport for 2a (the only transport `ai.InvoiceExtract` supports today) (§5.3).

---

## 1. Package / file layout

### New files
| Path | Purpose |
|---|---|
| `migrations/014_invoice_ingestions.up.sql` | additive: `invoice_ingestions` outbox table + `invoices.source` column |
| `migrations/014_invoice_ingestions.down.sql` | paired down (destructive header; drops the new table + column) |
| `internal/service/invoice_ingest.go` | `IngestionService` + `IngestInvoiceFromDocument` — the fuzzy→exact orchestration |
| `internal/service/invoice_ingest_test.go` | unit tests: validation, soft-fail, field-mapping (fake `invoiceExtractorAI`) |
| `internal/service/invoice_ingest_integration_test.go` | `//go:build integration`: real PG, end-to-end + idempotency + soft-fail |
| `internal/store/invoice_ingestions.go` | outbox store: `InsertInvoiceIngestion` (claims the dedupe slot, returns `ErrIdempotencyConflict`) |
| `internal/api/invoice_ingest.go` | `IngestInvoice` HTTP handler (JSON in; 201/409/422/503/400 mapping) |

### Edited files
| Path | Edit |
|---|---|
| `internal/store/financials.go` | add `Source *string` to `CreateInvoiceParams` **and** the INSERT column list; add `CreateInvoiceLineItemsJSON` is **not** needed (line items deferred — §7.6) |
| `internal/store/errors.go` *(or wherever it lands)* | **reuse** the existing `store.ErrIdempotencyConflict`; do **not** mint a second sentinel. It currently lives package-level in `internal/store/field.go` (`var ErrIdempotencyConflict`), so it is already importable as `store.ErrIdempotencyConflict` — no move required |
| `internal/service/budget.go` | add `Source *string` to `CreateInvoiceInput`, thread it into `store.CreateInvoiceParams`; add **one** package-level helper or reuse `IngestionService` calling `CreateInvoice` (see §7.5) |
| `internal/api/financials.go` | (no change required if the new handler lives in `invoice_ingest.go`; it reuses `callerOrgIDFromClaims`, `writeErrorResponse`, the response envelope helpers already in this package) |
| `internal/api/router.go` | mount `POST /api/v1/projects/{projectID}/invoices/ingest` **inside the existing `/invoices` subrouter** (already `r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))`) — one line |
| `cmd/server/main.go` | construct `IngestionService` (pool, `*ai.Client`, financials store, ingestion store, feed store, audit) and wire it onto the financials handler / a new handler |
| `internal/service/audit.go` | **no edit** — reuse `AuditResourceInvoice`; add the new action **string literal** `"ingestion.invoice.extracted"` at the call site (no constant needed; mirrors how `"invoice.created"` is a literal) |

### Explicitly **NOT** touched (this is the isolation proof)
- `internal/agentic/*` — **no** new port, capability, or orchestrator method. The leaf gate has nothing new to police.
- `internal/physics`, `internal/currency` — imported *by* `service` for validation only; never the reverse.
- `internal/worker/jobs.go` — **no** River job in 2a (YAGNI; the async trigger is the 2b motivator that promotes the port — §11).
- `internal/api/middleware/bodysize.go` — **no** multipart; 2a is JSON-only (§5.3).
- `internal/ai/*` — `InvoiceExtract` already exists; unchanged.

---

## 2. Key Go types / interfaces / signatures

### `internal/service/invoice_ingest.go`

```go
package service

// invoiceExtractorAI is the consumer-side seam over *ai.Client — one method wide,
// so tests inject a fake without an HTTP server. Mirrors cascadeReasonerAI in
// internal/service/agentic.go. Deliberately defined HERE (not promoted to an
// agentic port) because 2a has exactly one consumer; §11 promotes it in 2b.
type invoiceExtractorAI interface {
    InvoiceExtract(ctx context.Context, req ai.InvoiceExtractRequest) (*ai.InvoiceExtractResponse, error)
}

// ErrInvoiceExtractionInvalid is the sentinel for "the fuzzy AI output failed the
// deterministic gate" (bad currency, non-positive total, line-sum mismatch beyond
// tolerance, empty vendor). Handler maps it to 422.
var ErrInvoiceExtractionInvalid = errors.New("service: invoice extraction failed validation")

type IngestionService struct {
    pool     *pgxpool.Pool
    ai       invoiceExtractorAI          // nil-safe (typed-nil guard in constructor) → behaves like ErrUnconfigured
    budget   *BudgetService              // reuse its CreateInvoice → ALL invariants apply (§7.5)
    ingStore *store.InvoiceIngestionStore
    feedStore *store.FeedCardsStore
    audit    AuditRecorder               // NoopAuditRecorder{} if nil
}

// NewIngestionService wires the deps. Takes the concrete *ai.Client (not the
// interface) to dodge the typed-nil hazard — store only a non-nil client into the
// interface field, exactly as NewCascadeReasoner does.
func NewIngestionService(
    pool *pgxpool.Pool,
    client *ai.Client,
    budget *BudgetService,
    ing *store.InvoiceIngestionStore,
    feed *store.FeedCardsStore,
    audit AuditRecorder,
) *IngestionService

// IngestInvoiceInput — what the handler hands the service. DocumentURL XOR Text.
type IngestInvoiceInput struct {
    ProjectID      uuid.UUID
    IdempotencyKey uuid.UUID  // client-supplied; the dedupe anchor (§6)
    DocumentURL    string     // signed URL to a jpeg/png/gif/webp image
    Text           string     // OR raw text; exactly one of the two
    CurrencyOverride *string  // optional operator override; if set, AI currency must match or 422
    WBSCode        *string    // optional cost-code hint; persisted as-is, NEVER fed to AI math
}

type IngestInvoiceResult struct {
    Invoice    models.Invoice
    ReviewCard models.FeedCard
}

func (s *IngestionService) IngestInvoiceFromDocument(
    ctx context.Context,
    callerOrgID uuid.UUID,
    callerUserSub string,
    in IngestInvoiceInput,
) (IngestInvoiceResult, error)
```

### `internal/store/invoice_ingestions.go`

```go
package store

// Reuses store.ErrIdempotencyConflict (already declared in field.go).

type InvoiceIngestionStore struct{}
func NewInvoiceIngestionStore() *InvoiceIngestionStore { return &InvoiceIngestionStore{} }

type InsertInvoiceIngestionParams struct {
    ProjectID      uuid.UUID
    OrgID          uuid.UUID
    IdempotencyKey uuid.UUID
    InvoiceID      uuid.UUID  // FK to the invoice created earlier in the SAME tx
    FeedCardID     uuid.UUID  // FK to the review card created earlier in the SAME tx
    ExtractedBy    uuid.UUID  // resolved users.id of the caller (sub)
}

// InsertInvoiceIngestion claims (project_id, idempotency_key) by INSERT. On the
// UNIQUE violation it returns store.ErrIdempotencyConflict (mapped from SQLSTATE
// 23505 via the existing isUniqueViolation helper). It does NOT read anything
// back — the conflict is the LAST statement the caller runs in the tx, so the tx
// rolls back cleanly (no 25P02 poisoning). See §6.
func (s *InvoiceIngestionStore) InsertInvoiceIngestion(ctx context.Context, tx pgx.Tx, p InsertInvoiceIngestionParams) error
```

### `internal/store/financials.go` (edit)

```go
type CreateInvoiceParams struct {
    ProjectID     uuid.UUID
    OrgID         uuid.UUID
    VendorName    string
    InvoiceNumber *string
    AmountCents   int64
    CurrencyCode  string
    WBSCode       *string
    DueDate       *time.Time
    Source        *string // NEW: 'manual' | 'ai_ingest'; nil → column DEFAULT 'manual'
}
// INSERT column list gains `source`, $9. Manual callers pass nil → DEFAULT applies.
```

---

## 3. End-to-end ingestion flow (document → invoice + review card + audit, ONE tx, idempotent)

```
POST /api/v1/projects/{projectID}/invoices/ingest        (Owner|Admin, JSON body)
```

**Handler (`internal/api/invoice_ingest.go`) — no tx, pre-service:**
1. Decode JSON `{ idempotency_key, document_url?, text?, currency_code?, wbs_code? }`.
2. Validate: `idempotency_key` parses as UUID (else **400**); exactly one of `document_url`/`text`
   is non-empty (else **400**).
3. Pull `callerOrgID` + `callerUserSub` from JWT claims (`callerOrgIDFromClaims`). Parse `projectID`
   from URL.
4. Build `IngestInvoiceInput`; call `IngestionService.IngestInvoiceFromDocument`.
5. Map result → HTTP (§5.4).

**Service `IngestInvoiceFromDocument` — fuzzy phase OUTSIDE the tx, then ONE write tx:**

6. **Fuzzy: AI extract (no tx open, no DB locks held).**
   `aiCtx := ai.ContextWithOrgID(ctx, callerOrgID.String())`, then
   `resp, err := s.ai.InvoiceExtract(aiCtx, ai.InvoiceExtractRequest{DocumentURL: in.DocumentURL, Text: in.Text})`.
   - `s.ai == nil` (no AI wiring) **or** `errors.Is(err, ai.ErrUnconfigured)` → return `ai.ErrUnconfigured`
     wrapped → handler **503**. **Nothing was written; no tx to roll back.** Graceful degradation.
   - `ai.ErrUnsupportedMediaType` / `ai.ErrImageTooLarge` / fetch-404 / decode error → wrap → **422/502**
     as appropriate. Still no DB state.
7. **Exact: deterministic validation/coercion (the trust boundary — §7).** Validate **all** AI-sourced
   fields, not just money: vendor non-empty, currency in {USD,CAD}, total > 0, line-sum reconciliation
   (§7.3), issued_date parse-or-drop (§7.4). Any hard failure → `ErrInvoiceExtractionInvalid` → **422**.
   No DB write.
8. **Open the write tx** (`pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx) error { ... })`):
   1. `store.VerifyProjectInOrg(ctx, tx, projectID, callerOrgID)` — cross-tenant guard (→ `ErrNotFound`/404).
   2. **Create invoice** via the *validated* path. Two equivalent implementations (pick one, §7.5):
      either call `s.budget.createInvoiceTx(ctx, tx, …, Source=ptr("ai_ingest"))` (an extracted tx-body
      helper that re-applies the vendor/amount/currency invariants), or replicate **all three**
      invariants inline then `s.finStore.CreateInvoice(tx, …, Source: ptr("ai_ingest"))`. The invoice
      lands `status='pending'` via table DEFAULT — **AI never auto-approves money.**
   3. **Create review feed card** `s.feedStore.CreateFeedCard(ctx, tx, store.CreateFeedCardParams{
      OrgID: callerOrgID, ProjectID: &projectID, CardType: "invoice_review",
      Title: "Review invoice from <vendor>: <invoice_no>", Body: <human summary>,
      Priority: <normal | urgent on mismatch>, TargetRole: ptr("admin"),
      Actions: <JSONB [{approve_invoice, invoice_id},{reject_invoice, invoice_id}]> })`.
      **`OrgID` is mandatory** (non-pointer, NOT NULL) — do not omit it.
   4. **Claim the idempotency slot LAST** (this is the dedupe enforcement point, §6):
      `err := s.ingStore.InsertInvoiceIngestion(ctx, tx, InsertInvoiceIngestionParams{ ProjectID,
      OrgID, IdempotencyKey, InvoiceID: inv.ID, FeedCardID: card.ID, ExtractedBy })`.
      - On `store.ErrIdempotencyConflict` → **return it from the tx fn**. The whole tx (invoice + card)
        rolls back. Because the conflicting INSERT is the **last** statement, the tx is not used again —
        no `25P02`. Service surfaces `store.ErrIdempotencyConflict`; handler → **409 bare** (§5.4, §6).
   5. **Audit** `s.audit.Record(ctx, tx, AuditEntry{ OrgID: callerOrgID, UserSub: callerUserSub,
      Action: "ingestion.invoice.extracted", ResourceType: AuditResourceInvoice, ResourceID: inv.ID,
      After: marshalAudit(inv), Metadata: marshalAudit({vendor, ai_total_cents, persisted_total_cents,
      currency, line_item_count, total_mismatch, source:"ai_ingest", idempotency_key}) })`.
      `ResourceID` is the real `inv.ID` (non-nil) so `audit.Record` does not skip it.
9. **Commit.** Invoice + card + outbox + audit land **all-or-nothing**.
10. Return `IngestInvoiceResult{Invoice, ReviewCard}` → handler **201** `{ data: { invoice, review_card } }`.

**Re-submit (same `idempotency_key`):** step 8.4 hits the UNIQUE → tx rolls back → **409**, no new rows,
no duplicate card, no duplicate audit. (Note the honest cost: a same-key replay still pays for the AI
call in step 6 because extraction precedes the claim — see §6 "known limitation" and §9 risk 3.)

---

## 4. Trigger + RBAC

- **Trigger (2a, the only one):** synchronous `POST /api/v1/projects/{projectID}/invoices/ingest`,
  **JSON** body. No River job in 2a.
- **RBAC:** mounted **inside the existing `/invoices` subrouter** in `router.go`, which already does
  `r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))`. So ingest inherits the **exact** gate as manual
  `CreateInvoice` — AP is office staff; superintendents/field workers are excluded. One added line:
  `r.Post("/ingest", financials.IngestInvoice)`.
- **SetupGate / auth:** unchanged — route is under `/api/v1/projects/...`, already behind auth + the setup gate.

---

## 5. Transport, response codes, and the PDF/multipart deferral

### 5.1 Why JSON, not multipart
`ai.InvoiceExtract` consumes **only** `DocumentURL` (a signed URL the AI client fetches itself) or
inline `Text` — it has **no raw-bytes path**. A multipart upload endpoint would additionally need
(a) byte→signed-URL staging (there is **no object-storage seam** in the repo today — `PhotoAssetID`s
are metadata-only) and (b) PDF→image rasterization (the fetch path accepts **only** `image/jpeg|png|gif|webp`;
verified in `internal/ai/image.go`). Both are **new dependencies not in `TECH_STACK.md`**. 2a uses the
transport the existing task already supports and **defers raw upload + PDF to 2b**.

### 5.2 Deferred to 2b (documented, not silently dropped)
- Raw file upload (multipart, `FileUploadMaxBodyBytes` 25 MiB cap) + object-storage staging seam.
- PDF→image rasterization.
- The async River trigger (`IngestInvoiceArgs` + worker) — this is the trigger that **promotes the
  agentic port** (§11).
- Line-item persistence as relational rows (§7.6).
- The wired approve/reject **action handler** for the review card — 2a *creates* the card; flipping
  `pending→approved` already exists via `PUT /invoices/{invoiceID}` (status), which the card's actions
  deep-link to. (Call this out: 2a surfaces the review item; the existing update endpoint actions it.)

### 5.3 Request body (2a)
```json
{
  "idempotency_key": "<uuid>",          // required
  "document_url": "https://.../inv.png", // XOR text; signed URL to jpeg/png/gif/webp
  "text": "...",                         // XOR document_url
  "currency_code": "USD",                // optional operator override (must match AI or 422)
  "wbs_code": "03-30-00"                 // optional
}
```

### 5.4 Response codes
| Code | When |
|---|---|
| `201` | success — `{ data: { invoice, review_card } }` |
| `409` | idempotency replay (same key) — **bare** error body, field-sync parity (§6) |
| `422` | extraction invalid (bad currency, total ≤ 0, vendor empty, line-sum mismatch beyond tolerance) **or** unsupported media type |
| `400` | bad JSON, missing/invalid `idempotency_key`, neither-or-both of `document_url`/`text` |
| `503` | AI unconfigured (`ai.ErrUnconfigured`) — pipeline degraded, nothing written |
| `502` | AI transport error (timeout, 5xx — non-circuit) |
| `503` | AI circuit-open (breaker open, remaining window in Retry-After) |

---

## 6. Idempotency design + exact enforcement point

- **Key:** client-supplied `idempotency_key` (UUID) in the request body — the **proven house pattern**
  (`task_progress`/`crew_checkins`/`daily_logs` in `internal/store/field.go`, `ErrIdempotencyConflict`
  → 409). Client-supplied (not content-derived) so a deliberate re-OCR of a corrected scan uses a fresh
  key while a network retry reuses the same one.
- **Where enforced (authoritative):** a DB `UNIQUE (project_id, idempotency_key)` constraint on the new
  `invoice_ingestions` outbox table, checked by `InsertInvoiceIngestion` **inside the write tx, as the
  LAST statement** (step 8.4). Two concurrent same-key submits both extract, both build invoice+card,
  both try to claim — exactly one wins the UNIQUE; the loser's **entire tx rolls back** (invoice + card
  never commit). **No duplicate invoice can ever commit.**
- **No in-tx read-back (the fix for the runner-up's fatal flaw).** On conflict we return immediately and
  let the tx roll back, exactly like `field.go`. We do **NOT** SELECT the prior row in the poisoned tx
  (that would hit `25P02`). The 409 body is **bare** (matches field-sync; `field.go` returns
  `writeErrorResponse(..., "idempotency key already used")` with no record echo).
  - *Timeout-recovery note:* a client whose connection dropped after commit can re-`GET /invoices`
    (existing list endpoint) to find the created row, or re-`POST /ingest` with the same key and read
    the 409 as "already ingested." We deliberately **do not** add a 409-with-body in 2a (it would
    require either the impossible in-tx read or a second pre-check tx — both rejected). If field
    experience shows recovery is painful, add a `GET /invoices?idempotency_key=` lookup in 2b.
- **Outbox, not a UNIQUE on `invoices` directly:** the `invoices` table has **no** natural unique
  business key — duplicate vendor/amount/date are *legitimate* (split invoices), and the manual
  `CreateInvoice` path must stay unconstrained. The outbox isolates *ingestion* dedupe from the
  *invoice* domain.
- **Known limitation (state it in the API doc):** the key dedupes the **request**, not the **document**.
  A client that sends a fresh key per submit of the *same physical invoice* will pay for a second AI call
  and create a second `pending` invoice. The **human review card is the backstop** for true vendor-level
  duplicates. A content-hash second dimension or a `(vendor_name, invoice_number)` soft-duplicate signal
  is a **2b option**, explicitly out of scope here.

---

## 7. Money integrity + validation of AI-extracted fields (no floats, no blind totals)

The `ai.InvoiceExtractResponse` is **fuzzy, untrusted input** even though `TotalCents`/`AmountCents`/
`UnitCents` arrive typed as `int64`. The deterministic gate runs in step 7 **before any write**:

### 7.1 Currency
`currency.Validate(resp.CurrencyCode)` — reject anything but USD/CAD → `ErrInvoiceExtractionInvalid`.
If `in.CurrencyOverride` is set, the AI value **must equal** it (else 422 — no silent coercion, no
cross-currency math). Note the AI field is `invoice_currency_code` (JSON tag); the Go field is
`CurrencyCode` — map it correctly.

### 7.2 Total
Require `resp.TotalCents > 0` (matches `BudgetService.CreateInvoice`'s `amount_cents must be positive`).
Pure `int64` comparison; **no `float64` anywhere.**

### 7.3 Line-item reconciliation (deterministic, integer-only — uses the RIGHT currency API)
Sum line amounts by **folding with `currency.Money.Add`** (which raises `currency.ErrCrossCurrency` on a
mixed-currency fold) — **not** `currency.SumByCurrency` (that buckets by code and silently drops
empty-currency entries; it does **not** raise `ErrCrossCurrency`). Each line inherits the invoice's
validated currency.
- **Only sum `AmountCents`.** Do **not** recompute `quantity × unit_cents == amount_cents` — that is
  AI arithmetic you'd be re-trusting, and the AI schema makes `quantity`/`unit_cents` **optional**
  (default 0), so a legit line can have them zero. Summing amounts is the only sound check.
- **Policy on mismatch (the money-correctness judgment — flag for owner sign-off, §9 risk 4):** when
  line items are present and `Σ AmountCents != TotalCents`:
  - within a small fork-configured tolerance (default **0** cents, i.e. exact) → persist `TotalCents`,
    set `total_mismatch=true` in audit metadata, **bump the review card priority to `urgent`**.
  - beyond tolerance → **reject 422** (`ErrInvoiceExtractionInvalid`), nothing persisted.
  - when `line_items` is empty → trust `TotalCents` (still `>0`, currency-checked). Real invoices have
    tax/shipping lines the model may not fold; empty-or-omitted lines must not 422 a valid invoice.
- **The persisted `amount_cents` is always the deterministically validated integer** — never a raw AI
  field copied through unchecked, never an AI sum.

### 7.4 Non-money AI fields (closes the exact/fuzzy leak the critique caught)
- **VendorName:** reject empty → 422 (the column is `TEXT NOT NULL`; `""` would satisfy NOT NULL and
  create garbage). This invariant **must** be applied — the manual path enforces it
  (`budget.go`: `vendor_name is required`), so the ingest path must too (see §7.5).
- **InvoiceNo:** persisted to `invoice_number` (nullable). Empty → store NULL.
- **IssuedDate:** parse `YYYY-MM-DD`; on parse failure, drop it (do not map issued→due) and note
  `issued_date_unparsed` in audit metadata. **The `invoices` table has no `issued_date` column** (only
  `due_date`/`paid_date`); 2a **discards issued_date** (documented data loss, like line items). Do not
  shove it into `due_date`.

### 7.5 Route through the validated path — do NOT bypass `BudgetService`'s invariants
The critique's single biggest finding: calling `store.CreateInvoice` directly **skips**
`BudgetService.CreateInvoice`'s vendor/amount/currency guards. **Resolution (pick one, both acceptable):**
- **(a) Preferred:** extract `BudgetService.CreateInvoice`'s tx body into an unexported
  `createInvoiceTx(ctx, tx, callerOrgID, callerUserSub, in CreateInvoiceInput) (models.Invoice, error)`
  that does the three validations + `store.CreateInvoice` + audit, and have **both** the public
  `CreateInvoice` (opens its own tx) and `IngestionService` (passes its existing tx) call it. This keeps
  exactly **one** money-validation chokepoint.
- **(b) Acceptable fallback:** replicate **all three** invariants (currency, vendor non-empty, total > 0)
  inline in `IngestInvoiceFromDocument` before `store.CreateInvoice`. Riskier (drift), but works if (a)'s
  refactor is deemed too broad for the chunk.

Add `Source *string` to `CreateInvoiceInput` and thread it to `store.CreateInvoiceParams.Source`
(`"ai_ingest"` for ingest, `nil`→DEFAULT `'manual'` for the existing path). **This changes the struct
the manual path consumes** — update its one call site (the field defaults to nil, behavior unchanged).

### 7.6 Line items deferred
The schema models only the rolled-up total (no `invoice_line_items` table). 2a persists the validated
total; line-item detail is used **only** for the §7.3 reconciliation and then captured in audit
`Metadata` (`line_item_count`, `total_mismatch`). A relational line-items table (each row
`amount_cents BIGINT` + `currency_code VARCHAR(3)` to satisfy linter rule 2) is a later chunk if budget
roll-ups ever need per-line cost-code attribution.

### 7.7 Rendering (no float in business code)
`currency` has **no** `Format`/`Display` helper (verified). For the card `Body` and audit, format the
amount with a **local helper that does integer formatting** (e.g. `fmt.Sprintf("%s %d.%02d", code,
cents/100, cents%100)` — pure integer division/modulo, no `float64`). Do **not** write
`float64(cents)/100` anywhere. (If a shared formatter is wanted, a `currency.Format(cents int64, code
string) string` is a clean, in-scope addition with its own test — optional, not required for 2a.)

---

## 8. Isolation + exact/fuzzy compliance (how the gate stays green)

### 8.1 Isolation — nothing added to `internal/agentic`
`scripts/check-isolation.sh` enforces **two** things: (Check 1) `internal/physics`+`internal/currency`
must not transitively import `internal/agentic`; (Check 2) `internal/agentic` imports no other
`internal/*` (incl. TestImports). **2a touches neither package.** The new code lives in `service`/`store`/
`api` — layers that *already* legitimately import `ai`/`store`/`currency` (e.g. `service/agentic.go`,
`service/budget.go`). The dependency arrow stays inward. **`make lint-isolation` has nothing new to
evaluate and stays green.**

### 8.2 Why service-layer AI ingestion is legitimate isolation, not a loophole
The agentic leaf gate reserves `internal/agentic` for **cross-module orchestrated reasoning** (the AI
harness must not entangle the deterministic engine, and vice versa). A **linear single-write** ingestion
is correctly a `service` concern — `internal/ai` is a normal internal package `service` consumes
directly (the pre-Phase-1 `DailyBriefing`/`InvoiceExtract` call sites did exactly this). The
authoritative engines (`physics`/`currency`) are untouched and uninvolved (invoices don't feed the CPM
schedule), so there is no determinism surface to protect here **beyond money — and money is gated**
(§7).

### 8.3 Exact/fuzzy — enforced behaviorally, on the tx boundary
Everything **before** `BeginTxFunc` is fuzzy (the AI extract). Everything **inside** the tx is
deterministic (validate **all** AI fields → `CreateInvoice` → card → outbox claim → audit). **The AI
never reaches the database; deterministic code re-derives and validates every AI-sourced value that
lands in a NOT-NULL or money column before any write.** The consumer-side `invoiceExtractorAI` interface
keeps the service decoupled from the concrete client and makes soft-fail testable.

### 8.4 Escalation resolution (the harness-vs-service fork)
The fork — "does the VISION 'harness ingestion role' mandate `internal/agentic` as the home for *all*
AI ingestion?" — is **resolved in favor of the service home for 2a**, on the grounds in §0/§8.2: the
isolation contract permits it, and the orchestrator machinery is unearned for a one-call/one-write flow.
**This is a conscious, documented call, not an improvisation** (VISION's protocol bar). The path to the
harness home is real and mechanical (§11), so the decision is reversible at near-zero cost if 2b's second
trigger (or owner direction) calls for it. If the owner wants it in the harness now, that is a one-line
escalation to `ESCALATION_LOG.md` and §11 becomes the 2a plan instead.

---

## 9. Migration (needed — minimal, additive, linter-clean)

Latest migration is `013_drop_a2a` (verified), so this is **`014`**. Two additive changes; passes all
5 migration-linter rules.

```sql
-- migrations/014_invoice_ingestions.up.sql

-- Idempotency outbox for AI invoice ingestion. Isolates ingestion dedupe from
-- the invoices domain so the manual-entry path stays unconstrained.
CREATE TABLE invoice_ingestions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id),
    org_id          UUID NOT NULL REFERENCES organizations(id),
    idempotency_key UUID NOT NULL,
    invoice_id      UUID NOT NULL REFERENCES invoices(id),
    feed_card_id    UUID NOT NULL REFERENCES feed_cards(id),
    extracted_by    UUID NOT NULL,               -- users.id of the caller; no FK (cross-binary/author convention, see note)
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, idempotency_key)         -- THE idempotency anchor
);

-- Distinguish AI-ingested invoices from manual entry in review.
ALTER TABLE invoices ADD COLUMN source TEXT NOT NULL DEFAULT 'manual';  -- 'manual' | 'ai_ingest'
```

```sql
-- migrations/014_invoice_ingestions.down.sql
-- buildos:destructive: rollback of 2a invoice-ingestion outbox + invoices.source provenance column
ALTER TABLE invoices DROP COLUMN source;
DROP TABLE invoice_ingestions;
```

**Linter compliance (the 5 rules):**
1. *Forbidden money types:* none — no `_cents` column on the new table, no DECIMAL/FLOAT/etc. ✓
2. *`_cents` needs sibling `currency_code`:* N/A — `invoice_ingestions` has no `_cents` column. ✓
3. *Paired up/down:* both files present. ✓
4. *Destructive opt-in:* the `.down.sql` carries `-- buildos:destructive:` for its `DROP COLUMN` +
   `DROP TABLE`. ✓
5. *`CREATE INDEX CONCURRENTLY`:* **no `CREATE INDEX`** — the `UNIQUE (...)` is an **inline table
   constraint** on a table freshly created in the same migration (its backing index is created
   non-concurrently by definition, which is fine on a brand-new empty table). Rule 5 applies to
   standalone `CREATE INDEX`, not inline constraints. ✓

**Notes:** `source NOT NULL DEFAULT 'manual'` back-fills existing rows as manual (correct; forward-only).
`org_id` is denormalized (derivable from `project_id`) but matches the repo's per-query org-filter
convention. `extracted_by` is a bare UUID with no FK — consistent with how author/sub UUIDs are stored
elsewhere; document the choice. If the implementer prefers, FK it to `users(id)` (additive, harmless).

---

## 10. Ordered implementation task breakdown (bottom-up; each step compiles)

> Build inner→outer so every stage compiles before the next depends on it. Tasks 1–6 are largely
> non-conflicting (different files); 7–9 are the wiring + verification tail.

1. **Migration 014** — write `014_invoice_ingestions.{up,down}.sql`. Run `make lint-migrations` +
   `make migrate` (local PG) to confirm it applies and rolls back. *(No Go yet.)*
2. **Store: provenance column** — add `Source *string` to `store.CreateInvoiceParams` and the
   `CreateInvoice` INSERT (`source`, $9). Update the **one** manual call site
   (`BudgetService.CreateInvoice` → `store.CreateInvoiceParams`) to pass `nil`. `go build ./internal/store/...`.
3. **Store: outbox** — new `internal/store/invoice_ingestions.go`: `InvoiceIngestionStore`,
   `InsertInvoiceIngestionParams`, `InsertInvoiceIngestion` (INSERT; map `isUniqueViolation` →
   `store.ErrIdempotencyConflict`; **no read-back**). `go build ./internal/store/...`.
4. **Service: validated create chokepoint** — refactor `BudgetService.CreateInvoice` per §7.5(a): extract
   `createInvoiceTx`; thread `Source` through `CreateInvoiceInput`. Confirm existing budget tests still
   pass (`go test ./internal/service/... -run TestBudget`).
5. **Service: ingestion** — new `internal/service/invoice_ingest.go`: `invoiceExtractorAI`,
   `ErrInvoiceExtractionInvalid`, `IngestionService`, `NewIngestionService` (typed-nil guard),
   `IngestInvoiceFromDocument` (steps 6–10 of §3, validation per §7, integer-only amount formatter for
   the card body). `go build ./internal/service/...`.
6. **Service unit tests** — `invoice_ingest_test.go`: fake `invoiceExtractorAI` returning canned
   responses + an `ai.ErrUnconfigured` case. Cover: happy mapping; empty vendor → 422; bad currency →
   422; total ≤ 0 → 422; line-sum exact-match persists `TotalCents`; line-sum mismatch (beyond tolerance)
   → 422; mismatch within tolerance → persist + `total_mismatch` + urgent priority; `ai.ErrUnconfigured`
   → `ai.ErrUnconfigured` (no panic, no write). `go test ./internal/service/...`.
7. **API handler** — new `internal/api/invoice_ingest.go`: decode JSON, validate key + XOR, call service,
   map errors (§5.4). Add the `IngestionService` (or a `BudgetServicer`-style narrow interface) to the
   handler struct. `go build ./internal/api/...`.
8. **Router wiring** — add `r.Post("/ingest", financials.IngestInvoice)` inside the existing `/invoices`
   subrouter in `router.go`; construct `IngestionService` in `cmd/server/main.go` and pass it to the
   handler. `go build ./...`.
9. **Integration test + gates** — `invoice_ingest_integration_test.go` (`//go:build integration`, §11
   criteria). Run `make lint-isolation`, `make audit`, `make test-integration`.

---

## 11. Deferred-port seam (where 2b promotes the agentic harness)

When the **second trigger** (River job / webhook / bulk) or batch-reconciliation reasoning lands in 2b,
promote — a **mechanical lift** that does **not** touch the money chokepoint (§7.5), the outbox
(`invoice_ingestions`), or migration 014:

1. Add domain-neutral types to `internal/agentic` (new `ingestion.go`, leaf): `ExtractedInvoice`,
   `ExtractedLineItem`, `IngestInvoiceInput`, `IngestInvoiceResult`, and ports `DocExtractor`
   (`ExtractInvoice(ctx, DocumentRef) (ExtractedInvoice, error)`) + `DocIngestWorkspace`
   (`ApplyIngestedInvoice(ctx, in, ExtractedInvoice) (IngestInvoiceResult, error)`), plus
   `ErrExtractorUnavailable` (mirrors `ErrReasonerUnavailable`).
2. Add `const IngestInvoice Capability = "ingest_invoice"` + a registry `Descriptor`, and
   `Orchestrator.RunIngestInvoice` (capability gate → extract (soft-fail) → apply).
3. Move `IngestInvoiceFromDocument`'s fuzzy half behind a `DocExtractor` adapter and its tx body behind a
   `DocIngestWorkspace` adapter in `internal/service` — the code already exists; it slides behind the
   ports. `invoiceExtractorAI` becomes the adapter's internal seam.
4. Soft-fail policy for the dual caller: the orchestrator returns the `ErrExtractorUnavailable` sentinel;
   the **HTTP handler maps it to 503**, the **River worker swallows it to `return nil`** (a successful
   no-op, so River does not retry a missing-key condition). Document this at both call sites.

Until then, "reusable ingestion seam" means the **pattern + outbox shape + validation chokepoint**, not
shared types — accurate, not oversold.

---

## 12. Verification criteria (definition of done)

### 12.1 Integration test (`internal/service/invoice_ingest_integration_test.go`, ephemeral PG)
Using `testdb.NewPool(t)` (freshly migrated pool, auto-cleanup):
- **`TestIngestInvoice_DocumentToInvoiceCardAudit`** — seed org+project; call
  `IngestInvoiceFromDocument` with a **fake** `invoiceExtractorAI` returning a valid `InvoiceExtractResponse`
  (vendor, invoice_no, USD, positive total, line items summing to the total). Assert:
  - exactly **one** `invoices` row, `status='pending'`, `source='ai_ingest'`, `amount_cents` ==
    validated total, `currency_code='USD'`;
  - exactly **one** `feed_cards` row, `card_type='invoice_review'`, `target_role='admin'`, actions JSONB
    carrying the `invoice_id`;
  - exactly **one** `invoice_ingestions` row linking the invoice + card;
  - an `audit_log` row `action='ingestion.invoice.extracted'`, `resource_type='invoice'`,
    `resource_id=invoice.id`.
- **`TestIngestInvoice_Idempotent`** — call twice with the **same** `idempotency_key`; second call returns
  `store.ErrIdempotencyConflict`; assert still exactly **one** invoice, **one** card, **one** outbox row,
  **one** audit row (no duplicates).
- **`TestIngestInvoice_SoftFailNoKey`** — construct `IngestionService` with a **nil** `*ai.Client` (or a
  fake returning `ai.ErrUnconfigured`); assert the call returns (an error that `errors.Is`)
  `ai.ErrUnconfigured`, and assert **zero** `invoices` / `feed_cards` / `invoice_ingestions` rows were
  written (no partial state).
- **`TestIngestInvoice_RejectsBadFuzzyOutput`** — empty vendor and bad currency each return
  `ErrInvoiceExtractionInvalid`; assert zero rows written.

### 12.2 Unit tests
Per task 6 (§10) — validation matrix + field mapping (`invoice_currency_code`→`CurrencyCode`) +
soft-fail, all with a fake extractor, no DB.

### 12.3 Hard gates — all must stay green
- `make lint-isolation` — **green** (nothing added to `internal/agentic`; core untouched).
- `make audit` — **green** (`lint-migrations` + `lint-migrations-test` + `test` + `test-prod` +
  `bench-physics`). Migration 014 passes the 5 rules (§9); physics benches are unaffected (no
  `internal/physics` change).
- `make test-integration` — **green** (the four integration tests above + existing suite).
- Composite Currency: **no `float64` in the ingestion path** — grep the new files for `float64` and
  confirm only integer formatting (§7.7); the persisted `amount_cents` is the validated integer.

### 12.4 Manual smoke (optional, post-merge)
With a fork that has an Anthropic key configured: `POST /invoices/ingest` with a signed image URL as an
owner/admin → 201 with invoice + review card; re-POST same key → 409; POST as a field worker → 403; POST
on a fork with no key → 503.

---

## 13. Top risks (carry into ultracode/ultrareview)

1. **JSON `document_url`/`text`-only transport is a scope perception risk, not a correctness one.** Real
   AP is mostly PDF; raw upload + PDF rasterization + object-storage staging are **2b** (new deps, must
   clear `TECH_STACK.md`). 2a proves extraction+persistence value with the only transport
   `ai.InvoiceExtract` supports today. *State this plainly to the owner.*
2. **Synchronous Opus call holds the HTTP request open for seconds.** Acceptable for a watched
   single-doc operator flow; async River (the throughput fix) is 2b and is the trigger that promotes the
   port. A client timeout *after* commit + bare-409-on-retry means the client recovers the invoice via
   `GET /invoices` (§6 timeout note).
3. **Client-supplied key dedupes the request, not the document.** Same physical invoice under two keys →
   two `pending` invoices + a second paid AI call. The **human review card is the backstop**; content-hash
   or `(vendor, invoice_number)` soft-duplicate is a 2b option. *Document in the API spec.*

---

## Appendix A — verified ground truth (load-bearing facts checked against the code)

| Claim | Verified |
|---|---|
| Latest migration `013_drop_a2a`; next is `014` | `migrations/` listing ✓ |
| `field.go` returns `ErrIdempotencyConflict` and does **no** in-tx read-back (conflict is last act) | `internal/store/field.go` ✓ — kills the runner-up's in-tx-dedupe-after-conflict plan |
| `currency` has **no** Format/Display helper (only Validate/New/Zero/Add/Sub/SameCurrency/SumByCurrency) | `internal/currency/currency.go` ✓ — §7.7 uses integer formatting |
| `Money.Add` raises `ErrCrossCurrency`; `SumByCurrency` does **not** (buckets + drops empties) | `internal/currency/currency.go` ✓ — §7.3 folds with `Add` |
| `BudgetService.CreateInvoice` validates currency + vendor-non-empty + amount>0 | `internal/service/budget.go` ✓ — §7.5 must not bypass it |
| `store.CreateInvoice` has **no** `Source`; status DEFAULT 'pending' | `internal/store/financials.go` ✓ — §7.5 adds `Source` |
| `CreateFeedCardParams` requires non-pointer `OrgID`, `ProjectID *uuid.UUID` | `internal/store/feed_cards.go` ✓ — §3 sets both |
| `audit.Record` no-ops on `ResourceID == uuid.Nil` (and empty OrgID/Action/ResourceType) | `internal/service/audit.go` ✓ — §3 uses real `inv.ID` |
| `InvoiceExtractResponse`: int64 cents, `invoice_currency_code` JSON tag, line items require only description+amount_cents | `internal/ai/tasks.go` ✓ — §7.1/§7.3 |
| `ai.InvoiceExtract` accepts `DocumentURL` XOR `Text` only — **no raw bytes** | `internal/ai/tasks.go` ✓ — §5.1 |
| Image fetch accepts only `image/jpeg|png|gif|webp` (no PDF); `ErrUnsupportedMediaType`/`ErrImageTooLarge` | `internal/ai/image.go` ✓ — §5.1 |
| `ai.ErrUnconfigured` soft-fail (no HTTP attempt when key missing) | `internal/ai` (resolveKey) ✓ — §3 step 6 |
| `/invoices` subrouter already `r.Use(mw.RequireRole(mw.RoleOwner, mw.RoleAdmin))` | `internal/api/router.go` ✓ — §4 |
| `internal/agentic` leaf gate checks Imports **and** TestImports; core (physics/currency) closure must exclude agentic | `scripts/check-isolation.sh` ✓ — §8.1 (2a touches neither) |
