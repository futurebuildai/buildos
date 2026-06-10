#!/usr/bin/env bash
# shellcheck disable=SC2016
# (^ the single-quoted $identifiers in this file are GraphQL variables and jq
#    filter arguments — literal BY DESIGN; they must never be shell-expanded.)
#
# teardown-kelbrook-legacy.sh — decommission the FAILED legacy "Kelbrook"
# deployment attempts: old Railway projects plus the stale Cloudflare DNS
# records that still point at them.
#
# SAFETY MODEL (the whole point of this script):
#   * EXPLICIT ids only. You hand it --railway-project-id / --cf-zone-id +
#     --cf-record-id values you collected yourself. There is NO name-pattern
#     matching and NO list-all-and-filter deletion — a typo can never widen
#     the blast radius beyond the ids you typed.
#   * Look before delete. Every target is FETCHED and PRINTED first (project
#     name + environments + services + domains; DNS record type + name +
#     content) so you see exactly what is about to die.
#   * Typed confirmation. Each deletion requires the resource's EXACT name,
#     read from /dev/tty (so it cannot be piped or scripted away). A mismatch
#     SKIPS that target and moves on — it never aborts the whole run.
#   * DNS first, then projects: records stop resolving before the services
#     they point at are destroyed, so nothing hands out an answer that leads
#     to a dying deployment.
#   * --dry-run fetches and prints everything (read-only calls only) and shows
#     the exact mutation/DELETE bodies it WOULD send — no prompts, no deletes.
#
# RAILWAY GRAPHQL CAVEAT: the public API docs are thin; the operation/field
# names below must be validated against the live introspected schema on the
# first credentialed run. Use --dry-run to inspect the bodies without sending
# any mutation.
#
# Where the ids come from (dashboard click-paths, ordering, post-checks):
# see the "Teardown checklist" (§8) in deploy/railway/README.md.
#
# Usage:
#   export RAILWAY_API_TOKEN=...      # required when --railway-project-id given
#   export CLOUDFLARE_API_TOKEN=...   # required when --cf-record-id given
#   deploy/railway/teardown-kelbrook-legacy.sh \
#     --cf-zone-id 023e105f4ecef8ad9ca31a8372d0c353 \
#     --cf-record-id 372e67954025e0ba6aaa6d586b9e0b59 \
#     --cf-record-id 5dd64a4f7e1b437d9d6a31e2177d4c41 \
#     --railway-project-id 11111111-2222-3333-4444-555555555555 \
#     --dry-run
#
# Flags:
#   --railway-project-id <id>  Railway project to delete (repeatable)
#   --cf-zone-id <id>          Cloudflare zone the record ids live in (one zone)
#   --cf-record-id <id>        Cloudflare DNS record to delete (repeatable)
#   --dry-run                  fetch + print only; no prompts, no deletions
#   -h | --help                show this header
#
# Tokens are read from the environment and ride in 0600 header files
# (curl -H @file), never external-command argv. They are needed even for
# --dry-run because the look-before-delete fetches are real (read-only) calls.
#
# Config (overridable from the environment):
#   RAILWAY_GRAPHQL_URL  default https://backboard.railway.com/graphql/v2
#   CLOUDFLARE_API_URL   default https://api.cloudflare.com/client/v4
set -euo pipefail

: "${RAILWAY_GRAPHQL_URL:=https://backboard.railway.com/graphql/v2}"
: "${CLOUDFLARE_API_URL:=https://api.cloudflare.com/client/v4}"

RAILWAY_PROJECT_IDS=()
CF_RECORD_IDS=()
CF_ZONE_ID=""
DRY_RUN=0
DELETED=()
SKIPPED=()

log()  { printf '\033[36m[teardown]\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[33m[teardown]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[31m[teardown]\033[0m %s\n' "$*" >&2; }
die()  { err "$@"; exit 1; }

# usage prints the doc header above (from the "# teardown-kelbrook-legacy.sh —"
# line down to `set -euo pipefail`) so the docs never drift from --help.
usage() { awk '/^# teardown-kelbrook-legacy\.sh/ { found = 1 } !found { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "${BASH_SOURCE[0]}"; }

need_value() { # need_value <flag> <value-or-empty>
  if [[ -z "${2:-}" ]]; then
    err "$1 requires a value"
    usage >&2
    exit 2
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --railway-project-id) need_value "$1" "${2:-}"; RAILWAY_PROJECT_IDS+=("$2"); shift 2 ;;
    --cf-zone-id)         need_value "$1" "${2:-}"; CF_ZONE_ID="$2";             shift 2 ;;
    --cf-record-id)       need_value "$1" "${2:-}"; CF_RECORD_IDS+=("$2");       shift 2 ;;
    --dry-run)            DRY_RUN=1; shift ;;
    -h|--help)            usage; exit 0 ;;
    *) err "unknown argument: $1"; usage >&2; exit 2 ;;
  esac
done

if [[ "${#RAILWAY_PROJECT_IDS[@]}" -eq 0 && "${#CF_RECORD_IDS[@]}" -eq 0 ]]; then
  err "nothing to do: pass --railway-project-id and/or --cf-zone-id + --cf-record-id"
  usage >&2
  exit 2
fi
if [[ "${#CF_RECORD_IDS[@]}" -gt 0 && -z "${CF_ZONE_ID}" ]]; then
  err "--cf-record-id requires --cf-zone-id (records are addressed within a zone)"
  usage >&2
  exit 2
fi
if [[ -n "${CF_ZONE_ID}" && "${#CF_RECORD_IDS[@]}" -eq 0 ]]; then
  err "--cf-zone-id given without any --cf-record-id — this script never lists a zone to pick targets; pass explicit record ids"
  usage >&2
  exit 2
fi

command -v curl >/dev/null 2>&1 || die "curl is required but not on PATH"
command -v jq   >/dev/null 2>&1 || die "jq is required but not on PATH"

# Tokens are required even for --dry-run: the look-before-delete fetches are
# real (read-only) API calls.
if [[ "${#RAILWAY_PROJECT_IDS[@]}" -gt 0 ]]; then
  [[ -n "${RAILWAY_API_TOKEN:-}" ]] || die "RAILWAY_API_TOKEN must be exported to inspect/delete Railway projects"
fi
if [[ "${#CF_RECORD_IDS[@]}" -gt 0 ]]; then
  [[ -n "${CLOUDFLARE_API_TOKEN:-}" ]] || die "CLOUDFLARE_API_TOKEN must be exported to inspect/delete DNS records"
fi

umask 077
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/buildos-teardown.XXXXXX")"
trap 'rm -rf "${WORK_DIR}"' EXIT

# Bearer tokens ride in 0600 header files (curl -H @file), never argv.
RAILWAY_AUTH_HEADER_FILE="${WORK_DIR}/railway.header"
CF_AUTH_HEADER_FILE="${WORK_DIR}/cloudflare.header"
if [[ -n "${RAILWAY_API_TOKEN:-}" ]]; then
  printf 'Authorization: Bearer %s\n' "${RAILWAY_API_TOKEN}" > "${RAILWAY_AUTH_HEADER_FILE}"
fi
if [[ -n "${CLOUDFLARE_API_TOKEN:-}" ]]; then
  printf 'Authorization: Bearer %s\n' "${CLOUDFLARE_API_TOKEN}" > "${CF_AUTH_HEADER_FILE}"
fi

# NOTE: best-effort field names against thin public docs — validate via
# introspection on the first credentialed run (see header caveat).
Q_PROJECT='query buildosTeardownProject($id: String!) { project(id: $id) { id name environments { edges { node { id name } } } services { edges { node { id name } } } } }'
Q_DOMAINS='query buildosTeardownDomains($projectId: String!, $environmentId: String!, $serviceId: String!) { domains(projectId: $projectId, environmentId: $environmentId, serviceId: $serviceId) { customDomains { domain } serviceDomains { domain } } }'
M_PROJECT_DELETE='mutation buildosProjectDelete($id: String!) { projectDelete(id: $id) }'

# gql <label> <query> <vars-json-file>
# One Railway GraphQL round-trip; ALWAYS sends (used for read-only lookups —
# mutations are gated separately behind --dry-run + typed confirmation).
# Nothing secret travels in variables here (resource ids only).
gql() {
  local label="$1" query="$2" vars_file="$3" body resp
  body="${WORK_DIR}/gql-body.json"
  jq -n --arg q "${query}" --slurpfile v "${vars_file}" '{query: $q, variables: $v[0]}' > "${body}"
  if ! resp="$(curl -sS --max-time 60 -X POST \
        -H 'Content-Type: application/json' \
        -H "@${RAILWAY_AUTH_HEADER_FILE}" \
        --data-binary "@${body}" \
        "${RAILWAY_GRAPHQL_URL}")"; then
    err "${label}: HTTP request to Railway API failed"
    return 1
  fi
  if ! jq -e . >/dev/null 2>&1 <<<"${resp}"; then
    err "${label}: non-JSON response from Railway API (auth or transport failure?)"
    return 1
  fi
  if jq -e '.errors and (.errors | length > 0)' >/dev/null 2>&1 <<<"${resp}"; then
    err "${label}: GraphQL errors (field names may need validating against the live schema — see header):"
    jq -r '.errors[] | "  - " + .message' <<<"${resp}" >&2
    return 1
  fi
  printf '%s' "${resp}"
}

cf_api() { # cf_api <method> <path-under-/client/v4>
  curl -sS --max-time 60 -X "$1" -H "@${CF_AUTH_HEADER_FILE}" "${CLOUDFLARE_API_URL}/$2"
}

# confirm_exact <expected-name> <what>
# Typed confirmation read from the controlling terminal so it cannot be piped
# in. Returns 0 only when the operator types the resource's EXACT name.
confirm_exact() {
  local expected="$1" what="$2" answer=""
  # Probe with a real open: -e/-r on /dev/tty pass even when no controlling
  # terminal is attached (CI, setsid, cron) — only an open attempt is honest.
  if ! ( : < /dev/tty ) 2>/dev/null; then
    die "no controlling terminal — refusing to delete without interactive confirmation (use --dry-run to inspect)"
  fi
  printf '\nType the EXACT name of this %s to DELETE it (anything else skips):\n  expected: %s\n  > ' \
    "${what}" "${expected}" > /dev/tty
  IFS= read -r answer < /dev/tty || true
  [[ "${answer}" == "${expected}" ]]
}

# ── Cloudflare DNS records (phase 1 — DNS dies first) ────────────────────────

teardown_cf_record() { # teardown_cf_record <record-id>
  local rid="$1" resp rec_type rec_name rec_content
  log "Cloudflare DNS record ${rid} (zone ${CF_ZONE_ID}):"
  if ! resp="$(cf_api GET "zones/${CF_ZONE_ID}/dns_records/${rid}")"; then
    warn "  fetch failed (transport) — skipping"
    SKIPPED+=("cf-record ${rid} (fetch failed)")
    return 0
  fi
  if ! jq -e '.success == true' >/dev/null 2>&1 <<<"${resp}"; then
    warn "  Cloudflare API error: $(jq -c '.errors // []' <<<"${resp}" 2>/dev/null || printf '?') — skipping"
    SKIPPED+=("cf-record ${rid} (API error)")
    return 0
  fi
  rec_type="$(jq -r '.result.type' <<<"${resp}")"
  rec_name="$(jq -r '.result.name' <<<"${resp}")"
  rec_content="$(jq -r '.result.content' <<<"${resp}")"
  log "  ${rec_type} ${rec_name} -> ${rec_content}"

  if (( DRY_RUN )); then
    log "  DRY-RUN: would require typing '${rec_name}', then send:"
    log "    DELETE ${CLOUDFLARE_API_URL}/zones/${CF_ZONE_ID}/dns_records/${rid}"
    SKIPPED+=("cf-record ${rec_type} ${rec_name} (dry-run)")
    return 0
  fi
  if ! confirm_exact "${rec_name}" "DNS record"; then
    warn "  confirmation mismatch — skipping ${rec_name}"
    SKIPPED+=("cf-record ${rec_type} ${rec_name} (not confirmed)")
    return 0
  fi
  if resp="$(cf_api DELETE "zones/${CF_ZONE_ID}/dns_records/${rid}")" \
     && jq -e '.success == true' >/dev/null 2>&1 <<<"${resp}"; then
    log "  deleted ${rec_type} ${rec_name}"
    DELETED+=("cf-record ${rec_type} ${rec_name} (${rid})")
  else
    warn "  DELETE failed: $(jq -c '.errors // []' <<<"${resp}" 2>/dev/null || printf '?')"
    SKIPPED+=("cf-record ${rec_type} ${rec_name} (delete failed)")
  fi
}

# ── Railway projects (phase 2 — after DNS stopped resolving to them) ────────

# print_project_domains — best-effort domain listing for the look-before-delete
# block. The domains query shape is per service instance and schema-uncertain,
# so failures degrade to a pointer at the dashboard instead of blocking.
print_project_domains() { # print_project_domains <project-id> <project-json>
  local pid="$1" detail="$2" vars="${WORK_DIR}/vars.json" env_id svc_id resp domains
  while IFS=$'\t' read -r env_id svc_id; do
    [[ -n "${env_id}" && -n "${svc_id}" ]] || continue
    jq -n --arg p "${pid}" --arg e "${env_id}" --arg s "${svc_id}" \
      '{projectId: $p, environmentId: $e, serviceId: $s}' > "${vars}"
    if resp="$(gql "domains lookup (${svc_id}@${env_id})" "${Q_DOMAINS}" "${vars}" 2>/dev/null)"; then
      domains="$(jq -r '[(.data.domains.customDomains[]?.domain), (.data.domains.serviceDomains[]?.domain)] | join(", ")' <<<"${resp}")"
      if [[ -n "${domains}" ]]; then
        log "    domains: ${domains}"
      fi
    else
      log "    domains: lookup failed for one service instance — check the dashboard"
    fi
  done < <(jq -r '
      .data.project as $prj
      | $prj.environments.edges[]?.node.id as $e
      | $prj.services.edges[]?.node.id as $s
      | [$e, $s] | @tsv' <<<"${detail}")
}

teardown_railway_project() { # teardown_railway_project <project-id>
  local pid="$1" vars="${WORK_DIR}/vars.json" resp name envs svcs
  log "Railway project ${pid}:"
  jq -n --arg id "${pid}" '{id: $id}' > "${vars}"
  if ! resp="$(gql "project lookup (${pid})" "${Q_PROJECT}" "${vars}")"; then
    warn "  fetch failed — skipping"
    SKIPPED+=("railway-project ${pid} (fetch failed)")
    return 0
  fi
  if ! jq -e '.data.project.id' >/dev/null 2>&1 <<<"${resp}"; then
    warn "  project not found (already deleted?) — skipping"
    SKIPPED+=("railway-project ${pid} (not found)")
    return 0
  fi
  name="$(jq -r '.data.project.name' <<<"${resp}")"
  envs="$(jq -r '[.data.project.environments.edges[]?.node.name] | join(", ")' <<<"${resp}")"
  svcs="$(jq -r '[.data.project.services.edges[]?.node.name] | join(", ")' <<<"${resp}")"
  log "  name        : ${name}"
  log "  environments: ${envs:-none}"
  log "  services    : ${svcs:-none}"
  print_project_domains "${pid}" "${resp}"

  if (( DRY_RUN )); then
    log "  DRY-RUN: would require typing '${name}', then send:"
    log "    mutation: ${M_PROJECT_DELETE}"
    log "    variables: {\"id\": \"${pid}\"}"
    SKIPPED+=("railway-project '${name}' (dry-run)")
    return 0
  fi
  if ! confirm_exact "${name}" "Railway project"; then
    warn "  confirmation mismatch — skipping '${name}'"
    SKIPPED+=("railway-project '${name}' (not confirmed)")
    return 0
  fi
  jq -n --arg id "${pid}" '{id: $id}' > "${vars}"
  if gql "projectDelete(${name})" "${M_PROJECT_DELETE}" "${vars}" >/dev/null; then
    log "  deleted project '${name}'"
    DELETED+=("railway-project '${name}' (${pid})")
  else
    warn "  projectDelete failed — skipping '${name}'"
    SKIPPED+=("railway-project '${name}' (delete failed)")
  fi
}

print_summary() {
  local item
  log "──────────── summary ────────────"
  if [[ "${#DELETED[@]}" -gt 0 ]]; then
    log "deleted:"
    for item in "${DELETED[@]}"; do log "  - ${item}"; done
  else
    log "deleted: none"
  fi
  if [[ "${#SKIPPED[@]}" -gt 0 ]]; then
    log "skipped:"
    for item in "${SKIPPED[@]}"; do log "  - ${item}"; done
  else
    log "skipped: none"
  fi
}

main() {
  local rid pid mode=""
  if (( DRY_RUN )); then mode=" [DRY-RUN: read-only fetches; no prompts, no deletions]"; fi
  log "legacy Kelbrook teardown${mode}"

  # Order is deliberate: DNS first, so nothing resolves to a service that is
  # about to be destroyed; Railway projects second.
  if [[ "${#CF_RECORD_IDS[@]}" -gt 0 ]]; then
    log "phase 1/2 — Cloudflare DNS records"
    for rid in "${CF_RECORD_IDS[@]}"; do
      teardown_cf_record "${rid}"
    done
  fi
  if [[ "${#RAILWAY_PROJECT_IDS[@]}" -gt 0 ]]; then
    log "phase 2/2 — Railway projects"
    for pid in "${RAILWAY_PROJECT_IDS[@]}"; do
      teardown_railway_project "${pid}"
    done
  fi
  print_summary
}

main
