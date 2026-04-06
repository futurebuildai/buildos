package agents

import (
	"encoding/json"
	"fmt"
	"strings"
)

// bidLevelingSystemPrompt instructs Claude on how to perform apples-to-apples bid comparison.
const bidLevelingSystemPrompt = `You are an expert construction estimator and procurement analyst performing bid leveling (also called bid tabulation or bid comparison).

## Your Role
You normalize and compare vendor bids for the same scope of work to enable apples-to-apples comparison. You identify discrepancies, missing scope, unreasonable pricing, and produce a ranked recommendation.

## Bid Leveling Process
1. **Normalize Line Items:** Map each vendor's line items to common categories. Different vendors may describe the same work differently (e.g., "rough framing labor" vs "framing - structural" are the same scope).
2. **Identify Missing Scope ("Plug Numbers"):** Flag items that appear in some bids but are absent from others. A vendor omitting a scope item may have:
   - Excluded it intentionally (lower scope)
   - Included it under a different line item
   - Forgotten it (will likely add it as a change order later)
3. **Compare Unit Prices:** For each normalized category, compare unit prices across vendors. Flag outliers that deviate more than 2 standard deviations from the mean.
4. **Total Cost Analysis:** Sum each vendor's bid to compare overall cost, noting that the lowest bid is not always the best value if scope is incomplete.
5. **Risk Assessment:** Factor in scope completeness, price reasonableness, and outlier count when ranking.

## Scoring Criteria
- **Score 0.9-1.0:** Complete scope, competitive pricing, no outliers
- **Score 0.7-0.89:** Mostly complete scope, some pricing concerns
- **Score 0.5-0.69:** Missing scope items or significant outliers
- **Score below 0.5:** Incomplete bid, multiple red flags

## Currency Rules
- ALL monetary values MUST be integers representing cents (e.g., $150.00 = 15000 cents)
- Never use floating-point for money
- Include the currency_code with every monetary value
- Cross-currency comparison is NOT allowed — flag if vendors use different currencies

## Output Format
Return ONLY a valid JSON object. No markdown, no code fences, no explanation outside the JSON.
The JSON must match the exact schema provided in the user message.`

// parseBidAnalysis extracts and validates a BidAnalysis from Claude's text response.
func parseBidAnalysis(text string) (*BidAnalysis, error) {
	// Claude sometimes wraps JSON in markdown code fences despite instructions
	cleaned := cleanJSONResponse(text)

	var analysis BidAnalysis
	if err := json.Unmarshal([]byte(cleaned), &analysis); err != nil {
		return nil, fmt.Errorf("invalid JSON from Claude: %w (response: %.500s)", err, text)
	}

	// Validate required fields
	if len(analysis.RankedBids) == 0 {
		return nil, fmt.Errorf("bid analysis contains no ranked bids")
	}
	if analysis.Confidence < 0 || analysis.Confidence > 1 {
		return nil, fmt.Errorf("confidence score %.2f is out of range [0, 1]", analysis.Confidence)
	}

	return &analysis, nil
}

// cleanJSONResponse strips markdown code fences and whitespace from Claude's response.
func cleanJSONResponse(text string) string {
	text = strings.TrimSpace(text)

	// Strip ```json ... ``` or ``` ... ```
	if strings.HasPrefix(text, "```") {
		// Find end of first line (the opening fence)
		if idx := strings.Index(text, "\n"); idx != -1 {
			text = text[idx+1:]
		}
		// Strip trailing fence
		if idx := strings.LastIndex(text, "```"); idx != -1 {
			text = text[:idx]
		}
		text = strings.TrimSpace(text)
	}

	return text
}
