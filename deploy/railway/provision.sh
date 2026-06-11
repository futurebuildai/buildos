#!/usr/bin/env bash
# shellcheck disable=SC2016
# (^ the single-quoted $identifiers in this file are GraphQL variables, jq
#    filter arguments, and Railway's ${{...}} reference syntax — they are
#    literal BY DESIGN and must never be shell-expanded.)
#
# provision.sh — idempotent provisioning of one BuildOS fork's Railway estate.
#
# BuildOS is single-tenant per customer fork (ADR-002): ONE Railway project per
# fork (default "buildos-fork0") with one environment per stage ("staging",
# "production") and, in EACH environment, two services running the same GHCR
# image — BUILDOS_ROLE selects the binary:
#   server — HTTP API + web console. Railway injects $PORT (the app reads it,
#            default 8080); healthcheck is GET /ready.
#   worker — River job daemon; serves /ready + /metrics on its $PORT too.
# plus a Railway managed Postgres per environment. Migrations are NOT a Railway
# service — each deploy workflow runs the migrate role as a one-shot
# `docker run -e BUILDOS_ROLE=migrate` from the GitHub runner against that
# environment's DATABASE_URL (docs/deploy-runbook.md §4).
#
# IDEMPOTENT: every resource is looked up by name first (or pinned outright
# via --project-id) and only created when missing, so re-running after a
# partial failure is safe. Variable maps (ONE variableCollectionUpsert per
# service per environment), registry credentials, and the /ready healthcheck
# are (re)applied on every run; the instance IMAGE is only set when this
# script creates the service, so a re-run never clobbers a digest pinned
# later by the deploy workflows.
#
# RAILWAY GRAPHQL CAVEAT (read before the first real run): the public API docs
# are thin. The operation/field names below are best-effort and MUST be
# validated against the live introspected schema on the FIRST credentialed
# run. Always start with --dry-run — it prints every GraphQL body (secret
# values redacted) without sending anything.
#
# Secrets: each environment gets ITS OWN `make fork-init` output dir — a fresh
# keypair for production, NEVER shared with staging. Passing the same dir for
# two environments is a hard error. Each dir must contain private.pem,
# public.pem, vault_master_key.txt, bootstrap_token.txt and must NOT be
# world-readable. Secret values travel via files (jq --rawfile, curl
# --data-binary @file, curl -H @file), never external-command argv, and are
# redacted in every log line including --dry-run output. The GHCR pull token
# (--ghcr-pull-token-file) follows the same rules: file-borne, never argv,
# redacted everywhere, and the file must not be world-readable.
#
# Usage:
#   export RAILWAY_API_TOKEN=...        # required for real runs
#   deploy/railway/provision.sh \
#     --secrets-dir staging=./forks/fork0-staging/secrets \
#     --secrets-dir production=./forks/fork0-production/secrets \
#     --staging-domain staging.futurebuild.ai \
#     --production-domain app.futurebuild.ai \
#     --ghcr-pull-username <machine-user> \
#     --ghcr-pull-token-file <path-to-read-packages-pat>
#
# Flags:
#   --project-name <name>       Railway project name (default: buildos-fork0)
#   --project-id <id>           use this existing Railway project and SKIP the
#                               lookup/create path entirely. REQUIRED with
#                               TEAM/workspace tokens: they cannot run the
#                               `me { projects }` lookup (Not Authorized) —
#                               create the empty project once in the dashboard
#                               and pass its ID. Personal tokens may omit this
#                               and use the by-name lookup/create path.
#   --environments <a,b,...>    environments to provision (default: staging,production)
#   --image <ref>               initial image (default: ghcr.io/futurebuildai/buildos:latest)
#   --ghcr-pull-username <user> GHCR pull username (requires --ghcr-pull-token-file).
#                               GHCR images are PRIVATE by default — without
#                               registry credentials Railway cannot pull the
#                               image, every roll 401s, and /ready never goes 200.
#   --ghcr-pull-token-file <p>  file holding a GitHub PAT (classic) scoped to
#                               read:packages ONLY (machine account recommended).
#                               Must not be world-readable; the value is
#                               redacted in all output, including --dry-run.
#   --secrets-dir <env>=<dir>   fork-init output dir for <env> (repeatable).
#                               A bare <dir> is allowed only when provisioning
#                               exactly one environment.
#   --staging-domain <fqdn>     custom domain for the staging server service
#   --production-domain <fqdn>  custom domain for the production server service
#   --dry-run                   print every GraphQL operation; send nothing
#   -h | --help                 show this header
#
# Output (real runs only): provision-output.env next to this script — the
# project/environment/service IDs under the EXACT GitHub Actions secret names
# the deploy workflows consume, plus next-step instructions. The IDs are not
# credentials, but don't commit the file.
#
# Config (overridable from the environment):
#   RAILWAY_GRAPHQL_URL  API endpoint (default https://backboard.railway.com/graphql/v2)
#   PROVISION_OUTPUT     where to write the ID manifest
#                        (default <script dir>/provision-output.env)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

: "${RAILWAY_GRAPHQL_URL:=https://backboard.railway.com/graphql/v2}"
: "${PROVISION_OUTPUT:=${SCRIPT_DIR}/provision-output.env}"

PROJECT_NAME="buildos-fork0"
PROJECT_ID_ARG=""
ENVIRONMENTS_CSV="staging,production"
IMAGE="ghcr.io/futurebuildai/buildos:latest"
GHCR_PULL_USERNAME=""
GHCR_PULL_TOKEN_FILE=""
STAGING_DOMAIN=""
PRODUCTION_DOMAIN=""
DRY_RUN=0
SECRETS_DIR_ARGS=()

# TRUSTED_PROXY_CIDRS placeholder — Railway's edge proxies every request, so
# the app must trust the proxy peer before honoring X-Forwarded-For. These
# CIDRs (CGNAT + RFC1918 ranges Railway is known to use internally) are a
# PLACEHOLDER. VERIFY-AT-FIRST-DEPLOY: check the server's boot log / the
# observed peer address and tighten this on BOTH services in BOTH environments.
TRUSTED_PROXY_CIDRS_PLACEHOLDER="100.64.0.0/10,10.0.0.0/8"

log()  { printf '\033[36m[provision]\033[0m %s\n' "$*" >&2; }
warn() { printf '\033[33m[provision]\033[0m %s\n' "$*" >&2; }
err()  { printf '\033[31m[provision]\033[0m %s\n' "$*" >&2; }
die()  { err "$@"; exit 1; }

# usage prints the doc header above (from the "# provision.sh —" line down to
# `set -euo pipefail`) so the docs never drift from the --help text.
usage() { awk '/^# provision\.sh/ { found = 1 } !found { next } /^#/ { sub(/^# ?/, ""); print; next } { exit }' "${BASH_SOURCE[0]}"; }

need_value() { # need_value <flag> <value-or-empty>
  if [[ -z "${2:-}" ]]; then
    err "$1 requires a value"
    usage >&2
    exit 2
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --project-name)      need_value "$1" "${2:-}"; PROJECT_NAME="$2";        shift 2 ;;
    --project-id)        need_value "$1" "${2:-}"; PROJECT_ID_ARG="$2";      shift 2 ;;
    --environments)      need_value "$1" "${2:-}"; ENVIRONMENTS_CSV="$2";    shift 2 ;;
    --image)             need_value "$1" "${2:-}"; IMAGE="$2";               shift 2 ;;
    --ghcr-pull-username)   need_value "$1" "${2:-}"; GHCR_PULL_USERNAME="$2";   shift 2 ;;
    --ghcr-pull-token-file) need_value "$1" "${2:-}"; GHCR_PULL_TOKEN_FILE="$2"; shift 2 ;;
    --secrets-dir)       need_value "$1" "${2:-}"; SECRETS_DIR_ARGS+=("$2"); shift 2 ;;
    --staging-domain)    need_value "$1" "${2:-}"; STAGING_DOMAIN="$2";      shift 2 ;;
    --production-domain) need_value "$1" "${2:-}"; PRODUCTION_DOMAIN="$2";   shift 2 ;;
    --dry-run)           DRY_RUN=1; shift ;;
    -h|--help)           usage; exit 0 ;;
    *) err "unknown argument: $1"; usage >&2; exit 2 ;;
  esac
done

command -v curl >/dev/null 2>&1 || die "curl is required but not on PATH"
command -v jq   >/dev/null 2>&1 || die "jq is required but not on PATH (1.6+ for --rawfile)"

if (( ! DRY_RUN )); then
  [[ -n "${RAILWAY_API_TOKEN:-}" ]] || die "RAILWAY_API_TOKEN must be exported (or use --dry-run)"
fi

# GHCR pull credentials: both flags or neither, and the token file gets the
# same world-readable refusal as the secrets dirs (it is a credential).
if [[ -n "${GHCR_PULL_USERNAME}" || -n "${GHCR_PULL_TOKEN_FILE}" ]]; then
  [[ -n "${GHCR_PULL_USERNAME}" && -n "${GHCR_PULL_TOKEN_FILE}" ]] \
    || die "--ghcr-pull-username and --ghcr-pull-token-file must be given together"
  [[ -f "${GHCR_PULL_TOKEN_FILE}" && -r "${GHCR_PULL_TOKEN_FILE}" ]] \
    || die "GHCR pull token file is not a readable file: ${GHCR_PULL_TOKEN_FILE}"
  _tok_perm="$(stat -c '%a' "${GHCR_PULL_TOKEN_FILE}")"
  _tok_other="${_tok_perm: -1}"
  if (( _tok_other & 4 )); then
    die "GHCR pull token file is world-readable (mode ${_tok_perm}): ${GHCR_PULL_TOKEN_FILE} — run: chmod o-rwx '${GHCR_PULL_TOKEN_FILE}'"
  fi
fi

# Split the environment list, dropping empty fragments from stray commas.
ENVIRONMENTS=()
IFS=',' read -r -a _raw_envs <<<"${ENVIRONMENTS_CSV}"
for _e in "${_raw_envs[@]}"; do
  [[ -n "${_e}" ]] && ENVIRONMENTS+=("${_e}")
done
[[ "${#ENVIRONMENTS[@]}" -ge 1 ]] || die "--environments produced an empty list: '${ENVIRONMENTS_CSV}'"

env_listed() { # env_listed <name> — is <name> in the --environments list?
  local e
  for e in "${ENVIRONMENTS[@]}"; do
    if [[ "$e" == "$1" ]]; then return 0; fi
  done
  return 1
}

# env_suffix maps an environment name to its GitHub-secret suffix:
# staging → STAGING, production → PRODUCTION (non-alnum chars become _).
env_suffix() { printf '%s' "$1" | tr '[:lower:]' '[:upper:]' | tr -c '[:alnum:]' '_'; }

if [[ -n "${STAGING_DOMAIN}" ]] && ! env_listed staging; then
  warn "--staging-domain given but 'staging' is not in --environments — ignored"
fi
if [[ -n "${PRODUCTION_DOMAIN}" ]] && ! env_listed production; then
  warn "--production-domain given but 'production' is not in --environments — ignored"
fi

# ── Secrets dirs ─────────────────────────────────────────────────────────────
# One fork-init output dir PER environment. Reuse across environments is a
# hard error: production must run on a fresh keypair, never staging's.

validate_secrets_dir() { # validate_secrets_dir <env> <dir>
  local env_name="$1" dir="$2" perm other f
  [[ -d "${dir}" ]] || die "secrets dir for '${env_name}' is not a directory: ${dir}"
  perm="$(stat -c '%a' "${dir}")"
  other="${perm: -1}"
  if (( other & 4 )); then
    die "secrets dir for '${env_name}' is world-readable (mode ${perm}): ${dir} — run: chmod o-rwx '${dir}'"
  fi
  for f in private.pem public.pem vault_master_key.txt bootstrap_token.txt; do
    [[ -f "${dir}/${f}" && -r "${dir}/${f}" ]] \
      || die "secrets dir for '${env_name}' is missing ${f} — point --secrets-dir at a 'make fork-init' OUT dir"
  done
}

declare -A SECRETS_DIRS=()
declare -A _seen_dirs=()
for _spec in "${SECRETS_DIR_ARGS[@]}"; do
  if [[ "${_spec}" == *=* ]]; then
    _env_name="${_spec%%=*}"
    _dir="${_spec#*=}"
  else
    if [[ "${#ENVIRONMENTS[@]}" -ne 1 ]]; then
      die "bare --secrets-dir '${_spec}' is ambiguous with multiple environments; use --secrets-dir <env>=<dir>"
    fi
    _env_name="${ENVIRONMENTS[0]}"
    _dir="${_spec}"
  fi
  env_listed "${_env_name}" || die "--secrets-dir names unknown environment '${_env_name}' (not in: ${ENVIRONMENTS_CSV})"
  _dir="$(cd "${_dir}" 2>/dev/null && pwd)" || die "secrets dir does not exist: ${_spec}"
  validate_secrets_dir "${_env_name}" "${_dir}"
  if [[ -n "${_seen_dirs[${_dir}]:-}" ]]; then
    die "secrets dir reuse: ${_dir} given for both '${_seen_dirs[${_dir}]}' and '${_env_name}'." \
        "Each environment needs its own fork-init output (fresh keypair for prod, never shared)."
  fi
  _seen_dirs[${_dir}]="${_env_name}"
  SECRETS_DIRS[${_env_name}]="${_dir}"
done

for _env_name in "${ENVIRONMENTS[@]}"; do
  [[ -n "${SECRETS_DIRS[${_env_name}]:-}" ]] \
    || die "no --secrets-dir for environment '${_env_name}' — run: make fork-init OUT=./forks/fork0-${_env_name}/secrets ... then pass --secrets-dir ${_env_name}=<dir>"
done

# ── GraphQL plumbing ─────────────────────────────────────────────────────────

umask 077
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/buildos-provision.XXXXXX")"
trap 'rm -rf "${WORK_DIR}"' EXIT

# The bearer token rides in a 0600 header file (curl -H @file), never argv.
AUTH_HEADER_FILE="${WORK_DIR}/auth.header"
if [[ -n "${RAILWAY_API_TOKEN:-}" ]]; then
  printf 'Authorization: Bearer %s\n' "${RAILWAY_API_TOKEN}" > "${AUTH_HEADER_FILE}"
fi

# Raw body of the MOST RECENT gql round-trip, kept so callers can branch on
# specific GraphQL errors (team-token "Not Authorized" on `me`; the
# already-exists family from customDomainCreate). Reset at the top of every
# gql call so stale errors can never match.
GQL_LAST_RESPONSE="${WORK_DIR}/gql-last-response.json"
: > "${GQL_LAST_RESPONSE}"

# Registry pull credentials (GHCR images are PRIVATE by default): when the
# --ghcr-pull-* flags are given, this {username, password} object is injected
# as `registryCredentials` into ServiceCreateInput and
# ServiceInstanceUpdateInput so Railway can pull the image. The token rides
# via jq --rawfile (never argv); the redacted twin is what every log line and
# --dry-run body shows. Field names carry the same validate-on-first-run
# caveat as every other mutation here (see header).
RC_FILE="${WORK_DIR}/registry-credentials.json"
RC_REDACTED_FILE="${WORK_DIR}/registry-credentials-redacted.json"
if [[ -n "${GHCR_PULL_USERNAME}" ]]; then
  jq -n --arg u "${GHCR_PULL_USERNAME}" --rawfile t "${GHCR_PULL_TOKEN_FILE}" \
    '{username: $u, password: ($t | rtrimstr("\n"))}' > "${RC_FILE}"
  jq -n --arg u "${GHCR_PULL_USERNAME}" \
    '{username: $u, password: "[REDACTED]"}' > "${RC_REDACTED_FILE}"
else
  printf 'null\n' > "${RC_FILE}"
  printf 'null\n' > "${RC_REDACTED_FILE}"
fi

# NOTE: every operation below is best-effort against thin public docs —
# validate the field names via introspection on the first credentialed run.
# VALIDATED ON THE FIRST CREDENTIALED RUN (2026-06-10): projects live under
# the WORKSPACE, not under `me.projects` (which returns empty for
# workspace-homed accounts), and ProjectCreateInput REQUIRES workspaceId.
# Q_WORKSPACES/Q_PROJECTS below reflect the live schema. TEAM/workspace
# tokens still cannot query `me` at all — use --project-id for those.
Q_WORKSPACES='query buildosWorkspaces { me { workspaces { id name } } }'
Q_PROJECTS='query buildosProjects($workspaceId: String!) { projects(workspaceId: $workspaceId) { edges { node { id name } } } }'
Q_PROJECT_DETAIL='query buildosProjectDetail($id: String!) { project(id: $id) { id name environments { edges { node { id name } } } services { edges { node { id name } } } } }'
# Q_DOMAINS surfaces the DNS (CNAME) target the Cloudflare step needs; the
# `status { dnsRecords ... }` selection is the schema-uncertain part — its
# failure degrades to printing the raw response (see print_domain_dns_target).
Q_DOMAINS='query buildosDomains($projectId: String!, $environmentId: String!, $serviceId: String!) { domains(projectId: $projectId, environmentId: $environmentId, serviceId: $serviceId) { customDomains { id domain status { dnsRecords { hostlabel requiredValue } } } } }'
M_PROJECT_CREATE='mutation buildosProjectCreate($input: ProjectCreateInput!) { projectCreate(input: $input) { id } }'
M_ENV_CREATE='mutation buildosEnvironmentCreate($input: EnvironmentCreateInput!) { environmentCreate(input: $input) { id } }'
M_SERVICE_CREATE='mutation buildosServiceCreate($input: ServiceCreateInput!) { serviceCreate(input: $input) { id } }'
M_VARIABLE_COLLECTION_UPSERT='mutation buildosVariableCollectionUpsert($input: VariableCollectionUpsertInput!) { variableCollectionUpsert(input: $input) }'
M_SVC_INSTANCE_UPDATE='mutation buildosServiceInstanceUpdate($serviceId: String!, $environmentId: String!, $input: ServiceInstanceUpdateInput!) { serviceInstanceUpdate(serviceId: $serviceId, environmentId: $environmentId, input: $input) }'
M_CUSTOM_DOMAIN_CREATE='mutation buildosCustomDomainCreate($input: CustomDomainCreateInput!) { customDomainCreate(input: $input) { id domain } }'

# gql <label> <query> <vars-json-file> <redacted-vars-json>
# One GraphQL round-trip. The variables travel via a file (--data-binary
# @file) so secret material never appears in external-command argv. In
# --dry-run the operation (query + REDACTED variables) is printed and an empty
# data envelope is returned so callers keep walking the create path end to end.
gql() {
  local label="$1" query="$2" vars_file="$3" redacted="$4" body resp
  : > "${GQL_LAST_RESPONSE}" # reset so error-matching helpers never see a stale body
  if (( DRY_RUN )); then
    {
      printf 'DRY-RUN GraphQL %s\n' "${label}"
      printf '  query    : %s\n' "${query}"
      # compact to one line for readability; fall back to verbatim
      printf '  variables: %s\n' "$(jq -c . <<<"${redacted}" 2>/dev/null || printf '%s' "${redacted}")"
    } >&2
    printf '{"data":{}}'
    return 0
  fi
  body="${WORK_DIR}/gql-body.json"
  jq -n --arg q "${query}" --slurpfile v "${vars_file}" '{query: $q, variables: $v[0]}' > "${body}"
  if ! resp="$(curl -sS --max-time 60 -X POST \
        -H 'Content-Type: application/json' \
        -H "@${AUTH_HEADER_FILE}" \
        --data-binary "@${body}" \
        "${RAILWAY_GRAPHQL_URL}")"; then
    err "${label}: HTTP request to Railway API failed"
    return 1
  fi
  if ! jq -e . >/dev/null 2>&1 <<<"${resp}"; then
    err "${label}: non-JSON response from Railway API (auth or transport failure?)"
    return 1
  fi
  # Keep the raw body (0600 workdir file) so callers can branch on specific
  # error messages; it is never logged wholesale (may echo variables back).
  printf '%s' "${resp}" > "${GQL_LAST_RESPONSE}"
  if jq -e '.errors and (.errors | length > 0)' >/dev/null 2>&1 <<<"${resp}"; then
    err "${label}: GraphQL errors (field names may need validating against the live schema — see header):"
    jq -r '.errors[] | "  - " + .message' <<<"${resp}" >&2
    return 1
  fi
  printf '%s' "${resp}"
}

# last_gql_error_matches <ERE> — case-insensitive match against the error
# messages of the MOST RECENT gql round-trip. Empty/absent body (transport
# failure, dry-run) never matches.
last_gql_error_matches() {
  jq -r '[.errors[]?.message // empty] | join("\n")' "${GQL_LAST_RESPONSE}" 2>/dev/null \
    | grep -qiE "$1"
}

# ── Resource ensure-functions (lookup by name, create only if missing) ──────

declare -A ENV_IDS=()
declare -A SVC_IDS=()
declare -A SVC_CREATED=()
MANUAL_FOLLOWUPS=()
PROJECT_ID=""
PROJECT_DETAIL='{"data":{}}'

# ensure_project — two paths:
#   * --project-id given: trust it and SKIP lookup/create entirely. This is
#     the path for TEAM/workspace tokens, which cannot query `me` at all.
#   * otherwise (personal tokens): discover the workspace via
#     `me { workspaces }`, look the project up by name in that workspace
#     (projects(workspaceId:) — me.projects is EMPTY for workspace-homed
#     accounts), and create it (with the required workspaceId) when missing.
ensure_project() {
  local vars="${WORK_DIR}/vars.json" resp id ws_id ws_count
  if [[ -n "${PROJECT_ID_ARG}" ]]; then
    PROJECT_ID="${PROJECT_ID_ARG}"
    log "using existing project ${PROJECT_ID} (--project-id) — lookup/create skipped (team-token-safe path)"
    return 0
  fi
  jq -n '{}' > "${vars}"
  if ! resp="$(gql "workspaces lookup" "${Q_WORKSPACES}" "${vars}" '{}')"; then
    if last_gql_error_matches 'not authorized'; then
      err "Railway rejected the 'me' lookup with Not Authorized."
      err "TEAM/workspace tokens cannot query 'me' — only PERSONAL tokens can."
      err "Fix (the recommended path for team tokens): create the empty project"
      err "'${PROJECT_NAME}' once in the Railway dashboard, then re-run with"
      err "--project-id <its-id> to skip the lookup/create entirely."
      exit 2
    fi
    die "workspace lookup failed"
  fi
  ws_count="$(jq -r '.data.me.workspaces | length' <<<"${resp}" 2>/dev/null || printf '0')"
  if (( DRY_RUN )) && [[ "${ws_count}" == "0" || -z "${ws_count}" ]]; then
    ws_id="DRY_RUN_WORKSPACE_ID"
  elif [[ "${ws_count}" == "1" ]]; then
    ws_id="$(jq -r '.data.me.workspaces[0].id' <<<"${resp}")"
  else
    err "expected exactly one Railway workspace, found ${ws_count}:"
    jq -r '.data.me.workspaces[] | "  - " + .name + " (" + .id + ")"' <<<"${resp}" >&2 || true
    die "ambiguous workspace — create the project in the dashboard and re-run with --project-id"
  fi
  log "workspace ${ws_id}"

  jq -n --arg w "${ws_id}" '{workspaceId: $w}' > "${vars}"
  resp="$(gql "projects lookup (workspace)" "${Q_PROJECTS}" "${vars}" "$(<"${vars}")")" \
    || die "project lookup failed"
  id="$(jq -r --arg n "${PROJECT_NAME}" \
        '.data.projects.edges[]?.node | select(.name == $n) | .id' <<<"${resp}" | head -n1)"
  if [[ -n "${id}" ]]; then
    log "project '${PROJECT_NAME}' already exists (${id}) — reusing"
    PROJECT_ID="${id}"
    return 0
  fi
  jq -n --arg name "${PROJECT_NAME}" --arg w "${ws_id}" \
    '{input: {name: $name, workspaceId: $w}}' > "${vars}"
  resp="$(gql "projectCreate(${PROJECT_NAME})" "${M_PROJECT_CREATE}" "${vars}" "$(<"${vars}")")" \
    || die "projectCreate(${PROJECT_NAME}) failed"
  id="$(jq -r '.data.projectCreate.id // empty' <<<"${resp}")"
  if [[ -z "${id}" ]]; then
    (( DRY_RUN )) || die "projectCreate returned no id"
    id="DRY_RUN_PROJECT_ID"
  fi
  PROJECT_ID="${id}"
  log "created project '${PROJECT_NAME}' (${PROJECT_ID})"
}

load_project_detail() {
  local vars="${WORK_DIR}/vars.json"
  jq -n --arg id "${PROJECT_ID}" '{id: $id}' > "${vars}"
  PROJECT_DETAIL="$(gql "project detail" "${Q_PROJECT_DETAIL}" "${vars}" "$(<"${vars}")")" \
    || die "project detail lookup failed"
}

ensure_environment() { # ensure_environment <name>
  local name="$1" vars="${WORK_DIR}/vars.json" resp id
  id="$(jq -r --arg n "${name}" \
        '.data.project.environments.edges[]?.node | select(.name == $n) | .id' <<<"${PROJECT_DETAIL}" | head -n1)"
  if [[ -n "${id}" ]]; then
    log "environment '${name}' already exists (${id}) — reusing"
  else
    jq -n --arg p "${PROJECT_ID}" --arg n "${name}" '{input: {projectId: $p, name: $n}}' > "${vars}"
    resp="$(gql "environmentCreate(${name})" "${M_ENV_CREATE}" "${vars}" "$(<"${vars}")")" \
      || die "environmentCreate(${name}) failed"
    id="$(jq -r '.data.environmentCreate.id // empty' <<<"${resp}")"
    if [[ -z "${id}" ]]; then
      (( DRY_RUN )) || die "environmentCreate(${name}) returned no id"
      id="DRY_RUN_ENV_$(env_suffix "${name}")"
    fi
    log "created environment '${name}' (${id})"
  fi
  ENV_IDS[${name}]="${id}"
}

# Railway services are PROJECT-scoped: one service gets an instance in every
# environment, configured per environment via serviceInstanceUpdate /
# variableUpsert. So serviceCreate runs once per name, and the per-environment
# GitHub secret names (RAILWAY_SVC_ID_SERVER_STAGING vs ..._PRODUCTION) may
# carry the same id — they stay split so a future per-environment service
# split needs no workflow changes.
ensure_service() { # ensure_service <name> <role>
  local name="$1" role="$2" vars="${WORK_DIR}/vars.json" resp id redacted
  local varmap="${WORK_DIR}/varmap.json" varmap_red="${WORK_DIR}/varmap-redacted.json"
  id="$(jq -r --arg n "${name}" \
        '.data.project.services.edges[]?.node | select(.name == $n) | .id' <<<"${PROJECT_DETAIL}" | head -n1)"
  if [[ -n "${id}" ]]; then
    log "service '${name}' already exists (${id}) — reusing"
  else
    # serviceCreate with source.image starts deploying IMMEDIATELY, so the
    # full variable map rides along in ServiceCreateInput.variables (a JSON
    # map — public-API supported) and the first boot comes up configured
    # instead of crash-looping with no env. The map is built from the FIRST
    # --environments entry; apply_service_vars then (re)applies each
    # environment's own authoritative values via variableCollectionUpsert.
    # registryCredentials rides along too when --ghcr-pull-* was given.
    # First-run schema caveat (header): validate the `variables` and
    # `registryCredentials` field names on the first credentialed run.
    build_service_vars "${ENVIRONMENTS[0]}" "${role}" "${varmap}" "${varmap_red}"
    jq -n --arg p "${PROJECT_ID}" --arg n "${name}" --arg img "${IMAGE}" \
          --slurpfile m "${varmap}" --slurpfile rc "${RC_FILE}" \
          '{input: ({projectId: $p, name: $n, source: {image: $img}, variables: $m[0]}
                    + (if $rc[0] == null then {} else {registryCredentials: $rc[0]} end))}' > "${vars}"
    redacted="$(jq -n --arg p "${PROJECT_ID}" --arg n "${name}" --arg img "${IMAGE}" \
          --slurpfile m "${varmap_red}" --slurpfile rc "${RC_REDACTED_FILE}" \
          '{input: ({projectId: $p, name: $n, source: {image: $img}, variables: $m[0]}
                    + (if $rc[0] == null then {} else {registryCredentials: $rc[0]} end))}')"
    resp="$(gql "serviceCreate(${name})" "${M_SERVICE_CREATE}" "${vars}" "${redacted}")" \
      || die "serviceCreate(${name}) failed"
    id="$(jq -r '.data.serviceCreate.id // empty' <<<"${resp}")"
    if [[ -z "${id}" ]]; then
      (( DRY_RUN )) || die "serviceCreate(${name}) returned no id"
      id="DRY_RUN_SVC_$(env_suffix "${name}")"
    fi
    SVC_CREATED[${name}]=1
    log "created service '${name}' (${id})"
  fi
  SVC_IDS[${name}]="${id}"
}

# build_instance_input <svc-id> <env-id> <set-image:0|1> <rc-file>
# ServiceInstanceUpdateInput body on stdout. registryCredentials is included
# on EVERY run when --ghcr-pull-* was given (so re-running converges an
# existing estate that is 401ing on pulls), with the usual first-run schema
# caveat; pass RC_REDACTED_FILE to build the loggable twin.
build_instance_input() {
  local s="$1" e="$2" set_img="$3" rcf="$4"
  jq -n --arg s "$s" --arg e "$e" --arg img "${IMAGE}" --argjson setimg "$set_img" \
        --slurpfile rc "${rcf}" \
    '{serviceId: $s, environmentId: $e,
      input: ({healthcheckPath: "/ready"}
              + (if $setimg == 1 then {source: {image: $img}} else {} end)
              + (if $rc[0] == null then {} else {registryCredentials: $rc[0]} end))}'
}

# configure_instance points Railway's healthcheck at GET /ready (both roles
# serve it — the runbook's readiness probe). The image is pinned at the
# instance only when this run CREATED the service: on a re-run the instance may
# already be pinned to a digest by the deploy workflows, and resetting it to
# the bootstrap tag would silently roll the deployment back.
configure_instance() { # configure_instance <env-name> <svc-name>
  local env_name="$1" svc_name="$2" vars="${WORK_DIR}/vars.json" set_img=0 redacted
  if [[ -n "${SVC_CREATED[${svc_name}]:-}" ]]; then
    set_img=1
  fi
  build_instance_input "${SVC_IDS[${svc_name}]}" "${ENV_IDS[${env_name}]}" "${set_img}" "${RC_FILE}" > "${vars}"
  redacted="$(build_instance_input "${SVC_IDS[${svc_name}]}" "${ENV_IDS[${env_name}]}" "${set_img}" "${RC_REDACTED_FILE}")"
  gql "serviceInstanceUpdate(${svc_name}@${env_name})" "${M_SVC_INSTANCE_UPDATE}" "${vars}" "${redacted}" >/dev/null \
    || die "serviceInstanceUpdate(${svc_name}@${env_name}) failed"
}

# build_service_vars <env-name> <role> <out-json> <redacted-out-json>
# The COMPLETE variable map for one service in one environment, per
# docs/deploy-runbook.md §2 — built once so serviceCreate (first boot) and
# variableCollectionUpsert (every run) can never drift. Secrets are read via
# jq --rawfile (JSON-encoded safely, never argv); single-line secrets get one
# trailing newline trimmed, PEM blocks stay verbatim. The redacted twin
# replaces secret values with [REDACTED] for all logging incl. --dry-run.
# Both files land in the 0600 workdir and die with the EXIT trap.
#
# PORT is deliberately NOT set — Railway injects it and the app reads $PORT
# (default 8080). SENTRY_DSN is optional and left for the operator (see
# provision-output.env). DATABASE_URL is the Railway reference variable: it
# resolves to this environment's managed Postgres once a service literally
# named "Postgres" exists in it (different name → fix the reference target).
#
# BUILDOS_BOOTSTRAP_TOKEN goes to the SERVER role ONLY: only cmd/server seeds
# it; the worker never reads it. It is first-boot-only — REMOVE (or rotate)
# the variable on the server service after the first owner claims at
# POST /api/v1/auth/claim.
build_service_vars() {
  local env_name="$1" role="$2" out="$3" redacted_out="$4"
  local dir="${SECRETS_DIRS[${env_name}]}"
  jq -n --arg role "${role}" --arg env "${env_name}" --arg cidrs "${TRUSTED_PROXY_CIDRS_PLACEHOLDER}" \
        --rawfile priv "${dir}/private.pem" --rawfile pub "${dir}/public.pem" \
        --rawfile vault "${dir}/vault_master_key.txt" --rawfile boot "${dir}/bootstrap_token.txt" '
    {
      BUILDOS_ROLE: $role,
      DATABASE_URL: "${{Postgres.DATABASE_URL}}",
      JWT_PRIVATE_KEY_PEM: $priv,
      JWT_PUBLIC_KEY_PEM: $pub,
      VAULT_MASTER_KEY: ($vault | rtrimstr("\n")),
      SENTRY_ENVIRONMENT: $env,
      TRUSTED_PROXY_CIDRS: $cidrs
    }
    + (if $role == "server"
       then {BUILDOS_BOOTSTRAP_TOKEN: ($boot | rtrimstr("\n"))}
       else {} end)' > "${out}"
  jq 'with_entries(if .key | IN("JWT_PRIVATE_KEY_PEM", "JWT_PUBLIC_KEY_PEM", "VAULT_MASTER_KEY", "BUILDOS_BOOTSTRAP_TOKEN")
                   then .value = "[REDACTED]" else . end)' \
    "${out}" > "${redacted_out}"
}

# apply_service_vars <env-name> <svc-name> <role>
# ONE variableCollectionUpsert carrying the whole map — N sequential
# variableUpserts can each trigger their own redeploy; the collection upsert
# applies atomically. Same first-run schema caveat as every mutation here:
# validate variableCollectionUpsert / VariableCollectionUpsertInput against
# the live schema on the first credentialed run.
apply_service_vars() {
  local env_name="$1" svc_name="$2" role="$3"
  local vars="${WORK_DIR}/vars.json" varmap="${WORK_DIR}/varmap.json"
  local varmap_red="${WORK_DIR}/varmap-redacted.json" redacted
  build_service_vars "${env_name}" "${role}" "${varmap}" "${varmap_red}"
  jq -n --arg p "${PROJECT_ID}" --arg e "${ENV_IDS[${env_name}]}" --arg s "${SVC_IDS[${svc_name}]}" \
        --slurpfile m "${varmap}" \
        '{input: {projectId: $p, environmentId: $e, serviceId: $s, variables: $m[0]}}' > "${vars}"
  redacted="$(jq -n --arg p "${PROJECT_ID}" --arg e "${ENV_IDS[${env_name}]}" --arg s "${SVC_IDS[${svc_name}]}" \
        --slurpfile m "${varmap_red}" \
        '{input: {projectId: $p, environmentId: $e, serviceId: $s, variables: $m[0]}}')"
  gql "variableCollectionUpsert(${svc_name}@${env_name})" "${M_VARIABLE_COLLECTION_UPSERT}" "${vars}" "${redacted}" >/dev/null \
    || die "variableCollectionUpsert(${svc_name}@${env_name}) failed"
}

# ensure_postgres — managed Postgres, one per environment.
#
# The API path attempted here is the idempotency LOOKUP (is a service named
# "Postgres" already in the project?). Actually CREATING managed Postgres over
# GraphQL needs the full template wiring — image + attached volume + generated
# credentials + the DATABASE_URL variable — and that mutation set is NOT
# reliably documented (see the header caveat). A bare serviceCreate with a
# postgres image would come up with NO VOLUME: a data-loss footgun we refuse
# to automate. So when the service is missing we print the exact manual
# fallback (the official CLI performs the complete wiring) and continue;
# DATABASE_URL wiring is then manual and recorded in provision-output.env.
ensure_postgres() { # ensure_postgres <env-name>
  local env_name="$1" id
  id="$(jq -r '.data.project.services.edges[]?.node | select(.name == "Postgres") | .id' <<<"${PROJECT_DETAIL}" | head -n1)"
  if [[ -n "${id}" ]]; then
    log "Postgres service already present (${id}) — verify it has an instance + volume in '${env_name}'"
    return 0
  fi
  warn "managed Postgres for '${env_name}' must be added manually (see comment above ensure_postgres):"
  warn "    railway link --project ${PROJECT_ID} --environment ${env_name}"
  warn "    railway add --database postgres"
  warn "then copy that service's DATABASE_URL into the GitHub secret DATABASE_URL_$(env_suffix "${env_name}")"
  warn "(the deploy workflows run one-shot migrations with it). The server/worker"
  warn 'DATABASE_URL is already set to the reference ${{Postgres.DATABASE_URL}} and'
  warn "resolves on its own once the Postgres service exists in '${env_name}'."
  MANUAL_FOLLOWUPS+=("Add managed Postgres to '${env_name}' (railway add --database postgres), then fill GitHub secret DATABASE_URL_$(env_suffix "${env_name}").")
}

# print_domain_dns_target <env-name> <fqdn>
# After customDomainCreate (or already-exists), surface the DNS (CNAME) target
# the §3 Cloudflare step needs. The `status { dnsRecords ... }` selection is
# the schema-uncertain part (header caveat): a failed query or an
# un-extractable field degrades to printing the raw response + a dashboard
# pointer — never fails the run. The response carries domain names only (no
# secrets), so printing it is safe.
print_domain_dns_target() {
  local env_name="$1" fqdn="$2" vars="${WORK_DIR}/vars.json" resp target
  jq -n --arg p "${PROJECT_ID}" --arg e "${ENV_IDS[${env_name}]}" --arg s "${SVC_IDS[server]}" \
    '{projectId: $p, environmentId: $e, serviceId: $s}' > "${vars}"
  if ! resp="$(gql "domains lookup (${fqdn})" "${Q_DOMAINS}" "${vars}" "$(<"${vars}")")"; then
    warn "could not query the DNS target for ${fqdn} (the status/dnsRecords selection is schema-uncertain) —"
    warn "read the CNAME target in the dashboard instead: server@${env_name} -> Settings -> Networking."
    return 0
  fi
  if (( DRY_RUN )); then
    log "DRY-RUN: would print the DNS (CNAME) target for ${fqdn} here (paste it into Cloudflare, §3)."
    return 0
  fi
  target="$(jq -r --arg d "${fqdn}" \
    '[.data.domains.customDomains[]? | select(.domain == $d) | .status.dnsRecords[]?.requiredValue // empty] | join(", ")' \
    <<<"${resp}" 2>/dev/null || true)"
  if [[ -n "${target}" ]]; then
    log "DNS target for ${fqdn}: ${target}"
    log "  -> Cloudflare (README §3): create the PROXIED CNAME ${fqdn} -> ${target}"
  else
    warn "could not extract the DNS target for ${fqdn} — the schema for the target field is uncertain."
    warn "Full domains response below; find the CNAME target in it (or in the dashboard) for the §3 Cloudflare step:"
    jq -c '.data' <<<"${resp}" >&2
  fi
}

# ensure_domain — custom domain on the SERVER service only (the worker has no
# public surface). Idempotent WITHOUT swallowing real failures: an error
# matching the already-exists family is informational; ANY OTHER GraphQL error
# is FATAL — treating input-shape errors as "may already exist" would let
# re-runs never converge. On success or already-exists, the domain's DNS
# (CNAME) target is printed for the §3 Cloudflare step.
ensure_domain() { # ensure_domain <env-name> <fqdn>
  local env_name="$1" fqdn="$2" vars="${WORK_DIR}/vars.json"
  [[ -n "${fqdn}" ]] || return 0
  jq -n --arg p "${PROJECT_ID}" --arg e "${ENV_IDS[${env_name}]}" --arg s "${SVC_IDS[server]}" --arg d "${fqdn}" \
    '{input: {projectId: $p, environmentId: $e, serviceId: $s, domain: $d}}' > "${vars}"
  if gql "customDomainCreate(${fqdn} -> server@${env_name})" "${M_CUSTOM_DOMAIN_CREATE}" "${vars}" "$(<"${vars}")" >/dev/null; then
    log "custom domain ${fqdn} attached to server@${env_name}"
  elif last_gql_error_matches 'already|exists|in use'; then
    log "custom domain ${fqdn} already exists — reusing"
  else
    die "customDomainCreate(${fqdn} -> server@${env_name}) failed with a non-already-exists error (messages above) — fix the input/schema and re-run"
  fi
  print_domain_dns_target "${env_name}" "${fqdn}"
}

# write_output — the ID manifest under the EXACT GitHub Actions secret names
# the deploy workflows consume, plus every manual follow-up. Real runs only.
write_output() {
  local out="${PROVISION_OUTPUT}" env_name suf item
  {
    printf '# provision-output.env — generated by deploy/railway/provision.sh (%s)\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    printf '# Resource IDs for Railway project "%s". Copy each line below into a\n' "${PROJECT_NAME}"
    printf '# GitHub Actions secret of the SAME name (repo Settings -> Secrets and\n'
    printf '# variables -> Actions). IDs are not credentials, but keep this file out of git.\n'
    printf 'RAILWAY_PROJECT_ID=%s\n' "${PROJECT_ID}"
    for env_name in "${ENVIRONMENTS[@]}"; do
      suf="$(env_suffix "${env_name}")"
      printf 'RAILWAY_ENV_ID_%s=%s\n' "${suf}" "${ENV_IDS[${env_name}]}"
      printf 'RAILWAY_SVC_ID_SERVER_%s=%s\n' "${suf}" "${SVC_IDS[server]}"
      printf 'RAILWAY_SVC_ID_WORKER_%s=%s\n' "${suf}" "${SVC_IDS[worker]}"
    done
    cat <<'EOT'
# NOTE: Railway services are project-scoped (one instance per environment), so
# the per-environment SVC ids above may be identical. The secret names stay
# split so a future per-environment service split needs no workflow changes.
#
# Next steps (manual):
#   1. RAILWAY_API_TOKEN — the token used for this run — as a GitHub secret.
#   2. DATABASE_URL_STAGING / DATABASE_URL_PRODUCTION — from each environment's
#      managed Postgres service variables (see any Postgres fallback notes
#      below; the deploy workflows run one-shot migrations with these).
#   3. R2_ENDPOINT / R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY / R2_BUCKET — from
#      Cloudflare R2 (the DB-backup workflow's object store).
#   4. Repo VARIABLES (vars.*): STAGING_BASE_URL=https://staging.futurebuild.ai
#      and PROD_BASE_URL=https://app.futurebuild.ai.
#   5. VERIFY-AT-FIRST-DEPLOY: TRUSTED_PROXY_CIDRS was set to the placeholder
#      "100.64.0.0/10,10.0.0.0/8". Check the server's boot log / the observed
#      peer address behind Railway's edge and tighten it on BOTH services in
#      BOTH environments.
#   6. BUILDOS_BOOTSTRAP_TOKEN is first-boot-only and set on the SERVER service
#      only (only cmd/server seeds it; the worker never reads it). After the
#      first owner claims (POST /api/v1/auth/claim), DELETE (or rotate) it.
#   7. SENTRY_DSN (optional) was not set — add it per environment when ready.
EOT
    if [[ "${#MANUAL_FOLLOWUPS[@]}" -gt 0 ]]; then
      printf '#\n# Manual follow-ups recorded during this run:\n'
      for item in "${MANUAL_FOLLOWUPS[@]}"; do
        printf '#   - %s\n' "${item}"
      done
    fi
  } > "${out}"
  chmod 600 "${out}"
  log "wrote ${out}"
}

main() {
  local mode="" env_name
  if (( DRY_RUN )); then mode=" [DRY-RUN: printing GraphQL, sending nothing]"; fi
  log "provisioning '${PROJECT_NAME}' (environments: ${ENVIRONMENTS_CSV}; image: ${IMAGE})${mode}"

  if [[ "${IMAGE}" == ghcr.io/* && -z "${GHCR_PULL_USERNAME}" ]]; then
    warn "no --ghcr-pull-username/--ghcr-pull-token-file given: if ${IMAGE} is PRIVATE (the GHCR"
    warn "default), Railway cannot pull it — every roll 401s and /ready never goes 200. Pass the"
    warn "flags (read:packages PAT) or set registry credentials in the dashboard (README §2,"
    warn "'GHCR pull access')."
    MANUAL_FOLLOWUPS+=("Provide GHCR pull credentials if ${IMAGE} is private: re-run with --ghcr-pull-username/--ghcr-pull-token-file, or Railway service -> Settings -> Source -> registry credentials.")
  fi

  ensure_project
  load_project_detail

  for env_name in "${ENVIRONMENTS[@]}"; do
    ensure_environment "${env_name}"
  done
  ensure_service server server
  ensure_service worker worker

  for env_name in "${ENVIRONMENTS[@]}"; do
    configure_instance "${env_name}" server
    configure_instance "${env_name}" worker
    apply_service_vars "${env_name}" server server
    apply_service_vars "${env_name}" worker worker
    ensure_postgres "${env_name}"
    case "${env_name}" in
      staging)    ensure_domain "${env_name}" "${STAGING_DOMAIN}" ;;
      production) ensure_domain "${env_name}" "${PRODUCTION_DOMAIN}" ;;
    esac
  done

  if (( DRY_RUN )); then
    log "dry-run complete — nothing was sent and provision-output.env was NOT written."
    log "Validate the GraphQL bodies above against the live schema, then re-run without --dry-run."
  else
    write_output
    log "done — next steps are at the bottom of ${PROVISION_OUTPUT}"
  fi
}

main
