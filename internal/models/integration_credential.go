package models

import (
	"time"

	"github.com/google/uuid"
)

// IntegrationCredential mirrors a row in integration_credentials — a
// per-org 3rd-party API key held in the encrypted BYOK vault (WS3).
//
// Secret handling: Ciphertext and Nonce are AES-256-GCM material (see
// internal/cryptobox) and carry json:"-" so the secret bytes can never
// leak onto the wire. The cleartext key itself is never a field on this
// struct — it exists only transiently inside the service layer during
// Seal (set) and Open (resolve). Last4 is the last 4 chars of the
// cleartext key, kept for UI display only ("sk-…ab12"); it is NOT
// secret and is safe to emit.
type IntegrationCredential struct {
	ID         uuid.UUID `json:"id"`
	OrgID      uuid.UUID `json:"org_id"`
	Provider   string    `json:"provider"`
	Label      string    `json:"label"`
	Ciphertext []byte    `json:"-"` // AES-256-GCM ciphertext — never emitted
	Nonce      []byte    `json:"-"` // GCM nonce — never emitted
	KeyVersion int       `json:"-"` // cryptobox master-key version — internal only
	Last4      string    `json:"last4"`
	IsActive   bool      `json:"is_active"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
