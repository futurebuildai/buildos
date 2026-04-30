package models

import (
	"encoding/json"
	"testing"
)

func TestPipelineStage_Probability(t *testing.T) {
	cases := map[PipelineStage]int{
		StageLead:             10,
		StageQualified:        25,
		StageEstimateSent:     50,
		StageVerbalCommitment: 75,
		StagePermitApplied:    85,
		StagePermitIssued:     100,
		StageLost:             0,
		PipelineStage("BOGUS"): 0, // unknown stage falls through
	}
	for stage, want := range cases {
		if got := stage.Probability(); got != want {
			t.Errorf("%s.Probability() = %d, want %d", stage, got, want)
		}
	}
}

func TestPipelineStage_IsTerminal(t *testing.T) {
	cases := map[PipelineStage]bool{
		StageLead:             false,
		StageQualified:        false,
		StageEstimateSent:     false,
		StageVerbalCommitment: false,
		StagePermitApplied:    false,
		StagePermitIssued:     true,
		StageLost:             true,
	}
	for stage, want := range cases {
		if got := stage.IsTerminal(); got != want {
			t.Errorf("%s.IsTerminal() = %v, want %v", stage, got, want)
		}
	}
}

func TestCanTransition(t *testing.T) {
	type tc struct {
		from PipelineStage
		to   PipelineStage
		want bool
	}
	cases := []tc{
		// Forward through the pipeline
		{StageLead, StageQualified, true},
		{StageQualified, StageEstimateSent, true},
		{StageEstimateSent, StageVerbalCommitment, true},
		{StageVerbalCommitment, StagePermitApplied, true},
		{StagePermitApplied, StagePermitIssued, true},
		// LOST is reachable from any non-terminal
		{StageLead, StageLost, true},
		{StageQualified, StageLost, true},
		{StagePermitApplied, StageLost, true},
		// Skipping stages forbidden
		{StageLead, StageEstimateSent, false},
		{StageLead, StagePermitIssued, false},
		{StageQualified, StagePermitApplied, false},
		// Backward forbidden
		{StageQualified, StageLead, false},
		{StagePermitApplied, StageQualified, false},
		// Terminal sources can't transition
		{StagePermitIssued, StageLost, false},
		{StagePermitIssued, StageLead, false},
		{StageLost, StageLead, false},
		{StageLost, StagePermitIssued, false},
		// Self-transition forbidden
		{StageLead, StageLead, false},
		{StagePermitIssued, StagePermitIssued, false},
		// Unknown source
		{PipelineStage("BOGUS"), StageQualified, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.want {
			t.Errorf("CanTransition(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestPipelineEstimateLineItems_Scan(t *testing.T) {
	t.Run("nil source", func(t *testing.T) {
		var items PipelineEstimateLineItems
		if err := items.Scan(nil); err != nil {
			t.Fatalf("Scan(nil) returned error: %v", err)
		}
		if items != nil {
			t.Errorf("Scan(nil) should produce nil slice; got %v", items)
		}
	})

	t.Run("empty bytes", func(t *testing.T) {
		var items PipelineEstimateLineItems = []PipelineEstimateLineItem{{WBSCode: "stale"}}
		if err := items.Scan([]byte{}); err != nil {
			t.Fatalf("Scan(empty) returned error: %v", err)
		}
		if items != nil {
			t.Errorf("Scan(empty) should clear to nil; got %v", items)
		}
	})

	t.Run("valid JSON", func(t *testing.T) {
		raw := []byte(`[{"wbs_code":"6.0","description":"Foundation","estimated_cents":2500000,"unit":"sqft","quantity":1800}]`)
		var items PipelineEstimateLineItems
		if err := items.Scan(raw); err != nil {
			t.Fatalf("Scan(valid): %v", err)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
		if items[0].WBSCode != "6.0" || items[0].EstimatedCents != 2_500_000 {
			t.Errorf("decoded item = %+v", items[0])
		}
	})

	t.Run("string source", func(t *testing.T) {
		raw := `[{"wbs_code":"7.0","description":"Framing","estimated_cents":1000000}]`
		var items PipelineEstimateLineItems
		if err := items.Scan(raw); err != nil {
			t.Fatalf("Scan(string): %v", err)
		}
		if len(items) != 1 || items[0].WBSCode != "7.0" {
			t.Errorf("decoded = %+v", items)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		var items PipelineEstimateLineItems
		if err := items.Scan([]byte("not-json")); err == nil {
			t.Error("Scan(bad-json) should error")
		}
	})

	t.Run("unsupported type", func(t *testing.T) {
		var items PipelineEstimateLineItems
		if err := items.Scan(42); err == nil {
			t.Error("Scan(int) should error")
		}
	})

	t.Run("round-trip via Marshal", func(t *testing.T) {
		original := PipelineEstimateLineItems{
			{WBSCode: "9.0", Description: "Roofing", EstimatedCents: 450_000, Unit: "sqft", Quantity: 2400},
			{WBSCode: "10.0", Description: "Siding", EstimatedCents: 320_000},
		}
		raw, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var decoded PipelineEstimateLineItems
		if err := decoded.Scan(raw); err != nil {
			t.Fatalf("Scan: %v", err)
		}
		if len(decoded) != len(original) {
			t.Fatalf("length: got %d, want %d", len(decoded), len(original))
		}
		for i := range original {
			if decoded[i] != original[i] {
				t.Errorf("item[%d]: got %+v, want %+v", i, decoded[i], original[i])
			}
		}
	})
}

func TestIsValidEstimateStatus(t *testing.T) {
	for _, ok := range []string{"draft", "sent", "revised", "accepted"} {
		if !IsValidEstimateStatus(ok) {
			t.Errorf("IsValidEstimateStatus(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "DRAFT", "approved", "bogus"} {
		if IsValidEstimateStatus(bad) {
			t.Errorf("IsValidEstimateStatus(%q) = true, want false", bad)
		}
	}
}

func TestIsValidPermitStatus(t *testing.T) {
	for _, ok := range []string{
		"not_submitted", "submitted", "under_review",
		"revisions_requested", "approved", "denied",
	} {
		if !IsValidPermitStatus(ok) {
			t.Errorf("IsValidPermitStatus(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "APPROVED", "issued", "bogus"} {
		if IsValidPermitStatus(bad) {
			t.Errorf("IsValidPermitStatus(%q) = true, want false", bad)
		}
	}
}
