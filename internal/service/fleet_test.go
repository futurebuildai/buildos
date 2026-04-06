package service

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

func TestNewFleetService_NotNil(t *testing.T) {
	svc := NewFleetService(nil)
	if svc == nil {
		t.Fatal("expected non-nil FleetService")
	}
	if svc.store != nil {
		t.Error("expected nil store")
	}
}

// ---------------------------------------------------------------------------
// CreateAsset — validation
// ---------------------------------------------------------------------------

func TestFleet_CreateAsset_MissingName(t *testing.T) {
	svc := NewFleetService(nil)
	_, err := svc.CreateAsset(nil, &models.FleetAsset{
		Name:      "",
		AssetType: "excavator",
	})
	if !errors.Is(err, ErrMissingAssetName) {
		t.Errorf("expected ErrMissingAssetName, got: %v", err)
	}
}

func TestFleet_CreateAsset_MissingType(t *testing.T) {
	svc := NewFleetService(nil)
	_, err := svc.CreateAsset(nil, &models.FleetAsset{
		Name:      "Backhoe",
		AssetType: "",
	})
	if !errors.Is(err, ErrMissingAssetType) {
		t.Errorf("expected ErrMissingAssetType, got: %v", err)
	}
}

func TestFleet_CreateAsset_BothMissing_NameCheckedFirst(t *testing.T) {
	svc := NewFleetService(nil)
	_, err := svc.CreateAsset(nil, &models.FleetAsset{
		Name:      "",
		AssetType: "",
	})
	// Name is checked before type
	if !errors.Is(err, ErrMissingAssetName) {
		t.Errorf("expected ErrMissingAssetName (checked first), got: %v", err)
	}
}

func TestFleet_CreateAsset_DefaultStatus(t *testing.T) {
	svc := NewFleetService(nil)
	asset := &models.FleetAsset{
		Name:      "Test Crane",
		AssetType: "crane",
		Status:    "",
	}

	func() {
		defer func() { recover() }()
		_, _ = svc.CreateAsset(nil, asset)
	}()

	if asset.Status != models.AssetStatusAvailable {
		t.Errorf("expected default status %q, got %q", models.AssetStatusAvailable, asset.Status)
	}
}

func TestFleet_CreateAsset_ExplicitStatusPreserved(t *testing.T) {
	svc := NewFleetService(nil)
	statuses := []string{
		models.AssetStatusAvailable,
		models.AssetStatusAllocated,
		models.AssetStatusMaintenance,
		models.AssetStatusRetired,
	}
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			asset := &models.FleetAsset{
				Name:      "Test",
				AssetType: "truck",
				Status:    status,
			}
			func() {
				defer func() { recover() }()
				_, _ = svc.CreateAsset(nil, asset)
			}()
			if asset.Status != status {
				t.Errorf("expected preserved status %q, got %q", status, asset.Status)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AllocateAsset — date range validation
// ---------------------------------------------------------------------------

func TestFleet_AllocateAsset_EndBeforeStart(t *testing.T) {
	svc := NewFleetService(nil)
	now := time.Now().UTC()
	alloc := &models.EquipmentAllocation{
		StartDate: now,
		EndDate:   now.Add(-24 * time.Hour),
	}
	_, err := svc.AllocateAsset(nil, alloc)
	if !errors.Is(err, ErrInvalidDateRange) {
		t.Errorf("expected ErrInvalidDateRange, got: %v", err)
	}
}

func TestFleet_AllocateAsset_EndEqualsStart(t *testing.T) {
	svc := NewFleetService(nil)
	now := time.Now().UTC()
	alloc := &models.EquipmentAllocation{
		StartDate: now,
		EndDate:   now,
	}
	_, err := svc.AllocateAsset(nil, alloc)
	if !errors.Is(err, ErrInvalidDateRange) {
		t.Errorf("expected ErrInvalidDateRange for equal dates, got: %v", err)
	}
}

func TestFleet_AllocateAsset_ValidDateRange_ReachesStore(t *testing.T) {
	svc := NewFleetService(nil)
	now := time.Now().UTC()
	alloc := &models.EquipmentAllocation{
		AssetID:   uuid.New(),
		ProjectID: uuid.New(),
		StartDate: now,
		EndDate:   now.Add(7 * 24 * time.Hour),
	}
	// Valid date range passes validation, then panics on nil store (GetAsset)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.AllocateAsset(nil, alloc)
	}()
	if !panicked {
		t.Error("expected panic from nil store for valid date range")
	}
}

// ---------------------------------------------------------------------------
// isExclusionViolation — comprehensive tests
// ---------------------------------------------------------------------------

func TestFleet_IsExclusionViolation(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"postgres 23P01 code", fmt.Errorf("ERROR: 23P01"), true},
		{"exclusion keyword", fmt.Errorf("violates exclusion constraint"), true},
		{"both code and keyword", fmt.Errorf("23P01 exclusion"), true},
		{"unrelated error", fmt.Errorf("connection refused"), false},
		{"unique violation 23505", fmt.Errorf("23505 unique constraint"), false},
		{"empty error message", fmt.Errorf(""), false},
		{"partial match exclu", fmt.Errorf("exclu"), false},
		{"case sensitive Exclusion", fmt.Errorf("Exclusion"), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isExclusionViolation(tc.err); got != tc.want {
				t.Errorf("isExclusionViolation(%q) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// FleetSummary struct
// ---------------------------------------------------------------------------

func TestFleetSummary_ZeroValues(t *testing.T) {
	s := &FleetSummary{}
	if s.TotalAssets != 0 || s.Available != 0 || s.Allocated != 0 || s.InMaintenance != 0 {
		t.Error("expected all zero values for empty FleetSummary")
	}
}

func TestFleetSummary_Fields(t *testing.T) {
	s := &FleetSummary{
		TotalAssets:   10,
		Available:     5,
		Allocated:     3,
		InMaintenance: 2,
	}
	if s.TotalAssets != 10 {
		t.Errorf("TotalAssets = %d, want 10", s.TotalAssets)
	}
	if s.Available != 5 {
		t.Errorf("Available = %d, want 5", s.Available)
	}
	if s.Allocated != 3 {
		t.Errorf("Allocated = %d, want 3", s.Allocated)
	}
	if s.InMaintenance != 2 {
		t.Errorf("InMaintenance = %d, want 2", s.InMaintenance)
	}
}

// ---------------------------------------------------------------------------
// Sentinel error messages
// ---------------------------------------------------------------------------

func TestFleet_SentinelErrorMessages(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{ErrAssetNotFound, "fleet asset not found"},
		{ErrAllocationConflict, "equipment allocation conflict: overlapping date range"},
		{ErrInvalidDateRange, "end_date must be after start_date"},
		{ErrMissingAssetName, "asset name is required"},
		{ErrMissingAssetType, "asset type is required"},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if tc.err.Error() != tc.want {
				t.Errorf("got %q, want %q", tc.err.Error(), tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ListAssets / ListAllocations — nil store panics (reaches store)
// ---------------------------------------------------------------------------

func TestFleet_ListAssets_ReachesStore(t *testing.T) {
	svc := NewFleetService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.ListAssets(nil, uuid.New())
	}()
	if !panicked {
		t.Error("expected panic from nil store on ListAssets")
	}
}

func TestFleet_ListAllocations_ReachesStore(t *testing.T) {
	svc := NewFleetService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.ListAllocations(nil, uuid.New())
	}()
	if !panicked {
		t.Error("expected panic from nil store on ListAllocations")
	}
}

// ---------------------------------------------------------------------------
// Summary — nil store panics (reaches store for ListAssets)
// ---------------------------------------------------------------------------

func TestFleet_Summary_ReachesStore(t *testing.T) {
	svc := NewFleetService(nil)
	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		_, _ = svc.Summary(nil, uuid.New())
	}()
	if !panicked {
		t.Error("expected panic from nil store on Summary")
	}
}

// ---------------------------------------------------------------------------
// Asset status constants
// ---------------------------------------------------------------------------

func TestFleet_AssetStatusConstants(t *testing.T) {
	if models.AssetStatusAvailable != "available" {
		t.Errorf("AssetStatusAvailable = %q", models.AssetStatusAvailable)
	}
	if models.AssetStatusAllocated != "allocated" {
		t.Errorf("AssetStatusAllocated = %q", models.AssetStatusAllocated)
	}
	if models.AssetStatusMaintenance != "maintenance" {
		t.Errorf("AssetStatusMaintenance = %q", models.AssetStatusMaintenance)
	}
	if models.AssetStatusRetired != "retired" {
		t.Errorf("AssetStatusRetired = %q", models.AssetStatusRetired)
	}
}
