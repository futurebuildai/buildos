# Tech Stack Configuration

This file is the single source of truth for the project's technology choices. All workflows and skills reference this file instead of hardcoding tech stacks.

**System:** BuildOS (System of Execution)

---

## Backend

- **Language:** Go 1.24
- **Framework:** stdlib net/http + chi router (go-chi/chi/v5)
- **ORM / Data Access:** pgx/v5 (jackc/pgx) — raw SQL with pgx types, no ORM

## Frontend

- **Framework:** Vite + Lit (vanilla Web Components)
- **Bundler:** Vite
- **Styling:** Vanilla CSS (CSS custom properties for design tokens)
- **Language:** TypeScript (strict mode)

## Mobile (if applicable)

- **Framework:** Flutter — field surfaces only (crew check-in, daily logs, photo capture, offline-first job site interactions)

## Database

- **Primary:** PostgreSQL 16+ with pgvector extension (semantic search, AI embeddings)
- **Cache:** Redis 7 via Asynq (background job queue + caching)
- **Search (if applicable):** pgvector (vector similarity search within PostgreSQL)
- **Message Queue (if applicable):** Redis 7 via Asynq (task queue for async workflows)

## API

- **Style:** REST
- **Spec Format:** OpenAPI 3.1
- **Auth Model:** Native email/password. BuildOS owns identity end-to-end — argon2id password hashing, its own RS256 JWT issuer/verifier (`JWT_PRIVATE_KEY_PEM`/`JWT_PUBLIC_KEY_PEM`, `iss=buildos`, `aud=buildos`), server-revocable opaque refresh tokens, and bootstrap-token first-owner claim. No external OIDC provider. Local RBAC (`owner` > `admin` > `superintendent` > `field_worker`). AI endpoints are role-gated only — the `plan_tier` gate was removed post-pivot (ESC-002), since standalone forks have no billing tiers (the `plan_tier` claim is retained, dormant, for a possible future tiering model).

## Infrastructure & Deployment

- **CI/CD:** GitHub Actions
- **Hosting:** DigitalOcean / Railway
- **Containerization:** Docker (multi-stage builds)
- **Orchestration:** Docker Compose (local dev), Railway (production)

## Developer Tooling

- **IDE:** GoLand (JetBrains)
- **Package Manager:** Go Modules (backend), npm (frontend)
- **Linter:** golangci-lint (backend), ESLint (frontend)
- **Formatter:** gofmt (backend), Prettier (frontend)
- **Task Runner:** Makefile

## Testing

- **Unit Test Framework:** go test
- **Integration Test Framework:** Testcontainers-go
- **E2E Test Framework:** Playwright
- **Coverage Tool:** go cover (backend), c8 (frontend)

## Observability

- **Logging:** structured JSON via slog (Go stdlib)
- **Tracing:** OpenTelemetry
- **Metrics:** Prometheus
- **Error Tracking:** Sentry

## AI Service Layer

- **Integration Model:** Native + BYOK. BuildOS calls the Anthropic Messages API directly (`internal/ai`) using the org's own key stored in the encrypted vault. No external AI gateway/broker. Missing/invalid key soft-fails (`503 SERVICE_UNAVAILABLE`) — the server boots without keys.
- **Primary Provider:** Anthropic — Claude Opus 4.6 (default model: reasoning, agent orchestration, complex decisions) and Claude Sonnet 4.5 (fast model: high-throughput tasks, classification, summarization)
- **Agent endpoints:** Daily briefing + schedule-adjustment recommendations, plan-tier gated. Procurement vendor review surfaces as a local feed card (no autonomous approval engine, no agent-to-agent webhooks).
- **Embeddings:** Anthropic if available; fall back to open-source (e.g., nomic-embed, BGE) for pgvector ingestion if needed
- **On-Device (Flutter):** Open-source small models permitted for offline field scenarios only (e.g., on-device OCR, photo classification)
- **Open Source Policy:** Open-source models permitted ONLY for edge/niche use cases — on-device Flutter inference, domain-specific embeddings, or capabilities Anthropic does not offer. All core intelligence routes through Anthropic.

## Constraints & Preferences

- **Currency (Composite Currency Pattern):** ALL monetary values stored as the **Composite Currency Pattern**: `amount_cents BIGINT` paired with `currency_code VARCHAR(3) DEFAULT 'USD'`. Supported currencies: USD (United States Dollar) and CAD (Canadian Dollar). No floating-point currency. Display formatting is a frontend concern only. Cross-currency arithmetic is **forbidden** — values with different `currency_code` MUST NOT be summed, compared, or subtracted. Aggregations must group by `currency_code`.
- **Currency Enforcement (CI Hard Gates):**
  - **SQL Migration Linter (CRITICAL — hard fail in GitHub Actions):** Script scans `migrations/*.sql` for: (1) forbidden types (`DECIMAL`, `NUMERIC`, `REAL`, `DOUBLE PRECISION`, `MONEY`, `FLOAT`) on columns matching monetary patterns (`cost`, `price`, `amount`, `total`, `budget`, `cents`, `fee`, `payment`, `invoice`, `balance`, `revenue`, `expense`) — any match fails CI; (2) any `amount_cents` column (or column ending in `_cents` matching monetary patterns) that lacks a corresponding `currency_code` column in the same CREATE TABLE statement — fails CI. No exemptions.
  - **Go Struct Naming Convention:** All monetary fields MUST end in `Cents` (e.g., `TotalActualCostCents`, `EstimatedPriceCents`) with a companion field ending in `CurrencyCode` (e.g., `TotalActualCostCurrencyCode`). `golangci-lint` custom rule flags `float32`/`float64` fields on structs containing monetary field names.
  - **TypeScript ESLint Rule:** Custom rule flags `number` type annotations on properties matching monetary name patterns unless the property name ends in `Cents`. Properties ending in `Cents` must have a sibling property ending in `CurrencyCode`. Enforced via `eslint-plugin-fb` in frontend lint config.
- **Numerical Typography:** JetBrains Mono for all numerical data fields in the UI.
- **AI-First Principle:** Anthropic Claude is the default AI provider across the ecosystem. Do not introduce Google Vertex, OpenAI, or other commercial LLM providers unless Anthropic cannot serve the use case. Open-source models are acceptable for edge cases only.
- **Native Identity:** BuildOS handles all authentication flows itself (claim, login, refresh, logout, password reset). It mints and validates its own RS256 JWTs and enforces local RBAC. No external identity provider.
- **Transactional Email:** Resend — password-reset delivery (`internal/mailer`). API key set in-app via the encrypted integrations vault; absent key soft-fails.
- **Standalone Deployment:** Each customer runs their own forked BuildOS repo as a self-contained spoke. No external proprietary service dependency.
- **MCP connectors (Phase 3b-ii):** the MCP Streamable-HTTP client is **hand-rolled** (JSON-RPC 2.0 + a minimal SSE reader over `net/http`) in `internal/connectors` — **NO new dependency** (no `mcp-go`/SDK). Owner-approved 2026-06-09. All outbound connector egress goes through an SSRF-guarded `http.Client` (https-only + resolve-and-pin private-IP denylist via `net.Dialer.Control` + no redirects).
- **pgvector:** Used for AI-powered features (semantic search, document similarity, recommendation). Vectors stored alongside relational data in the same PostgreSQL instance.
- **Asynq:** Redis-backed task queue for background jobs (report generation, notification delivery, AI inference pipelines). No separate message broker needed.
- **Flutter Scope:** Mobile app covers field-only surfaces. All administrative, planning, and management workflows are web-only.
