package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
)

// BudgetRunner is the dependency surface CorporateRollupWorker needs from
// the service layer. Defined as an interface here (consumer side) to keep
// the worker package free of an internal/service import — service already
// imports worker for River job args, and a back-edge would create a cycle.
type BudgetRunner interface {
	RunCorporateRollup(ctx context.Context) (rowsAffected int64, err error)
}

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

// FieldNotificationRetryArgs is the River job payload for a single
// notification delivery attempt. Payload is a raw JSON document whose
// shape depends on NotificationType (sms: {"to","body"}, push: FCM/APNs
// shape, email: SES/Resend shape). End-to-end JSON typing keeps the
// data flow consistent with the JSONB DLQ column.
type FieldNotificationRetryArgs struct {
	UserID           uuid.UUID       `json:"user_id"`
	NotificationType string          `json:"notification_type"`
	Payload          json.RawMessage `json:"payload"`
}

func (FieldNotificationRetryArgs) Kind() string { return "field_notification_retry" }

// InsertOpts caps River retries at MaxNotificationAttempts. After the
// final attempt fails the worker records to the DLQ and returns the
// error so River discards the job.
func (FieldNotificationRetryArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 6}
}

// NotificationDeliverer is the dependency surface
// FieldNotificationRetryWorker needs from the service layer. Defined
// here (consumer side) for the same reason BudgetRunner is — keeps
// worker free of an internal/service import (service already imports
// worker for River args, and a back-edge would create a cycle).
type NotificationDeliverer interface {
	DeliverNotification(ctx context.Context, attempt int, args FieldNotificationRetryArgs) error
}

type DelayCascadeArgs struct {
	ProjectID uuid.UUID `json:"project_id"`
}

func (DelayCascadeArgs) Kind() string { return "delay_cascade" }

// A2AWebhookDispatchArgs is the River job payload for an outbound A2A
// webhook to The Brain. The full envelope (event_type, payload,
// trace_id, idempotency_key, timestamp, iss, org_id) is reconstructed
// in the worker; we serialize only the fields needed to do that here.
//
// Payload is a raw JSON document — the same shape Brain receives in
// the WebhookEvent.Payload field. End-to-end JSON typing matches the
// JSONB DLQ column.
type A2AWebhookDispatchArgs struct {
	OrgID          uuid.UUID       `json:"org_id"`
	EventType      string          `json:"event_type"`
	Payload        json.RawMessage `json:"payload"`
	TraceID        string          `json:"trace_id,omitempty"`
	IdempotencyKey uuid.UUID       `json:"idempotency_key"`
}

func (A2AWebhookDispatchArgs) Kind() string { return "a2a_webhook_dispatch" }

// InsertOpts caps River retries at MaxA2AOutboundAttempts. After the
// final attempt fails the worker records to the DLQ and returns the
// error so River discards the job.
func (A2AWebhookDispatchArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 6}
}

// MaxA2AOutboundAttempts is the cap on River retries for the
// a2a_webhook_dispatch job. Mirrors the field notification queue's
// budget and backoff so ops have one mental model for both queues.
const MaxA2AOutboundAttempts = 6

// A2AWebhookDeliverer is the dependency surface
// A2AWebhookDispatchWorker needs from the service layer. Defined
// here (consumer side) for the same reason BudgetRunner /
// NotificationDeliverer are — keeps worker free of an internal/service
// import.
type A2AWebhookDeliverer interface {
	DeliverA2AWebhook(ctx context.Context, attempt int, args A2AWebhookDispatchArgs) error
}

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

// ProcurementChecker is the dependency surface ProcurementCheckWorker
// needs from the service layer. Defined here (consumer side) for the
// same reason BudgetRunner / NotificationDeliverer are — keeps worker
// free of an internal/service import (service already imports worker
// for River args, and a back-edge would create a cycle).
type ProcurementChecker interface {
	RecomputeStatuses(ctx context.Context) (rowsChanged int64, err error)
}

// ProcurementCheckWorker runs the daily procurement health sweep:
// flips OK/WARNING/CRITICAL based on each row's must_order_date
// relative to today + the warning window. ORDERED rows are terminal.
type ProcurementCheckWorker struct {
	river.WorkerDefaults[ProcurementCheckArgs]
	Checker ProcurementChecker
}

// NewProcurementCheckWorker panics if checker is nil — wiring errors
// should fail at startup, not at the first scheduled tick.
func NewProcurementCheckWorker(c ProcurementChecker) *ProcurementCheckWorker {
	if c == nil {
		panic("worker: ProcurementCheckWorker requires non-nil ProcurementChecker")
	}
	return &ProcurementCheckWorker{Checker: c}
}

func (w *ProcurementCheckWorker) Work(ctx context.Context, job *river.Job[ProcurementCheckArgs]) error {
	started := time.Now()
	rows, err := w.Checker.RecomputeStatuses(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "procurement_check failed", "error", err)
		return fmt.Errorf("procurement_check: %w", err)
	}
	slog.InfoContext(ctx, "procurement_check completed",
		"rows_changed", rows,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	return nil
}

type HydrateProjectWorker struct {
	river.WorkerDefaults[HydrateProjectArgs]
}

func (w *HydrateProjectWorker) Work(ctx context.Context, job *river.Job[HydrateProjectArgs]) error {
	slog.InfoContext(ctx, "hydrate_project: not yet implemented", "project_id", job.Args.ProjectID)
	return nil
}

// CorporateRollupWorker aggregates project_budgets into corporate_budgets
// once per scheduled run. Idempotent within a calendar quarter; quarter
// rollover creates new rows automatically. See
// service.BudgetService.RunCorporateRollup for the aggregation rules.
type CorporateRollupWorker struct {
	river.WorkerDefaults[CorporateRollupArgs]
	Runner BudgetRunner
}

// NewCorporateRollupWorker panics if runner is nil — wiring error should
// fail at startup, not at the first scheduled tick.
func NewCorporateRollupWorker(runner BudgetRunner) *CorporateRollupWorker {
	if runner == nil {
		panic("worker: CorporateRollupWorker requires non-nil BudgetRunner")
	}
	return &CorporateRollupWorker{Runner: runner}
}

func (w *CorporateRollupWorker) Work(ctx context.Context, job *river.Job[CorporateRollupArgs]) error {
	started := time.Now()
	rows, err := w.Runner.RunCorporateRollup(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "corporate_rollup failed", "error", err)
		return fmt.Errorf("corporate_rollup: %w", err)
	}
	slog.InfoContext(ctx, "corporate_rollup completed",
		"rows_affected", rows,
		"duration_ms", time.Since(started).Milliseconds(),
	)
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

// FieldNotificationRetryWorker delivers a notification (SMS / push /
// email) and lets River retry on failure with the custom backoff
// schedule defined in NextRetry. The worker is thin: it delegates to a
// NotificationDeliverer (the service layer) which holds the sender +
// the DLQ writer.
type FieldNotificationRetryWorker struct {
	river.WorkerDefaults[FieldNotificationRetryArgs]
	Deliverer NotificationDeliverer
}

// NewFieldNotificationRetryWorker panics if deliverer is nil — wiring
// errors should fail at startup, not at the first scheduled tick.
func NewFieldNotificationRetryWorker(d NotificationDeliverer) *FieldNotificationRetryWorker {
	if d == nil {
		panic("worker: FieldNotificationRetryWorker requires non-nil NotificationDeliverer")
	}
	return &FieldNotificationRetryWorker{Deliverer: d}
}

func (w *FieldNotificationRetryWorker) Work(ctx context.Context, job *river.Job[FieldNotificationRetryArgs]) error {
	return w.Deliverer.DeliverNotification(ctx, job.Attempt, job.Args)
}

// NextRetry overrides River's default exponential policy with a
// schedule tuned for transient transport failures: aggressive at first
// (30s), then back off to ~hourly. Total wall clock from first failure
// to discard: 30s + 60s + 2m + 5m + 30m + 1h ≈ 1h38m.
func (w *FieldNotificationRetryWorker) NextRetry(job *river.Job[FieldNotificationRetryArgs]) time.Time {
	delays := []time.Duration{
		30 * time.Second,
		60 * time.Second,
		2 * time.Minute,
		5 * time.Minute,
		30 * time.Minute,
		60 * time.Minute,
	}
	idx := job.Attempt
	if idx >= len(delays) {
		idx = len(delays) - 1
	}
	return time.Now().Add(delays[idx])
}

type DelayCascadeWorker struct {
	river.WorkerDefaults[DelayCascadeArgs]
}

func (w *DelayCascadeWorker) Work(ctx context.Context, job *river.Job[DelayCascadeArgs]) error {
	slog.InfoContext(ctx, "delay_cascade: not yet implemented", "project_id", job.Args.ProjectID)
	return nil
}

// A2AWebhookDispatchWorker drains queued outbound A2A events and
// delegates the JWS signing + HTTP POST + DLQ-on-final-failure to a
// service-layer Deliverer. Custom backoff in NextRetry.
type A2AWebhookDispatchWorker struct {
	river.WorkerDefaults[A2AWebhookDispatchArgs]
	Deliverer A2AWebhookDeliverer
}

// NewA2AWebhookDispatchWorker panics if deliverer is nil — wiring
// errors should fail at startup, not at the first scheduled tick.
func NewA2AWebhookDispatchWorker(d A2AWebhookDeliverer) *A2AWebhookDispatchWorker {
	if d == nil {
		panic("worker: A2AWebhookDispatchWorker requires non-nil A2AWebhookDeliverer")
	}
	return &A2AWebhookDispatchWorker{Deliverer: d}
}

func (w *A2AWebhookDispatchWorker) Work(ctx context.Context, job *river.Job[A2AWebhookDispatchArgs]) error {
	return w.Deliverer.DeliverA2AWebhook(ctx, job.Attempt, job.Args)
}

// NextRetry mirrors the field notification queue's backoff schedule:
// aggressive at first (30s), then back off to ~hourly. Total wall
// clock from first failure to discard: 30s + 60s + 2m + 5m + 30m + 1h
// ≈ 1h38m.
func (w *A2AWebhookDispatchWorker) NextRetry(job *river.Job[A2AWebhookDispatchArgs]) time.Time {
	delays := []time.Duration{
		30 * time.Second,
		60 * time.Second,
		2 * time.Minute,
		5 * time.Minute,
		30 * time.Minute,
		60 * time.Minute,
	}
	idx := job.Attempt
	if idx >= len(delays) {
		idx = len(delays) - 1
	}
	return time.Now().Add(delays[idx])
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
