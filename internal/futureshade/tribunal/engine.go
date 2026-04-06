package tribunal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

// ConsensusEngine orchestrates the multi-model decision process.
// Uses a 2-role Claude jury: Architect (Opus, antagonist), Reviewer (Sonnet),
// and Coordinator (Sonnet, synthesizer).
type ConsensusEngine struct {
	jury Jury
	repo *Repository
}

// NewConsensusEngine creates a new engine with the given jury and storage.
func NewConsensusEngine(jury Jury, repo *Repository) *ConsensusEngine {
	return &ConsensusEngine{
		jury: jury,
		repo: repo,
	}
}

// Review processes a request through the Tribunal.
// The Architect (Claude Opus) and Reviewer (Claude Sonnet) deliberate in parallel,
// then the Coordinator (Claude Sonnet) synthesizes a final verdict.
//
// FAIL-CLOSED: If JSON parsing of the coordinator response fails, the decision
// defaults to REJECTED. This prevents un-validated approvals.
func (e *ConsensusEngine) Review(ctx context.Context, req TribunalRequest) (*TribunalResponse, error) {
	// Create Decision Record
	decisionID := uuid.New()

	// Parallelize expert opinions
	type opinionResult struct {
		role string
		vote ModelVote
		err  error
	}

	resultChan := make(chan opinionResult, 2)
	var wg sync.WaitGroup

	// The Architect (Claude Opus 4.6 - antagonist)
	wg.Add(1)
	go func() {
		defer wg.Done()
		vote, err := e.consultExpert(ctx, e.jury.Architect, ai.ModelTypeOpus, "The Architect", ArchitectSystemPrompt, req)
		resultChan <- opinionResult{role: "architect", vote: vote, err: err}
	}()

	// The Reviewer (Claude Sonnet 4.5 - institutional memory)
	wg.Add(1)
	go func() {
		defer wg.Done()
		vote, err := e.consultExpert(ctx, e.jury.Reviewer, ai.ModelTypeSonnet, "The Reviewer", ReviewerSystemPrompt, req)
		resultChan <- opinionResult{role: "reviewer", vote: vote, err: err}
	}()

	wg.Wait()
	close(resultChan)

	var votes []ModelVote
	var expertContext strings.Builder

	expertContext.WriteString(fmt.Sprintf("Intent: %s\nContext: %s\n\n", req.Intent, req.Context))

	for res := range resultChan {
		if res.err != nil {
			// Create an ABSTAIN vote for the error case
			votes = append(votes, ModelVote{
				DecisionID: decisionID,
				ExpertRole: res.role,
				Vote:       VoteAbstain,
				Reasoning:  fmt.Sprintf("Error consulting expert: %v", res.err),
			})
			expertContext.WriteString(fmt.Sprintf("Expert %s failed: %v\n\n", res.role, res.err))
		} else {
			res.vote.DecisionID = decisionID
			votes = append(votes, res.vote)
			expertContext.WriteString(fmt.Sprintf("Expert %s voted %s:\n%s\n\n", res.role, res.vote.Vote, res.vote.Reasoning))
		}
	}

	// Coordinator Synthesis (Claude Sonnet 4.5)
	coordinatorReq := ai.NewTextRequest(ai.ModelTypeSonnet,
		fmt.Sprintf("%s\n\n---\n\n%s", CoordinatorSystemPrompt, expertContext.String()))

	coordResp, err := e.jury.Coordinator.GenerateContent(ctx, coordinatorReq)
	if err != nil {
		return nil, fmt.Errorf("coordinator failed: %w", err)
	}

	// Parse Coordinator JSON Output
	var finalVerdict struct {
		Status         DecisionStatus `json:"status"`
		ConsensusScore float64        `json:"consensus_score"`
		Summary        string         `json:"summary"`
		Plan           string         `json:"plan"`
	}

	// Strip markdown code blocks if present (basic sanitization)
	cleanText := strings.TrimPrefix(coordResp.Text, "```json")
	cleanText = strings.TrimSuffix(cleanText, "```")
	cleanText = strings.TrimSpace(cleanText)

	// FAIL-CLOSED: Default to REJECTED if JSON parsing fails.
	// This prevents un-validated approvals from malformed AI output.
	finalVerdict.Status = DecisionRejected
	finalVerdict.Summary = "Coordinator produced invalid JSON or failed to reason. Defaulting to REJECTED."
	finalVerdict.ConsensusScore = 0.0

	if err := json.Unmarshal([]byte(cleanText), &finalVerdict); err != nil {
		// JSON parse failed - defaults above ensure REJECTED status
		slog.Debug("tribunal coordinator JSON parse failure", "error", err, "decision_id", decisionID)
	}

	// Persist Everything
	// 1. Save Decision (skip if repo is nil - for testing)
	if e.repo != nil {
		if err := e.saveDecision(ctx, decisionID, req, finalVerdict.Status, finalVerdict.ConsensusScore, finalVerdict.Summary); err != nil {
			return nil, err
		}

		// 2. Save Votes
		for _, v := range votes {
			if err := e.saveVote(ctx, v); err != nil {
				// Log error but don't fail the whole operation for individual vote save failures
				slog.Debug("tribunal vote save failure", "error", err, "decision_id", decisionID, "expert_role", v.ExpertRole)
			}
		}
	}

	return &TribunalResponse{
		DecisionID:     decisionID,
		Status:         finalVerdict.Status,
		ConsensusScore: finalVerdict.ConsensusScore,
		Summary:        finalVerdict.Summary,
		Plan:           finalVerdict.Plan,
	}, nil
}

// consultExpert sends a request to a single expert and parses the vote.
func (e *ConsensusEngine) consultExpert(ctx context.Context, client ai.Client, model ai.ModelType, name, systemPrompt string, req TribunalRequest) (ModelVote, error) {
	// Sanitize Input (Prompt Injection Vector)
	// Simple sanitization: remove potential delimiter abuse
	cleanIntent := strings.ReplaceAll(req.Intent, "---", " ")
	cleanContext := strings.ReplaceAll(req.Context, "---", " ")

	prompt := fmt.Sprintf("%s\n\nInput Intent: %s\nContext: %s", systemPrompt, cleanIntent, cleanContext)
	aiReq := ai.NewTextRequest(model, prompt)

	// Enforce 30s timeout for individual expert calls
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	start := time.Now()
	resp, err := client.GenerateContent(ctxWithTimeout, aiReq)
	duration := time.Since(start)

	if err != nil {
		return ModelVote{}, err
	}

	// Parse Vote (case insensitive, loose matching)
	vote := VoteAbstain
	normalizedText := strings.ToUpper(resp.Text)
	if strings.Contains(normalizedText, "VOTE: YEA") || strings.Contains(normalizedText, "[VOTE]: YEA") || strings.Contains(normalizedText, "VOTE:YEA") {
		vote = VoteYea
	} else if strings.Contains(normalizedText, "VOTE: NAY") || strings.Contains(normalizedText, "[VOTE]: NAY") || strings.Contains(normalizedText, "VOTE:NAY") {
		vote = VoteNay
	}

	return ModelVote{
		ExpertRole: name,
		Vote:       vote,
		Reasoning:  resp.Text,
		ModelUsed:  string(model),
		LatencyMs:  int(duration.Milliseconds()),
		TokenCount: resp.TokensUsed,
	}, nil
}

func (e *ConsensusEngine) saveDecision(ctx context.Context, id uuid.UUID, req TribunalRequest, status DecisionStatus, score float64, summary string) error {
	return e.repo.CreateDecision(ctx, id, req, status, score, summary)
}

func (e *ConsensusEngine) saveVote(ctx context.Context, v ModelVote) error {
	return e.repo.CreateVote(ctx, v)
}

// Diagnose performs self-healing analysis on a runtime error.
// Uses Claude Sonnet with Temperature=0 for deterministic output.
func (e *ConsensusEngine) Diagnose(ctx context.Context, req DiagnosisRequest) (*DiagnosisResponse, error) {
	sessionID := uuid.New()
	start := time.Now()

	// Build the diagnostic prompt
	var stateStr strings.Builder
	for k, v := range req.SystemState {
		stateStr.WriteString(fmt.Sprintf("- %s: %s\n", k, v))
	}

	prompt := fmt.Sprintf(`%s

---
Error Trace:
%s

Method Context: %s

System State:
%s
---

Output the JSON diagnosis:`, DiagnosticianSystemPrompt, req.ErrorTrace, req.MethodContext, stateStr.String())

	// Use Claude Sonnet with Temperature=0 for determinism
	aiReq := ai.GenerateRequest{
		Model:       ai.ModelTypeSonnet,
		Parts:       []ai.ContentPart{{Text: prompt}},
		Temperature: 0.0, // Deterministic output
	}

	// Enforce timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	resp, err := e.jury.Coordinator.GenerateContent(ctxWithTimeout, aiReq)
	if err != nil {
		return nil, fmt.Errorf("diagnosis failed: %w", err)
	}

	duration := time.Since(start)

	// Parse the JSON response
	cleanText := strings.TrimPrefix(resp.Text, "```json")
	cleanText = strings.TrimSuffix(cleanText, "```")
	cleanText = strings.TrimSpace(cleanText)

	var diagResult struct {
		FaultDiagnosis  string  `json:"fault_diagnosis"`
		ConfidenceScore float64 `json:"confidence_score"`
		Reasoning       string  `json:"reasoning"`
		ProposedAction  struct {
			Type  string `json:"type"`
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"proposed_action"`
	}

	if err := json.Unmarshal([]byte(cleanText), &diagResult); err != nil {
		return nil, fmt.Errorf("failed to parse diagnosis JSON: %w (raw: %s)", err, cleanText)
	}

	// Validate the proposed action type
	if !IsValidActionType(diagResult.ProposedAction.Type) {
		return nil, fmt.Errorf("invalid action type proposed: %s", diagResult.ProposedAction.Type)
	}

	return &DiagnosisResponse{
		Diagnosis:  diagResult.FaultDiagnosis,
		Confidence: diagResult.ConfidenceScore,
		Reasoning:  diagResult.Reasoning,
		ProposedAction: ProposedAction{
			Type:  diagResult.ProposedAction.Type,
			Key:   diagResult.ProposedAction.Key,
			Value: diagResult.ProposedAction.Value,
		},
		SessionID: sessionID,
		LatencyMs: int(duration.Milliseconds()),
		ModelUsed: "claude-sonnet",
	}, nil
}
