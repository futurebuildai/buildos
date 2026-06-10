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

func TestFeedbackStore_SubmitListTriageRoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFeedbackStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Feedback Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, err := s.Insert(ctx, tx, InsertFeedbackParams{
			OrgID:    orgID,
			UserSub:  "grant-sub",
			Category: "friction",
			Message:  "the wizard's holiday picker needs a year view",
			Context:  []byte(`{"route":"/setup/calendar","role":"owner"}`),
		})
		if err != nil {
			return err
		}
		if row.Status != "new" || row.Category != "friction" || row.UserSub != "grant-sub" {
			t.Errorf("inserted row = %+v", row)
		}

		// List unfiltered + filtered (paginated).
		all, err := s.ListByOrg(ctx, tx, orgID, "", 1, 100)
		if err != nil {
			return err
		}
		if len(all.Feedback) != 1 || all.Feedback[0].ID != row.ID || all.Total != 1 {
			t.Errorf("list all = %+v", all)
		}
		newOnly, err := s.ListByOrg(ctx, tx, orgID, "new", 1, 100)
		if err != nil {
			return err
		}
		if len(newOnly.Feedback) != 1 {
			t.Errorf("list status=new = %+v", newOnly)
		}

		// Triage: status moves, note set; then a second pass with a nil
		// note must KEEP the existing note.
		note := "scheduled for the onboarding polish pass"
		triaged, err := s.UpdateStatus(ctx, tx, orgID, row.ID, "planned", &note)
		if err != nil {
			return err
		}
		if triaged.Status != "planned" || triaged.TriageNote != note {
			t.Errorf("triaged row = %+v", triaged)
		}
		again, err := s.UpdateStatus(ctx, tx, orgID, row.ID, "shipped", nil)
		if err != nil {
			return err
		}
		if again.Status != "shipped" || again.TriageNote != note {
			t.Errorf("nil note must keep the existing note: %+v", again)
		}

		// The filter no longer matches "new".
		newOnly, err = s.ListByOrg(ctx, tx, orgID, "new", 1, 100)
		if err != nil {
			return err
		}
		if len(newOnly.Feedback) != 0 {
			t.Errorf("status=new after triage = %+v, want empty", newOnly)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestFeedbackStore_OrgScoping(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFeedbackStore()
	ctx := context.Background()

	orgA, orgB := uuid.New(), uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, err := s.Insert(ctx, tx, InsertFeedbackParams{
			OrgID: orgA, UserSub: "a-sub", Category: "bug", Message: "only A sees this",
		})
		if err != nil {
			return err
		}

		// Org B sees nothing.
		bRows, err := s.ListByOrg(ctx, tx, orgB, "", 1, 100)
		if err != nil {
			return err
		}
		if len(bRows.Feedback) != 0 || bRows.Total != 0 {
			t.Errorf("org B list = %+v, want empty (cross-org leak)", bRows)
		}

		// Org B cannot triage A's row — ErrNotFound, row untouched.
		if _, err := s.UpdateStatus(ctx, tx, orgB, row.ID, "declined", nil); !errors.Is(err, ErrNotFound) {
			t.Errorf("cross-org UpdateStatus err = %v, want ErrNotFound", err)
		}
		got, err := s.ListByOrg(ctx, tx, orgA, "new", 1, 100)
		if err != nil {
			return err
		}
		if len(got.Feedback) != 1 {
			t.Errorf("org A row must remain status=new after the foreign triage attempt: %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}

func TestFeedbackStore_CheckConstraintsEnforced(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFeedbackStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Check Co")

	// Each bad write in its own tx — a CHECK violation poisons the tx.
	badInsert := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.Insert(ctx, tx, InsertFeedbackParams{
			OrgID: orgID, Category: "rant", Message: "x",
		})
		return err
	})
	if badInsert == nil {
		t.Error("category outside the CHECK list must be rejected by the DB")
	}

	badStatus := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, err := s.Insert(ctx, tx, InsertFeedbackParams{
			OrgID: orgID, Category: "bug", Message: "ok",
		})
		if err != nil {
			return err
		}
		_, err = s.UpdateStatus(ctx, tx, orgID, row.ID, "someday", nil)
		return err
	})
	if badStatus == nil {
		t.Error("status outside the CHECK list must be rejected by the DB")
	}
}

func TestFeedbackStore_PaginationDrainsBacklog(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewFeedbackStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Page Co")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		seen := map[uuid.UUID]bool{}
		for i := 0; i < 5; i++ {
			row, err := s.Insert(ctx, tx, InsertFeedbackParams{
				OrgID: orgID, UserSub: "u", Category: "idea", Message: "report",
			})
			if err != nil {
				return err
			}
			seen[row.ID] = false
		}

		// Page through with per_page=2: 3 pages, totals consistent,
		// every row reachable exactly once — the harvest poller's
		// no-silent-truncation contract.
		for page := 1; page <= 3; page++ {
			p, err := s.ListByOrg(ctx, tx, orgID, "", page, 2)
			if err != nil {
				return err
			}
			if p.Total != 5 || p.TotalPages != 3 {
				t.Errorf("page %d meta = total %d pages %d, want 5/3", page, p.Total, p.TotalPages)
			}
			for _, f := range p.Feedback {
				if dup, ok := seen[f.ID]; !ok || dup {
					t.Errorf("row %s missing or duplicated across pages", f.ID)
				}
				seen[f.ID] = true
			}
		}
		for id, found := range seen {
			if !found {
				t.Errorf("row %s unreachable through pagination", id)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
}
