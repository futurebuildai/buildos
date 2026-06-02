#!/usr/bin/env bash
# restore-db.sh — restore a BuildOS fork database from a backup-db.sh dump.
#
# This is the DESTRUCTIVE half of the DR story: pg_restore --clean drops and
# recreates every object the dump contains, so it WILL overwrite the target
# database. It therefore refuses to run unless the operator explicitly confirms
# (--confirm or BACKUP_RESTORE_CONFIRM=1) — the same opt-in posture the
# migration linter forces for destructive DDL.
#
# What it does, in order:
#   1. Verify the dump file exists.
#   2. If a <dump>.sha256 sidecar is present, verify the dump's integrity and
#      ABORT on mismatch (a corrupted backup must never silently half-restore
#      over a live database).
#   3. Require explicit confirmation (destructive-op guard).
#   4. pg_restore --clean --if-exists --no-owner --no-privileges into the target.
#
# Usage:
#   scripts/restore-db.sh <dump-file> --confirm
#   BACKUP_RESTORE_CONFIRM=1 scripts/restore-db.sh <dump-file>
#
# Config:
#   DATABASE_URL             libpq DSN of the target database (the one to
#                            overwrite). Defaults to the local dev DSN.
#   BACKUP_RESTORE_CONFIRM   set to 1 to confirm without the --confirm flag.
set -euo pipefail

: "${DATABASE_URL:=postgres://fb_user:fb_pass@localhost:5433/futurebuild_os?sslmode=disable}"
: "${BACKUP_RESTORE_CONFIRM:=0}"

log() { printf '\033[36m[restore-db]\033[0m %s\n' "$*" >&2; }
err() { printf '\033[31m[restore-db]\033[0m %s\n' "$*" >&2; }

DUMP=""
CONFIRM="${BACKUP_RESTORE_CONFIRM}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --confirm) CONFIRM=1; shift ;;
    -h|--help) sed -n '2,30p' "${BASH_SOURCE[0]}"; exit 0 ;;
    -*) err "unknown argument: $1"; exit 2 ;;
    *) DUMP="$1"; shift ;;
  esac
done

if [[ -z "${DUMP}" ]]; then
  err "usage: restore-db.sh <dump-file> --confirm"
  exit 2
fi
if [[ ! -f "${DUMP}" ]]; then
  err "dump file not found: ${DUMP}"
  exit 1
fi

# ---- integrity check (when a sidecar exists) -------------------------------
if [[ -f "${DUMP}.sha256" ]]; then
  log "verifying integrity against ${DUMP}.sha256"
  ok=0
  if command -v sha256sum >/dev/null 2>&1; then
    ( cd "$(dirname "${DUMP}")" && sha256sum -c "$(basename "${DUMP}").sha256" >/dev/null 2>&1 ) && ok=1
  elif command -v shasum >/dev/null 2>&1; then
    ( cd "$(dirname "${DUMP}")" && shasum -a 256 -c "$(basename "${DUMP}").sha256" >/dev/null 2>&1 ) && ok=1
  else
    err "no sha256sum/shasum available to verify integrity; refusing to restore"
    exit 1
  fi
  if [[ "${ok}" -ne 1 ]]; then
    err "CHECKSUM MISMATCH for ${DUMP} — refusing to restore a corrupt backup"
    exit 1
  fi
  log "integrity OK"
else
  log "no .sha256 sidecar for ${DUMP}; skipping integrity check"
fi

# ---- destructive-op guard --------------------------------------------------
if [[ "${CONFIRM}" -ne 1 ]]; then
  err "restore is DESTRUCTIVE — it will overwrite ${DATABASE_URL%%\?*}"
  err "re-run with --confirm (or BACKUP_RESTORE_CONFIRM=1) to proceed"
  exit 3
fi

command -v pg_restore >/dev/null 2>&1 || { err "pg_restore not found on PATH"; exit 127; }

log "restoring ${DUMP} → ${DATABASE_URL%%\?*}"
# --clean --if-exists drops existing objects first (idempotent re-restore);
# --no-owner/--no-privileges so a fresh fork role can own the result.
pg_restore --dbname="${DATABASE_URL}" --clean --if-exists --no-owner --no-privileges "${DUMP}"
log "restore complete"
