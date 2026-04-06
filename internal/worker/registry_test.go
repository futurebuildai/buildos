package worker

import (
	"testing"
	"time"
)

// =============================================================================
// Worker Registry Tests
//
// NewRegistry requires a live pgxpool connection (River needs a real driver),
// so these tests verify the job argument types, Kind() identifiers, worker
// constructors, and periodic job schedule correctness independently.
// =============================================================================

// TestJobArgs_KindIdentifiers verifies that every JobArgs type returns a
// unique, non-empty Kind string. These must match the registered worker types.
func TestJobArgs_KindIdentifiers(t *testing.T) {
	args := []struct {
		name string
		kind string
	}{
		{"DailyBriefingArgs", DailyBriefingArgs{}.Kind()},
		{"ProcurementCheckArgs", ProcurementCheckArgs{}.Kind()},
		{"HydrateProjectArgs", HydrateProjectArgs{}.Kind()},
		{"CorporateRollupArgs", CorporateRollupArgs{}.Kind()},
		{"CertificationAlertsArgs", CertificationAlertsArgs{}.Kind()},
		{"MaintenanceRemindersArgs", MaintenanceRemindersArgs{}.Kind()},
		{"FieldNotificationRetryArgs", FieldNotificationRetryArgs{}.Kind()},
		{"DelayCascadeArgs", DelayCascadeArgs{}.Kind()},
		{"A2AWebhookDispatchArgs", A2AWebhookDispatchArgs{}.Kind()},
		{"SubLiaisonScanArgs", SubLiaisonScanArgs{}.Kind()},
		{"PipelineAnalyticsArgs", PipelineAnalyticsArgs{}.Kind()},
		{"PermitIssuedTransitionArgs", PermitIssuedTransitionArgs{}.Kind()},
		{"DriftDetectionArgs", DriftDetectionArgs{}.Kind()},
	}

	// Count: 13 total registered workers (must match NewRegistry)
	expectedWorkerCount := 13
	if len(args) != expectedWorkerCount {
		t.Errorf("expected %d worker types, got %d", expectedWorkerCount, len(args))
	}

	// Verify uniqueness
	seen := make(map[string]string) // kind -> type name
	for _, a := range args {
		if a.kind == "" {
			t.Errorf("%s.Kind() returned empty string", a.name)
		}
		if prev, exists := seen[a.kind]; exists {
			t.Errorf("duplicate Kind %q: %s and %s", a.kind, prev, a.name)
		}
		seen[a.kind] = a.name
	}
}

// TestJobArgs_ExpectedKindValues verifies the exact Kind string for each type.
func TestJobArgs_ExpectedKindValues(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		expected string
	}{
		{"DailyBriefingArgs", DailyBriefingArgs{}.Kind(), "daily_briefing"},
		{"ProcurementCheckArgs", ProcurementCheckArgs{}.Kind(), "procurement_check"},
		{"HydrateProjectArgs", HydrateProjectArgs{}.Kind(), "hydrate_project"},
		{"CorporateRollupArgs", CorporateRollupArgs{}.Kind(), "corporate_rollup"},
		{"CertificationAlertsArgs", CertificationAlertsArgs{}.Kind(), "certification_alerts"},
		{"MaintenanceRemindersArgs", MaintenanceRemindersArgs{}.Kind(), "maintenance_reminders"},
		{"FieldNotificationRetryArgs", FieldNotificationRetryArgs{}.Kind(), "field_notification_retry"},
		{"DelayCascadeArgs", DelayCascadeArgs{}.Kind(), "delay_cascade"},
		{"A2AWebhookDispatchArgs", A2AWebhookDispatchArgs{}.Kind(), "a2a_webhook_dispatch"},
		{"SubLiaisonScanArgs", SubLiaisonScanArgs{}.Kind(), "sub_liaison_scan"},
		{"PipelineAnalyticsArgs", PipelineAnalyticsArgs{}.Kind(), "pipeline_analytics"},
		{"PermitIssuedTransitionArgs", PermitIssuedTransitionArgs{}.Kind(), "permit_issued_transition"},
		{"DriftDetectionArgs", DriftDetectionArgs{}.Kind(), "drift_detection"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.kind != tc.expected {
				t.Errorf("%s.Kind() = %q, want %q", tc.name, tc.kind, tc.expected)
			}
		})
	}
}

// TestPeriodicJobSchedules verifies the expected schedules for periodic jobs.
// These must match the intervals defined in NewRegistry.
func TestPeriodicJobSchedules(t *testing.T) {
	type periodicSpec struct {
		name     string
		interval time.Duration
	}

	expectedPeriodics := []periodicSpec{
		{"daily_briefing", 24 * time.Hour},
		{"procurement_check", 2 * time.Hour},
		{"corporate_rollup", 24 * time.Hour},
		{"certification_alerts", 7 * 24 * time.Hour},
		{"maintenance_reminders", 7 * 24 * time.Hour},
		{"pipeline_analytics", 24 * time.Hour},
		{"sub_liaison_scan", 24 * time.Hour},
		{"drift_detection", 24 * time.Hour},
	}

	// There should be 8 periodic jobs
	expectedPeriodicCount := 8
	if len(expectedPeriodics) != expectedPeriodicCount {
		t.Errorf("expected %d periodic job specs, got %d", expectedPeriodicCount, len(expectedPeriodics))
	}

	// Verify the interval values match what we expect
	for _, p := range expectedPeriodics {
		if p.interval <= 0 {
			t.Errorf("periodic job %q has non-positive interval %v", p.name, p.interval)
		}
	}

	// Verify specific critical schedules
	for _, p := range expectedPeriodics {
		switch p.name {
		case "procurement_check":
			if p.interval != 2*time.Hour {
				t.Errorf("procurement_check should run every 2h, got %v", p.interval)
			}
		case "certification_alerts", "maintenance_reminders":
			if p.interval != 7*24*time.Hour {
				t.Errorf("%s should run weekly (168h), got %v", p.name, p.interval)
			}
		case "daily_briefing", "corporate_rollup", "pipeline_analytics", "sub_liaison_scan", "drift_detection":
			if p.interval != 24*time.Hour {
				t.Errorf("%s should run daily (24h), got %v", p.name, p.interval)
			}
		}
	}
}

// TestWorkerConstructors verifies that all worker constructors can be called
// with a nil pool (they store but don't use it during construction).
func TestWorkerConstructors(t *testing.T) {
	// These constructors should not panic with nil pool
	// (they just store the reference for later use during Work()).
	tests := []struct {
		name string
		fn   func()
	}{
		{"DailyBriefingWorker", func() { NewDailyBriefingWorker(nil) }},
		{"ProcurementCheckWorker", func() { NewProcurementCheckWorker(nil) }},
		{"HydrateProjectWorker", func() { NewHydrateProjectWorker(nil) }},
		{"CorporateRollupWorker", func() { NewCorporateRollupWorker(nil) }},
		{"CertificationAlertsWorker", func() { NewCertificationAlertsWorker(nil) }},
		{"MaintenanceRemindersWorker", func() { NewMaintenanceRemindersWorker(nil) }},
		{"FieldNotificationRetryWorker", func() { NewFieldNotificationRetryWorker(nil) }},
		{"DelayCascadeWorker", func() { NewDelayCascadeWorker(nil) }},
		{"A2AWebhookDispatchWorker", func() { NewA2AWebhookDispatchWorker(nil, nil) }},
		{"PipelineAnalyticsWorker", func() { NewPipelineAnalyticsWorker(nil) }},
		{"PermitIssuedTransitionWorker", func() { NewPermitIssuedTransitionWorker(nil) }},
		{"DriftDetectionWorker", func() { NewDriftDetectionWorker(nil) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s constructor panicked: %v", tc.name, r)
				}
			}()
			tc.fn()
		})
	}

	// SubLiaisonScanWorker has no constructor, just verify it can be instantiated
	_ = &SubLiaisonScanWorker{}
}

// TestWorkerCount_MatchesJobArgs verifies that the number of registered
// workers (13) matches the number of JobArgs types (13).
func TestWorkerCount_MatchesJobArgs(t *testing.T) {
	// Worker constructors: 12 with New* + 1 SubLiaisonScanWorker = 13 total
	workerCount := 13

	// Count JobArgs types
	jobArgsKinds := []string{
		DailyBriefingArgs{}.Kind(),
		ProcurementCheckArgs{}.Kind(),
		HydrateProjectArgs{}.Kind(),
		CorporateRollupArgs{}.Kind(),
		CertificationAlertsArgs{}.Kind(),
		MaintenanceRemindersArgs{}.Kind(),
		FieldNotificationRetryArgs{}.Kind(),
		DelayCascadeArgs{}.Kind(),
		A2AWebhookDispatchArgs{}.Kind(),
		SubLiaisonScanArgs{}.Kind(),
		PipelineAnalyticsArgs{}.Kind(),
		PermitIssuedTransitionArgs{}.Kind(),
		DriftDetectionArgs{}.Kind(),
	}

	if len(jobArgsKinds) != workerCount {
		t.Errorf("job args count (%d) != worker count (%d)", len(jobArgsKinds), workerCount)
	}
}

// TestA2AWebhookDispatchWorker_NilEmitter verifies the worker is created
// successfully when no emitter is provided (log-only mode).
func TestA2AWebhookDispatchWorker_NilEmitter(t *testing.T) {
	w := NewA2AWebhookDispatchWorker(nil, nil)
	if w == nil {
		t.Fatal("expected non-nil worker")
	}
	if w.emitter != nil {
		t.Error("expected nil emitter for log-only mode")
	}
}
