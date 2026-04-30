package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// ProcurementStore manages the procurement_items table.
type ProcurementStore struct{}

// NewProcurementStore creates a new ProcurementStore.
func NewProcurementStore() *ProcurementStore { return &ProcurementStore{} }

// CreateProcurementItemParams is the input for inserting a procurement
// item. NeedByDate is optional — many items are scoped without a hard
// date until the schedule pins one. The caller is expected to have
// already verified ProjectID lives in OrgID via VerifyProjectInOrg.
type CreateProcurementItemParams struct {
	ProjectID                 uuid.UUID
	OrgID                     uuid.UUID
	Name                      string
	WBSCode                   string
	EstimatedCostCents        int64
	EstimatedCostCurrencyCode string
	LeadTimeDays              int
	WeatherBufferDays         int
	VendorID                  *uuid.UUID
	NeedByDate                *time.Time
}

// CreateProcurementItem inserts a row with status='OK'. must_order_date
// is derived in Go (need_by - lead_time - weather_buffer); when
// need_by_date is nil, must_order_date is also nil. Computing in Go
// keeps the SQL parameter types unambiguous and lets unit tests assert
// the result without a database round-trip.
func (s *ProcurementStore) CreateProcurementItem(ctx context.Context, tx pgx.Tx, p CreateProcurementItemParams) (models.ProcurementItem, error) {
	var mustOrder *time.Time
	if p.NeedByDate != nil {
		d := p.NeedByDate.AddDate(0, 0, -(p.LeadTimeDays + p.WeatherBufferDays))
		mustOrder = &d
	}

	var item models.ProcurementItem
	err := tx.QueryRow(ctx, `
		INSERT INTO procurement_items (
			project_id, org_id, name, wbs_code,
			estimated_cost_cents, estimated_cost_currency_code,
			lead_time_days, weather_buffer_days, vendor_id,
			need_by_date, must_order_date
		) VALUES (
			$1, $2, $3, $4,
			$5, $6,
			$7, $8, $9,
			$10, $11
		)
		RETURNING id, project_id, org_id, name, wbs_code,
		          estimated_cost_cents, estimated_cost_currency_code,
		          lead_time_days, weather_buffer_days, vendor_id,
		          need_by_date, must_order_date,
		          status, ordered_at, po_number,
		          status_changed_at, created_at, updated_at`,
		p.ProjectID, p.OrgID, p.Name, p.WBSCode,
		p.EstimatedCostCents, p.EstimatedCostCurrencyCode,
		p.LeadTimeDays, p.WeatherBufferDays, p.VendorID,
		p.NeedByDate, mustOrder,
	).Scan(
		&item.ID, &item.ProjectID, &item.OrgID, &item.Name, &item.WBSCode,
		&item.EstimatedCostCents, &item.EstimatedCostCurrencyCode,
		&item.LeadTimeDays, &item.WeatherBufferDays, &item.VendorID,
		&item.NeedByDate, &item.MustOrderDate,
		&item.Status, &item.OrderedAt, &item.PONumber,
		&item.StatusChangedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return models.ProcurementItem{}, fmt.Errorf("insert procurement_item: %w", err)
	}
	return item, nil
}

// ListProcurementItemsParams controls a procurement listing.
type ListProcurementItemsParams struct {
	ProjectID    uuid.UUID
	OrgID        uuid.UUID
	StatusFilter []string // empty = no filter (returns all statuses)
}

// ListProcurementItems returns all procurement items for a project,
// scoped to the caller's org. Order: critical items first, then by
// must_order_date ASC (NULLs last), then created_at DESC. Status filter
// is optional.
func (s *ProcurementStore) ListProcurementItems(ctx context.Context, tx pgx.Tx, p ListProcurementItemsParams) ([]models.ProcurementItem, error) {
	args := []any{p.ProjectID, p.OrgID}
	q := `
		SELECT id, project_id, org_id, name, wbs_code,
		       estimated_cost_cents, estimated_cost_currency_code,
		       lead_time_days, weather_buffer_days, vendor_id,
		       need_by_date, must_order_date,
		       status, ordered_at, po_number,
		       status_changed_at, created_at, updated_at
		FROM procurement_items
		WHERE project_id = $1 AND org_id = $2`
	if len(p.StatusFilter) > 0 {
		args = append(args, p.StatusFilter)
		q += " AND status = ANY($3)"
	}
	q += `
		ORDER BY
		  CASE status
		    WHEN 'CRITICAL' THEN 1
		    WHEN 'WARNING'  THEN 2
		    WHEN 'OK'       THEN 3
		    WHEN 'ORDERED'  THEN 4
		    ELSE 5
		  END ASC,
		  must_order_date ASC NULLS LAST,
		  created_at DESC`

	rows, err := tx.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query procurement_items: %w", err)
	}
	defer rows.Close()

	out := make([]models.ProcurementItem, 0)
	for rows.Next() {
		var it models.ProcurementItem
		if err := rows.Scan(
			&it.ID, &it.ProjectID, &it.OrgID, &it.Name, &it.WBSCode,
			&it.EstimatedCostCents, &it.EstimatedCostCurrencyCode,
			&it.LeadTimeDays, &it.WeatherBufferDays, &it.VendorID,
			&it.NeedByDate, &it.MustOrderDate,
			&it.Status, &it.OrderedAt, &it.PONumber,
			&it.StatusChangedAt, &it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan procurement_item: %w", err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// ErrProcurementItemNotFound is returned when a single-row read or
// update misses (id + project_id + org_id all required to match).
var ErrProcurementItemNotFound = errors.New("procurement_items: not found")

// UpdateProcurementItemParams is the input for an update. All fields
// are optional pointers — nil means "leave unchanged". Status, when
// transitioning to ORDERED, requires PONumber and OrderedAt to be
// supplied (service-layer enforces this).
type UpdateProcurementItemParams struct {
	ItemID    uuid.UUID
	ProjectID uuid.UUID
	OrgID     uuid.UUID
	Status    *string
	PONumber  *string
	OrderedAt *time.Time
}

// UpdateProcurementItem applies a partial update. status_changed_at is
// stamped iff status changed. updated_at is bumped via trigger or, if
// no trigger exists, by the SET clause below.
func (s *ProcurementStore) UpdateProcurementItem(ctx context.Context, tx pgx.Tx, p UpdateProcurementItemParams) (models.ProcurementItem, error) {
	var item models.ProcurementItem
	err := tx.QueryRow(ctx, `
		UPDATE procurement_items
		SET status            = COALESCE($4, status),
		    po_number         = COALESCE($5, po_number),
		    ordered_at        = COALESCE($6, ordered_at),
		    status_changed_at = CASE WHEN $4 IS NOT NULL AND $4 <> status THEN now() ELSE status_changed_at END,
		    updated_at        = now()
		WHERE id = $1 AND project_id = $2 AND org_id = $3
		RETURNING id, project_id, org_id, name, wbs_code,
		          estimated_cost_cents, estimated_cost_currency_code,
		          lead_time_days, weather_buffer_days, vendor_id,
		          need_by_date, must_order_date,
		          status, ordered_at, po_number,
		          status_changed_at, created_at, updated_at`,
		p.ItemID, p.ProjectID, p.OrgID,
		p.Status, p.PONumber, p.OrderedAt,
	).Scan(
		&item.ID, &item.ProjectID, &item.OrgID, &item.Name, &item.WBSCode,
		&item.EstimatedCostCents, &item.EstimatedCostCurrencyCode,
		&item.LeadTimeDays, &item.WeatherBufferDays, &item.VendorID,
		&item.NeedByDate, &item.MustOrderDate,
		&item.Status, &item.OrderedAt, &item.PONumber,
		&item.StatusChangedAt, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.ProcurementItem{}, ErrProcurementItemNotFound
		}
		return models.ProcurementItem{}, fmt.Errorf("update procurement_item: %w", err)
	}
	return item, nil
}
