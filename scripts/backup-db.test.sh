#!/usr/bin/env bash
# backup-db.test.sh — DB-free regression coverage for backup-db.sh retention
# and restore-db.sh guards.
#
# No Postgres is touched: the retention cases exercise `backup-db.sh
# --prune-only` against fixture dirs of fake timestamped dump files with a
# FIXED clock (BACKUP_NOW_EPOCH), and the restore cases assert restore-db.sh
# bails at its integrity/confirmation guards BEFORE it would ever call
# pg_restore. Run from the repo root: `bash scripts/backup-db.test.sh`.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BACKUP="${SCRIPT_DIR}/backup-db.sh"
RESTORE="${SCRIPT_DIR}/restore-db.sh"

# Fixed reference clock so the fixtures are fully deterministic.
NOW=1700000000   # 2023-11-14T22:13:20Z
DAY=86400

EXIT_CODE=0
PASSED=0
FAILED=0

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT

pass() { echo "✓ $1"; PASSED=$((PASSED + 1)); }
fail() { echo "✗ $1"; cat /tmp/backup-test-out 2>/dev/null; FAILED=$((FAILED + 1)); EXIT_CODE=1; }

# stamp_of <epoch> → the UTC filename stamp backup-db.sh embeds/parses.
stamp_of() { date -u -d "@$1" +%Y%m%dT%H%M%SZ; }

# mk_dump <dir> <epoch> — create a fake dump + a valid sha256 sidecar so the
# sidecar-pruning path is exercised too.
mk_dump() {
  local dir="$1" ep="$2" name
  name="buildos-testdb-$(stamp_of "${ep}").dump"
  echo "fake-dump ${ep}" > "${dir}/${name}"
  ( cd "${dir}" && { sha256sum "${name}" 2>/dev/null || shasum -a 256 "${name}"; } > "${name}.sha256" )
  echo "${name}"
}

exists() { [[ -f "$1" ]]; }

# ---- retention: keeps everything inside the window ------------------------
case_keeps_recent() {
  local d="${WORK}/keeps-recent"; mkdir -p "${d}"
  mk_dump "${d}" $(( NOW - 1 * DAY )) >/dev/null
  mk_dump "${d}" $(( NOW - 2 * DAY )) >/dev/null
  mk_dump "${d}" $(( NOW - 3 * DAY )) >/dev/null

  BACKUP_DIR="${d}" BACKUP_NOW_EPOCH="${NOW}" BACKUP_RETENTION_DAYS=30 BACKUP_RETAIN_MIN=2 \
    bash "${BACKUP}" --prune-only >/tmp/backup-test-out 2>&1

  local n; n="$(find "${d}" -name '*.dump' | wc -l | tr -d ' ')"
  if [[ "${n}" -eq 3 ]]; then pass "keeps-recent: all 3 in-window dumps survive"; else
    fail "keeps-recent: expected 3 dumps, got ${n}"; fi
}

# ---- retention: prunes old beyond the floor, sidecars go with them --------
case_prunes_old() {
  local d="${WORK}/prunes-old"; mkdir -p "${d}"
  local r1 r2 o1 o2
  r1="$(mk_dump "${d}" $(( NOW - 1 * DAY )))"
  r2="$(mk_dump "${d}" $(( NOW - 2 * DAY )))"
  o1="$(mk_dump "${d}" $(( NOW - 40 * DAY )))"
  o2="$(mk_dump "${d}" $(( NOW - 50 * DAY )))"

  BACKUP_DIR="${d}" BACKUP_NOW_EPOCH="${NOW}" BACKUP_RETENTION_DAYS=30 BACKUP_RETAIN_MIN=2 \
    bash "${BACKUP}" --prune-only >/tmp/backup-test-out 2>&1

  if exists "${d}/${r1}" && exists "${d}/${r2}" \
     && ! exists "${d}/${o1}" && ! exists "${d}/${o2}" \
     && ! exists "${d}/${o1}.sha256" && ! exists "${d}/${o2}.sha256"; then
    pass "prunes-old: 2 recent kept, 2 old dumps + their sidecars pruned"
  else
    fail "prunes-old: survivors not as expected"
  fi
}

# ---- retention: floor keeps newest N even when all are old ----------------
case_floor() {
  local d="${WORK}/floor"; mkdir -p "${d}"
  local f40 f41 f42 f43 f44
  f40="$(mk_dump "${d}" $(( NOW - 40 * DAY )))"
  f41="$(mk_dump "${d}" $(( NOW - 41 * DAY )))"
  f42="$(mk_dump "${d}" $(( NOW - 42 * DAY )))"
  f43="$(mk_dump "${d}" $(( NOW - 43 * DAY )))"
  f44="$(mk_dump "${d}" $(( NOW - 44 * DAY )))"

  BACKUP_DIR="${d}" BACKUP_NOW_EPOCH="${NOW}" BACKUP_RETENTION_DAYS=30 BACKUP_RETAIN_MIN=3 \
    bash "${BACKUP}" --prune-only >/tmp/backup-test-out 2>&1

  if exists "${d}/${f40}" && exists "${d}/${f41}" && exists "${d}/${f42}" \
     && ! exists "${d}/${f43}" && ! exists "${d}/${f44}"; then
    pass "floor: newest 3 kept despite all being beyond the window"
  else
    fail "floor: expected newest 3 to survive"
  fi
}

# ---- restore: refuses without confirmation --------------------------------
case_restore_no_confirm() {
  local d="${WORK}/restore-nc"; mkdir -p "${d}"
  echo "x" > "${d}/b.dump"   # no sidecar → integrity skipped, hits the guard
  if bash "${RESTORE}" "${d}/b.dump" >/tmp/backup-test-out 2>&1; then
    fail "restore-no-confirm: expected nonzero exit, got 0"
  else
    pass "restore-no-confirm: refused without --confirm"
  fi
}

# ---- restore: refuses on a missing dump file ------------------------------
case_restore_missing() {
  if bash "${RESTORE}" "${WORK}/does-not-exist.dump" --confirm >/tmp/backup-test-out 2>&1; then
    fail "restore-missing: expected nonzero exit, got 0"
  else
    pass "restore-missing: refused on missing dump"
  fi
}

# ---- restore: refuses on a checksum mismatch (before the guard) -----------
case_restore_bad_checksum() {
  local d="${WORK}/restore-bad"; mkdir -p "${d}"
  echo "real-contents" > "${d}/b.dump"
  echo "deadbeef  b.dump" > "${d}/b.dump.sha256"   # wrong hash
  if bash "${RESTORE}" "${d}/b.dump" --confirm >/tmp/backup-test-out 2>&1; then
    fail "restore-bad-checksum: expected nonzero exit, got 0"
  else
    pass "restore-bad-checksum: refused on integrity mismatch"
  fi
}

echo "=== backup-db.sh / restore-db.sh regression suite ==="
case_keeps_recent
case_prunes_old
case_floor
case_restore_no_confirm
case_restore_missing
case_restore_bad_checksum

echo ""
echo "Result: ${PASSED} passed, ${FAILED} failed"
exit "${EXIT_CODE}"
