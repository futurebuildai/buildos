package agents

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/service"
)

// ProcurementAgent monitors procurement items and generates alerts.
type ProcurementAgent struct {
	pool     *pgxpool.Pool
	procSvc  *service.ProcurementService
	logger   *slog.Logger
}

// NewProcurementAgent creates a new ProcurementAgent.
func NewProcurementAgent(pool *pgxpool.Pool, procSvc *service.ProcurementService, logger *slog.Logger) *ProcurementAgent {
	return &ProcurementAgent{
		pool:    pool,
		procSvc: procSvc,
		logger:  logger,
	}
}

// RunCheck evaluates all procurement items across all orgs and creates alert cards.
func (a *ProcurementAgent) RunCheck(ctx context.Context) error {
	// Get all org IDs
	rows, err := a.pool.Query(ctx, `SELECT id FROM organizations`)
	if err != nil {
		return fmt.Errorf("querying organizations: %w", err)
	}
	defer rows.Close()

	var orgIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scanning org ID: %w", err)
		}
		orgIDs = append(orgIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	totalChanges := 0
	for _, orgID := range orgIDs {
		changes, err := a.procSvc.CheckProcurementStatuses(ctx, orgID)
		if err != nil {
			a.logger.Error("procurement check failed for org",
				"org_id", orgID, "error", err)
			continue
		}

		if len(changes) > 0 {
			if err := a.procSvc.CreateAlertCards(ctx, changes); err != nil {
				a.logger.Error("failed to create procurement alerts",
					"org_id", orgID, "error", err)
				continue
			}
			totalChanges += len(changes)

			for _, ch := range changes {
				a.logger.Info("procurement status changed",
					"item_id", ch.ItemID,
					"project_id", ch.ProjectID,
					"old_status", ch.OldStatus,
					"new_status", ch.NewStatus,
					"days_left", ch.DaysLeft,
				)
			}
		}
	}

	a.logger.Info("procurement check completed", "total_changes", totalChanges)
	return nil
}
