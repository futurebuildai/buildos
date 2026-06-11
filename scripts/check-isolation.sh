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

# --- Check 3: connectors is leaf-ish (only imports internal/agentic) ----
# internal/connectors is the integration seam: it produces agentic.Tool values
# and must reach the rest of BuildOS only through ports it declares (implemented
# by adapters in internal/service). The dependency arrow is
# agentic <- connectors <- service; connectors importing internal/service (or
# store/ai/worker) would invert the layering and risk an import cycle the moment
# service holds a connectors.Connector. It MAY import internal/agentic.
echo "Check 3: internal/connectors must import no internal/* except internal/agentic"
CONNECTORS_INTERNAL_IMPORTS="$(go list -f '{{range .Imports}}{{.}}
{{end}}{{range .TestImports}}{{.}}
{{end}}' ./internal/connectors/... \
    | sort -u \
    | grep "^${MODULE}/internal/" \
    | grep -v "^${MODULE}/internal/connectors" \
    | grep -v "^${MODULE}/internal/agentic" || true)"
if [[ -n "$CONNECTORS_INTERNAL_IMPORTS" ]]; then
    echo "FAIL: internal/connectors imports forbidden internal/* packages:"
    echo "$CONNECTORS_INTERNAL_IMPORTS" | sed 's/^/  > /'
    echo ""
    echo "  > internal/connectors may import ONLY stdlib + github.com/google/uuid"
    echo "  > + internal/agentic. Reach internal/service capabilities through a"
    echo "  > port interface DECLARED in internal/connectors and implemented by an"
    echo "  > adapter in internal/service (arrow: agentic <- connectors <- service)."
    exit 1
fi
echo "  PASS: internal/connectors imports only internal/agentic."
echo ""

# Defensive: the deterministic core must not pull in connectors either.
echo "Check 3b: core packages must not depend on internal/connectors"
# shellcheck disable=SC2086
if echo "$DEPS" | grep -q "internal/connectors"; then
    echo "FAIL: core packages transitively depend on internal/connectors:"
    echo "$DEPS" | grep "internal/connectors" | sed 's/^/  > /'
    exit 1
fi
echo "  PASS: core has no dependency on internal/connectors."
echo ""

# --- Check 4: storage is a leaf (no internal/* imports) ------------------
# internal/storage is the object-storage substrate (DAILY_REPORTS_CLIENT_UPDATES
# Chunk A): an ObjectStore port + a hand-rolled SigV4 R2 adapter. Like the
# agentic harness it is a LEAF — it imports ONLY stdlib (incl. crypto/*) and
# declares the port the rest of BuildOS consumes; the per-fork credential
# adapter lives in internal/service. It MUST NOT import internal/service,
# internal/store, internal/ai, internal/worker, internal/physics, or
# internal/currency (the dependency arrow points inward: callers -> storage).
echo "Check 4: internal/storage must import no other internal/* package (leaf)"
STORAGE_INTERNAL_IMPORTS="$(go list -f '{{range .Imports}}{{.}}
{{end}}{{range .TestImports}}{{.}}
{{end}}' ./internal/storage/... \
    | sort -u \
    | grep "^${MODULE}/internal/" \
    | grep -v "^${MODULE}/internal/storage" || true)"
if [[ -n "$STORAGE_INTERNAL_IMPORTS" ]]; then
    echo "FAIL: internal/storage imports forbidden internal/* packages:"
    echo "$STORAGE_INTERNAL_IMPORTS" | sed 's/^/  > /'
    echo ""
    echo "  > internal/storage is a LEAF: import only stdlib (incl. crypto/*)."
    echo "  > Reach domain capabilities through the ObjectStore port it declares,"
    echo "  > implemented by an adapter in internal/service (arrow: caller -> storage)."
    exit 1
fi
echo "  PASS: internal/storage imports no other internal/* package."
echo ""

# Defensive: the deterministic core must not pull in storage either.
echo "Check 4b: core packages must not depend on internal/storage"
# shellcheck disable=SC2086
if echo "$DEPS" | grep -q "internal/storage"; then
    echo "FAIL: core packages transitively depend on internal/storage:"
    echo "$DEPS" | grep "internal/storage" | sed 's/^/  > /'
    exit 1
fi
echo "  PASS: core has no dependency on internal/storage."
echo ""

echo "=== Isolation Lint Complete ==="
echo "RESULT: PASSED"
exit 0
