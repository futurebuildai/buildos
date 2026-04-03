package worker

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/futurebuild/futurebuild-os/internal/agents"
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

type HydrateProjectWorker struct {
	river.WorkerDefaults[HydrateProjectArgs]
}

func (w *HydrateProjectWorker) Work(ctx context.Context, job *river.Job[HydrateProjectArgs]) error {
	slog.InfoContext(ctx, "hydrate_project: not yet implemented", "project_id", job.Args.ProjectID)
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

type CertificationAlertsWorker struct {
	river.WorkerDefaults[CertificationAlertsArgs]
}

func (w *CertificationAlertsWorker) Work(ctx context.Context, job *river.Job[CertificationAlertsArgs]) error {
	slog.InfoContext(ctx, "certification_alerts: not yet implemented")
	return nil
}

type MaintenanceRemindersWorker struct {
	river.WorkerDefaults[MaintenanceRemindersArgs]
}

func (w *MaintenanceRemindersWorker) Work(ctx context.Context, job *river.Job[MaintenanceRemindersArgs]) error {
	slog.InfoContext(ctx, "maintenance_reminders: not yet implemented")
	return nil
}

type FieldNotificationRetryWorker struct {
	river.WorkerDefaults[FieldNotificationRetryArgs]
}

func (w *FieldNotificationRetryWorker) Work(ctx context.Context, job *river.Job[FieldNotificationRetryArgs]) error {
	slog.InfoContext(ctx, "field_notification_retry: not yet implemented", "user_id", job.Args.UserID)
	return nil
}

type DelayCascadeWorker struct {
	river.WorkerDefaults[DelayCascadeArgs]
}

func (w *DelayCascadeWorker) Work(ctx context.Context, job *river.Job[DelayCascadeArgs]) error {
	slog.InfoContext(ctx, "delay_cascade: not yet implemented", "project_id", job.Args.ProjectID)
	return nil
}

type A2AWebhookDispatchWorker struct {
	river.WorkerDefaults[A2AWebhookDispatchArgs]
}

func (w *A2AWebhookDispatchWorker) Work(ctx context.Context, job *river.Job[A2AWebhookDispatchArgs]) error {
	slog.InfoContext(ctx, "a2a_webhook_dispatch: not yet implemented", "event_type", job.Args.EventType)
	return nil
}

type SubLiaisonScanWorker struct {
	river.WorkerDefaults[SubLiaisonScanArgs]
}

func (w *SubLiaisonScanWorker) Work(ctx context.Context, job *river.Job[SubLiaisonScanArgs]) error {
	slog.InfoContext(ctx, "sub_liaison_scan: not yet implemented")
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
