package worker

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
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

// --- Workers (placeholder implementations for Sprint 0 wiring) ---

type DailyBriefingWorker struct {
	river.WorkerDefaults[DailyBriefingArgs]
}

func (w *DailyBriefingWorker) Work(ctx context.Context, job *river.Job[DailyBriefingArgs]) error {
	slog.InfoContext(ctx, "daily_briefing: not yet implemented")
	return nil
}

type ProcurementCheckWorker struct {
	river.WorkerDefaults[ProcurementCheckArgs]
}

func (w *ProcurementCheckWorker) Work(ctx context.Context, job *river.Job[ProcurementCheckArgs]) error {
	slog.InfoContext(ctx, "procurement_check: not yet implemented")
	return nil
}

type HydrateProjectWorker struct {
	river.WorkerDefaults[HydrateProjectArgs]
}

func (w *HydrateProjectWorker) Work(ctx context.Context, job *river.Job[HydrateProjectArgs]) error {
	slog.InfoContext(ctx, "hydrate_project: not yet implemented", "project_id", job.Args.ProjectID)
	return nil
}

type CorporateRollupWorker struct {
	river.WorkerDefaults[CorporateRollupArgs]
}

func (w *CorporateRollupWorker) Work(ctx context.Context, job *river.Job[CorporateRollupArgs]) error {
	slog.InfoContext(ctx, "corporate_rollup: not yet implemented")
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

type PipelineAnalyticsWorker struct {
	river.WorkerDefaults[PipelineAnalyticsArgs]
}

func (w *PipelineAnalyticsWorker) Work(ctx context.Context, job *river.Job[PipelineAnalyticsArgs]) error {
	slog.InfoContext(ctx, "pipeline_analytics: not yet implemented")
	return nil
}

type PermitIssuedTransitionWorker struct {
	river.WorkerDefaults[PermitIssuedTransitionArgs]
}

func (w *PermitIssuedTransitionWorker) Work(ctx context.Context, job *river.Job[PermitIssuedTransitionArgs]) error {
	slog.InfoContext(ctx, "permit_issued_transition: not yet implemented", "prospect_id", job.Args.ProspectID)
	return nil
}
