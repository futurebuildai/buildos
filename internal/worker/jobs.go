package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/futurebuild/futurebuild-os/internal/a2a"
	"github.com/futurebuild/futurebuild-os/internal/agents"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/physics"
	"github.com/futurebuild/futurebuild-os/internal/service"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

// --- Job Args (implement river.JobArgs) ---

type DailyBriefingArgs struct{}

func (DailyBriefingArgs) Kind() string { return "daily_briefing" }

type ProcurementCheckArgs struct{}

func (ProcurementCheckArgs) Kind() string { return "procurement_check" }

type HydrateProjectArgs struct {
	ProjectID uuid.UUID `json:"project_id"`
}

func (HydrateProjectArgs) Kind() string { return "hydrate_project" }

type CorporateRollupArgs struct{}

func (CorporateRollupArgs) Kind() string { return "corporate_rollup" }

type CertificationAlertsArgs struct{}

func (CertificationAlertsArgs) Kind() string { return "certification_alerts" }

type MaintenanceRemindersArgs struct{}

func (MaintenanceRemindersArgs) Kind() string { return "maintenance_reminders" }

type FieldNotificationRetryArgs struct {
	UserID           uuid.UUID `json:"user_id"`
	NotificationType string    `json:"notification_type"`
	Payload          string    `json:"payload"`
}

func (FieldNotificationRetryArgs) Kind() string { return "field_notification_retry" }

type DelayCascadeArgs struct {
	ProjectID uuid.UUID `json:"project_id"`
	WBSCodes  []string  `json:"wbs_codes,omitempty"` // Optional: specific WBS codes that triggered the cascade
}

func (DelayCascadeArgs) Kind() string { return "delay_cascade" }

type A2AWebhookDispatchArgs struct {
	EventType string `json:"event_type"`
	Payload   string `json:"payload"`
	TraceID   string `json:"trace_id"`
}

func (A2AWebhookDispatchArgs) Kind() string { return "a2a_webhook_dispatch" }

type SubLiaisonScanArgs struct{}

func (SubLiaisonScanArgs) Kind() string { return "sub_liaison_scan" }

type PipelineAnalyticsArgs struct{}

func (PipelineAnalyticsArgs) Kind() string { return "pipeline_analytics" }

type PermitIssuedTransitionArgs struct {
	ProspectID       uuid.UUID `json:"prospect_id"`
	PermitIssuedDate string    `json:"permit_issued_date"` // RFC 3339 date
}

func (PermitIssuedTransitionArgs) Kind() string { return "permit_issued_transition" }

type DriftDetectionArgs struct {
	OrgID uuid.UUID `json:"org_id"`
}

func (DriftDetectionArgs) Kind() string { return "drift_detection" }

// --- Workers ---

// DailyBriefingWorker generates morning briefing feed cards for all orgs.
type DailyBriefingWorker struct {
	river.WorkerDefaults[DailyBriefingArgs]
	pool *pgxpool.Pool
}

// NewDailyBriefingWorker creates a worker with database access.
func NewDailyBriefingWorker(pool *pgxpool.Pool) *DailyBriefingWorker {
	return &DailyBriefingWorker{pool: pool}
}

func (w *DailyBriefingWorker) Work(ctx context.Context, job *river.Job[DailyBriefingArgs]) error {
	slog.InfoContext(ctx, "daily_briefing: starting")

	feedStore := store.NewFeedStore(w.pool)
	feedSvc := service.NewFeedService(feedStore)
	agent := agents.NewDailyFocusAgent(w.pool, feedSvc, slog.Default())

	rows, err := w.pool.Query(ctx, `SELECT id FROM organizations`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var orgIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		orgIDs = append(orgIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, orgID := range orgIDs {
		if err := agent.GenerateBriefings(ctx, orgID); err != nil {
			slog.ErrorContext(ctx, "daily_briefing: failed for org", "org_id", orgID, "error", err)
			continue
		}
	}

	slog.InfoContext(ctx, "daily_briefing: completed", "org_count", len(orgIDs))
	return nil
}

// ProcurementCheckWorker evaluates procurement item urgency and creates alerts.
type ProcurementCheckWorker struct {
	river.WorkerDefaults[ProcurementCheckArgs]
	pool *pgxpool.Pool
}

// NewProcurementCheckWorker creates a worker with database access.
func NewProcurementCheckWorker(pool *pgxpool.Pool) *ProcurementCheckWorker {
	return &ProcurementCheckWorker{pool: pool}
}

func (w *ProcurementCheckWorker) Work(ctx context.Context, job *river.Job[ProcurementCheckArgs]) error {
	slog.InfoContext(ctx, "procurement_check: starting")

	feedStore := store.NewFeedStore(w.pool)
	feedSvc := service.NewFeedService(feedStore)
	procStore := store.NewProcurementStore(w.pool)
	procSvc := service.NewProcurementService(procStore, feedSvc)
	agent := agents.NewProcurementAgent(w.pool, procSvc, slog.Default())

	if err := agent.RunCheck(ctx); err != nil {
		return err
	}

	slog.InfoContext(ctx, "procurement_check: completed")
	return nil
}

// HydrateProjectWorker queries a newly created project, ensures data consistency,
// and creates a feed card announcing the new project setup.
// Idempotent: re-running produces the same result (card creation is additive).
type HydrateProjectWorker struct {
	river.WorkerDefaults[HydrateProjectArgs]
	pool *pgxpool.Pool
}

// NewHydrateProjectWorker creates a worker with database access.
func NewHydrateProjectWorker(pool *pgxpool.Pool) *HydrateProjectWorker {
	return &HydrateProjectWorker{pool: pool}
}

func (w *HydrateProjectWorker) Work(ctx context.Context, job *river.Job[HydrateProjectArgs]) error {
	slog.InfoContext(ctx, "hydrate_project: starting", "project_id", job.Args.ProjectID)

	// Query the project by ID
	var projectName, projectStatus string
	var orgID uuid.UUID
	err := w.pool.QueryRow(ctx, `
		SELECT name, status, org_id FROM projects WHERE id = $1`,
		job.Args.ProjectID,
	).Scan(&projectName, &projectStatus, &orgID)
	if err != nil {
		return fmt.Errorf("hydrate_project: querying project: %w", err)
	}

	// Count tasks for the project
	var taskCount int
	err = w.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM project_tasks WHERE project_id = $1`,
		job.Args.ProjectID,
	).Scan(&taskCount)
	if err != nil {
		return fmt.Errorf("hydrate_project: counting tasks: %w", err)
	}

	// Count tasks by status for a summary
	var completedCount, inProgressCount, blockedCount int
	rows, err := w.pool.Query(ctx, `
		SELECT status, COUNT(*) FROM project_tasks
		WHERE project_id = $1
		GROUP BY status`, job.Args.ProjectID)
	if err != nil {
		return fmt.Errorf("hydrate_project: status summary query: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if scanErr := rows.Scan(&status, &count); scanErr != nil {
			return fmt.Errorf("hydrate_project: scanning status: %w", scanErr)
		}
		switch models.TaskStatus(status) {
		case models.TaskStatusCompleted:
			completedCount = count
		case models.TaskStatusInProgress:
			inProgressCount = count
		case models.TaskStatusBlocked:
			blockedCount = count
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("hydrate_project: rows error: %w", err)
	}

	// Create a feed card announcing the project hydration
	feedStore := store.NewFeedStore(w.pool)
	feedSvc := service.NewFeedService(feedStore)

	body := fmt.Sprintf("Project %q hydrated: %d tasks (%d in progress, %d completed, %d blocked)",
		projectName, taskCount, inProgressCount, completedCount, blockedCount)

	projectID := job.Args.ProjectID
	card := &models.FeedCard{
		OrgID:     orgID,
		ProjectID: &projectID,
		CardType:  models.CardTypeProgress,
		Title:     fmt.Sprintf("Project Setup Complete: %s", projectName),
		Body:      body,
		Priority:  models.PriorityNormal,
		Status:    models.FeedStatusActive,
	}

	if _, cardErr := feedSvc.CreateCard(ctx, card); cardErr != nil {
		slog.ErrorContext(ctx, "hydrate_project: failed to create feed card", "error", cardErr)
		// Non-fatal: the hydration itself succeeded
	}

	slog.InfoContext(ctx, "hydrate_project: completed",
		"project_id", job.Args.ProjectID,
		"project_name", projectName,
		"task_count", taskCount,
	)
	return nil
}

// CorporateRollupWorker runs the quarterly budget rollup for all organizations.
type CorporateRollupWorker struct {
	river.WorkerDefaults[CorporateRollupArgs]
	pool *pgxpool.Pool
}

// NewCorporateRollupWorker creates a worker with database access.
func NewCorporateRollupWorker(pool *pgxpool.Pool) *CorporateRollupWorker {
	return &CorporateRollupWorker{pool: pool}
}

func (w *CorporateRollupWorker) Work(ctx context.Context, job *river.Job[CorporateRollupArgs]) error {
	slog.InfoContext(ctx, "corporate_rollup: starting")

	financialStore := store.NewFinancialStore(w.pool)
	svc := service.NewCorporateFinancialsService(financialStore)

	// Get all org IDs
	rows, err := w.pool.Query(ctx, `SELECT id FROM organizations`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var orgIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		orgIDs = append(orgIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, orgID := range orgIDs {
		if err := svc.RunCorporateRollup(ctx, orgID); err != nil {
			slog.ErrorContext(ctx, "corporate_rollup failed for org", "org_id", orgID, "error", err)
			continue // Don't fail the whole job for one org
		}
	}

	slog.InfoContext(ctx, "corporate_rollup: completed", "org_count", len(orgIDs))
	return nil
}

// CertificationAlertsWorker queries employees with certifications expiring within
// 30 days and creates feed cards for each. Priority is high if < 7 days, normal otherwise.
// Idempotent: creates additive feed cards; duplicates are acceptable (cards are distinct per run).
type CertificationAlertsWorker struct {
	river.WorkerDefaults[CertificationAlertsArgs]
	pool *pgxpool.Pool
}

// NewCertificationAlertsWorker creates a worker with database access.
func NewCertificationAlertsWorker(pool *pgxpool.Pool) *CertificationAlertsWorker {
	return &CertificationAlertsWorker{pool: pool}
}

func (w *CertificationAlertsWorker) Work(ctx context.Context, job *river.Job[CertificationAlertsArgs]) error {
	slog.InfoContext(ctx, "certification_alerts: starting")

	feedStore := store.NewFeedStore(w.pool)
	feedSvc := service.NewFeedService(feedStore)
	hrStore := store.NewHRStore(w.pool)

	// Get all org IDs
	rows, err := w.pool.Query(ctx, `SELECT id FROM organizations`)
	if err != nil {
		return fmt.Errorf("certification_alerts: querying organizations: %w", err)
	}
	defer rows.Close()

	var orgIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if scanErr := rows.Scan(&id); scanErr != nil {
			return fmt.Errorf("certification_alerts: scanning org: %w", scanErr)
		}
		orgIDs = append(orgIDs, id)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("certification_alerts: rows error: %w", err)
	}

	now := time.Now().UTC()
	var totalAlerts int

	for _, orgID := range orgIDs {
		// Query certifications expiring within 30 days for this org
		certs, certErr := hrStore.ListExpiringCertifications(ctx, orgID, 30)
		if certErr != nil {
			slog.ErrorContext(ctx, "certification_alerts: failed for org",
				"org_id", orgID, "error", certErr)
			continue
		}

		for _, cert := range certs {
			daysRemaining := int(math.Ceil(cert.ExpiryDate.Sub(now).Hours() / 24))

			// Determine priority based on days remaining
			priority := models.PriorityNormal
			if daysRemaining < 7 {
				priority = models.PriorityUrgent
			}

			// Look up employee name for the card body
			var employeeName string
			emp, empErr := hrStore.GetEmployee(ctx, cert.EmployeeID)
			if empErr == nil {
				employeeName = fmt.Sprintf("%s %s", emp.FirstName, emp.LastName)
			} else {
				employeeName = cert.EmployeeID.String()
			}

			body := fmt.Sprintf("%s certification %q (Cert #%s) expires in %d days on %s",
				employeeName, cert.CertType, cert.CertNumber,
				daysRemaining, cert.ExpiryDate.Format("2006-01-02"))

			card := &models.FeedCard{
				OrgID:    orgID,
				CardType: "certification_alert",
				Title:    fmt.Sprintf("Certification Expiring: %s", cert.CertType),
				Body:     body,
				Priority: priority,
				Status:   models.FeedStatusActive,
			}

			if _, cardErr := feedSvc.CreateCard(ctx, card); cardErr != nil {
				slog.ErrorContext(ctx, "certification_alerts: failed to create card",
					"cert_id", cert.ID, "error", cardErr)
				continue
			}
			totalAlerts++
		}
	}

	slog.InfoContext(ctx, "certification_alerts: completed",
		"org_count", len(orgIDs),
		"alerts_created", totalAlerts,
	)
	return nil
}

// MaintenanceRemindersWorker queries fleet assets with allocations ending within
// 14 days and assets in maintenance status, then creates feed cards.
// Idempotent: creates additive feed cards per run.
type MaintenanceRemindersWorker struct {
	river.WorkerDefaults[MaintenanceRemindersArgs]
	pool *pgxpool.Pool
}

// NewMaintenanceRemindersWorker creates a worker with database access.
func NewMaintenanceRemindersWorker(pool *pgxpool.Pool) *MaintenanceRemindersWorker {
	return &MaintenanceRemindersWorker{pool: pool}
}

func (w *MaintenanceRemindersWorker) Work(ctx context.Context, job *river.Job[MaintenanceRemindersArgs]) error {
	slog.InfoContext(ctx, "maintenance_reminders: starting")

	feedStore := store.NewFeedStore(w.pool)
	feedSvc := service.NewFeedService(feedStore)

	// Get all org IDs
	orgRows, err := w.pool.Query(ctx, `SELECT id FROM organizations`)
	if err != nil {
		return fmt.Errorf("maintenance_reminders: querying organizations: %w", err)
	}
	defer orgRows.Close()

	var orgIDs []uuid.UUID
	for orgRows.Next() {
		var id uuid.UUID
		if scanErr := orgRows.Scan(&id); scanErr != nil {
			return fmt.Errorf("maintenance_reminders: scanning org: %w", scanErr)
		}
		orgIDs = append(orgIDs, id)
	}
	if err := orgRows.Err(); err != nil {
		return fmt.Errorf("maintenance_reminders: rows error: %w", err)
	}

	now := time.Now().UTC()
	cutoff := now.AddDate(0, 0, 14)
	var totalReminders int

	for _, orgID := range orgIDs {
		// Query fleet assets with allocations ending within 14 days
		// These assets will need post-use maintenance checks
		rows, queryErr := w.pool.Query(ctx, `
			SELECT fa.id, fa.name, fa.asset_type, ea.end_date
			FROM fleet_assets fa
			JOIN equipment_allocations ea ON ea.asset_id = fa.id
			WHERE fa.org_id = $1
				AND ea.end_date >= $2
				AND ea.end_date <= $3
				AND fa.status != 'retired'
			ORDER BY ea.end_date`, orgID, now, cutoff)
		if queryErr != nil {
			slog.ErrorContext(ctx, "maintenance_reminders: query failed",
				"org_id", orgID, "error", queryErr)
			continue
		}

		type assetReminder struct {
			ID        uuid.UUID
			Name      string
			AssetType string
			EndDate   time.Time
		}

		var reminders []assetReminder
		for rows.Next() {
			var r assetReminder
			if scanErr := rows.Scan(&r.ID, &r.Name, &r.AssetType, &r.EndDate); scanErr != nil {
				slog.ErrorContext(ctx, "maintenance_reminders: scan failed", "error", scanErr)
				continue
			}
			reminders = append(reminders, r)
		}
		rows.Close()

		for _, r := range reminders {
			daysUntil := int(math.Ceil(r.EndDate.Sub(now).Hours() / 24))

			priority := models.PriorityNormal
			if daysUntil <= 3 {
				priority = models.PriorityUrgent
			}

			body := fmt.Sprintf("Asset %q (%s) allocation ends in %d days on %s. Schedule maintenance check.",
				r.Name, r.AssetType, daysUntil, r.EndDate.Format("2006-01-02"))

			card := &models.FeedCard{
				OrgID:    orgID,
				CardType: "maintenance_reminder",
				Title:    fmt.Sprintf("Maintenance Due: %s", r.Name),
				Body:     body,
				Priority: priority,
				Status:   models.FeedStatusActive,
			}

			if _, cardErr := feedSvc.CreateCard(ctx, card); cardErr != nil {
				slog.ErrorContext(ctx, "maintenance_reminders: card creation failed",
					"asset_id", r.ID, "error", cardErr)
				continue
			}
			totalReminders++
		}

		// Also flag assets currently in maintenance status that may need attention
		maintenanceRows, mErr := w.pool.Query(ctx, `
			SELECT id, name, asset_type FROM fleet_assets
			WHERE org_id = $1 AND status = 'maintenance'
			ORDER BY name`, orgID)
		if mErr != nil {
			slog.ErrorContext(ctx, "maintenance_reminders: maintenance status query failed",
				"org_id", orgID, "error", mErr)
			continue
		}

		for maintenanceRows.Next() {
			var assetID uuid.UUID
			var name, assetType string
			if scanErr := maintenanceRows.Scan(&assetID, &name, &assetType); scanErr != nil {
				continue
			}

			card := &models.FeedCard{
				OrgID:    orgID,
				CardType: "maintenance_reminder",
				Title:    fmt.Sprintf("In Maintenance: %s", name),
				Body:     fmt.Sprintf("Asset %q (%s) is currently in maintenance. Review and return to service when ready.", name, assetType),
				Priority: models.PriorityLow,
				Status:   models.FeedStatusActive,
			}

			if _, cardErr := feedSvc.CreateCard(ctx, card); cardErr != nil {
				slog.ErrorContext(ctx, "maintenance_reminders: card creation failed",
					"asset_id", assetID, "error", cardErr)
				continue
			}
			totalReminders++
		}
		maintenanceRows.Close()
	}

	slog.InfoContext(ctx, "maintenance_reminders: completed",
		"org_count", len(orgIDs),
		"reminders_created", totalReminders,
	)
	return nil
}

// FieldNotificationRetryWorker retries failed field notifications with exponential backoff.
// Backoff schedule: 30s, 60s, 120s, 300s, 900s, 3600s (6 retries max).
type FieldNotificationRetryWorker struct {
	river.WorkerDefaults[FieldNotificationRetryArgs]
	pool *pgxpool.Pool
}

// NewFieldNotificationRetryWorker creates a worker with database access.
func NewFieldNotificationRetryWorker(pool *pgxpool.Pool) *FieldNotificationRetryWorker {
	return &FieldNotificationRetryWorker{pool: pool}
}

func (w *FieldNotificationRetryWorker) Work(ctx context.Context, job *river.Job[FieldNotificationRetryArgs]) error {
	slog.InfoContext(ctx, "field_notification_retry: processing",
		"user_id", job.Args.UserID,
		"type", job.Args.NotificationType,
	)

	a2aStore := store.NewA2AStore(w.pool)

	// Backoff schedule in seconds: 30s, 60s, 120s, 300s, 900s, 3600s
	backoffSchedule := []int{30, 60, 120, 300, 900, 3600}

	// --- Phase 1: Process DLQ entries (legacy failed notifications) ---
	dlqEntries, err := a2aStore.ListPendingDLQEntries(ctx, 50)
	if err != nil {
		return fmt.Errorf("listing DLQ entries: %w", err)
	}

	for _, entry := range dlqEntries {
		if entry.RetryCount >= entry.MaxRetries {
			_ = a2aStore.FailDLQEntry(ctx, entry.ID, "max retries exceeded")
			continue
		}

		// Attempt delivery (log for now; real implementation calls push service)
		slog.InfoContext(ctx, "field_notification_retry: delivering DLQ notification",
			"entry_id", entry.ID,
			"user_id", entry.UserID,
			"type", entry.NotificationType,
			"retry", entry.RetryCount,
		)

		if err := a2aStore.CompleteDLQEntry(ctx, entry.ID); err != nil {
			backoff := backoffSchedule[0]
			if entry.RetryCount < len(backoffSchedule) {
				backoff = backoffSchedule[entry.RetryCount]
			}
			_ = a2aStore.IncrementDLQRetry(ctx, entry.ID, err.Error(), backoff)
		}
	}

	// --- Phase 2: Process notification outbox entries ---
	outboxEntries, err := a2aStore.ListPendingOutboxEntries(ctx, 50)
	if err != nil {
		// Table may not exist yet if migration hasn't run; log and continue.
		slog.WarnContext(ctx, "field_notification_retry: outbox query failed (table may not exist yet)",
			"error", err)
		outboxEntries = nil
	}

	for _, entry := range outboxEntries {
		if entry.RetryCount >= entry.MaxRetries {
			_ = a2aStore.FailOutboxEntry(ctx, entry.ID, "max retries exceeded")
			continue
		}

		// Attempt delivery: log the notification details for now.
		// Real implementation would dispatch to push notification service, SMS gateway, etc.
		slog.InfoContext(ctx, "field_notification_retry: delivering outbox notification",
			"entry_id", entry.ID,
			"user_id", entry.UserID,
			"org_id", entry.OrgID,
			"type", entry.NotificationType,
			"title", entry.Title,
			"retry", entry.RetryCount,
		)

		// Mark as sent. If the real delivery fails, this would be replaced
		// with IncrementOutboxRetry and the appropriate error.
		if markErr := a2aStore.MarkOutboxSent(ctx, entry.ID); markErr != nil {
			backoff := backoffSchedule[0]
			if entry.RetryCount < len(backoffSchedule) {
				backoff = backoffSchedule[entry.RetryCount]
			}
			_ = a2aStore.IncrementOutboxRetry(ctx, entry.ID, markErr.Error(), backoff)
		}
	}

	slog.InfoContext(ctx, "field_notification_retry: completed",
		"dlq_processed", len(dlqEntries),
		"outbox_processed", len(outboxEntries),
	)
	return nil
}

// DelayCascadeWorker runs the CPM forward pass on a project and detects cascading
// schedule changes. When a task's early start shifts, a feed card is created for
// each affected task to notify the project team.
// Idempotent: re-running recalculates from current data; feed cards are additive.
type DelayCascadeWorker struct {
	river.WorkerDefaults[DelayCascadeArgs]
	pool *pgxpool.Pool
}

// NewDelayCascadeWorker creates a worker with database access.
func NewDelayCascadeWorker(pool *pgxpool.Pool) *DelayCascadeWorker {
	return &DelayCascadeWorker{pool: pool}
}

func (w *DelayCascadeWorker) Work(ctx context.Context, job *river.Job[DelayCascadeArgs]) error {
	projectID := job.Args.ProjectID
	slog.InfoContext(ctx, "delay_cascade: starting",
		"project_id", projectID,
		"wbs_codes", job.Args.WBSCodes,
	)

	// Look up the project's org_id and start date
	var orgID uuid.UUID
	var projectName string
	var projectStart time.Time
	err := w.pool.QueryRow(ctx, `
		SELECT org_id, name, COALESCE(start_date, created_at) FROM projects WHERE id = $1`,
		projectID,
	).Scan(&orgID, &projectName, &projectStart)
	if err != nil {
		return fmt.Errorf("delay_cascade: querying project: %w", err)
	}

	// Load all tasks for the project
	taskRows, err := w.pool.Query(ctx, `
		SELECT id, project_id, wbs_code, name, is_inspection,
			early_start, early_finish, late_start, late_finish,
			total_float_days, is_on_critical_path,
			calculated_duration, weather_adjusted_duration, manual_override_days,
			override_reason, status,
			planned_start, planned_end, actual_start, actual_end,
			verified_by_vision, verification_confidence, is_human_review_required
		FROM project_tasks WHERE project_id = $1
		ORDER BY wbs_code`, projectID)
	if err != nil {
		return fmt.Errorf("delay_cascade: querying tasks: %w", err)
	}
	defer taskRows.Close()

	var tasks []models.ProjectTask
	// Save previous early starts for cascade detection
	previousES := make(map[uuid.UUID]time.Time)
	for taskRows.Next() {
		var t models.ProjectTask
		if scanErr := taskRows.Scan(
			&t.ID, &t.ProjectID, &t.WBSCode, &t.Name, &t.IsInspection,
			&t.EarlyStart, &t.EarlyFinish, &t.LateStart, &t.LateFinish,
			&t.TotalFloatDays, &t.IsOnCriticalPath,
			&t.CalculatedDuration, &t.WeatherAdjustedDuration, &t.ManualOverrideDays,
			&t.OverrideReason, &t.Status,
			&t.PlannedStart, &t.PlannedEnd, &t.ActualStart, &t.ActualEnd,
			&t.VerifiedByVision, &t.VerificationConfidence, &t.IsHumanReviewRequired,
		); scanErr != nil {
			return fmt.Errorf("delay_cascade: scanning task: %w", scanErr)
		}
		if t.EarlyStart != nil {
			previousES[t.ID] = *t.EarlyStart
		}
		tasks = append(tasks, t)
	}
	if err := taskRows.Err(); err != nil {
		return fmt.Errorf("delay_cascade: task rows error: %w", err)
	}

	if len(tasks) == 0 {
		slog.InfoContext(ctx, "delay_cascade: no tasks found, skipping", "project_id", projectID)
		return nil
	}

	// Load dependencies
	depRows, err := w.pool.Query(ctx, `
		SELECT id, project_id, predecessor_id, successor_id, dependency_type, lag_days, is_inspection_gate
		FROM task_dependencies WHERE project_id = $1`, projectID)
	if err != nil {
		return fmt.Errorf("delay_cascade: querying dependencies: %w", err)
	}
	defer depRows.Close()

	var deps []models.TaskDependency
	for depRows.Next() {
		var d models.TaskDependency
		if scanErr := depRows.Scan(
			&d.ID, &d.ProjectID, &d.PredecessorID, &d.SuccessorID,
			&d.DependencyType, &d.LagDays, &d.IsInspectionGate,
		); scanErr != nil {
			return fmt.Errorf("delay_cascade: scanning dependency: %w", scanErr)
		}
		deps = append(deps, d)
	}
	if err := depRows.Err(); err != nil {
		return fmt.Errorf("delay_cascade: dependency rows error: %w", err)
	}

	// Build the dependency graph and run the CPM forward pass
	graph := physics.BuildDependencyGraph(tasks, deps)

	// Check for cycles before proceeding
	if cycleErr := physics.DetectCycle(graph); cycleErr != nil {
		slog.ErrorContext(ctx, "delay_cascade: cycle detected in task graph",
			"project_id", projectID, "error", cycleErr)
		return fmt.Errorf("delay_cascade: %w", cycleErr)
	}

	cal := &physics.StandardCalendar{}
	schedule, err := physics.ForwardPass(graph, projectStart, cal, nil)
	if err != nil {
		return fmt.Errorf("delay_cascade: forward pass failed: %w", err)
	}

	// Run backward pass for critical path detection
	criticalPath, err := physics.BackwardPass(graph, schedule, cal, nil)
	if err != nil {
		slog.WarnContext(ctx, "delay_cascade: backward pass failed, continuing with forward results",
			"project_id", projectID, "error", err)
	}

	// Detect cascade effects: tasks whose early start changed
	feedStore := store.NewFeedStore(w.pool)
	feedSvc := service.NewFeedService(feedStore)

	var affectedCount int
	for taskID, newSched := range schedule {
		prevES, existed := previousES[taskID]
		if !existed {
			// Task had no previous early start — this is a new calculation, not a cascade
			continue
		}

		// Compare with 1-minute tolerance (matching CPM truncation)
		if newSched.EarlyStart.Truncate(time.Minute).Equal(prevES.Truncate(time.Minute)) {
			continue
		}

		// Early start shifted — this is a cascade effect
		affectedCount++
		delta := newSched.EarlyStart.Sub(prevES)
		direction := "delayed"
		if delta < 0 {
			direction = "advanced"
			delta = -delta
		}
		deltaDays := delta.Hours() / 24

		priority := models.PriorityNormal
		if newSched.IsCritical {
			priority = models.PriorityUrgent
		}

		body := fmt.Sprintf("Task %q (%s) has been %s by %.1f days. New early start: %s",
			newSched.WBSCode, graph.Tasks[taskID].Name,
			direction, deltaDays,
			newSched.EarlyStart.Format("2006-01-02"))

		pid := projectID
		card := &models.FeedCard{
			OrgID:     orgID,
			ProjectID: &pid,
			CardType:  "schedule_update",
			Title:     fmt.Sprintf("Schedule Cascade: %s %s", newSched.WBSCode, direction),
			Body:      body,
			Priority:  priority,
			Status:    models.FeedStatusActive,
		}

		if _, cardErr := feedSvc.CreateCard(ctx, card); cardErr != nil {
			slog.ErrorContext(ctx, "delay_cascade: failed to create cascade card",
				"task_id", taskID, "error", cardErr)
		}
	}

	slog.InfoContext(ctx, "delay_cascade: completed",
		"project_id", projectID,
		"project_name", projectName,
		"total_tasks", len(tasks),
		"affected_tasks", affectedCount,
		"critical_path_length", len(criticalPath),
	)
	return nil
}

// A2AWebhookDispatchWorker sends outbound A2A webhooks from OS to Brain.
type A2AWebhookDispatchWorker struct {
	river.WorkerDefaults[A2AWebhookDispatchArgs]
	pool    *pgxpool.Pool
	emitter *a2a.Emitter // nil = log-only mode (no emitter configured)
}

// NewA2AWebhookDispatchWorker creates a worker with database access.
// If emitter is nil, the worker logs dispatches but does not send them.
func NewA2AWebhookDispatchWorker(pool *pgxpool.Pool, emitter *a2a.Emitter) *A2AWebhookDispatchWorker {
	return &A2AWebhookDispatchWorker{pool: pool, emitter: emitter}
}

func (w *A2AWebhookDispatchWorker) Work(ctx context.Context, job *river.Job[A2AWebhookDispatchArgs]) error {
	slog.InfoContext(ctx, "a2a_webhook_dispatch: sending",
		"event_type", job.Args.EventType,
		"trace_id", job.Args.TraceID,
	)

	if w.emitter == nil {
		slog.InfoContext(ctx, "a2a_webhook_dispatch: no emitter configured, skipping",
			"event_type", job.Args.EventType,
			"trace_id", job.Args.TraceID,
		)
		return nil
	}

	// Emit the webhook via the JWS-signed A2A emitter.
	// The payload from the job args is a raw JSON string; send as-is.
	var payload any
	if job.Args.Payload != "" {
		payload = json.RawMessage(job.Args.Payload)
	}

	if err := w.emitter.Emit(ctx, job.Args.EventType, payload); err != nil {
		slog.ErrorContext(ctx, "a2a_webhook_dispatch: emit failed",
			"event_type", job.Args.EventType,
			"trace_id", job.Args.TraceID,
			"error", err,
		)
		return fmt.Errorf("a2a webhook dispatch: %w", err)
	}

	slog.InfoContext(ctx, "a2a_webhook_dispatch: completed",
		"event_type", job.Args.EventType,
		"trace_id", job.Args.TraceID,
	)
	return nil
}

// SubLiaisonScanWorker scans procurement items for pending sub-contractor bids
// older than 7 days. For each stale item, it creates a follow-up feed card and
// marks the item status as follow_up_needed so the project team is alerted.
// Idempotent: feed cards are additive per run; status transitions are one-way.
type SubLiaisonScanWorker struct {
	river.WorkerDefaults[SubLiaisonScanArgs]
	pool *pgxpool.Pool
}

// NewSubLiaisonScanWorker creates a worker with database access.
func NewSubLiaisonScanWorker(pool *pgxpool.Pool) *SubLiaisonScanWorker {
	return &SubLiaisonScanWorker{pool: pool}
}

func (w *SubLiaisonScanWorker) Work(ctx context.Context, job *river.Job[SubLiaisonScanArgs]) error {
	slog.InfoContext(ctx, "sub_liaison_scan: starting")

	feedStore := store.NewFeedStore(w.pool)
	feedSvc := service.NewFeedService(feedStore)

	// Query procurement items in PENDING status with created_at older than 7 days.
	// These represent items awaiting sub-contractor bids that have gone stale.
	cutoff := time.Now().UTC().AddDate(0, 0, -7)
	rows, err := w.pool.Query(ctx, `
		SELECT pi.id, pi.org_id, pi.project_id, pi.name, pi.wbs_code, pi.created_at
		FROM procurement_items pi
		WHERE pi.status = 'PENDING'
			AND pi.created_at < $1
		ORDER BY pi.created_at ASC`, cutoff)
	if err != nil {
		return fmt.Errorf("sub_liaison_scan: querying stale items: %w", err)
	}
	defer rows.Close()

	type staleItem struct {
		ID        uuid.UUID
		OrgID     uuid.UUID
		ProjectID uuid.UUID
		Name      string
		WBSCode   string
		CreatedAt time.Time
	}

	var items []staleItem
	for rows.Next() {
		var item staleItem
		if scanErr := rows.Scan(&item.ID, &item.OrgID, &item.ProjectID, &item.Name, &item.WBSCode, &item.CreatedAt); scanErr != nil {
			return fmt.Errorf("sub_liaison_scan: scanning item: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("sub_liaison_scan: rows error: %w", err)
	}

	var followUpCount int
	for _, item := range items {
		staleDays := int(time.Since(item.CreatedAt).Hours() / 24)

		priority := models.PriorityNormal
		if staleDays > 14 {
			priority = models.PriorityUrgent
		}

		body := fmt.Sprintf("Procurement item %q (WBS %s) has been pending for %d days without a sub-contractor bid. Follow up required.",
			item.Name, item.WBSCode, staleDays)

		projectID := item.ProjectID
		card := &models.FeedCard{
			OrgID:     item.OrgID,
			ProjectID: &projectID,
			CardType:  models.CardTypeSubConfirmation,
			Title:     fmt.Sprintf("Follow-Up Needed: %s", item.Name),
			Body:      body,
			Priority:  priority,
			Status:    models.FeedStatusActive,
		}

		if _, cardErr := feedSvc.CreateCard(ctx, card); cardErr != nil {
			slog.ErrorContext(ctx, "sub_liaison_scan: failed to create card",
				"item_id", item.ID, "error", cardErr)
			continue
		}

		// Mark the item as needing follow-up by transitioning to WARNING status.
		// WARNING indicates the item needs attention without being critical yet.
		_, updateErr := w.pool.Exec(ctx, `
			UPDATE procurement_items
			SET status = 'WARNING', status_changed_at = now(), updated_at = now()
			WHERE id = $1 AND status = 'PENDING'`, item.ID)
		if updateErr != nil {
			slog.ErrorContext(ctx, "sub_liaison_scan: failed to update item status",
				"item_id", item.ID, "error", updateErr)
			continue
		}

		followUpCount++
	}

	slog.InfoContext(ctx, "sub_liaison_scan: completed",
		"scanned", len(items),
		"follow_ups_created", followUpCount,
	)
	return nil
}

// PipelineAnalyticsWorker recalculates weighted pipeline revenue for all orgs.
type PipelineAnalyticsWorker struct {
	river.WorkerDefaults[PipelineAnalyticsArgs]
	pool *pgxpool.Pool
}

// NewPipelineAnalyticsWorker creates a worker with database access.
func NewPipelineAnalyticsWorker(pool *pgxpool.Pool) *PipelineAnalyticsWorker {
	return &PipelineAnalyticsWorker{pool: pool}
}

func (w *PipelineAnalyticsWorker) Work(ctx context.Context, job *river.Job[PipelineAnalyticsArgs]) error {
	slog.InfoContext(ctx, "pipeline_analytics: starting")

	pipelineStore := store.NewPipelineStore(w.pool)
	svc := service.NewPipelineService(pipelineStore)

	// Get all org IDs
	rows, err := w.pool.Query(ctx, `SELECT id FROM organizations`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var orgIDs []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		orgIDs = append(orgIDs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, orgID := range orgIDs {
		analytics, err := svc.Analytics(ctx, orgID)
		if err != nil {
			slog.ErrorContext(ctx, "pipeline_analytics: failed for org", "org_id", orgID, "error", err)
			continue
		}
		slog.InfoContext(ctx, "pipeline_analytics: computed", "org_id", orgID, "currencies", len(analytics))
	}

	slog.InfoContext(ctx, "pipeline_analytics: completed", "org_count", len(orgIDs))
	return nil
}

// PermitIssuedTransitionWorker handles the atomic Kanban→CPM transition
// when a prospect reaches PERMIT_ISSUED.
type PermitIssuedTransitionWorker struct {
	river.WorkerDefaults[PermitIssuedTransitionArgs]
	pool *pgxpool.Pool
}

// NewPermitIssuedTransitionWorker creates a worker with database access.
func NewPermitIssuedTransitionWorker(pool *pgxpool.Pool) *PermitIssuedTransitionWorker {
	return &PermitIssuedTransitionWorker{pool: pool}
}

func (w *PermitIssuedTransitionWorker) Work(ctx context.Context, job *river.Job[PermitIssuedTransitionArgs]) error {
	slog.InfoContext(ctx, "permit_issued_transition: starting", "prospect_id", job.Args.ProspectID)

	pipelineStore := store.NewPipelineStore(w.pool)
	svc := service.NewPipelineService(pipelineStore)

	prospect, err := svc.AdvanceProspect(ctx, job.Args.ProspectID)
	if err != nil {
		slog.ErrorContext(ctx, "permit_issued_transition: failed", "prospect_id", job.Args.ProspectID, "error", err)
		return err
	}

	slog.InfoContext(ctx, "permit_issued_transition: completed",
		"prospect_id", job.Args.ProspectID,
		"project_id", prospect.ProjectID,
		"stage", prospect.PipelineStage,
	)
	return nil
}

// DriftDetectionWorker runs the drift detection agent for a specific org.
// Analyzes actual vs. predicted task durations and emits calibration_drift feed cards.
// Periodic: daily at 07:00 UTC.
type DriftDetectionWorker struct {
	river.WorkerDefaults[DriftDetectionArgs]
	pool *pgxpool.Pool
}

// NewDriftDetectionWorker creates a worker with database access.
func NewDriftDetectionWorker(pool *pgxpool.Pool) *DriftDetectionWorker {
	return &DriftDetectionWorker{pool: pool}
}

func (w *DriftDetectionWorker) Work(ctx context.Context, job *river.Job[DriftDetectionArgs]) error {
	slog.InfoContext(ctx, "drift_detection: starting", "org_id", job.Args.OrgID)

	feedStore := store.NewFeedStore(w.pool)
	feedSvc := service.NewFeedService(feedStore)
	feedWriter := agents.NewPgFeedWriter(feedSvc.CreateCard)

	driftRepo := agents.NewPgDriftRepository(w.pool)
	agent := agents.NewDriftDetectionAgent(driftRepo).WithFeedWriter(feedWriter)

	if err := agent.Execute(ctx); err != nil {
		return fmt.Errorf("drift_detection: %w", err)
	}

	slog.InfoContext(ctx, "drift_detection: completed", "org_id", job.Args.OrgID)
	return nil
}
