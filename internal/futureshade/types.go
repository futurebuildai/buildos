package futureshade

import (
	"time"
)

// ShadowDoc represents a document analyzed by the Shadow layer.
type ShadowDoc struct {
	ID          string                 `json:"id"`
	SourceType  string                 `json:"source_type"` // "PRD", "Spec", "Code"
	SourceID    string                 `json:"source_id"`   // File path or DB ID
	ContentHash string                 `json:"content_hash"`
	Analysis    map[string]interface{} `json:"analysis"` // JSONB bucket for AI output
	CreatedAt   time.Time              `json:"created_at"`
}
