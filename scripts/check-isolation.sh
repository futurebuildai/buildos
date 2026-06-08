#!/usr/bin/env bash
# check-isolation.sh — agentic leaf-isolation enforcement
#
# Hard-failing CI gate enforcing the internal/agentic isolation contract in
# BOTH directions:
#
#   1. NO AGENTIC IN CORE: the deterministic core packages (internal/physics,
#      internal/currency) must never have internal/agentic anywhere in their
#      transitive dependency closure — the AI harness must not leak into the
#      authoritative schedule/money engines.
#   2. AGENTIC IS A LEAF: internal/agentic must import NO other internal/*
#      package (only stdlib + github.com/google/uuid + log/slog). It declares
#      its own ports; the dependency arrow points inward (callers → agentic),
#      never outward. This is the direction future phases are most likely to
#      drift, so the gate must defend it explicitly.
#
# Exit code 0 = PASS, 1 = FAIL

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

MODULE="github.com/futurebuildai/buildos"
FORBIDDEN="internal/agentic"
CORE_PACKAGES="./internal/physics/... ./internal/currency/..."

echo "=== Isolation Lint ==="
echo ""

# --- Check 1: core must not depend on agentic ----------------------------
echo "Check 1: core packages must not depend on ${FORBIDDEN}"
echo "  packages: ${CORE_PACKAGES}"
# shellcheck disable=SC2086
DEPS="$(go list -deps ${CORE_PACKAGES})"
if echo "$DEPS" | grep -q "${FORBIDDEN}"; then
    echo "FAIL: core packages transitively depend on ${FORBIDDEN}:"
    echo "$DEPS" | grep "${FORBIDDEN}" | sed 's/^/  > /'
    echo ""
    echo "  > internal/physics and internal/currency are the authoritative"
    echo "  > deterministic engines; the AI harness must never be in their"
    echo "  > dependency closure. Break the import that points back at agentic."
    exit 1
fi
echo "  PASS: core has no dependency on ${FORBIDDEN}."
echo ""

# --- Check 2: agentic must be a leaf (no internal/* imports) -------------
echo "Check 2: ${FORBIDDEN} must import no other internal/* package (leaf)"
AGENTIC_INTERNAL_IMPORTS="$(go list -f '{{range .Imports}}{{.}}
{{end}}{{range .TestImports}}{{.}}
{{end}}' ./internal/agentic/... \
    | sort -u \
    | grep "^${MODULE}/internal/" \
    | grep -v "^${MODULE}/internal/agentic" || true)"
if [[ -n "$AGENTIC_INTERNAL_IMPORTS" ]]; then
    echo "FAIL: ${FORBIDDEN} imports forbidden internal/* packages:"
    echo "$AGENTIC_INTERNAL_IMPORTS" | sed 's/^/  > /'
    echo ""
    echo "  > internal/agentic is a LEAF: import only stdlib +"
    echo "  > github.com/google/uuid + log/slog, and reach domain services"
    echo "  > only through the ports it declares. Move the dependency behind a"
    echo "  > port interface implemented by an adapter in internal/service."
    exit 1
fi
echo "  PASS: ${FORBIDDEN} imports no other internal/* package."
echo ""

echo "=== Isolation Lint Complete ==="
echo "RESULT: PASSED"
exit 0
