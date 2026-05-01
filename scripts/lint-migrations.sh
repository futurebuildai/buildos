#!/usr/bin/env bash
# lint-migrations.sh — migration safety + Composite Currency Pattern enforcement
#
# Hard-failing CI gate (GitHub Actions). No exemptions for the
# Composite Currency rules; the migration-safety rules support a
# documented opt-out comment for genuinely-safe cases.
#
# Five checks:
#   1. FORBIDDEN TYPES: Scans migrations/*.sql for DECIMAL, NUMERIC, REAL,
#      DOUBLE PRECISION, MONEY, or FLOAT on columns matching monetary patterns.
#      EXCEPTION: gps_lat, gps_lng, lat, lng columns are explicitly allowed
#      to use DOUBLE PRECISION (they are coordinates, not currency).
#
#   2. ORPHAN CENTS: Any column ending in _cents that matches monetary patterns
#      must have a corresponding currency_code column in the same CREATE TABLE.
#
#   3. PAIRED MIGRATIONS: Every migrations/NNN_name.up.sql must have a matching
#      migrations/NNN_name.down.sql. For irreversible migrations, the .down.sql
#      can be a single comment line documenting why (e.g. "-- irreversible: data
#      backfill").
#
#   4. DESTRUCTIVE OPS: DROP TABLE / DROP COLUMN / TRUNCATE / ALTER TYPE ... DROP
#      VALUE require an opt-in header comment of the form
#      "-- buildos:destructive: <reason>". Forces operator consent and surfaces
#      the reason in PR review + the deployment runbook.
#
#   5. CONCURRENT INDEX: Plain CREATE INDEX takes an ACCESS EXCLUSIVE lock for
#      the duration of the build. CREATE INDEX CONCURRENTLY builds without
#      blocking writes. Opt-out: append "-- buildos:lock-ok: <reason>" on the
#      same line for genuinely-small / fresh-table cases.
#
# Exit code 0 = PASS, 1 = FAIL

set -euo pipefail

MIGRATION_DIR="${1:-migrations}"
EXIT_CODE=0

# Monetary column name patterns (case-insensitive grep)
MONETARY_PATTERNS="cost|price|amount|total|budget|cents|fee|payment|invoice|balance|revenue|expense"

# Forbidden SQL types for monetary columns
FORBIDDEN_TYPES="DECIMAL|NUMERIC|REAL|DOUBLE PRECISION|MONEY|FLOAT"

# Coordinate columns that are allowed to use DOUBLE PRECISION
COORDINATE_PATTERNS="gps_lat|gps_lng|^lat$|^lng$"

echo "=== Composite Currency Pattern Linter ==="
echo "Scanning: ${MIGRATION_DIR}/*.sql"
echo ""

# --------------------------------------------------------------------------
# Check 1: Forbidden types on monetary columns
# --------------------------------------------------------------------------
echo "--- Check 1: Forbidden floating-point types on monetary columns ---"

for file in "${MIGRATION_DIR}"/*.up.sql; do
    [ -f "$file" ] || continue

    line_num=0
    while IFS= read -r line; do
        line_num=$((line_num + 1))

        # Skip comments and empty lines
        [[ "$line" =~ ^[[:space:]]*-- ]] && continue
        [[ -z "${line// /}" ]] && continue

        # Convert to lowercase for matching
        lower_line=$(echo "$line" | tr '[:upper:]' '[:lower:]')

        # Check if this line defines a column with a forbidden type
        for forbidden in DECIMAL NUMERIC REAL MONEY FLOAT; do
            forbidden_lower=$(echo "$forbidden" | tr '[:upper:]' '[:lower:]')

            if echo "$lower_line" | grep -qiE "\b${forbidden_lower}\b"; then
                # Check if this line also contains a monetary column name
                if echo "$lower_line" | grep -qiE "(${MONETARY_PATTERNS})"; then
                    echo "FAIL: ${file}:${line_num}: Forbidden type '${forbidden}' on monetary column"
                    echo "  > ${line}"
                    EXIT_CODE=1
                fi
            fi
        done

        # Special check for DOUBLE PRECISION (two words)
        if echo "$lower_line" | grep -qiE "double precision"; then
            if echo "$lower_line" | grep -qiE "(${MONETARY_PATTERNS})"; then
                # Check it's NOT a coordinate column
                if ! echo "$lower_line" | grep -qiE "(${COORDINATE_PATTERNS})"; then
                    echo "FAIL: ${file}:${line_num}: Forbidden type 'DOUBLE PRECISION' on monetary column"
                    echo "  > ${line}"
                    EXIT_CODE=1
                fi
            fi
        fi

    done < "$file"
done

if [ "$EXIT_CODE" -eq 0 ]; then
    echo "PASS: No forbidden types found on monetary columns."
fi
echo ""

# --------------------------------------------------------------------------
# Check 2: Orphan _cents columns without currency_code
# --------------------------------------------------------------------------
echo "--- Check 2: Orphan _cents columns (missing currency_code) ---"

ORPHAN_EXIT=0

for file in "${MIGRATION_DIR}"/*.up.sql; do
    [ -f "$file" ] || continue

    # Extract CREATE TABLE blocks and check each one
    # We parse the file to find table definitions and their columns
    in_create=false
    table_name=""
    cents_columns=()
    has_currency_code=false
    block_start=0

    line_num=0
    while IFS= read -r line; do
        line_num=$((line_num + 1))
        lower_line=$(echo "$line" | tr '[:upper:]' '[:lower:]')

        # Detect start of CREATE TABLE
        if echo "$lower_line" | grep -qiE "^[[:space:]]*create table"; then
            # If we were in a previous table, check it
            if [ "$in_create" = true ] && [ ${#cents_columns[@]} -gt 0 ]; then
                if [ "$has_currency_code" = false ]; then
                    for col in "${cents_columns[@]}"; do
                        echo "FAIL: ${file}: Table '${table_name}' has monetary column '${col}' but no currency_code column"
                        ORPHAN_EXIT=1
                    done
                fi
            fi

            in_create=true
            table_name=$(echo "$lower_line" | sed -E 's/.*create table[[:space:]]+(if not exists[[:space:]]+)?([a-z_]+).*/\2/')
            cents_columns=()
            has_currency_code=false
            block_start=$line_num
            continue
        fi

        # Detect end of CREATE TABLE (closing paren + semicolon)
        if [ "$in_create" = true ]; then
            if echo "$lower_line" | grep -qE "^\);"; then
                # Check the completed table
                if [ ${#cents_columns[@]} -gt 0 ] && [ "$has_currency_code" = false ]; then
                    for col in "${cents_columns[@]}"; do
                        echo "FAIL: ${file}: Table '${table_name}' has monetary column '${col}' but no currency_code column"
                        ORPHAN_EXIT=1
                    done
                fi
                in_create=false
                continue
            fi

            # Look for _cents columns matching monetary patterns
            if echo "$lower_line" | grep -qiE "_cents\b" && echo "$lower_line" | grep -qiE "(${MONETARY_PATTERNS})"; then
                col_name=$(echo "$lower_line" | sed -E 's/^[[:space:]]+([a-z_]+).*/\1/')
                cents_columns+=("$col_name")
            fi

            # Look for currency_code columns
            if echo "$lower_line" | grep -qiE "currency_code"; then
                has_currency_code=true
            fi
        fi

    done < "$file"

    # Check the last table if file ended inside a CREATE TABLE
    if [ "$in_create" = true ] && [ ${#cents_columns[@]} -gt 0 ] && [ "$has_currency_code" = false ]; then
        for col in "${cents_columns[@]}"; do
            echo "FAIL: ${file}: Table '${table_name}' has monetary column '${col}' but no currency_code column"
            ORPHAN_EXIT=1
        done
    fi
done

if [ "$ORPHAN_EXIT" -ne 0 ]; then
    EXIT_CODE=1
else
    echo "PASS: All _cents columns have corresponding currency_code columns."
fi

# --------------------------------------------------------------------------
# Check 3: Every up migration has a matching down migration
# --------------------------------------------------------------------------
echo "--- Check 3: Paired up/down migrations ---"

PAIR_EXIT=0
for up_file in "${MIGRATION_DIR}"/*.up.sql; do
    [ -f "$up_file" ] || continue
    down_file="${up_file%.up.sql}.down.sql"
    if [ ! -f "$down_file" ]; then
        echo "FAIL: ${up_file} has no matching ${down_file##*/}"
        PAIR_EXIT=1
    fi
done

if [ "$PAIR_EXIT" -eq 0 ]; then
    echo "PASS: Every up migration has a matching down migration."
else
    EXIT_CODE=1
fi
echo ""

# --------------------------------------------------------------------------
# Check 4: Destructive operations require opt-in header comment
# --------------------------------------------------------------------------
# Each *.up.sql is treated as a unit: at least one line containing a
# destructive op (DROP TABLE / DROP COLUMN / TRUNCATE / DROP VALUE) requires
# a "-- buildos:destructive:" header anywhere in the file. The reason
# string after the colon goes into PR review and the deployment runbook.
echo "--- Check 4: Destructive ops require opt-in comment ---"

DESTRUCT_EXIT=0
DESTRUCTIVE_PATTERN='\b(drop[[:space:]]+table|drop[[:space:]]+column|truncate[[:space:]]+table|alter[[:space:]]+type[[:space:]]+[a-z_]+[[:space:]]+drop[[:space:]]+value)\b'

for file in "${MIGRATION_DIR}"/*.up.sql; do
    [ -f "$file" ] || continue
    lower_file=$(tr '[:upper:]' '[:lower:]' < "$file")

    if echo "$lower_file" | grep -qE "$DESTRUCTIVE_PATTERN"; then
        if ! grep -qiE -- '-- *buildos:destructive:' "$file"; then
            echo "FAIL: ${file}: contains destructive op (DROP TABLE / DROP COLUMN / TRUNCATE / DROP VALUE)"
            echo "  > add a header comment '-- buildos:destructive: <reason>' to acknowledge"
            DESTRUCT_EXIT=1
        fi
    fi
done

if [ "$DESTRUCT_EXIT" -eq 0 ]; then
    echo "PASS: No destructive ops without opt-in header."
else
    EXIT_CODE=1
fi
echo ""

# --------------------------------------------------------------------------
# Check 5: CREATE INDEX must be CONCURRENTLY (or per-line opt-out)
# --------------------------------------------------------------------------
# Plain CREATE INDEX takes ACCESS EXCLUSIVE for the duration of the build.
# CONCURRENTLY avoids the lock at the cost of being non-transactional.
# Opt-out: append "-- buildos:lock-ok: <reason>" on the same line.
echo "--- Check 5: CREATE INDEX requires CONCURRENTLY ---"

LOCK_EXIT=0

for file in "${MIGRATION_DIR}"/*.up.sql; do
    [ -f "$file" ] || continue

    line_num=0
    while IFS= read -r line; do
        line_num=$((line_num + 1))
        lower_line=$(echo "$line" | tr '[:upper:]' '[:lower:]')

        # Skip pure comment lines.
        [[ "$lower_line" =~ ^[[:space:]]*-- ]] && continue

        # Match CREATE INDEX (also CREATE UNIQUE INDEX) but not
        # CREATE INDEX CONCURRENTLY.
        if echo "$lower_line" | grep -qE '\bcreate[[:space:]]+(unique[[:space:]]+)?index\b'; then
            if echo "$lower_line" | grep -qE '\bconcurrently\b'; then
                continue
            fi
            # Same-line opt-out comment.
            if echo "$line" | grep -qiE -- '-- *buildos:lock-ok:'; then
                continue
            fi
            echo "FAIL: ${file}:${line_num}: CREATE INDEX without CONCURRENTLY"
            echo "  > ${line}"
            echo "  > use CREATE INDEX CONCURRENTLY, or append '-- buildos:lock-ok: <reason>'"
            LOCK_EXIT=1
        fi
    done < "$file"
done

if [ "$LOCK_EXIT" -eq 0 ]; then
    echo "PASS: All CREATE INDEX statements use CONCURRENTLY or have an opt-out."
else
    EXIT_CODE=1
fi
echo ""

echo "=== Lint Complete ==="

if [ "$EXIT_CODE" -ne 0 ]; then
    echo "RESULT: FAILED — Fix the above violations before merging."
    exit 1
fi

echo "RESULT: PASSED"
exit 0
