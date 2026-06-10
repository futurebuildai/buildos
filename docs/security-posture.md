# BuildOS security posture (Phase 4c gate)

Result of the Phase 4c security review — an 8-surface multi-agent audit (auth/JWT,
RBAC/IDOR, injection, secrets/crypto, SSRF/egress, headers/CORS, DoS/rate-limit,
PII/logging) with adversarial verification of every candidate, plus dependency
scans and a k6 load harness ([`scripts/k6/`](../scripts/k6/README.md)).

**Threat model:** single-tenant per customer fork (ADR-002) — tenant isolation
*is* deployment isolation. Missing multi-tenant RLS is by design and not a
finding. **No critical issues.** The two HIGH findings are fixed in code; the
rest are fixed, deployment-required, or tracked below.

## Dependency scans
- **Go:** `govulncheck ./...` → **no vulnerabilities**.
- **Web (`web/`):** the shipped bundle (`lit`, `@lit-labs/signals`) has **zero**
  known vulns. A critical happy-dom test-VM RCE was closed (bumped 15→20.10.2;
  231 web tests still green). The remaining `vite`/`esbuild` advisories are
  **dev-server-only** (not in the production build) and need a vite 5→8 major
  bump — tracked, not shipped.

## Findings + status

| # | Sev | Finding | Status |
|---|-----|---------|--------|
| H-1 | High | **SSRF** via invoice-ingest `document_url` (unguarded server-side fetch → IMDS/internal hosts) | **FIXED** — the document fetch now defaults to the SSRF-guarded egress client (private-IP/metadata denylist at the *resolved* dial address → defeats DNS-rebind; redirects refused) + an https-only scheme allowlist. `internal/ai/{client,image}.go`. Regression test `TestFetchDocumentImage_RejectsNonHTTPS`. |
| H-2 | High | **Rate-limit bypass** — `chi.RealIP` trusts `X-Forwarded-For` unconditionally, so the only per-IP brute-force defense on `/auth/login` + `/password-reset/*` is spoofable; no per-account lockout | **FIXED (XFF) + tracked (lockout)** — `mw.RealIP(trustedProxyCIDRs)` honors forwarding headers only from a configured trusted-proxy allowlist (`TRUSTED_PROXY_CIDRS`); empty default = use the real TCP peer (fail-safe). Test `TestRealIP`. Per-account lockout = tracked follow-up (the XFF fix restores IP-based limiting; see below). |
| M-1 | Med | Expensive AI endpoints (daily-briefing, chat, recommend-adjustments) have no per-(org,user) cooldown — one caller can run up the org's Anthropic bill | **Tracked** — partially mitigated by the global per-IP limiter (now spoof-resistant); a dedicated per-(org,user) AI throttle is the follow-up. |
| L-1 | Low | Login timing side-channel enables email enumeration | **FIXED** — the unknown-email / no-password paths now run argon2id against a fixed dummy hash (constant-time). `internal/service/auth.go`. |
| L-2 | Low | Missing `X-Content-Type-Options` / `Referrer-Policy` / `X-Frame-Options` | **FIXED (API)** — `mw.SecurityHeaders` sets nosniff + `Referrer-Policy: no-referrer` + `X-Frame-Options: DENY` on every API/4xx/5xx response. **NOTE:** the Go server serves no static assets — the SPA HTML/JS/CSS are served by a separate static host/reverse proxy, which **must set its own** `X-Frame-Options`/`nosniff`/**CSP** (the browser-rendered console is where clickjacking/XSS-DiD headers matter). See deployment hardening below. |
| L-3 | Low | No HSTS | **Deployment-required** — the Go server runs plain HTTP behind a TLS terminator; emit `Strict-Transport-Security` *there*. See [fork-onboarding](./fork-onboarding.md). |
| L-4 | Low | Unbounded `duration_days` → CPM per-day loop is a cheap authenticated DoS | **FIXED** — migration `019` adds a `CHECK (duration_days BETWEEN 0 AND 36500)` guarding every write path. |
| L-5 | Low | PII catalog omitted password/token/secret field names | **FIXED** — added `password`, `*_token`, `secret`, `api_key`, `private_key`, `authorization`, `jwt`, etc. at Restricted in `pii.FieldClass`. |
| L-6 | Low | Free-text field input stored unscrubbed in `audit_log.after_state` (content PII the field-name scrubber can't see) | **Tracked/accepted** — field-name scrubbing fundamentally can't catch content PII; documented. Follow-up: audit structured fields only + a hash/length of free text. |
| I-1 | Info | MCP connector errors logged the resolved IP/host unscrubbed | **FIXED** — the WARN line is now host-free; the raw error detail moved to DEBUG (off in prod). `internal/connectors/mcp.go`. |

**Verified-clean (no finding):** all SQL is parameterized (pgx, no concat); the
AES-256-GCM vault uses a fresh random nonce per seal + verifies the tag; JWT
validation pins RS256 + checks exp/iss/aud (no `alg=none`); the `DEV_AUTH_MODE`
header bypass is build-tag-gated (prod stub + fail-fast); RBAC gates every
protected route and org-scoping is per-query (no IDOR found; the agents surface
keeps its role gate post-ESC-002); a request body-size cap (`mw.MaxBodySize`)
and the per-IP limiter are mounted; no secrets in code or git history.

## Required deployment hardening (per fork)
1. **TLS + HSTS** at the ingress/terminator (the app serves plain HTTP).
2. **`TRUSTED_PROXY_CIDRS` — REQUIRED if the fork is behind a load balancer /
   reverse proxy.** Set it to the LB/ingress subnet(s) so the per-IP rate limiter
   keys on the real client IP (from `X-Forwarded-For`). **If you leave it empty
   while proxied, every client collapses into ONE shared rate-limit bucket and
   the whole org trips 429s** (the server logs a WARN at boot when it's empty).
   Leave empty ONLY for a directly-exposed fork — spoofed headers are then ignored.
3. **SPA security headers** — the static host/reverse proxy serving the web
   console must set `X-Frame-Options: DENY` (or CSP `frame-ancestors`),
   `X-Content-Type-Options: nosniff`, and a `Content-Security-Policy` on the
   HTML/JS responses (the Go server only covers the API).
4. **Network-restrict `/metrics`** to the Prometheus scraper (unauth by convention).
5. Run the **k6 harness** (`scripts/k6/`) against a staging instance; the
   `auth_login.js` flood must show 429s (limiter shedding) and zero 5xx.

## Tracked follow-ups (non-blocking)
- Per-account login lockout (store-backed failed-attempt counter + backoff,
  survives IP rotation) and a dedicated stricter throttle on the auth routes.
- A per-(org,user) rate limit on the AI endpoints (cost-abuse guard).
- Audit-log free-text PII handling (M-style: structured-only + content hash).
- The web `vite`/`esbuild` dev-server advisories (vite 5→8 major bump).
- A write-time `document_url` validator at the API boundary (reuse the connector
  `validateMCPEndpoint` shape) for a clean 400 — the dial-time egress guard is
  already the authoritative SSRF defense, so this is UX/consistency only.
- `CORS`: there is no CORS middleware — fine because the deployment must serve the
  SPA + API **same-origin** (reverse proxy) and auth is bearer-token-in-header (no
  cookies → no CSRF). If a fork ever serves the SPA cross-origin, add a strict
  allowlisted CORS config.
- Migration 019 uses a plain `ADD CONSTRAINT CHECK` (validates existing rows under
  a brief lock); for a large pre-existing fork, prefer `NOT VALID` + a separate
  `VALIDATE CONSTRAINT`. Fresh forks are unaffected.
