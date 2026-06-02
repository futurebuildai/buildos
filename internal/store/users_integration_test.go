//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// mustCreateUser inserts a native user via the store and returns its id,
// failing the test on error. Centralizes the happy-path create so the token
// tests (which need a valid user_id FK) stay focused on their own assertions.
func mustCreateUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, s *UserStore, p CreateUserParams) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		u, err := s.CreateUser(ctx, tx, p)
		if err != nil {
			return err
		}
		id = u.ID
		return nil
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return id
}

// ---------- users ----------

func TestUserStore_CreateAndLookupRoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewUserStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	var created uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		u, err := s.CreateUser(ctx, tx, CreateUserParams{
			OrgID:        orgID,
			Email:        "Owner@Example.com", // mixed case — lookups are case-insensitive
			DisplayName:  "Ada Owner",
			Role:         "owner",
			PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$abc$def",
			// Locale intentionally empty → defaults to en-US at the store.
		})
		if err != nil {
			return err
		}
		created = u.ID
		if u.Locale != "en-US" {
			t.Errorf("locale default = %q, want en-US", u.Locale)
		}
		if u.Role != "owner" {
			t.Errorf("role = %q, want owner", u.Role)
		}
		if u.PasswordHash == "" {
			t.Error("password hash not round-tripped")
		}
		if u.LastLoginAt != nil {
			t.Errorf("last_login_at = %v, want nil on fresh user", u.LastLoginAt)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}

	// Case-insensitive lookups by email (org-scoped + global) and by id all
	// resolve to the same row.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		byEmail, err := s.GetUserByEmail(ctx, tx, orgID, "owner@example.com")
		if err != nil {
			return err
		}
		if byEmail.ID != created {
			t.Errorf("GetUserByEmail id = %v, want %v", byEmail.ID, created)
		}
		byGlobal, err := s.GetUserByEmailGlobal(ctx, tx, "OWNER@EXAMPLE.COM")
		if err != nil {
			return err
		}
		if byGlobal.ID != created {
			t.Errorf("GetUserByEmailGlobal id = %v, want %v", byGlobal.ID, created)
		}
		byID, err := s.GetUserByID(ctx, tx, created)
		if err != nil {
			return err
		}
		if byID.Email != "Owner@Example.com" {
			t.Errorf("GetUserByID email = %q, want original casing preserved", byID.Email)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("lookup tx: %v", err)
	}
}

func TestUserStore_CreateUser_DuplicateEmailConflicts(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewUserStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Dup Co")

	mustCreateUser(t, ctx, pool, s, CreateUserParams{
		OrgID: orgID, Email: "dup@example.com", DisplayName: "First",
		Role: "owner", PasswordHash: "h",
	})

	// Same org + same email (different casing) must violate the
	// users_org_email_uidx UNIQUE (org_id, lower(email)) index.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.CreateUser(ctx, tx, CreateUserParams{
			OrgID: orgID, Email: "DUP@example.com", DisplayName: "Second",
			Role: "admin", PasswordHash: "h2",
		})
		return err
	})
	if err == nil {
		t.Fatal("duplicate email in same org should conflict, got nil")
	}
}

func TestUserStore_Lookups_NotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewUserStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Empty Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := s.GetUserByEmail(ctx, tx, orgID, "nobody@example.com"); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetUserByEmail miss = %v, want ErrNotFound", err)
		}
		if _, err := s.GetUserByEmailGlobal(ctx, tx, "nobody@example.com"); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetUserByEmailGlobal miss = %v, want ErrNotFound", err)
		}
		if _, err := s.GetUserByID(ctx, tx, uuid.New()); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetUserByID miss = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestUserStore_CountUsersInOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewUserStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Count Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		n, err := s.CountUsersInOrg(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if n != 0 {
			t.Errorf("count on empty org = %d, want 0", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("empty count tx: %v", err)
	}

	mustCreateUser(t, ctx, pool, s, CreateUserParams{
		OrgID: orgID, Email: "first@example.com", DisplayName: "First",
		Role: "owner", PasswordHash: "h",
	})

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		n, err := s.CountUsersInOrg(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("count after one create = %d, want 1", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("count tx: %v", err)
	}
}

func TestUserStore_UpdatePasswordHash(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewUserStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Reset Co")
	userID := mustCreateUser(t, ctx, pool, s, CreateUserParams{
		OrgID: orgID, Email: "reset@example.com", DisplayName: "User",
		Role: "owner", PasswordHash: "old-hash",
	})

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.UpdatePasswordHash(ctx, tx, userID, "new-hash"); err != nil {
			return err
		}
		u, err := s.GetUserByID(ctx, tx, userID)
		if err != nil {
			return err
		}
		if u.PasswordHash != "new-hash" {
			t.Errorf("password hash = %q, want new-hash", u.PasswordHash)
		}
		// Missing user → ErrNotFound.
		if err := s.UpdatePasswordHash(ctx, tx, uuid.New(), "x"); !errors.Is(err, ErrNotFound) {
			t.Errorf("UpdatePasswordHash on missing = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestUserStore_TouchLastLogin(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewUserStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Login Co")
	userID := mustCreateUser(t, ctx, pool, s, CreateUserParams{
		OrgID: orgID, Email: "login@example.com", DisplayName: "User",
		Role: "owner", PasswordHash: "h",
	})

	now := time.Now().UTC().Truncate(time.Second)
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.TouchLastLogin(ctx, tx, userID, now); err != nil {
			return err
		}
		u, err := s.GetUserByID(ctx, tx, userID)
		if err != nil {
			return err
		}
		if u.LastLoginAt == nil || !u.LastLoginAt.Equal(now) {
			t.Errorf("last_login_at = %v, want %v", u.LastLoginAt, now)
		}
		if err := s.TouchLastLogin(ctx, tx, uuid.New(), now); !errors.Is(err, ErrNotFound) {
			t.Errorf("TouchLastLogin on missing = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

// ---------- auth_refresh_tokens ----------

func TestUserStore_RefreshTokenLifecycle(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewUserStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Token Co")
	userID := mustCreateUser(t, ctx, pool, s, CreateUserParams{
		OrgID: orgID, Email: "tok@example.com", DisplayName: "User",
		Role: "owner", PasswordHash: "h",
	})

	now := time.Now().UTC()
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		id, err := s.CreateRefreshToken(ctx, tx, CreateRefreshTokenParams{
			UserID: userID, OrgID: orgID, TokenHash: "hash-active",
			ExpiresAt: now.Add(24 * time.Hour),
		})
		if err != nil {
			return err
		}

		// Active token resolves by hash.
		got, err := s.GetActiveRefreshToken(ctx, tx, "hash-active", now)
		if err != nil {
			return err
		}
		if got.ID != id || got.UserID != userID {
			t.Errorf("got token (%v,%v), want (%v,%v)", got.ID, got.UserID, id, userID)
		}

		// An expired token is excluded (evaluate "now" past its expiry).
		if _, err := s.GetActiveRefreshToken(ctx, tx, "hash-active", now.Add(48*time.Hour)); !errors.Is(err, ErrNotFound) {
			t.Errorf("expired lookup = %v, want ErrNotFound", err)
		}

		// Revoke → no longer active.
		if err := s.RevokeRefreshToken(ctx, tx, id, now); err != nil {
			return err
		}
		if _, err := s.GetActiveRefreshToken(ctx, tx, "hash-active", now); !errors.Is(err, ErrNotFound) {
			t.Errorf("revoked lookup = %v, want ErrNotFound", err)
		}
		// Double-revoke detects the race → ErrNotFound.
		if err := s.RevokeRefreshToken(ctx, tx, id, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("double revoke = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestUserStore_RevokeAllRefreshTokensForUser(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewUserStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Logout Co")
	userID := mustCreateUser(t, ctx, pool, s, CreateUserParams{
		OrgID: orgID, Email: "all@example.com", DisplayName: "User",
		Role: "owner", PasswordHash: "h",
	})

	now := time.Now().UTC()
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, h := range []string{"h1", "h2", "h3"} {
			if _, err := s.CreateRefreshToken(ctx, tx, CreateRefreshTokenParams{
				UserID: userID, OrgID: orgID, TokenHash: h,
				ExpiresAt: now.Add(time.Hour),
			}); err != nil {
				return err
			}
		}
		n, err := s.RevokeAllRefreshTokensForUser(ctx, tx, userID, now)
		if err != nil {
			return err
		}
		if n != 3 {
			t.Errorf("revoked %d tokens, want 3", n)
		}
		// A second sweep finds nothing active.
		n2, err := s.RevokeAllRefreshTokensForUser(ctx, tx, userID, now)
		if err != nil {
			return err
		}
		if n2 != 0 {
			t.Errorf("second sweep revoked %d, want 0", n2)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

// ---------- auth_password_reset_tokens ----------

func TestUserStore_PasswordResetTokenLifecycle(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewUserStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "PwReset Co")
	userID := mustCreateUser(t, ctx, pool, s, CreateUserParams{
		OrgID: orgID, Email: "pw@example.com", DisplayName: "User",
		Role: "owner", PasswordHash: "h",
	})

	now := time.Now().UTC()
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		id, err := s.CreatePasswordResetToken(ctx, tx, CreatePasswordResetTokenParams{
			UserID: userID, TokenHash: "reset-hash", ExpiresAt: now.Add(time.Hour),
		})
		if err != nil {
			return err
		}

		got, err := s.GetActivePasswordResetToken(ctx, tx, "reset-hash", now)
		if err != nil {
			return err
		}
		if got.ID != id || got.UserID != userID {
			t.Errorf("got reset token (%v,%v), want (%v,%v)", got.ID, got.UserID, id, userID)
		}

		// Expired reset token excluded.
		if _, err := s.GetActivePasswordResetToken(ctx, tx, "reset-hash", now.Add(2*time.Hour)); !errors.Is(err, ErrNotFound) {
			t.Errorf("expired reset lookup = %v, want ErrNotFound", err)
		}

		// Redeem → no longer active; replay redeem detects reuse → ErrNotFound.
		if err := s.RedeemPasswordResetToken(ctx, tx, id, now); err != nil {
			return err
		}
		if _, err := s.GetActivePasswordResetToken(ctx, tx, "reset-hash", now); !errors.Is(err, ErrNotFound) {
			t.Errorf("redeemed lookup = %v, want ErrNotFound", err)
		}
		if err := s.RedeemPasswordResetToken(ctx, tx, id, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("replay redeem = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
