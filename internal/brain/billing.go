package brain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// BillingClient wraps Brain's GET /api/billing/usage and /usage/daily.
// These return AI-token usage already metered with the configured
// markup applied — BuildOS surfaces them as the "AI usage this month"
// dashboard. No write endpoints; meter writes happen inside Brain when
// Maestro calls Anthropic.
type BillingClient struct {
	c *Client
}

// UsageSummary is the aggregated response from GET /api/billing/usage.
// Field shapes mirror Brain's metering store; cents are after markup.
type UsageSummary struct {
	OrgID          string       `json:"org_id"`
	Start          time.Time    `json:"start"`
	End            time.Time    `json:"end"`
	TotalTokens    int64        `json:"total_tokens"`
	InputTokens    int64        `json:"input_tokens"`
	OutputTokens   int64        `json:"output_tokens"`
	CostCents      int64        `json:"cost_cents"`
	CurrencyCode   string       `json:"currency_code"`
	ModelBreakdown []ModelUsage `json:"model_breakdown,omitempty"`
}

// ModelUsage is a per-model row in the usage summary.
type ModelUsage struct {
	Model        string `json:"model"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CostCents    int64  `json:"cost_cents"`
}

// DailyUsage is one row of the GET /api/billing/usage/daily response.
type DailyUsage struct {
	Date         string `json:"date"` // YYYY-MM-DD
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CostCents    int64  `json:"cost_cents"`
}

// DailyUsageResponse wraps a date-keyed array of DailyUsage rows.
type DailyUsageResponse struct {
	OrgID string       `json:"org_id"`
	Start string       `json:"start"`
	End   string       `json:"end"`
	Days  []DailyUsage `json:"days"`
}

// UsageRange is the optional query window for both billing endpoints.
// Zero values mean "use Brain's default" (current calendar month).
type UsageRange struct {
	Start time.Time
	End   time.Time
}

// query builds the ?start=&end= suffix for either endpoint, or empty
// string when both are zero.
func (r UsageRange) query() string {
	if r.Start.IsZero() && r.End.IsZero() {
		return ""
	}
	v := url.Values{}
	if !r.Start.IsZero() {
		v.Set("start", r.Start.UTC().Format(time.RFC3339))
	}
	if !r.End.IsZero() {
		v.Set("end", r.End.UTC().Format(time.RFC3339))
	}
	return "?" + v.Encode()
}

// GetUsageSummary returns aggregated AI-token usage for the caller's
// org over the given window. Org is implicit from the JWT.
func (b *BillingClient) GetUsageSummary(ctx context.Context, r UsageRange) (*UsageSummary, error) {
	raw, err := b.c.doRequest(ctx, "GET", "/api/billing/usage"+r.query(), nil)
	if err != nil {
		return nil, err
	}
	var resp UsageSummary
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("brain.Billing.GetUsageSummary: decode response: %w", err)
	}
	return &resp, nil
}

// GetDailyUsage returns per-day usage rows for chart rendering.
func (b *BillingClient) GetDailyUsage(ctx context.Context, r UsageRange) (*DailyUsageResponse, error) {
	raw, err := b.c.doRequest(ctx, "GET", "/api/billing/usage/daily"+r.query(), nil)
	if err != nil {
		return nil, err
	}
	var resp DailyUsageResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("brain.Billing.GetDailyUsage: decode response: %w", err)
	}
	return &resp, nil
}
