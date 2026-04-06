package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/agents/tools"
	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

// MaxAgentTurns is the safety limit for tool-use iterations.
// Prevents infinite loops if the model keeps requesting tools.
const MaxAgentTurns = 15

// safeErrorJSON returns a JSON-safe error string to prevent injection via unescaped error messages.
func safeErrorJSON(err error) string {
	escaped, _ := json.Marshal(err.Error())
	return fmt.Sprintf(`{"error":%s}`, escaped)
}

// AgentRunner executes Claude-powered agent conversations with tool use.
// It manages the tool-use loop: send message -> receive tool_use -> execute -> send results -> repeat.
type AgentRunner struct {
	aiClient ai.Client
	tools    *tools.Registry
	maxTurns int
}

// NewAgentRunner creates a runner with the given AI client and tool registry.
func NewAgentRunner(aiClient ai.Client, toolRegistry *tools.Registry) *AgentRunner {
	return &AgentRunner{
		aiClient: aiClient,
		tools:    toolRegistry,
		maxTurns: MaxAgentTurns,
	}
}

// WithMaxTurns overrides the default max turns safety limit.
func (r *AgentRunner) WithMaxTurns(n int) *AgentRunner {
	if n > 0 {
		r.maxTurns = n
	}
	return r
}

// AgentResult contains the final output of an agent run.
type AgentResult struct {
	// Text is the final text response to show the user.
	Text string

	// FeedCards collects any feed cards generated during tool execution.
	FeedCards []*models.FeedCard

	// ToolsUsed tracks which tools were called for observability.
	ToolsUsed []string

	// TotalTokens is the cumulative token usage across all turns.
	TotalTokens int

	// Turns is the number of request/response cycles.
	Turns int
}

// ProjectContext provides scoping for tool execution.
type ProjectContext struct {
	ProjectID uuid.UUID
	OrgID     uuid.UUID
	UserID    uuid.UUID
}

// Run executes a Claude agent conversation.
//
// Flow:
//  1. Send system prompt + initial user message + tool definitions to Claude
//  2. If Claude responds with tool_use blocks -> execute each tool -> append results -> loop
//  3. If Claude responds with end_turn -> return final text
//  4. Safety: break after maxTurns iterations
func (r *AgentRunner) Run(ctx context.Context, systemPrompt string, userMessage string, projectCtx ProjectContext) (*AgentResult, error) {
	start := time.Now()

	// Inject project scope into context for tool handlers
	scopedCtx := tools.WithScope(ctx, tools.Scope{
		ProjectID: projectCtx.ProjectID,
		OrgID:     projectCtx.OrgID,
		UserID:    projectCtx.UserID,
	})

	// Build initial message history
	messages := []ai.Message{
		{
			Role:    "user",
			Content: []ai.ContentPart{{Text: userMessage}},
		},
	}

	toolDefs := r.tools.Definitions()

	result := &AgentResult{}

	for turn := 0; turn < r.maxTurns; turn++ {
		result.Turns = turn + 1

		// Send request to Claude
		req := ai.NewAgentRequest(ai.ModelTypeOpus, systemPrompt, messages, toolDefs)
		resp, err := r.aiClient.GenerateContent(scopedCtx, req)
		if err != nil {
			return nil, fmt.Errorf("agent turn %d: %w", turn, err)
		}

		result.TotalTokens += resp.TokensUsed

		// Append the assistant's response to conversation history
		messages = append(messages, ai.Message{
			Role:    "assistant",
			Content: resp.RawContent,
		})

		// If no tool calls, we're done — Claude has a final answer
		if resp.StopReason != "tool_use" || len(resp.ToolCalls) == 0 {
			result.Text = resp.Text
			slog.Info("agent run completed",
				"turns", result.Turns,
				"total_tokens", result.TotalTokens,
				"tools_used", result.ToolsUsed,
				"duration_ms", time.Since(start).Milliseconds(),
			)
			return result, nil
		}

		// Execute each tool call and build tool_result messages
		var toolResults []ai.ContentPart
		for _, tc := range resp.ToolCalls {
			result.ToolsUsed = append(result.ToolsUsed, tc.Name)

			slog.Info("agent executing tool",
				"tool", tc.Name,
				"turn", turn,
				"tool_use_id", tc.ID,
			)

			toolOutput, err := r.tools.Execute(scopedCtx, tc.Name, tc.Input)
			if err != nil {
				slog.Error("agent tool execution failed",
					"tool", tc.Name,
					"error", err,
				)
				// Return error as tool result so Claude can reason about it
				toolResults = append(toolResults, ai.ContentPart{
					ToolResult: &ai.ToolResultBlock{
						ToolUseID: tc.ID,
						Content:   safeErrorJSON(err),
						IsError:   true,
					},
				})
				continue
			}

			toolResults = append(toolResults, ai.ContentPart{
				ToolResult: &ai.ToolResultBlock{
					ToolUseID: tc.ID,
					Content:   toolOutput,
					IsError:   false,
				},
			})
		}

		// Append tool results as a user message (Anthropic convention)
		messages = append(messages, ai.Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	// Safety: hit max turns
	slog.Warn("agent hit max turns limit",
		"max_turns", r.maxTurns,
		"tools_used", result.ToolsUsed,
	)
	result.Text = "I've been working on this but need to wrap up. Here's what I've found so far: " + result.Text
	return result, nil
}

// RunWithHistory executes an agent conversation with existing message history.
// Used for multi-turn chat where the user has already exchanged messages.
func (r *AgentRunner) RunWithHistory(ctx context.Context, systemPrompt string, messages []ai.Message, projectCtx ProjectContext) (*AgentResult, error) {
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
		resp, err := r.aiClient.GenerateContent(scopedCtx, req)
		if err != nil {
			return nil, fmt.Errorf("agent turn %d: %w", turn, err)
		}

		result.TotalTokens += resp.TokensUsed

		msgs = append(msgs, ai.Message{
			Role:    "assistant",
			Content: resp.RawContent,
		})

		if resp.StopReason != "tool_use" || len(resp.ToolCalls) == 0 {
			result.Text = resp.Text
			slog.Info("agent run completed",
				"turns", result.Turns,
				"total_tokens", result.TotalTokens,
				"tools_used", result.ToolsUsed,
				"duration_ms", time.Since(start).Milliseconds(),
			)
			return result, nil
		}

		var toolResults []ai.ContentPart
		for _, tc := range resp.ToolCalls {
			result.ToolsUsed = append(result.ToolsUsed, tc.Name)

			toolOutput, err := r.tools.Execute(scopedCtx, tc.Name, tc.Input)
			if err != nil {
				toolResults = append(toolResults, ai.ContentPart{
					ToolResult: &ai.ToolResultBlock{
						ToolUseID: tc.ID,
						Content:   safeErrorJSON(err),
						IsError:   true,
					},
				})
				continue
			}
			toolResults = append(toolResults, ai.ContentPart{
				ToolResult: &ai.ToolResultBlock{
					ToolUseID: tc.ID,
					Content:   toolOutput,
					IsError:   false,
				},
			})
		}

		msgs = append(msgs, ai.Message{
			Role:    "user",
			Content: toolResults,
		})
	}

	slog.Warn("agent hit max turns limit", "max_turns", r.maxTurns)
	result.Text = "I've been working on this but need to wrap up. Here's what I've found so far: " + result.Text
	return result, nil
}

// BuildContextPrefix creates the project context section for system prompts.
// Includes project details, schedule summary, and active alerts as structured text.
func BuildContextPrefix(project interface{}, schedule interface{}, tasks interface{}) string {
	var prefix string

	if project != nil {
		if b, err := json.MarshalIndent(project, "", "  "); err == nil {
			prefix += "## Project Details\n```json\n" + string(b) + "\n```\n\n"
		}
	}
	if schedule != nil {
		if b, err := json.MarshalIndent(schedule, "", "  "); err == nil {
			prefix += "## Schedule Summary\n```json\n" + string(b) + "\n```\n\n"
		}
	}
	if tasks != nil {
		if b, err := json.MarshalIndent(tasks, "", "  "); err == nil {
			prefix += "## Today's Focus Tasks\n```json\n" + string(b) + "\n```\n\n"
		}
	}

	return prefix
}
