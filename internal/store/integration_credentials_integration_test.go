//go:build integration

package store

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// upsertParams builds a credential-upsert input with synthetic sealed bytes.
// The store never inspects the ciphertext/nonce — sealing is the service's job
// (cryptobox) — so opaque test bytes are sufficient to exercise the SQL.
func upsertParams(orgID uuid.UUID, provider, label, last4 string) UpsertActiveCredentialParams {
	return UpsertActiveCredentialParams{
		OrgID:      orgID,
		Provider:   provider,
		Label:      label,
		Ciphertext: []byte("ciphertext-" + last4),
		Nonce:      []byte("nonce-" + last4),
		KeyVersion: 1,
		Last4:      last4,
		CreatedBy:  "owner-sub",
	}
}

func TestIntegrationCredentialStore_UpsertRotatesActive(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewIntegrationCredentialStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Vault Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		first, err := s.UpsertActive(ctx, tx, upsertParams(orgID, "anthropic", "first key", "aaaa"))
		if err != nil {
			return err
		}
		if !first.IsActive || first.Last4 != "aaaa" {
			t.Errorf("first credential = %+v, want active last4=aaaa", first)
		}

		// Rotate: a second upsert for the SAME (org, provider) must
		// deactivate the first and activate the second. The partial unique
		// index integration_credentials_active_uidx forbids two active rows;
		// this insert succeeding proves the deactivate ran in the same tx.
		second, err := s.UpsertActive(ctx, tx, upsertParams(orgID, "anthropic", "rotated key", "bbbb"))
		if err != nil {
			return err
		}
		if second.ID == first.ID {
			t.Error("rotation should insert a new row, not update in place")
		}

		// Exactly one active row, and it's the new one.
		active, err := s.GetActiveByProvider(ctx, tx, orgID, "anthropic")
		if err != nil {
			return err
		}
		if active.ID != second.ID || active.Last4 != "bbbb" {
			t.Errorf("active credential = %+v, want second (last4=bbbb)", active)
		}

		// ListByOrg returns both rows (active + the deactivated original).
		all, err := s.ListByOrg(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if len(all) != 2 {
			t.Fatalf("ListByOrg len = %d, want 2", len(all))
		}
		activeCount := 0
		for _, c := range all {
			if c.IsActive {
				activeCount++
			}
		}
		if activeCount != 1 {
			t.Errorf("active rows = %d, want exactly 1", activeCount)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestIntegrationCredentialStore_GetActive_NotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewIntegrationCredentialStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Empty Vault Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// No credential set → soft-fail sentinel (the resolver treats this as
		// "AI unconfigured", not an error).
		if _, err := s.GetActiveByProvider(ctx, tx, orgID, "anthropic"); !errors.Is(err, ErrNotFound) {
			t.Errorf("GetActiveByProvider on empty = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestIntegrationCredentialStore_DeactivateByProvider(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewIntegrationCredentialStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Deactivate Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := s.UpsertActive(ctx, tx, upsertParams(orgID, "resend", "email key", "zzzz")); err != nil {
			return err
		}

		// First deactivate flips the one active row.
		n, err := s.DeactivateByProvider(ctx, tx, orgID, "resend")
		if err != nil {
			return err
		}
		if n != 1 {
			t.Errorf("deactivate affected %d rows, want 1", n)
		}
		if _, err := s.GetActiveByProvider(ctx, tx, orgID, "resend"); !errors.Is(err, ErrNotFound) {
			t.Errorf("after deactivate, GetActive = %v, want ErrNotFound", err)
		}

		// Second deactivate is a no-op (nothing active) → 0 rows, which the
		// service maps to ErrNotFound.
		n2, err := s.DeactivateByProvider(ctx, tx, orgID, "resend")
		if err != nil {
			return err
		}
		if n2 != 0 {
			t.Errorf("second deactivate affected %d rows, want 0", n2)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestIntegrationCredentialStore_ListByOrg_IsolatesAndOrders(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewIntegrationCredentialStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Two distinct providers for org A; one for org B.
		if _, err := s.UpsertActive(ctx, tx, upsertParams(orgA, "anthropic", "ai", "a1a1")); err != nil {
			return err
		}
		if _, err := s.UpsertActive(ctx, tx, upsertParams(orgA, "resend", "email", "a2a2")); err != nil {
			return err
		}
		if _, err := s.UpsertActive(ctx, tx, upsertParams(orgB, "anthropic", "other-org", "b1b1")); err != nil {
			return err
		}

		aList, err := s.ListByOrg(ctx, tx, orgA)
		if err != nil {
			return err
		}
		if len(aList) != 2 {
			t.Fatalf("org A list len = %d, want 2", len(aList))
		}
		// Org isolation: none of org A's rows belong to org B.
		for _, c := range aList {
			if c.OrgID != orgA {
				t.Errorf("org A list contains foreign org_id %v", c.OrgID)
			}
		}

		bList, err := s.ListByOrg(ctx, tx, orgB)
		if err != nil {
			return err
		}
		if len(bList) != 1 || bList[0].Last4 != "b1b1" {
			t.Errorf("org B list = %+v, want single b1b1 row", bList)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
