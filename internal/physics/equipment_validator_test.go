package physics

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// These tests cover the gate logic that runs BEFORE ValidateEquipmentConstraints
// touches the pgx pool. A nil *pgxpool.Pool is passed deliberately: any path that
// reached the COUNT query would panic, proving the prefix/requirement gates
// short-circuit. The actual query path is exercised by the integration test.

func TestValidateEquipmentConstraints_NonSitePrepIsSkipped(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	// WBS 9.1 is not a Site Prep (7.x) task — must return nil without DB access.
	if err := ValidateEquipmentConstraints(context.Background(), nil, uuid.New(), "9.1", start, end); err != nil {
		t.Errorf("non-site-prep task: err = %v, want nil", err)
	}
}

func TestValidateEquipmentConstraints_UnmappedSitePrepIsSkipped(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	// 7.9 is a Site Prep code with no equipment requirement — return nil, no DB.
	if err := ValidateEquipmentConstraints(context.Background(), nil, uuid.New(), "7.9", start, end); err != nil {
		t.Errorf("unmapped site-prep task: err = %v, want nil", err)
	}
}

func TestValidateProjectEquipment_SkipsTasksNeedingNoEquipment(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	schedule := []TaskSchedule{
		{WBSCode: "9.1", EarlyStart: start, EarlyFinish: start.Add(24 * time.Hour)}, // not 7.x
		{WBSCode: "7.9", EarlyStart: start, EarlyFinish: start.Add(24 * time.Hour)}, // 7.x, unmapped
	}
	// Neither task requires equipment, so the nil pool is never dereferenced.
	warnings := ValidateProjectEquipment(context.Background(), nil, uuid.New(), schedule)
	if len(warnings) != 0 {
		t.Errorf("warnings = %+v, want none", warnings)
	}
}

func TestSitePrepEquipmentRequirements(t *testing.T) {
	want := map[string]string{
		"7.1": "excavator",
		"7.2": "excavator",
		"7.3": "compactor",
		"7.4": "grader",
		"7.5": "concrete_pump",
	}
	if len(SitePrepEquipmentRequirements) != len(want) {
		t.Fatalf("requirement map size = %d, want %d", len(SitePrepEquipmentRequirements), len(want))
	}
	for code, typ := range want {
		if got := SitePrepEquipmentRequirements[code]; got != typ {
			t.Errorf("requirement[%s] = %q, want %q", code, got, typ)
		}
	}
}
