package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/futurebuildai/buildos/internal/store"
)

// TestValidateEmail covers the minimal structural gate at the auth boundary.
// It is deliberately permissive — the real check is the confirmation email —
// but it must reject obvious garbage (no "@", empty local/domain, embedded
// whitespace) so junk never reaches the store as a "valid" identity.
func TestValidateEmail(t *testing.T) {
	valid := []string{
		"a@b",
		"owner@example.com",
		"first.last+tag@sub.domain.io",
	}
	for _, e := range valid {
		if err := validateEmail(e); err != nil {
			t.Errorf("validateEmail(%q) = %v, want nil", e, err)
		}
	}

	invalid := []string{
		"",                // empty
		"no-at-sign",      // missing @
		"@nolocal.com",    // empty local part
		"nodomain@",       // empty domain part
		"has space@x.com", // embedded space
		"tab\t@x.com",     // embedded tab
		"new\nline@x.com", // embedded newline
	}
	for _, e := range invalid {
		err := validateEmail(e)
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("validateEmail(%q) = %v, want ErrInvalidInput", e, err)
		}
	}
}

// TestValidatePassword covers the length floor (12). The argon2id hash, not
// composition rules, is what defends long passphrases — so the only gate is
// length, and the boundary (11 reject / 12 accept) must hold exactly.
func TestValidatePassword(t *testing.T) {
	if err := validatePassword("01234567890"); !errors.Is(err, ErrInvalidInput) { // 11 chars
		t.Errorf("validatePassword(11 chars) = %v, want ErrInvalidInput", err)
	}
	if err := validatePassword("012345678901"); err != nil { // 12 chars
		t.Errorf("validatePassword(12 chars) = %v, want nil", err)
	}
	if err := validatePassword("a-very-long-correct-horse-battery-staple"); err != nil {
		t.Errorf("validatePassword(long) = %v, want nil", err)
	}
}

// TestMapAuthStoreError covers the auth-specific store→service translation:
// a Postgres unique violation (23505 — e.g. duplicate email on claim/reset)
// becomes ErrInvalidInput (so the handler returns 400/409, not 500) and carries
// the constraint name; everything else falls through to the shared mapper
// (store.ErrNotFound → ErrNotFound; nil → nil; other → passthrough).
func TestMapAuthStoreError(t *testing.T) {
	if got := mapAuthStoreError(nil); got != nil {
		t.Errorf("mapAuthStoreError(nil) = %v, want nil", got)
	}

	dup := &pgconn.PgError{Code: "23505", ConstraintName: "users_email_key"}
	got := mapAuthStoreError(dup)
	if !errors.Is(got, ErrInvalidInput) {
		t.Fatalf("mapAuthStoreError(23505) = %v, want ErrInvalidInput", got)
	}
	if got.Error() == "" || !errors.Is(got, ErrInvalidInput) {
		t.Errorf("mapAuthStoreError(23505) should wrap the constraint name, got %q", got.Error())
	}

	if got := mapAuthStoreError(store.ErrNotFound); !errors.Is(got, ErrNotFound) {
		t.Errorf("mapAuthStoreError(store.ErrNotFound) = %v, want ErrNotFound", got)
	}

	// A non-unique pg error (not 23505) must NOT be downgraded to a 4xx —
	// it passes through so the handler surfaces a 500.
	other := fmt.Errorf("boom")
	if got := mapAuthStoreError(other); !errors.Is(got, other) {
		t.Errorf("mapAuthStoreError(other) = %v, want passthrough", got)
	}
}
