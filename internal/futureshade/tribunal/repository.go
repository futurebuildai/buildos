package tribunal

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides data access for Tribunal decisions.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new Tribunal repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// ListDecisions returns paginated tribunal decisions with optional filtering.
func (r *Repository) ListDecisions(ctx context.Context, filter ListDecisionsFilter) (*ListDecisionsResponse, error) {
	// Set defaults
	if filter.Limit <= 0 || filter.Limit > 100 {
		filter.Limit = 20
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	// Build query with conditions — org_id is always required for tenant isolation
	baseQuery := `
		SELECT
			d.id,
			d.case_id,
			d.status,
			d.category,
			d.description,
			d.created_at,
			ARRAY_AGG(DISTINCT v.expert_role) FILTER (WHERE v.expert_role IS NOT NULL) as models_consulted
		FROM tribunal_decisions d
		LEFT JOIN tribunal_votes v ON d.id = v.decision_id
	`

	countQuery := `SELECT COUNT(*) FROM tribunal_decisions d`

	conditions := []string{fmt.Sprintf("d.org_id = $1")}
	args := []interface{}{filter.OrgID}
	argNum := 2

	// Add filters
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("d.status = $%d", argNum))
		args = append(args, filter.Status)
		argNum++
	}

	if filter.Category != "" {
		conditions = append(conditions, fmt.Sprintf("d.category = $%d", argNum))
		args = append(args, filter.Category)
		argNum++
	}

	if filter.StartDate != nil {
		conditions = append(conditions, fmt.Sprintf("d.created_at >= $%d", argNum))
		args = append(args, filter.StartDate)
		argNum++
	}

	if filter.EndDate != nil {
		conditions = append(conditions, fmt.Sprintf("d.created_at <= $%d", argNum))
		args = append(args, filter.EndDate)
		argNum++
	}

	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf("d.description ILIKE $%d", argNum))
		args = append(args, "%"+filter.Search+"%")
		argNum++
	}

	// Build WHERE clause
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count
	var total int
	countQueryFull := countQuery + whereClause
	if err := r.db.QueryRow(ctx, countQueryFull, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count decisions: %w", err)
	}

	// Add GROUP BY, ORDER BY, and pagination
	fullQuery := baseQuery + whereClause + fmt.Sprintf(`
		GROUP BY d.id
		ORDER BY d.created_at DESC
		LIMIT $%d OFFSET $%d
	`, argNum, argNum+1)
	args = append(args, filter.Limit, filter.Offset)

	// Execute query
	rows, err := r.db.Query(ctx, fullQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("query decisions: %w", err)
	}
	defer rows.Close()

	decisions := make([]DecisionSummary, 0)
	for rows.Next() {
		var d DecisionSummary
		var models []string

		if err := rows.Scan(&d.ID, &d.CaseID, &d.Status, &d.Category, &d.Description, &d.Timestamp, &models); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}

		if models == nil {
			d.ModelsConsulted = []string{}
		} else {
			d.ModelsConsulted = models
		}

		decisions = append(decisions, d)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate decisions: %w", err)
	}

	return &ListDecisionsResponse{
		Decisions: decisions,
		Total:     total,
		HasMore:   filter.Offset+len(decisions) < total,
	}, nil
}

// GetDecision returns a single decision with all its votes, scoped by org_id.
func (r *Repository) GetDecision(ctx context.Context, orgID, id uuid.UUID) (*DecisionDetail, error) {
	// Get decision
	decisionQuery := `
		SELECT id, case_id, status, category, description, created_at
		FROM tribunal_decisions
		WHERE id = $1 AND org_id = $2
	`

	var d DecisionDetail
	err := r.db.QueryRow(ctx, decisionQuery, id, orgID).Scan(
		&d.ID, &d.CaseID, &d.Status, &d.Category, &d.Description, &d.Timestamp,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("query decision: %w", err)
	}

	// Get votes
	votesQuery := `
		SELECT id, decision_id, expert_role, vote, reasoning, confidence, model_used, tokens_used, duration_ms
		FROM tribunal_votes
		WHERE decision_id = $1
		ORDER BY expert_role
	`

	rows, err := r.db.Query(ctx, votesQuery, id)
	if err != nil {
		return nil, fmt.Errorf("query votes: %w", err)
	}
	defer rows.Close()

	votes := make([]ModelVote, 0)
	for rows.Next() {
		var v ModelVote
		if err := rows.Scan(&v.ID, &v.DecisionID, &v.ExpertRole, &v.Vote, &v.Reasoning, &v.Confidence, &v.ModelUsed, &v.TokenCount, &v.LatencyMs); err != nil {
			return nil, fmt.Errorf("scan vote: %w", err)
		}
		votes = append(votes, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate votes: %w", err)
	}

	d.Votes = votes

	// Extract policy links from vote reasoning (markdown links to docs/specs)
	d.PolicyLinks = extractPolicyLinks(votes)

	return &d, nil
}

// extractPolicyLinks finds markdown links in vote reasoning that point to docs or specs.
// Pattern: [text](path) where path starts with docs/ or specs/
func extractPolicyLinks(votes []ModelVote) []string {
	linkRegex := regexp.MustCompile(`\[([^\]]+)\]\(((?:docs|specs)/[^)]+)\)`)
	linkSet := make(map[string]bool)

	for _, v := range votes {
		matches := linkRegex.FindAllStringSubmatch(v.Reasoning, -1)
		for _, match := range matches {
			if len(match) >= 3 {
				linkSet[match[2]] = true
			}
		}
	}

	links := make([]string, 0, len(linkSet))
	for link := range linkSet {
		links = append(links, link)
	}
	return links
}

// CreateDecision stores a new Tribunal decision.
func (r *Repository) CreateDecision(ctx context.Context, id uuid.UUID, req TribunalRequest, status DecisionStatus, score float64, summary string) error {
	query := `
		INSERT INTO tribunal_decisions (id, org_id, case_id, category, description, status, reasoning, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
	`
	_, err := r.db.Exec(ctx, query, id, req.OrgID, req.CaseID, req.Category, req.Intent, status, summary)
	if err != nil {
		return fmt.Errorf("create decision: %w", err)
	}
	return nil
}

// CreateVote stores a model's vote.
func (r *Repository) CreateVote(ctx context.Context, v ModelVote) error {
	query := `
		INSERT INTO tribunal_votes (id, decision_id, expert_role, vote, reasoning, confidence, model_used, tokens_used, duration_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	id := uuid.New()
	if v.ID != uuid.Nil {
		id = v.ID
	}

	_, err := r.db.Exec(ctx, query, id, v.DecisionID, v.ExpertRole, v.Vote, v.Reasoning, v.Confidence, v.ModelUsed, v.TokenCount, v.LatencyMs)
	if err != nil {
		return fmt.Errorf("create vote: %w", err)
	}
	return nil
}

// DecisionExistsByCaseID checks if a decision with the given case_id already exists within the org.
// Used for idempotency checks.
func (r *Repository) DecisionExistsByCaseID(ctx context.Context, orgID uuid.UUID, caseID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM tribunal_decisions WHERE case_id = $1 AND org_id = $2)`
	var exists bool
	err := r.db.QueryRow(ctx, query, caseID, orgID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check decision exists: %w", err)
	}
	return exists, nil
}
