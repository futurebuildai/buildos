package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/brain"
	"github.com/futurebuildai/buildos/internal/store"
)

// MaestroDailyBriefer is the consumer-side interface AgentsService
// needs from the Brain client. Defined here so tests can substitute a
// fake without spinning up an HTTP server, and so AgentsService
// doesn't transitively pin the entire brain.Client surface.
//
// Wraps the typed daily_briefing Maestro task (ADR-001 D5) shipped in
// PR #14. Replaces the earlier free-form MaestroChatter so callers get
// structured cost metadata back on every invocation.
type MaestroDailyBriefer interface {
	DailyBriefing(ctx context.Context, req brain.DailyBriefingRequest) (*brain.DailyBriefingResponse, error)
}

// AgentsService orchestrates Brain Maestro calls for BuildOS-side
// agent endpoints. Today: DailyBriefing. Future: SubLiaison,
// ProcurementNarrative, etc. — same shape, same orchestration.
type AgentsService struct {
	pool       *pgxpool.Pool
	fieldStore *store.FieldStore
	feedStore  *store.FeedCardsStore
	maestro    MaestroDailyBriefer
	audit      AuditRecorder
}

// NewAgentsService wires the dependencies. The maestro client is
// invoked with the caller's Bearer token already in ctx (auth
// middleware stashes it). A nil AuditRecorder is replaced with the
// no-op so tests can omit it.
func NewAgentsService(pool *pgxpool.Pool, fields *store.FieldStore, feed *store.FeedCardsStore, maestro MaestroDailyBriefer, audit AuditRecorder) *AgentsService {
	if audit == nil {
		audit = NoopAuditRecorder{}
	}
	return &AgentsService{pool: pool, fieldStore: fields, feedStore: feed, maestro: maestro, audit: audit}
}

// DailyBriefing is the response from GenerateDailyBriefing — what the
// handler returns and the mobile/web client renders. SessionID is
// passed back so the client can continue the conversation if the user
// asks follow-up questions.
type DailyBriefing struct {
	Reply      string    `json:"reply"`
	SessionID  uuid.UUID `json:"session_id"`
	TaskCount  int       `json:"task_count"`
	AlertCount int       `json:"alert_count"`
}

// GenerateDailyBriefing assembles the caller's open tasks + recent
// active feed cards and dispatches the typed daily_briefing Maestro
// task. Brain owns the prompt template; BuildOS only assembles the
// structured context. The caller's plan tier is enforced by middleware
// (RequirePlanTier), not here — keeping the service free of HTTP
// concerns.
//
// On success, writes a billing-audit row carrying the metered cost
// (run_id, tokens_used, cost_cents, currency_code) — this is the
// per-call meter that rolls into the org's monthly AI usage line per
// ADR-001 D5. The audit write happens AFTER the Maestro call so failed
// invocations don't pollute the bill; if the audit insert itself
// fails, the briefing still returns to the caller (audit is logged at
// WARN by AuditService.Record but not propagated).
//
// The function does NOT persist the briefing as a feed card today;
// that's a follow-up PR once we want push-style delivery. For now the
// frontend caches the response client-side.
func (s *AgentsService) GenerateDailyBriefing(ctx context.Context, callerOrgID uuid.UUID, callerOIDCSubject, callerRole string) (DailyBriefing, error) {
	if callerOrgID == uuid.Nil {
		return DailyBriefing{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	if callerOIDCSubject == "" {
		return DailyBriefing{}, fmt.Errorf("%w: caller oidc subject is required", ErrInvalidInput)
	}

	type loadedContext struct {
		taskNames   []string
		alertTitles []string
	}

	var lc loadedContext
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		userID, err := s.fieldStore.LookupUserIDBySubject(ctx, tx, callerOIDCSubject, callerOrgID)
		if err != nil {
			return err
		}
		tasks, err := s.fieldStore.ListAssignedTasks(ctx, tx, store.ListAssignedTasksParams{
			UserID: userID,
			OrgID:  callerOrgID,
		})
		if err != nil {
			return err
		}
		lc.taskNames = make([]string, 0, len(tasks))
		for _, t := range tasks {
			lc.taskNames = append(lc.taskNames, t.WBSCode+" "+t.Name)
		}

		// Cards: only run the role/sub-targeted query when role is
		// non-empty. ListFeedCards needs both fields to evaluate the
		// targeting predicate.
		if callerRole != "" {
			res, err := s.feedStore.ListFeedCards(ctx, tx, store.ListFeedCardsParams{
				OrgID:             callerOrgID,
				CallerOIDCSubject: callerOIDCSubject,
				CallerRole:        callerRole,
				StatusFilter:      []string{"active"},
				PriorityFilter:    []string{"critical", "urgent"},
				Limit:             20,
			})
			if err != nil {
				return err
			}
			lc.alertTitles = make([]string, 0, len(res.Cards))
			for _, c := range res.Cards {
				lc.alertTitles = append(lc.alertTitles, "["+c.Priority+"] "+c.Title)
			}
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return DailyBriefing{}, ErrNotFound
		}
		return DailyBriefing{}, fmt.Errorf("daily briefing: load context: %w", err)
	}

	// Token plumbing happens via ctx — auth middleware stashed it.
	// Brain's daily_briefing task assembles the prompt from the
	// structured context; BuildOS no longer builds the prompt
	// itself.
	resp, err := s.maestro.DailyBriefing(ctx, brain.DailyBriefingRequest{
		Tasks:    lc.taskNames,
		Alerts:   lc.alertTitles,
		UserRole: callerRole,
	})
	if err != nil {
		return DailyBriefing{}, fmt.Errorf("daily briefing: maestro: %w", err)
	}

	s.recordDailyBriefingAudit(ctx, callerOrgID, callerOIDCSubject, resp, len(lc.taskNames), len(lc.alertTitles))

	return DailyBriefing{
		Reply:      resp.Reply,
		SessionID:  resp.SessionID,
		TaskCount:  len(lc.taskNames),
		AlertCount: len(lc.alertTitles),
	}, nil
}

// recordDailyBriefingAudit writes the per-call billing-audit row in a
// short standalone tx. Resource type is "ai_run" (a Maestro
// invocation, not a domain object); resource id is the Brain-issued
// run_id. Metadata carries the cost_cents / tokens_used block per
// ADR-001 D5. *_cents fields are Confidential-class per the PII
// catalog so the existing audit scrubber preserves them.
//
// A standalone tx is used (rather than wrapping the load-context tx
// or some other surrounding mutation) because daily_briefing is a
// read-only flow with no domain mutation to commit alongside. If the
// tx itself fails to begin, AuditService.Record still falls through
// to its log-and-swallow path; we never fail the user's briefing
// because audit couldn't be written.
func (s *AgentsService) recordDailyBriefingAudit(ctx context.Context, orgID uuid.UUID, userSub string, resp *brain.DailyBriefingResponse, taskCount, alertCount int) {
	metadata, err := json.Marshal(struct {
		RunID        uuid.UUID `json:"run_id"`
		SessionID    uuid.UUID `json:"session_id"`
		TokensUsed   int64     `json:"tokens_used"`
		CostCents    int64     `json:"cost_cents"`
		CurrencyCode string    `json:"currency_code"`
		TaskCount    int       `json:"task_count"`
		AlertCount   int       `json:"alert_count"`
		Task         string    `json:"task"`
	}{
		RunID:        resp.RunID,
		SessionID:    resp.SessionID,
		TokensUsed:   resp.TokensUsed,
		CostCents:    resp.CostCents,
		CurrencyCode: resp.CurrencyCode,
		TaskCount:    taskCount,
		AlertCount:   alertCount,
		Task:         "daily_briefing",
	})
	if err != nil {
		// Marshal failures here are programmer error (the struct
		// is fully composed of marshalable types). Log and skip
		// rather than returning; the briefing already succeeded.
		return
	}

	_ = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        orgID,
			UserSub:      userSub,
			Action:       "ai.daily_briefing.invoked",
			ResourceType: AuditResourceAIRun,
			ResourceID:   resp.RunID,
			Metadata:     metadata,
		})
		return nil
	})
}
