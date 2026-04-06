package tools

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
)

// =============================================================================
// Registry Tests
// =============================================================================

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	defs := r.Definitions()
	if len(defs) != 0 {
		t.Errorf("new registry should have 0 definitions, got %d", len(defs))
	}
}

func TestRegistry_Register_And_Get(t *testing.T) {
	r := NewRegistry()
	tool := Tool{
		Definition: ai.ToolDefinition{
			Name:        "test_tool",
			Description: "A test tool",
			InputSchema: json.RawMessage(`{"type":"object"}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			return `{"ok":true}`, nil
		},
	}
	r.Register(tool)

	got, ok := r.Get("test_tool")
	if !ok {
		t.Fatal("Get returned false for registered tool")
	}
	if got.Definition.Name != "test_tool" {
		t.Errorf("got name %q, want %q", got.Definition.Name, "test_tool")
	}
	if got.Definition.Description != "A test tool" {
		t.Errorf("got description %q, want %q", got.Definition.Description, "A test tool")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	_, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get returned true for nonexistent tool")
	}
}

func TestRegistry_Register_Duplicate_Panics(t *testing.T) {
	r := NewRegistry()
	tool := Tool{
		Definition: ai.ToolDefinition{
			Name: "dup_tool",
		},
		Handler: func(_ context.Context, _ json.RawMessage) (string, error) {
			return "", nil
		},
	}
	r.Register(tool)

	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatal("expected panic on duplicate registration")
		}
		msg, ok := rec.(string)
		if !ok {
			t.Fatalf("panic value is not a string: %T", rec)
		}
		if !strings.Contains(msg, "duplicate tool registration") {
			t.Errorf("panic message %q does not contain expected text", msg)
		}
	}()
	r.Register(tool)
}

func TestRegistry_Definitions(t *testing.T) {
	r := NewRegistry()
	names := []string{"tool_a", "tool_b", "tool_c"}
	for _, name := range names {
		r.Register(Tool{
			Definition: ai.ToolDefinition{Name: name, Description: "desc_" + name},
			Handler:    func(_ context.Context, _ json.RawMessage) (string, error) { return "", nil },
		})
	}

	defs := r.Definitions()
	if len(defs) != 3 {
		t.Fatalf("expected 3 definitions, got %d", len(defs))
	}

	found := make(map[string]bool)
	for _, d := range defs {
		found[d.Name] = true
	}
	for _, name := range names {
		if !found[name] {
			t.Errorf("definition for %q not found in Definitions()", name)
		}
	}
}

func TestRegistry_Execute_Success(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{
		Definition: ai.ToolDefinition{Name: "echo"},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			return string(input), nil
		},
	})

	result, err := r.Execute(context.Background(), "echo", json.RawMessage(`{"msg":"hello"}`))
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result != `{"msg":"hello"}` {
		t.Errorf("got %q, want %q", result, `{"msg":"hello"}`)
	}
}

func TestRegistry_Execute_UnknownTool(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute(context.Background(), "unknown", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), "unknown tool") {
		t.Errorf("error %q does not contain 'unknown tool'", err.Error())
	}
}

func TestRegistry_ConcurrentReads(t *testing.T) {
	r := NewRegistry()
	for i := 0; i < 10; i++ {
		name := "tool_" + strings.Repeat("x", i+1)
		r.Register(Tool{
			Definition: ai.ToolDefinition{Name: name},
			Handler:    func(_ context.Context, _ json.RawMessage) (string, error) { return "", nil },
		})
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Definitions()
			_, _ = r.Get("tool_x")
		}()
	}
	wg.Wait()
}

// =============================================================================
// Scope Context Tests
// =============================================================================

func TestWithScope_And_GetScope(t *testing.T) {
	projectID := uuid.New()
	orgID := uuid.New()
	userID := uuid.New()

	ctx := WithScope(context.Background(), Scope{
		ProjectID: projectID,
		OrgID:     orgID,
		UserID:    userID,
	})

	scope, ok := GetScope(ctx)
	if !ok {
		t.Fatal("GetScope returned false")
	}
	if scope.ProjectID != projectID {
		t.Errorf("ProjectID = %s, want %s", scope.ProjectID, projectID)
	}
	if scope.OrgID != orgID {
		t.Errorf("OrgID = %s, want %s", scope.OrgID, orgID)
	}
	if scope.UserID != userID {
		t.Errorf("UserID = %s, want %s", scope.UserID, userID)
	}
}

func TestGetScope_MissingContext(t *testing.T) {
	_, ok := GetScope(context.Background())
	if ok {
		t.Error("expected GetScope to return false for context without scope")
	}
}

func TestMustGetScope_Panics_Without_Scope(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from MustGetScope without scope")
		}
	}()
	MustGetScope(context.Background())
}

func TestMustGetScope_Returns_Scope(t *testing.T) {
	projectID := uuid.New()
	ctx := WithScope(context.Background(), Scope{ProjectID: projectID})
	scope := MustGetScope(ctx)
	if scope.ProjectID != projectID {
		t.Errorf("ProjectID = %s, want %s", scope.ProjectID, projectID)
	}
}

// =============================================================================
// ActionRunnerAdapter Tests
// =============================================================================

func TestActionRunnerAdapter_ExecuteAction_Success(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{
		Definition: ai.ToolDefinition{Name: "test_action"},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			scope := MustGetScope(ctx)
			return `{"project":"` + scope.ProjectID.String() + `"}`, nil
		},
	})

	adapter := NewActionRunnerAdapter(r)
	projectID := uuid.New()
	orgID := uuid.New()
	userID := uuid.New()

	result, err := adapter.ExecuteAction(
		context.Background(),
		"test_action",
		json.RawMessage(`{}`),
		projectID, orgID, userID,
	)
	if err != nil {
		t.Fatalf("ExecuteAction returned error: %v", err)
	}
	if !strings.Contains(result, projectID.String()) {
		t.Errorf("result %q does not contain project ID %s", result, projectID)
	}
}

func TestActionRunnerAdapter_ExecuteAction_UnknownAction(t *testing.T) {
	r := NewRegistry()
	adapter := NewActionRunnerAdapter(r)

	_, err := adapter.ExecuteAction(
		context.Background(),
		"nonexistent_action",
		json.RawMessage(`{}`),
		uuid.New(), uuid.New(), uuid.New(),
	)
	if err == nil {
		t.Fatal("expected error for unknown action")
	}
	if !strings.Contains(err.Error(), "execute approved action") {
		t.Errorf("error %q does not reference approved action", err.Error())
	}
}

// =============================================================================
// Tool Definition Validation Tests
// =============================================================================

func TestToolDefinitions_HaveValidSchemas(t *testing.T) {
	r := NewRegistry()

	// Register all stub tools (no DB dependencies)
	RegisterBudgetTools(r)
	RegisterProjectTools(r)
	RegisterScheduleTools(r)
	RegisterMarketTools(r)
	RegisterCommunicationTools(r)

	defs := r.Definitions()
	if len(defs) == 0 {
		t.Fatal("no tool definitions registered")
	}

	for _, d := range defs {
		t.Run(d.Name, func(t *testing.T) {
			if d.Name == "" {
				t.Error("tool has empty name")
			}
			if d.Description == "" {
				t.Errorf("tool %q has empty description", d.Name)
			}
			if len(d.InputSchema) == 0 {
				t.Errorf("tool %q has empty input schema", d.Name)
			}
			// Verify the schema is valid JSON
			var schema map[string]interface{}
			if err := json.Unmarshal(d.InputSchema, &schema); err != nil {
				t.Errorf("tool %q has invalid JSON schema: %v", d.Name, err)
			}
			// All schemas should be objects
			if schemaType, ok := schema["type"]; ok {
				if schemaType != "object" {
					t.Errorf("tool %q schema type is %q, want 'object'", d.Name, schemaType)
				}
			}
		})
	}
}

func TestStubToolNames_AreUnique(t *testing.T) {
	r := NewRegistry()
	RegisterBudgetTools(r)
	RegisterProjectTools(r)
	RegisterScheduleTools(r)
	RegisterMarketTools(r)
	RegisterCommunicationTools(r)

	defs := r.Definitions()
	seen := make(map[string]bool)
	for _, d := range defs {
		if seen[d.Name] {
			t.Errorf("duplicate tool name: %q", d.Name)
		}
		seen[d.Name] = true
	}
}

// =============================================================================
// Budget Tools Tests (stub implementations)
// =============================================================================

func TestBudgetTools_GetBudgetSummary_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterBudgetTools(r)

	projectID := uuid.New()
	ctx := WithScope(context.Background(), Scope{
		ProjectID: projectID,
		OrgID:     uuid.New(),
		UserID:    uuid.New(),
	})

	result, err := r.Execute(ctx, "get_budget_summary", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, projectID.String()) {
		t.Errorf("result does not contain project ID")
	}
	if !strings.Contains(result, `"currency_code":"USD"`) {
		t.Errorf("result does not contain USD currency code")
	}
}

func TestBudgetTools_EstimateCostImpact_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterBudgetTools(r)

	input := json.RawMessage(`{"description":"add 100sqft","square_footage_delta":100}`)
	result, err := r.Execute(context.Background(), "estimate_cost_impact", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	// 100 sqft * 15000 cents/sqft = 1500000 cents ($15,000)
	if cents, ok := resp["estimated_cost_cents"].(float64); ok {
		if int64(cents) != 1500000 {
			t.Errorf("estimated_cost_cents = %v, want 1500000", cents)
		}
	} else {
		t.Error("estimated_cost_cents not found or wrong type")
	}
}

func TestBudgetTools_EstimateCostImpact_InvalidInput(t *testing.T) {
	r := NewRegistry()
	RegisterBudgetTools(r)

	_, err := r.Execute(context.Background(), "estimate_cost_impact", json.RawMessage(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid input")
	}
}

// =============================================================================
// Project Tools Tests (stub implementations)
// =============================================================================

func TestProjectTools_GetProjectDetails_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterProjectTools(r)

	projectID := uuid.New()
	orgID := uuid.New()
	ctx := WithScope(context.Background(), Scope{
		ProjectID: projectID,
		OrgID:     orgID,
	})

	result, err := r.Execute(ctx, "get_project_details", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, projectID.String()) {
		t.Error("result does not contain project ID")
	}
	if !strings.Contains(result, orgID.String()) {
		t.Error("result does not contain org ID")
	}
}

func TestProjectTools_ListTasks_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterProjectTools(r)

	ctx := WithScope(context.Background(), Scope{
		ProjectID: uuid.New(),
	})

	result, err := r.Execute(ctx, "list_tasks", json.RawMessage(`{"status_filter":"Completed"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Completed") {
		t.Error("result does not contain status filter")
	}
}

func TestProjectTools_ListProcurementItems_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterProjectTools(r)

	projectID := uuid.New()
	ctx := WithScope(context.Background(), Scope{ProjectID: projectID})

	result, err := r.Execute(ctx, "list_procurement_items", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, projectID.String()) {
		t.Error("result does not contain project ID")
	}
}

func TestProjectTools_GetWeatherForecast_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterProjectTools(r)

	input := json.RawMessage(`{"latitude":30.2672,"longitude":-97.7431}`)
	result, err := r.Execute(context.Background(), "get_weather_forecast", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "30.2672") {
		t.Error("result does not contain latitude")
	}
}

func TestProjectTools_GetWeatherForecast_InvalidInput(t *testing.T) {
	r := NewRegistry()
	RegisterProjectTools(r)

	_, err := r.Execute(context.Background(), "get_weather_forecast", json.RawMessage(`{bad}`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// =============================================================================
// Schedule Tools Tests (stub implementations)
// =============================================================================

func TestScheduleTools_RecalculateSchedule_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterScheduleTools(r)

	projectID := uuid.New()
	ctx := WithScope(context.Background(), Scope{ProjectID: projectID})

	result, err := r.Execute(ctx, "recalculate_schedule", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, projectID.String()) {
		t.Error("result does not contain project ID")
	}
	if !strings.Contains(result, "true") {
		t.Error("result does not indicate success")
	}
}

func TestScheduleTools_DelayTask_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterScheduleTools(r)

	taskID := uuid.New()
	ctx := WithScope(context.Background(), Scope{ProjectID: uuid.New()})

	input := json.RawMessage(`{"task_id":"` + taskID.String() + `","delay_days":3,"reason":"rain"}`)
	result, err := r.Execute(ctx, "delay_task", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, taskID.String()) {
		t.Error("result does not contain task ID")
	}
	if !strings.Contains(result, "rain") {
		t.Error("result does not contain reason")
	}
}

func TestScheduleTools_DelayTask_InvalidTaskID(t *testing.T) {
	r := NewRegistry()
	RegisterScheduleTools(r)

	ctx := WithScope(context.Background(), Scope{ProjectID: uuid.New()})
	input := json.RawMessage(`{"task_id":"not-a-uuid","delay_days":1,"reason":"test"}`)
	_, err := r.Execute(ctx, "delay_task", input)
	if err == nil {
		t.Fatal("expected error for invalid task_id UUID")
	}
}

func TestScheduleTools_GetAgentFocusTasks_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterScheduleTools(r)

	ctx := WithScope(context.Background(), Scope{ProjectID: uuid.New()})
	result, err := r.Execute(ctx, "get_agent_focus_tasks", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "focus_tasks") {
		t.Error("result does not contain focus_tasks")
	}
}

// =============================================================================
// Market Tools Tests
// =============================================================================

func TestMarketTools_GetMarketConditions(t *testing.T) {
	r := NewRegistry()
	RegisterMarketTools(r)

	tests := []struct {
		name     string
		input    string
		wantErr  bool
		contains string
	}{
		{
			name:     "winter month",
			input:    `{"region":"TX-Austin","start_date":"2025-01-15"}`,
			contains: "January",
		},
		{
			name:     "summer month",
			input:    `{"start_date":"2025-07-01"}`,
			contains: "July",
		},
		{
			name:    "invalid date",
			input:   `{"start_date":"not-a-date"}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			input:   `{bad}`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := r.Execute(context.Background(), "get_market_conditions", json.RawMessage(tc.input))
			if tc.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.contains != "" && !strings.Contains(result, tc.contains) {
				t.Errorf("result does not contain %q", tc.contains)
			}
		})
	}
}

func TestMonthlySeasonalFactor(t *testing.T) {
	tests := []struct {
		month    int
		expected float64
	}{
		{1, 0.95}, {2, 0.95}, {3, 1.00}, {4, 1.05},
		{5, 1.10}, {6, 1.15}, {7, 1.15}, {8, 1.10},
		{9, 1.05}, {10, 1.00}, {11, 0.95}, {12, 0.95},
	}

	for _, tc := range tests {
		got := monthlySeasonalFactor(tc.month)
		if got != tc.expected {
			t.Errorf("monthlySeasonalFactor(%d) = %v, want %v", tc.month, got, tc.expected)
		}
	}

	// Out of range returns 1.0
	if got := monthlySeasonalFactor(0); got != 1.0 {
		t.Errorf("monthlySeasonalFactor(0) = %v, want 1.0", got)
	}
	if got := monthlySeasonalFactor(13); got != 1.0 {
		t.Errorf("monthlySeasonalFactor(13) = %v, want 1.0", got)
	}
}

func TestLaborAvailability(t *testing.T) {
	tests := []struct {
		month    int
		expected string
	}{
		{1, "available"}, {2, "available"}, {3, "moderate"}, {4, "moderate"},
		{5, "tight"}, {6, "tight"}, {7, "tight"}, {8, "tight"},
		{9, "moderate"}, {10, "moderate"}, {11, "available"}, {12, "available"},
	}

	for _, tc := range tests {
		got := laborAvailability(tc.month)
		if got != tc.expected {
			t.Errorf("laborAvailability(%d) = %q, want %q", tc.month, got, tc.expected)
		}
	}
}

// =============================================================================
// Communication Tools Tests (stub implementations)
// =============================================================================

func TestCommunicationTools_SendSMS_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterCommunicationTools(r) // nil notifSvc = stub mode

	input := json.RawMessage(`{"contact_id":"` + uuid.New().String() + `","message":"Hello, World!"}`)
	result, err := r.Execute(context.Background(), "send_sms", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "queued") {
		t.Error("result does not indicate queued status")
	}
	if !strings.Contains(result, "stub") {
		t.Error("result does not indicate stub mode")
	}
}

func TestCommunicationTools_SendEmail_Stub(t *testing.T) {
	r := NewRegistry()
	RegisterCommunicationTools(r)

	input := json.RawMessage(`{"to":"test@example.com","subject":"Test","body":"Hello"}`)
	result, err := r.Execute(context.Background(), "send_email", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "queued") {
		t.Error("result does not indicate queued status")
	}
}

func TestCommunicationTools_DraftMessage(t *testing.T) {
	r := NewRegistry()
	RegisterCommunicationTools(r)

	input := json.RawMessage(`{"channel":"email","to_name":"Bob","to_address":"bob@test.com","body":"Draft body","context":"Testing"}`)
	result, err := r.Execute(context.Background(), "draft_message", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if resp["draft"] != true {
		t.Error("expected draft=true")
	}
	if resp["channel"] != "email" {
		t.Errorf("channel = %v, want 'email'", resp["channel"])
	}
	if resp["to_name"] != "Bob" {
		t.Errorf("to_name = %v, want 'Bob'", resp["to_name"])
	}
}

func TestCommunicationTools_InvalidInput(t *testing.T) {
	r := NewRegistry()
	RegisterCommunicationTools(r)

	badInput := json.RawMessage(`{not valid}`)
	_, err := r.Execute(context.Background(), "send_sms", badInput)
	if err == nil {
		t.Error("expected error for invalid JSON input to send_sms")
	}

	_, err = r.Execute(context.Background(), "send_email", badInput)
	if err == nil {
		t.Error("expected error for invalid JSON input to send_email")
	}

	_, err = r.Execute(context.Background(), "draft_message", badInput)
	if err == nil {
		t.Error("expected error for invalid JSON input to draft_message")
	}
}

// =============================================================================
// FormatCents Tests
// =============================================================================

func TestFormatCents(t *testing.T) {
	tests := []struct {
		cents    int64
		expected string
	}{
		{0, "0.00"},
		{1, "0.01"},
		{99, "0.99"},
		{100, "1.00"},
		{123456, "1,234.56"},
		{1500000, "15,000.00"},
		{100000000, "1,000,000.00"},
		{-123456, "-1,234.56"},
		{-100, "-1.00"},
		{50, "0.50"},
	}

	for _, tc := range tests {
		got := formatCents(tc.cents)
		if got != tc.expected {
			t.Errorf("formatCents(%d) = %q, want %q", tc.cents, got, tc.expected)
		}
	}
}

// =============================================================================
// Scope Missing from Context -- Handler Panic Tests
// =============================================================================

func TestToolHandler_Panics_WithoutScope(t *testing.T) {
	r := NewRegistry()
	RegisterBudgetTools(r)

	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("expected panic when calling get_budget_summary without scope")
		}
	}()

	// Call handler without scope in context -- should panic via MustGetScope
	_, _ = r.Execute(context.Background(), "get_budget_summary", nil)
}

// =============================================================================
// Feed Tools Tests (with mock FeedCardWriter)
// =============================================================================

// testFeedWriter implements FeedCardWriter for testing.
type testFeedWriter struct {
	mu       sync.Mutex
	cards    []*models.FeedCard
	returnID uuid.UUID
	err      error
}

func (w *testFeedWriter) CreateCard(_ context.Context, card *models.FeedCard) (uuid.UUID, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.cards = append(w.cards, card)
	if w.err != nil {
		return uuid.Nil, w.err
	}
	if w.returnID != uuid.Nil {
		return w.returnID, nil
	}
	return uuid.New(), nil
}

func TestFeedTools_Registration(t *testing.T) {
	writer := &testFeedWriter{}
	r := NewRegistry()
	RegisterFeedTools(r, writer)

	_, ok1 := r.Get("write_feed_card")
	_, ok2 := r.Get("create_approval_card")
	if !ok1 {
		t.Error("write_feed_card not registered")
	}
	if !ok2 {
		t.Error("create_approval_card not registered")
	}
}

func TestFeedTools_WriteFeedCard(t *testing.T) {
	writer := &testFeedWriter{}
	r := NewRegistry()
	RegisterFeedTools(r, writer)

	ctx := WithScope(context.Background(), Scope{
		ProjectID: uuid.New(),
		OrgID:     uuid.New(),
	})

	input := json.RawMessage(`{
		"card_type":"daily_briefing",
		"headline":"Good morning",
		"body":"Today's update",
		"priority":"normal",
		"horizon":"today"
	}`)

	result, err := r.Execute(ctx, "write_feed_card", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "success") {
		t.Errorf("result does not contain success: %s", result)
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.cards) != 1 {
		t.Fatalf("expected 1 card created, got %d", len(writer.cards))
	}
	card := writer.cards[0]
	if card.CardType != "daily_briefing" {
		t.Errorf("card type = %q, want 'daily_briefing'", card.CardType)
	}
	if card.Title != "Good morning" {
		t.Errorf("title = %q, want 'Good morning'", card.Title)
	}
	if card.Priority != "normal" {
		t.Errorf("priority = %q, want 'normal'", card.Priority)
	}
}

func TestFeedTools_CreateApprovalCard(t *testing.T) {
	writer := &testFeedWriter{}
	r := NewRegistry()
	RegisterFeedTools(r, writer)

	ctx := WithScope(context.Background(), Scope{
		ProjectID: uuid.New(),
		OrgID:     uuid.New(),
	})

	input := json.RawMessage(`{
		"headline":"Delay framing by 2 days",
		"body":"Rain forecast for next 48 hours",
		"consequence":"2-day critical path slip",
		"priority":"urgent",
		"action_type":"delay_task",
		"action_payload":{"task_id":"abc","delay_days":2},
		"expires_hours":12
	}`)

	result, err := r.Execute(ctx, "create_approval_card", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "approval_card_id") {
		t.Errorf("result does not contain approval_card_id: %s", result)
	}
	if !strings.Contains(result, "delay_task") {
		t.Errorf("result does not contain action_type: %s", result)
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(writer.cards))
	}
	card := writer.cards[0]
	if card.CardType != models.CardTypeAgentApproval {
		t.Errorf("card type = %q, want %q", card.CardType, models.CardTypeAgentApproval)
	}
	if card.Priority != "urgent" {
		t.Errorf("priority = %q, want 'urgent'", card.Priority)
	}
	if card.Consequence == nil || *card.Consequence != "2-day critical path slip" {
		t.Error("consequence not set correctly")
	}
	if card.ExpiresAt == nil {
		t.Error("expires_at should be set")
	}
	if len(card.EngineData) == 0 {
		t.Error("engine_data should contain action_type and action_payload")
	}
	if len(card.Actions) == 0 {
		t.Error("actions should contain Approve/Reject/Modify buttons")
	}
}

func TestFeedTools_CreateApprovalCard_DefaultExpiry(t *testing.T) {
	writer := &testFeedWriter{}
	r := NewRegistry()
	RegisterFeedTools(r, writer)

	ctx := WithScope(context.Background(), Scope{
		ProjectID: uuid.New(),
		OrgID:     uuid.New(),
	})

	// No expires_hours provided -- should default to 24
	input := json.RawMessage(`{
		"headline":"Test",
		"body":"Test body",
		"priority":"normal",
		"action_type":"test_action",
		"action_payload":{}
	}`)

	_, err := r.Execute(ctx, "create_approval_card", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(writer.cards))
	}
	if writer.cards[0].ExpiresAt == nil {
		t.Fatal("expires_at should be set even with default")
	}
}

// =============================================================================
// RegisterProjectToolsWithDB Tests
// =============================================================================

type mockProjectLister struct {
	data interface{}
	err  error
}

func (m *mockProjectLister) GetProjectSummary(_ context.Context, _, _ uuid.UUID) (interface{}, error) {
	return m.data, m.err
}

type mockProcurementLister struct {
	data interface{}
	err  error
}

func (m *mockProcurementLister) ListProcurementItems(_ context.Context, _ uuid.UUID) (interface{}, error) {
	return m.data, m.err
}

func TestRegisterProjectToolsWithDB_WithMocks(t *testing.T) {
	r := NewRegistry()

	projLister := &mockProjectLister{
		data: map[string]string{"name": "Test Project"},
	}
	procLister := &mockProcurementLister{
		data: []string{"item1", "item2"},
	}

	RegisterProjectToolsWithDB(r, projLister, procLister)

	_, ok1 := r.Get("get_project_details")
	_, ok2 := r.Get("list_procurement_items")
	if !ok1 {
		t.Error("get_project_details not registered")
	}
	if !ok2 {
		t.Error("list_procurement_items not registered")
	}

	ctx := WithScope(context.Background(), Scope{
		ProjectID: uuid.New(),
		OrgID:     uuid.New(),
	})
	result, err := r.Execute(ctx, "get_project_details", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "Test Project") {
		t.Errorf("result does not contain expected data: %s", result)
	}

	result, err = r.Execute(ctx, "list_procurement_items", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "item1") {
		t.Errorf("result does not contain expected data: %s", result)
	}
}

func TestRegisterProjectToolsWithDB_NilListers(t *testing.T) {
	r := NewRegistry()
	RegisterProjectToolsWithDB(r, nil, nil)

	_, ok1 := r.Get("get_project_details")
	_, ok2 := r.Get("list_procurement_items")
	if ok1 {
		t.Error("get_project_details should not be registered with nil lister")
	}
	if ok2 {
		t.Error("list_procurement_items should not be registered with nil lister")
	}
}

// =============================================================================
// ScheduleRecalcAdapter Tests
// =============================================================================

func TestNewScheduleRecalcAdapter(t *testing.T) {
	engine := &ScheduleEngine{}
	adapter := NewScheduleRecalcAdapter(engine)
	if adapter == nil {
		t.Fatal("NewScheduleRecalcAdapter returned nil")
	}
	if adapter.engine != engine {
		t.Error("adapter engine reference mismatch")
	}
}

// =============================================================================
// CPMRecalcResult Tests
// =============================================================================

func TestCPMRecalcResult_JSONSerialization(t *testing.T) {
	result := CPMRecalcResult{
		TaskCount:         10,
		CriticalPathCount: 3,
		CriticalPath:      []string{"A", "B", "C"},
		Message:           "success",
	}

	b, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded CPMRecalcResult
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.TaskCount != 10 {
		t.Errorf("TaskCount = %d, want 10", decoded.TaskCount)
	}
	if decoded.CriticalPathCount != 3 {
		t.Errorf("CriticalPathCount = %d, want 3", decoded.CriticalPathCount)
	}
	if len(decoded.CriticalPath) != 3 {
		t.Errorf("CriticalPath length = %d, want 3", len(decoded.CriticalPath))
	}
}

// =============================================================================
// Communication Tools with Mock NotificationService
// =============================================================================

type mockNotificationService struct {
	smsErr   error
	emailErr error
	smsCalls int
	emailCalls int
}

func (m *mockNotificationService) QueueSMS(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _, _ string) (uuid.UUID, error) {
	m.smsCalls++
	if m.smsErr != nil {
		return uuid.Nil, m.smsErr
	}
	return uuid.New(), nil
}

func (m *mockNotificationService) QueueEmail(_ context.Context, _ uuid.UUID, _ *uuid.UUID, _, _, _ string) (uuid.UUID, error) {
	m.emailCalls++
	if m.emailErr != nil {
		return uuid.Nil, m.emailErr
	}
	return uuid.New(), nil
}

func TestCommunicationTools_SendSMS_WithService(t *testing.T) {
	mock := &mockNotificationService{}
	r := NewRegistry()
	RegisterCommunicationToolsWithService(r, mock)

	ctx := WithScope(context.Background(), Scope{
		ProjectID: uuid.New(),
		OrgID:     uuid.New(),
		UserID:    uuid.New(),
	})

	input := json.RawMessage(`{"contact_id":"` + uuid.New().String() + `","message":"Hello"}`)
	result, err := r.Execute(ctx, "send_sms", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "queued") {
		t.Error("result does not contain queued status")
	}
	if !strings.Contains(result, "log_id") {
		t.Error("result does not contain log_id")
	}
	if mock.smsCalls != 1 {
		t.Errorf("expected 1 SMS call, got %d", mock.smsCalls)
	}
}

func TestCommunicationTools_SendEmail_WithService(t *testing.T) {
	mock := &mockNotificationService{}
	r := NewRegistry()
	RegisterCommunicationToolsWithService(r, mock)

	ctx := WithScope(context.Background(), Scope{
		ProjectID: uuid.New(),
		OrgID:     uuid.New(),
		UserID:    uuid.New(),
	})

	input := json.RawMessage(`{"to":"test@example.com","subject":"Test","body":"Hello"}`)
	result, err := r.Execute(ctx, "send_email", input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "queued") {
		t.Error("result does not contain queued status")
	}
	if mock.emailCalls != 1 {
		t.Errorf("expected 1 email call, got %d", mock.emailCalls)
	}
}
