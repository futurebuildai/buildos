// Package ai provides vendor-agnostic AI client abstractions.
// This package enables switching between AI providers without modifying service
// layer code. FutureBuild OS uses Anthropic Claude exclusively.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// =============================================================================
// VENDOR-AGNOSTIC TYPES
// =============================================================================
// These types abstract away vendor-specific representations from the service
// layer, enabling clean dependency injection and testing.
// =============================================================================

// ContentPart represents a single part of multimodal AI input.
// Can contain text, image data, or other media.
type ContentPart struct {
	// Text is the text content of this part.
	// Mutually exclusive with Data, ToolUse, and ToolResult.
	Text string

	// MimeType specifies the media type when Data is present.
	// Examples: "image/jpeg", "image/png", "application/pdf"
	MimeType string

	// Data contains binary data for images, documents, etc.
	// Mutually exclusive with Text.
	Data []byte

	// ToolUse represents a tool call requested by the model.
	// Present in assistant messages when the model wants to use a tool.
	ToolUse *ToolUseBlock

	// ToolResult represents the result of executing a tool.
	// Present in user messages as a response to a tool_use block.
	ToolResult *ToolResultBlock
}

// ToolDefinition describes a tool available to the model.
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolUseBlock represents a tool call from the model.
type ToolUseBlock struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ToolResultBlock represents the result of executing a tool call.
type ToolResultBlock struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error"`
}

// Message represents a single turn in a multi-turn conversation.
// Used for agentic tool-use loops where context accumulates across turns.
type Message struct {
	Role    string        `json:"role"` // "user" or "assistant"
	Content []ContentPart `json:"content"`
}

// GenerateRequest encapsulates all parameters for content generation.
type GenerateRequest struct {
	// Model specifies which AI model to use.
	Model ModelType

	// Parts contains the multimodal input (text, images, etc.)
	// Used for simple single-turn requests. For multi-turn conversations
	// with tool use, use Messages instead.
	Parts []ContentPart

	// Messages contains the multi-turn conversation history.
	// When set, Parts is ignored. Used for agentic tool-use loops.
	Messages []Message

	// SystemPrompt provides system-level instructions to the model.
	// Sent as the "system" field in the Anthropic API.
	SystemPrompt string

	// Tools lists the tools available to the model for this request.
	// When provided, the model may respond with tool_use content blocks.
	Tools []ToolDefinition

	// MaxTokens limits the response length.
	// Zero means use model default.
	MaxTokens int

	// Temperature controls randomness (0.0 = deterministic, 1.0 = creative).
	// Zero means use model default.
	Temperature float32
}

// GenerateResponse contains the result of content generation.
type GenerateResponse struct {
	// Text is the generated text content.
	Text string

	// TokensUsed is the total token count (input + output).
	TokensUsed int

	// Confidence is the model confidence score (0.0 to 1.0).
	Confidence float32

	// ToolCalls contains tool-use requests from the model.
	// Non-empty when StopReason is "tool_use".
	ToolCalls []ToolUseBlock

	// StopReason indicates why the model stopped generating.
	// Values: "end_turn" (done), "tool_use" (wants tool results),
	// "max_tokens" (hit limit), "stop_sequence".
	StopReason string

	// RawContent preserves the full content blocks from the response.
	// Used by AgentRunner to reconstruct the assistant message
	// (including both text and tool_use blocks) for multi-turn conversations.
	RawContent []ContentPart
}

// ModelType specifies the AI model to use for generation.
// FutureBuild OS uses Anthropic Claude only.
type ModelType string

const (
	// ModelTypeOpus is Claude Opus 4.6 — primary reasoning model.
	ModelTypeOpus ModelType = "opus"
	// ModelTypeSonnet is Claude Sonnet 4.5 — fast, cost-effective model.
	ModelTypeSonnet ModelType = "sonnet"
)

// DefaultModelMap returns the model ID map for Anthropic models.
func DefaultModelMap() map[ModelType]string {
	return map[ModelType]string{
		ModelTypeOpus:   "claude-opus-4-6",
		ModelTypeSonnet: "claude-sonnet-4-5-20250929",
	}
}

// StreamChunk represents a single chunk in a streaming response.
type StreamChunk struct {
	Text       string           `json:"text,omitempty"`
	ToolUse    *ToolUseBlock    `json:"tool_use,omitempty"`
	ToolResult *ToolResultBlock `json:"tool_result,omitempty"`
	Done       bool             `json:"done"`
	StopReason string           `json:"stop_reason,omitempty"`
}

// Client defines the interface for AI operations.
// Uses vendor-agnostic types from types.go.
type Client interface {
	// GenerateContent generates text/multimodal content using the specified model.
	GenerateContent(ctx context.Context, req GenerateRequest) (GenerateResponse, error)

	// GenerateEmbedding generates a vector embedding for the given text.
	GenerateEmbedding(ctx context.Context, text string) ([]float32, error)

	// Close releases any resources used by the client.
	Close() error
}

// StreamingClient extends Client with streaming capabilities.
type StreamingClient interface {
	Client
	// StreamGenerateContent streams response chunks as they're generated.
	StreamGenerateContent(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error)
}

// NoOpClient is a stub AI client that returns errors for all operations.
// Used when AI credentials are not configured (demo/development mode).
type NoOpClient struct{}

func (n *NoOpClient) GenerateContent(_ context.Context, _ GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{}, fmt.Errorf("AI not configured: no Anthropic credentials provided")
}

func (n *NoOpClient) GenerateEmbedding(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("AI not configured: embeddings require a configured AI client")
}

func (n *NoOpClient) Close() error { return nil }

// NewTextRequest creates a simple text-only GenerateRequest.
func NewTextRequest(model ModelType, text string) GenerateRequest {
	return GenerateRequest{
		Model: model,
		Parts: []ContentPart{{Text: text}},
	}
}

// NewMultimodalRequest creates a GenerateRequest with text and image.
func NewMultimodalRequest(model ModelType, text string, imageData []byte, mimeType string) GenerateRequest {
	return GenerateRequest{
		Model: model,
		Parts: []ContentPart{
			{Text: text},
			{Data: imageData, MimeType: mimeType},
		},
	}
}

// NewAgentRequest creates a GenerateRequest for agentic tool-use conversations.
func NewAgentRequest(model ModelType, systemPrompt string, messages []Message, tools []ToolDefinition) GenerateRequest {
	return GenerateRequest{
		Model:        model,
		SystemPrompt: systemPrompt,
		Messages:     messages,
		Tools:        tools,
		MaxTokens:    8192,
	}
}
