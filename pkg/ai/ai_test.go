package ai

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

// =============================================================================
// Model Type Constants
// =============================================================================

func TestModelTypeConstants(t *testing.T) {
	if ModelTypeOpus != "opus" {
		t.Errorf("ModelTypeOpus = %q, want 'opus'", ModelTypeOpus)
	}
	if ModelTypeSonnet != "sonnet" {
		t.Errorf("ModelTypeSonnet = %q, want 'sonnet'", ModelTypeSonnet)
	}
}

func TestDefaultModelMap(t *testing.T) {
	m := DefaultModelMap()

	opusID, ok := m[ModelTypeOpus]
	if !ok {
		t.Fatal("ModelTypeOpus not found in DefaultModelMap")
	}
	if opusID != "claude-opus-4-6" {
		t.Errorf("Opus model ID = %q, want 'claude-opus-4-6'", opusID)
	}

	sonnetID, ok := m[ModelTypeSonnet]
	if !ok {
		t.Fatal("ModelTypeSonnet not found in DefaultModelMap")
	}
	if sonnetID != "claude-sonnet-4-5-20250929" {
		t.Errorf("Sonnet model ID = %q, want 'claude-sonnet-4-5-20250929'", sonnetID)
	}

	if len(m) != 2 {
		t.Errorf("DefaultModelMap has %d entries, want 2", len(m))
	}
}

// =============================================================================
// Request Construction
// =============================================================================

func TestNewTextRequest(t *testing.T) {
	req := NewTextRequest(ModelTypeSonnet, "Hello, world")
	if req.Model != ModelTypeSonnet {
		t.Errorf("Model = %q, want %q", req.Model, ModelTypeSonnet)
	}
	if len(req.Parts) != 1 {
		t.Fatalf("Parts length = %d, want 1", len(req.Parts))
	}
	if req.Parts[0].Text != "Hello, world" {
		t.Errorf("Parts[0].Text = %q, want 'Hello, world'", req.Parts[0].Text)
	}
	if len(req.Messages) != 0 {
		t.Error("Messages should be empty for text request")
	}
}

func TestNewMultimodalRequest(t *testing.T) {
	imgData := []byte{0xFF, 0xD8, 0xFF} // fake JPEG header
	req := NewMultimodalRequest(ModelTypeOpus, "Describe this image", imgData, "image/jpeg")

	if req.Model != ModelTypeOpus {
		t.Errorf("Model = %q, want %q", req.Model, ModelTypeOpus)
	}
	if len(req.Parts) != 2 {
		t.Fatalf("Parts length = %d, want 2", len(req.Parts))
	}
	if req.Parts[0].Text != "Describe this image" {
		t.Errorf("Parts[0].Text = %q, want 'Describe this image'", req.Parts[0].Text)
	}
	if len(req.Parts[1].Data) != 3 {
		t.Errorf("Parts[1].Data length = %d, want 3", len(req.Parts[1].Data))
	}
	if req.Parts[1].MimeType != "image/jpeg" {
		t.Errorf("Parts[1].MimeType = %q, want 'image/jpeg'", req.Parts[1].MimeType)
	}
}

func TestNewAgentRequest(t *testing.T) {
	tools := []ToolDefinition{
		{
			Name:        "test_tool",
			Description: "A test tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
	}
	messages := []Message{
		{Role: "user", Content: []ContentPart{{Text: "Use the tool"}}},
	}

	req := NewAgentRequest(ModelTypeOpus, "You are a helpful assistant", messages, tools)

	if req.Model != ModelTypeOpus {
		t.Errorf("Model = %q, want %q", req.Model, ModelTypeOpus)
	}
	if req.SystemPrompt != "You are a helpful assistant" {
		t.Errorf("SystemPrompt = %q, want 'You are a helpful assistant'", req.SystemPrompt)
	}
	if len(req.Messages) != 1 {
		t.Fatalf("Messages length = %d, want 1", len(req.Messages))
	}
	if req.Messages[0].Role != "user" {
		t.Errorf("Messages[0].Role = %q, want 'user'", req.Messages[0].Role)
	}
	if len(req.Tools) != 1 {
		t.Fatalf("Tools length = %d, want 1", len(req.Tools))
	}
	if req.Tools[0].Name != "test_tool" {
		t.Errorf("Tools[0].Name = %q, want 'test_tool'", req.Tools[0].Name)
	}
	if req.MaxTokens != 8192 {
		t.Errorf("MaxTokens = %d, want 8192", req.MaxTokens)
	}
}

// =============================================================================
// Type Validation
// =============================================================================

func TestContentPart_TextOnly(t *testing.T) {
	p := ContentPart{Text: "hello"}
	if p.Text != "hello" {
		t.Errorf("Text = %q, want 'hello'", p.Text)
	}
	if p.ToolUse != nil {
		t.Error("ToolUse should be nil for text-only part")
	}
	if p.ToolResult != nil {
		t.Error("ToolResult should be nil for text-only part")
	}
}

func TestContentPart_ToolUse(t *testing.T) {
	toolUse := &ToolUseBlock{
		ID:    "toolu_123",
		Name:  "my_tool",
		Input: json.RawMessage(`{"key":"value"}`),
	}
	p := ContentPart{ToolUse: toolUse}
	if p.ToolUse == nil {
		t.Fatal("ToolUse should not be nil")
	}
	if p.ToolUse.ID != "toolu_123" {
		t.Errorf("ToolUse.ID = %q, want 'toolu_123'", p.ToolUse.ID)
	}
	if p.ToolUse.Name != "my_tool" {
		t.Errorf("ToolUse.Name = %q, want 'my_tool'", p.ToolUse.Name)
	}
}

func TestContentPart_ToolResult(t *testing.T) {
	toolResult := &ToolResultBlock{
		ToolUseID: "toolu_123",
		Content:   `{"result":"success"}`,
		IsError:   false,
	}
	p := ContentPart{ToolResult: toolResult}
	if p.ToolResult == nil {
		t.Fatal("ToolResult should not be nil")
	}
	if p.ToolResult.ToolUseID != "toolu_123" {
		t.Errorf("ToolResult.ToolUseID = %q, want 'toolu_123'", p.ToolResult.ToolUseID)
	}
	if p.ToolResult.IsError {
		t.Error("ToolResult.IsError should be false")
	}
}

func TestToolDefinition_JSONRoundTrip(t *testing.T) {
	td := ToolDefinition{
		Name:        "get_weather",
		Description: "Get weather for a location",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"location":{"type":"string"}},"required":["location"]}`),
	}

	b, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded ToolDefinition
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Name != td.Name {
		t.Errorf("Name = %q, want %q", decoded.Name, td.Name)
	}
	if decoded.Description != td.Description {
		t.Errorf("Description = %q, want %q", decoded.Description, td.Description)
	}

	// Verify input_schema round-trips correctly
	var schema map[string]interface{}
	if err := json.Unmarshal(decoded.InputSchema, &schema); err != nil {
		t.Fatalf("InputSchema is invalid JSON: %v", err)
	}
	if schema["type"] != "object" {
		t.Errorf("InputSchema type = %v, want 'object'", schema["type"])
	}
}

func TestMessage_JSONSerialization(t *testing.T) {
	msg := Message{
		Role: "user",
		Content: []ContentPart{
			{Text: "Hello"},
			{Text: "World"},
		},
	}

	b, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Role != "user" {
		t.Errorf("Role = %q, want 'user'", decoded.Role)
	}
	if len(decoded.Content) != 2 {
		t.Errorf("Content length = %d, want 2", len(decoded.Content))
	}
}

func TestGenerateResponse_Fields(t *testing.T) {
	resp := GenerateResponse{
		Text:       "The answer is 42.",
		TokensUsed: 150,
		Confidence: 0.95,
		StopReason: "end_turn",
		ToolCalls: []ToolUseBlock{
			{ID: "tc_1", Name: "calc", Input: json.RawMessage(`{}`)},
		},
		RawContent: []ContentPart{
			{Text: "The answer is 42."},
		},
	}

	if resp.Text != "The answer is 42." {
		t.Errorf("Text = %q", resp.Text)
	}
	if resp.TokensUsed != 150 {
		t.Errorf("TokensUsed = %d, want 150", resp.TokensUsed)
	}
	if resp.Confidence != 0.95 {
		t.Errorf("Confidence = %f, want 0.95", resp.Confidence)
	}
	if resp.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want 'end_turn'", resp.StopReason)
	}
	if len(resp.ToolCalls) != 1 {
		t.Errorf("ToolCalls length = %d, want 1", len(resp.ToolCalls))
	}
	if len(resp.RawContent) != 1 {
		t.Errorf("RawContent length = %d, want 1", len(resp.RawContent))
	}
}

func TestStreamChunk_Fields(t *testing.T) {
	// Text chunk
	textChunk := StreamChunk{Text: "hello"}
	if textChunk.Text != "hello" {
		t.Errorf("Text = %q, want 'hello'", textChunk.Text)
	}
	if textChunk.Done {
		t.Error("Done should be false for text chunk")
	}

	// Done chunk
	doneChunk := StreamChunk{Done: true, StopReason: "end_turn"}
	if !doneChunk.Done {
		t.Error("Done should be true")
	}
	if doneChunk.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want 'end_turn'", doneChunk.StopReason)
	}

	// Tool use chunk
	toolChunk := StreamChunk{
		ToolUse: &ToolUseBlock{ID: "tc_1", Name: "test", Input: json.RawMessage(`{}`)},
	}
	if toolChunk.ToolUse == nil {
		t.Fatal("ToolUse should not be nil")
	}
	if toolChunk.ToolUse.Name != "test" {
		t.Errorf("ToolUse.Name = %q, want 'test'", toolChunk.ToolUse.Name)
	}
}

// =============================================================================
// MockClient Tests
// =============================================================================

func TestMockClient_ImplementsClientInterface(t *testing.T) {
	// Compile-time interface check
	var _ Client = (*MockClient)(nil)
}

func TestMockClient_GenerateContent_DefaultNilResponse(t *testing.T) {
	mock := &MockClient{}
	_, err := mock.GenerateContent(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("expected error when GenerateResponse is nil")
	}
	if err.Error() != "mock response not configured" {
		t.Errorf("error = %q, want 'mock response not configured'", err.Error())
	}
}

func TestMockClient_GenerateContent_ConfiguredResponse(t *testing.T) {
	mock := &MockClient{}
	mock.SetResponse(GenerateResponse{
		Text:       "Mocked response",
		TokensUsed: 42,
		StopReason: "end_turn",
	})

	resp, err := mock.GenerateContent(context.Background(), NewTextRequest(ModelTypeSonnet, "test"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Text != "Mocked response" {
		t.Errorf("Text = %q, want 'Mocked response'", resp.Text)
	}
	if resp.TokensUsed != 42 {
		t.Errorf("TokensUsed = %d, want 42", resp.TokensUsed)
	}
}

func TestMockClient_GenerateContent_ConfiguredError(t *testing.T) {
	mock := &MockClient{}
	expectedErr := errors.New("rate limit exceeded")
	mock.SetError(expectedErr)

	_, err := mock.GenerateContent(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("error = %v, want %v", err, expectedErr)
	}
}

func TestMockClient_GenerateContent_ErrorTakesPrecedence(t *testing.T) {
	mock := &MockClient{}
	mock.SetResponse(GenerateResponse{Text: "should not appear"})
	mock.SetError(errors.New("forced error"))

	_, err := mock.GenerateContent(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("expected error to take precedence over response")
	}
}

func TestMockClient_GenerateContent_RecordsCalls(t *testing.T) {
	mock := &MockClient{}
	mock.SetResponse(GenerateResponse{Text: "ok"})

	req1 := NewTextRequest(ModelTypeSonnet, "first")
	req2 := NewTextRequest(ModelTypeOpus, "second")

	_, _ = mock.GenerateContent(context.Background(), req1)
	_, _ = mock.GenerateContent(context.Background(), req2)

	if len(mock.GenerateCalls) != 2 {
		t.Fatalf("GenerateCalls length = %d, want 2", len(mock.GenerateCalls))
	}
	if mock.GenerateCalls[0].Model != ModelTypeSonnet {
		t.Errorf("call[0].Model = %q, want %q", mock.GenerateCalls[0].Model, ModelTypeSonnet)
	}
	if mock.GenerateCalls[1].Model != ModelTypeOpus {
		t.Errorf("call[1].Model = %q, want %q", mock.GenerateCalls[1].Model, ModelTypeOpus)
	}
}

func TestMockClient_GenerateEmbedding_Default(t *testing.T) {
	mock := &MockClient{}
	resp, err := mock.GenerateEmbedding(context.Background(), "test text")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != nil {
		t.Errorf("expected nil response, got %v", resp)
	}
	if len(mock.EmbeddingCalls) != 1 {
		t.Fatalf("EmbeddingCalls length = %d, want 1", len(mock.EmbeddingCalls))
	}
	if mock.EmbeddingCalls[0] != "test text" {
		t.Errorf("EmbeddingCalls[0] = %q, want 'test text'", mock.EmbeddingCalls[0])
	}
}

func TestMockClient_GenerateEmbedding_WithResponse(t *testing.T) {
	mock := &MockClient{
		EmbeddingResponse: []float32{0.1, 0.2, 0.3},
	}

	resp, err := mock.GenerateEmbedding(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp) != 3 {
		t.Fatalf("embedding length = %d, want 3", len(resp))
	}
	if resp[0] != 0.1 {
		t.Errorf("resp[0] = %f, want 0.1", resp[0])
	}
}

func TestMockClient_GenerateEmbedding_WithError(t *testing.T) {
	mock := &MockClient{
		EmbeddingErr: errors.New("embedding error"),
	}

	_, err := mock.GenerateEmbedding(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMockClient_Close(t *testing.T) {
	mock := &MockClient{}
	if err := mock.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestMockClient_ThreadSafety(t *testing.T) {
	mock := &MockClient{}
	mock.SetResponse(GenerateResponse{Text: "safe"})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mock.GenerateContent(context.Background(), NewTextRequest(ModelTypeSonnet, "test"))
		}()
	}
	wg.Wait()

	mock.mu.Lock()
	callCount := len(mock.GenerateCalls)
	mock.mu.Unlock()

	if callCount != 100 {
		t.Errorf("expected 100 calls, got %d", callCount)
	}
}

func TestMockClient_ThreadSafety_Embeddings(t *testing.T) {
	mock := &MockClient{
		EmbeddingResponse: []float32{0.5},
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, _ = mock.GenerateEmbedding(context.Background(), "text")
		}(i)
	}
	wg.Wait()

	mock.mu.Lock()
	callCount := len(mock.EmbeddingCalls)
	mock.mu.Unlock()

	if callCount != 50 {
		t.Errorf("expected 50 embedding calls, got %d", callCount)
	}
}

// =============================================================================
// NoOpClient Tests
// =============================================================================

func TestNoOpClient_ImplementsClientInterface(t *testing.T) {
	var _ Client = (*NoOpClient)(nil)
}

func TestNoOpClient_GenerateContent_ReturnsError(t *testing.T) {
	noop := &NoOpClient{}
	_, err := noop.GenerateContent(context.Background(), GenerateRequest{})
	if err == nil {
		t.Fatal("expected error from NoOpClient.GenerateContent")
	}
	if err.Error() != "AI not configured: no Anthropic credentials provided" {
		t.Errorf("error = %q", err.Error())
	}
}

func TestNoOpClient_GenerateEmbedding_ReturnsError(t *testing.T) {
	noop := &NoOpClient{}
	_, err := noop.GenerateEmbedding(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error from NoOpClient.GenerateEmbedding")
	}
}

func TestNoOpClient_Close_NoError(t *testing.T) {
	noop := &NoOpClient{}
	if err := noop.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

// =============================================================================
// AnthropicClient Tests (unit-level, no actual API calls)
// =============================================================================

func TestAnthropicClient_ImplementsStreamingClientInterface(t *testing.T) {
	var _ StreamingClient = (*AnthropicClient)(nil)
}

func TestNewAnthropicClient(t *testing.T) {
	client := NewAnthropicClient("test-key", DefaultModelMap())
	if client == nil {
		t.Fatal("NewAnthropicClient returned nil")
	}
	if client.apiKey != "test-key" {
		t.Errorf("apiKey = %q, want 'test-key'", client.apiKey)
	}
}

func TestAnthropicClient_GenerateEmbedding_Unsupported(t *testing.T) {
	client := NewAnthropicClient("key", DefaultModelMap())
	_, err := client.GenerateEmbedding(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error for unsupported operation")
	}
	if err.Error() != "embeddings not supported by Anthropic client" {
		t.Errorf("error = %q", err.Error())
	}
}

func TestAnthropicClient_Close(t *testing.T) {
	client := NewAnthropicClient("key", DefaultModelMap())
	if err := client.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestAnthropicClient_GenerateContent_UnknownModel(t *testing.T) {
	client := NewAnthropicClient("key", DefaultModelMap())
	_, err := client.GenerateContent(context.Background(), GenerateRequest{
		Model: ModelType("unknown_model"),
		Parts: []ContentPart{{Text: "test"}},
	})
	if err == nil {
		t.Fatal("expected error for unknown model type")
	}
	if err.Error() != "model type unknown_model not configured for Anthropic" {
		t.Errorf("error = %q", err.Error())
	}
}

func TestAnthropicClient_StreamGenerateContent_UnknownModel(t *testing.T) {
	client := NewAnthropicClient("key", DefaultModelMap())
	_, err := client.StreamGenerateContent(context.Background(), GenerateRequest{
		Model: ModelType("unknown_model"),
		Parts: []ContentPart{{Text: "test"}},
	})
	if err == nil {
		t.Fatal("expected error for unknown model type")
	}
}

// =============================================================================
// GenerateRequest Defaults
// =============================================================================

func TestGenerateRequest_ZeroValues(t *testing.T) {
	req := GenerateRequest{}
	if req.MaxTokens != 0 {
		t.Errorf("MaxTokens default = %d, want 0", req.MaxTokens)
	}
	if req.Temperature != 0 {
		t.Errorf("Temperature default = %f, want 0", req.Temperature)
	}
	if req.SystemPrompt != "" {
		t.Error("SystemPrompt should be empty by default")
	}
	if len(req.Tools) != 0 {
		t.Error("Tools should be empty by default")
	}
	if len(req.Messages) != 0 {
		t.Error("Messages should be empty by default")
	}
}
