package agents

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/futurebuild/futurebuild-os/internal/agents/tools"
	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

// =============================================================================
// Bid Leveling Agent Tests
// =============================================================================

// TestAnalyzeBids_RejectsFewerThan2Bids verifies the minimum bid count check.
func TestAnalyzeBids_RejectsFewerThan2Bids(t *testing.T) {
	mockClient := &ai.MockClient{}
	toolReg := tools.NewRegistry()
	runner := NewAgentRunner(mockClient, toolReg)

	agent := &BidLevelingAgent{
		claudeRunner: runner,
		logger:       slog.Default(),
	}

	orgID := uuid.New()
	projectID := uuid.New()

	tests := []struct {
		name string
		bids []BidInput
	}{
		{"zero_bids", []BidInput{}},
		{"one_bid", []BidInput{
			{Vendor: "Vendor A", LineItems: []BidLineItem{
				{Description: "Framing", Quantity: 100, UnitPriceCents: 1500, CurrencyCode: "USD"},
			}},
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := agent.AnalyzeBids(context.Background(), orgID, projectID, nil, tc.bids)
			if err == nil {
				t.Fatal("expected error for fewer than 2 bids")
			}
		})
	}
}

// TestAnalyzeBids_RejectsMissingCurrencyCode verifies validation of currency codes.
func TestAnalyzeBids_RejectsMissingCurrencyCode(t *testing.T) {
	mockClient := &ai.MockClient{}
	toolReg := tools.NewRegistry()
	runner := NewAgentRunner(mockClient, toolReg)

	agent := &BidLevelingAgent{
		claudeRunner: runner,
		logger:       slog.Default(),
	}

	orgID := uuid.New()
	projectID := uuid.New()

	bids := []BidInput{
		{
			Vendor: "Vendor A",
			LineItems: []BidLineItem{
				{Description: "Framing", Quantity: 100, UnitPriceCents: 1500, CurrencyCode: "USD"},
			},
		},
		{
			Vendor: "Vendor B",
			LineItems: []BidLineItem{
				{Description: "Framing", Quantity: 100, UnitPriceCents: 1400, CurrencyCode: ""}, // Missing!
			},
		},
	}

	_, err := agent.AnalyzeBids(context.Background(), orgID, projectID, nil, bids)
	if err == nil {
		t.Fatal("expected error for missing currency_code")
	}
}

// TestParseBidAnalysis_With2Bids_ReturnsRankedResults verifies that a valid
// 2-bid Claude response parses into ranked results correctly.
// NOTE: We test parseBidAnalysis directly rather than the full AnalyzeBids path
// because AnalyzeBids.persistAnalysis requires a live pgxpool connection.
func TestParseBidAnalysis_With2Bids_ReturnsRankedResults(t *testing.T) {
	analysisResp := BidAnalysis{
		RankedBids: []RankedBid{
			{Vendor: "Vendor A", Rank: 1, TotalCents: 150_000, CurrencyCode: "USD", Score: 0.92, Notes: "Complete scope, competitive pricing"},
			{Vendor: "Vendor B", Rank: 2, TotalCents: 175_000, CurrencyCode: "USD", Score: 0.78, Notes: "Missing ice shield"},
		},
		MissingScope: []MissingScopeItem{
			{Description: "Ice & Water Shield", PresentIn: []string{"Vendor A"}, MissingFrom: []string{"Vendor B"}, IsPlugNumber: true},
		},
		OutlierFlags: []OutlierFlag{
			{Vendor: "Vendor B", Description: "Ridge Vent", UnitPriceCents: 5000, CurrencyCode: "USD", AvgPriceCents: 2500, DeviationPct: 100.0, Direction: "high"},
		},
		Recommendation: "Vendor A is recommended with complete scope and competitive pricing.",
		Confidence:     0.88,
	}

	analysisJSON, _ := json.Marshal(analysisResp)
	result, err := parseBidAnalysis(string(analysisJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify ranked bids
	if len(result.RankedBids) != 2 {
		t.Fatalf("expected 2 ranked bids, got %d", len(result.RankedBids))
	}
	if result.RankedBids[0].Vendor != "Vendor A" {
		t.Errorf("expected rank 1 vendor 'Vendor A', got %q", result.RankedBids[0].Vendor)
	}
	if result.RankedBids[0].Rank != 1 {
		t.Errorf("expected rank 1, got %d", result.RankedBids[0].Rank)
	}

	// Verify BIGINT cents in results
	if result.RankedBids[0].TotalCents != 150_000 {
		t.Errorf("expected TotalCents 150000, got %d", result.RankedBids[0].TotalCents)
	}
	if result.RankedBids[0].CurrencyCode != "USD" {
		t.Errorf("expected CurrencyCode USD, got %q", result.RankedBids[0].CurrencyCode)
	}

	// Verify missing scope
	if len(result.MissingScope) != 1 {
		t.Errorf("expected 1 missing scope item, got %d", len(result.MissingScope))
	}

	// Verify outlier flags
	if len(result.OutlierFlags) != 1 {
		t.Errorf("expected 1 outlier flag, got %d", len(result.OutlierFlags))
	}
	if result.OutlierFlags[0].Direction != "high" {
		t.Errorf("expected direction 'high', got %q", result.OutlierFlags[0].Direction)
	}
	// Verify BIGINT cents in outlier
	if result.OutlierFlags[0].UnitPriceCents != 5000 {
		t.Errorf("expected UnitPriceCents 5000, got %d", result.OutlierFlags[0].UnitPriceCents)
	}
	if result.OutlierFlags[0].AvgPriceCents != 2500 {
		t.Errorf("expected AvgPriceCents 2500, got %d", result.OutlierFlags[0].AvgPriceCents)
	}

	// Verify confidence
	if result.Confidence < 0 || result.Confidence > 1.0 {
		t.Errorf("confidence %f out of range [0, 1]", result.Confidence)
	}

	// Verify recommendation
	if result.Recommendation == "" {
		t.Error("expected non-empty recommendation")
	}
}

// TestAnalyzeBids_AIClientError verifies error propagation from AI client.
func TestAnalyzeBids_AIClientError(t *testing.T) {
	mockClient := &ai.MockClient{}
	mockClient.SetError(errForTest("AI service unavailable"))

	toolReg := tools.NewRegistry()
	runner := NewAgentRunner(mockClient, toolReg)

	agent := &BidLevelingAgent{
		claudeRunner: runner,
		logger:       slog.Default(),
	}

	orgID := uuid.New()
	projectID := uuid.New()

	bids := []BidInput{
		{Vendor: "A", LineItems: []BidLineItem{{Description: "X", Quantity: 1, UnitPriceCents: 100, CurrencyCode: "USD"}}},
		{Vendor: "B", LineItems: []BidLineItem{{Description: "X", Quantity: 1, UnitPriceCents: 200, CurrencyCode: "USD"}}},
	}

	_, err := agent.AnalyzeBids(context.Background(), orgID, projectID, nil, bids)
	if err == nil {
		t.Fatal("expected error from AI client failure")
	}
}

// =============================================================================
// parseBidAnalysis Tests -- Claude response parsing
// =============================================================================

// TestParseBidAnalysis_ValidJSON verifies parsing of a clean JSON response.
func TestParseBidAnalysis_ValidJSON(t *testing.T) {
	raw := `{
		"ranked_bids": [
			{"vendor": "A", "rank": 1, "total_cents": 100000, "currency_code": "USD", "score": 0.9, "notes": "Good"}
		],
		"missing_scope": [],
		"outlier_flags": [],
		"recommendation": "Go with A",
		"confidence": 0.85
	}`

	analysis, err := parseBidAnalysis(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(analysis.RankedBids) != 1 {
		t.Errorf("expected 1 ranked bid, got %d", len(analysis.RankedBids))
	}
	if analysis.Confidence != 0.85 {
		t.Errorf("expected confidence 0.85, got %f", analysis.Confidence)
	}
}

// TestParseBidAnalysis_WrappedInCodeFence verifies stripping of markdown fences.
func TestParseBidAnalysis_WrappedInCodeFence(t *testing.T) {
	raw := "```json\n" + `{
		"ranked_bids": [
			{"vendor": "A", "rank": 1, "total_cents": 100000, "currency_code": "USD", "score": 0.9, "notes": "Good"}
		],
		"missing_scope": [],
		"outlier_flags": [],
		"recommendation": "Go with A",
		"confidence": 0.85
	}` + "\n```"

	analysis, err := parseBidAnalysis(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(analysis.RankedBids) != 1 {
		t.Errorf("expected 1 ranked bid, got %d", len(analysis.RankedBids))
	}
}

// TestParseBidAnalysis_EmptyRankedBids verifies rejection of empty results.
func TestParseBidAnalysis_EmptyRankedBids(t *testing.T) {
	raw := `{
		"ranked_bids": [],
		"missing_scope": [],
		"outlier_flags": [],
		"recommendation": "No bids to rank",
		"confidence": 0.5
	}`

	_, err := parseBidAnalysis(raw)
	if err == nil {
		t.Fatal("expected error for empty ranked bids")
	}
}

// TestParseBidAnalysis_InvalidConfidence verifies rejection of out-of-range confidence.
func TestParseBidAnalysis_InvalidConfidence(t *testing.T) {
	raw := `{
		"ranked_bids": [
			{"vendor": "A", "rank": 1, "total_cents": 100000, "currency_code": "USD", "score": 0.9, "notes": "Good"}
		],
		"missing_scope": [],
		"outlier_flags": [],
		"recommendation": "Go with A",
		"confidence": 1.5
	}`

	_, err := parseBidAnalysis(raw)
	if err == nil {
		t.Fatal("expected error for confidence > 1.0")
	}
}

// TestParseBidAnalysis_InvalidJSON verifies handling of malformed JSON.
func TestParseBidAnalysis_InvalidJSON(t *testing.T) {
	_, err := parseBidAnalysis("this is not json")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// TestCleanJSONResponse verifies markdown fence stripping.
func TestCleanJSONResponse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain_json", `{"key": "value"}`, `{"key": "value"}`},
		{"code_fence", "```json\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"backtick_only", "```\n{\"key\": \"value\"}\n```", `{"key": "value"}`},
		{"whitespace", "  {\"key\": \"value\"}  ", `{"key": "value"}`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanJSONResponse(tc.input)
			if got != tc.want {
				t.Errorf("cleanJSONResponse(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// =============================================================================
// BidInput / BidLineItem BIGINT cents validation
// =============================================================================

// TestBidInput_MonetaryValues verifies that all monetary fields use BIGINT cents.
func TestBidInput_MonetaryValues(t *testing.T) {
	bid := BidInput{
		Vendor: "Test Vendor",
		LineItems: []BidLineItem{
			{
				Description:    "Roofing Shingles",
				Quantity:       65,
				UnitPriceCents: 999_999_999_99, // Large value
				CurrencyCode:   "USD",
			},
		},
	}

	// Verify no precision loss
	if bid.LineItems[0].UnitPriceCents != 999_999_999_99 {
		t.Errorf("UnitPriceCents lost precision: got %d", bid.LineItems[0].UnitPriceCents)
	}

	// JSON round-trip
	data, _ := json.Marshal(bid)
	var decoded BidInput
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON round-trip failed: %v", err)
	}
	if decoded.LineItems[0].UnitPriceCents != 999_999_999_99 {
		t.Errorf("UnitPriceCents lost precision after JSON round-trip: got %d", decoded.LineItems[0].UnitPriceCents)
	}
}

// TestRankedBid_MonetaryValues verifies BIGINT cents in ranked bid results.
func TestRankedBid_MonetaryValues(t *testing.T) {
	rb := RankedBid{
		Vendor:       "Test",
		Rank:         1,
		TotalCents:   15_000_000,
		CurrencyCode: "USD",
		Score:        0.95,
	}

	if rb.TotalCents != 15_000_000 {
		t.Errorf("TotalCents lost precision: got %d", rb.TotalCents)
	}
	if rb.CurrencyCode != "USD" {
		t.Errorf("expected CurrencyCode USD, got %q", rb.CurrencyCode)
	}
}

// TestOutlierFlag_MonetaryValues verifies BIGINT cents in outlier flags.
func TestOutlierFlag_MonetaryValues(t *testing.T) {
	of := OutlierFlag{
		Vendor:         "Test",
		Description:    "Ridge Vent",
		UnitPriceCents: 5000,
		CurrencyCode:   "USD",
		AvgPriceCents:  2500,
		DeviationPct:   100.0,
		Direction:      "high",
	}

	if of.UnitPriceCents != 5000 {
		t.Errorf("UnitPriceCents: got %d", of.UnitPriceCents)
	}
	if of.AvgPriceCents != 2500 {
		t.Errorf("AvgPriceCents: got %d", of.AvgPriceCents)
	}
	if of.CurrencyCode != "USD" {
		t.Errorf("CurrencyCode: got %q", of.CurrencyCode)
	}
}

// =============================================================================
// Helpers
// =============================================================================

// errForTest creates a simple error for test use.
type testError string

func errForTest(msg string) error { return testError(msg) }
func (e testError) Error() string { return string(e) }
