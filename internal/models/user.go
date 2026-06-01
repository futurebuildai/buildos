package models

import (
	"time"

	"github.com/google/uuid"
)

// User mirrors the users table row. With native authentication BuildOS owns
// the credential directly: PasswordHash holds an argon2id encoded hash and is
// never serialized to the wire (json:"-"). OIDCSubject is a legacy column,
// nullable now that identities are minted locally rather than by The Brain.
type User struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	Email        string     `json:"email"`
	DisplayName  string     `json:"display_name"`
	Role         string     `json:"role"`
	Locale       string     `json:"locale"`
	OIDCSubject  *string    `json:"-"`
	PasswordHash string     `json:"-"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// RefreshToken mirrors an auth_refresh_tokens row. TokenHash (hex sha256) is
// internal-only; the cleartext lives only in the client's possession.
type RefreshToken struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	OrgID      uuid.UUID  `json:"org_id"`
	TokenHash  string     `json:"-"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

// PasswordResetToken mirrors an auth_password_reset_tokens row. Single-use:
// valid only while RedeemedAt is nil and now < ExpiresAt.
type PasswordResetToken struct {
	ID         uuid.UUID  `json:"id"`
	UserID     uuid.UUID  `json:"user_id"`
	TokenHash  string     `json:"-"`
	IssuedAt   time.Time  `json:"issued_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RedeemedAt *time.Time `json:"redeemed_at,omitempty"`
}
