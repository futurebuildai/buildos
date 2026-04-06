package gateway

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ExecutionStatus represents the status of a skill execution.
type ExecutionStatus string

const (
	StatusPending   ExecutionStatus = "PENDING"
	StatusRunning   ExecutionStatus = "RUNNING"
	StatusCompleted ExecutionStatus = "COMPLETED"
	StatusFailed    ExecutionStatus = "FAILED"
)

// ExecutionLog represents a record in shadow_execution_logs.
type ExecutionLog struct {
	ID           uuid.UUID       `json:"id"`
	SkillID      string          `json:"skill_id"`
	ExecutionID  string          `json:"execution_id"`
	Status       ExecutionStatus `json:"status"`
	InputParams  map[string]any  `json:"input_params,omitempty"`
	OutputResult map[string]any  `json:"output_result,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	DurationMs   *int            `json:"duration_ms,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

// Repository provides CRUD operations for shadow_execution_logs.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a new gateway repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// CreateExecutionLog creates a new execution log entry in PENDING state.
func (r *Repository) CreateExecutionLog(ctx context.Context, id uuid.UUID, skillID string, params map[string]any) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO shadow_execution_logs (id, skill_id, execution_id, status, input_params)
		VALUES ($1, $2, $3, 'PENDING', $4)
	`, id, skillID, id.String(), params)
	if err != nil {
		return fmt.Errorf("create execution log: %w", err)
	}
	return nil
}

// MarkRunning transitions an execution log to RUNNING state.
func (r *Repository) MarkRunning(ctx context.Context, id uuid.UUID) error {
	result, err := r.db.Exec(ctx, `
		UPDATE shadow_execution_logs
		SET status = 'RUNNING'
		WHERE id = $1 AND status = 'PENDING'
	`, id)
	if err != nil {
		return fmt.Errorf("mark running: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("execution log %s not found or not in PENDING state", id)
	}
	return nil
}

// UpdateExecutionStatus updates the status and result/error of an execution log.
// Used to mark COMPLETED or FAILED.
func (r *Repository) UpdateExecutionStatus(ctx context.Context, id uuid.UUID, status ExecutionStatus, resultSummary, errorMessage *string, durationMs int) error {
	// Build output_result JSONB from summary if provided
	var outputResult interface{}
	if resultSummary != nil {
		outputResult = map[string]any{"summary": *resultSummary}
	}

	result, err := r.db.Exec(ctx, `
		UPDATE shadow_execution_logs
		SET status = $2, output_result = $3, error_message = $4, duration_ms = $5, completed_at = NOW()
		WHERE id = $1
	`, id, status, outputResult, errorMessage, durationMs)
	if err != nil {
		return fmt.Errorf("update execution status: %w", err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("execution log %s not found", id)
	}
	return nil
}

// GetExecutionLog retrieves an execution log by ID.
func (r *Repository) GetExecutionLog(ctx context.Context, id uuid.UUID) (*ExecutionLog, error) {
	var log ExecutionLog
	err := r.db.QueryRow(ctx, `
		SELECT id, skill_id, execution_id, status, input_params,
		       output_result, error_message, duration_ms, created_at, completed_at
		FROM shadow_execution_logs WHERE id = $1
	`, id).Scan(
		&log.ID, &log.SkillID, &log.ExecutionID, &log.Status, &log.InputParams,
		&log.OutputResult, &log.ErrorMessage, &log.DurationMs, &log.CreatedAt, &log.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("get execution log: %w", err)
	}
	return &log, nil
}

// ListBySkill retrieves all execution logs for a given skill ID.
func (r *Repository) ListBySkill(ctx context.Context, skillID string) ([]ExecutionLog, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, skill_id, execution_id, status, input_params,
		       output_result, error_message, duration_ms, created_at, completed_at
		FROM shadow_execution_logs
		WHERE skill_id = $1
		ORDER BY created_at DESC
	`, skillID)
	if err != nil {
		return nil, fmt.Errorf("list by skill: %w", err)
	}
	defer rows.Close()

	var logs []ExecutionLog
	for rows.Next() {
		var log ExecutionLog
		err := rows.Scan(
			&log.ID, &log.SkillID, &log.ExecutionID, &log.Status, &log.InputParams,
			&log.OutputResult, &log.ErrorMessage, &log.DurationMs, &log.CreatedAt, &log.CompletedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan execution log: %w", err)
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

// GetStatus retrieves just the status of an execution log.
// Useful for idempotency checks.
func (r *Repository) GetStatus(ctx context.Context, id uuid.UUID) (ExecutionStatus, error) {
	var status ExecutionStatus
	err := r.db.QueryRow(ctx, `
		SELECT status FROM shadow_execution_logs WHERE id = $1
	`, id).Scan(&status)
	if err != nil {
		return "", fmt.Errorf("get status: %w", err)
	}
	return status, nil
}
