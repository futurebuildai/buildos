# HANDOFF — current state of play

This file is the per-session living state of the BuildOS branch. Update
it at the end of every working session so the next session (different
workstation, different Claude instance, you on Monday morning) lands
with full context in 60 seconds.

**Update protocol:** at the end of a session, edit the four sections
below: "Last shipped", "In flight", "Blocked", "Next up". Keep prose
short — bullet lists, not essays. Anchor to commit SHAs and PR numbers.

Companion docs:
- [CLAUDE.md](./CLAUDE.md) — architecture + conventions (slow-changing)
- [.agents/handoff/ADR-001](./.agents/handoff/ADR-001-vision-alignment.md), [ADR-002](./.agents/handoff/ADR-002-single-tenant-fork-model.md) — strategic decisions
- [.agents/handoff/PRODUCTION_READINESS_PLAN.md](./.agents/handoff/PRODUCTION_READINESS_PLAN.md) — phase plan
- [.agents/handoff/NEXT_STEPS.md](./.agents/handoff/NEXT_STEPS.md) — prioritized backlog with concrete entry points
- [docs/fork-onboarding.md](./docs/fork-onboarding.md) — per-customer fork provisioning runbook

---

## Last shipped (most recent → older)

- **2026-05-01** PR #6 [`b3...`] PII classification + Sentry BeforeSend masking. `internal/pii/` package + `obs.scrubSentryEvent`. Closes the GDPR/CCPA gap on Sentry egress.
- **2026-05-01** PR #5 [`ca36fa3`] OpenTelemetry tracing — `internal/obs/tracer.go`, brain-client `otelhttp` wrap, router middleware, log-correlation (trace_id+span_id alongside request_id).
- **2026-05-01** PR #4 [`7facfce`] `cmd/buildos-fork-init` keypair generator + `docs/fork-onboarding.md`.
- **2026-05-01** PR #3 [`29ea6fe`] SecretSource abstraction (env/file/chain) + `LoadWithSource()`.
- **2026-05-01** PR #2 [`699a64d`] Sprints 1-5 + Phase F core (44 commits — domain endpoints, Brain integration, production hardening, Dockerfile, D8 build-tag hardening).

5 PRs merged in one session under the L8 self-audit gate. Every
landed commit had `make audit` green + integration suite green +
govulncheck clean.

## In flight

Nothing on a branch waiting for review right now. The session ended
with all branches deleted (merged) except whatever the next session
opens.

## Blocked

- **GitHub Actions workflows** (`.github/workflows/{ci,release}.yml`)
  still un-pushed. The token Claude Code is using doesn't have GitHub's
  `workflow` scope. The YAML is correct + tested locally; ship it via
  one of:
  - GitHub UI: paste the YAML through "Add file → Create new file"
  - Local clone: push from a token that has `workflow` scope
  - Refresh: `gh auth refresh -h github.com -s workflow` then re-push
  Files exist locally between sessions if you keep the worktree alive;
  if not, they're at the bottom of this doc as an appendix.

## Next up (prioritized — pick from the top)

See [.agents/handoff/NEXT_STEPS.md](./.agents/handoff/NEXT_STEPS.md)
for the full prioritized backlog with entry-point file paths.

Top three an L8 PE would queue:

1. **Audit-log JSONB scrub** — same `internal/pii` package, different
   egress. Wrap audit insert with `pii.ScrubJSON(blob, pii.Restricted)`.
   ~100 LOC + tests. High-leverage compliance posture for any EU fork.
2. **Structured-log PII scrubbing** — slog handler that runs
   `pii.MaskString` over attribute values whose key matches the
   catalog. Sits behind the existing `obs.CorrelatingHandler`.
3. **D7 wave 3 finish** — Schedule + Pipeline service audit recording.
   Pattern is established (Fleet + Procurement already wired); copy
   the shape. Half a session.

## Working agreement (L8 self-audit gate)

The owner delegated the merge gate to Claude when they said "merge
any commits should they pass your L8 quality and scope alignment
gate." This means a session can:

1. Open a branch
2. Ship work to L8 quality
3. Self-audit (build + vet + lint + test + integration + govulncheck
   + composite-currency lint + bench-physics; per-commit messages
   PR-quality; backward compat preserved or breaking change documented)
4. Open a PR with a real description
5. Merge it via `gh pr merge N --merge` if the gate passes

Don't merge if any of: tests failing, vet failing, govulncheck
flagging, integration regression, composite-currency violation, or
the change has a meaningful security implication you haven't
explicitly thought through.

When in doubt, open the PR but don't merge — let the human review.

## Things to NOT do

- **Don't add Postgres RLS** (per ADR-002). Single-tenant fork model =
  tenant isolation through deployment isolation. RLS would add 3-10%
  query overhead for zero security benefit.
- **Don't enable per-tenant rate limiting**. Per-IP is enough; one
  tenant per deployment.
- **Don't use `git push origin --force` or rewrite published history**
  on main. Force-push to feature branches only when strictly necessary.
- **Don't commit secrets**. The fork-init tool writes `private.pem`
  with mode 0600; document that it goes into a secret store, not the
  repo.
- **Don't try to push `.github/workflows/*`** with the current Claude
  Code OAuth token — it doesn't have `workflow` scope. Push from a
  user clone or via the UI.
- **Don't break composite-currency invariants**. Every monetary column
  pairs `*_cents BIGINT` with `*_currency_code VARCHAR(3)`. The
  migration linter is hard CI.
- **Don't bypass D8** by adding new dev-only auth paths without the
  `//go:build !prod` tag.

## Cross-repo coordination (BuildOS ↔ The Brain)

These items live across both repos; tracked here because they're easy
to forget when working on one side only. The Brain repo lives at
`../futurebuild-brain` (per `replace` directive in `go.mod`).

| Item | Side | Status | Notes |
|---|---|---|---|
| OIDC issuer + JWKS | Brain | live | stable wire protocol; `iss="fb-brain"` `aud="fb-os"` legacy values, do not rename without coordination |
| A2A inbound webhook | BuildOS | live | `/api/v1/a2a/webhook`; JWS-verified |
| A2A outbound webhook | BuildOS | live (signing key per fork) | each fork signs with its own RSA-2048; public key registers in Brain's JWKS |
| `WebhookEvent.OrgID` field | Brain | optional today, should become required | when Brain enforces this, BuildOS continues to send it as it does now |
| Maestro Chat | Brain | live | called from `internal/service/agents.go` DailyBriefing |
| Billing usage | Brain | live | proxied at `/api/v1/billing/usage` |
| LocalBlue → Brain → BuildOS | Brain | live | `localblue.lead_captured` event handled in `internal/service/a2a.go` |
| Stripe billing engine | Brain | not yet | gating G1 |
| Vault / SecretsManager backends for SecretSource | BuildOS | env+file shipped; vault next when first customer fork needs it | `internal/config/secrets.go` interface ready |

---

## Appendix: workflow YAML (if not in worktree)

If the `.github/workflows/{ci,release}.yml` files aren't in your
checkout, they were generated in [PR #5's predecessor session]. The
content is captured in the commit message of branch
`ci/github-workflows` (now deleted) — recoverable via `git reflog`
or reconstructable from this doc's recipe. Recipe:

**`.github/workflows/ci.yml`** mounts six jobs on every PR + push to main:
- lint-migrations (5 rules) + linter regression suite
- gofmt + go vet (both default and `-tags=prod`)
- govulncheck CI-blocking (both build modes)
- unit tests + prod-mode test
- integration tests (Testcontainers + Postgres)
- bench-physics (CPM gate)
- multi-arch docker build (linux/amd64 + linux/arm64) + Trivy scan

**`.github/workflows/release.yml`** fires on tag push (`v*.*.*`):
- multi-arch image to GHCR
- cosign keyless OIDC signing (no signing key to manage)
- syft SBOM in CycloneDX + SPDX, attached via `cosign attest`
- GitHub Release with auto-changelog from `git log`
