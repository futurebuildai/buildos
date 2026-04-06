// Package gateway connects Tribunal decisions to skill execution.
// Unlike the reference implementation which used Asynq (Redis), this version
// executes skills synchronously and logs execution results to PostgreSQL.
package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/futureshade"
	"github.com/futurebuild/futurebuild-os/internal/futureshade/skills"
	"github.com/google/uuid"
)

// PlanAction represents a single action in a remediation plan.
type PlanAction struct {
	SkillID string         `json:"skill_id"`
	Params  map[string]any `json:"params"`
}

// RemediationPlan represents a Tribunal-generated plan of actions.
type RemediationPlan struct {
	Actions []PlanAction `json:"actions"`
}

// ExecutionGateway connects Tribunal decisions to skill execution.
// Skills are executed synchronously and results are logged to the database.
type ExecutionGateway struct {
	config   futureshade.Config
	registry *skills.Registry
	repo     *Repository
	logger   *slog.Logger
}

// NewExecutionGateway creates a new execution gateway.
func NewExecutionGateway(
	config futureshade.Config,
	registry *skills.Registry,
	repo *Repository,
	logger *slog.Logger,
) *ExecutionGateway {
	if logger == nil {
		logger = slog.Default()
	}
	return &ExecutionGateway{
		config:   config,
		registry: registry,
		repo:     repo,
		logger:   logger,
	}
}

// ExecutePlan parses a remediation plan JSON and executes skills synchronously.
// Returns an error if any skill_id in the plan is not registered (validation-first).
// All actions are atomically validated before any are executed.
//
// Circuit Breaker: Returns nil immediately if FutureShade is disabled.
func (g *ExecutionGateway) ExecutePlan(ctx context.Context, decisionID uuid.UUID, planJSON []byte) error {
	// Circuit Breaker: Check if FutureShade is enabled
	if !g.config.Enabled {
		g.logger.Info("FutureShade disabled, skipping plan execution",
			"decision_id", decisionID,
		)
		return nil
	}

	// Parse the remediation plan
	var plan RemediationPlan
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return fmt.Errorf("parse remediation plan: %w", err)
	}

	if len(plan.Actions) == 0 {
		g.logger.Debug("empty remediation plan, nothing to execute",
			"decision_id", decisionID,
		)
		return nil
	}

	// Validation-First: Check all skill_ids exist before executing any
	var unknownSkills []string
	for _, action := range plan.Actions {
		if !g.registry.Has(action.SkillID) {
			unknownSkills = append(unknownSkills, action.SkillID)
		}
	}
	if len(unknownSkills) > 0 {
		return fmt.Errorf("unknown skill IDs in plan: %v", unknownSkills)
	}

	// Execute skills and log results
	var executed int
	for _, action := range plan.Actions {
		execID := uuid.New()

		// Create PENDING execution log
		if g.repo != nil {
			if err := g.repo.CreateExecutionLog(ctx, execID, action.SkillID, action.Params); err != nil {
				g.logger.Error("failed to create execution log",
					"decision_id", decisionID,
					"skill_id", action.SkillID,
					"error", err,
				)
				// Continue with other actions - partial success is acceptable
				continue
			}

			// Mark as running
			if err := g.repo.MarkRunning(ctx, execID); err != nil {
				g.logger.Error("failed to mark execution as running",
					"execution_id", execID,
					"error", err,
				)
			}
		}

		// Execute the skill
		skill, _ := g.registry.Get(action.SkillID)
		start := time.Now()
		result, execErr := skill.Execute(ctx, action.Params)
		duration := time.Since(start)

		// Update execution log with result
		if g.repo != nil {
			if execErr != nil {
				errMsg := execErr.Error()
				if updateErr := g.repo.UpdateExecutionStatus(ctx, execID, StatusFailed, nil, &errMsg, int(duration.Milliseconds())); updateErr != nil {
					g.logger.Warn("failed to update execution status to failed",
						"error", updateErr, "execution_id", execID, "skill_id", action.SkillID)
				}
			} else {
				summary := result.Summary
				if updateErr := g.repo.UpdateExecutionStatus(ctx, execID, StatusCompleted, &summary, nil, int(duration.Milliseconds())); updateErr != nil {
					g.logger.Warn("failed to update execution status to completed",
						"error", updateErr, "execution_id", execID, "skill_id", action.SkillID)
				}
			}
		}

		if execErr != nil {
			g.logger.Error("skill execution failed",
				"decision_id", decisionID,
				"execution_id", execID,
				"skill_id", action.SkillID,
				"duration_ms", duration.Milliseconds(),
				"error", execErr,
			)
			continue
		}

		executed++
		g.logger.Debug("skill executed successfully",
			"decision_id", decisionID,
			"execution_id", execID,
			"skill_id", action.SkillID,
			"duration_ms", duration.Milliseconds(),
			"summary", result.Summary,
		)
	}

	g.logger.Info("remediation plan executed",
		"decision_id", decisionID,
		"total_actions", len(plan.Actions),
		"executed", executed,
	)

	return nil
}

// ExecutePlanFromJSON is a convenience method that takes a JSON string.
func (g *ExecutionGateway) ExecutePlanFromJSON(ctx context.Context, decisionID uuid.UUID, planJSON string) error {
	return g.ExecutePlan(ctx, decisionID, []byte(planJSON))
}

// ValidatePlan validates a remediation plan without executing it.
// Useful for pre-flight validation before Tribunal approval.
func (g *ExecutionGateway) ValidatePlan(planJSON []byte) error {
	var plan RemediationPlan
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return fmt.Errorf("parse remediation plan: %w", err)
	}

	var unknownSkills []string
	for _, action := range plan.Actions {
		if !g.registry.Has(action.SkillID) {
			unknownSkills = append(unknownSkills, action.SkillID)
		}
	}
	if len(unknownSkills) > 0 {
		return errors.New("unknown skill IDs: " + fmt.Sprintf("%v", unknownSkills))
	}

	return nil
}
