#!/usr/bin/env bash
# e2e-backend.sh — boot a real BuildOS backend for end-to-end testing.
#
# This is the harness behind the "backend-dependent E2E" lane: it stands up a
# live `cmd/server` against a migrated Postgres using NATIVE auth (NOT the
# DEV_AUTH_MODE=header rig), so the whole first-run → claim → wizard → operate
# flow is exercised exactly as a real fork would. The web Playwright journeys
# and the Flutter sync integration test drive that backend.
#
# What it does, in order:
#   1. Generates an EPHEMERAL per-run RSA keypair (JWT signing) + AES-256 vault
#      master key + first-owner bootstrap token. None are persisted; they live
#      in a tmpdir wiped on exit. The vault key being present means the BYOK
#      /integrations routes mount, so the integrations CRUD journey is testable.
#   2. Runs migrations (idempotent) and TRUNCATEs the org graph for a clean,
#      deterministic slate every run (this is a throwaway E2E database — see the
#      safety note below).
#   3. Seeds a single fork-zero organization row (onboarding_complete=false).
#      The server's SeedBootstrapTokenIfNeeded then materializes the bootstrap
#      token against that org at boot, so POST /api/v1/auth/claim works.
#   4. Boots the server in the background and waits for GET /ready.
#   5. Exports the contract the tests need (E2E_API_URL, E2E_BOOTSTRAP_TOKEN,
#      E2E_OWNER_EMAIL, E2E_OWNER_PASSWORD) and either runs a passed command
#      (tearing the server down afterwards) or stays foreground until Ctrl-C.
#
# Usage:
#   scripts/e2e-backend.sh [--db-up] [--reset|--no-reset] [--seed-field] [--seed-schedule] -- <command...>
#   scripts/e2e-backend.sh [--db-up]                 # foreground, no command
#
# Modes:
#   (default)        Seeds a single onboarding-INCOMPLETE org and lets the server
#                    materialize the bootstrap token against it. This is the shape
#                    the WEB first-run → wizard journey needs (the wizard must run).
#   --seed-field     Seeds an onboarding-COMPLETE org WITH a project + one task, and
#                    inserts the bootstrap-token hash directly (the server's boot
#                    seeder no-ops when there's no incomplete org). This is the shape
#                    the MOBILE outbox→/field journey needs: claim the owner, then
#                    POST progress against a real task. Exports E2E_PROJECT_ID +
#                    E2E_TASK_ID on top of the usual contract.
#   --seed-schedule  Leaves onboarding INCOMPLETE (the wizard still runs) but plants
#                    a project with a linear FS task chain into the fork-zero org.
#                    The WEB recalc-cascade journey logs in as the owner created by
#                    the first-run/wizard journey, then recalculates that project to
#                    exercise the CPM engine + cascade-diff path. Exports
#                    E2E_SCHEDULE_PROJECT_NAME. Composable with the default web run.
#
# Examples:
#   # Local: bring up the compose DB, run the web live journeys, tear down.
#   scripts/e2e-backend.sh --db-up -- npm --prefix web run test:e2e:live
#
#   # Mobile: operable org + project + task, run the Flutter live sync test.
#   scripts/e2e-backend.sh --db-up --seed-field -- \
#     flutter --no-version-check test test/live/sync_live_test.dart
#
#   # CI: DB is a service container; DATABASE_URL is exported by the job.
#   scripts/e2e-backend.sh -- bash scripts/e2e-run-all.sh
#
#   # Local debugging: just boot a live backend and leave it running.
#   scripts/e2e-backend.sh --db-up
#
# SAFETY: this harness OWNS its database state — on each run it TRUNCATEs the
# organizations graph (CASCADE) for determinism. Point DATABASE_URL ONLY at a
# throwaway E2E/dev database, never a database with data you care about. The
# default DSN is the local docker-compose dev DB on port 5433.
set -euo pipefail

# ----------------------------------------------------------------------------
# Resolve repo root (this script lives in scripts/).
# ----------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

# ----------------------------------------------------------------------------
# Config (all overridable from the environment).
# ----------------------------------------------------------------------------
: "${DATABASE_URL:=postgres://fb_user:fb_pass@localhost:5433/futurebuild_os?sslmode=disable}"
: "${PORT:=8080}"
HOST="127.0.0.1"
API_URL="http://${HOST}:${PORT}"
OWNER_EMAIL="${E2E_OWNER_EMAIL:-owner@e2e.test}"
OWNER_PASSWORD="${E2E_OWNER_PASSWORD:-correct horse battery staple}"
ORG_SLUG="e2e-fork"
ORG_NAME="E2E Fork"

DO_DB_UP=0
DO_RESET=1
DO_SEED_FIELD=0
DO_SEED_SCHEDULE=0
CMD=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --db-up) DO_DB_UP=1; shift ;;
    --reset) DO_RESET=1; shift ;;
    --no-reset) DO_RESET=0; shift ;;
    --seed-field) DO_SEED_FIELD=1; shift ;;
    --seed-schedule) DO_SEED_SCHEDULE=1; shift ;;
    --) shift; CMD=("$@"); break ;;
    *) echo "e2e-backend: unknown argument: $1" >&2; exit 2 ;;
  esac
done

log() { printf '\033[36m[e2e-backend]\033[0m %s\n' "$*" >&2; }
err() { printf '\033[31m[e2e-backend]\033[0m %s\n' "$*" >&2; }

# ----------------------------------------------------------------------------
# Ephemeral secrets in a tmpdir wiped on exit. The server reads the PEM CONTENT
# from env (JWT_PRIVATE_KEY_PEM / JWT_PUBLIC_KEY_PEM), not a path, so we keep
# the files only long enough to read them into variables.
# ----------------------------------------------------------------------------
TMPDIR_E2E="$(mktemp -d)"
SERVER_PID=""

cleanup() {
  local rc=$?
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    log "stopping server (pid ${SERVER_PID})"
    # `go run` spawns a *compiled child* that holds :8080; killing only the
    # go-run parent leaks that child (stale binary → next run can't bind, and
    # readiness probes hit the old code). We launch the server via `setsid` in
    # its own process group, so kill the whole group (negative PID) to reap
    # both. Fall back to a plain kill if the group signal misses.
    kill -- "-${SERVER_PID}" 2>/dev/null || kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${TMPDIR_E2E}"
  exit "${rc}"
}
trap cleanup EXIT INT TERM

# base64url-no-pad token: 32 random bytes → exactly 43 chars, matching the
# server's RawURLEncoding 43-char bootstrap-token validation.
gen_bootstrap_token() {
  openssl rand 32 | openssl base64 -A | tr '+/' '-_' | tr -d '='
}

log "generating ephemeral JWT keypair + vault key + bootstrap token"
openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 \
  -out "${TMPDIR_E2E}/jwt_private.pem" 2>/dev/null
openssl rsa -in "${TMPDIR_E2E}/jwt_private.pem" -pubout \
  -out "${TMPDIR_E2E}/jwt_public.pem" 2>/dev/null

JWT_PRIVATE_KEY_PEM="$(cat "${TMPDIR_E2E}/jwt_private.pem")"
JWT_PUBLIC_KEY_PEM="$(cat "${TMPDIR_E2E}/jwt_public.pem")"
VAULT_MASTER_KEY="$(openssl rand -base64 32)"
BUILDOS_BOOTSTRAP_TOKEN="$(gen_bootstrap_token)"

export JWT_PRIVATE_KEY_PEM JWT_PUBLIC_KEY_PEM VAULT_MASTER_KEY BUILDOS_BOOTSTRAP_TOKEN
export DATABASE_URL PORT
export APP_BASE_URL="http://localhost:5173"
export MAIL_FROM="e2e@buildos.test"
# Native auth path — the dev-header bypass MUST be off so the real claim/login
# flow is what gets tested.
unset DEV_AUTH_MODE || true

# ----------------------------------------------------------------------------
# Optionally bring up the compose Postgres and wait for it.
# ----------------------------------------------------------------------------
if [[ "${DO_DB_UP}" -eq 1 ]]; then
  log "starting docker-compose Postgres"
  make db-up >/dev/null
fi

wait_for_db() {
  log "waiting for Postgres at ${DATABASE_URL%%\?*}"
  for _ in $(seq 1 60); do
    if psql "${DATABASE_URL}" -c 'SELECT 1' >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  err "Postgres did not become reachable"
  return 1
}
wait_for_db

# ----------------------------------------------------------------------------
# Migrate, then reset to a deterministic clean slate, then seed fork-zero org.
# ----------------------------------------------------------------------------
log "applying migrations"
make migrate >/dev/null

if [[ "${DO_RESET}" -eq 1 ]]; then
  log "resetting org graph (TRUNCATE organizations CASCADE)"
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -q \
    -c "TRUNCATE organizations RESTART IDENTITY CASCADE;"
fi

log "seeding fork-zero organization (${ORG_SLUG})"
psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -q \
  -c "INSERT INTO organizations (name, slug) VALUES ('${ORG_NAME}', '${ORG_SLUG}')
      ON CONFLICT (slug) DO NOTHING;"

# Single helper: run a query and capture a single scalar. -tA = tuples-only +
# unaligned (no header/footer/whitespace). -q is essential for INSERT ... RETURNING:
# without it psql emits the "INSERT 0 1" command tag on a second line, which would
# pollute the captured id (and break the next statement that interpolates it).
psql_scalar() { psql "${DATABASE_URL}" -tAq -v ON_ERROR_STOP=1 -c "$1"; }

E2E_PROJECT_ID=""
E2E_TASK_ID=""

if [[ "${DO_SEED_FIELD}" -eq 1 ]]; then
  # Mobile field journey needs an OPERABLE org (past the SetupGate) with a real
  # task to report progress against. The server's boot seeder only attaches the
  # bootstrap token to an INCOMPLETE org, so when we pre-complete onboarding we
  # must insert the token hash ourselves. The hash is sha256-hex of the cleartext
  # (matches internal/service/setup.go hashBootstrapToken / auth.HashOpaqueToken).
  log "seed-field: marking org onboarding-complete + seeding token, project, task"
  ORG_ID="$(psql_scalar "SELECT id FROM organizations WHERE slug = '${ORG_SLUG}';")"
  TOKEN_HASH="$(printf '%s' "${BUILDOS_BOOTSTRAP_TOKEN}" | sha256sum | cut -d' ' -f1)"

  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -q \
    -c "UPDATE organizations SET onboarding_complete = true,
          onboarding_completed_at = now() WHERE id = '${ORG_ID}';" \
    -c "INSERT INTO setup_bootstrap_tokens (org_id, token_hash, expires_at)
        VALUES ('${ORG_ID}', '${TOKEN_HASH}', now() + interval '7 days')
        ON CONFLICT (token_hash) DO NOTHING;"

  E2E_PROJECT_ID="$(psql_scalar "INSERT INTO projects (org_id, name, status, gsf)
      VALUES ('${ORG_ID}', 'E2E Field House', 'active', 2400) RETURNING id;")"
  E2E_TASK_ID="$(psql_scalar "INSERT INTO project_tasks
        (project_id, wbs_code, name, duration_days, status, percent_complete)
      VALUES ('${E2E_PROJECT_ID}', '03-30', 'Foundation', 5, 'pending', 0)
      RETURNING id;")"
  export E2E_PROJECT_ID E2E_TASK_ID
  log "seed-field: project=${E2E_PROJECT_ID} task=${E2E_TASK_ID}"
fi

if [[ "${DO_SEED_SCHEDULE}" -eq 1 ]]; then
  # The web recalc-cascade journey needs a project with a real task graph so the
  # CPM engine has something to compute and a critical path to surface. Unlike
  # --seed-field, we DO NOT pre-complete onboarding or seed the token hash: the
  # web lane drives the first-run claim → 6-step wizard itself (the onboarding
  # live spec), and the wizard must run against an INCOMPLETE org. We only plant
  # the project + a linear FS task chain (Site Prep → Foundation → Framing) into
  # the same fork-zero org the owner claims into; the tasks carry no CPM results
  # yet (early_*/late_*/is_critical NULL), so the schedule page opens on the
  # "not computed" empty state and the first Recalculate computes + cascades it.
  log "seed-schedule: seeding project + linear FS task chain (no onboarding change)"
  SCHED_ORG_ID="$(psql_scalar "SELECT id FROM organizations WHERE slug = '${ORG_SLUG}';")"

  E2E_SCHEDULE_PROJECT_NAME="E2E Tower"
  SCHED_PROJECT_ID="$(psql_scalar "INSERT INTO projects (org_id, name, status, gsf)
      VALUES ('${SCHED_ORG_ID}', '${E2E_SCHEDULE_PROJECT_NAME}', 'active', 3000)
      RETURNING id;")"

  SCHED_A_ID="$(psql_scalar "INSERT INTO project_tasks
        (project_id, wbs_code, name, duration_days, status, percent_complete)
      VALUES ('${SCHED_PROJECT_ID}', '01-00', 'Site Prep', 3, 'pending', 0)
      RETURNING id;")"
  SCHED_B_ID="$(psql_scalar "INSERT INTO project_tasks
        (project_id, wbs_code, name, duration_days, status, percent_complete)
      VALUES ('${SCHED_PROJECT_ID}', '03-30', 'Foundation', 5, 'pending', 0)
      RETURNING id;")"
  SCHED_C_ID="$(psql_scalar "INSERT INTO project_tasks
        (project_id, wbs_code, name, duration_days, status, percent_complete)
      VALUES ('${SCHED_PROJECT_ID}', '06-10', 'Framing', 8, 'pending', 0)
      RETURNING id;")"

  # Linear finish-to-start chain → every task is on the critical path, so the
  # first recalc reports critical_path_changed=true (cascade notice renders).
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -q \
    -c "INSERT INTO task_dependencies (project_id, predecessor_id, successor_id, dependency_type)
        VALUES ('${SCHED_PROJECT_ID}', '${SCHED_A_ID}', '${SCHED_B_ID}', 'FS'),
               ('${SCHED_PROJECT_ID}', '${SCHED_B_ID}', '${SCHED_C_ID}', 'FS');"

  export E2E_SCHEDULE_PROJECT_NAME
  log "seed-schedule: project=${SCHED_PROJECT_ID} (${E2E_SCHEDULE_PROJECT_NAME}) tasks=3 deps=2"
fi

# ----------------------------------------------------------------------------
# Boot the server in the background; wait for readiness.
# ----------------------------------------------------------------------------
log "booting cmd/server on ${API_URL} (native auth, vault on)"
# setsid → new process group so cleanup() can reap the whole tree (the go-run
# parent AND its compiled child that actually binds :8080). SERVER_PID is the
# group leader's PID, also the PGID.
setsid go run ./cmd/server >"${TMPDIR_E2E}/server.log" 2>&1 &
SERVER_PID=$!

wait_for_ready() {
  for _ in $(seq 1 120); do
    if ! kill -0 "${SERVER_PID}" 2>/dev/null; then
      err "server process exited during startup; last log lines:"
      tail -n 40 "${TMPDIR_E2E}/server.log" >&2 || true
      return 1
    fi
    if curl -fsS "${API_URL}/ready" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  err "server did not become ready; last log lines:"
  tail -n 40 "${TMPDIR_E2E}/server.log" >&2 || true
  return 1
}
wait_for_ready
log "server ready"

# ----------------------------------------------------------------------------
# Export the test contract and either run the command or idle in foreground.
# ----------------------------------------------------------------------------
export E2E_API_URL="${API_URL}"
export E2E_BOOTSTRAP_TOKEN="${BUILDOS_BOOTSTRAP_TOKEN}"
export E2E_OWNER_EMAIL="${OWNER_EMAIL}"
export E2E_OWNER_PASSWORD="${OWNER_PASSWORD}"

if [[ "${#CMD[@]}" -gt 0 ]]; then
  log "running: ${CMD[*]}"
  "${CMD[@]}"
  rc=$?
  log "command exited with ${rc}"
  exit "${rc}"
else
  log "no command given — backend is live at ${API_URL}"
  log "  E2E_BOOTSTRAP_TOKEN=${E2E_BOOTSTRAP_TOKEN}"
  log "  E2E_OWNER_EMAIL=${E2E_OWNER_EMAIL}"
  log "press Ctrl-C to stop"
  wait "${SERVER_PID}"
fi
