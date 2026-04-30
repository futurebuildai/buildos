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
