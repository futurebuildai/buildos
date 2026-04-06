package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	anthropicAPIVersion = "2023-06-01"
	anthropicBaseURL    = "https://api.anthropic.com/v1/messages"
)

// AnthropicClient implements Client for Anthropic (Claude).
// Supports system prompts, tool use, and multi-turn conversations.
type AnthropicClient struct {
	apiKey   string
	modelMap map[ModelType]string
	client   *http.Client
}

// NewAnthropicClient creates a new Anthropic client.
func NewAnthropicClient(apiKey string, modelMap map[ModelType]string) *AnthropicClient {
	return &AnthropicClient{
		apiKey:   apiKey,
		modelMap: modelMap,
		client:   &http.Client{Timeout: 120 * time.Second}, // Opus needs longer for reasoning
	}
}

// GenerateContent generates content using the Anthropic Messages API.
// Supports simple text requests (via Parts), multi-turn tool-use conversations
// (via Messages), system prompts, and tool definitions.
func (c *AnthropicClient) GenerateContent(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	modelID, ok := c.modelMap[req.Model]
	if !ok {
		return GenerateResponse{}, fmt.Errorf("model type %s not configured for Anthropic", req.Model)
	}

	anthropicReq := c.buildRequest(modelID, req)

	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicBaseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read body for error details but cap at 512 bytes to prevent OOM
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return GenerateResponse{}, fmt.Errorf("anthropic api error: status %s: %s", resp.Status, string(errBody))
	}

	return c.parseResponse(resp.Body)
}

// buildRequest constructs the Anthropic API request body.
func (c *AnthropicClient) buildRequest(modelID string, req GenerateRequest) map[string]interface{} {
	anthropicReq := map[string]interface{}{
		"model": modelID,
	}

	// Max tokens
	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}
	anthropicReq["max_tokens"] = maxTokens

	// Temperature
	if req.Temperature > 0 {
		anthropicReq["temperature"] = req.Temperature
	}

	// System prompt
	if req.SystemPrompt != "" {
		anthropicReq["system"] = req.SystemPrompt
	}

	// Tools
	if len(req.Tools) > 0 {
		tools := make([]map[string]interface{}, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = map[string]interface{}{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": json.RawMessage(t.InputSchema),
			}
		}
		anthropicReq["tools"] = tools
	}

	// Messages: prefer Messages field, fall back to Parts for backward compat
	if len(req.Messages) > 0 {
		anthropicReq["messages"] = messagesToAnthropic(req.Messages)
	} else {
		anthropicReq["messages"] = []map[string]interface{}{
			{
				"role":    "user",
				"content": partsToAnthropic(req.Parts),
			},
		}
	}

	return anthropicReq
}

// parseResponse decodes the Anthropic API response into a GenerateResponse.
func (c *AnthropicClient) parseResponse(body io.Reader) (GenerateResponse, error) {
	var anthropicResp struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text,omitempty"`
			ID    string          `json:"id,omitempty"`
			Name  string          `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		StopReason string `json:"stop_reason"`
		Usage      struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.NewDecoder(body).Decode(&anthropicResp); err != nil {
		return GenerateResponse{}, fmt.Errorf("decode response: %w", err)
	}

	var (
		fullText   string
		toolCalls  []ToolUseBlock
		rawContent []ContentPart
	)

	for _, block := range anthropicResp.Content {
		switch block.Type {
		case "text":
			fullText += block.Text
			rawContent = append(rawContent, ContentPart{Text: block.Text})
		case "tool_use":
			tc := ToolUseBlock{
				ID:    block.ID,
				Name:  block.Name,
				Input: block.Input,
			}
			toolCalls = append(toolCalls, tc)
			rawContent = append(rawContent, ContentPart{ToolUse: &tc})
		}
	}

	return GenerateResponse{
		Text:       fullText,
		TokensUsed: anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		Confidence: 1.0,
		ToolCalls:  toolCalls,
		StopReason: anthropicResp.StopReason,
		RawContent: rawContent,
	}, nil
}

// StreamGenerateContent streams response chunks using Anthropic's streaming API.
// Parses SSE events: message_start, content_block_start, content_block_delta,
// content_block_stop, message_delta, message_stop.
func (c *AnthropicClient) StreamGenerateContent(ctx context.Context, req GenerateRequest) (<-chan StreamChunk, error) {
	modelID, ok := c.modelMap[req.Model]
	if !ok {
		return nil, fmt.Errorf("model type %s not configured for Anthropic", req.Model)
	}

	anthropicReq := c.buildRequest(modelID, req)
	anthropicReq["stream"] = true

	reqBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	// Streaming uses context for cancellation. Add a 10-minute dial/TLS backstop
	// to prevent leaked connections if the caller forgets to cancel the context.
	streamClient := &http.Client{Timeout: 10 * time.Minute}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", anthropicBaseURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic api error: status %s: %s", resp.Status, string(errBody))
	}

	out := make(chan StreamChunk, 64)

	go func() {
		defer close(out)
		defer resp.Body.Close()
		c.parseSSEStream(ctx, resp.Body, out)
	}()

	return out, nil
}

// parseSSEStream reads Anthropic SSE events and sends StreamChunks to the output channel.
func (c *AnthropicClient) parseSSEStream(ctx context.Context, body io.Reader, out chan<- StreamChunk) {
	// Buffer for reading SSE lines. Cap at 10MB to prevent unbounded growth.
	const maxBufSize = 10 << 20
	buf := make([]byte, 0, 4096)
	readBuf := make([]byte, 1024)

	// State for buffering tool_use blocks
	var currentToolUse *ToolUseBlock
	var toolInputBuf bytes.Buffer

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := body.Read(readBuf)
		if n > 0 {
			if len(buf)+n > maxBufSize {
				return // buffer exceeded safety limit
			}
			buf = append(buf, readBuf[:n]...)
		}

		// Process complete SSE events (terminated by \n\n)
		for {
			idx := bytes.Index(buf, []byte("\n\n"))
			if idx == -1 {
				break
			}

			eventBlock := string(buf[:idx])
			buf = buf[idx+2:]

			var eventType, eventData string
			for _, line := range bytes.Split([]byte(eventBlock), []byte("\n")) {
				l := string(line)
				if len(l) == 0 {
					continue
				}
				if len(l) > 6 && l[:6] == "event:" {
					v := l[6:]
					if len(v) > 0 && v[0] == ' ' {
						v = v[1:]
					}
					eventType = v
				} else if len(l) > 5 && l[:5] == "data:" {
					v := l[5:]
					if len(v) > 0 && v[0] == ' ' {
						v = v[1:]
					}
					eventData = v
				}
			}

			if eventData == "" {
				continue
			}

			switch eventType {
			case "content_block_start":
				var block struct {
					ContentBlock struct {
						Type string `json:"type"`
						ID   string `json:"id"`
						Name string `json:"name"`
					} `json:"content_block"`
				}
				if json.Unmarshal([]byte(eventData), &block) == nil && block.ContentBlock.Type == "tool_use" {
					currentToolUse = &ToolUseBlock{
						ID:   block.ContentBlock.ID,
						Name: block.ContentBlock.Name,
					}
					toolInputBuf.Reset()
				}

			case "content_block_delta":
				var delta struct {
					Delta struct {
						Type        string `json:"type"`
						Text        string `json:"text"`
						PartialJSON string `json:"partial_json"`
					} `json:"delta"`
				}
				if json.Unmarshal([]byte(eventData), &delta) == nil {
					switch delta.Delta.Type {
					case "text_delta":
						select {
						case out <- StreamChunk{Text: delta.Delta.Text}:
						case <-ctx.Done():
							return
						}
					case "input_json_delta":
						if currentToolUse != nil {
							toolInputBuf.WriteString(delta.Delta.PartialJSON)
						}
					}
				}

			case "content_block_stop":
				if currentToolUse != nil {
					inputJSON := toolInputBuf.String()
					if inputJSON == "" {
						inputJSON = "{}"
					}
					currentToolUse.Input = json.RawMessage(inputJSON)
					select {
					case out <- StreamChunk{ToolUse: currentToolUse}:
					case <-ctx.Done():
						return
					}
					currentToolUse = nil
				}

			case "message_delta":
				var md struct {
					Delta struct {
						StopReason string `json:"stop_reason"`
					} `json:"delta"`
				}
				if json.Unmarshal([]byte(eventData), &md) == nil && md.Delta.StopReason != "" {
					select {
					case out <- StreamChunk{Done: true, StopReason: md.Delta.StopReason}:
					case <-ctx.Done():
						return
					}
				}

			case "message_stop":
				// Final event - channel will be closed by deferred close
				return
			}
		}

		if err != nil {
			return // EOF or read error
		}
	}
}

// GenerateEmbedding is not supported by Anthropic (they recommend Voyage AI or others).
func (c *AnthropicClient) GenerateEmbedding(_ context.Context, _ string) ([]float32, error) {
	return nil, fmt.Errorf("embeddings not supported by Anthropic client")
}

// Close is a no-op for HTTP client.
func (c *AnthropicClient) Close() error {
	return nil
}

// messagesToAnthropic converts the vendor-agnostic Message slice to Anthropic API format.
func messagesToAnthropic(messages []Message) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(messages))
	for _, msg := range messages {
		result = append(result, map[string]interface{}{
			"role":    msg.Role,
			"content": partsToAnthropic(msg.Content),
		})
	}
	return result
}

// partsToAnthropic converts ContentParts to the Anthropic content format.
// Handles text, images (base64), tool_use, and tool_result blocks.
func partsToAnthropic(parts []ContentPart) interface{} {
	// Simple case: single text part -> return string
	if len(parts) == 1 && parts[0].Text != "" && parts[0].ToolUse == nil && parts[0].ToolResult == nil {
		return parts[0].Text
	}

	blocks := make([]map[string]interface{}, 0, len(parts))
	for _, p := range parts {
		switch {
		case p.ToolResult != nil:
			blocks = append(blocks, map[string]interface{}{
				"type":        "tool_result",
				"tool_use_id": p.ToolResult.ToolUseID,
				"content":     p.ToolResult.Content,
				"is_error":    p.ToolResult.IsError,
			})
		case p.ToolUse != nil:
			blocks = append(blocks, map[string]interface{}{
				"type":  "tool_use",
				"id":    p.ToolUse.ID,
				"name":  p.ToolUse.Name,
				"input": json.RawMessage(p.ToolUse.Input),
			})
		case p.Text != "":
			blocks = append(blocks, map[string]interface{}{
				"type": "text",
				"text": p.Text,
			})
		case len(p.Data) > 0:
			blocks = append(blocks, map[string]interface{}{
				"type": "image",
				"source": map[string]interface{}{
					"type":       "base64",
					"media_type": p.MimeType,
					"data":       base64.StdEncoding.EncodeToString(p.Data),
				},
			})
		}
	}
	return blocks
}
