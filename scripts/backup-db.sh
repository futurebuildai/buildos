#!/usr/bin/env bash
# backup-db.sh — per-fork Postgres backup with retention + integrity sidecar.
#
# BuildOS is single-tenant per customer fork (ADR-002): each customer owns one
# database in one deployment, so backup is a per-fork operational concern, not a
# central fleet job. This script is the building block an operator schedules
# (cron / Kubernetes CronJob / a River maintenance job) to capture point-in-time
# logical backups and prune old ones. See docs/dr-runbook.md for the policy and
# the restore drill.
#
# What it does, in order:
#   1. pg_dump the database in custom format (-Fc) — compressed, and the format
#      pg_restore needs for selective/parallel restore.
#   2. Write a .sha256 sidecar next to the dump so restore can verify integrity
#      before clobbering a live database.
#   3. Optionally hand the dump to a storage-agnostic upload hook
#      (BACKUP_UPLOAD_CMD) — e.g. `aws s3 cp`, `gsutil cp`, `rclone copy`. The
#      script stays cloud-agnostic: no AWS/GCP SDK or CLI is a hard dependency.
#   4. Prune old local backups by the timestamp embedded in each filename:
#      drop anything older than BACKUP_RETENTION_DAYS, but always keep at least
#      BACKUP_RETAIN_MIN of the most-recent backups (so a stalled backup cron
#      can never erase the last good copy).
#
# Filenames are buildos-<db>-<UTC>.dump where <UTC> is YYYYmmddTHHMMSSZ. Pruning
# reads that embedded timestamp (NOT mtime) so copying/syncing backups around
# can't change what survives a prune.
#
# Tiered grandfather-father-son retention (keep N daily / M weekly / K monthly)
# is intentionally NOT reimplemented in bash — push that to the object store's
# lifecycle rules (S3 lifecycle, GCS object lifecycle), which is where it
# belongs operationally. This script's local retention is the on-host floor.
#
# Usage:
#   scripts/backup-db.sh                 # dump + sidecar + upload + prune
#   scripts/backup-db.sh --prune-only    # prune existing backups, take no dump
#   scripts/backup-db.sh --no-prune      # dump + sidecar + upload, keep all
#
# Config (all overridable from the environment):
#   DATABASE_URL           libpq DSN of the database to back up.
#   BACKUP_DIR             destination dir for dumps (default ./backups).
#   BACKUP_RETENTION_DAYS  age window in days (default 30).
#   BACKUP_RETAIN_MIN      floor of most-recent backups always kept (default 7).
#   BACKUP_UPLOAD_CMD      optional command template; "{file}" is replaced with
#                          the dump path (e.g. 'aws s3 cp {file} s3://bucket/db/').
#   BACKUP_NOW_EPOCH       override "now" (unix seconds) — testing seam only.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

: "${DATABASE_URL:=postgres://fb_user:fb_pass@localhost:5433/futurebuild_os?sslmode=disable}"
: "${BACKUP_DIR:=${ROOT_DIR}/backups}"
: "${BACKUP_RETENTION_DAYS:=30}"
: "${BACKUP_RETAIN_MIN:=7}"
: "${BACKUP_UPLOAD_CMD:=}"
: "${BACKUP_NOW_EPOCH:=$(date -u +%s)}"

DO_DUMP=1
DO_PRUNE=1

while [[ $# -gt 0 ]]; do
  case "$1" in
    --prune-only) DO_DUMP=0; DO_PRUNE=1; shift ;;
    --no-prune)   DO_PRUNE=0; shift ;;
    -h|--help)    sed -n '2,40p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "backup-db: unknown argument: $1" >&2; exit 2 ;;
  esac
done

log() { printf '\033[36m[backup-db]\033[0m %s\n' "$*" >&2; }
err() { printf '\033[31m[backup-db]\033[0m %s\n' "$*" >&2; }

# db_name extracts the database name from a libpq URL DSN for the filename.
# Strips scheme/host/credentials and any ?query suffix; falls back to "db".
db_name() {
  local dsn="${DATABASE_URL}"
  dsn="${dsn%%\?*}"      # drop ?sslmode=... etc.
  dsn="${dsn##*/}"       # keep the last path segment (the dbname)
  [[ -n "${dsn}" ]] && printf '%s' "${dsn}" || printf 'db'
}

# epoch_of_dump parses the UTC timestamp embedded in a dump filename
# (buildos-<db>-YYYYmmddTHHMMSSZ.dump) into unix seconds. Prints nothing and
# returns 1 when the name doesn't match (so unknown files are never pruned).
epoch_of_dump() {
  local base ts
  base="$(basename "$1")"
  # Capture the 8-digit date + T + 6-digit time + Z stamp.
  if [[ "${base}" =~ ([0-9]{8})T([0-9]{6})Z\.dump$ ]]; then
    local d="${BASH_REMATCH[1]}" t="${BASH_REMATCH[2]}"
    # Reassemble to an ISO form date(1) can parse as UTC.
    ts="${d:0:4}-${d:4:2}-${d:6:2}T${t:0:2}:${t:2:2}:${t:4:2}Z"
    date -u -d "${ts}" +%s 2>/dev/null && return 0
  fi
  return 1
}

take_dump() {
  command -v pg_dump >/dev/null 2>&1 || { err "pg_dump not found on PATH"; exit 127; }
  mkdir -p "${BACKUP_DIR}"
  local stamp out
  stamp="$(date -u -d "@${BACKUP_NOW_EPOCH}" +%Y%m%dT%H%M%SZ)"
  out="${BACKUP_DIR}/buildos-$(db_name)-${stamp}.dump"

  log "dumping $(db_name) → ${out}"
  # -Fc custom format (compressed, restore-flexible); --no-owner/--no-privileges
  # so a restore into a fresh fork DB with a different role still works.
  if ! pg_dump --dbname="${DATABASE_URL}" -Fc --no-owner --no-privileges -f "${out}"; then
    err "pg_dump failed; removing partial ${out}"
    rm -f "${out}"
    exit 1
  fi

  # Integrity sidecar — restore verifies this before clobbering a live DB.
  if command -v sha256sum >/dev/null 2>&1; then
    ( cd "${BACKUP_DIR}" && sha256sum "$(basename "${out}")" > "$(basename "${out}").sha256" )
  elif command -v shasum >/dev/null 2>&1; then
    ( cd "${BACKUP_DIR}" && shasum -a 256 "$(basename "${out}")" > "$(basename "${out}").sha256" )
  else
    err "no sha256sum/shasum; skipping integrity sidecar"
  fi
  log "wrote $(du -h "${out}" | cut -f1) dump + checksum"

  if [[ -n "${BACKUP_UPLOAD_CMD}" ]]; then
    local cmd="${BACKUP_UPLOAD_CMD//\{file\}/${out}}"
    log "uploading via: ${cmd}"
    # shellcheck disable=SC2086
    bash -c "${cmd}"
    if [[ -f "${out}.sha256" ]]; then
      local cmd2="${BACKUP_UPLOAD_CMD//\{file\}/${out}.sha256}"
      bash -c "${cmd2}"
    fi
  fi
}

prune() {
  [[ -d "${BACKUP_DIR}" ]] || { log "no backup dir ${BACKUP_DIR}; nothing to prune"; return 0; }

  # Collect dumps sorted NEWEST-first by embedded timestamp. We pair each
  # epoch with its path so the sort is on the parsed time, not the string.
  local pairs=() f e
  while IFS= read -r f; do
    [[ -n "${f}" ]] || continue
    if e="$(epoch_of_dump "${f}")"; then
      pairs+=("${e} ${f}")
    fi
  done < <(find "${BACKUP_DIR}" -maxdepth 1 -type f -name 'buildos-*.dump' 2>/dev/null)

  if [[ "${#pairs[@]}" -eq 0 ]]; then
    log "no parseable backups in ${BACKUP_DIR}"
    return 0
  fi

  # Sort descending by epoch (newest first).
  local sorted
  sorted="$(printf '%s\n' "${pairs[@]}" | sort -rn)"

  local cutoff=$(( BACKUP_NOW_EPOCH - BACKUP_RETENTION_DAYS * 86400 ))
  local idx=0 kept=0 pruned=0
  while IFS= read -r line; do
    [[ -n "${line}" ]] || continue
    idx=$(( idx + 1 ))
    local ep="${line%% *}" path="${line#* }"
    # Floor: always keep the newest BACKUP_RETAIN_MIN regardless of age.
    if [[ "${idx}" -le "${BACKUP_RETAIN_MIN}" ]]; then
      kept=$(( kept + 1 ))
      continue
    fi
    if [[ "${ep}" -ge "${cutoff}" ]]; then
      kept=$(( kept + 1 ))
      continue
    fi
    log "pruning $(basename "${path}") (older than ${BACKUP_RETENTION_DAYS}d, beyond ${BACKUP_RETAIN_MIN}-keep floor)"
    rm -f "${path}" "${path}.sha256"
    pruned=$(( pruned + 1 ))
  done <<< "${sorted}"

  log "retention: kept ${kept}, pruned ${pruned} (window ${BACKUP_RETENTION_DAYS}d, floor ${BACKUP_RETAIN_MIN})"
}

[[ "${DO_DUMP}" -eq 1 ]] && take_dump
[[ "${DO_PRUNE}" -eq 1 ]] && prune
log "done"
