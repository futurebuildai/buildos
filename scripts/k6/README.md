# BuildOS k6 load harness (Phase 4c)

Load / soak tests for the hottest and most expensive BuildOS paths, plus an
adversarial rate-limit check. [k6](https://k6.io) is a **test-only external CLI**
(Go binary, run manually against a staging/load instance) — it is **not** a Go
module or npm dependency and adds nothing to the build. It is intentionally
absent from `.agents/TECH_STACK.md`'s dependency lists for that reason; this
README is the flag.

## Scripts

| Script | Targets | What it proves |
|---|---|---|
| `smoke.js` | `/health`, `/ready`, `/field/sync` | Target + fixtures are wired (run first). |
| `field_sync.js` | `GET /api/v1/field/sync` | The field read hot path (tasks + feed cards + 4a-ii equipment) holds p95 < 500ms under ramp to 50 VUs. |
| `schedule_recalc.js` | `POST …/schedule/recalculate` | The CPM physics write path stays p95 < 1s under concurrency (bench gates cap pure CPM at ≤200ms/80 tasks, ≤500ms/200). |
| `auth_login.js` | `POST /api/v1/auth/login` | The per-IP limiter (50 rps / 100 burst) holds under a 150 rps flood — past the burst it returns **429**, never 5xx, so argon2id can't be weaponized into a DoS amplifier. |

## Running

k6 is not installed in CI. Install locally (`brew install k6` / [docs](https://k6.io/docs/get-started/installation/)) and run against a **staging build with `DEV_AUTH_MODE=header`** (the authenticated scripts send `X-Dev-Auth`; the prod build tag no-ops that header, so prod would 401 — never point these at prod).

```bash
# 1. Smoke (1 VU)
k6 run -e BASE_URL=https://staging.example.com \
       -e ORG_ID=<uuid> -e USER_ID=<uuid> -e ROLE=field_worker \
       scripts/k6/smoke.js

# 2. Field read load
k6 run -e BASE_URL=… -e ORG_ID=… -e USER_ID=… -e ROLE=field_worker \
       scripts/k6/field_sync.js

# 3. CPM recalc load (needs a seeded project graph)
k6 run -e BASE_URL=… -e ORG_ID=… -e USER_ID=… -e ROLE=superintendent \
       -e PROJECT_ID=<uuid> scripts/k6/schedule_recalc.js

# 4. Rate-limiter / argon2id flood (run from one source IP)
k6 run -e BASE_URL=… scripts/k6/auth_login.js
```

Env (see `lib/config.js`): `BASE_URL`, `ORG_ID`, `USER_ID`, `ROLE`
(`owner`>`admin`>`superintendent`>`field_worker`), `PROJECT_ID`. Seed the fixtures
via the onboarding wizard or a SQL seed before running.

## Interpreting

- A run **passes** when every `thresholds` line is green (k6 exits non-zero on
  breach — usable as a CI gate against a dedicated load env).
- `field_sync` / `schedule_recalc` breaching p95 → check DB pool saturation
  (`/ready`, the `buildos_http_request_duration_seconds` histogram, pool stats)
  and whether the CPM graph is larger than the bench fixtures.
- `auth_login`: a **high 429 share is the success condition**, not a failure —
  it shows the limiter shedding load before argon2id work. Any **5xx** is a real
  finding (the limiter isn't shedding, or a downstream is crashing).

## SLO targets (tune per fork)

- Reads (`field/sync`, gantt, tasks): p95 < 500ms, p99 < 1s, error rate < 1%.
- CPM recalc: p95 < 1s, p99 < 2s, error rate < 1%.
- Login flood: 0 × 5xx; limiter engages (≥10% 429 under 150 rps).

Pair a load run with the Prometheus dashboards + alerts
(`deploy/prometheus/`) and the observability runbook for the server-side view.
