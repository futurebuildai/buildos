package models

import (
	"time"

	"github.com/google/uuid"
)

// Asset is a row in the assets table: a registry entry for a blob held in the
// per-fork S3-compatible object store (Cloudflare R2). The bytes live in R2;
// this row tracks the opaque object key, content-type, size, lifecycle status,
// and uploader. (DAILY_REPORTS_CLIENT_UPDATES Chunk A.)
//
// StorageKey is json:"-": clients NEVER receive the raw object key — they get
// short-lived signed URLs (or the same-origin proxy) instead. Exposing the key
// would leak the bucket layout and let a client probe sibling objects.
type Asset struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	ProjectID   *uuid.UUID `json:"project_id,omitempty"`
	StorageKey  string     `json:"-"` // opaque bucket key — never serialized to clients
	ContentType string     `json:"content_type"`
	SizeBytes   int64      `json:"size_bytes"`
	// Status lifecycle: pending -> ready -> failed.
	Status         string     `json:"status"`
	UploadedBy     string     `json:"uploaded_by"` // caller's JWT sub
	ChecksumSHA256 *string    `json:"checksum_sha256,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	ConfirmedAt    *time.Time `json:"confirmed_at,omitempty"`
}

// Asset status constants.
const (
	AssetStatusPending = "pending"
	AssetStatusReady   = "ready"
	AssetStatusFailed  = "failed"
)
