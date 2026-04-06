package tribunal

import (
	"context"
	"testing"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReview_ConsensusYea(t *testing.T) {
	// Setup Mocks — all three jury members are Claude
	archMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: "[VOTE]: YEA\n[REASONING]: Safe. No security issues found."},
	}
	reviewerMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: "[VOTE]: YEA\n[REASONING]: Consistent with existing patterns."},
	}
	coordMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: `{"status": "APPROVED", "consensus_score": 0.95, "summary": "LGTM", "plan": "Deploy"}`},
	}

	jury := Jury{
		Coordinator: coordMock,
		Architect:   archMock,
		Reviewer:    reviewerMock,
	}

	// Pass nil repo to skip DB writes in unit test
	engine := NewConsensusEngine(jury, nil)

	ctx := context.Background()
	req := TribunalRequest{
		CaseID:   "CASE-123",
		Category: "code_review",
		Intent:   "Fix login bug",
		Context:  "diff...",
	}

	resp, err := engine.Review(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, DecisionApproved, resp.Status)
	assert.InDelta(t, 0.95, resp.ConsensusScore, 0.01)
	assert.Equal(t, "LGTM", resp.Summary)
	assert.Equal(t, "Deploy", resp.Plan)

	// Verify all AI clients were called
	assert.Len(t, archMock.GenerateCalls, 1, "Architect should be called once")
	assert.Len(t, reviewerMock.GenerateCalls, 1, "Reviewer should be called once")
	assert.Len(t, coordMock.GenerateCalls, 1, "Coordinator should be called once")
}

func TestReview_FailClosed_InvalidJSON(t *testing.T) {
	// When coordinator returns invalid JSON, the decision MUST default to REJECTED.
	archMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: "[VOTE]: YEA\n[REASONING]: Looks good."},
	}
	reviewerMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: "[VOTE]: YEA\n[REASONING]: Consistent."},
	}
	coordMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: "This is not valid JSON at all"},
	}

	jury := Jury{
		Coordinator: coordMock,
		Architect:   archMock,
		Reviewer:    reviewerMock,
	}

	engine := NewConsensusEngine(jury, nil)
	ctx := context.Background()

	resp, err := engine.Review(ctx, TribunalRequest{
		CaseID:   "CASE-FAIL",
		Category: "code_review",
		Intent:   "Test fail-closed behavior",
		Context:  "test",
	})

	require.NoError(t, err)
	assert.Equal(t, DecisionRejected, resp.Status, "Invalid JSON must result in REJECTED (fail-closed)")
	assert.Equal(t, 0.0, resp.ConsensusScore)
}

func TestReview_ExpertError_CreatesAbstainVote(t *testing.T) {
	// When one expert fails, the system should create an ABSTAIN vote and continue.
	archMock := &ai.MockClient{
		GenerateError: assert.AnError, // Simulate error
	}
	reviewerMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: "[VOTE]: NAY\n[REASONING]: Breaks consistency."},
	}
	coordMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: `{"status": "REJECTED", "consensus_score": 0.3, "summary": "One expert failed, one NAY", "plan": ""}`},
	}

	jury := Jury{
		Coordinator: coordMock,
		Architect:   archMock,
		Reviewer:    reviewerMock,
	}

	engine := NewConsensusEngine(jury, nil)
	ctx := context.Background()

	resp, err := engine.Review(ctx, TribunalRequest{
		CaseID:   "CASE-PARTIAL",
		Category: "code_review",
		Intent:   "Test partial failure",
		Context:  "test",
	})

	require.NoError(t, err)
	assert.Equal(t, DecisionRejected, resp.Status)
}

func TestReview_NayVoteParsing(t *testing.T) {
	// Verify that NAY votes are correctly parsed from various formats
	archMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: "[VOTE]: NAY\n[REASONING]: Security risk found."},
	}
	reviewerMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: "VOTE:NAY\nBad pattern detected."},
	}
	coordMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{Text: `{"status": "REJECTED", "consensus_score": 0.1, "summary": "Both experts voted NAY", "plan": ""}`},
	}

	jury := Jury{
		Coordinator: coordMock,
		Architect:   archMock,
		Reviewer:    reviewerMock,
	}

	engine := NewConsensusEngine(jury, nil)
	ctx := context.Background()

	resp, err := engine.Review(ctx, TribunalRequest{
		CaseID:   "CASE-NAY",
		Category: "security",
		Intent:   "Test NAY voting",
		Context:  "malicious code",
	})

	require.NoError(t, err)
	assert.Equal(t, DecisionRejected, resp.Status)
}

func TestDiagnose_ValidResponse(t *testing.T) {
	coordMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{
			Text: `{"fault_diagnosis": "CONFIG_DRIFT", "confidence_score": 0.9, "reasoning": "Config mismatch", "proposed_action": {"type": "UPDATE_CONFIG", "key": "db_pool_max", "value": "50"}}`,
		},
	}

	jury := Jury{
		Coordinator: coordMock,
		Architect:   &ai.MockClient{},
		Reviewer:    &ai.MockClient{},
	}

	engine := NewConsensusEngine(jury, nil)
	ctx := context.Background()

	resp, err := engine.Diagnose(ctx, DiagnosisRequest{
		ErrorTrace:    "connection pool exhausted: config drift detected",
		MethodContext: "ScheduleService.Recalculate",
		SystemState: map[string]string{
			"db_pool_max": "25",
			"db_pool_min": "5",
		},
	})

	require.NoError(t, err)
	assert.Equal(t, "CONFIG_DRIFT", resp.Diagnosis)
	assert.InDelta(t, 0.9, resp.Confidence, 0.01)
	assert.Equal(t, "UPDATE_CONFIG", resp.ProposedAction.Type)
	assert.Equal(t, "db_pool_max", resp.ProposedAction.Key)
	assert.Equal(t, "50", resp.ProposedAction.Value)
	assert.Equal(t, "claude-sonnet", resp.ModelUsed)
}

func TestDiagnose_InvalidActionType(t *testing.T) {
	coordMock := &ai.MockClient{
		GenerateResponse: &ai.GenerateResponse{
			Text: `{"fault_diagnosis": "UNKNOWN", "confidence_score": 0.5, "reasoning": "Unclear", "proposed_action": {"type": "DROP_TABLE", "key": "", "value": ""}}`,
		},
	}

	jury := Jury{
		Coordinator: coordMock,
		Architect:   &ai.MockClient{},
		Reviewer:    &ai.MockClient{},
	}

	engine := NewConsensusEngine(jury, nil)
	ctx := context.Background()

	_, err := engine.Diagnose(ctx, DiagnosisRequest{
		ErrorTrace:    "something went wrong",
		MethodContext: "SomeMethod",
		SystemState:   map[string]string{},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid action type proposed")
}

func TestIsValidActionType(t *testing.T) {
	assert.True(t, IsValidActionType("UPDATE_CONFIG"))
	assert.True(t, IsValidActionType("CLEAR_CACHE"))
	assert.True(t, IsValidActionType("RETRY"))
	assert.True(t, IsValidActionType("NO_OP"))
	assert.False(t, IsValidActionType("DROP_TABLE"))
	assert.False(t, IsValidActionType("DELETE"))
	assert.False(t, IsValidActionType(""))
}
