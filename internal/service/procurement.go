package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

var (
	ErrProcurementNotFound   = errors.New("procurement item not found")
	ErrInvalidProcStatus     = errors.New("invalid procurement status")
	ErrMissingDescription    = errors.New("description is required")
	ErrNegativeCost          = errors.New("estimated cost must be >= 0")
)

// ProcurementService provides business logic for procurement items.
type ProcurementService struct {
	store    *store.ProcurementStore
	feedSvc  *FeedService
}

// NewProcurementService creates a new ProcurementService.
func NewProcurementService(s *store.ProcurementStore, feedSvc *FeedService) *ProcurementService {
	return &ProcurementService{store: s, feedSvc: feedSvc}
}

// CreateItem validates and inserts a procurement item.
func (svc *ProcurementService) CreateItem(ctx context.Context, item *models.ProcurementItem) (uuid.UUID, error) {
	if item.Description == "" {
		return uuid.Nil, ErrMissingDescription
	}
	if item.EstimatedCostCents < 0 {
		return uuid.Nil, ErrNegativeCost
	}
	if item.EstimatedCostCurrencyCode == "" {
		item.EstimatedCostCurrencyCode = "USD"
	}
	if !SupportedCurrencies[item.EstimatedCostCurrencyCode] {
		return uuid.Nil, ErrInvalidCurrency
	}
	if item.Status == "" {
		item.Status = models.ProcurementPending
	}
	if !models.ValidProcurementStatuses[item.Status] {
		return uuid.Nil, ErrInvalidProcStatus
	}
	return svc.store.CreateItem(ctx, item)
}

// GetItem returns a procurement item by ID.
func (svc *ProcurementService) GetItem(ctx context.Context, itemID uuid.UUID) (*models.ProcurementItem, error) {
	item, err := svc.store.GetItem(ctx, itemID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProcurementNotFound
		}
		return nil, err
	}
	return item, nil
}

// ListByProject returns all procurement items for a project.
func (svc *ProcurementService) ListByProject(ctx context.Context, projectID uuid.UUID) ([]models.ProcurementItem, error) {
	return svc.store.ListByProject(ctx, projectID)
}

// UpdateItem updates a procurement item's status and optional fields.
func (svc *ProcurementService) UpdateItem(ctx context.Context, itemID uuid.UUID, status models.ProcurementStatus, supplierName, supplierContact *string) error {
	if !models.ValidProcurementStatuses[status] {
		return ErrInvalidProcStatus
	}
	err := svc.store.UpdateItem(ctx, itemID, status, supplierName, supplierContact)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProcurementNotFound
	}
	return err
}

// CancelItem soft-deletes a procurement item by setting status to CANCELLED.
func (svc *ProcurementService) CancelItem(ctx context.Context, itemID uuid.UUID) error {
	err := svc.store.UpdateStatus(ctx, itemID, models.ProcurementCancelled)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrProcurementNotFound
	}
	return err
}

// CostSummary returns per-currency cost totals for a project.
// Cross-currency arithmetic is forbidden — returns separate rows per currency.
func (svc *ProcurementService) CostSummary(ctx context.Context, projectID uuid.UUID) ([]models.ProcurementCostSummary, error) {
	return svc.store.CostSummaryByProject(ctx, projectID)
}

// CheckProcurementStatuses evaluates all PENDING/WARNING items for an org
// and transitions them based on days until must_order_date.
// Returns the items that changed status.
func (svc *ProcurementService) CheckProcurementStatuses(ctx context.Context, orgID uuid.UUID) ([]StatusChange, error) {
	items, err := svc.store.ListByOrgAndStatus(ctx, orgID, models.ProcurementPending, models.ProcurementWarning)
	if err != nil {
		return nil, err
	}

	var changes []StatusChange
	now := time.Now().UTC()

	for _, item := range items {
		if item.MustOrderDate == nil {
			continue
		}

		daysUntilOrder := int(math.Ceil(item.MustOrderDate.Sub(now).Hours() / 24))
		var newStatus models.ProcurementStatus

		switch {
		case daysUntilOrder <= 0:
			newStatus = models.ProcurementCritical
		case daysUntilOrder <= 3:
			newStatus = models.ProcurementCritical
		case daysUntilOrder <= 7:
			newStatus = models.ProcurementWarning
		default:
			continue // No change needed
		}

		if newStatus == item.Status {
			continue
		}

		if err := svc.store.UpdateStatus(ctx, item.ID, newStatus); err != nil {
			return nil, fmt.Errorf("updating status for %s: %w", item.ID, err)
		}

		changes = append(changes, StatusChange{
			ItemID:    item.ID,
			ProjectID: item.ProjectID,
			OrgID:     item.OrgID,
			OldStatus: item.Status,
			NewStatus: newStatus,
			Item:      item,
			DaysLeft:  daysUntilOrder,
		})
	}

	return changes, nil
}

// StatusChange represents a procurement item that changed urgency level.
type StatusChange struct {
	ItemID    uuid.UUID
	ProjectID uuid.UUID
	OrgID     uuid.UUID
	OldStatus models.ProcurementStatus
	NewStatus models.ProcurementStatus
	Item      models.ProcurementItem
	DaysLeft  int
}

// CreateAlertCards generates feed cards for procurement status changes.
func (svc *ProcurementService) CreateAlertCards(ctx context.Context, changes []StatusChange) error {
	if svc.feedSvc == nil {
		return nil
	}
	for _, change := range changes {
		priority := models.PriorityNormal
		title := fmt.Sprintf("Procurement: %s", change.Item.Description)
		body := ""

		switch change.NewStatus {
		case models.ProcurementWarning:
			priority = models.PriorityUrgent
			body = fmt.Sprintf("Order within %d days — %s (%d cents %s)",
				change.DaysLeft, change.Item.Description,
				change.Item.EstimatedCostCents, change.Item.EstimatedCostCurrencyCode)
		case models.ProcurementCritical:
			priority = models.PriorityCritical
			if change.DaysLeft <= 0 {
				body = fmt.Sprintf("OVERDUE — order now: %s (%d cents %s)",
					change.Item.Description,
					change.Item.EstimatedCostCents, change.Item.EstimatedCostCurrencyCode)
			} else {
				body = fmt.Sprintf("Order immediately (≤3 days): %s (%d cents %s)",
					change.Item.Description,
					change.Item.EstimatedCostCents, change.Item.EstimatedCostCurrencyCode)
			}
		}

		card := &models.FeedCard{
			OrgID:    change.OrgID,
			ProjectID: &change.ProjectID,
			CardType: models.CardTypeProcurement,
			Title:    title,
			Body:     body,
			Priority: priority,
			Status:   models.FeedStatusActive,
		}
		if _, err := svc.feedSvc.CreateCard(ctx, card); err != nil {
			return fmt.Errorf("creating procurement alert card: %w", err)
		}
	}
	return nil
}
