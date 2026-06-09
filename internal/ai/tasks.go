package ai

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// This file implements the typed task surface for native AI calls.
// Each method is a single-shot, discriminated AI call dispatched
// natively to Anthropic. The request/response struct shapes carry
// only domain data — no cost/token/usage metadata, which was a
// billing concern and is intentionally absent in the standalone
// deployment.
//
// Model selection per task:
//   - DailyBriefing, IntentClassify → FastModel (cheap classification /
//     prose).
//   - InvoiceExtract, ProcurementRecommend, TribunalReview,
//     UpdateSchedule, DelayCascadeReason → Model (Opus; heavier
//     reasoning).
//
// Each method returns ErrUnconfigured when no Anthropic key is available
// for the org (via the KeyResolver). The org id is read from the context
// (ContextWithOrgID).

// ---- daily_briefing ----------------------------------------------------

// DailyBriefingRequest is the input payload for the daily briefing — the
// morning briefing surface the field/web home screen renders.
//
// SessionID is preserved for shape-compatibility with the prior service
// contract; the native client is single-shot (no server-side session),
// so it is echoed back unchanged on the response.
type DailyBriefingRequest struct {
	SessionID *uuid.UUID `json:"session_id,omitempty"`
	Tasks     []string   `json:"tasks"`
	Alerts    []string   `json:"alerts"`
	UserRole  string     `json:"user_role,omitempty"`
}

// DailyBriefingResponse carries the rendered briefing back to the caller.
type DailyBriefingResponse struct {
	SessionID uuid.UUID `json:"session_id"`
	Reply     string    `json:"reply"`
}

const dailyBriefingSystem = `You are the BuildOS daily briefing assistant for a residential construction team. ` +
	`Given today's tasks and active alerts, write a concise, prioritized morning briefing in plain prose. ` +
	`Lead with the most urgent alerts, then the day's tasks. Keep it under 200 words. Address the reader by their role when provided.`

// DailyBriefing generates the morning briefing prose. Uses FastModel.
func (c *Client) DailyBriefing(ctx context.Context, req DailyBriefingRequest) (*DailyBriefingResponse, error) {
	prompt, err := json.Marshal(struct {
		Tasks    []string `json:"tasks"`
		Alerts   []string `json:"alerts"`
		UserRole string   `json:"user_role,omitempty"`
	}{Tasks: req.Tasks, Alerts: req.Alerts, UserRole: req.UserRole})
	if err != nil {
		return nil, fmt.Errorf("ai.DailyBriefing: marshal prompt: %w", err)
	}

	reply, err := c.callText(ctx, "daily_briefing", c.fastModel, dailyBriefingSystem, []contentBlock{
		textBlock("Generate the daily briefing for this context:\n" + string(prompt)),
	})
	if err != nil {
		return nil, fmt.Errorf("ai.DailyBriefing: %w", err)
	}

	// Single-shot: echo the caller's session id (or mint one) so the
	// response shape matches the prior contract.
	sid := uuid.New()
	if req.SessionID != nil {
		sid = *req.SessionID
	}
	return &DailyBriefingResponse{SessionID: sid, Reply: reply}, nil
}

// ---- intent_classify --------------------------------------------------

// IntentClassifyRequest is the input for intent classification — fed
// inbound free-form text and returns a structured intent + entities.
type IntentClassifyRequest struct {
	Utterance string `json:"utterance"`
	// Channel is "sms", "voice", "chat", "email". Optional; defaults to
	// "chat".
	Channel string `json:"channel,omitempty"`
}

// IntentClassifyResponse is the structured classification. Confidence is
// 0.0–1.0; treat <0.6 as ambiguous.
type IntentClassifyResponse struct {
	Intent     string            `json:"intent"`
	Confidence float64           `json:"confidence"`
	Entities   map[string]string `json:"entities,omitempty"`
}

const intentClassifySystem = `You classify inbound construction-domain messages into a structured intent. ` +
	`Return the intent label, a 0.0-1.0 confidence, and any extracted entities as string key/value pairs.`

var intentClassifySchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "intent": {"type": "string", "description": "the classified intent label"},
    "confidence": {"type": "number", "description": "0.0 to 1.0 confidence"},
    "entities": {"type": "object", "additionalProperties": {"type": "string"}, "description": "extracted entities"}
  },
  "required": ["intent", "confidence"]
}`)

// IntentClassify classifies an utterance. Tool call, uses FastModel.
func (c *Client) IntentClassify(ctx context.Context, req IntentClassifyRequest) (*IntentClassifyResponse, error) {
	if req.Utterance == "" {
		return nil, fmt.Errorf("ai.IntentClassify: utterance is required")
	}
	channel := req.Channel
	if channel == "" {
		channel = "chat"
	}

	raw, err := c.callTool(ctx, "intent_classify", c.fastModel, intentClassifySystem,
		[]contentBlock{textBlock(fmt.Sprintf("Channel: %s\nMessage: %s", channel, req.Utterance))},
		"classify_intent", intentClassifySchema)
	if err != nil {
		return nil, fmt.Errorf("ai.IntentClassify: %w", err)
	}

	var out IntentClassifyResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai.IntentClassify: decode tool output: %w", err)
	}
	return &out, nil
}

// ---- invoice_extract --------------------------------------------------

// InvoiceExtractRequest is the input for invoice extraction. Either
// DocumentURL (a signed URL to a PDF/image) or raw Text must be set.
type InvoiceExtractRequest struct {
	DocumentURL string `json:"document_url,omitempty"`
	Text        string `json:"text,omitempty"`
}

// InvoiceExtractLineItem is one row from the parsed invoice. AmountCents
// + the response-level CurrencyCode are paired per the Composite
// Currency Pattern.
type InvoiceExtractLineItem struct {
	Description string `json:"description"`
	Quantity    int    `json:"quantity"`
	UnitCents   int64  `json:"unit_cents"`
	AmountCents int64  `json:"amount_cents"`
}

// InvoiceExtractResponse is the parsed invoice.
type InvoiceExtractResponse struct {
	VendorName   string                   `json:"vendor_name"`
	InvoiceNo    string                   `json:"invoice_no"`
	IssuedDate   string                   `json:"issued_date,omitempty"` // YYYY-MM-DD
	TotalCents   int64                    `json:"total_cents"`
	CurrencyCode string                   `json:"invoice_currency_code"`
	LineItems    []InvoiceExtractLineItem `json:"line_items"`
}

const invoiceExtractSystem = `You extract structured invoice data from a vendor invoice (image or text). ` +
	`All monetary values must be returned as integer cents (no floats). ` +
	`currency_code must be a 3-letter ISO code (USD or CAD). ` +
	`issued_date must be YYYY-MM-DD. Return one line item per row.`

var invoiceExtractSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "vendor_name": {"type": "string"},
    "invoice_no": {"type": "string"},
    "issued_date": {"type": "string", "description": "YYYY-MM-DD"},
    "total_cents": {"type": "integer", "description": "total in integer cents"},
    "invoice_currency_code": {"type": "string", "description": "USD or CAD"},
    "line_items": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "description": {"type": "string"},
          "quantity": {"type": "integer"},
          "unit_cents": {"type": "integer"},
          "amount_cents": {"type": "integer"}
        },
        "required": ["description", "amount_cents"]
      }
    }
  },
  "required": ["vendor_name", "invoice_no", "total_cents", "invoice_currency_code", "line_items"]
}`)

// InvoiceExtract extracts structured invoice data. Tool call, uses Model
// (Opus). When DocumentURL is set, the document is fetched and included
// as an image content block; otherwise the Text falls back to a text
// block.
func (c *Client) InvoiceExtract(ctx context.Context, req InvoiceExtractRequest) (*InvoiceExtractResponse, error) {
	if req.DocumentURL == "" && req.Text == "" {
		return nil, fmt.Errorf("ai.InvoiceExtract: document_url or text is required")
	}

	var userContent []contentBlock
	if req.DocumentURL != "" {
		mediaType, b64, err := c.fetchDocumentImage(ctx, req.DocumentURL)
		if err != nil {
			return nil, fmt.Errorf("ai.InvoiceExtract: %w", err)
		}
		userContent = []contentBlock{
			textBlock("Extract the invoice data from the attached document image."),
			{Type: "image", Source: &imageSource{Type: "base64", MediaType: mediaType, Data: b64}},
		}
	} else {
		userContent = []contentBlock{
			textBlock("Extract the invoice data from the following text:\n" + req.Text),
		}
	}

	raw, err := c.callTool(ctx, "invoice_extract", c.model, invoiceExtractSystem,
		userContent, "extract_invoice", invoiceExtractSchema)
	if err != nil {
		return nil, fmt.Errorf("ai.InvoiceExtract: %w", err)
	}

	var out InvoiceExtractResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai.InvoiceExtract: decode tool output: %w", err)
	}
	return &out, nil
}

// ---- procurement_recommend --------------------------------------------

// ProcurementRecommendRequest is the input for procurement
// recommendations.
type ProcurementRecommendRequest struct {
	MaterialRequestID uuid.UUID `json:"material_request_id"`
	BudgetCents       int64     `json:"budget_cents,omitempty"`
	CurrencyCode      string    `json:"currency_code,omitempty"` // USD | CAD
}

// ProcurementVendorRec is one ranked vendor recommendation.
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
	Recommendations []ProcurementVendorRec `json:"recommendations"`
}

const procurementRecommendSystem = `You recommend vendors for a construction material request. ` +
	`Return ranked recommendations with predicted spend in integer cents, a 3-letter currency code, ` +
	`a 0.0-1.0 confidence, and brief reasoning. Bias toward staying within the provided budget.`

var procurementRecommendSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "recommendations": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "vendor_id": {"type": "string", "description": "vendor UUID"},
          "vendor_name": {"type": "string"},
          "predicted_spend_cents": {"type": "integer"},
          "currency_code": {"type": "string", "description": "USD or CAD"},
          "confidence": {"type": "number"},
          "reasoning": {"type": "string"}
        },
        "required": ["vendor_id", "vendor_name", "predicted_spend_cents", "currency_code", "confidence"]
      }
    }
  },
  "required": ["recommendations"]
}`)

// ProcurementRecommend ranks vendor recommendations. Tool call, uses
// Model (Opus).
func (c *Client) ProcurementRecommend(ctx context.Context, req ProcurementRecommendRequest) (*ProcurementRecommendResponse, error) {
	if req.MaterialRequestID == uuid.Nil {
		return nil, fmt.Errorf("ai.ProcurementRecommend: material_request_id is required")
	}

	prompt, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai.ProcurementRecommend: marshal prompt: %w", err)
	}

	raw, err := c.callTool(ctx, "procurement_recommend", c.model, procurementRecommendSystem,
		[]contentBlock{textBlock("Recommend vendors for this material request:\n" + string(prompt))},
		"recommend_vendors", procurementRecommendSchema)
	if err != nil {
		return nil, fmt.Errorf("ai.ProcurementRecommend: %w", err)
	}

	var out ProcurementRecommendResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai.ProcurementRecommend: decode tool output: %w", err)
	}
	return &out, nil
}

// ---- tribunal_review --------------------------------------------------

// TribunalReviewRequest is the input for tribunal review of change
// orders, disputes, and warranty claims.
type TribunalReviewRequest struct {
	DisputeID uuid.UUID       `json:"dispute_id"`
	Facts     json.RawMessage `json:"facts"`
}

// TribunalReviewResponse is the structured recommendation.
// Recommendation is one of "approve", "deny", "escalate".
type TribunalReviewResponse struct {
	Recommendation string  `json:"recommendation"`
	Confidence     float64 `json:"confidence"`
	Rationale      string  `json:"rationale"`
}

const tribunalReviewSystem = `You review a construction dispute / change order / warranty claim and recommend a disposition. ` +
	`recommendation must be exactly one of "approve", "deny", or "escalate". ` +
	`Provide a 0.0-1.0 confidence and a concise rationale. The human PM/owner makes the final call.`

var tribunalReviewSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "recommendation": {"type": "string", "enum": ["approve", "deny", "escalate"]},
    "confidence": {"type": "number"},
    "rationale": {"type": "string"}
  },
  "required": ["recommendation", "confidence", "rationale"]
}`)

// TribunalReview produces a structured review recommendation. Tool call,
// uses Model (Opus).
func (c *Client) TribunalReview(ctx context.Context, req TribunalReviewRequest) (*TribunalReviewResponse, error) {
	if req.DisputeID == uuid.Nil {
		return nil, fmt.Errorf("ai.TribunalReview: dispute_id is required")
	}
	if len(req.Facts) == 0 {
		return nil, fmt.Errorf("ai.TribunalReview: facts is required")
	}

	raw, err := c.callTool(ctx, "tribunal_review", c.model, tribunalReviewSystem,
		[]contentBlock{textBlock("Review this dispute fact pattern:\n" + string(req.Facts))},
		"review_dispute", tribunalReviewSchema)
	if err != nil {
		return nil, fmt.Errorf("ai.TribunalReview: %w", err)
	}

	var out TribunalReviewResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai.TribunalReview: decode tool output: %w", err)
	}
	return &out, nil
}

// ---- update_schedule --------------------------------------------------

// ScheduleTaskSnapshot is the per-task projection sent to the model.
type ScheduleTaskSnapshot struct {
	TaskID          uuid.UUID `json:"task_id"`
	WBSCode         string    `json:"wbs_code"`
	Name            string    `json:"name"`
	DurationDays    int       `json:"duration_days"`
	Status          string    `json:"status"`
	PercentComplete int       `json:"percent_complete"`
	IsCritical      bool      `json:"is_critical"`
}

// ScheduleDepSnapshot is the per-dependency projection. DependencyType
// is the wire form ("FS", "SS", "FF", "SF").
type ScheduleDepSnapshot struct {
	PredecessorID  uuid.UUID `json:"predecessor_id"`
	SuccessorID    uuid.UUID `json:"successor_id"`
	DependencyType string    `json:"dependency_type"`
	LagDays        int       `json:"lag_days"`
}

// UpdateScheduleRequest is the input for schedule update recommendations.
// The model recommends duration adjustments; BuildOS owns the CPM engine
// and re-validates every recommendation.
type UpdateScheduleRequest struct {
	ProjectID        uuid.UUID              `json:"project_id"`
	ProjectStartDate string                 `json:"project_start_date,omitempty"` // RFC3339
	Tasks            []ScheduleTaskSnapshot `json:"tasks"`
	Dependencies     []ScheduleDepSnapshot  `json:"dependencies"`
}

// ScheduleAdjustment is one recommended duration change. NewDurationDays
// is a pointer so the model can omit it for a "no change / monitor only"
// row while still attaching a rationale.
type ScheduleAdjustment struct {
	TaskID          uuid.UUID `json:"task_id"`
	NewDurationDays *int      `json:"new_duration_days,omitempty"`
	Rationale       string    `json:"rationale,omitempty"`
}

// UpdateScheduleResponse carries the recommended adjustments.
type UpdateScheduleResponse struct {
	Adjustments []ScheduleAdjustment `json:"adjustments"`
}

const updateScheduleSystem = `You recommend duration adjustments for construction project tasks based on the schedule snapshot. ` +
	`Return a list of adjustments. For each, include the task_id, an optional new_duration_days (omit for monitor-only rows), ` +
	`and a concise rationale. Do not recommend extending tasks already 100% complete. ` +
	`You only recommend — the CPM engine re-validates every change.`

var updateScheduleSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "adjustments": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "task_id": {"type": "string", "description": "task UUID"},
          "new_duration_days": {"type": "integer", "description": "omit for monitor-only"},
          "rationale": {"type": "string"}
        },
        "required": ["task_id"]
      }
    }
  },
  "required": ["adjustments"]
}`)

// UpdateSchedule recommends schedule duration adjustments. Tool call,
// uses Model (Opus).
func (c *Client) UpdateSchedule(ctx context.Context, req UpdateScheduleRequest) (*UpdateScheduleResponse, error) {
	if req.ProjectID == uuid.Nil {
		return nil, fmt.Errorf("ai.UpdateSchedule: project_id is required")
	}
	if len(req.Tasks) == 0 {
		return nil, fmt.Errorf("ai.UpdateSchedule: at least one task is required")
	}

	prompt, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai.UpdateSchedule: marshal prompt: %w", err)
	}

	raw, err := c.callTool(ctx, "update_schedule", c.model, updateScheduleSystem,
		[]contentBlock{textBlock("Recommend schedule adjustments for this project:\n" + string(prompt))},
		"recommend_adjustments", updateScheduleSchema)
	if err != nil {
		return nil, fmt.Errorf("ai.UpdateSchedule: %w", err)
	}

	var out UpdateScheduleResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai.UpdateSchedule: decode tool output: %w", err)
	}
	return &out, nil
}

// ---- delay_cascade -----------------------------------------------------

// DelayCascadeSlippedTask is one schedule task whose dates moved after a
// CPM recompute. EarlyFinish / LateFinish are wire-form date strings
// (the caller decides the layout; typically YYYY-MM-DD). FloatDays is the
// remaining total float in whole days; a critical task has float 0.
type DelayCascadeSlippedTask struct {
	WBS         string `json:"wbs"`
	Name        string `json:"name"`
	EarlyFinish string `json:"early_finish"`
	LateFinish  string `json:"late_finish"`
	FloatDays   int    `json:"float_days"`
	IsCritical  bool   `json:"is_critical"`
}

// DelayCascadeProcurement is one procurement line in the slipped project's
// orbit. WBS is the cost-code key that lets the model correlate the line to a
// slipped task / budget line. LeadTimeDays + MustOrderBy give the model the
// ordering pressure; MustOrderBy is a wire-form date string (may be empty when
// unknown).
type DelayCascadeProcurement struct {
	WBS          string `json:"wbs"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	LeadTimeDays int    `json:"lead_time_days,omitempty"`
	MustOrderBy  string `json:"must_order_by,omitempty"`
}

// DelayCascadeBudget is one cost-coded budget line. All monetary values
// are integer cents paired with CurrencyCode per the Composite Currency
// Pattern — never floats.
type DelayCascadeBudget struct {
	WBS            string `json:"wbs"`
	EstimatedCents int64  `json:"estimated_cents"`
	CommittedCents int64  `json:"committed_cents"`
	ActualCents    int64  `json:"actual_cents"`
	CurrencyCode   string `json:"currency_code"`
}

// DelayCascadeReasonRequest is the input for cross-module delay-cascade
// reasoning. The CPM engine has already recomputed the schedule and
// identified the slipped tasks; the model reasons about the downstream
// blast radius across procurement, crew, and budget. It never recomputes
// the schedule or the money — those stay with the deterministic engine.
type DelayCascadeReasonRequest struct {
	ProjectName  string                    `json:"project_name"`
	SlippedTasks []DelayCascadeSlippedTask `json:"slipped_tasks"`
	Procurement  []DelayCascadeProcurement `json:"procurement,omitempty"`
	Budget       []DelayCascadeBudget      `json:"budget,omitempty"`
}

// CascadeImpact is one downstream impact the model surfaces. Module is
// one of "schedule", "procurement", "crew", "budget". Severity is one of
// "critical", "high", "normal", "low". Title/Body render as a feed card;
// RecommendedAction is the suggested human next step.
type CascadeImpact struct {
	Module            string `json:"module"`
	Severity          string `json:"severity"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	RecommendedAction string `json:"recommended_action"`
}

// DelayCascadeReasonResponse carries the ranked cross-module impacts.
type DelayCascadeReasonResponse struct {
	Impacts []CascadeImpact `json:"impacts"`
}

const delayCascadeSystem = `You assess the cross-module blast radius of a schedule slip on a residential construction project. ` +
	`The CPM engine has already recomputed the schedule and identified the slipped tasks; you do NOT recompute schedule dates or any monetary totals. ` +
	`Reason about downstream impacts across four modules: "schedule", "procurement", "crew", and "budget". ` +
	`For each impact return the module, a severity of exactly one of "critical", "high", "normal", or "low", a short title, a concise body, and a recommended human next step. ` +
	`Prioritize critical-path slips and procurement lines whose ordering window the slip puts at risk. Surface only material impacts — do not invent ones that aren't supported by the context.`

var delayCascadeSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "impacts": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "module": {"type": "string", "enum": ["schedule", "procurement", "crew", "budget"]},
          "severity": {"type": "string", "enum": ["critical", "high", "normal", "low"]},
          "title": {"type": "string"},
          "body": {"type": "string"},
          "recommended_action": {"type": "string"}
        },
        "required": ["module", "severity", "title", "body", "recommended_action"]
      }
    }
  },
  "required": ["impacts"]
}`)

// DelayCascadeReason assesses the cross-module impact of a schedule slip.
// Tool call, uses Model (Opus). Inherits ErrUnconfigured from callTool
// when no Anthropic key is configured for the org.
func (c *Client) DelayCascadeReason(ctx context.Context, req DelayCascadeReasonRequest) (*DelayCascadeReasonResponse, error) {
	if len(req.SlippedTasks) == 0 {
		return nil, fmt.Errorf("ai.DelayCascadeReason: at least one slipped task is required")
	}

	prompt, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai.DelayCascadeReason: marshal prompt: %w", err)
	}

	raw, err := c.callTool(ctx, "delay_cascade", c.model, delayCascadeSystem,
		[]contentBlock{textBlock("Assess the cross-module impact of this schedule slip:\n" + string(prompt))},
		"assess_delay_cascade", delayCascadeSchema)
	if err != nil {
		return nil, fmt.Errorf("ai.DelayCascadeReason: %w", err)
	}

	var out DelayCascadeReasonResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai.DelayCascadeReason: decode tool output: %w", err)
	}
	return &out, nil
}

// ---- foresight_risk ----------------------------------------------------

// ForesightProcurementRisk is one procurement line the deterministic engine
// has flagged as ordering-window-at-risk. Status is READ from the DB (set by
// procurement_check) — the model does NOT re-derive it. DaysUntilMustOrder is
// the engine-computed integer days until the ordering window closes.
type ForesightProcurementRisk struct {
	WBS                string `json:"wbs"`
	Description        string `json:"description"`
	Status             string `json:"status"`
	DaysUntilMustOrder int    `json:"days_until_must_order"`
}

// ForesightScheduleRisk is one schedule task the engine has flagged as
// slip-prone. RemainingFloatDays + IsCritical + PercentComplete are all
// CPM/engine-computed; the model never recomputes a schedule date or float.
type ForesightScheduleRisk struct {
	WBS                string `json:"wbs"`
	Name               string `json:"name"`
	RemainingFloatDays int    `json:"remaining_float_days"`
	IsCritical         bool   `json:"is_critical"`
	PercentComplete    int    `json:"percent_complete"`
}

// ForesightBudgetRisk is one cost-coded budget line trending over estimate.
// All monetary values are integer cents paired with CurrencyCode per the
// Composite Currency Pattern — never floats. BurnPercent is the engine-computed
// integer percent (ActualCents*100/EstimatedCents); the model never recomputes it.
type ForesightBudgetRisk struct {
	WBS            string `json:"wbs"`
	EstimatedCents int64  `json:"estimated_cents"`
	CommittedCents int64  `json:"committed_cents"`
	ActualCents    int64  `json:"actual_cents"`
	CurrencyCode   string `json:"currency_code"`
	BurnPercent    int    `json:"burn_percent"`
}

// ForesightRiskRequest is the input for the cross-module foresight risk
// judgment. The deterministic engine has ALREADY computed every metric across
// the three dimensions; the model judges materiality/severity/phrasing only and
// never recomputes a schedule date or a monetary total.
type ForesightRiskRequest struct {
	ProjectName string                     `json:"project_name"`
	Procurement []ForesightProcurementRisk `json:"procurement,omitempty"`
	Schedule    []ForesightScheduleRisk    `json:"schedule,omitempty"`
	Budget      []ForesightBudgetRisk      `json:"budget,omitempty"`
}

// ForesightRiskItem is one judged, materiality-ranked risk. RiskType anchors
// the dedup subject and is exactly one of "procurement_criticality",
// "schedule_slip", or "budget_burn". Severity is one of "critical", "high",
// "normal", "low". WBS is the dedup subject code. Title/Body render as a feed
// card; RecommendedAction is the suggested human next step. The response schema
// carries NO numeric metric fields — those stay with the deterministic engine.
type ForesightRiskItem struct {
	RiskType          string `json:"risk_type"`
	WBS               string `json:"wbs"`
	Severity          string `json:"severity"`
	Title             string `json:"title"`
	Body              string `json:"body"`
	RecommendedAction string `json:"recommended_action"`
}

// ForesightRiskResponse carries the materiality-ranked risks.
type ForesightRiskResponse struct {
	Risks []ForesightRiskItem `json:"risks"`
}

const foresightRiskSystem = `You judge the materiality of standing cross-module risks on a residential construction project. ` +
	`You receive three categories the deterministic engine has ALREADY computed: procurement criticality (items whose ordering window the schedule puts at risk), ` +
	`schedule-slip risk (critical-path or low-float tasks), and budget burn (lines trending over estimate). ` +
	`You NEVER recompute a schedule date or a monetary total — every date, float day, and dollar figure (integer cents) is given. ` +
	`For each material risk return risk_type (exactly one of "procurement_criticality", "schedule_slip", or "budget_burn"), wbs, severity ` +
	`(exactly one of "critical", "high", "normal", or "low"), a short title, a concise body, and a recommended human next step. ` +
	`OMIT immaterial or well-managed risks — do not surface a card for every breach.`

var foresightRiskSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "risks": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "risk_type": {"type": "string", "enum": ["procurement_criticality", "schedule_slip", "budget_burn"]},
          "wbs": {"type": "string"},
          "severity": {"type": "string", "enum": ["critical", "high", "normal", "low"]},
          "title": {"type": "string"},
          "body": {"type": "string"},
          "recommended_action": {"type": "string"}
        },
        "required": ["risk_type", "wbs", "severity", "title", "body", "recommended_action"]
      }
    }
  },
  "required": ["risks"]
}`)

// ForesightRiskJudgment judges the materiality of computed cross-module risks.
// Tool call, uses Model (Opus). Inherits ErrUnconfigured from callTool when no
// Anthropic key is configured for the org. Returns an error if all three input
// arrays are empty (the orchestrator's material-signal gate guarantees they
// aren't, but we defend it here).
func (c *Client) ForesightRiskJudgment(ctx context.Context, req ForesightRiskRequest) (*ForesightRiskResponse, error) {
	if len(req.Procurement) == 0 && len(req.Schedule) == 0 && len(req.Budget) == 0 {
		return nil, fmt.Errorf("ai.ForesightRiskJudgment: at least one risk is required")
	}

	prompt, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ai.ForesightRiskJudgment: marshal prompt: %w", err)
	}

	raw, err := c.callTool(ctx, "foresight_risk", c.model, foresightRiskSystem,
		[]contentBlock{textBlock("Judge the materiality of these standing cross-module risks:\n" + string(prompt))},
		"judge_foresight_risk", foresightRiskSchema)
	if err != nil {
		return nil, fmt.Errorf("ai.ForesightRiskJudgment: %w", err)
	}

	var out ForesightRiskResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai.ForesightRiskJudgment: decode tool output: %w", err)
	}
	return &out, nil
}
