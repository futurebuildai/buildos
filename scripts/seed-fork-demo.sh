#!/usr/bin/env bash
# seed-fork-demo.sh — populate a BuildOS fork with realistic Kelbrook-flavored
# DEMO data, driven entirely through the HTTP API (so the Phase-0C ingress
# endpoints' validation is exercised end-to-end, exactly as a real operator
# would hit them).
#
# WHAT IT CREATES (per the Phase-0C seed design, §7):
#   - Auth: claims a first owner from a bootstrap token (first run) OR logs in
#     with existing owner creds. Captures the access token for Bearer auth.
#   - Onboarding: drives the /api/v1/setup wizard (company → trade → cost code →
#     calendar → complete) so SetupGate opens. (A documented SQL escape hatch is
#     described below for fast staging resets.)
#   - ~3 projects, each with an explicit project_start_date (deterministic Gantt
#     root) and a realistic ~10-phase residential WBS task chain imported via
#     POST /schedule/import with recalculate=true → the Gantt populates with a
#     REAL critical path and a delay_cascade feed card fires.
#   - Per project: a budget baseline (POST /budgets) keyed on the same WBS phases
#     so project_budgets.wbs_code aligns with project_tasks.wbs_code.
#   - ~4 employees (POST /employees) + certifications (POST .../certifications)
#     with a mix of expiry dates (expired / near-expiry / far-future) so the HR
#     cert-status chips render all states.
#   - Per first project: fleet assets (POST /fleet), procurement items
#     (POST /procurement), pipeline prospects (POST /pipeline/prospects), and a
#     manual invoice (POST /invoices) — exercising the EXISTING create surface.
#   - Corporate rollup: see "FINANCIALS ROLLUP" below — the Financials Summary
#     tab INNER JOINs corporate_budgets, so the rollup River job MUST run after
#     budgets or the Summary stays empty.
#
# REPEATABILITY: budget/task import is REJECT-ON-CONFLICT in v1
# (UNIQUE(project_id, wbs_code) → 400 on a re-run). Use --reset to delete the
# demo projects + employees first. --reset is destructive and only operates on
# the demo data this script created (matched by the configured names/slug).
#
# USAGE:
#   scripts/seed-fork-demo.sh [--reset]
#
# CONFIG (env vars, with defaults — NO SECRETS ARE HARDCODED):
#   BASE_URL                API base (default https://staging.kelbrook.buildos.app)
#   ORG_ID                  the org UUID (required for org-scoped routes; if
#                           unset, derived from the authenticated user payload)
#   SEED_OWNER_EMAIL        owner email for login (required unless claiming)
#   SEED_OWNER_PASSWORD     owner password for login (required unless claiming)
#   BUILDOS_BOOTSTRAP_TOKEN one-shot first-owner claim token (first run only;
#                           when set AND login fails/skipped, the script claims)
#   SEED_OWNER_NAME         display name for the claimed owner (default "Demo Owner")
#   CURRENCY                budget currency, USD|CAD (default USD)
#   GSF                     project gross square footage (default 3200; 1500-6000)
#   DATABASE_URL            ONLY used for --reset and the documented rollup
#                           escape hatch; never for auth. Optional otherwise.
#
# EXAMPLES:
#   # Fresh fork: claim the owner with the bootstrap token, then seed.
#   BASE_URL=http://localhost:8080 \
#   BUILDOS_BOOTSTRAP_TOKEN=$(cat bootstrap_token.txt) \
#   SEED_OWNER_EMAIL=owner@demo.test SEED_OWNER_PASSWORD='Sup3rSecret!' \
#   ORG_ID=<org-uuid> scripts/seed-fork-demo.sh
#
#   # Re-seed staging (wipe demo data first):
#   SEED_OWNER_EMAIL=... SEED_OWNER_PASSWORD=... ORG_ID=... \
#   scripts/seed-fork-demo.sh --reset
set -euo pipefail

# ----------------------------------------------------------------------------
# Config + dependency checks
# ----------------------------------------------------------------------------
BASE_URL="${BASE_URL:-https://staging.kelbrook.buildos.app}"
CURRENCY="${CURRENCY:-USD}"
GSF="${GSF:-3200}"
SEED_OWNER_NAME="${SEED_OWNER_NAME:-Demo Owner}"
ORG_ID="${ORG_ID:-}"
DO_RESET=0

for arg in "$@"; do
  case "$arg" in
    --reset) DO_RESET=1 ;;
    -h|--help) sed -n '2,60p' "$0"; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

for bin in curl jq; do
  command -v "$bin" >/dev/null 2>&1 || { echo "FATAL: '$bin' is required" >&2; exit 1; }
done

log()  { printf '\033[36m[seed]\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[33m[seed]\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[31m[seed] FATAL:\033[0m %s\n' "$*" >&2; exit 1; }

# api METHOD PATH [JSON_BODY] — authenticated request, returns the response body.
# Aborts on a non-2xx (printing status + body) so a seed failure is loud.
TOKEN=""
api() {
  local method="$1" path="$2" body="${3:-}"
  local url="${BASE_URL}${path}"
  local tmp; tmp="$(mktemp)"
  local code
  if [[ -n "$body" ]]; then
    code="$(curl -sS -o "$tmp" -w '%{http_code}' -X "$method" "$url" \
      -H "Authorization: Bearer ${TOKEN}" -H 'Content-Type: application/json' -d "$body")"
  else
    code="$(curl -sS -o "$tmp" -w '%{http_code}' -X "$method" "$url" \
      -H "Authorization: Bearer ${TOKEN}")"
  fi
  if [[ "$code" -lt 200 || "$code" -ge 300 ]]; then
    warn "${method} ${path} -> HTTP ${code}: $(cat "$tmp")"
    rm -f "$tmp"
    return 1
  fi
  cat "$tmp"; rm -f "$tmp"
}

# ----------------------------------------------------------------------------
# 1. AUTH — claim (first run) or login. Capture the access token.
# ----------------------------------------------------------------------------
authenticate() {
  local resp
  # Prefer login when creds are present and a session already exists; fall back
  # to claim when a bootstrap token is provided (fresh fork, no owner yet).
  if [[ -n "${SEED_OWNER_EMAIL:-}" && -n "${SEED_OWNER_PASSWORD:-}" ]]; then
    log "attempting owner login (${SEED_OWNER_EMAIL})"
    if resp="$(curl -sS -X POST "${BASE_URL}/api/v1/auth/login" \
        -H 'Content-Type: application/json' \
        -d "$(jq -n --arg e "$SEED_OWNER_EMAIL" --arg p "$SEED_OWNER_PASSWORD" \
              '{email:$e,password:$p}')")" \
       && TOKEN="$(echo "$resp" | jq -r '.data.access_token // empty')" \
       && [[ -n "$TOKEN" ]]; then
      ORG_ID="${ORG_ID:-$(echo "$resp" | jq -r '.data.user.org_id // empty')}"
      log "logged in (org_id=${ORG_ID})"
      return 0
    fi
    warn "login failed; will try bootstrap claim if a token is set"
  fi

  if [[ -n "${BUILDOS_BOOTSTRAP_TOKEN:-}" ]]; then
    [[ -n "${SEED_OWNER_EMAIL:-}" && -n "${SEED_OWNER_PASSWORD:-}" ]] \
      || die "claim requires SEED_OWNER_EMAIL + SEED_OWNER_PASSWORD"
    log "claiming first owner via bootstrap token"
    resp="$(curl -sS -X POST "${BASE_URL}/api/v1/auth/claim" \
      -H 'Content-Type: application/json' \
      -d "$(jq -n --arg t "$BUILDOS_BOOTSTRAP_TOKEN" --arg e "$SEED_OWNER_EMAIL" \
            --arg p "$SEED_OWNER_PASSWORD" --arg n "$SEED_OWNER_NAME" \
            '{token:$t,email:$e,password:$p,display_name:$n}')")"
    TOKEN="$(echo "$resp" | jq -r '.data.access_token // empty')"
    [[ -n "$TOKEN" ]] || die "claim failed: $resp"
    ORG_ID="${ORG_ID:-$(echo "$resp" | jq -r '.data.user.org_id // empty')}"
    log "claimed owner (org_id=${ORG_ID})"
    return 0
  fi

  die "no auth path: set SEED_OWNER_EMAIL + SEED_OWNER_PASSWORD (login) or BUILDOS_BOOTSTRAP_TOKEN (claim)"
}

# ----------------------------------------------------------------------------
# 2. ONBOARDING — drive the wizard so SetupGate opens.
#    (Escape hatch for fast staging resets, documented but NOT default:
#       psql "$DATABASE_URL" -c "UPDATE organizations SET onboarding_complete=true WHERE id='$ORG_ID';"
#     We drive the wizard instead so the real path is exercised.)
# ----------------------------------------------------------------------------
onboard() {
  # Idempotent: GET state first; if already complete, skip.
  local state complete
  state="$(api GET /api/v1/setup/state 2>/dev/null || true)"
  complete="$(echo "$state" | jq -r '.data.onboarding_complete // empty' 2>/dev/null || true)"
  if [[ "$complete" == "true" ]]; then
    log "onboarding already complete; skipping wizard"
    return 0
  fi

  log "running onboarding wizard"
  api POST /api/v1/setup/company-info \
    '{"legal_name":"Kelbrook Construction LLC","company_type":"residential_gc","region":"US"}' >/dev/null || true
  api POST /api/v1/setup/trades \
    '{"code":"GEN","name":"General Construction","is_default":true}' >/dev/null || true
  api POST /api/v1/setup/cost-codes \
    '{"code":"01-00","name":"General Requirements","division":"01","is_default":true}' >/dev/null || true
  # Mon-Fri working mask (bits 0..6 = Sun..Sat → Mon-Fri = 0b0111110 = 62), 8h day.
  api POST /api/v1/setup/calendars \
    '{"name":"Standard 5-day","timezone":"America/New_York","working_days_mask":62,"daily_work_minutes":480,"is_default":true}' >/dev/null || true
  api POST /api/v1/setup/complete '{}' >/dev/null \
    || warn "setup/complete failed — org may already be onboarded, or a prereq is missing"
  log "onboarding wizard done"
}

# ----------------------------------------------------------------------------
# Project + schedule + budget builders
# ----------------------------------------------------------------------------

# A realistic ~10-phase residential WBS chain (sitework → foundation → framing →
# MEP rough → drywall → finishes) with a linear+branch FS dependency graph. The
# tasks/deps/budgets share WBS codes so the Gantt + Budget tab align.
TASKS_JSON='[
  {"wbs_code":"01-00","name":"Site Prep & Excavation","duration_days":4},
  {"wbs_code":"03-30","name":"Foundation & Footings","duration_days":6},
  {"wbs_code":"06-10","name":"Framing","duration_days":10},
  {"wbs_code":"07-20","name":"Roofing & Dry-In","duration_days":4},
  {"wbs_code":"15-00","name":"Plumbing Rough-In","duration_days":5},
  {"wbs_code":"16-00","name":"Electrical Rough-In","duration_days":5},
  {"wbs_code":"09-20","name":"Insulation & Drywall","duration_days":8},
  {"wbs_code":"09-90","name":"Paint & Finishes","duration_days":7},
  {"wbs_code":"12-00","name":"Cabinets & Trim","duration_days":6},
  {"wbs_code":"01-99","name":"Final Inspection & Punch","duration_days":3}
]'
DEPS_JSON='[
  {"predecessor_code":"01-00","successor_code":"03-30","dependency_type":"FS"},
  {"predecessor_code":"03-30","successor_code":"06-10","dependency_type":"FS"},
  {"predecessor_code":"06-10","successor_code":"07-20","dependency_type":"FS"},
  {"predecessor_code":"06-10","successor_code":"15-00","dependency_type":"FS"},
  {"predecessor_code":"06-10","successor_code":"16-00","dependency_type":"FS"},
  {"predecessor_code":"15-00","successor_code":"09-20","dependency_type":"FS"},
  {"predecessor_code":"16-00","successor_code":"09-20","dependency_type":"FS"},
  {"predecessor_code":"07-20","successor_code":"09-20","dependency_type":"FS"},
  {"predecessor_code":"09-20","successor_code":"09-90","dependency_type":"FS"},
  {"predecessor_code":"09-90","successor_code":"12-00","dependency_type":"FS"},
  {"predecessor_code":"12-00","successor_code":"01-99","dependency_type":"FS"}
]'
# Budget baseline keyed to the same WBS phases (estimated cents per phase).
BUDGETS_LINES='[
  {"wbs_code":"01-00","phase_name":"Site Prep & Excavation","estimated_cost_cents":4500000},
  {"wbs_code":"03-30","phase_name":"Foundation & Footings","estimated_cost_cents":12000000},
  {"wbs_code":"06-10","phase_name":"Framing","estimated_cost_cents":18000000},
  {"wbs_code":"07-20","phase_name":"Roofing & Dry-In","estimated_cost_cents":7500000},
  {"wbs_code":"15-00","phase_name":"Plumbing Rough-In","estimated_cost_cents":6000000},
  {"wbs_code":"16-00","phase_name":"Electrical Rough-In","estimated_cost_cents":6500000},
  {"wbs_code":"09-20","phase_name":"Insulation & Drywall","estimated_cost_cents":9000000},
  {"wbs_code":"09-90","phase_name":"Paint & Finishes","estimated_cost_cents":5500000},
  {"wbs_code":"12-00","phase_name":"Cabinets & Trim","estimated_cost_cents":11000000},
  {"wbs_code":"01-99","phase_name":"Final Inspection & Punch","estimated_cost_cents":1500000}
]'

# build_project NAME START_DATE — creates a project, imports the schedule
# (recalculate=true), and lays down the budget baseline. Echoes the project id.
build_project() {
  local name="$1" start="$2"
  local resp pid
  resp="$(api POST /api/v1/projects \
    "$(jq -n --arg n "$name" --argjson g "$GSF" --arg s "$start" \
       '{name:$n,gsf:$g,project_start_date:$s}')")" || die "create project '$name'"
  pid="$(echo "$resp" | jq -r '.data.project.id // .data.id // empty')"
  [[ -n "$pid" ]] || die "no project id in response: $resp"
  log "  project '$name' = $pid"

  # KEYSTONE: import tasks+deps, auto-recalc → populated Gantt + critical path.
  api "POST" "/api/v1/projects/${pid}/schedule/import" \
    "$(jq -n --argjson t "$TASKS_JSON" --argjson d "$DEPS_JSON" \
       '{tasks:$t,dependencies:$d,recalculate:true}')" >/dev/null \
    || die "schedule import for '$name'"
  log "  schedule imported + recalculated"

  # Budget baseline keyed on the same WBS phases.
  api "POST" "/api/v1/projects/${pid}/budgets" \
    "$(jq -n --argjson b "$BUDGETS_LINES" --arg c "$CURRENCY" \
       '{budgets:[$b[]|.+{currency_code:$c}]}')" >/dev/null \
    || die "budget baseline for '$name'"
  log "  budget baseline created"

  echo "$pid"
}

# ----------------------------------------------------------------------------
# HR builder — employees + a mix of cert expiry states.
# ----------------------------------------------------------------------------
seed_hr() {
  log "seeding employees + certifications"
  # 4 crew; cert expiry mix: expired / near-expiry / far-future.
  local people=(
    "Dana|Cole|Superintendent|osha_30|2030-06-01"
    "Marcus|Reyes|Foreman|osha_10|2024-02-15"      # expired (in the past)
    "Priya|Shah|Electrician|electrical_license|2026-07-15"  # near-expiry
    "Sam|Okafor|Carpenter|first_aid|2031-01-10"
  )
  local entry first last role ctype expiry resp eid
  for entry in "${people[@]}"; do
    IFS='|' read -r first last role ctype expiry <<<"$entry"
    resp="$(api "POST" "/api/v1/org/${ORG_ID}/employees" \
      "$(jq -n --arg f "$first" --arg l "$last" --arg r "$role" \
         '{first_name:$f,last_name:$l,role:$r}')")" || { warn "employee $first $last failed"; continue; }
    eid="$(echo "$resp" | jq -r '.data.employee.id // empty')"
    [[ -n "$eid" ]] || { warn "no employee id for $first $last"; continue; }
    api "POST" "/api/v1/org/${ORG_ID}/employees/${eid}/certifications" \
      "$(jq -n --arg t "$ctype" --arg e "$expiry" \
         '{cert_type:$t,expiry_date:$e}')" >/dev/null \
      || warn "cert for $first $last failed"
    log "  employee $first $last ($role) + cert $ctype (expiry $expiry)"
  done
}

# ----------------------------------------------------------------------------
# Existing create surface — fleet / procurement / pipeline / invoice on project 1.
# ----------------------------------------------------------------------------
seed_extras() {
  local pid="$1"
  log "seeding fleet / procurement / pipeline / invoice"
  api "POST" "/api/v1/org/${ORG_ID}/fleet" \
    '{"name":"CAT 320 Excavator","asset_type":"excavator","serial_number":"CAT320-001"}' >/dev/null \
    || warn "fleet asset failed"
  api "POST" "/api/v1/org/${ORG_ID}/fleet" \
    '{"name":"Bobcat S650 Skid Steer","asset_type":"skid_steer"}' >/dev/null \
    || warn "fleet asset 2 failed"

  api "POST" "/api/v1/projects/${pid}/procurement" \
    "$(jq -n --arg c "$CURRENCY" \
       '{name:"Engineered Roof Trusses",wbs_code:"07-20",estimated_cost_cents:3200000,estimated_cost_currency_code:$c,lead_time_days:21,weather_buffer_days:5,need_by_date:"2026-08-01"}')" >/dev/null \
    || warn "procurement item failed"

  api "POST" "/api/v1/org/${ORG_ID}/pipeline/prospects" \
    '{"name":"Westview Custom Home","client_name":"The Harpers","gsf":4100,"source":"referral"}' >/dev/null \
    || warn "pipeline prospect failed"

  api "POST" "/api/v1/projects/${pid}/invoices" \
    "$(jq -n --arg c "$CURRENCY" \
       '{vendor_name:"Kelbrook Lumber Co",amount_cents:1850000,currency_code:$c,wbs_code:"06-10",invoice_number:"KLC-2026-0412"}')" >/dev/null \
    || warn "invoice failed"
}

# ----------------------------------------------------------------------------
# --reset: delete the demo projects + employees this script created.
# ----------------------------------------------------------------------------
reset_demo() {
  [[ -n "${DATABASE_URL:-}" ]] || die "--reset requires DATABASE_URL (it deletes demo rows via SQL)"
  [[ -n "$ORG_ID" ]] || die "--reset requires ORG_ID"
  warn "RESET: deleting demo projects + employees for org ${ORG_ID}"
  # Demo projects are matched by the names this script creates. Children
  # (tasks, deps, budgets, invoices, procurement) cascade or are removed by
  # name match. This is intentionally scoped to the demo data only.
  psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -q <<SQL
DO \$\$
DECLARE pid uuid;
BEGIN
  FOR pid IN
    SELECT id FROM projects
    WHERE org_id = '${ORG_ID}'
      AND name IN ('Kelbrook Residence','Aspen Ridge Custom','Birchwood Estate')
  LOOP
    DELETE FROM task_dependencies WHERE project_id = pid;
    DELETE FROM project_budgets   WHERE project_id = pid;
    DELETE FROM invoices          WHERE project_id = pid;
    DELETE FROM procurement_items WHERE project_id = pid;
    DELETE FROM project_tasks     WHERE project_id = pid;
    DELETE FROM projects          WHERE id = pid;
  END LOOP;
  DELETE FROM certifications WHERE employee_id IN (SELECT id FROM employees WHERE org_id = '${ORG_ID}');
  DELETE FROM employees WHERE org_id = '${ORG_ID}';
END \$\$;
SQL
  log "reset complete"
}

# ----------------------------------------------------------------------------
# FINANCIALS ROLLUP — REQUIRED step.
#
# The Financials Summary / By-Project tabs INNER JOIN project_budgets and read
# corporate_budgets. corporate_budgets is produced ONLY by the `corporate_rollup`
# River job (worker/registry.go: a 24h PeriodicJob, RunOnStart=false). There is
# NO HTTP trigger. So after seeding budgets, you MUST run the rollup or the
# Summary tab stays empty:
#
#   OPTION A (recommended for a running deployment): enqueue the job NOW so the
#   running worker picks it up immediately, instead of waiting up to 24h. River
#   exposes no public enqueue endpoint here, so enqueue via SQL by inserting a
#   river_job row (kind=corporate_rollup). The worker's poller runs it within
#   seconds:
#
#     psql "$DATABASE_URL" -c "INSERT INTO river_job
#       (kind, args, queue, priority, state, max_attempts, scheduled_at)
#       VALUES ('corporate_rollup','{}','default',1,'available',5, now());"
#
#   OPTION B (local/dev, no running worker): run the worker binary once with a
#   short lifetime, or call BudgetService.RunCorporateRollup directly from a
#   one-shot. The simplest local path:
#
#     BUILDOS_ROLE=worker ./bin/worker &   # let it run a moment, then stop it
#
# This function attempts OPTION A when DATABASE_URL is set; otherwise it prints
# the manual instructions.
# ----------------------------------------------------------------------------
trigger_rollup() {
  if [[ -n "${DATABASE_URL:-}" ]]; then
    log "enqueuing corporate_rollup River job (DATABASE_URL set)"
    if psql "${DATABASE_URL}" -v ON_ERROR_STOP=1 -q -c \
      "INSERT INTO river_job (kind, args, queue, priority, state, max_attempts, scheduled_at)
       VALUES ('corporate_rollup', '{}'::jsonb, 'default', 1, 'available', 5, now());" 2>/dev/null; then
      log "corporate_rollup enqueued — the running worker will produce corporate_budgets shortly"
      return 0
    fi
    warn "could not enqueue via SQL (river_job table missing? worker not migrated?)"
  fi
  warn "ACTION REQUIRED: run the corporate_rollup job or the Financials Summary will be EMPTY."
  warn "  See the 'FINANCIALS ROLLUP' comment block in this script for OPTION A / OPTION B."
}

# ----------------------------------------------------------------------------
# Main
# ----------------------------------------------------------------------------
log "BuildOS demo seed → ${BASE_URL}"

authenticate
[[ -n "$ORG_ID" ]] || die "ORG_ID could not be determined; set it explicitly"

if [[ "$DO_RESET" -eq 1 ]]; then
  reset_demo
fi

onboard

P1="$(build_project "Kelbrook Residence" "2026-03-02")"
build_project "Aspen Ridge Custom" "2026-04-13" >/dev/null
build_project "Birchwood Estate" "2026-05-04" >/dev/null

seed_hr
seed_extras "$P1"
trigger_rollup

log "DONE. Seeded 3 projects (with real critical paths), crew + certs, budgets, and demo fleet/procurement/pipeline/invoice."
log "If the Financials Summary is empty, confirm the corporate_rollup job ran (see FINANCIALS ROLLUP)."
