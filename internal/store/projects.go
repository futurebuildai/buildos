package store

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// VerifyProjectInOrg returns nil if the project belongs to the given
// org, ErrNotFound otherwise. Shared by every per-domain store and
// service that guards project-scoped operations (financials, schedule,
// future fleet/HR allocations).
//
// Lives at package scope rather than as a method on a ProjectStore type
// because it has no state and every call site is the same shape. The
// PipelineStore's VerifyProspectInOrg sits next to its prospect data
// and remains a method — different table, different domain.
func VerifyProjectInOrg(ctx context.Context, tx pgx.Tx, projectID, orgID uuid.UUID) error {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM projects WHERE id = $1 AND org_id = $2)`,
		projectID, orgID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify project in org: %w", err)
	}
	if !exists {
		return ErrNotFound
	}
	return nil
}
