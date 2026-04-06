package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/service"
)

// BriefingResult holds the output of a daily briefing generation.
type BriefingResult struct {
	ProjectID   uuid.UUID `json:"project_id"`
	ProjectName string    `json:"project_name"`
	Title       string    `json:"title"`
	Bullets     []string  `json:"bullets"`
	Priority    string    `json:"priority"`
}

// DailyFocusAgent generates morning briefing feed cards per project.
type DailyFocusAgent struct {
	pool         *pgxpool.Pool
	feedSvc      *service.FeedService
	logger       *slog.Logger
	claudeRunner *AgentRunner // nil = SQL heuristics only, set = Claude-powered reasoning
}

// NewDailyFocusAgent creates a new DailyFocusAgent.
func NewDailyFocusAgent(pool *pgxpool.Pool, feedSvc *service.FeedService, logger *slog.Logger) *DailyFocusAgent {
	return &DailyFocusAgent{
		pool:    pool,
		feedSvc: feedSvc,
		logger:  logger,
	}
}

// GenerateBriefings creates briefing feed cards for all active projects in an org.
func (a *DailyFocusAgent) GenerateBriefings(ctx context.Context, orgID uuid.UUID) error {
	// Fetch active projects
	rows, err := a.pool.Query(ctx, `
		SELECT id, name, status FROM projects
		WHERE org_id = $1 AND status = 'active'`, orgID)
	if err != nil {
		return fmt.Errorf("querying active projects: %w", err)
	}
	defer rows.Close()

	type project struct {
		ID     uuid.UUID
		Name   string
		Status string
	}
	var projects []project
	for rows.Next() {
		var p project
		if err := rows.Scan(&p.ID, &p.Name, &p.Status); err != nil {
			return fmt.Errorf("scanning project: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, proj := range projects {
		briefing, err := a.generateProjectBriefing(ctx, proj.ID, proj.Name)
		if err != nil {
			a.logger.Error("failed to generate briefing",
				"project_id", proj.ID, "error", err)
			continue
		}

		if err := a.createBriefingCard(ctx, orgID, briefing); err != nil {
			a.logger.Error("failed to create briefing card",
				"project_id", proj.ID, "error", err)
			continue
		}

		a.logger.Info("briefing generated",
			"project_id", proj.ID,
			"title", briefing.Title,
			"priority", briefing.Priority,
		)
	}

	return nil
}

// generateProjectBriefing builds a briefing for a single project.
// When claudeRunner is set, delegates to Claude for intelligent analysis.
// Otherwise falls back to SQL heuristics.
func (a *DailyFocusAgent) generateProjectBriefing(ctx context.Context, projectID uuid.UUID, projectName string) (*BriefingResult, error) {
	// Claude-powered path: richer analysis with tool use
	if a.claudeRunner != nil {
		result, err := a.processProjectWithClaude(ctx, projectID, projectName)
		if err != nil {
			a.logger.Warn("claude briefing failed, falling back to SQL heuristics",
				"project_id", projectID, "error", err)
			// Fall through to SQL heuristics below
		} else {
			return result, nil
		}
	}

	return a.generateProjectBriefingSQL(ctx, projectID, projectName)
}

// generateProjectBriefingSQL builds a briefing using direct SQL queries (fallback path).
func (a *DailyFocusAgent) generateProjectBriefingSQL(ctx context.Context, projectID uuid.UUID, projectName string) (*BriefingResult, error) {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	tomorrow := today.Add(24 * time.Hour)

	// Count today's scheduled tasks
	var todayTaskCount int
	_ = a.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM project_tasks
		WHERE project_id = $1
			AND scheduled_start >= $2 AND scheduled_start < $3`,
		projectID, today, tomorrow).Scan(&todayTaskCount)

	// Count overdue tasks
	var overdueCount int
	_ = a.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM project_tasks
		WHERE project_id = $1
			AND scheduled_end < $2
			AND status NOT IN ('completed', 'cancelled')`,
		projectID, today).Scan(&overdueCount)

	// Check critical procurement items
	var criticalProcCount int
	_ = a.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM procurement_items
		WHERE project_id = $1 AND status = 'CRITICAL'`,
		projectID).Scan(&criticalProcCount)

	// Build briefing
	bullets := []string{
		fmt.Sprintf("%d tasks scheduled for today", todayTaskCount),
	}
	priority := models.PriorityNormal

	if overdueCount > 0 {
		bullets = append(bullets, fmt.Sprintf("%d overdue tasks need attention", overdueCount))
		priority = models.PriorityUrgent
	}

	if criticalProcCount > 0 {
		bullets = append(bullets, fmt.Sprintf("%d critical procurement items require immediate ordering", criticalProcCount))
		priority = models.PriorityCritical
	}

	if overdueCount == 0 && criticalProcCount == 0 {
		bullets = append(bullets, "All on track — no blockers detected")
	}

	title := fmt.Sprintf("Daily Briefing: %s", projectName)

	return &BriefingResult{
		ProjectID:   projectID,
		ProjectName: projectName,
		Title:       title,
		Bullets:     bullets,
		Priority:    priority,
	}, nil
}

// createBriefingCard creates a feed card from a briefing result.
func (a *DailyFocusAgent) createBriefingCard(ctx context.Context, orgID uuid.UUID, briefing *BriefingResult) error {
	body := strings.Join(briefing.Bullets, "\n• ")
	if len(briefing.Bullets) > 0 {
		body = "• " + body
	}

	actionsJSON, _ := json.Marshal([]map[string]string{
		{"label": "View Project", "action_type": "navigate", "payload": briefing.ProjectID.String()},
	})

	card := &models.FeedCard{
		OrgID:    orgID,
		ProjectID: &briefing.ProjectID,
		CardType: models.CardTypeBriefing,
		Title:    briefing.Title,
		Body:     body,
		Priority: briefing.Priority,
		Actions:  actionsJSON,
		Status:   models.FeedStatusActive,
	}

	_, err := a.feedSvc.CreateCard(ctx, card)
	return err
}
