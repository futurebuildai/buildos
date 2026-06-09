package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ConnectorConfig is one org's post-deploy configuration for a single
// integration connector (Phase 3b). One row per (org_id, connector_name).
// Connectors are DEFAULT-OFF: absence of a row means disabled, so a row only
// ever encodes an explicit admin opt-in (and forward-compatible settings).
//
// Config carries non-secret connector settings (3b-ii: endpoint, allowed-tools).
// It must NEVER hold credentials — those belong in the encrypted vault
// (internal/cryptobox / integration_credentials), not here.
type ConnectorConfig struct {
	ID            uuid.UUID       `json:"id"`
	OrgID         uuid.UUID       `json:"org_id"`
	ConnectorName string          `json:"connector_name"`
	Kind          string          `json:"kind"` // "builtin" | "mcp" (Phase 3b-ii)
	Enabled       bool            `json:"enabled"`
	Config        json.RawMessage `json:"config"`
	UpdatedBy     string          `json:"updated_by"` // OIDC subject of the last writer
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
}

// ConnectorTool is one cached tool for an MCP connector instance (Phase 3b-ii),
// fetched from the server's tools/list at refresh time. tool_name is the REMOTE
// (un-namespaced) name; the service namespaces it before mounting into chat.
type ConnectorTool struct {
	ID            uuid.UUID       `json:"id"`
	OrgID         uuid.UUID       `json:"org_id"`
	ConnectorName string          `json:"connector_name"`
	ToolName      string          `json:"tool_name"`
	Description   string          `json:"description"`
	InputSchema   json.RawMessage `json:"input_schema"`
	FetchedAt     time.Time       `json:"fetched_at"`
}
