package agents

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
)

// SubLiaisonAgent handles subcontractor communication:
// - Scans upcoming tasks for unconfirmed subs
// - Generates notification feed cards
// - Processes inbound SMS/email responses using Claude
// - Outbox pattern for failed sends
type SubLiaisonAgent struct {
	pool         *pgxpool.Pool
	feedSvc      *service.FeedService
	logger       *slog.Logger
	claudeRunner *AgentRunner // nil = keyword matching only, set = Claude-powered NLU
}

// NewSubLiaisonAgent creates a new SubLiaisonAgent.
func NewSubLiaisonAgent(pool *pgxpool.Pool, logger *slog.Logger) *SubLiaisonAgent {
	return &SubLiaisonAgent{pool: pool, logger: logger}
}

// WithFeedService sets the feed service for creating notification cards.
func (a *SubLiaisonAgent) WithFeedService(feedSvc *service.FeedService) *SubLiaisonAgent {
	a.feedSvc = feedSvc
	return a
}

// WithClaudeRunner sets the AgentRunner for Claude-powered message understanding.
func (a *SubLiaisonAgent) WithClaudeRunner(runner *AgentRunner) *SubLiaisonAgent {
	a.claudeRunner = runner
	return a
}

// ScanPending scans pending subcontractor approvals and generates notification cards.
// Looks for tasks starting within the next 5 business days that have no confirmed sub.
// NOTE: Not yet invoked from SubLiaisonScanWorker (which uses procurement-item-based scanning).
// Will be wired when task-level subcontractor confirmation tracking is activated.
func (a *SubLiaisonAgent) ScanPending(ctx context.Context) error {
	a.logger.Info("sub_liaison: scanning for unconfirmed subcontractors")

	// Query tasks starting soon without confirmed sub responses
	rows, err := a.pool.Query(ctx, `
		SELECT pt.id, pt.project_id, pt.name, pt.wbs_code, pt.early_start,
		       p.org_id, p.name as project_name
		FROM project_tasks pt
		JOIN projects p ON pt.project_id = p.id
		WHERE pt.status IN ('Pending', 'Ready')
		  AND pt.early_start IS NOT NULL
		  AND pt.early_start <= $1
		  AND pt.early_start > now()
		  AND NOT EXISTS (
		      SELECT 1 FROM communication_logs cl
		      WHERE cl.task_id = pt.id
		        AND cl.status = 'DELIVERED'
		        AND cl.response_parsed = 'confirmed'
		  )
		ORDER BY pt.early_start ASC
		LIMIT 50`,
		time.Now().UTC().Add(5*24*time.Hour))
	if err != nil {
		return fmt.Errorf("querying unconfirmed tasks: %w", err)
	}
	defer rows.Close()

	type pendingTask struct {
		TaskID      uuid.UUID
		ProjectID   uuid.UUID
		TaskName    string
		WBSCode     string
		EarlyStart  *time.Time
		OrgID       uuid.UUID
		ProjectName string
	}

	var tasks []pendingTask
	for rows.Next() {
		var t pendingTask
		if err := rows.Scan(&t.TaskID, &t.ProjectID, &t.TaskName, &t.WBSCode,
			&t.EarlyStart, &t.OrgID, &t.ProjectName); err != nil {
			return fmt.Errorf("scanning pending task: %w", err)
		}
		tasks = append(tasks, t)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if len(tasks) == 0 {
		a.logger.Info("sub_liaison: no unconfirmed tasks found")
		return nil
	}

	// Generate feed cards for unconfirmed tasks
	for _, t := range tasks {
		daysUntil := 0
		if t.EarlyStart != nil {
			daysUntil = int(time.Until(*t.EarlyStart).Hours() / 24)
		}

		priority := models.PriorityNormal
		if daysUntil <= 2 {
			priority = models.PriorityUrgent
		}
		if daysUntil <= 0 {
			priority = models.PriorityCritical
		}

		if a.feedSvc != nil {
			body := fmt.Sprintf("Task %q (%s) starts in %d day(s) — subcontractor has not confirmed. Consider resending confirmation or arranging backup.",
				t.TaskName, t.WBSCode, daysUntil)

			card := &models.FeedCard{
				OrgID:     t.OrgID,
				ProjectID: &t.ProjectID,
				CardType:  models.CardTypeSubConfirmation,
				Title:     fmt.Sprintf("Sub Unconfirmed: %s", t.TaskName),
				Body:      body,
				Priority:  priority,
				Status:    models.FeedStatusActive,
				TaskID:    &t.TaskID,
			}
			agentSource := "SubLiaisonAgent"
			card.AgentSource = &agentSource

			if _, err := a.feedSvc.CreateCard(ctx, card); err != nil {
				a.logger.Error("failed to create sub confirmation card",
					"task_id", t.TaskID, "error", err)
				continue
			}
		}

		a.logger.Info("sub_liaison: unconfirmed sub detected",
			"task_id", t.TaskID,
			"task_name", t.TaskName,
			"days_until_start", daysUntil,
			"priority", priority,
		)
	}

	a.logger.Info("sub_liaison: scan complete", "unconfirmed_count", len(tasks))
	return nil
}

// ProcessInbound handles an inbound message from a subcontractor.
// Uses Claude for NLU when available, falls back to keyword matching.
// NOTE: Not yet invoked from a worker or handler. Will be wired when the inbound
// SMS/email webhook receiver is implemented.
func (a *SubLiaisonAgent) ProcessInbound(ctx context.Context, contactName, contactCompany, taskName string, taskID, projectID, orgID uuid.UUID, messageBody string) error {
	a.logger.Info("sub_liaison: processing inbound message",
		"contact", contactName,
		"task_id", taskID,
		"body_length", len(messageBody),
	)

	// Claude-powered path: nuanced NLU
	if a.claudeRunner != nil {
		err := a.handleInboundWithClaude(ctx, contactName, contactCompany, taskName, taskID, projectID, orgID, messageBody)
		if err != nil {
			a.logger.Warn("claude inbound processing failed, falling back to keyword matching",
				"task_id", taskID, "error", err)
			// Fall through to keyword matching
		} else {
			return nil
		}
	}

	// Fallback: keyword-based parsing
	return a.handleInboundFallback(ctx, contactName, taskName, taskID, projectID, orgID, messageBody)
}

// handleInboundFallback uses keyword matching when Claude is unavailable.
func (a *SubLiaisonAgent) handleInboundFallback(ctx context.Context, contactName, taskName string, taskID, projectID, orgID uuid.UUID, body string) error {
	normalized := normalizeBody(body)

	isConfirmation := containsAny(normalized, "yes", "confirm", "done", "complete", "will be there", "on my way")
	isDelay := containsDelayIndicator(normalized)

	if isConfirmation && a.feedSvc != nil {
		card := &models.FeedCard{
			OrgID:     orgID,
			ProjectID: &projectID,
			CardType:  models.CardTypeSubConfirmation,
			Title:     fmt.Sprintf("Sub Confirmed: %s", taskName),
			Body:      fmt.Sprintf("%s confirmed for task %q", contactName, taskName),
			Priority:  models.PriorityLow,
			Status:    models.FeedStatusActive,
			TaskID:    &taskID,
		}
		agentSource := "SubLiaisonAgent"
		card.AgentSource = &agentSource
		if _, err := a.feedSvc.CreateCard(ctx, card); err != nil {
			a.logger.Error("failed to create confirmation card", "task_id", taskID, "error", err)
		}
	} else if isDelay && a.feedSvc != nil {
		card := &models.FeedCard{
			OrgID:     orgID,
			ProjectID: &projectID,
			CardType:  models.CardTypeSubConfirmation,
			Title:     fmt.Sprintf("Sub Delay: %s", taskName),
			Body:      fmt.Sprintf("%s reported a delay for task %q: %s", contactName, taskName, body),
			Priority:  models.PriorityUrgent,
			Status:    models.FeedStatusActive,
			TaskID:    &taskID,
		}
		agentSource := "SubLiaisonAgent"
		card.AgentSource = &agentSource
		if _, err := a.feedSvc.CreateCard(ctx, card); err != nil {
			a.logger.Error("failed to create delay card", "task_id", taskID, "error", err)
		}
	}

	return nil
}
