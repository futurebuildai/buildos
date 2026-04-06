package agents

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/agents/tools"
	"github.com/futurebuild/futurebuild-os/pkg/ai"
)

// StreamAgentRunner executes Claude-powered agent conversations with streaming output.
// Same tool-use loop as AgentRunner, but streams text deltas to an output channel.
type StreamAgentRunner struct {
	aiClient ai.StreamingClient
	tools    *tools.Registry
	maxTurns int
}

// NewStreamAgentRunner creates a streaming runner.
func NewStreamAgentRunner(client ai.StreamingClient, toolRegistry *tools.Registry) *StreamAgentRunner {
	return &StreamAgentRunner{
		aiClient: client,
		tools:    toolRegistry,
		maxTurns: MaxAgentTurns,
	}
}

// RunStreaming executes the agent loop, streaming text deltas and tool events to `out`.
// Returns the final AgentResult for persistence after the stream completes.
func (r *StreamAgentRunner) RunStreaming(
	ctx context.Context,
	systemPrompt string,
	messages []ai.Message,
	projectCtx ProjectContext,
	out chan<- ai.StreamChunk,
) (*AgentResult, error) {
	start := time.Now()

	scopedCtx := tools.WithScope(ctx, tools.Scope{
		ProjectID: projectCtx.ProjectID,
		OrgID:     projectCtx.OrgID,
		UserID:    projectCtx.UserID,
	})

	toolDefs := r.tools.Definitions()
	result := &AgentResult{}

	// Copy messages to avoid mutating caller's slice
	msgs := make([]ai.Message, len(messages))
	copy(msgs, messages)

	for turn := 0; turn < r.maxTurns; turn++ {
		result.Turns = turn + 1

		req := ai.NewAgentRequest(ai.ModelTypeOpus, systemPrompt, msgs, toolDefs)
		stream, err := r.aiClient.StreamGenerateContent(scopedCtx, req)
		if err != nil {
			return nil, fmt.Errorf("agent stream turn %d: %w", turn, err)
		}

		// Collect the full response while streaming deltas to the caller
		var textBuf strings.Builder
		var rawContent []ai.ContentPart
		var toolCalls []ai.ToolUseBlock
		var stopReason string

		for chunk := range stream {
			if chunk.Text != "" {
				textBuf.WriteString(chunk.Text)
				// Forward text delta to the caller
				select {
				case out <- ai.StreamChunk{Text: chunk.Text}:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}

			if chunk.ToolUse != nil {
				toolCalls = append(toolCalls, *chunk.ToolUse)
				rawContent = append(rawContent, ai.ContentPart{ToolUse: chunk.ToolUse})
				// Notify caller of tool invocation
				select {
				case out <- ai.StreamChunk{ToolUse: chunk.ToolUse}:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}

			if chunk.Done {
				stopReason = chunk.StopReason
			}
		}

		fullText := textBuf.String()
		// Build raw content from accumulated text
		if fullText != "" {
			rawContent = append([]ai.ContentPart{{Text: fullText}}, rawContent...)
		}

		// Append assistant response to history
		msgs = append(msgs, ai.Message{
			Role:    "assistant",
			Content: rawContent,
		})

		// If no tool calls, we're done
		if stopReason != "tool_use" || len(toolCalls) == 0 {
			result.Text = fullText
			slog.Info("stream agent run completed",
				"turns", result.Turns,
				"tools_used", result.ToolsUsed,
				"duration_ms", time.Since(start).Milliseconds(),
			)
			return result, nil
		}

		// Execute tool calls
		var toolResults []ai.ContentPart
		for _, tc := range toolCalls {
			result.ToolsUsed = append(result.ToolsUsed, tc.Name)

			slog.Info("stream agent executing tool",
				"tool", tc.Name,
				"turn", turn,
			)

			toolOutput, execErr := r.tools.Execute(scopedCtx, tc.Name, tc.Input)
			if execErr != nil {
				toolResults = append(toolResults, ai.ContentPart{
					ToolResult: &ai.ToolResultBlock{
						ToolUseID: tc.ID,
						Content:   safeErrorJSON(execErr),
						IsError:   true,
					},
				})
				select {
				case out <- ai.StreamChunk{ToolResult: &ai.ToolResultBlock{
					ToolUseID: tc.ID,
					Content:   safeErrorJSON(execErr),
					IsError:   true,
				}}:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				continue
			}

			toolResults = append(toolResults, ai.ContentPart{
				ToolResult: &ai.ToolResultBlock{
					ToolUseID: tc.ID,
					Content:   toolOutput,
					IsError:   false,
				},
			})
			select {
			case out <- ai.StreamChunk{ToolResult: &ai.ToolResultBlock{
				ToolUseID: tc.ID,
				Content:   toolOutput,
				IsError:   false,
			}}:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		msgs = append(msgs, ai.Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	slog.Warn("stream agent hit max turns limit", "max_turns", r.maxTurns)
	result.Text = "I've been working on this but need to wrap up. Here's what I've found so far: " + result.Text
	return result, nil
}
