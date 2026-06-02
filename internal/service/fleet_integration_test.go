//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// newFleetService wires a FleetService to a fresh migrated pool with a real
// audit recorder (so the in-tx fleet.asset.created/allocated writes hit
// audit_log) and a seeded org + project. Returns the service + a fixture
// carrying the org/project ids; the pool is reachable via svc.pool for direct
// audit_log asserts.
func newFleetService(t *testing.T) (*FleetService, *fleetFixture) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	projectID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Ridgeline Equipment Co")
	testdb.SeedProject(t, pool, projectID, orgID, "Oak Hollow New Build")

	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	svc := NewFleetService(pool, store.NewFleetStore(), audit)
	return svc, &fleetFixture{orgID: orgID, projectID: projectID}
}

type fleetFixture struct {
	orgID     uuid.UUID
	projectID uuid.UUID
}

func fleetAuditCount(t *testing.T, s *FleetService, orgID uuid.UUID, action string) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE org_id = $1 AND action = $2`,
		orgID, action).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

// TestFleetService_CreateAsset_PersistsAndAudits is the canonical create
// round-trip: a valid asset is persisted with the default 'available' status,
// the serial is trimmed (blank → NULL), and exactly one fleet.asset.created
// audit row is written inside the same tx.
func TestFleetService_CreateAsset_PersistsAndAudits(t *testing.T) {
	svc, fx := newFleetService(t)
	ctx := context.Background()

	serial := "  SN-77421  " // trimmed by the service
	asset, err := svc.CreateAsset(ctx, fx.orgID, "owner-sub", CreateAssetInput{
		Name:         "  Cat 320 Excavator  ", // trimmed
		AssetType:    "  excavator  ",         // trimmed
		SerialNumber: &serial,
	})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	if asset.ID == uuid.Nil {
		t.Fatal("created asset has nil id")
	}
	if asset.Name != "Cat 320 Excavator" || asset.AssetType != "excavator" {
		t.Errorf("asset = %q/%q, want trimmed name+type", asset.Name, asset.AssetType)
	}
	if asset.Status != "available" {
		t.Errorf("status = %q, want default available", asset.Status)
	}
	if asset.SerialNumber == nil || *asset.SerialNumber != "SN-77421" {
		t.Errorf("serial = %v, want trimmed SN-77421", asset.SerialNumber)
	}
	if got := fleetAuditCount(t, svc, fx.orgID, "fleet.asset.created"); got != 1 {
		t.Errorf("fleet.asset.created audit rows = %d, want 1", got)
	}

	// Round-trips through the org-scoped list read.
	assets, err := svc.ListAssets(ctx, fx.orgID, nil)
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(assets) != 1 || assets[0].ID != asset.ID {
		t.Errorf("ListAssets = %+v, want the created asset", assets)
	}
}

// TestFleetService_CreateAsset_BlankSerialNormalizes proves the empty-serial
// path: a whitespace-only serial is treated as "no serial" (NULL), not a
// blank string.
func TestFleetService_CreateAsset_BlankSerialNormalizes(t *testing.T) {
	svc, fx := newFleetService(t)
	ctx := context.Background()

	blank := "   "
	asset, err := svc.CreateAsset(ctx, fx.orgID, "owner-sub", CreateAssetInput{
		Name: "Skid Steer", AssetType: "loader", SerialNumber: &blank,
	})
	if err != nil {
		t.Fatalf("CreateAsset: %v", err)
	}
	if asset.SerialNumber != nil {
		t.Errorf("blank serial should normalize to NULL, got %q", *asset.SerialNumber)
	}
}

// TestFleetService_CreateAsset_Validation covers the pre-tx input gates — each
// rejects with ErrInvalidInput before any row is written, so no audit row is
// produced for a rejected create.
func TestFleetService_CreateAsset_Validation(t *testing.T) {
	svc, fx := newFleetService(t)
	ctx := context.Background()

	if _, err := svc.CreateAsset(ctx, uuid.Nil, "o", CreateAssetInput{Name: "x", AssetType: "y"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateAsset(nil org) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateAsset(ctx, fx.orgID, "o", CreateAssetInput{Name: "  ", AssetType: "y"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateAsset(blank name) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateAsset(ctx, fx.orgID, "o", CreateAssetInput{Name: "x", AssetType: "  "}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateAsset(blank type) = %v, want ErrInvalidInput", err)
	}
	if got := fleetAuditCount(t, svc, fx.orgID, "fleet.asset.created"); got != 0 {
		t.Errorf("rejected creates wrote %d audit rows, want 0", got)
	}
}

// TestFleetService_AllocateAsset_BooksAndConflicts exercises the allocation
// mutation: a first booking succeeds (one fleet.asset.allocated audit row), an
// overlapping range for the same asset trips the GiST exclusion constraint and
// surfaces as ErrAllocationConflict (no second audit row), and a
// non-overlapping range books cleanly.
func TestFleetService_AllocateAsset_BooksAndConflicts(t *testing.T) {
	svc, fx := newFleetService(t)
	ctx := context.Background()

	asset, err := svc.CreateAsset(ctx, fx.orgID, "owner-sub", CreateAssetInput{Name: "Loader 1", AssetType: "loader"})
	if err != nil {
		t.Fatalf("seed CreateAsset: %v", err)
	}

	may1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	may10 := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	may15 := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	may20 := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)

	alloc, err := svc.AllocateAsset(ctx, fx.orgID, "owner-sub", AllocateAssetInput{
		AssetID: asset.ID, ProjectID: fx.projectID, StartDate: may1, EndDate: may10,
	})
	if err != nil {
		t.Fatalf("first AllocateAsset: %v", err)
	}
	if alloc.ID == uuid.Nil || alloc.AssetID != asset.ID || alloc.ProjectID != fx.projectID {
		t.Errorf("allocation = %+v, want booked to asset+project", alloc)
	}
	if got := fleetAuditCount(t, svc, fx.orgID, "fleet.asset.allocated"); got != 1 {
		t.Errorf("fleet.asset.allocated audit rows = %d, want 1", got)
	}

	// Overlapping [may1, may15) intersects [may1, may10) → conflict.
	if _, err := svc.AllocateAsset(ctx, fx.orgID, "owner-sub", AllocateAssetInput{
		AssetID: asset.ID, ProjectID: fx.projectID, StartDate: may1, EndDate: may15,
	}); !errors.Is(err, ErrAllocationConflict) {
		t.Errorf("overlapping AllocateAsset = %v, want ErrAllocationConflict", err)
	}
	// The rolled-back conflict left no second audit row.
	if got := fleetAuditCount(t, svc, fx.orgID, "fleet.asset.allocated"); got != 1 {
		t.Errorf("after conflict, fleet.asset.allocated audit rows = %d, want 1", got)
	}

	// Non-overlapping [may15, may20) books cleanly.
	if _, err := svc.AllocateAsset(ctx, fx.orgID, "owner-sub", AllocateAssetInput{
		AssetID: asset.ID, ProjectID: fx.projectID, StartDate: may15, EndDate: may20,
	}); err != nil {
		t.Errorf("non-overlapping AllocateAsset: %v", err)
	}
	if got := fleetAuditCount(t, svc, fx.orgID, "fleet.asset.allocated"); got != 2 {
		t.Errorf("fleet.asset.allocated audit rows = %d, want 2", got)
	}
}

// TestFleetService_AllocateAsset_CrossTenantHidden proves the two org-scoping
// guards: allocating from another org's vantage either via a foreign asset or
// a foreign project fails as not-found (existence not leaked), and writes
// nothing.
func TestFleetService_AllocateAsset_CrossTenantHidden(t *testing.T) {
	svc, fx := newFleetService(t)
	ctx := context.Background()

	asset, err := svc.CreateAsset(ctx, fx.orgID, "owner-sub", CreateAssetInput{Name: "Crane", AssetType: "crane"})
	if err != nil {
		t.Fatalf("seed CreateAsset: %v", err)
	}
	may1 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	may10 := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)

	// A foreign org can't see fx's asset → ErrFleetAssetNotFound (the asset
	// guard runs first).
	otherOrg := uuid.New()
	if _, err := svc.AllocateAsset(ctx, otherOrg, "intruder", AllocateAssetInput{
		AssetID: asset.ID, ProjectID: fx.projectID, StartDate: may1, EndDate: may10,
	}); !errors.Is(err, ErrFleetAssetNotFound) {
		t.Errorf("cross-org asset AllocateAsset = %v, want ErrFleetAssetNotFound", err)
	}

	// Own asset but a project owned by another org → ErrNotFound (project guard).
	foreignProject := uuid.New() // never seeded under fx.orgID
	if _, err := svc.AllocateAsset(ctx, fx.orgID, "owner-sub", AllocateAssetInput{
		AssetID: asset.ID, ProjectID: foreignProject, StartDate: may1, EndDate: may10,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign-project AllocateAsset = %v, want ErrNotFound", err)
	}

	if got := fleetAuditCount(t, svc, fx.orgID, "fleet.asset.allocated"); got != 0 {
		t.Errorf("cross-tenant allocates wrote %d audit rows, want 0", got)
	}
}

// TestFleetService_Validation covers the pre-tx gates on the allocate + list
// entrypoints — each rejects with ErrInvalidInput before any row is touched.
func TestFleetService_Validation(t *testing.T) {
	svc, fx := newFleetService(t)
	ctx := context.Background()

	may1 := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	may10 := time.Date(2026, 7, 10, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		in   AllocateAssetInput
	}{
		{"nil asset", AllocateAssetInput{AssetID: uuid.Nil, ProjectID: fx.projectID, StartDate: may1, EndDate: may10}},
		{"nil project", AllocateAssetInput{AssetID: uuid.New(), ProjectID: uuid.Nil, StartDate: may1, EndDate: may10}},
		{"zero dates", AllocateAssetInput{AssetID: uuid.New(), ProjectID: fx.projectID}},
		{"end before start", AllocateAssetInput{AssetID: uuid.New(), ProjectID: fx.projectID, StartDate: may10, EndDate: may1}},
		{"end equals start", AllocateAssetInput{AssetID: uuid.New(), ProjectID: fx.projectID, StartDate: may1, EndDate: may1}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := svc.AllocateAsset(ctx, fx.orgID, "o", c.in); !errors.Is(err, ErrInvalidInput) {
				t.Errorf("AllocateAsset(%s) = %v, want ErrInvalidInput", c.name, err)
			}
		})
	}

	// ListAssets gates: nil org + an unknown status filter value.
	if _, err := svc.ListAssets(ctx, uuid.Nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("ListAssets(nil org) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.ListAssets(ctx, fx.orgID, []string{"bogus"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("ListAssets(bad status) = %v, want ErrInvalidInput", err)
	}
}
