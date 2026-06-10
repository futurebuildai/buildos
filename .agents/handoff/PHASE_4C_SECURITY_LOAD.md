# Phase 4c — security review + load harness (FINAL Phase-4 gate)

**Status:** BUILT on `feat/phase-4c-security-load` (committed, not pushed/merged). All gates green. Awaiting owner review → merge.
Deliverables: a security-review pass (8-surface multi-agent audit + fixes) and a k6 load harness. Posture report: [docs/security-posture.md](../../docs/security-posture.md).

## What this is
The closing Phase-4 hardening gate. Two parts:
1. **Security review** — an 8-surface adversarial audit (25 agents: 8 finders → per-candidate verify → synthesis) of auth/JWT, RBAC/IDOR, injection, secrets/crypto, SSRF/egress, headers/CORS, DoS/rate-limit, PII/logging. No critical issues; **2 HIGH fixed in code**, the rest fixed / deployment-required / tracked (full table in the posture doc).
2. **Load harness** — `scripts/k6/` (smoke, field-sync, CPM-recalc, and an adversarial rate-limiter/argon2id flood) + a runbook. k6 is a test-only external CLI (not a dep); run against staging.

## Fixes landed
- **H-1 SSRF (invoice `document_url`)** — the document fetch now defaults to the SSRF-guarded `connectors.NewEgressClient` (private-IP/metadata denylist at the *resolved* dial address → defeats DNS-rebind; redirects refused) and an **https-only** scheme allowlist, kept separate from the Anthropic client. `internal/ai/{client,image}.go` + regression test.
- **H-2 rate-limit XFF spoof** — replaced `chi.RealIP` with `mw.RealIP(TRUSTED_PROXY_CIDRS)`: forwarding headers are honored ONLY from a configured trusted-proxy allowlist; empty default = the real TCP peer (fail-safe). `internal/api/middleware/realip.go` + config + test.
- **Security headers** — `mw.SecurityHeaders` (nosniff, `Referrer-Policy: no-referrer`, `X-Frame-Options: DENY`).
- **Login enumeration** — dummy-argon2id on the unknown-email/no-password paths (constant-time). `internal/service/auth.go`.
- **CPM DoS** — migration `019` caps `duration_days` (`CHECK 0..36500`) across all write paths.
- **PII catalog** — added secret-bearing field names (password/token/secret/api_key/…) at Restricted.
- **MCP error log** — host detail moved to DEBUG; the WARN line is host-free.
- **Deps** — `govulncheck` clean; closed a happy-dom (test-only) critical.

## Tracked follow-ups (documented, non-blocking)
Per-account login lockout (store-backed); a per-(org,user) AI-endpoint throttle; audit-log free-text-PII handling; the web vite/esbuild dev-server advisories (vite 5→8); **deployment-required:** TLS+HSTS at the terminator, `TRUSTED_PROXY_CIDRS`, network-restrict `/metrics`. See the posture doc.

## Max-effort code review (`/code-review max`, 9 angles + verify + sweep)
The SSRF fix was independently verified **complete** (a finder ran probe tests: the dial-time guard defeats DNS-rebind, decimal/octal-IP, userinfo, and redirect vectors). **8 findings fixed:** the **AI duration adjuster** missed the new upper-bound (a hallucinated `>36500` would roll back the whole apply batch) → symmetric skip + a `36500` const + regression test; the **login timing-equalizer** silently degraded if startup hashing failed → `NewAuthService` now fails fast; **RealIP** could honor a forged `X-Real-IP` when XFF was present-but-all-trusted → falls back to the peer + 2 new tests (all-trusted-XFF, IPv6); the **k6 `auth_login` threshold** was unsatisfiable (global `checks` rate) → scoped per-check. **Operational HIGH (documented, deployment-required):** dropping `chi.RealIP`'s unconditional XFF trust means a fork **already behind an LB** that upgrades without `TRUSTED_PROXY_CIDRS` collapses all clients into one rate-limit bucket → a **boot WARN** + the posture doc now marks `TRUSTED_PROXY_CIDRS` **REQUIRED for proxied forks**. Also clarified the security headers cover the **API only** (the SPA static host must set its own X-Frame-Options/CSP/nosniff). A write-time `document_url` validator was prototyped then reverted (it blocked the loopback test harness, and the dial-time guard is already authoritative) — tracked as a UX nicety.

## Gates
`make audit` ALL PASSED (incl. lint-migrations on 019 + test-prod + bench) · `make lint-isolation` (ai→connectors edge ok) · build/vet (default/prod) · `make test-integration` (migration 019 applies) · new unit tests (RealIP spoof-resistance, https-only doc fetch) · web typecheck/test/build green · k6 scripts authored.

## Definition of done
- [x] Security audit (8 surfaces, verified) + posture report · [x] 2 HIGH + cheap medium/low fixed; rest documented · [x] k6 load harness + runbook · [x] gates green · [ ] `/code-review` (owner) · [x] HANDOFF/NEXT_STEPS updated. **This closes Phase 4.**
