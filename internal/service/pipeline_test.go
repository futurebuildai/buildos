package service

import (
	"testing"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

func TestNextPipelineStage(t *testing.T) {
	tests := []struct {
		from    models.PipelineStage
		want    models.PipelineStage
		wantErr bool
	}{
		{models.StageLead, models.StageQualified, false},
		{models.StageQualified, models.StageEstimateSent, false},
		{models.StageEstimateSent, models.StageVerbalCommitment, false},
		{models.StageVerbalCommitment, models.StagePermitApplied, false},
		{models.StagePermitApplied, models.StagePermitIssued, false},
		{models.StagePermitIssued, "", true},  // already at final stage
		{models.StageLost, "", true},          // LOST is not in StageOrder
		{models.PipelineStage("BOGUS"), "", true},
	}

	for _, tc := range tests {
		t.Run(string(tc.from), func(t *testing.T) {
			got, err := nextPipelineStage(tc.from)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error for stage %s, got next=%s", tc.from, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("nextPipelineStage(%s) = %s, want %s", tc.from, got, tc.want)
			}
		})
	}
}

func TestValidateStageTransition(t *testing.T) {
	// Valid transitions
	validPairs := [][2]models.PipelineStage{
		{models.StageLead, models.StageQualified},
		{models.StageQualified, models.StageEstimateSent},
		{models.StagePermitApplied, models.StagePermitIssued},
	}
	for _, pair := range validPairs {
		t.Run(string(pair[0])+"->"+string(pair[1]), func(t *testing.T) {
			if err := ValidateStageTransition(pair[0], pair[1]); err != nil {
				t.Errorf("expected valid transition %s->%s, got error: %v", pair[0], pair[1], err)
			}
		})
	}

	// Invalid transitions (skipping stages)
	invalidPairs := [][2]models.PipelineStage{
		{models.StageLead, models.StagePermitIssued},
		{models.StageLead, models.StageEstimateSent},
		{models.StageQualified, models.StagePermitApplied},
	}
	for _, pair := range invalidPairs {
		t.Run(string(pair[0])+"->"+string(pair[1])+"_invalid", func(t *testing.T) {
			if err := ValidateStageTransition(pair[0], pair[1]); err == nil {
				t.Errorf("expected error for invalid transition %s->%s", pair[0], pair[1])
			}
		})
	}
}

func TestStageProbability(t *testing.T) {
	tests := []struct {
		stage models.PipelineStage
		want  int
	}{
		{models.StageLead, 10},
		{models.StageQualified, 25},
		{models.StageEstimateSent, 50},
		{models.StageVerbalCommitment, 75},
		{models.StagePermitApplied, 85},
		{models.StagePermitIssued, 100},
		{models.StageLost, 0},
	}

	for _, tc := range tests {
		t.Run(string(tc.stage), func(t *testing.T) {
			got, ok := models.StageProbability[tc.stage]
			if !ok {
				t.Fatalf("stage %s not found in StageProbability", tc.stage)
			}
			if got != tc.want {
				t.Errorf("StageProbability[%s] = %d, want %d", tc.stage, got, tc.want)
			}
		})
	}
}

func TestStageOrder(t *testing.T) {
	if len(models.StageOrder) != 6 {
		t.Fatalf("StageOrder has %d entries, want 6", len(models.StageOrder))
	}

	// Verify LOST is not in StageOrder
	for _, s := range models.StageOrder {
		if s == models.StageLost {
			t.Error("LOST should not be in StageOrder")
		}
	}

	// Verify order
	expected := []models.PipelineStage{
		models.StageLead,
		models.StageQualified,
		models.StageEstimateSent,
		models.StageVerbalCommitment,
		models.StagePermitApplied,
		models.StagePermitIssued,
	}
	for i, s := range expected {
		if models.StageOrder[i] != s {
			t.Errorf("StageOrder[%d] = %s, want %s", i, models.StageOrder[i], s)
		}
	}
}

func TestEstimateConstants(t *testing.T) {
	if models.EstimateStatusDraft != "draft" {
		t.Errorf("EstimateStatusDraft = %q, want %q", models.EstimateStatusDraft, "draft")
	}
	if models.EstimateStatusSent != "sent" {
		t.Errorf("EstimateStatusSent = %q, want %q", models.EstimateStatusSent, "sent")
	}
	if models.EstimateStatusRevised != "revised" {
		t.Errorf("EstimateStatusRevised = %q, want %q", models.EstimateStatusRevised, "revised")
	}
	if models.EstimateStatusAccepted != "accepted" {
		t.Errorf("EstimateStatusAccepted = %q, want %q", models.EstimateStatusAccepted, "accepted")
	}
}

func TestPermitConstants(t *testing.T) {
	if models.PermitStatusNotSubmitted != "not_submitted" {
		t.Errorf("PermitStatusNotSubmitted = %q, want %q", models.PermitStatusNotSubmitted, "not_submitted")
	}
	if models.PermitStatusApproved != "approved" {
		t.Errorf("PermitStatusApproved = %q, want %q", models.PermitStatusApproved, "approved")
	}
	if models.PermitStatusDenied != "denied" {
		t.Errorf("PermitStatusDenied = %q, want %q", models.PermitStatusDenied, "denied")
	}
}

func TestPipelineServiceEstimateValidation(t *testing.T) {
	// Can't create service without a real store, but we can test that
	// the currency validation logic is correct by checking the supported currencies map.
	if !SupportedCurrencies["USD"] {
		t.Error("USD should be supported")
	}
	if !SupportedCurrencies["CAD"] {
		t.Error("CAD should be supported")
	}
	if SupportedCurrencies["EUR"] {
		t.Error("EUR should not be supported")
	}
	if SupportedCurrencies["GBP"] {
		t.Error("GBP should not be supported")
	}
}

func TestWeightedRevenue(t *testing.T) {
	// Simulate weighted revenue calculation
	// $100,000 estimate at ESTIMATE_SENT (50%) = $50,000 weighted
	estimateCents := int64(10_000_000) // $100,000
	probability := models.StageProbability[models.StageEstimateSent]
	weighted := estimateCents * int64(probability) / 100

	if weighted != 5_000_000 {
		t.Errorf("weighted = %d, want 5000000", weighted)
	}

	// $250,000 at VERBAL_COMMITMENT (75%) = $187,500
	estimateCents = int64(25_000_000)
	probability = models.StageProbability[models.StageVerbalCommitment]
	weighted = estimateCents * int64(probability) / 100

	if weighted != 18_750_000 {
		t.Errorf("weighted = %d, want 18750000", weighted)
	}

	// $500,000 at PERMIT_ISSUED (100%) = $500,000
	estimateCents = int64(50_000_000)
	probability = models.StageProbability[models.StagePermitIssued]
	weighted = estimateCents * int64(probability) / 100

	if weighted != 50_000_000 {
		t.Errorf("weighted = %d, want 50000000", weighted)
	}
}
