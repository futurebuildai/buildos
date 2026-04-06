package worker

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

// =============================================================================
// Job Args JSON Serialization Tests
//
// These tests verify that job args with data fields serialize/deserialize
// correctly, which is critical for River job persistence.
// =============================================================================

func TestHydrateProjectArgs_JSONRoundTrip(t *testing.T) {
	projectID := uuid.New()
	args := HydrateProjectArgs{ProjectID: projectID}

	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded HydrateProjectArgs
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ProjectID != projectID {
		t.Errorf("ProjectID = %s, want %s", decoded.ProjectID, projectID)
	}
}

func TestFieldNotificationRetryArgs_JSONRoundTrip(t *testing.T) {
	userID := uuid.New()
	args := FieldNotificationRetryArgs{
		UserID:           userID,
		NotificationType: "push",
		Payload:          `{"title":"Test notification"}`,
	}

	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded FieldNotificationRetryArgs
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.UserID != userID {
		t.Errorf("UserID = %s, want %s", decoded.UserID, userID)
	}
	if decoded.NotificationType != "push" {
		t.Errorf("NotificationType = %q, want 'push'", decoded.NotificationType)
	}
	if decoded.Payload != `{"title":"Test notification"}` {
		t.Errorf("Payload = %q, want original payload", decoded.Payload)
	}
}

func TestDelayCascadeArgs_JSONRoundTrip(t *testing.T) {
	projectID := uuid.New()
	args := DelayCascadeArgs{
		ProjectID: projectID,
		WBSCodes:  []string{"01.010", "01.020", "02.010"},
	}

	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DelayCascadeArgs
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ProjectID != projectID {
		t.Errorf("ProjectID = %s, want %s", decoded.ProjectID, projectID)
	}
	if len(decoded.WBSCodes) != 3 {
		t.Fatalf("WBSCodes length = %d, want 3", len(decoded.WBSCodes))
	}
	if decoded.WBSCodes[0] != "01.010" {
		t.Errorf("WBSCodes[0] = %q, want '01.010'", decoded.WBSCodes[0])
	}
}

func TestDelayCascadeArgs_NilWBSCodes(t *testing.T) {
	args := DelayCascadeArgs{
		ProjectID: uuid.New(),
	}

	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DelayCascadeArgs
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	// WBSCodes should be nil (omitempty) when not provided
	if decoded.WBSCodes != nil {
		t.Errorf("WBSCodes should be nil when empty, got %v", decoded.WBSCodes)
	}
}

func TestA2AWebhookDispatchArgs_JSONRoundTrip(t *testing.T) {
	args := A2AWebhookDispatchArgs{
		EventType: "schedule.updated",
		Payload:   `{"project_id":"abc"}`,
		TraceID:   "trace-123",
	}

	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded A2AWebhookDispatchArgs
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.EventType != "schedule.updated" {
		t.Errorf("EventType = %q, want 'schedule.updated'", decoded.EventType)
	}
	if decoded.Payload != `{"project_id":"abc"}` {
		t.Errorf("Payload = %q", decoded.Payload)
	}
	if decoded.TraceID != "trace-123" {
		t.Errorf("TraceID = %q, want 'trace-123'", decoded.TraceID)
	}
}

func TestPermitIssuedTransitionArgs_JSONRoundTrip(t *testing.T) {
	prospectID := uuid.New()
	args := PermitIssuedTransitionArgs{
		ProspectID:       prospectID,
		PermitIssuedDate: "2025-06-15T00:00:00Z",
	}

	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded PermitIssuedTransitionArgs
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ProspectID != prospectID {
		t.Errorf("ProspectID = %s, want %s", decoded.ProspectID, prospectID)
	}
	if decoded.PermitIssuedDate != "2025-06-15T00:00:00Z" {
		t.Errorf("PermitIssuedDate = %q", decoded.PermitIssuedDate)
	}
}

func TestDriftDetectionArgs_JSONRoundTrip(t *testing.T) {
	orgID := uuid.New()
	args := DriftDetectionArgs{OrgID: orgID}

	b, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DriftDetectionArgs
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.OrgID != orgID {
		t.Errorf("OrgID = %s, want %s", decoded.OrgID, orgID)
	}
}

// =============================================================================
// Worker Constructor Return Value Tests
// =============================================================================

func TestWorkerConstructors_ReturnNonNil(t *testing.T) {
	tests := []struct {
		name   string
		worker interface{}
	}{
		{"DailyBriefingWorker", NewDailyBriefingWorker(nil)},
		{"ProcurementCheckWorker", NewProcurementCheckWorker(nil)},
		{"HydrateProjectWorker", NewHydrateProjectWorker(nil)},
		{"CorporateRollupWorker", NewCorporateRollupWorker(nil)},
		{"CertificationAlertsWorker", NewCertificationAlertsWorker(nil)},
		{"MaintenanceRemindersWorker", NewMaintenanceRemindersWorker(nil)},
		{"FieldNotificationRetryWorker", NewFieldNotificationRetryWorker(nil)},
		{"DelayCascadeWorker", NewDelayCascadeWorker(nil)},
		{"A2AWebhookDispatchWorker", NewA2AWebhookDispatchWorker(nil, nil)},
		{"PipelineAnalyticsWorker", NewPipelineAnalyticsWorker(nil)},
		{"PermitIssuedTransitionWorker", NewPermitIssuedTransitionWorker(nil)},
		{"DriftDetectionWorker", NewDriftDetectionWorker(nil)},
		{"SubLiaisonScanWorker", NewSubLiaisonScanWorker(nil)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.worker == nil {
				t.Errorf("%s constructor returned nil", tc.name)
			}
		})
	}
}

// =============================================================================
// Job Args with Empty/Zero Fields
// =============================================================================

func TestEmptyJobArgs_Kind(t *testing.T) {
	// Verify that zero-value args still return correct Kind strings
	tests := []struct {
		name     string
		kind     string
		expected string
	}{
		{"DailyBriefingArgs{}", DailyBriefingArgs{}.Kind(), "daily_briefing"},
		{"ProcurementCheckArgs{}", ProcurementCheckArgs{}.Kind(), "procurement_check"},
		{"CorporateRollupArgs{}", CorporateRollupArgs{}.Kind(), "corporate_rollup"},
		{"CertificationAlertsArgs{}", CertificationAlertsArgs{}.Kind(), "certification_alerts"},
		{"MaintenanceRemindersArgs{}", MaintenanceRemindersArgs{}.Kind(), "maintenance_reminders"},
		{"SubLiaisonScanArgs{}", SubLiaisonScanArgs{}.Kind(), "sub_liaison_scan"},
		{"PipelineAnalyticsArgs{}", PipelineAnalyticsArgs{}.Kind(), "pipeline_analytics"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.kind != tc.expected {
				t.Errorf("Kind() = %q, want %q", tc.kind, tc.expected)
			}
		})
	}
}

func TestHydrateProjectArgs_ZeroUUID(t *testing.T) {
	args := HydrateProjectArgs{}
	if args.ProjectID != uuid.Nil {
		t.Errorf("zero-value ProjectID should be uuid.Nil, got %s", args.ProjectID)
	}
	if args.Kind() != "hydrate_project" {
		t.Errorf("Kind() = %q, want 'hydrate_project'", args.Kind())
	}
}

func TestFieldNotificationRetryArgs_EmptyFields(t *testing.T) {
	args := FieldNotificationRetryArgs{}
	if args.UserID != uuid.Nil {
		t.Errorf("zero-value UserID should be uuid.Nil")
	}
	if args.NotificationType != "" {
		t.Errorf("zero-value NotificationType should be empty")
	}
	if args.Payload != "" {
		t.Errorf("zero-value Payload should be empty")
	}
}

func TestA2AWebhookDispatchArgs_EmptyFields(t *testing.T) {
	args := A2AWebhookDispatchArgs{}
	if args.EventType != "" {
		t.Errorf("zero-value EventType should be empty")
	}
	if args.Payload != "" {
		t.Errorf("zero-value Payload should be empty")
	}
	if args.TraceID != "" {
		t.Errorf("zero-value TraceID should be empty")
	}
}

func TestDriftDetectionArgs_ZeroOrgID(t *testing.T) {
	args := DriftDetectionArgs{}
	if args.OrgID != uuid.Nil {
		t.Errorf("zero-value OrgID should be uuid.Nil")
	}
}

func TestPermitIssuedTransitionArgs_EmptyFields(t *testing.T) {
	args := PermitIssuedTransitionArgs{}
	if args.ProspectID != uuid.Nil {
		t.Errorf("zero-value ProspectID should be uuid.Nil")
	}
	if args.PermitIssuedDate != "" {
		t.Errorf("zero-value PermitIssuedDate should be empty")
	}
}
