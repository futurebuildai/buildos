package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AgentConfig is one org's post-deploy configuration for a single agentic
// capability (Phase 3a). It is the DB-backed, admin-editable counterpart to the
// in-code capability catalog (internal/agentic.Descriptor): the catalog says
// what the binary CAN run; this row says whether an org has it ENABLED and how
// it is tuned. Absence of a row means "enabled with the catalog default" — so a
// row only ever encodes an override.
//
// Config carries capability-specific tuning (a JSON object). It must NEVER hold
// secrets — credentials belong in the encrypted vault (internal/cryptobox), not
// here.
type AgentConfig struct {
	ID         uuid.UUID       `json:"id"`
	OrgID      uuid.UUID       `json:"org_id"`
	Capability string          `json:"capability"`
	Enabled    bool            `json:"enabled"`
	Config     json.RawMessage `json:"config"`
	UpdatedBy  string          `json:"updated_by"` // OIDC subject of the last writer
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}
