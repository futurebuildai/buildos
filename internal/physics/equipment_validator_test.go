package physics

import (
	"testing"
	"time"
)

// TestSitePrepEquipmentRequirements verifies the WBS->equipment mapping is complete.
func TestSitePrepEquipmentRequirements(t *testing.T) {
	expected := map[string]string{
		"7.1": "excavator",
		"7.2": "excavator",
		"7.3": "compactor",
		"7.4": "grader",
		"7.5": "concrete_pump",
	}

	for code, expectedType := range expected {
		got, ok := SitePrepEquipmentRequirements[code]
		if !ok {
			t.Errorf("missing equipment requirement for WBS %s", code)
			continue
		}
		if got != expectedType {
			t.Errorf("WBS %s: expected %s, got %s", code, expectedType, got)
		}
	}
}

// TestValidateEquipmentConstraints_NonSitePrepSkipped verifies that non-7.x WBS codes
// are skipped entirely -- no DB call needed, always returns nil.
func TestValidateEquipmentConstraints_NonSitePrepSkipped(t *testing.T) {
	now := time.Now()

	// These WBS codes should all pass through without any DB access.
	nonSitePrepCodes := []string{
		"5.2",  // Permit Issued
		"8.1",  // Foundation
		"10.3", // Rough-In Electrical
		"12.1", // Drywall
		"14.2", // Final Paint
		"",     // Empty code
	}

	for _, code := range nonSitePrepCodes {
		// Pass nil db -- if it tried to query, it would panic.
		// That's the test: it should NOT touch the DB.
		err := ValidateEquipmentConstraints(nil, nil, [16]byte{}, code, now, now.AddDate(0, 0, 5))
		if err != nil {
			t.Errorf("WBS %q should be skipped but got error: %v", code, err)
		}
	}
}

// TestValidateEquipmentConstraints_UnmappedSitePrepSkipped verifies that 7.x codes
// without a specific equipment mapping are also skipped.
func TestValidateEquipmentConstraints_UnmappedSitePrepSkipped(t *testing.T) {
	now := time.Now()

	// 7.9 is not in SitePrepEquipmentRequirements
	err := ValidateEquipmentConstraints(nil, nil, [16]byte{}, "7.9", now, now.AddDate(0, 0, 3))
	if err != nil {
		t.Errorf("WBS 7.9 (unmapped) should be skipped but got error: %v", err)
	}
}

// TestValidateProjectEquipment_FiltersNonSitePrep verifies that ValidateProjectEquipment
// only processes 7.x tasks from the schedule.
func TestValidateProjectEquipment_FiltersNonSitePrep(t *testing.T) {
	now := time.Now()
	schedule := []TaskSchedule{
		{WBSCode: "5.2", EarlyStart: now, EarlyFinish: now.AddDate(0, 0, 1)},
		{WBSCode: "8.1", EarlyStart: now, EarlyFinish: now.AddDate(0, 0, 5)},
		{WBSCode: "10.3", EarlyStart: now, EarlyFinish: now.AddDate(0, 0, 3)},
	}

	// nil DB -- none of these should trigger a DB call
	warnings := ValidateProjectEquipment(nil, nil, [16]byte{}, schedule)
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings for non-site-prep tasks, got %d", len(warnings))
	}
}

// TestEquipmentWarningFields verifies the EquipmentWarning struct fields.
func TestEquipmentWarningFields(t *testing.T) {
	now := time.Now()
	w := EquipmentWarning{
		TaskWBSCode:  "7.1",
		RequiredType: "excavator",
		StartDate:    now,
		EndDate:      now.AddDate(0, 0, 5),
		Message:      "excavator required for WBS 7.1 but not allocated",
	}

	if w.TaskWBSCode != "7.1" {
		t.Error("TaskWBSCode mismatch")
	}
	if w.RequiredType != "excavator" {
		t.Error("RequiredType mismatch")
	}
	if w.Message == "" {
		t.Error("Message should not be empty")
	}
}
