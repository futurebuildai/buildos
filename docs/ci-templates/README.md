# CI workflow templates — pending relocation to `.github/workflows/`

These two YAML files are the canonical CI + release pipelines for
BuildOS. They live at this path (rather than `.github/workflows/`)
because the GitHub OAuth token Claude Code was using during the
session that authored them lacked the `workflow` scope, which blocks
pushes to `.github/workflows/*`.

## To activate from your workstation

From a clone with a token that has `workflow` scope (the standard
`gh auth login` flow grants this; if missing, run `gh auth refresh -h
github.com -s workflow`):

```bash
git mv docs/ci-templates/ci.yml      .github/workflows/ci.yml
git mv docs/ci-templates/release.yml .github/workflows/release.yml
git rm  docs/ci-templates/README.md
git commit -m "ci: activate CI + release workflows"
git push origin <branch>
```

After the workflows are in place, this directory should be empty
and can be deleted entirely.

## What each workflow does

### `ci.yml` — fires on every PR + push to main

- `lint-migrations` — 5 rules (composite-currency, paired up/down,
  destructive opt-in, CONCURRENTLY index, currency_code pairing) +
  the linter regression suite
- `lint-go` — gofmt, `go vet` (default + `-tags=prod`), govulncheck
  CI-blocking on both build modes
- `test` — `make test` + `make test-prod`
- `test-integration` — Testcontainers-backed Postgres
- `bench-physics` — CPM gate (80-task ≤200ms, 200-task ≤500ms)
- `docker-build` — multi-arch (linux/amd64 + linux/arm64) +
  Trivy scan (CI-blocking on HIGH/CRITICAL)

### `release.yml` — fires on tag push (`v*.*.*`)

- Multi-arch image to GHCR (`ghcr.io/futurebuildai/buildos:<tag>`)
- cosign keyless OIDC signing (no signing key to manage)
- syft SBOM in CycloneDX + SPDX, attached via `cosign attest`
- GitHub Release with auto-changelog from `git log`

Customer forks verifying signatures use:

```bash
cosign verify --certificate-identity-regexp '...' \
  ghcr.io/futurebuildai/buildos@<digest>
```

## Why this approach instead of pasting via UI

Tracking the YAML in version control means:
- The next maintainer doesn't have to re-derive content from chat
  transcripts or commit messages
- Reviewers can comment on specific lines via GitHub's normal PR
  review flow when the activation PR opens
- `git diff` works for future changes
- The relocation is one commit (versus two: paste, then tweak)
