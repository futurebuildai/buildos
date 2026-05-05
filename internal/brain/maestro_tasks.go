package brain

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// This file implements the typed Maestro task surface introduced in
// ADR-001 D5. Distinct from maestro.go's Chat (session-bound,
// free-form) — these are discriminated, single-shot AI calls Brain
// routes to specific Anthropic prompts and tools, with cost metering
// rolled into each response so BuildOS can write a billing-audit row
// per call.
//
// Wire shape: POST /v1/ai/tasks with body {task: "<name>", input: ...}
// → response {run_id, tokens_used, cost_cents, currency_code, output: ...}.
// The discriminator field keeps the OpenAPI spec mechanical (one path,
// one schema with oneOf on `task`) and lets Brain add new tasks
// without bumping the BuildOS client.

// CostMetadata is embedded in every typed task response. Each Maestro
// invocation produces a metered run, and BuildOS persists these into
// internal/billing/ audit rows per ADR-001 D5. The cost is already
// markup-applied — Brain charges its own customers downstream.
//
// CurrencyCode is always "USD" or "CAD" (the only two BuildOS
// supports per the Composite Currency Pattern). Cross-currency
// arithmetic is not allowed; aggregations group by CurrencyCode.
type CostMetadata struct {
	RunID        uuid.UUID `json:"run_id"`
	TokensUsed   int64     `json:"tokens_used"`
	CostCents    int64     `json:"cost_cents"`
	CurrencyCode string    `json:"currency_code"`
}

// taskEnvelope is the outer body Brain expects on POST /v1/ai/tasks.
// Task is the discriminator; Input is the task-specific JSON shape
// (its concrete type is determined by Task).
type taskEnvelope struct {
	Task  string          `json:"task"`
	Input json.RawMessage `json:"input"`
}

// taskResult is the outer envelope Brain returns. Output mirrors
// taskEnvelope.Input — its concrete type is determined by the task
// name the caller dispatched. Cost fields are uniform across tasks.
type taskResult struct {
	RunID        uuid.UUID       `json:"run_id"`
	TokensUsed   int64           `json:"tokens_used"`
	CostCents    int64           `json:"cost_cents"`
	CurrencyCode string          `json:"currency_code"`
	Output       json.RawMessage `json:"output"`
}

// runTask is the shared encode/decode pipeline every typed method
// funnels through. Keeps each public method to ~3 lines: build input
// struct, runTask, compose response.
//
// The method takes a pointer to the typed output struct so the
// caller's compiler enforces the right shape — there's no path that
// silently returns a zero value when the discriminator mismatches.
func (m *MaestroClient) runTask(ctx context.Context, task string, input, output any) (*CostMetadata, error) {
	inputRaw, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("brain.Maestro.runTask(%s): marshal input: %w", task, err)
	}
	env := taskEnvelope{Task: task, Input: inputRaw}

	raw, err := m.c.doRequest(ctx, "POST", "/v1/ai/tasks", env)
	if err != nil {
		return nil, err
	}

	var result taskResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("brain.Maestro.runTask(%s): decode result envelope: %w", task, err)
	}
	if len(result.Output) > 0 {
		if err := json.Unmarshal(result.Output, output); err != nil {
			return nil, fmt.Errorf("brain.Maestro.runTask(%s): decode output: %w", task, err)
		}
	}

	return &CostMetadata{
		RunID:        result.RunID,
		TokensUsed:   result.TokensUsed,
		CostCents:    result.CostCents,
		CurrencyCode: result.CurrencyCode,
	}, nil
}

// ---- daily_briefing ----------------------------------------------------

// DailyBriefingRequest is the input payload for the daily_briefing
// task — the morning briefing surface AgentsService.GenerateDailyBriefing
// renders for the field/web home screen. Brain assembles the
// briefing prose from these context fields.
//
// SessionID is optional: when set, Brain attaches the call to an
// existing pre_construction session row so the user can ask follow-up
// questions in the same conversation. Omit for a fresh session.
type DailyBriefingRequest struct {
	SessionID *uuid.UUID `json:"session_id,omitempty"`
	Tasks     []string   `json:"tasks"`
	Alerts    []string   `json:"alerts"`
	UserRole  string     `json:"user_role,omitempty"`
}

// DailyBriefingResponse carries the rendered briefing back to the
// caller. SessionID is always populated (server-assigned when the
// request omitted one).
type DailyBriefingResponse struct {
	CostMetadata
	SessionID uuid.UUID `json:"session_id"`
	Reply     string    `json:"reply"`
}

// DailyBriefing dispatches the daily_briefing Maestro task.
func (m *MaestroClient) DailyBriefing(ctx context.Context, req DailyBriefingRequest) (*DailyBriefingResponse, error) {
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()

	var out struct {
		SessionID uuid.UUID `json:"session_id"`
		Reply     string    `json:"reply"`
	}
	cost, err := m.runTask(ctx, "daily_briefing", req, &out)
	if err != nil {
		return nil, fmt.Errorf("brain.Maestro.DailyBriefing: %w", err)
	}
	return &DailyBriefingResponse{
		CostMetadata: *cost,
		SessionID:    out.SessionID,
		Reply:        out.Reply,
	}, nil
}

// ---- intent_classify --------------------------------------------------

// IntentClassifyRequest is the input for the intent_classify task —
// fed inbound free-form text (SMS from SubLiaison, voice transcript,
// chat message) and returns a structured intent + entity extraction.
type IntentClassifyRequest struct {
	Utterance string `json:"utterance"`
	// Channel is "sms", "voice", "chat", "email" — used by Brain to
	// pick the prompt variant. Optional; defaults to "chat".
	Channel string `json:"channel,omitempty"`
}

// IntentClassifyResponse is the structured classification.
// Confidence is a 0.0–1.0 float; treat <0.6 as ambiguous (caller
// should ask the user to clarify rather than auto-routing).
type IntentClassifyResponse struct {
	CostMetadata
	Intent     string            `json:"intent"`
	Confidence float64           `json:"confidence"`
	Entities   map[string]string `json:"entities,omitempty"`
}

// IntentClassify dispatches the intent_classify Maestro task.
func (m *MaestroClient) IntentClassify(ctx context.Context, req IntentClassifyRequest) (*IntentClassifyResponse, error) {
	if req.Utterance == "" {
		return nil, fmt.Errorf("brain.Maestro.IntentClassify: utterance is required")
	}
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()

	var out struct {
		Intent     string            `json:"intent"`
		Confidence float64           `json:"confidence"`
		Entities   map[string]string `json:"entities,omitempty"`
	}
	cost, err := m.runTask(ctx, "intent_classify", req, &out)
	if err != nil {
		return nil, fmt.Errorf("brain.Maestro.IntentClassify: %w", err)
	}
	return &IntentClassifyResponse{
		CostMetadata: *cost,
		Intent:       out.Intent,
		Confidence:   out.Confidence,
		Entities:     out.Entities,
	}, nil
}

// ---- invoice_extract --------------------------------------------------

// InvoiceExtractRequest is the input for the invoice_extract task.
// Either DocumentURL (a signed URL to a PDF/image in BuildOS S3) or
// raw Text must be set; Brain validates and returns 400 if both empty.
type InvoiceExtractRequest struct {
	DocumentURL string `json:"document_url,omitempty"`
	Text        string `json:"text,omitempty"`
}

// InvoiceExtractLineItem is one row from the parsed invoice.
// AmountCents + CurrencyCode are paired per the Composite Currency
// Pattern; the linter at scripts/lint-migrations.sh enforces the
// invariant in storage, and this shape mirrors it on the wire.
type InvoiceExtractLineItem struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	UnitCents   int64  `json:"unit_cents"`
	AmountCents int64  `json:"amount_cents"`
}

// InvoiceExtractResponse is the parsed invoice. TotalCents is the
// summed amount; LineItems are the per-row breakdown.
type InvoiceExtractResponse struct {
	CostMetadata
	VendorName   string                   `json:"vendor_name"`
	InvoiceNo    string                   `json:"invoice_no"`
	IssuedDate   string                   `json:"issued_date,omitempty"` // YYYY-MM-DD
	TotalCents   int64                    `json:"total_cents"`
	CurrencyCode string                   `json:"invoice_currency_code"` // distinct from CostMetadata.CurrencyCode
	LineItems    []InvoiceExtractLineItem `json:"line_items"`
}

// InvoiceExtract dispatches the invoice_extract Maestro task.
func (m *MaestroClient) InvoiceExtract(ctx context.Context, req InvoiceExtractRequest) (*InvoiceExtractResponse, error) {
	if req.DocumentURL == "" && req.Text == "" {
		return nil, fmt.Errorf("brain.Maestro.InvoiceExtract: document_url or text is required")
	}
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()

	var out struct {
		VendorName   string                   `json:"vendor_name"`
		InvoiceNo    string                   `json:"invoice_no"`
		IssuedDate   string                   `json:"issued_date,omitempty"`
		TotalCents   int64                    `json:"total_cents"`
		CurrencyCode string                   `json:"invoice_currency_code"`
		LineItems    []InvoiceExtractLineItem `json:"line_items"`
	}
	cost, err := m.runTask(ctx, "invoice_extract", req, &out)
	if err != nil {
		return nil, fmt.Errorf("brain.Maestro.InvoiceExtract: %w", err)
	}
	return &InvoiceExtractResponse{
		CostMetadata: *cost,
		VendorName:   out.VendorName,
		InvoiceNo:    out.InvoiceNo,
		IssuedDate:   out.IssuedDate,
		TotalCents:   out.TotalCents,
		CurrencyCode: out.CurrencyCode,
		LineItems:    out.LineItems,
	}, nil
}

// ---- procurement_recommend --------------------------------------------

// ProcurementRecommendRequest is the input for the procurement_recommend
// task. Brain reads the material request, looks up vendor history,
// applies sourcing heuristics, and returns ranked recommendations.
type ProcurementRecommendRequest struct {
	MaterialRequestID uuid.UUID `json:"material_request_id"`
	// BudgetCents is the project's budget remaining for this line —
	// Brain biases recommendations toward staying inside it.
	BudgetCents  int64  `json:"budget_cents,omitempty"`
	CurrencyCode string `json:"currency_code,omitempty"` // USD | CAD
}

// ProcurementVendorRec is one ranked vendor recommendation.
// PredictedSpendCents pairs with CurrencyCode (Composite Currency
// Pattern). Confidence is 0.0–1.0; <0.5 means Brain recommends the PM
// reviews the bid manually before issuing the PO.
type ProcurementVendorRec struct {
	VendorID            uuid.UUID `json:"vendor_id"`
	VendorName          string    `json:"vendor_name"`
	PredictedSpendCents int64     `json:"predicted_spend_cents"`
	CurrencyCode        string    `json:"currency_code"`
	Confidence          float64   `json:"confidence"`
	Reasoning           string    `json:"reasoning,omitempty"`
}

// ProcurementRecommendResponse carries the ranked recommendations.
type ProcurementRecommendResponse struct {
	CostMetadata
	Recommendations []ProcurementVendorRec `json:"recommendations"`
}

// ProcurementRecommend dispatches the procurement_recommend Maestro task.
func (m *MaestroClient) ProcurementRecommend(ctx context.Context, req ProcurementRecommendRequest) (*ProcurementRecommendResponse, error) {
	if req.MaterialRequestID == uuid.Nil {
		return nil, fmt.Errorf("brain.Maestro.ProcurementRecommend: material_request_id is required")
	}
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()

	var out struct {
		Recommendations []ProcurementVendorRec `json:"recommendations"`
	}
	cost, err := m.runTask(ctx, "procurement_recommend", req, &out)
	if err != nil {
		return nil, fmt.Errorf("brain.Maestro.ProcurementRecommend: %w", err)
	}
	return &ProcurementRecommendResponse{
		CostMetadata:    *cost,
		Recommendations: out.Recommendations,
	}, nil
}

// ---- tribunal_review --------------------------------------------------

// TribunalReviewRequest is the input for the tribunal_review task.
// Tribunal is BuildOS's structured-review surface for change orders,
// disputes, and warranty claims. Maestro provides a recommendation;
// the human PM/owner makes the final call (autonomy levels deferred
// per ADR-001 D11).
type TribunalReviewRequest struct {
	DisputeID uuid.UUID `json:"dispute_id"`
	// Facts is the structured fact pattern the tribunal-frontend
	// captured. JSON shape evolves with the UI; Brain treats it as
	// passthrough.
	Facts json.RawMessage `json:"facts"`
}

// TribunalReviewResponse is the LLM's structured recommendation.
// Recommendation is one of "approve", "deny", "escalate" — the
// caller maps it to a dispute-state transition.
type TribunalReviewResponse struct {
	CostMetadata
	Recommendation string  `json:"recommendation"`
	Confidence     float64 `json:"confidence"`
	Rationale      string  `json:"rationale"`
}

// TribunalReview dispatches the tribunal_review Maestro task.
func (m *MaestroClient) TribunalReview(ctx context.Context, req TribunalReviewRequest) (*TribunalReviewResponse, error) {
	if req.DisputeID == uuid.Nil {
		return nil, fmt.Errorf("brain.Maestro.TribunalReview: dispute_id is required")
	}
	if len(req.Facts) == 0 {
		return nil, fmt.Errorf("brain.Maestro.TribunalReview: facts is required")
	}
	ctx, cancel := m.withTimeout(ctx)
	defer cancel()

	var out struct {
		Recommendation string  `json:"recommendation"`
		Confidence     float64 `json:"confidence"`
		Rationale      string  `json:"rationale"`
	}
	cost, err := m.runTask(ctx, "tribunal_review", req, &out)
	if err != nil {
		return nil, fmt.Errorf("brain.Maestro.TribunalReview: %w", err)
	}
	return &TribunalReviewResponse{
		CostMetadata:   *cost,
		Recommendation: out.Recommendation,
		Confidence:     out.Confidence,
		Rationale:      out.Rationale,
	}, nil
}
