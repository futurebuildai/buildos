//go:build integration

package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// seedFeedCard inserts one feed card via the store and returns its id.
func seedFeedCard(t *testing.T, pool *pgxpool.Pool, p store.CreateFeedCardParams) uuid.UUID {
	t.Helper()
	cards := store.NewFeedCardsStore()
	var id uuid.UUID
	err := pgx.BeginTxFunc(context.Background(), pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		c, err := cards.CreateFeedCard(context.Background(), tx, p)
		if err != nil {
			return err
		}
		id = c.ID
		return nil
	})
	if err != nil {
		t.Fatalf("seed feed card: %v", err)
	}
	return id
}

// newFeedServiceFixture wires a FeedService over a fresh pool with a
// discard logger + a capturing audit recorder, plus a seeded org+user.
func newFeedServiceFixture(t *testing.T) (*FeedService, *capturingAuditRecorder, *pgxpool.Pool, uuid.UUID, string) {
	t.Helper()
	pool := testdb.NewPool(t)
	rec := &capturingAuditRecorder{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewFeedService(pool, store.NewFeedCardsStore(), logger, rec)

	orgID := uuid.New()
	userID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedUser(t, pool, userID, orgID)
	return svc, rec, pool, orgID, userID.String()
}

// TestFeedService_ListFeed covers the read-only paging query (happy
// path + pagination defaults) and the four input-validation guards.
func TestFeedService_ListFeed(t *testing.T) {
	svc, _, pool, orgID, subject := newFeedServiceFixture(t)
	ctx := context.Background()

	uid := uuid.MustParse(subject)
	seedFeedCard(t, pool, store.CreateFeedCardParams{
		OrgID: orgID, CardType: "info", Title: "for the caller", Body: "b",
		Priority: "normal", TargetUserID: &uid, Actions: json.RawMessage(`null`),
	})

	t.Run("returns the caller's card with pagination defaults applied", func(t *testing.T) {
		// Page/PerPage left zero → defaulted to 1/50 inside the service.
		res, err := svc.ListFeed(ctx, FeedListOptions{
			CallerOrgID:       orgID,
			CallerOIDCSubject: subject,
			CallerRole:        "field_worker",
		})
		if err != nil {
			t.Fatalf("ListFeed: %v", err)
		}
		if res.Total != 1 || len(res.Cards) != 1 {
			t.Fatalf("res = %+v, want exactly 1 card", res)
		}
	})

	t.Run("oversized perPage is clamped to 200", func(t *testing.T) {
		// PerPage > 200 exercises the upper-bound clamp (the default-zero
		// and <=0 legs are covered by the pagination-defaults case above).
		// The call must still succeed and return the caller's card; the
		// clamp itself is the branch under test.
		res, err := svc.ListFeed(ctx, FeedListOptions{
			CallerOrgID:       orgID,
			CallerOIDCSubject: subject,
			CallerRole:        "field_worker",
			PerPage:           5000,
		})
		if err != nil {
			t.Fatalf("ListFeed(perPage=5000): %v", err)
		}
		if res.Total != 1 || len(res.Cards) != 1 {
			t.Fatalf("res = %+v, want exactly 1 card after clamp", res)
		}
	})

	t.Run("validation guards", func(t *testing.T) {
		base := FeedListOptions{CallerOrgID: orgID, CallerOIDCSubject: subject, CallerRole: "field_worker"}
		cases := map[string]FeedListOptions{
			"nil org":       {CallerOIDCSubject: subject, CallerRole: "field_worker"},
			"empty subject": {CallerOrgID: orgID, CallerRole: "field_worker"},
			"empty role":    {CallerOrgID: orgID, CallerOIDCSubject: subject},
			"bad status":    withStatus(base, "nope"),
			"bad priority":  withPriority(base, "nope"),
		}
		for name, opts := range cases {
			if _, err := svc.ListFeed(ctx, opts); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("%s: err = %v, want ErrInvalidInput", name, err)
			}
		}
	})
}

func withStatus(o FeedListOptions, s string) FeedListOptions {
	o.StatusFilter = []string{s}
	return o
}

func withPriority(o FeedListOptions, p string) FeedListOptions {
	o.PriorityFilter = []string{p}
	return o
}

// TestFeedService_DismissCard covers the dismiss tx + audit, the
// not-found / cross-org leg, and the input guard.
func TestFeedService_DismissCard(t *testing.T) {
	svc, rec, pool, orgID, subject := newFeedServiceFixture(t)
	ctx := context.Background()

	uid := uuid.MustParse(subject)
	cardID := seedFeedCard(t, pool, store.CreateFeedCardParams{
		OrgID: orgID, CardType: "info", Title: "dismiss me", Body: "b",
		Priority: "normal", TargetUserID: &uid, Actions: json.RawMessage(`null`),
	})

	t.Run("transitions to dismissed and audits", func(t *testing.T) {
		out, err := svc.DismissCard(ctx, orgID, subject, cardID)
		if err != nil {
			t.Fatalf("DismissCard: %v", err)
		}
		if out.Status != "dismissed" {
			t.Errorf("status = %q, want dismissed", out.Status)
		}
		if len(rec.entries) == 0 || rec.entries[len(rec.entries)-1].Action != "feed.card.dismissed" {
			t.Errorf("audit not recorded: %+v", rec.entries)
		}
	})

	t.Run("unknown card surfaces ErrFeedCardNotFound", func(t *testing.T) {
		if _, err := svc.DismissCard(ctx, orgID, subject, uuid.New()); !errors.Is(err, ErrFeedCardNotFound) {
			t.Fatalf("err = %v, want ErrFeedCardNotFound", err)
		}
	})

	t.Run("nil card id is rejected", func(t *testing.T) {
		if _, err := svc.DismissCard(ctx, orgID, subject, uuid.Nil); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})
}

// TestFeedService_ActionCard covers the action tx + audit + post-commit
// log line, the not-found leg, and the two input guards.
func TestFeedService_ActionCard(t *testing.T) {
	svc, rec, pool, orgID, subject := newFeedServiceFixture(t)
	ctx := context.Background()

	uid := uuid.MustParse(subject)
	cardID := seedFeedCard(t, pool, store.CreateFeedCardParams{
		OrgID: orgID, CardType: "alert", Title: "action me", Body: "b",
		Priority: "urgent", TargetUserID: &uid, Actions: json.RawMessage(`null`),
	})

	t.Run("transitions to actioned and audits the payload", func(t *testing.T) {
		out, err := svc.ActionCard(ctx, orgID, subject, cardID, FeedActionInput{
			ActionType: "approve",
			Payload:    json.RawMessage(`{"ok":true}`),
		})
		if err != nil {
			t.Fatalf("ActionCard: %v", err)
		}
		if out.Status != "actioned" {
			t.Errorf("status = %q, want actioned", out.Status)
		}
		if len(rec.entries) == 0 || rec.entries[len(rec.entries)-1].Action != "feed.card.actioned" {
			t.Errorf("audit not recorded: %+v", rec.entries)
		}
	})

	t.Run("unknown card surfaces ErrFeedCardNotFound", func(t *testing.T) {
		_, err := svc.ActionCard(ctx, orgID, subject, uuid.New(), FeedActionInput{ActionType: "approve"})
		if !errors.Is(err, ErrFeedCardNotFound) {
			t.Fatalf("err = %v, want ErrFeedCardNotFound", err)
		}
	})

	t.Run("input guards", func(t *testing.T) {
		// Empty action_type.
		if _, err := svc.ActionCard(ctx, orgID, subject, cardID, FeedActionInput{}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("empty action_type: err = %v, want ErrInvalidInput", err)
		}
		// Oversized payload.
		big := json.RawMessage(`"` + strings.Repeat("x", MaxFeedActionPayloadBytes+1) + `"`)
		if _, err := svc.ActionCard(ctx, orgID, subject, cardID, FeedActionInput{ActionType: "approve", Payload: big}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("oversized payload: err = %v, want ErrInvalidInput", err)
		}
	})
}
