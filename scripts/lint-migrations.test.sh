#!/usr/bin/env bash
# lint-migrations.test.sh — regression coverage for the migration linter.
#
# Each fixture lives in scripts/lint-migrations-fixtures/<name>/. The
# `pass` fixture must exit 0; every `fail-*` fixture must exit 1. Run
# from the repo root: `bash scripts/lint-migrations.test.sh`.

set -uo pipefail

SCRIPT="$(dirname "$0")/lint-migrations.sh"
FIXTURES="$(dirname "$0")/lint-migrations-fixtures"

EXIT_CODE=0
PASSED=0
FAILED=0

run_case() {
    local name="$1"
    local expected="$2"  # "pass" or "fail"
    local fixture="$FIXTURES/$name"

    if [ ! -d "$fixture" ]; then
        echo "MISSING: fixture dir $fixture"
        EXIT_CODE=1
        return
    fi

    bash "$SCRIPT" "$fixture" > /tmp/lint-test-out 2>&1
    local got=$?

    case "$expected" in
        pass)
            if [ "$got" -eq 0 ]; then
                echo "✓ $name: expected pass, got pass"
                PASSED=$((PASSED + 1))
            else
                echo "✗ $name: expected pass, got fail (rc=$got)"
                cat /tmp/lint-test-out
                FAILED=$((FAILED + 1))
                EXIT_CODE=1
            fi
            ;;
        fail)
            if [ "$got" -ne 0 ]; then
                echo "✓ $name: expected fail, got fail"
                PASSED=$((PASSED + 1))
            else
                echo "✗ $name: expected fail, got pass"
                cat /tmp/lint-test-out
                FAILED=$((FAILED + 1))
                EXIT_CODE=1
            fi
            ;;
    esac
}

echo "=== lint-migrations.sh regression suite ==="
run_case "pass"               "pass"
run_case "fail-no-down"       "fail"
run_case "fail-destructive"   "fail"
run_case "fail-create-index"  "fail"

echo ""
echo "Result: $PASSED passed, $FAILED failed"
exit $EXIT_CODE
