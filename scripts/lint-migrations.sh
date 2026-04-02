#!/usr/bin/env bash
# lint-migrations.sh — Composite Currency Pattern Enforcement
#
# Hard-failing CI gate (GitHub Actions). No exemptions.
#
# Two checks:
#   1. FORBIDDEN TYPES: Scans migrations/*.sql for DECIMAL, NUMERIC, REAL,
#      DOUBLE PRECISION, MONEY, or FLOAT on columns matching monetary patterns.
#      EXCEPTION: gps_lat, gps_lng, lat, lng columns are explicitly allowed
#      to use DOUBLE PRECISION (they are coordinates, not currency).
#
#   2. ORPHAN CENTS: Any column ending in _cents that matches monetary patterns
#      must have a corresponding currency_code column in the same CREATE TABLE.
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

echo ""
echo "=== Lint Complete ==="

if [ "$EXIT_CODE" -ne 0 ]; then
    echo "RESULT: FAILED — Fix the above violations before merging."
    exit 1
fi

echo "RESULT: PASSED"
exit 0
