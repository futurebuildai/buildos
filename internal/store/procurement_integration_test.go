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

func TestProcurementStore_Create_ComputesMustOrderDate(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProcurementStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	// need_by - 14 (lead) - 7 (buffer) = need_by - 21 days
	needBy := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	wantMustOrder := needBy.AddDate(0, 0, -21)

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.CreateProcurementItem(ctx, tx, CreateProcurementItemParams{
			ProjectID:                 projectID,
			OrgID:                     orgID,
			Name:                      "framing lumber",
			WBSCode:                   "06.10.10",
			EstimatedCostCents:        450000,
			EstimatedCostCurrencyCode: "USD",
			LeadTimeDays:              14,
			WeatherBufferDays:         7,
			NeedByDate:                &needBy,
		})
		if err != nil {
			return err
		}
		if got.Status != models.ProcurementStatusOK {
			t.Errorf("default status = %q, want OK", got.Status)
		}
		if got.MustOrderDate == nil {
			t.Fatal("must_order_date should be computed when need_by_date is set")
		}
		if !got.MustOrderDate.Equal(wantMustOrder) {
			t.Errorf("must_order_date = %s, want %s", got.MustOrderDate, wantMustOrder)
		}
		if got.EstimatedCostCents != 450000 {
			t.Errorf("estimated_cost_cents = %d", got.EstimatedCostCents)
		}
		if got.EstimatedCostCurrencyCode != "USD" {
			t.Errorf("currency = %q", got.EstimatedCostCurrencyCode)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
}

func TestProcurementStore_Create_WithoutNeedBy_LeavesMustOrderNull(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProcurementStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.CreateProcurementItem(ctx, tx, CreateProcurementItemParams{
			ProjectID:                 projectID,
			OrgID:                     orgID,
			Name:                      "rough plumbing",
			WBSCode:                   "22.10.10",
			EstimatedCostCents:        85000,
			EstimatedCostCurrencyCode: "USD",
			LeadTimeDays:              7,
			WeatherBufferDays:         3,
			// NeedByDate intentionally nil.
		})
		if err != nil {
			return err
		}
		if got.MustOrderDate != nil {
			t.Errorf("must_order_date = %v, want nil when need_by_date is nil", got.MustOrderDate)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
}

func TestProcurementStore_List_OrderingAndStatusFilter(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProcurementStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	// Seed: OK, WARNING, CRITICAL, ORDERED — one of each.
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, name := range []string{"item-A", "item-B", "item-C", "item-D"} {
			_, err := s.CreateProcurementItem(ctx, tx, CreateProcurementItemParams{
				ProjectID:                 projectID,
				OrgID:                     orgID,
				Name:                      name,
				WBSCode:                   "01.00.00",
				EstimatedCostCents:        1000,
				EstimatedCostCurrencyCode: "USD",
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Override statuses post-insert. (CreateProcurementItem doesn't take
	// a status param; default is OK.)
	rows, err := pool.Query(ctx, `SELECT id FROM procurement_items WHERE project_id = $1 ORDER BY created_at ASC`, projectID)
	if err != nil {
		t.Fatalf("list ids: %v", err)
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan: %v", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if len(ids) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(ids))
	}
	statuses := []string{"WARNING", "CRITICAL", "ORDERED", "OK"}
	for i, id := range ids {
		if _, err := pool.Exec(ctx, `UPDATE procurement_items SET status = $1 WHERE id = $2`, statuses[i], id); err != nil {
			t.Fatalf("set status: %v", err)
		}
	}

	t.Run("no filter returns all in critical-first order", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			items, err := s.ListProcurementItems(ctx, tx, ListProcurementItemsParams{
				ProjectID: projectID, OrgID: orgID,
			})
			if err != nil {
				return err
			}
			if len(items) != 4 {
				t.Fatalf("got %d items, want 4", len(items))
			}
			wantOrder := []string{"CRITICAL", "WARNING", "OK", "ORDERED"}
			for i, w := range wantOrder {
				if items[i].Status != w {
					t.Errorf("position %d: status = %q, want %q", i, items[i].Status, w)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("status filter narrows", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			items, err := s.ListProcurementItems(ctx, tx, ListProcurementItemsParams{
				ProjectID:    projectID,
				OrgID:        orgID,
				StatusFilter: []string{"WARNING", "CRITICAL"},
			})
			if err != nil {
				return err
			}
			if len(items) != 2 {
				t.Errorf("got %d items, want 2 (WARNING + CRITICAL)", len(items))
			}
			for _, it := range items {
				if it.Status != "WARNING" && it.Status != "CRITICAL" {
					t.Errorf("filter leak: status = %q", it.Status)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cross-org isolation returns zero", func(t *testing.T) {
		otherOrg := uuid.New()
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			items, err := s.ListProcurementItems(ctx, tx, ListProcurementItemsParams{
				ProjectID: projectID, OrgID: otherOrg,
			})
			if err != nil {
				return err
			}
			if len(items) != 0 {
				t.Errorf("got %d items, want 0 (cross-org)", len(items))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestProcurementStore_UpdateAndStatusChangedAt(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProcurementStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	var item models.ProcurementItem
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.CreateProcurementItem(ctx, tx, CreateProcurementItemParams{
			ProjectID:                 projectID,
			OrgID:                     orgID,
			Name:                      "windows",
			WBSCode:                   "08.50.00",
			EstimatedCostCents:        200000,
			EstimatedCostCurrencyCode: "USD",
		})
		if err != nil {
			return err
		}
		item = got
		return nil
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	originalChange := item.StatusChangedAt

	// Sleep enough that now() advances visibly under the second.
	time.Sleep(50 * time.Millisecond)

	t.Run("status change bumps status_changed_at", func(t *testing.T) {
		ordered := models.ProcurementStatusOrdered
		po := "PO-12345"
		now := time.Now().UTC()
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			got, err := s.UpdateProcurementItem(ctx, tx, UpdateProcurementItemParams{
				ItemID:    item.ID,
				ProjectID: projectID,
				OrgID:     orgID,
				Status:    &ordered,
				PONumber:  &po,
				OrderedAt: &now,
			})
			if err != nil {
				return err
			}
			if got.Status != "ORDERED" {
				t.Errorf("status = %q, want ORDERED", got.Status)
			}
			if got.PONumber == nil || *got.PONumber != po {
				t.Errorf("po_number = %v, want %s", got.PONumber, po)
			}
			if !got.StatusChangedAt.After(originalChange) {
				t.Errorf("status_changed_at not advanced: %s !> %s", got.StatusChangedAt, originalChange)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("update on non-existent row returns not-found", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			ordered := "OK"
			_, err := s.UpdateProcurementItem(ctx, tx, UpdateProcurementItemParams{
				ItemID:    uuid.New(),
				ProjectID: projectID,
				OrgID:     orgID,
				Status:    &ordered,
			})
			if !errors.Is(err, ErrProcurementItemNotFound) {
				t.Errorf("err = %v, want ErrProcurementItemNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cross-org update returns not-found", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
			otherOrg := uuid.New()
			ok := "OK"
			_, err := s.UpdateProcurementItem(ctx, tx, UpdateProcurementItemParams{
				ItemID:    item.ID,
				ProjectID: projectID,
				OrgID:     otherOrg,
				Status:    &ok,
			})
			if !errors.Is(err, ErrProcurementItemNotFound) {
				t.Errorf("err = %v, want ErrProcurementItemNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

// TestProcurementStore_GetProcurementItem covers the single-row fetch
// used by RecommendVendors to confirm ownership + pull budget context:
// a matching (id, org) returns the row; an org mismatch or unknown id
// both collapse to ErrProcurementItemNotFound (no row-existence leak).
func TestProcurementStore_GetProcurementItem(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProcurementStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	var item models.ProcurementItem
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.CreateProcurementItem(ctx, tx, CreateProcurementItemParams{
			ProjectID:                 projectID,
			OrgID:                     orgID,
			Name:                      "roof trusses",
			WBSCode:                   "06.17.00",
			EstimatedCostCents:        320000,
			EstimatedCostCurrencyCode: "CAD",
		})
		if err != nil {
			return err
		}
		item = got
		return nil
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	t.Run("matching id+org returns the row with budget context", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			got, err := s.GetProcurementItem(ctx, tx, item.ID, orgID)
			if err != nil {
				return err
			}
			if got.ID != item.ID {
				t.Errorf("id = %v, want %v", got.ID, item.ID)
			}
			if got.EstimatedCostCents != 320000 || got.EstimatedCostCurrencyCode != "CAD" {
				t.Errorf("budget context = %d %s, want 320000 CAD", got.EstimatedCostCents, got.EstimatedCostCurrencyCode)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cross-org returns not-found", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			_, err := s.GetProcurementItem(ctx, tx, item.ID, uuid.New())
			if !errors.Is(err, ErrProcurementItemNotFound) {
				t.Errorf("err = %v, want ErrProcurementItemNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unknown id returns not-found", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			_, err := s.GetProcurementItem(ctx, tx, uuid.New(), orgID)
			if !errors.Is(err, ErrProcurementItemNotFound) {
				t.Errorf("err = %v, want ErrProcurementItemNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

// TestProcurementStore_CreateProcurementRecommendation covers the
// AI-recommendation insert: a batch of rows sharing one RunID commit
// together, with the nullable VendorID populated on one row and nil on
// another, and Reasoning round-tripping. Proves the FK to the parent
// item + the composite-currency spend pair persist.
func TestProcurementStore_CreateProcurementRecommendation(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProcurementStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	var item models.ProcurementItem
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.CreateProcurementItem(ctx, tx, CreateProcurementItemParams{
			ProjectID:                 projectID,
			OrgID:                     orgID,
			Name:                      "HVAC units",
			WBSCode:                   "23.00.00",
			EstimatedCostCents:        500000,
			EstimatedCostCurrencyCode: "USD",
		})
		if err != nil {
			return err
		}
		item = got
		return nil
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	runID := uuid.New()
	knownVendor := uuid.New()
	reasoning := "lowest predicted spend + on-time history"

	var recs []models.ProcurementRecommendation
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// Row 1: a known vendor with reasoning.
		r1, err := s.CreateProcurementRecommendation(ctx, tx, CreateProcurementRecommendationParams{
			ProcurementItemID:          item.ID,
			OrgID:                      orgID,
			RunID:                      runID,
			VendorID:                   &knownVendor,
			VendorName:                 "Northwind HVAC",
			PredictedSpendCents:        480000,
			PredictedSpendCurrencyCode: "USD",
			ConfidencePct:              88,
			Reasoning:                  &reasoning,
		})
		if err != nil {
			return err
		}
		// Row 2: an as-yet-unknown vendor (nil VendorID, nil reasoning).
		r2, err := s.CreateProcurementRecommendation(ctx, tx, CreateProcurementRecommendationParams{
			ProcurementItemID:          item.ID,
			OrgID:                      orgID,
			RunID:                      runID,
			VendorID:                   nil,
			VendorName:                 "Untracked Supply Co",
			PredictedSpendCents:        510000,
			PredictedSpendCurrencyCode: "USD",
			ConfidencePct:              61,
		})
		if err != nil {
			return err
		}
		recs = []models.ProcurementRecommendation{r1, r2}
		return nil
	})
	if err != nil {
		t.Fatalf("create recommendations: %v", err)
	}

	if len(recs) != 2 {
		t.Fatalf("got %d recs, want 2", len(recs))
	}
	for _, r := range recs {
		if r.RunID != runID {
			t.Errorf("rec %v run_id = %v, want shared %v", r.ID, r.RunID, runID)
		}
		if r.ProcurementItemID != item.ID {
			t.Errorf("rec %v item_id = %v, want %v", r.ID, r.ProcurementItemID, item.ID)
		}
		if r.CreatedAt.IsZero() {
			t.Errorf("rec %v created_at is zero", r.ID)
		}
	}
	// Row 1: vendor + reasoning populated.
	if recs[0].VendorID == nil || *recs[0].VendorID != knownVendor {
		t.Errorf("rec[0] vendor_id = %v, want %v", recs[0].VendorID, knownVendor)
	}
	if recs[0].Reasoning == nil || *recs[0].Reasoning != reasoning {
		t.Errorf("rec[0] reasoning = %v, want %q", recs[0].Reasoning, reasoning)
	}
	if recs[0].ConfidencePct != 88 || recs[0].PredictedSpendCents != 480000 {
		t.Errorf("rec[0] confidence/spend = %d/%d, want 88/480000", recs[0].ConfidencePct, recs[0].PredictedSpendCents)
	}
	// Row 2: nullable vendor + reasoning stay nil.
	if recs[1].VendorID != nil {
		t.Errorf("rec[1] vendor_id = %v, want nil", recs[1].VendorID)
	}
	if recs[1].Reasoning != nil {
		t.Errorf("rec[1] reasoning = %v, want nil", recs[1].Reasoning)
	}

	// Both rows persisted under the shared run.
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM procurement_recommendations WHERE run_id = $1`, runID).Scan(&count); err != nil {
		t.Fatalf("count by run: %v", err)
	}
	if count != 2 {
		t.Errorf("rows for run = %d, want 2", count)
	}
}

// TestProcurementStore_RecomputeStatuses_GuardsAndDefaults covers the two
// deterministic input legs the bulk-transition test skips (it always passes a
// non-negative window and a fixed non-zero Now):
//   - WarningWindowDays < 0 → early error, no UPDATE issued;
//   - Now.IsZero() → defaults to time.Now().UTC(), so an item whose
//     must_order_date sits far in the past flips OK → CRITICAL.
func TestProcurementStore_RecomputeStatuses_GuardsAndDefaults(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProcurementStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	// Seed one item with a must_order_date far in the past (need_by 2020,
	// so must_order ~2019). Against a defaulted real-now it must read CRITICAL.
	needBy := time.Date(2020, 1, 10, 0, 0, 0, 0, time.UTC)
	var itemID uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		it, e := s.CreateProcurementItem(ctx, tx, CreateProcurementItemParams{
			ProjectID:                 projectID,
			OrgID:                     orgID,
			Name:                      "stale",
			WBSCode:                   "01.00.00",
			EstimatedCostCents:        1000,
			EstimatedCostCurrencyCode: "USD",
			LeadTimeDays:              5,
			WeatherBufferDays:         1,
			NeedByDate:                &needBy,
		})
		if e != nil {
			return e
		}
		itemID = it.ID
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Negative window → guard error before any UPDATE.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		n, e := s.RecomputeStatuses(ctx, tx, RecomputeStatusesParams{WarningWindowDays: -1, Now: time.Now().UTC()})
		if e == nil {
			t.Error("negative window should error")
		}
		if n != 0 {
			t.Errorf("rowsChanged = %d, want 0 on guard error", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("negative-window tx: %v", err)
	}

	// Zero Now → defaults to time.Now().UTC(); the 2019 must_order_date is
	// well in the past so the item flips OK → CRITICAL (one row changed).
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		n, e := s.RecomputeStatuses(ctx, tx, RecomputeStatusesParams{WarningWindowDays: 7}) // Now left zero
		if e != nil {
			return e
		}
		if n != 1 {
			t.Errorf("rowsChanged = %d, want 1 (defaulted now flips stale → CRITICAL)", n)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("zero-now tx: %v", err)
	}

	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM procurement_items WHERE id = $1`, itemID).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != models.ProcurementStatusCritical {
		t.Errorf("status = %q, want %q", status, models.ProcurementStatusCritical)
	}
}

func TestProcurementStore_RecomputeStatuses_BulkTransition(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewProcurementStore()
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedProject(t, pool, projectID, orgID, "Test Project")

	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	window := 7

	// Seed three items with different distance to must_order_date.
	//
	//   past:    must_order=apr-25  → CRITICAL (now is may-1)
	//   warn:    must_order=may-5   → WARNING  (within 7-day window)
	//   future:  must_order=jun-1   → OK       (well outside window)
	//   ordered: must_order=apr-25 status=ORDERED → unchanged
	specs := []struct {
		name       string
		needBy     time.Time
		leadDays   int
		buffer     int
		setOrdered bool
	}{
		{"past", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 5, 1, false},   // must_order = apr-25
		{"warn", time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC), 5, 2, false},  // must_order = may-5
		{"future", time.Date(2026, 6, 8, 0, 0, 0, 0, time.UTC), 5, 2, false}, // must_order = jun-1
		{"ordered", time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), 5, 1, true},
	}

	ids := make(map[string]uuid.UUID, len(specs))
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		for _, sp := range specs {
			needBy := sp.needBy
			it, err := s.CreateProcurementItem(ctx, tx, CreateProcurementItemParams{
				ProjectID:                 projectID,
				OrgID:                     orgID,
				Name:                      sp.name,
				WBSCode:                   "01.00.00",
				EstimatedCostCents:        1000,
				EstimatedCostCurrencyCode: "USD",
				LeadTimeDays:              sp.leadDays,
				WeatherBufferDays:         sp.buffer,
				NeedByDate:                &needBy,
			})
			if err != nil {
				return err
			}
			ids[sp.name] = it.ID
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Mark the "ordered" row as ORDERED — recompute must skip it.
	if _, err := pool.Exec(ctx, `UPDATE procurement_items SET status = 'ORDERED' WHERE id = $1`, ids["ordered"]); err != nil {
		t.Fatalf("set ORDERED: %v", err)
	}

	var rowsChanged int64
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		got, err := s.RecomputeStatuses(ctx, tx, RecomputeStatusesParams{
			WarningWindowDays: window,
			Now:               now,
		})
		if err != nil {
			return err
		}
		rowsChanged = got
		return nil
	})
	if err != nil {
		t.Fatalf("recompute: %v", err)
	}

	// past + warn flipped from default OK → CRITICAL/WARNING; future
	// stayed at OK. ordered was skipped. Total flips = 2.
	if rowsChanged != 2 {
		t.Errorf("rowsChanged = %d, want 2", rowsChanged)
	}

	// Verify the final statuses.
	want := map[string]string{
		"past":    models.ProcurementStatusCritical,
		"warn":    models.ProcurementStatusWarning,
		"future":  models.ProcurementStatusOK,
		"ordered": models.ProcurementStatusOrdered,
	}
	for name, id := range ids {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status FROM procurement_items WHERE id = $1`, id).Scan(&status); err != nil {
			t.Fatalf("read status %s: %v", name, err)
		}
		if status != want[name] {
			t.Errorf("%s: status = %q, want %q", name, status, want[name])
		}
	}
}
