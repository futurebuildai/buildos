// Package agentic is the isolated AI-orchestration core for BuildOS — the
// "agentic harness" that wraps the deterministic CPM/PM engine. It owns the
// orchestration of multi-step, cross-module AI reasoning (e.g. turning a
// schedule slip into a reasoned cross-module cascade) without ever touching
// the deterministic engine's math.
//
// Isolation contract (HARD — do not break):
//
//   - This package is a LEAF. It imports ONLY the standard library,
//     github.com/google/uuid, and log/slog. It MUST NOT import any other
//     internal/* package — in particular not internal/service, internal/store,
//     internal/ai, internal/worker, internal/physics, or internal/currency.
//   - It declares its own port interfaces (see ports.go). Concrete adapters
//     that bridge to AI providers, the database, and the deterministic engine
//     live in internal/service and implement these ports. Dependency direction
//     is always inward: callers depend on agentic, never the reverse.
//   - internal/physics and internal/currency (the deterministic core) MUST
//     NEVER import internal/agentic.
//
// Exact/Fuzzy split (HARD): everything in this package is JUDGMENT only. The
// AI reasons about blast radius and recommends; it never recomputes the
// schedule or any monetary total. The deterministic engine stays
// authoritative — the Workspace port loads engine-computed facts in and
// applies advisory output (feed cards + audit) out, all behind a transaction
// boundary the adapter owns. agentic itself holds no pgx, no currency math,
// and no CPM.
package agentic
