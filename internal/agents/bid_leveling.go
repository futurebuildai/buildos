package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/service"
)

// BidInput represents a single vendor bid for comparison.
// All monetary values use Composite Currency Pattern (BIGINT cents + currency_code).
type BidInput struct {
	Vendor    string         `json:"vendor"`
	LineItems []BidLineItem  `json:"line_items"`
}

// BidLineItem is a single line item within a vendor bid.
type BidLineItem struct {
	Description    string `json:"description"`
	Quantity       int    `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	CurrencyCode   string `json:"currency_code"`
}

// BidAnalysis is the output of the bid leveling agent.
type BidAnalysis struct {
	RankedBids     []RankedBid     `json:"ranked_bids"`
	MissingScope   []MissingScopeItem `json:"missing_scope"`
	OutlierFlags   []OutlierFlag   `json:"outlier_flags"`
	Recommendation string          `json:"recommendation"`
	Confidence     float64         `json:"confidence"`
}

// RankedBid represents a vendor bid with its ranking and score.
type RankedBid struct {
	Vendor       string  `json:"vendor"`
	Rank         int     `json:"rank"`
	TotalCents   int64   `json:"total_cents"`
	CurrencyCode string  `json:"currency_code"`
	Score        float64 `json:"score"`
	Notes        string  `json:"notes"`
}

// MissingScopeItem identifies a scope item present in some bids but absent in others.
type MissingScopeItem struct {
	Description string   `json:"description"`
	PresentIn   []string `json:"present_in"`
	MissingFrom []string `json:"missing_from"`
	IsPlugNumber bool    `json:"is_plug_number"`
}

// OutlierFlag marks a line item whose price deviates significantly from peers.
type OutlierFlag struct {
	Vendor         string  `json:"vendor"`
	Description    string  `json:"description"`
	UnitPriceCents int64   `json:"unit_price_cents"`
	CurrencyCode   string  `json:"currency_code"`
	AvgPriceCents  int64   `json:"avg_price_cents"`
	DeviationPct   float64 `json:"deviation_pct"`
	Direction      string  `json:"direction"` // "high" or "low"
}

// BidLevelingAgent uses Claude to perform apples-to-apples bid comparison.
type BidLevelingAgent struct {
	claudeRunner *AgentRunner
	pool         *pgxpool.Pool
	feedSvc      *service.FeedService
	logger       *slog.Logger
}

// NewBidLevelingAgent creates a new BidLevelingAgent.
func NewBidLevelingAgent(
	claudeRunner *AgentRunner,
	pool *pgxpool.Pool,
	feedSvc *service.FeedService,
	logger *slog.Logger,
) *BidLevelingAgent {
	return &BidLevelingAgent{
		claudeRunner: claudeRunner,
		pool:         pool,
		feedSvc:      feedSvc,
		logger:       logger,
	}
}

// AnalyzeBids sends multiple vendor bids to Claude for normalization and comparison.
// Returns a BidAnalysis with ranked recommendations, missing scope, and outlier flags.
func (a *BidLevelingAgent) AnalyzeBids(ctx context.Context, orgID, projectID uuid.UUID, itemID *uuid.UUID, bids []BidInput) (*BidAnalysis, error) {
	if len(bids) < 2 {
		return nil, fmt.Errorf("bid leveling requires at least 2 bids, got %d", len(bids))
	}

	// Validate all currency codes are consistent within each bid
	for _, bid := range bids {
		for _, li := range bid.LineItems {
			if li.CurrencyCode == "" {
				return nil, fmt.Errorf("missing currency_code in line item for vendor %s", bid.Vendor)
			}
		}
	}

	// Serialize bids for Claude
	bidsJSON, err := json.MarshalIndent(bids, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling bids: %w", err)
	}

	userMessage := fmt.Sprintf(`Analyze and compare these %d vendor bids for the same scope of work.

## Bids Data
%s

## Instructions
1. Normalize all line items to common categories for apples-to-apples comparison.
2. Identify any scope items that are present in some bids but missing from others ("plug numbers").
3. Compare unit prices across vendors and flag outliers (>2 standard deviations from mean).
4. Rank the bids from best to worst value, considering total cost, scope completeness, and price reasonableness.
5. Provide a clear recommendation with confidence score (0.0-1.0).

Respond with a JSON object matching this structure exactly:
{
  "ranked_bids": [{"vendor": "...", "rank": 1, "total_cents": 0, "currency_code": "USD", "score": 0.95, "notes": "..."}],
  "missing_scope": [{"description": "...", "present_in": ["Vendor A"], "missing_from": ["Vendor B"], "is_plug_number": true}],
  "outlier_flags": [{"vendor": "...", "description": "...", "unit_price_cents": 0, "currency_code": "USD", "avg_price_cents": 0, "deviation_pct": 0.0, "direction": "high"}],
  "recommendation": "...",
  "confidence": 0.85
}

All monetary values must be in cents (integer). Do not use floating-point for money.
Return ONLY the JSON object, no markdown fences or extra text.`, len(bids), string(bidsJSON))

	projectCtx := ProjectContext{
		ProjectID: projectID,
		OrgID:     orgID,
		UserID:    uuid.Nil, // Agent user
	}

	// Use a limited turn count since this is a single-shot analysis
	runner := a.claudeRunner.WithMaxTurns(3)
	result, err := runner.Run(ctx, bidLevelingSystemPrompt, userMessage, projectCtx)
	if err != nil {
		return nil, fmt.Errorf("claude bid analysis: %w", err)
	}

	a.logger.Info("bid leveling claude completed",
		"project_id", projectID,
		"bid_count", len(bids),
		"turns", result.Turns,
		"tokens", result.TotalTokens,
	)

	// Parse Claude's response
	analysis, err := parseBidAnalysis(result.Text)
	if err != nil {
		return nil, fmt.Errorf("parsing bid analysis: %w", err)
	}

	// Persist the analysis
	if err := a.persistAnalysis(ctx, orgID, projectID, itemID, bids, analysis); err != nil {
		a.logger.Error("failed to persist bid analysis",
			"project_id", projectID,
			"error", err,
		)
		// Non-fatal: return the analysis even if persistence fails
	}

	return analysis, nil
}

// persistAnalysis stores the bid analysis in the database.
func (a *BidLevelingAgent) persistAnalysis(ctx context.Context, orgID, projectID uuid.UUID, itemID *uuid.UUID, bids []BidInput, analysis *BidAnalysis) error {
	bidsData, err := json.Marshal(bids)
	if err != nil {
		return fmt.Errorf("marshaling bids data: %w", err)
	}

	analysisData, err := json.Marshal(analysis)
	if err != nil {
		return fmt.Errorf("marshaling analysis: %w", err)
	}

	_, err = a.pool.Exec(ctx, `
		INSERT INTO bid_analyses (org_id, project_id, item_id, bid_count, bids_data, analysis, recommendation, confidence, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		orgID, projectID, itemID, len(bids), bidsData, analysisData,
		analysis.Recommendation, analysis.Confidence, time.Now().UTC(),
	)
	if err != nil {
		return fmt.Errorf("inserting bid analysis: %w", err)
	}

	return nil
}
