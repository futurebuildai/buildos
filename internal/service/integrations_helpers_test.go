package service

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/futurebuildai/buildos/internal/store"
)

// TestLastNRunes covers the non-secret last4 display-hint helper. It must be
// rune-safe (not byte-safe) so a multi-byte key tail isn't split mid-character,
// and must return the whole string when it is shorter than n rather than
// panicking on a negative slice index.
func TestLastNRunes(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"sk-ant-abcd1234", 4, "1234"},
		{"abc", 4, "abc"},   // shorter than n → whole string
		{"", 4, ""},         // empty
		{"wxyz", 4, "wxyz"}, // exactly n
		{"héllo", 2, "lo"},  // multi-byte earlier; tail is ASCII
		{"clé😀", 2, "é😀"},   // multi-byte tail must not split the emoji
	}
	for _, c := range cases {
		if got := lastNRunes(c.in, c.n); got != c.want {
			t.Errorf("lastNRunes(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

// TestMapVaultStoreError covers the vault-specific store→service translation:
// a Postgres unique violation (23505 — duplicate active credential per
// org+provider) becomes ErrInvalidInput (handler → 400/409, not 500);
// store.ErrNotFound → the service ErrNotFound sentinel; nil → nil; any other
// error passes through so unexpected failures surface as 500.
func TestMapVaultStoreError(t *testing.T) {
	if got := mapVaultStoreError(nil); got != nil {
		t.Errorf("mapVaultStoreError(nil) = %v, want nil", got)
	}

	dup := &pgconn.PgError{Code: "23505", ConstraintName: "integration_credentials_org_provider_key"}
	if got := mapVaultStoreError(dup); !errors.Is(got, ErrInvalidInput) {
		t.Errorf("mapVaultStoreError(23505) = %v, want ErrInvalidInput", got)
	}

	if got := mapVaultStoreError(store.ErrNotFound); !errors.Is(got, ErrNotFound) {
		t.Errorf("mapVaultStoreError(store.ErrNotFound) = %v, want ErrNotFound", got)
	}

	other := fmt.Errorf("vault offline")
	if got := mapVaultStoreError(other); !errors.Is(got, other) {
		t.Errorf("mapVaultStoreError(other) = %v, want passthrough", got)
	}
}
