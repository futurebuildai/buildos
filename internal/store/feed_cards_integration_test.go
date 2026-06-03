//go:build integration

package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// seedFeedCards inserts a fixed test fixture used by the list/transition
// tests. Returns the list of card IDs in the order they were inserted.
//
// Cards layout (matches expectations of TestFeedCardsStore_*):
//
//	[0] active, target_user_id=userA,  priority=normal
//	[1] active, target_role=admin,     priority=urgent
//	[2] active, target_role=field_worker, priority=low
//	[3] active, target_role=admin,     priority=critical
//	[4] active, target_user_id=userB,  priority=normal
func seedFeedCards(t *testing.T, pool *pgxpool.Pool, ctx context.Context, orgID, userA, userB uuid.UUID) []uuid.UUID {
	t.Helper()
	s := NewFeedCardsStore()

	roleAdmin := "admin"
	roleField := "field_worker"

	specs := []CreateFeedCardParams{
		{OrgID: orgID, CardType: "info", Title: "for user A", Body: "u", Priority: "normal", TargetUserID: &userA, Actions: json.RawMessage(`null`)},
		{OrgID: orgID, CardType: "alert", Title: "admin alert", Body: "a", Priority: "urgent", TargetRole: &roleAdmin, Actions: json.RawMessage(`null`)},
		{OrgID: orgID, CardType: "info", Title: "field info", Body: "f", Priority: "low", TargetRole: &roleField, Actions: json.RawMessage(`null`)},
		{OrgID: orgID, CardType: "alert", Title: "critical admin", Body: "c", Priority: "critical", TargetRole: &roleAdmin, Actions: json.RawMessage(`null`)},
		{OrgID: orgID, CardType: "info", Title: "for user B", Body: "u", Priority: "normal", TargetUserID: &userB, Actions: json.RawMessage(`null`)},
	}

	ids := make([]uuid.UUID, 0, len(specs))
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, spec := range specs {
			c, err := s.CreateFeedCard(ctx, tx, spec)
			if err != nil {
				return err
			}
			ids = append(ids, c.ID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed feed cards: %v", err)
	}
	return ids
}

func TestFeedCardsStore_ListFeedCards_TargetingAndOrdering(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFeedCardsStore()
	ctx := context.Background()

	orgID := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedUser(t, pool, userA, orgID)
	testdb.SeedUser(t, pool, userB, orgID)

	ids := seedFeedCards(t, pool, ctx, orgID, userA, userB)
	if len(ids) != 5 {
		t.Fatalf("expected 5 seeded cards, got %d", len(ids))
	}

	// Mark one of the admin-role cards dismissed and another actioned
	// to exercise the status filter.
	if _, err := pool.Exec(ctx, `UPDATE feed_cards SET status = 'dismissed' WHERE id = $1`, ids[1]); err != nil {
		t.Fatalf("set dismissed: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE feed_cards SET status = 'actioned', actioned_at = now() WHERE id = $1`, ids[2]); err != nil {
		t.Fatalf("set actioned: %v", err)
	}

	t.Run("admin role sees user_A-targeted + admin role + critical admin", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			res, err := s.ListFeedCards(ctx, tx, ListFeedCardsParams{
				OrgID:             orgID,
				CallerOIDCSubject: userA.String(), // native JWT sub IS the user id
				CallerRole:        "admin",
			})
			if err != nil {
				return err
			}
			// userA-targeted + 2 admin-role cards = 3 — but one admin
			// card is dismissed, so default status='active' filter
			// returns 2 (userA + critical admin).
			if res.Total != 2 {
				t.Errorf("Total = %d, want 2", res.Total)
			}
			if len(res.Cards) != 2 {
				t.Errorf("len(Cards) = %d, want 2", len(res.Cards))
			}
			// Ordering: critical first, then normal.
			if len(res.Cards) >= 2 {
				if res.Cards[0].Priority != "critical" {
					t.Errorf("first card priority = %q, want critical", res.Cards[0].Priority)
				}
				if res.Cards[1].Priority != "normal" {
					t.Errorf("second card priority = %q, want normal", res.Cards[1].Priority)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("field_worker role does NOT see admin or other-user cards", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			res, err := s.ListFeedCards(ctx, tx, ListFeedCardsParams{
				OrgID:             orgID,
				CallerOIDCSubject: userB.String(),
				CallerRole:        "field_worker",
			})
			if err != nil {
				return err
			}
			// userB has 1 user-targeted active card. The field_worker
			// role-targeted card was mutated to actioned in setup, so
			// the default status='active' filter excludes it.
			if res.Total != 1 {
				t.Errorf("Total = %d, want 1", res.Total)
			}
			for _, c := range res.Cards {
				if c.TargetRole != nil && *c.TargetRole == "admin" {
					t.Errorf("filter leak: field_worker saw admin card %s", c.ID)
				}
				if c.TargetUserID != nil && *c.TargetUserID == userA {
					t.Errorf("filter leak: userB saw userA-targeted card %s", c.ID)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("status filter returns dismissed-only", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			res, err := s.ListFeedCards(ctx, tx, ListFeedCardsParams{
				OrgID:             orgID,
				CallerOIDCSubject: userA.String(),
				CallerRole:        "admin",
				StatusFilter:      []string{"dismissed"},
			})
			if err != nil {
				return err
			}
			if res.Total != 1 {
				t.Errorf("Total = %d, want 1", res.Total)
			}
			if len(res.Cards) == 1 && res.Cards[0].Status != "dismissed" {
				t.Errorf("status = %q, want dismissed", res.Cards[0].Status)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("priority filter narrows", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			res, err := s.ListFeedCards(ctx, tx, ListFeedCardsParams{
				OrgID:             orgID,
				CallerOIDCSubject: userA.String(),
				CallerRole:        "admin",
				PriorityFilter:    []string{"critical"},
			})
			if err != nil {
				return err
			}
			if res.Total != 1 {
				t.Errorf("Total = %d, want 1 critical card", res.Total)
			}
			if len(res.Cards) == 1 && res.Cards[0].Priority != "critical" {
				t.Errorf("priority = %q, want critical", res.Cards[0].Priority)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cross-org isolation", func(t *testing.T) {
		// Caller from a different org should see zero — even though
		// they share the same role.
		otherOrg := uuid.New()
		otherUser := uuid.New()
		testdb.SeedOrg(t, pool, otherOrg, "Other Org")
		testdb.SeedUser(t, pool, otherUser, otherOrg)

		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			res, err := s.ListFeedCards(ctx, tx, ListFeedCardsParams{
				OrgID:             otherOrg,
				CallerOIDCSubject: otherUser.String(),
				CallerRole:        "admin",
			})
			if err != nil {
				return err
			}
			if res.Total != 0 {
				t.Errorf("Total = %d, want 0 (cross-org)", res.Total)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

// TestFeedCardsStore_InputGuardsAndNotFound covers the deterministic legs the
// existing happy-path tests skip:
//   - CreateFeedCard's empty-actions normalization (nil Actions → JSONB `null`);
//     every fixture above passes an explicit json.RawMessage(`null`), so the
//     len(actions)==0 branch was unreached.
//   - GetFeedCard's no-rows short-circuit → ErrFeedCardNotFound.
//   - ListFeedCards' three input-guard clamps in one call: Limit>200 → 200,
//     Offset<0 → 0, and a malformed CallerOIDCSubject → uuid.Nil (role-targeted
//     cards still resolve since the subject only gates the target_user_id arm).
func TestFeedCardsStore_InputGuardsAndNotFound(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFeedCardsStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Guardrail Builders")
	roleAdmin := "admin"

	var cardID uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		c, e := s.CreateFeedCard(ctx, tx, CreateFeedCardParams{
			OrgID: orgID, CardType: "alert", Title: "guard", Body: "b",
			Priority: "critical", TargetRole: &roleAdmin, Actions: nil,
		})
		if e != nil {
			return e
		}
		cardID = c.ID
		if string(c.Actions) != "null" {
			t.Errorf("nil Actions normalized to %q, want null", string(c.Actions))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		// No-rows leg → ErrFeedCardNotFound.
		if _, e := s.GetFeedCard(ctx, tx, uuid.New(), orgID); !errors.Is(e, ErrFeedCardNotFound) {
			t.Errorf("GetFeedCard(missing) = %v, want ErrFeedCardNotFound", e)
		}

		// Out-of-range inputs exercise all three clamps in a single query;
		// the role-targeted card still resolves under the uuid.Nil subject.
		res, e := s.ListFeedCards(ctx, tx, ListFeedCardsParams{
			OrgID:             orgID,
			CallerOIDCSubject: "not-a-uuid",
			CallerRole:        "admin",
			Limit:             500,
			Offset:            -5,
		})
		if e != nil {
			return e
		}
		if res.Total != 1 || len(res.Cards) != 1 {
			t.Errorf("list with clamped inputs: Total=%d len=%d, want 1/1", res.Total, len(res.Cards))
		}
		if len(res.Cards) == 1 && res.Cards[0].ID != cardID {
			t.Errorf("listed card = %s, want %s", res.Cards[0].ID, cardID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

func TestFeedCardsStore_DismissAndActionTransitions(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFeedCardsStore()
	ctx := context.Background()

	orgID := uuid.New()
	userA := uuid.New()
	userB := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedUser(t, pool, userA, orgID)
	testdb.SeedUser(t, pool, userB, orgID)

	ids := seedFeedCards(t, pool, ctx, orgID, userA, userB)

	t.Run("dismiss transitions active to dismissed", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			c, err := s.DismissFeedCard(ctx, tx, ids[0], orgID)
			if err != nil {
				return err
			}
			if c.Status != "dismissed" {
				t.Errorf("status = %q, want dismissed", c.Status)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("dismiss is idempotent on already-dismissed", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			c, err := s.DismissFeedCard(ctx, tx, ids[0], orgID)
			if err != nil {
				return err
			}
			if c.Status != "dismissed" {
				t.Errorf("status = %q, want dismissed", c.Status)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("action transitions active to actioned with timestamp", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			c, err := s.ActionFeedCard(ctx, tx, ids[1], orgID)
			if err != nil {
				return err
			}
			if c.Status != "actioned" {
				t.Errorf("status = %q, want actioned", c.Status)
			}
			if c.ActionedAt == nil {
				t.Error("actioned_at should be set")
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("action on already-actioned returns ErrFeedCardNotFound", func(t *testing.T) {
		// The UPDATE filter requires status='active'; an actioned row
		// no longer matches, so the update finds nothing and we get
		// ErrFeedCardNotFound (treated as "no actionable row exists").
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			_, err := s.ActionFeedCard(ctx, tx, ids[1], orgID)
			if !errors.Is(err, ErrFeedCardNotFound) {
				t.Errorf("err = %v, want ErrFeedCardNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cross-org dismiss returns not-found", func(t *testing.T) {
		otherOrg := uuid.New()
		testdb.SeedOrg(t, pool, otherOrg, "Other Org")
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			_, err := s.DismissFeedCard(ctx, tx, ids[2], otherOrg)
			if !errors.Is(err, ErrFeedCardNotFound) {
				t.Errorf("err = %v, want ErrFeedCardNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("get returns the card scoped to org", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			c, err := s.GetFeedCard(ctx, tx, ids[3], orgID)
			if err != nil {
				return err
			}
			if c.ID != ids[3] {
				t.Errorf("id = %s, want %s", c.ID, ids[3])
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
