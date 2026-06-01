package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// UserStore manages the users table and the native-auth token tables
// (auth_refresh_tokens, auth_password_reset_tokens). Like the other stores it
// is stateless and every method takes a pgx.Tx so the service layer can
// compose a mutation with its audit-log write in one transaction.
type UserStore struct{}

// NewUserStore constructs a UserStore. Stateless — safe to share.
func NewUserStore() *UserStore { return &UserStore{} }

const userColumns = `id, org_id, email, display_name, role, locale,
	oidc_subject, password_hash, last_login_at, created_at, updated_at`

func scanUser(row pgx.Row) (models.User, error) {
	var u models.User
	var passwordHash *string
	err := row.Scan(
		&u.ID, &u.OrgID, &u.Email, &u.DisplayName, &u.Role, &u.Locale,
		&u.OIDCSubject, &passwordHash, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return models.User{}, err
	}
	if passwordHash != nil {
		u.PasswordHash = *passwordHash
	}
	return u, nil
}

// CreateUserParams is the input for inserting a native user. PasswordHash is
// an argon2id encoded hash (never cleartext). Role defaults to field_worker
// at the SQL layer if empty, but the service always supplies it explicitly.
type CreateUserParams struct {
	OrgID        uuid.UUID
	Email        string
	DisplayName  string
	Role         string
	PasswordHash string
	Locale       string
}

// CreateUser inserts a native (password-backed) user with no oidc_subject.
// The UNIQUE index users_org_email_uidx on (org_id, lower(email)) means a
// duplicate email in the same org surfaces as a unique-violation, which the
// service maps to a conflict.
func (s *UserStore) CreateUser(ctx context.Context, tx pgx.Tx, p CreateUserParams) (models.User, error) {
	locale := p.Locale
	if locale == "" {
		locale = "en-US"
	}
	return scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (org_id, email, display_name, role, locale, password_hash)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING `+userColumns,
		p.OrgID, p.Email, p.DisplayName, p.Role, locale, p.PasswordHash,
	))
}

// GetUserByEmail looks up a user by org + case-insensitive email. Returns
// ErrNotFound when no row matches. The login path uses this; a non-match maps
// to the same generic "invalid credentials" response as a password mismatch.
func (s *UserStore) GetUserByEmail(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, email string) (models.User, error) {
	u, err := scanUser(tx.QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE org_id = $1 AND lower(email) = lower($2)`,
		orgID, email,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, ErrNotFound
		}
		return models.User{}, fmt.Errorf("get user by email: %w", err)
	}
	return u, nil
}

// GetUserByEmailGlobal looks up a user by case-insensitive email across all
// orgs. In the single-tenant fork model (ADR-002) there is exactly one
// organization, so email is effectively globally unique and this backs the
// unauthenticated login path (which has no org_id to scope by). Returns
// ErrNotFound when no row matches; if more than one row matches (only
// possible in a future multi-tenant co-op variant) it returns the
// earliest-created match deterministically.
func (s *UserStore) GetUserByEmailGlobal(ctx context.Context, tx pgx.Tx, email string) (models.User, error) {
	u, err := scanUser(tx.QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE lower(email) = lower($1)
		ORDER BY created_at ASC
		LIMIT 1`, email,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, ErrNotFound
		}
		return models.User{}, fmt.Errorf("get user by email (global): %w", err)
	}
	return u, nil
}

// GetUserByID looks up a user by primary key. Returns ErrNotFound when absent.
func (s *UserStore) GetUserByID(ctx context.Context, tx pgx.Tx, id uuid.UUID) (models.User, error) {
	u, err := scanUser(tx.QueryRow(ctx, `
		SELECT `+userColumns+`
		FROM users
		WHERE id = $1`, id,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, ErrNotFound
		}
		return models.User{}, fmt.Errorf("get user by id: %w", err)
	}
	return u, nil
}

// CountUsersInOrg returns how many users exist for an org. The bootstrap
// flow uses this to enforce "first owner only" — redemption is refused once
// any user already exists.
func (s *UserStore) CountUsersInOrg(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE org_id = $1`, orgID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users in org: %w", err)
	}
	return n, nil
}

// UpdatePasswordHash replaces a user's stored hash (password reset / change).
// Returns ErrNotFound when the user row is missing.
func (s *UserStore) UpdatePasswordHash(ctx context.Context, tx pgx.Tx, userID uuid.UUID, passwordHash string) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE users SET password_hash = $2, updated_at = now()
		WHERE id = $1`, userID, passwordHash)
	if err != nil {
		return fmt.Errorf("update password hash: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// TouchLastLogin stamps last_login_at. Best-effort freshness for the UI;
// returns ErrNotFound when the user row is missing.
func (s *UserStore) TouchLastLogin(ctx context.Context, tx pgx.Tx, userID uuid.UUID, now time.Time) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE users SET last_login_at = $2 WHERE id = $1`, userID, now)
	if err != nil {
		return fmt.Errorf("touch last login: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- auth_refresh_tokens ----------

// CreateRefreshTokenParams stores a pre-hashed refresh token. The cleartext is
// shown to the client once; only TokenHash (hex sha256) lands in the DB.
type CreateRefreshTokenParams struct {
	UserID    uuid.UUID
	OrgID     uuid.UUID
	TokenHash string
	ExpiresAt time.Time
}

// CreateRefreshToken persists a refresh token row and returns its id.
func (s *UserStore) CreateRefreshToken(ctx context.Context, tx pgx.Tx, p CreateRefreshTokenParams) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO auth_refresh_tokens (user_id, org_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		p.UserID, p.OrgID, p.TokenHash, p.ExpiresAt,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert refresh token: %w", err)
	}
	return id, nil
}

// GetActiveRefreshToken looks up an unrevoked, unexpired refresh token by its
// hash. Returns ErrNotFound when no row matches — even if a row exists but is
// revoked or expired, so callers cannot distinguish those cases.
func (s *UserStore) GetActiveRefreshToken(ctx context.Context, tx pgx.Tx, hash string, now time.Time) (models.RefreshToken, error) {
	var t models.RefreshToken
	err := tx.QueryRow(ctx, `
		SELECT id, user_id, org_id, token_hash, issued_at, expires_at, revoked_at, last_used_at
		FROM auth_refresh_tokens
		WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > $2`,
		hash, now,
	).Scan(&t.ID, &t.UserID, &t.OrgID, &t.TokenHash, &t.IssuedAt, &t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.RefreshToken{}, ErrNotFound
		}
		return models.RefreshToken{}, fmt.Errorf("get active refresh token: %w", err)
	}
	return t, nil
}

// RevokeRefreshToken marks a single refresh token revoked by id. The WHERE
// double-checks revoked_at IS NULL so a concurrent rotation sees
// RowsAffected=0 and the caller can detect the race. Returns ErrNotFound when
// the token is missing or already revoked.
func (s *UserStore) RevokeRefreshToken(ctx context.Context, tx pgx.Tx, tokenID uuid.UUID, now time.Time) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE auth_refresh_tokens SET revoked_at = $2
		WHERE id = $1 AND revoked_at IS NULL`, tokenID, now)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RevokeAllRefreshTokensForUser revokes every active refresh token for a user
// (logout-everywhere / password reset). Returns the number revoked.
func (s *UserStore) RevokeAllRefreshTokensForUser(ctx context.Context, tx pgx.Tx, userID uuid.UUID, now time.Time) (int64, error) {
	cmd, err := tx.Exec(ctx, `
		UPDATE auth_refresh_tokens SET revoked_at = $2
		WHERE user_id = $1 AND revoked_at IS NULL`, userID, now)
	if err != nil {
		return 0, fmt.Errorf("revoke all refresh tokens: %w", err)
	}
	return cmd.RowsAffected(), nil
}

// ---------- auth_password_reset_tokens ----------

// CreatePasswordResetTokenParams stores a pre-hashed reset token.
type CreatePasswordResetTokenParams struct {
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
}

// CreatePasswordResetToken persists a reset token row and returns its id.
func (s *UserStore) CreatePasswordResetToken(ctx context.Context, tx pgx.Tx, p CreatePasswordResetTokenParams) (uuid.UUID, error) {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO auth_password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id`,
		p.UserID, p.TokenHash, p.ExpiresAt,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("insert password reset token: %w", err)
	}
	return id, nil
}

// GetActivePasswordResetToken looks up an unredeemed, unexpired reset token by
// hash. Returns ErrNotFound when no row matches.
func (s *UserStore) GetActivePasswordResetToken(ctx context.Context, tx pgx.Tx, hash string, now time.Time) (models.PasswordResetToken, error) {
	var t models.PasswordResetToken
	err := tx.QueryRow(ctx, `
		SELECT id, user_id, token_hash, issued_at, expires_at, redeemed_at
		FROM auth_password_reset_tokens
		WHERE token_hash = $1 AND redeemed_at IS NULL AND expires_at > $2`,
		hash, now,
	).Scan(&t.ID, &t.UserID, &t.TokenHash, &t.IssuedAt, &t.ExpiresAt, &t.RedeemedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.PasswordResetToken{}, ErrNotFound
		}
		return models.PasswordResetToken{}, fmt.Errorf("get active password reset token: %w", err)
	}
	return t, nil
}

// RedeemPasswordResetToken marks a reset token redeemed. The WHERE double-
// checks redeemed_at IS NULL so a replayed token sees RowsAffected=0. Returns
// ErrNotFound when missing or already redeemed.
func (s *UserStore) RedeemPasswordResetToken(ctx context.Context, tx pgx.Tx, tokenID uuid.UUID, now time.Time) error {
	cmd, err := tx.Exec(ctx, `
		UPDATE auth_password_reset_tokens SET redeemed_at = $2
		WHERE id = $1 AND redeemed_at IS NULL`, tokenID, now)
	if err != nil {
		return fmt.Errorf("redeem password reset token: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
