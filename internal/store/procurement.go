package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// ProcurementStore provides raw SQL access to the procurement_items table.
type ProcurementStore struct {
	pool *pgxpool.Pool
}

// NewProcurementStore creates a new ProcurementStore.
func NewProcurementStore(pool *pgxpool.Pool) *ProcurementStore {
	return &ProcurementStore{pool: pool}
}

// CreateItem inserts a new procurement item.
func (s *ProcurementStore) CreateItem(ctx context.Context, item *models.ProcurementItem) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO procurement_items (
			org_id, project_id, description,
			estimated_cost_cents, estimated_cost_currency_code,
			status, must_order_date, expected_delivery_date,
			supplier_name, supplier_contact
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		item.OrgID, item.ProjectID, item.Description,
		item.EstimatedCostCents, item.EstimatedCostCurrencyCode,
		item.Status, item.MustOrderDate, item.ExpectedDeliveryDate,
		item.SupplierName, item.SupplierContact,
	).Scan(&id)
	if err != nil {
		return uuid.Nil, fmt.Errorf("creating procurement item: %w", err)
	}
	return id, nil
}

// GetItem returns a procurement item by ID, scoped to org.
func (s *ProcurementStore) GetItem(ctx context.Context, orgID, itemID uuid.UUID) (*models.ProcurementItem, error) {
	var item models.ProcurementItem
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, project_id, description,
			estimated_cost_cents, estimated_cost_currency_code,
			status, must_order_date, expected_delivery_date,
			supplier_name, supplier_contact, created_at, updated_at
		FROM procurement_items WHERE id = $1 AND org_id = $2`, itemID, orgID,
	).Scan(
		&item.ID, &item.OrgID, &item.ProjectID, &item.Description,
		&item.EstimatedCostCents, &item.EstimatedCostCurrencyCode,
		&item.Status, &item.MustOrderDate, &item.ExpectedDeliveryDate,
		&item.SupplierName, &item.SupplierContact, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("getting procurement item: %w", err)
	}
	return &item, nil
}

// ListByProject returns all procurement items for a project.
func (s *ProcurementStore) ListByProject(ctx context.Context, projectID uuid.UUID) ([]models.ProcurementItem, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, project_id, description,
			estimated_cost_cents, estimated_cost_currency_code,
			status, must_order_date, expected_delivery_date,
			supplier_name, supplier_contact, created_at, updated_at
		FROM procurement_items
		WHERE project_id = $1
		ORDER BY CASE status
			WHEN 'CRITICAL' THEN 0 WHEN 'WARNING' THEN 1
			WHEN 'PENDING' THEN 2 WHEN 'DELIVERED' THEN 3 ELSE 4
		END, must_order_date ASC NULLS LAST`, projectID)
	if err != nil {
		return nil, fmt.Errorf("listing procurement items: %w", err)
	}
	defer rows.Close()

	return scanProcurementItems(rows)
}

// ListByOrgAndStatus returns items for an org filtered by status.
func (s *ProcurementStore) ListByOrgAndStatus(ctx context.Context, orgID uuid.UUID, statuses ...models.ProcurementStatus) ([]models.ProcurementItem, error) {
	statusStrings := make([]string, len(statuses))
	for i, st := range statuses {
		statusStrings[i] = string(st)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, project_id, description,
			estimated_cost_cents, estimated_cost_currency_code,
			status, must_order_date, expected_delivery_date,
			supplier_name, supplier_contact, created_at, updated_at
		FROM procurement_items
		WHERE org_id = $1 AND status = ANY($2)
		ORDER BY must_order_date ASC NULLS LAST`, orgID, statusStrings)
	if err != nil {
		return nil, fmt.Errorf("listing procurement by status: %w", err)
	}
	defer rows.Close()

	return scanProcurementItems(rows)
}

// UpdateItem updates a procurement item's mutable fields.
func (s *ProcurementStore) UpdateItem(ctx context.Context, itemID uuid.UUID, status models.ProcurementStatus, supplierName, supplierContact *string) error {
	query := `UPDATE procurement_items SET status = $2, updated_at = now()`
	args := []any{itemID, status}
	argIdx := 3

	if supplierName != nil {
		query += fmt.Sprintf(", supplier_name = $%d", argIdx)
		args = append(args, *supplierName)
		argIdx++
	}
	if supplierContact != nil {
		query += fmt.Sprintf(", supplier_contact = $%d", argIdx)
		args = append(args, *supplierContact)
		argIdx++
	}
	query += " WHERE id = $1"

	tag, err := s.pool.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("updating procurement item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// UpdateStatus transitions a procurement item to a new status.
func (s *ProcurementStore) UpdateStatus(ctx context.Context, itemID uuid.UUID, status models.ProcurementStatus) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE procurement_items SET status = $2, updated_at = now()
		WHERE id = $1`, itemID, status)
	if err != nil {
		return fmt.Errorf("updating procurement status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

// CostSummaryByProject returns per-currency cost totals for a project.
func (s *ProcurementStore) CostSummaryByProject(ctx context.Context, projectID uuid.UUID) ([]models.ProcurementCostSummary, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT estimated_cost_currency_code, SUM(estimated_cost_cents), COUNT(*)
		FROM procurement_items
		WHERE project_id = $1 AND status != 'CANCELLED'
		GROUP BY estimated_cost_currency_code`, projectID)
	if err != nil {
		return nil, fmt.Errorf("computing procurement cost summary: %w", err)
	}
	defer rows.Close()

	var summaries []models.ProcurementCostSummary
	for rows.Next() {
		var s models.ProcurementCostSummary
		if err := rows.Scan(&s.CurrencyCode, &s.TotalCents, &s.ItemCount); err != nil {
			return nil, fmt.Errorf("scanning cost summary: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, rows.Err()
}

func scanProcurementItems(rows pgx.Rows) ([]models.ProcurementItem, error) {
	var items []models.ProcurementItem
	for rows.Next() {
		var item models.ProcurementItem
		if err := rows.Scan(
			&item.ID, &item.OrgID, &item.ProjectID, &item.Description,
			&item.EstimatedCostCents, &item.EstimatedCostCurrencyCode,
			&item.Status, &item.MustOrderDate, &item.ExpectedDeliveryDate,
			&item.SupplierName, &item.SupplierContact, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning procurement item: %w", err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
