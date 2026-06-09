// Package connectors is the BuildOS integration seam — named providers of
// agentic tools that an operator enables and configures post-deploy. Built-in
// connectors (3b-i) are in-process and read-only; the MCP connector (3b-ii)
// proxies an external server.
//
// Layering (HARD — enforced by scripts/check-isolation.sh Check 3): this package
// imports ONLY the standard library, github.com/google/uuid, and
// github.com/futurebuildai/buildos/internal/agentic. It MUST NOT import
// internal/service, internal/store, internal/ai, or any other internal/* package.
// The dependency arrow is agentic <- connectors <- service: a connector produces
// agentic.Tool values; internal/service owns per-org enable/config + the
// MinRole floor + the merge into the per-request assistant registry.
package connectors

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/agentic"
)

// Caller is the sealed identity a connector binds into its tool executors,
// mirroring the experience flow's closure-binding — the model never sees these
// fields; they are captured at registry-build time, not supplied as tool args.
type Caller struct {
	OrgID uuid.UUID
	Role  string
	Sub   string
}

// Connector is a named provider of agentic tools. A connector PRODUCES tools
// (with identity-sealed executors); per-org enable and the admin MinRole floor
// are the SERVICE's job (service.ConnectorService), not the connector's.
type Connector interface {
	// Name is the stable connector id (matches connectors_config.connector_name).
	Name() string
	// Description is admin-facing prose for the GET catalog.
	Description() string
	// BuildTools returns the tools this connector contributes for the caller,
	// with identity-sealed executors. It returns an error only on an internal
	// failure; the service soft-fails (skips) a connector whose BuildTools errors.
	BuildTools(ctx context.Context, c Caller) ([]agentic.Tool, error)
}

// Builtins returns the built-in connector catalog the binary ships. 3b-i: the
// in-process reference connector only. (3b-ii adds the MCP connector type.)
func Builtins() []Connector {
	return []Connector{newReferenceConnector()}
}

// toolNameRE is Anthropic's tool-name constraint. A name outside this charset
// would 400 the whole Messages API call, so an invalid namespaced name is
// dropped by the service rather than mounted.
var toolNameRE = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// validToolName reports whether s is a legal Anthropic tool name.
func validToolName(s string) bool { return toolNameRE.MatchString(s) }

// ValidToolName is the exported guard used by the service when namespacing.
func ValidToolName(s string) bool { return validToolName(s) }

// NamespaceToolName deterministically prefixes a connector tool so it can never
// collide with an internal ERP tool (which use bare names like "list_projects")
// or with another connector's tool. The prefix encodes the connector name so
// two connectors are mutually disjoint.
func NamespaceToolName(connector, tool string) string {
	return "conn__" + connector + "__" + tool
}

// executorFunc adapts a plain function to agentic.ToolExecutor (local to this
// package so connectors does not depend on the service-layer adapter).
type executorFunc func(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error)

func (f executorFunc) Execute(ctx context.Context, input json.RawMessage) (agentic.ToolResult, error) {
	return f(ctx, input)
}
