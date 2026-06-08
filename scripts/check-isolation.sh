#!/usr/bin/env bash
# check-isolation.sh — agentic leaf-isolation enforcement
#
# Hard-failing CI gate. The deterministic core packages
# (internal/physics, internal/currency) are the authoritative
# schedule/money engines. They must NEVER depend on the AI harness
# package (internal/agentic) — the agentic package is a LEAF that
# imports only stdlib + github.com/google/uuid + log/slog, and the
# dependency arrow must never point back the other way.
#
# One check:
#   1. NO AGENTIC IN CORE: `go list -deps` of internal/physics and
#      internal/currency must not contain internal/agentic anywhere
#      in their transitive dependency closure.
#
# Exit code 0 = PASS, 1 = FAIL

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

FORBIDDEN="internal/agentic"
CORE_PACKAGES="./internal/physics/... ./internal/currency/..."

echo "=== Isolation Lint ==="
echo ""
echo "Checking that core packages do not depend on ${FORBIDDEN}:"
echo "  packages: ${CORE_PACKAGES}"
echo ""

# shellcheck disable=SC2086
DEPS="$(go list -deps ${CORE_PACKAGES})"

if echo "$DEPS" | grep -q "${FORBIDDEN}"; then
    echo "FAIL: core packages transitively depend on ${FORBIDDEN}:"
    echo "$DEPS" | grep "${FORBIDDEN}" | sed 's/^/  > /'
    echo ""
    echo "  > internal/physics and internal/currency are the authoritative"
    echo "  > deterministic engines; the AI harness must never be in their"
    echo "  > dependency closure. Break the import that points back at agentic."
    echo ""
    echo "RESULT: FAILED — Fix the above violation before merging."
    exit 1
fi

echo "PASS: core packages have no dependency on ${FORBIDDEN}."
echo ""
echo "=== Isolation Lint Complete ==="
echo "RESULT: PASSED"
exit 0
