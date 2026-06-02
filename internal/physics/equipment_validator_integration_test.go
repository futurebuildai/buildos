//go:build integration

package physics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestValidateEquipmentConstraints_Integration exercises the real COUNT query
// against fleet_assets + equipment_allocations (migration 003), proving the
// allocated vs. unallocated branches end-to-end.
func TestValidateEquipmentConstraints_Integration(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Equipment Co")
	testdb.SeedProject(t, pool, projectID, orgID, "Equipment Project")

	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)

	// WBS 7.4 requires a grader. With nothing allocated, expect an error.
	if err := ValidateEquipmentConstraints(ctx, pool, projectID, "7.4", start, end); err == nil {
		t.Fatal("expected error when no grader is allocated, got nil")
	}

	// Allocate a grader covering the task window.
	var assetID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO fleet_assets (org_id, name, asset_type)
		VALUES ($1, 'Grader 1', 'grader') RETURNING id`, orgID).Scan(&assetID); err != nil {
		t.Fatalf("insert fleet asset: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO equipment_allocations (asset_id, project_id, start_date, end_date)
		VALUES ($1, $2, $3, $4)`, assetID, projectID, start, end); err != nil {
		t.Fatalf("insert allocation: %v", err)
	}

	// Now the grader covers the window — expect nil.
	if err := ValidateEquipmentConstraints(ctx, pool, projectID, "7.4", start, end); err != nil {
		t.Errorf("expected nil with grader allocated, got %v", err)
	}

	// A different equipment type (7.3 needs a compactor) is still unallocated.
	if err := ValidateEquipmentConstraints(ctx, pool, projectID, "7.3", start, end); err == nil {
		t.Error("expected error for unallocated compactor (7.3), got nil")
	}
}

// TestValidateProjectEquipment_Integration proves the batch path collects a
// warning per under-provisioned Site Prep task while leaving covered tasks clean.
func TestValidateProjectEquipment_Integration(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()

	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Fleet Co")
	testdb.SeedProject(t, pool, projectID, orgID, "Fleet Project")

	start := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 4, 4, 0, 0, 0, 0, time.UTC)

	// Allocate an excavator (covers 7.1/7.2) but no grader (7.4).
	var excavatorID uuid.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO fleet_assets (org_id, name, asset_type)
		VALUES ($1, 'Excavator 1', 'excavator') RETURNING id`, orgID).Scan(&excavatorID); err != nil {
		t.Fatalf("insert excavator: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO equipment_allocations (asset_id, project_id, start_date, end_date)
		VALUES ($1, $2, $3, $4)`, excavatorID, projectID, start, end); err != nil {
		t.Fatalf("insert allocation: %v", err)
	}

	schedule := []TaskSchedule{
		{WBSCode: "7.1", EarlyStart: start, EarlyFinish: end}, // excavator: covered
		{WBSCode: "7.4", EarlyStart: start, EarlyFinish: end}, // grader: missing
		{WBSCode: "9.1", EarlyStart: start, EarlyFinish: end}, // not site prep: skipped
	}

	warnings := ValidateProjectEquipment(ctx, pool, projectID, schedule)
	if len(warnings) != 1 {
		t.Fatalf("warnings = %d, want 1 (%+v)", len(warnings), warnings)
	}
	if warnings[0].TaskWBSCode != "7.4" || warnings[0].RequiredType != "grader" {
		t.Errorf("warning = %+v, want 7.4/grader", warnings[0])
	}
}
