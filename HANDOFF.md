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

- **2026-05-04** PR #11 [`e9edb96`] Tier 1 #2: slog Restricted-class PII attribute scrub via `obs.CorrelatingHandler`. `Handle` rebuilds the `slog.Record` to mask attrs whose key matches `pii.FieldClass` at Restricted (email, phone, gps_*, oidc_subject, ip_address, etc.). Group attrs recurse. `WithAttrs`-baked attrs are also scrubbed so long-lived loggers can't smuggle PII. Confidential and below pass through unchanged (vendor names, *_cents, request_id/trace_id/span_id retained for triage). 6 new test functions (~14 catalog cases). Completes the 3-leg PII-egress trio: Sentry (PR #6) + audit JSONB (PR #10) + slog (PR #11).
- **2026-05-04** PR #10 [`92231c0`] Tier 1 #1: audit-log JSONB Restricted-class PII scrub. `AuditStore.InsertAudit` now wraps Before/After/Metadata in `pii.ScrubJSON(blob, pii.Restricted)` before INSERT. Confidential-class fields (vendor, *_cents, project) intentionally preserved for investigative value. 5 unit tests + 1 integration round-trip test.
- **2026-05-04** PR #9 [`6119ab3`] CI + release workflows activated at `.github/workflows/` (5 commits: relocation, gofmt sweep across repo, Trivy action `v0.36.0` real-tag pin, Dockerfile alpine bump 3.20→3.22). All 6 CI jobs green on first activation. Plus L8 SRE audit gate added to PR description protocol.
- **2026-05-04** PR #8 [`chore/workstation-switch`] CI workflow YAMLs recovered to `docs/ci-templates/` + workstation-switch checklist added to this file.
- **2026-05-01** PR #7 [`8ea5a3e`] HANDOFF.md + NEXT_STEPS.md + CLAUDE.md refresh — cross-session continuity docs.
- **2026-05-01** PR #6 [`d71a98f`] PII classification + Sentry BeforeSend masking. `internal/pii/` package + `obs.scrubSentryEvent`. Closes the GDPR/CCPA gap on Sentry egress.
- **2026-05-01** PR #5 [`ca36fa3`] OpenTelemetry tracing — `internal/obs/tracer.go`, brain-client `otelhttp` wrap, router middleware, log-correlation (trace_id+span_id alongside request_id).
- **2026-05-01** PR #4 [`7facfce`] `cmd/buildos-fork-init` keypair generator + `docs/fork-onboarding.md`.
- **2026-05-01** PR #3 [`29ea6fe`] SecretSource abstraction (env/file/chain) + `LoadWithSource()`.
- **2026-05-01** PR #2 [`699a64d`] Sprints 1-5 + Phase F core (44 commits — domain endpoints, Brain integration, production hardening, Dockerfile, D8 build-tag hardening).

10 PRs merged under the L8 self-audit gate (PR #9 added the L8 SRE
audit gate as a second checklist applied per PR). Every landed
commit had `make audit` green + integration suite green +
govulncheck clean. PRs #9 onward also have CI green at merge time
(no longer just local audit).

## In flight

Nothing on a branch waiting for review right now.

## Blocked

Nothing blocked right now. (PR #9 cleared the workflow-activation
blocker; CI is now live on `.github/workflows/{ci,release}.yml`.)

Known follow-up surfaced by PR #9 (not blocking, queued):

- `make audit` is weaker than CI: doesn't run `gofmt -l` strictly
  and doesn't smoke-build the Dockerfile. Add both to the local
  audit so future sessions catch issues before CI does.

## Next up (prioritized — pick from the top)

See [.agents/handoff/NEXT_STEPS.md](./.agents/handoff/NEXT_STEPS.md)
for the full prioritized backlog with entry-point file paths.

Top three an L8 PE would queue (#1 done in PR #10, #2 done in PR #11):

1. **D7 wave 3 finish** — Schedule + Pipeline service audit recording.
   Pattern is established (Fleet + Procurement already wired); copy
   the shape.
2. **S1.5 Brain Client Foundation** — resilience layer (retry +
   timeout + circuit), Maestro typed envelopes (D5), Hub proxy
   stubs (D6). Triggers Gate 1.
3. **Vault SecretSource backend** (Tier 2 #4) — `internal/config/vault.go`
   implementing the `SecretSource` interface for HashiCorp Vault.
   Required before first-fork cutover (Phase H).

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
| LocalBlue → Brain → BuildOS | Brain | partial | BuildOS handler shipped (`internal/service/a2a.go` `handleLocalblueLeadCaptured`); Brain-side type definitions deleted 2026-05-04 (orphan branch never merged). When Brain emitter wiring resumes, re-derive `LocalblueLeadCapturedPayload` from BuildOS's `localblueLeadCapturedPayload` struct as the canonical reference. |
| Stripe billing engine | Brain | not yet | gating G1 |
| Vault / SecretsManager backends for SecretSource | BuildOS | env+file shipped; vault next when first customer fork needs it | `internal/config/secrets.go` interface ready |

---

## Workstation switch checklist

Use when picking BuildOS up on a fresh workstation (e.g. switching
from a travel laptop to a primary dev box). The repos themselves are
already clean — this list is for the new environment.

### One-time setup on the new workstation

```bash
# 1. Clone both repos as siblings (the go.mod replace directive expects
#    futurebuild-brain at ../futurebuild-brain relative to buildos):
mkdir ~/repos && cd ~/repos
git clone https://github.com/futurebuildai/buildos.git
git clone https://github.com/futurebuildai/futurebuild-brain.git

# 2. Toolchain:
#    - Go 1.26+        (for build + tests)
#    - Docker           (for testcontainers-backed integration tests)
#    - golangci-lint    (for `make lint`)
#    - govulncheck      `go install golang.org/x/vuln/cmd/govulncheck@latest`
#    - gh CLI           (for PR ops)
#    - make

# 3. gh auth with workflow scope from the start. This unblocks the
#    `.github/workflows/` push that the Mac session couldn't do:
gh auth login                # follow prompts; choose HTTPS or SSH
gh auth refresh -h github.com -s workflow

# 4. git identity:
git config --global user.name "Your Name"
git config --global user.email "you@example.com"

# 5. Verify everything builds + tests:
cd ~/repos/buildos
make audit
make test-integration       # spawns Postgres via testcontainers — needs Docker

# 6. Activate the CI workflows that have been waiting in docs/ci-templates/
git checkout -b ci/activate-workflows
git mv docs/ci-templates/ci.yml      .github/workflows/ci.yml
git mv docs/ci-templates/release.yml .github/workflows/release.yml
git rm docs/ci-templates/README.md
git commit -m "ci: activate CI + release workflows"
git push -u origin ci/activate-workflows
gh pr create --base main --title "ci: activate CI + release workflows"
```

### What to read first

1. [HANDOFF.md](./HANDOFF.md) (this file) — top-of-file sections give
   you 60 seconds of "what just happened, what's next."
2. [CLAUDE.md](./CLAUDE.md) — architecture, conventions, hard CI gates.
3. [.agents/handoff/NEXT_STEPS.md](./.agents/handoff/NEXT_STEPS.md) —
   pick from Tier 1 to start work.
4. [.agents/handoff/ADR-002-single-tenant-fork-model.md](./.agents/handoff/ADR-002-single-tenant-fork-model.md)
   — the most recent strategic decision. Keep this top-of-mind: BuildOS
   is single-tenant per customer fork, not multi-tenant SaaS.

### Things you don't need to bring with you

- **The worktree** — every commit is on the remote.
- **Local secrets** — `.env` files are gitignored. The
  `internal/config.SecretSource` abstraction means a fresh workstation
  with its own `.env` (or no `.env`, with `CONFIG_SOURCE=file:/path`)
  works identically.
- **Local generated artifacts** — `bin/`, `bin/prod/`, `*.test`. All
  rebuildable.

### Things to NOT carry over

- Any `private.pem` from `make fork-init` runs. Those go straight into
  a customer's secret store, never into a worktree on a personal
  laptop.
- Any tokens or credentials baked into local shell history. Audit your
  `~/.bash_history` / `~/.zsh_history` before disposal if you're
  recycling the old machine.

---

## Appendix: workflow YAML (if not in worktree)

The CI + release YAMLs are tracked at
[`docs/ci-templates/`](./docs/ci-templates/) until a session with
`workflow` OAuth scope relocates them to `.github/workflows/`. See
that directory's README for activation steps. Summary of what they do:

**`ci.yml`** mounts six jobs on every PR + push to main:
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
