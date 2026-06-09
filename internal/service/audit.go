package service

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/obs"
	"github.com/futurebuildai/buildos/internal/store"
)

// Resource type constants used in audit_log.resource_type. Keep in
// sync across services so per-domain history queries stay consistent.
const (
	AuditResourceFeedCard                  = "feed_card"
	AuditResourceProcurementItem           = "procurement_item"
	AuditResourceFleetAsset                = "fleet_asset"
	AuditResourceEquipmentAlloc            = "equipment_allocation"
	AuditResourceInvoice                   = "invoice"
	AuditResourceTaskProgress              = "task_progress"
	AuditResourceCrewCheckin               = "crew_checkin"
	AuditResourceDailyLog                  = "daily_log"
	AuditResourceProjectTask               = "project_task"
	AuditResourceProspect                  = "prospect"
	AuditResourceEstimate                  = "estimate"
	AuditResourcePermit                    = "permit"
	AuditResourceSchedule                  = "schedule"
	AuditResourceAIRun                     = "ai_run"
	AuditResourceProcurementRecommendation = "procurement_recommendation"
	AuditResourceCascade                   = "cascade"
	AuditResourceForesight                 = "foresight"
	AuditResourceAgentConfig               = "agent_config"
)

// AuditRecorder is the consumer-side interface every mutating service
// uses. Defined here so each domain service can take it as a
// dependency without importing the *store directly — and so tests can
// pass a no-op via NewNoopAuditRecorder.
//
// The Record call writes inside the supplied tx so audit + mutation
// commit or roll back together. Errors are logged but never
// propagated up — a missing audit row is bad but losing the
// underlying action is worse.
type AuditRecorder interface {
	Record(ctx context.Context, tx pgx.Tx, p AuditEntry)
}

// AuditEntry is the public input shape. UserSub, RequestID, and the
// JSON blobs are all optional; the service helper fills request_id
// from ctx automatically when not supplied.
type AuditEntry struct {
	OrgID        uuid.UUID
	UserSub      string
	Action       string
	ResourceType string
	ResourceID   uuid.UUID
	Before       json.RawMessage
	After        json.RawMessage
	Metadata     json.RawMessage
}

// AuditService is the production AuditRecorder. Wraps *store.AuditStore
// and a logger; pulls request_id from ctx (matching the value sent in
// X-Request-ID and stamped in structured logs).
type AuditService struct {
	store  *store.AuditStore
	logger *slog.Logger
}

// NewAuditService creates a service bound to a store + logger.
func NewAuditService(s *store.AuditStore, logger *slog.Logger) *AuditService {
	return &AuditService{store: s, logger: logger}
}

// Record writes one audit row inside the supplied tx. Insert errors
// are logged at WARN — the surrounding tx still commits, so the
// underlying action lands; ops can grep the warn line to detect
// audit-write failures and follow up.
//
// Why "log + swallow" rather than "propagate": if the audit insert
// fails (column drift, transient DB hiccup, …) bubbling the error
// would force the tx to roll back and the user-visible action to
// fail. The audit log is a best-effort observability surface, not a
// hard correctness gate.
func (s *AuditService) Record(ctx context.Context, tx pgx.Tx, e AuditEntry) {
	if e.OrgID == uuid.Nil || e.Action == "" || e.ResourceType == "" || e.ResourceID == uuid.Nil {
		// Programmer error — log loudly so the developer notices
		// in dev/CI without breaking the user's request.
		s.logger.WarnContext(ctx, "audit.Record: invalid entry skipped",
			"org_id", e.OrgID, "action", e.Action,
			"resource_type", e.ResourceType, "resource_id", e.ResourceID)
		return
	}
	requestID := obs.RequestIDFromContext(ctx)
	err := s.store.InsertAudit(ctx, tx, store.InsertAuditParams{
		OrgID:        e.OrgID,
		UserSub:      e.UserSub,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		ResourceID:   e.ResourceID,
		Before:       e.Before,
		After:        e.After,
		Metadata:     e.Metadata,
		RequestID:    requestID,
	})
	if err != nil {
		s.logger.WarnContext(ctx, "audit.Record: insert failed; tx will still commit the action",
			"org_id", e.OrgID, "action", e.Action,
			"resource_type", e.ResourceType, "resource_id", e.ResourceID,
			"error", err)
	}
}

// NoopAuditRecorder is the test-time stand-in. Implements AuditRecorder
// without touching the database. Production code never wires this;
// it's here so unit tests of services that take an AuditRecorder
// don't need a real DB just to exercise the non-audit logic.
type NoopAuditRecorder struct{}

// Record on NoopAuditRecorder does nothing.
func (NoopAuditRecorder) Record(_ context.Context, _ pgx.Tx, _ AuditEntry) {}

// NewNoopAuditRecorder returns a no-op AuditRecorder.
func NewNoopAuditRecorder() AuditRecorder { return NoopAuditRecorder{} }
