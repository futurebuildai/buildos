package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

// MaxProgressChangePct is the safety limit: any estimated progress change
// greater than this percentage requires human review before being applied.
const MaxProgressChangePct = 50.0

// VerificationResult holds the output of a vision-based progress verification.
type VerificationResult struct {
	// TaskID is the ID of the task that was verified.
	TaskID uuid.UUID `json:"task_id"`

	// EstimatedProgress is the AI-estimated completion percentage (0-100).
	EstimatedProgress int `json:"estimated_progress"`

	// Confidence is the model's confidence in the estimate (0.0 to 1.0).
	Confidence float64 `json:"confidence"`

	// Notes is a free-form explanation from the AI model.
	Notes string `json:"notes"`

	// Issues lists any problems or concerns detected in the photo.
	Issues []string `json:"issues,omitempty"`

	// RequiresReview is true when the estimated progress deviates from
	// expected progress by more than MaxProgressChangePct.
	RequiresReview bool `json:"requires_review"`
}

// visionVerificationPrompt is the system prompt for Claude vision analysis.
const visionVerificationPrompt = `You are an AI construction progress analyst reviewing a photo of a construction task.
Your job is to estimate the actual completion percentage visible in the photo, and flag any quality or safety issues.

## Task Context
- Task ID: %s
- Expected progress: %d%%

## Instructions
1. Examine the photo carefully for visible construction progress.
2. Estimate the completion percentage (0-100) based on what you see.
3. Note your confidence level (0.0-1.0) in the estimate.
4. List any issues you observe (safety concerns, quality problems, deviations from expected work).
5. Provide a brief explanation of your assessment.

## Response Format
Respond with ONLY valid JSON in this exact format:
{
  "estimated_progress": <integer 0-100>,
  "confidence": <float 0.0-1.0>,
  "notes": "<brief explanation>",
  "issues": ["<issue 1>", "<issue 2>"]
}

Be conservative: if you cannot clearly determine progress, use a lower confidence score.
If the photo is unclear, dark, or does not appear to show the expected work, note this in issues.`

// VisionVerificationAgent uses Claude's vision capabilities to verify
// construction progress from photos.
type VisionVerificationAgent struct {
	aiClient ai.Client
}

// NewVisionVerificationAgent creates a new VisionVerificationAgent.
func NewVisionVerificationAgent(aiClient ai.Client) *VisionVerificationAgent {
	return &VisionVerificationAgent{aiClient: aiClient}
}

// VerifyProgress sends a photo to Claude with task context and returns
// a progress assessment. The photo is referenced by URL.
//
// Safety: if the estimated progress deviates from expectedProgress by more
// than MaxProgressChangePct, RequiresReview is set to true.
func (a *VisionVerificationAgent) VerifyProgress(
	ctx context.Context,
	taskID uuid.UUID,
	photoURL string,
	expectedProgress int,
) (*VerificationResult, error) {
	prompt := fmt.Sprintf(visionVerificationPrompt, taskID, expectedProgress)

	req := ai.GenerateRequest{
		Model: ai.ModelTypeSonnet, // Use Sonnet for cost-effective vision
		Parts: []ai.ContentPart{
			{Text: prompt},
			{
				Text:     fmt.Sprintf("Photo URL for analysis: %s", photoURL),
				MimeType: "image/jpeg",
			},
		},
		MaxTokens:   1024,
		Temperature: 0.1, // Low temperature for consistent, factual responses
	}

	resp, err := a.aiClient.GenerateContent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("vision verification: AI call failed: %w", err)
	}

	// Parse the JSON response from Claude
	result, err := parseVisionResponse(resp.Text)
	if err != nil {
		slog.Warn("vision verification: failed to parse AI response, using raw text",
			"task_id", taskID,
			"raw_response", resp.Text,
			"error", err,
		)
		// Return a low-confidence result that requires review
		return &VerificationResult{
			TaskID:            taskID,
			EstimatedProgress: expectedProgress,
			Confidence:        0.0,
			Notes:             fmt.Sprintf("AI response could not be parsed: %s", resp.Text),
			Issues:            []string{"AI response format error — manual review required"},
			RequiresReview:    true,
		}, nil
	}

	result.TaskID = taskID

	// Safety check: flag for human review if progress change exceeds threshold
	progressDelta := math.Abs(float64(result.EstimatedProgress - expectedProgress))
	if progressDelta > MaxProgressChangePct {
		result.RequiresReview = true
		slog.Info("vision verification: large progress delta requires review",
			"task_id", taskID,
			"expected", expectedProgress,
			"estimated", result.EstimatedProgress,
			"delta", progressDelta,
		)
	}

	slog.Info("vision verification completed",
		"task_id", taskID,
		"estimated_progress", result.EstimatedProgress,
		"confidence", result.Confidence,
		"requires_review", result.RequiresReview,
		"issue_count", len(result.Issues),
	)

	return result, nil
}

// parseVisionResponse parses the JSON output from Claude's vision analysis.
func parseVisionResponse(text string) (*VerificationResult, error) {
	var parsed struct {
		EstimatedProgress int      `json:"estimated_progress"`
		Confidence        float64  `json:"confidence"`
		Notes             string   `json:"notes"`
		Issues            []string `json:"issues"`
	}

	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("parsing vision response JSON: %w", err)
	}

	// Clamp values to valid ranges
	if parsed.EstimatedProgress < 0 {
		parsed.EstimatedProgress = 0
	}
	if parsed.EstimatedProgress > 100 {
		parsed.EstimatedProgress = 100
	}
	if parsed.Confidence < 0 {
		parsed.Confidence = 0
	}
	if parsed.Confidence > 1.0 {
		parsed.Confidence = 1.0
	}

	return &VerificationResult{
		EstimatedProgress: parsed.EstimatedProgress,
		Confidence:        parsed.Confidence,
		Notes:             parsed.Notes,
		Issues:            parsed.Issues,
		RequiresReview:    false, // Caller sets this based on delta check
	}, nil
}
