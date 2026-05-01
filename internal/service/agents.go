package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/brain"
	"github.com/futurebuildai/buildos/internal/store"
)

// MaestroChatter is the consumer-side interface AgentsService needs
// from the Brain client. Defined here so tests can substitute a fake
// without spinning up an HTTP server, and so AgentsService doesn't
// transitively pin the entire brain.Client surface.
type MaestroChatter interface {
	Chat(ctx context.Context, req brain.ChatRequest) (*brain.ChatResponse, error)
}

// AgentsService orchestrates Brain Maestro calls for BuildOS-side
// agent endpoints. Today: DailyBriefing. Future: SubLiaison,
// ProcurementNarrative, etc. — same shape, same orchestration.
type AgentsService struct {
	pool       *pgxpool.Pool
	fieldStore *store.FieldStore
	feedStore  *store.FeedCardsStore
	maestro    MaestroChatter
}

// NewAgentsService wires the dependencies. The maestro client is
// invoked with the caller's Bearer token already in ctx (auth
// middleware stashes it).
func NewAgentsService(pool *pgxpool.Pool, fields *store.FieldStore, feed *store.FeedCardsStore, maestro MaestroChatter) *AgentsService {
	return &AgentsService{pool: pool, fieldStore: fields, feedStore: feed, maestro: maestro}
}

// DailyBriefing is the response from GenerateDailyBriefing — what the
// handler returns and the mobile/web client renders. SessionID is
// passed back so the client can continue the conversation if the user
// asks follow-up questions.
type DailyBriefing struct {
	Reply       string    `json:"reply"`
	SessionID   uuid.UUID `json:"session_id"`
	TaskCount   int       `json:"task_count"`
	AlertCount  int       `json:"alert_count"`
}

// GenerateDailyBriefing assembles the caller's open tasks + recent
// active feed cards into a Maestro prompt and returns the LLM reply.
// The caller's plan tier is enforced by middleware (RequirePlanTier),
// not here — keeping the service free of HTTP concerns.
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
		taskNames  []string
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

	prompt := buildDailyBriefingPrompt(lc.taskNames, lc.alertTitles)

	// Token plumbing happens via ctx — auth middleware stashed it.
	resp, err := s.maestro.Chat(ctx, brain.ChatRequest{Message: prompt})
	if err != nil {
		return DailyBriefing{}, fmt.Errorf("daily briefing: maestro: %w", err)
	}

	return DailyBriefing{
		Reply:      resp.Reply,
		SessionID:  resp.SessionID,
		TaskCount:  len(lc.taskNames),
		AlertCount: len(lc.alertTitles),
	}, nil
}

// buildDailyBriefingPrompt formats the assembled context into the
// Maestro prompt. Kept as its own function so tests can assert on the
// exact phrasing without spinning up an HTTP server.
//
// Empty task list and empty alert list both produce sensible prompts;
// Maestro replies with "no work scheduled" / "no active alerts" in
// those cases.
func buildDailyBriefingPrompt(tasks, alerts []string) string {
	var b strings.Builder
	b.WriteString("Generate a brief, actionable morning briefing for a residential contractor. ")
	b.WriteString("Lead with the highest-priority alert if any; then summarize today's tasks. ")
	b.WriteString("Keep it under 6 sentences.\n\n")

	if len(alerts) == 0 {
		b.WriteString("Active alerts: (none)\n")
	} else {
		b.WriteString("Active alerts:\n")
		for _, a := range alerts {
			b.WriteString("  - ")
			b.WriteString(a)
			b.WriteByte('\n')
		}
	}

	if len(tasks) == 0 {
		b.WriteString("\nAssigned tasks today: (none)\n")
	} else {
		b.WriteString("\nAssigned tasks today:\n")
		for _, t := range tasks {
			b.WriteString("  - ")
			b.WriteString(t)
			b.WriteByte('\n')
		}
	}

	return b.String()
}
