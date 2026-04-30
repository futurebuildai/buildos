//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/testdb"
)

func mustParseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestPipelineStore_CreateProspect_RoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	gsf := 3200
	email := "client@example.com"

	var p models.Prospect
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		p, qErr = s.CreateProspect(ctx, tx, CreateProspectParams{
			OrgID:       orgID,
			Name:        "Smith Residence",
			ClientName:  "Jane Smith",
			ClientEmail: &email,
			GSF:         &gsf,
		})
		return qErr
	})
	if err != nil {
		t.Fatalf("create prospect: %v", err)
	}
	if p.PipelineStage != models.StageLead {
		t.Errorf("new prospect stage = %s, want LEAD", p.PipelineStage)
	}
	if p.ProbabilityPct != 10 {
		t.Errorf("new prospect probability_pct = %d, want 10", p.ProbabilityPct)
	}

	// Round-trip via Get.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		got, err := s.GetProspect(ctx, tx, p.ID, orgID)
		if err != nil {
			return err
		}
		if got.Name != "Smith Residence" || got.ClientName != "Jane Smith" {
			t.Errorf("round-trip mismatch: %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("get prospect: %v", err)
	}
}

func TestPipelineStore_AdvanceStage_UpdatesProbability(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	var p models.Prospect
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		p, qErr = s.CreateProspect(ctx, tx, CreateProspectParams{
			OrgID: orgID, Name: "p", ClientName: "c",
		})
		return qErr
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Advance LEAD → QUALIFIED; probability should jump from 10 → 25.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.AdvanceStage(ctx, tx, p.ID, orgID, models.StageQualified)
		if err != nil {
			return err
		}
		if got.PipelineStage != models.StageQualified {
			t.Errorf("advanced stage = %s, want QUALIFIED", got.PipelineStage)
		}
		if got.ProbabilityPct != 25 {
			t.Errorf("advanced probability = %d, want 25", got.ProbabilityPct)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
}

func TestPipelineStore_MarkLost_SetsReason(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	var p models.Prospect
	_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		p, _ = s.CreateProspect(ctx, tx, CreateProspectParams{OrgID: orgID, Name: "p", ClientName: "c"})
		return nil
	})

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.MarkLost(ctx, tx, p.ID, orgID, "client picked another GC")
		if err != nil {
			return err
		}
		if got.PipelineStage != models.StageLost {
			t.Errorf("stage = %s, want LOST", got.PipelineStage)
		}
		if got.ProbabilityPct != 0 {
			t.Errorf("probability_pct = %d, want 0", got.ProbabilityPct)
		}
		if got.LostReason == nil || *got.LostReason != "client picked another GC" {
			t.Errorf("lost_reason = %v", got.LostReason)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("mark lost: %v", err)
	}
}

func TestPipelineStore_CreateEstimate_AutoVersions(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	var p models.Prospect
	_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		p, _ = s.CreateProspect(ctx, tx, CreateProspectParams{OrgID: orgID, Name: "p", ClientName: "c"})
		return nil
	})

	// Insert 3 estimates → versions 1, 2, 3.
	versions := make([]int, 0, 3)
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for i := 0; i < 3; i++ {
			e, err := s.CreateEstimate(ctx, tx, CreateEstimateParams{
				ProspectID:          p.ID,
				TotalEstimatedCents: int64(1000_00 * (i + 1)),
				CurrencyCode:        "USD",
				LineItemsJSON:       []byte("[]"),
				MarginPct:           15,
			})
			if err != nil {
				return err
			}
			versions = append(versions, e.Version)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create estimates: %v", err)
	}
	for i, v := range versions {
		if v != i+1 {
			t.Errorf("estimate[%d].version = %d, want %d", i, v, i+1)
		}
	}
}

func TestPipelineStore_UpdateEstimateStatus_StampsSentAt(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	var prospect models.Prospect
	var est models.PipelineEstimate
	_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		prospect, _ = s.CreateProspect(ctx, tx, CreateProspectParams{OrgID: orgID, Name: "p", ClientName: "c"})
		est, _ = s.CreateEstimate(ctx, tx, CreateEstimateParams{
			ProspectID:    prospect.ID,
			CurrencyCode:  "USD",
			LineItemsJSON: []byte("[]"),
			MarginPct:     15,
		})
		return nil
	})
	if est.SentAt != nil {
		t.Fatalf("new estimate should have nil sent_at; got %v", est.SentAt)
	}

	// First transition to "sent" stamps sent_at.
	var sentOnce models.PipelineEstimate
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		sentOnce, qErr = s.UpdateEstimateStatus(ctx, tx, est.ID, "sent")
		return qErr
	})
	if err != nil {
		t.Fatalf("first sent: %v", err)
	}
	if sentOnce.SentAt == nil {
		t.Error("first sent should stamp sent_at")
	}

	// Re-send (sent → revised → sent) should preserve the original stamp.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, err := s.UpdateEstimateStatus(ctx, tx, est.ID, "revised")
		if err != nil {
			return err
		}
		_, err = s.UpdateEstimateStatus(ctx, tx, est.ID, "sent")
		return err
	})
	if err != nil {
		t.Fatalf("revise + resend: %v", err)
	}
	var resent models.PipelineEstimate
	_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		resent, _ = s.GetEstimate(ctx, tx, est.ID, prospect.ID)
		return nil
	})
	if resent.SentAt == nil || !resent.SentAt.Equal(*sentOnce.SentAt) {
		t.Errorf("re-sent should preserve original sent_at: got %v, want %v",
			resent.SentAt, sentOnce.SentAt)
	}
}

func TestPipelineStore_PermitCRUD(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	var prospect models.Prospect
	_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		prospect, _ = s.CreateProspect(ctx, tx, CreateProspectParams{OrgID: orgID, Name: "p", ClientName: "c"})
		return nil
	})

	var permit models.Permit
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		permit, qErr = s.CreatePermit(ctx, tx, CreatePermitParams{
			ProspectID:      prospect.ID,
			PermitType:      "building",
			Jurisdiction:    "Austin TX",
			FeeCents:        50000,
			FeeCurrencyCode: "USD",
		})
		return qErr
	})
	if err != nil {
		t.Fatalf("create permit: %v", err)
	}
	if permit.Status != "not_submitted" {
		t.Errorf("default status = %s, want not_submitted", permit.Status)
	}

	// Update — change status to submitted, set application_number.
	appNo := "BLDG-2026-0042"
	newStatus := "submitted"
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		updated, err := s.UpdatePermit(ctx, tx, UpdatePermitParams{
			PermitID:          permit.ID,
			ApplicationNumber: &appNo,
			Status:            &newStatus,
		})
		if err != nil {
			return err
		}
		if updated.Status != "submitted" {
			t.Errorf("updated status = %s, want submitted", updated.Status)
		}
		if updated.ApplicationNumber == nil || *updated.ApplicationNumber != appNo {
			t.Errorf("application_number = %v, want %s", updated.ApplicationNumber, appNo)
		}
		// fee_currency_code should be untouched (immutable post-create per design).
		if updated.FeeCurrencyCode != "USD" {
			t.Errorf("fee_currency_code mutated: %s", updated.FeeCurrencyCode)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("update permit: %v", err)
	}
}

func TestPipelineStore_CreateProjectFromProspect(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	addr := "123 Main St"
	gsf := 3200
	var prospect models.Prospect
	_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		prospect, _ = s.CreateProspect(ctx, tx, CreateProspectParams{
			OrgID: orgID, Name: "Smith House", ClientName: "Jane",
			Address: &addr, GSF: &gsf,
		})
		return nil
	})

	permitDate := mustParseDate("2026-04-15")
	var projectID uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		projectID, qErr = s.CreateProjectFromProspect(ctx, tx, CreateProjectFromProspectParams{
			OrgID:            orgID,
			Name:             prospect.Name,
			Address:          prospect.Address,
			GSF:              *prospect.GSF,
			PermitIssuedDate: permitDate,
		})
		return qErr
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if projectID == uuid.Nil {
		t.Fatal("expected non-nil project id")
	}

	// Verify project actually exists with the given org.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		return VerifyProjectInOrg(ctx, tx, projectID, orgID)
	})
	if err != nil {
		t.Errorf("project should be in org: %v", err)
	}
}

func TestPipelineStore_VerifyProspectInOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "A")
	testdb.SeedOrg(t, pool, orgB, "B")

	var p models.Prospect
	_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		p, _ = s.CreateProspect(ctx, tx, CreateProspectParams{OrgID: orgA, Name: "p", ClientName: "c"})
		return nil
	})

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		return s.VerifyProspectInOrg(ctx, tx, p.ID, orgA)
	})
	if err != nil {
		t.Errorf("same-org verify: %v", err)
	}

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		return s.VerifyProspectInOrg(ctx, tx, p.ID, orgB)
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org verify = %v, want ErrNotFound", err)
	}
}
