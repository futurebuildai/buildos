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

// mkProspect is a tx-scoped convenience for the list/analytics tests.
func mkProspect(ctx context.Context, t *testing.T, s *PipelineStore, tx pgx.Tx, orgID uuid.UUID, name string) models.Prospect {
	t.Helper()
	p, err := s.CreateProspect(ctx, tx, CreateProspectParams{OrgID: orgID, Name: name, ClientName: "c"})
	if err != nil {
		t.Fatalf("create prospect %q: %v", name, err)
	}
	return p
}

func TestPipelineStore_ListProspects_FilterAndPaginate(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Ridgeline")

	// 3 prospects; advance one to QUALIFIED so the stage filter has a target.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		a := mkProspect(ctx, t, s, tx, orgID, "Alpha")
		mkProspect(ctx, t, s, tx, orgID, "Bravo")
		mkProspect(ctx, t, s, tx, orgID, "Charlie")
		_, err := s.AdvanceStage(ctx, tx, a.ID, orgID, models.StageQualified)
		return err
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		all, err := s.ListProspects(ctx, tx, ListProspectsParams{OrgID: orgID})
		if err != nil {
			return err
		}
		if all.Total != 3 || len(all.Prospects) != 3 {
			t.Errorf("list all: Total=%d len=%d, want 3/3", all.Total, len(all.Prospects))
		}

		qualified, err := s.ListProspects(ctx, tx, ListProspectsParams{OrgID: orgID, Stage: string(models.StageQualified)})
		if err != nil {
			return err
		}
		if qualified.Total != 1 || len(qualified.Prospects) != 1 {
			t.Errorf("stage filter: Total=%d len=%d, want 1/1", qualified.Total, len(qualified.Prospects))
		}

		page1, err := s.ListProspects(ctx, tx, ListProspectsParams{OrgID: orgID, Page: 1, PerPage: 2})
		if err != nil {
			return err
		}
		if len(page1.Prospects) != 2 || page1.Total != 3 || page1.TotalPages != 2 {
			t.Errorf("paginate: len=%d Total=%d TotalPages=%d, want 2/3/2", len(page1.Prospects), page1.Total, page1.TotalPages)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

func TestPipelineStore_UpdateProspect_PartialAndCrossOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "A")
	testdb.SeedOrg(t, pool, orgB, "B")

	var p models.Prospect
	_ = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		p = mkProspect(ctx, t, s, tx, orgA, "Original")
		return nil
	})

	newName := "Renamed Residence"
	newNotes := "client wants a basement"
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.UpdateProspect(ctx, tx, UpdateProspectParams{
			ProspectID: p.ID, OrgID: orgA,
			Name: &newName, Notes: &newNotes,
		})
		if err != nil {
			return err
		}
		if got.Name != newName {
			t.Errorf("name = %q, want %q", got.Name, newName)
		}
		if got.Notes == nil || *got.Notes != newNotes {
			t.Errorf("notes = %v, want %q", got.Notes, newNotes)
		}
		// pipeline_stage stays LEAD — UpdateProspect never touches it.
		if got.PipelineStage != models.StageLead {
			t.Errorf("stage drifted to %s, want LEAD", got.PipelineStage)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}

	// Cross-org update → ErrNotFound.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, qErr := s.UpdateProspect(ctx, tx, UpdateProspectParams{ProspectID: p.ID, OrgID: orgB, Name: &newName})
		return qErr
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org update = %v, want ErrNotFound", err)
	}
}

func TestPipelineStore_ListEstimatesAndPermits(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Cornerstone")

	var prospect models.Prospect
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		prospect = mkProspect(ctx, t, s, tx, orgID, "Multi")
		for i := 0; i < 2; i++ {
			if _, err := s.CreateEstimate(ctx, tx, CreateEstimateParams{
				ProspectID: prospect.ID, CurrencyCode: "USD",
				LineItemsJSON: []byte("[]"), MarginPct: 15,
			}); err != nil {
				return err
			}
		}
		for _, j := range []string{"Austin TX", "Travis County"} {
			if _, err := s.CreatePermit(ctx, tx, CreatePermitParams{
				ProspectID: prospect.ID, PermitType: "building",
				Jurisdiction: j, FeeCents: 50000, FeeCurrencyCode: "USD",
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		ests, err := s.ListEstimatesForProspect(ctx, tx, prospect.ID)
		if err != nil {
			return err
		}
		if len(ests) != 2 {
			t.Errorf("estimates = %d, want 2", len(ests))
		}
		// newest version first.
		if len(ests) == 2 && ests[0].Version < ests[1].Version {
			t.Errorf("estimates not newest-first: %d then %d", ests[0].Version, ests[1].Version)
		}

		permits, err := s.ListPermitsForProspect(ctx, tx, prospect.ID)
		if err != nil {
			return err
		}
		if len(permits) != 2 {
			t.Errorf("permits = %d, want 2", len(permits))
		}

		// GetPermit scoped by (id, prospect): match, then wrong prospect → ErrNotFound.
		got, err := s.GetPermit(ctx, tx, permits[0].ID, prospect.ID)
		if err != nil {
			return err
		}
		if got.ID != permits[0].ID {
			t.Errorf("GetPermit id mismatch")
		}
		_, err = s.GetPermit(ctx, tx, permits[0].ID, uuid.New())
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("GetPermit(wrong prospect) = %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}

func TestPipelineStore_MarkProspectPermitIssued_LinksProject(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "A")
	testdb.SeedOrg(t, pool, orgB, "B")

	addr := "9 Oak Ln"
	gsf := 3200
	var prospect models.Prospect
	var projectID uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var qErr error
		prospect, qErr = s.CreateProspect(ctx, tx, CreateProspectParams{
			OrgID: orgA, Name: "Permit House", ClientName: "Jane", Address: &addr, GSF: &gsf,
		})
		if qErr != nil {
			return qErr
		}
		projectID, qErr = s.CreateProjectFromProspect(ctx, tx, CreateProjectFromProspectParams{
			OrgID: orgA, Name: prospect.Name, Address: prospect.Address,
			GSF: gsf, PermitIssuedDate: mustParseDate("2026-04-15"),
		})
		return qErr
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.MarkProspectPermitIssued(ctx, tx, prospect.ID, orgA, projectID)
		if err != nil {
			return err
		}
		if got.PipelineStage != models.StagePermitIssued {
			t.Errorf("stage = %s, want PERMIT_ISSUED", got.PipelineStage)
		}
		if got.ProbabilityPct != 100 {
			t.Errorf("probability = %d, want 100", got.ProbabilityPct)
		}
		if got.ProjectID == nil || *got.ProjectID != projectID {
			t.Errorf("project_id = %v, want %v", got.ProjectID, projectID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("mark permit issued: %v", err)
	}

	// Cross-org → ErrNotFound.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		_, qErr := s.MarkProspectPermitIssued(ctx, tx, prospect.ID, orgB, projectID)
		return qErr
	})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org mark = %v, want ErrNotFound", err)
	}
}

// TestPipelineStore_NotFoundPaths exercises the ErrNotFound (no-rows) leg of
// every pipeline getter/mutator whose happy path is covered elsewhere but whose
// not-found short-circuit was unreached: GetProspect, AdvanceStage, MarkLost,
// GetEstimate, UpdateEstimateStatus, UpdatePermit. Each is driven with a random
// non-existent id so the QueryRow returns pgx.ErrNoRows → ErrNotFound. (The
// UPDATE...RETURNING mutators return no rows when the WHERE matches nothing, so
// they map to ErrNotFound too — proving the optimistic "row must exist" contract.)
func TestPipelineStore_NotFoundPaths(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Ghost")

	missing := uuid.New() // never inserted

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, e := s.GetProspect(ctx, tx, missing, orgID); !errors.Is(e, ErrNotFound) {
			t.Errorf("GetProspect(missing) = %v, want ErrNotFound", e)
		}
		if _, e := s.AdvanceStage(ctx, tx, missing, orgID, models.StageQualified); !errors.Is(e, ErrNotFound) {
			t.Errorf("AdvanceStage(missing) = %v, want ErrNotFound", e)
		}
		if _, e := s.MarkLost(ctx, tx, missing, orgID, "n/a"); !errors.Is(e, ErrNotFound) {
			t.Errorf("MarkLost(missing) = %v, want ErrNotFound", e)
		}
		if _, e := s.GetEstimate(ctx, tx, missing, missing); !errors.Is(e, ErrNotFound) {
			t.Errorf("GetEstimate(missing) = %v, want ErrNotFound", e)
		}
		if _, e := s.UpdateEstimateStatus(ctx, tx, missing, "sent"); !errors.Is(e, ErrNotFound) {
			t.Errorf("UpdateEstimateStatus(missing) = %v, want ErrNotFound", e)
		}
		if _, e := s.UpdatePermit(ctx, tx, UpdatePermitParams{PermitID: missing}); !errors.Is(e, ErrNotFound) {
			t.Errorf("UpdatePermit(missing) = %v, want ErrNotFound", e)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("not-found tx: %v", err)
	}
}

func TestPipelineStore_ListPipelineAnalytics_WeightsAndExcludes(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewPipelineStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Vantage")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// USD prospect, advanced to QUALIFIED (probability 25), with an estimate.
		usd := mkProspect(ctx, t, s, tx, orgID, "USD Deal")
		if _, err := s.AdvanceStage(ctx, tx, usd.ID, orgID, models.StageQualified); err != nil {
			return err
		}
		if _, err := s.CreateEstimate(ctx, tx, CreateEstimateParams{
			ProspectID: usd.ID, TotalEstimatedCents: 1_000_000, CurrencyCode: "USD",
			LineItemsJSON: []byte("[]"), MarginPct: 15,
		}); err != nil {
			return err
		}
		// CAD prospect with an estimate (stays LEAD, probability 10).
		cad := mkProspect(ctx, t, s, tx, orgID, "CAD Deal")
		if _, err := s.CreateEstimate(ctx, tx, CreateEstimateParams{
			ProspectID: cad.ID, TotalEstimatedCents: 500_000, CurrencyCode: "CAD",
			LineItemsJSON: []byte("[]"), MarginPct: 15,
		}); err != nil {
			return err
		}
		// LOST prospect with an estimate → excluded.
		lost := mkProspect(ctx, t, s, tx, orgID, "Lost Deal")
		if _, err := s.CreateEstimate(ctx, tx, CreateEstimateParams{
			ProspectID: lost.ID, TotalEstimatedCents: 9_000_000, CurrencyCode: "USD",
			LineItemsJSON: []byte("[]"), MarginPct: 15,
		}); err != nil {
			return err
		}
		if _, err := s.MarkLost(ctx, tx, lost.ID, orgID, "gone"); err != nil {
			return err
		}
		// Estimate-less prospect → excluded (nothing to weight).
		mkProspect(ctx, t, s, tx, orgID, "No Estimate")
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		rows, err := s.ListPipelineAnalytics(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("analytics rows = %d, want 2 (USD + CAD; LOST + estimate-less excluded)", len(rows))
		}
		byCur := map[string]models.PipelineAnalyticsRow{}
		for _, r := range rows {
			byCur[r.CurrencyCode] = r
		}
		usd, ok := byCur["USD"]
		if !ok {
			t.Fatalf("USD bucket missing (LOST 9M must not leak in)")
		}
		// Only the QUALIFIED prospect (1M) counts; LOST excluded.
		if usd.TotalEstimatedCents != 1_000_000 {
			t.Errorf("USD total = %d, want 1000000 (LOST excluded)", usd.TotalEstimatedCents)
		}
		// weighted = 1_000_000 * 25 / 100 = 250_000.
		if usd.WeightedRevenueCents != 250_000 {
			t.Errorf("USD weighted = %d, want 250000", usd.WeightedRevenueCents)
		}
		if usd.ProspectCount != 1 {
			t.Errorf("USD prospect_count = %d, want 1", usd.ProspectCount)
		}
		cad := byCur["CAD"]
		// weighted = 500_000 * 10 / 100 = 50_000.
		if cad.WeightedRevenueCents != 50_000 {
			t.Errorf("CAD weighted = %d, want 50000", cad.WeightedRevenueCents)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read tx: %v", err)
	}
}
