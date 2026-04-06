package agents

import (
	"context"
	"fmt"
	"testing"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

func TestVisionVerificationAgent_VerifyProgress(t *testing.T) {
	taskID := uuid.New()

	tests := []struct {
		name             string
		mockResponse     string
		mockError        error
		expectedProgress int
		photoURL         string
		wantProgress     int
		wantReview       bool
		wantErr          bool
	}{
		{
			name: "normal_progress_within_threshold",
			mockResponse: `{
				"estimated_progress": 45,
				"confidence": 0.85,
				"notes": "Framing appears roughly 45% complete based on visible wall sections.",
				"issues": []
			}`,
			expectedProgress: 40,
			photoURL:         "https://storage.example.com/photos/task1.jpg",
			wantProgress:     45,
			wantReview:       false,
			wantErr:          false,
		},
		{
			name: "large_progress_delta_requires_review",
			mockResponse: `{
				"estimated_progress": 95,
				"confidence": 0.70,
				"notes": "Task appears nearly complete.",
				"issues": []
			}`,
			expectedProgress: 20,
			photoURL:         "https://storage.example.com/photos/task2.jpg",
			wantProgress:     95,
			wantReview:       true, // 75% delta > MaxProgressChangePct (50%)
			wantErr:          false,
		},
		{
			name: "progress_delta_exactly_at_threshold",
			mockResponse: `{
				"estimated_progress": 80,
				"confidence": 0.90,
				"notes": "Good progress visible.",
				"issues": []
			}`,
			expectedProgress: 30,
			photoURL:         "https://storage.example.com/photos/task3.jpg",
			wantProgress:     80,
			wantReview:       false, // 50% delta = MaxProgressChangePct, not exceeded
			wantErr:          false,
		},
		{
			name: "progress_delta_just_above_threshold",
			mockResponse: `{
				"estimated_progress": 82,
				"confidence": 0.90,
				"notes": "Good progress visible.",
				"issues": []
			}`,
			expectedProgress: 30,
			photoURL:         "https://storage.example.com/photos/task3b.jpg",
			wantProgress:     82,
			wantReview:       true, // 52% delta > MaxProgressChangePct (50%)
			wantErr:          false,
		},
		{
			name: "issues_detected",
			mockResponse: `{
				"estimated_progress": 30,
				"confidence": 0.60,
				"notes": "Work in progress but safety concerns noted.",
				"issues": ["Missing fall protection", "Debris on walkway"]
			}`,
			expectedProgress: 25,
			photoURL:         "https://storage.example.com/photos/task4.jpg",
			wantProgress:     30,
			wantReview:       false,
			wantErr:          false,
		},
		{
			name:             "ai_client_error",
			mockError:        fmt.Errorf("API rate limit exceeded"),
			expectedProgress: 50,
			photoURL:         "https://storage.example.com/photos/error.jpg",
			wantErr:          true,
		},
		{
			name: "unparseable_response_returns_review_required",
			mockResponse: `This is not valid JSON at all.
The model returned a narrative response instead.`,
			expectedProgress: 40,
			photoURL:         "https://storage.example.com/photos/bad.jpg",
			wantProgress:     40, // Falls back to expected progress
			wantReview:       true,
			wantErr:          false,
		},
		{
			name: "clamped_values",
			mockResponse: `{
				"estimated_progress": 150,
				"confidence": 2.0,
				"notes": "Model returned out-of-range values.",
				"issues": []
			}`,
			expectedProgress: 40,
			photoURL:         "https://storage.example.com/photos/clamp.jpg",
			wantProgress:     100, // Clamped from 150
			wantReview:       true, // 60% delta > 50%
			wantErr:          false,
		},
		{
			name: "negative_progress_clamped_to_zero",
			mockResponse: `{
				"estimated_progress": -10,
				"confidence": -0.5,
				"notes": "Negative values.",
				"issues": []
			}`,
			expectedProgress: 40,
			photoURL:         "https://storage.example.com/photos/negative.jpg",
			wantProgress:     0, // Clamped from -10
			wantReview:       false,
			wantErr:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &ai.MockClient{}
			if tt.mockError != nil {
				mockClient.SetError(tt.mockError)
			} else {
				mockClient.SetResponse(ai.GenerateResponse{
					Text:       tt.mockResponse,
					StopReason: "end_turn",
					TokensUsed: 100,
				})
			}

			agent := NewVisionVerificationAgent(mockClient)
			result, err := agent.VerifyProgress(context.Background(), taskID, tt.photoURL, tt.expectedProgress)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.TaskID != taskID {
				t.Errorf("TaskID = %v, want %v", result.TaskID, taskID)
			}
			if result.EstimatedProgress != tt.wantProgress {
				t.Errorf("EstimatedProgress = %d, want %d", result.EstimatedProgress, tt.wantProgress)
			}
			if result.RequiresReview != tt.wantReview {
				t.Errorf("RequiresReview = %v, want %v", result.RequiresReview, tt.wantReview)
			}

			// Verify at least one call was made to the AI client.
			// GenerateCalls is safe to read here since the agent call has returned.
			if len(mockClient.GenerateCalls) != 1 {
				t.Errorf("expected 1 AI call, got %d", len(mockClient.GenerateCalls))
			}
		})
	}
}

func TestVisionVerificationAgent_SafetyLimit(t *testing.T) {
	// Specifically test the MaxProgressChangePct = 50 safety boundary.
	mockClient := &ai.MockClient{}
	mockClient.SetResponse(ai.GenerateResponse{
		Text:       `{"estimated_progress": 80, "confidence": 0.9, "notes": "Looks good", "issues": []}`,
		StopReason: "end_turn",
		TokensUsed: 50,
	})

	agent := NewVisionVerificationAgent(mockClient)
	taskID := uuid.New()

	// Test: 80 - 25 = 55 delta > 50 threshold => requires review
	result, err := agent.VerifyProgress(context.Background(), taskID, "https://example.com/photo.jpg", 25)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.RequiresReview {
		t.Error("expected RequiresReview=true for 55% delta (80-25), got false")
	}

	// Test: 80 - 30 = 50 delta = 50 threshold => does NOT require review (not strictly greater)
	mockClient.SetResponse(ai.GenerateResponse{
		Text:       `{"estimated_progress": 80, "confidence": 0.9, "notes": "Looks good", "issues": []}`,
		StopReason: "end_turn",
		TokensUsed: 50,
	})

	result, err = agent.VerifyProgress(context.Background(), taskID, "https://example.com/photo.jpg", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.RequiresReview {
		t.Error("expected RequiresReview=false for exactly 50% delta (80-30), got true")
	}
}

func TestParseVisionResponse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "valid_json",
			input:   `{"estimated_progress": 50, "confidence": 0.8, "notes": "Half done", "issues": []}`,
			wantErr: false,
		},
		{
			name:    "invalid_json",
			input:   `not json`,
			wantErr: true,
		},
		{
			name:    "empty_string",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseVisionResponse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseVisionResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
