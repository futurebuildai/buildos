package worker

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// Registry holds the River client and workers configuration.
type Registry struct {
	Client  *river.Client[pgx.Tx]
	Workers *river.Workers
	pool    *pgxpool.Pool
	logger  *slog.Logger
}

// Dependencies bundles non-trivial services that workers need. Workers
// without dependencies stay zero-init; new workers gain fields here as
// they require service-layer access.
type Dependencies struct {
	BudgetRunner          BudgetRunner          // CorporateRollupWorker
	NotificationDeliverer NotificationDeliverer // FieldNotificationRetryWorker
	ProcurementChecker    ProcurementChecker    // ProcurementCheckWorker
	A2AWebhookDeliverer   A2AWebhookDeliverer   // A2AWebhookDispatchWorker (optional)
}

// NewRegistry creates a River worker registry with all job workers registered.
// It initializes the River client but does not start it — call Start() separately.
func NewRegistry(pool *pgxpool.Pool, logger *slog.Logger, deps Dependencies) (*Registry, error) {
	workers := river.NewWorkers()

	// Register all job workers.
	// Sprint 0: register placeholder workers to validate the wiring.
	// Full implementations arrive in later sprints.
	river.AddWorker(workers, &DailyBriefingWorker{})
	river.AddWorker(workers, NewProcurementCheckWorker(deps.ProcurementChecker))
	river.AddWorker(workers, &HydrateProjectWorker{})
	river.AddWorker(workers, NewCorporateRollupWorker(deps.BudgetRunner))
	river.AddWorker(workers, &CertificationAlertsWorker{})
	river.AddWorker(workers, &MaintenanceRemindersWorker{})
	river.AddWorker(workers, NewFieldNotificationRetryWorker(deps.NotificationDeliverer))
	river.AddWorker(workers, &DelayCascadeWorker{})
	if deps.A2AWebhookDeliverer != nil {
		river.AddWorker(workers, NewA2AWebhookDispatchWorker(deps.A2AWebhookDeliverer))
	} else {
		// No-op fallback for fork deployments that haven't provisioned
		// a signing key yet — jobs queue but the worker just logs
		// rather than panicking on a nil deliverer.
		river.AddWorker(workers, &noopA2AWorker{})
	}
	river.AddWorker(workers, &SubLiaisonScanWorker{})
	river.AddWorker(workers, &PipelineAnalyticsWorker{})
	river.AddWorker(workers, &PermitIssuedTransitionWorker{})

	periodicJobs := []*river.PeriodicJob{
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return DailyBriefingArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return ProcurementCheckArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return CorporateRollupArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(7*24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return CertificationAlertsArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(7*24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return MaintenanceRemindersArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return PipelineAnalyticsArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
		river.NewPeriodicJob(
			river.PeriodicInterval(24*time.Hour),
			func() (river.JobArgs, *river.InsertOpts) {
				return SubLiaisonScanArgs{}, nil
			},
			&river.PeriodicJobOpts{RunOnStart: false},
		),
	}

	client, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Workers:      workers,
		Queues:       map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 100}},
		PeriodicJobs: periodicJobs,
		Logger:       logger,
	})
	if err != nil {
		return nil, fmt.Errorf("creating river client: %w", err)
	}

	return &Registry{
		Client:  client,
		Workers: workers,
		pool:    pool,
		logger:  logger,
	}, nil
}

// noopA2AWorker is the fallback for deployments without an outbound
// A2A signing key wired. River still drains the queue, but the worker
// logs and discards rather than failing — fork operators see queue
// depth ticking up in metrics if they forgot to provision a key.
type noopA2AWorker struct {
	river.WorkerDefaults[A2AWebhookDispatchArgs]
}

func (w *noopA2AWorker) Work(ctx context.Context, job *river.Job[A2AWebhookDispatchArgs]) error {
	slog.WarnContext(ctx, "a2a_webhook_dispatch: no signing key wired; discarding",
		"event_type", job.Args.EventType,
		"org_id", job.Args.OrgID)
	return nil
}

// Start begins processing jobs. Blocks until ctx is cancelled.
func (r *Registry) Start(ctx context.Context) error {
	r.logger.Info("starting river worker")
	return r.Client.Start(ctx)
}
