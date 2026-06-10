package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---- test stubs --------------------------------------------------------

// stubKeyResolver is a KeyResolver returning a fixed key (or error).
type stubKeyResolver struct {
	key string
	err error
}

func (s stubKeyResolver) AnthropicKey(_ context.Context, _ string) (string, error) {
	return s.key, s.err
}

func staticKey(k string) KeyResolver { return stubKeyResolver{key: k} }

// newTaskTestClient wires a Client to a httptest server with fast
// retries.
func newTaskTestClient(t *testing.T, handler http.HandlerFunc) (*Client, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	c, err := NewClient(Config{
		KeyResolver: staticKey("sk-ant-test"),
		BaseURL:     srv.URL,
		Model:       "claude-opus-4-6",
		FastModel:   "claude-sonnet-4-5",
		Retry:       RetryConfig{MaxAttempts: 3, BaseDelayMs: 1, Multiplier: 2.0},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv.Close
}

// assertCommonHeaders checks the required Anthropic headers.
func assertCommonHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/v1/messages" {
		t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
	}
	if got := r.Header.Get("x-api-key"); got != "sk-ant-test" {
		t.Errorf("x-api-key = %q, want sk-ant-test", got)
	}
	if got := r.Header.Get("anthropic-version"); got != anthropicVersion {
		t.Errorf("anthropic-version = %q, want %s", got, anthropicVersion)
	}
	if got := r.Header.Get("content-type"); got != "application/json" {
		t.Errorf("content-type = %q", got)
	}
}

// decodeMessagesReq decodes the request body the client produced.
func decodeMessagesReq(t *testing.T, r *http.Request) messagesRequest {
	t.Helper()
	var req messagesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	return req
}

// writeToolUse writes a /v1/messages success body with a single
// tool_use block carrying input.
func writeToolUse(t *testing.T, w http.ResponseWriter, toolName string, input any) {
	t.Helper()
	rawIn, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal tool input: %v", err)
	}
	resp := messagesResponse{
		ID: "msg_x", Type: "message", Role: "assistant", Model: "m",
		StopReason: "tool_use",
		Content: []contentBlock{
			{Type: "tool_use", ID: "toolu_1", Name: toolName, Input: rawIn},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// writeText writes a /v1/messages success body with text blocks.
func writeText(w http.ResponseWriter, parts ...string) {
	blocks := make([]contentBlock, 0, len(parts))
	for _, p := range parts {
		blocks = append(blocks, contentBlock{Type: "text", Text: p})
	}
	resp := messagesResponse{
		ID: "msg_t", Type: "message", Role: "assistant", Model: "m",
		StopReason: "end_turn", Content: blocks,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// ---- NewClient / config ------------------------------------------------

func TestNewClient_RequiresKeyResolver(t *testing.T) {
	if _, err := NewClient(Config{}); err == nil {
		t.Error("NewClient without KeyResolver should error")
	}
}

func TestNewClient_AppliesDefaults(t *testing.T) {
	c, err := NewClient(Config{KeyResolver: staticKey("k")})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if c.model != defaultModel || c.fastModel != defaultFastModel {
		t.Errorf("models = %q/%q, want %q/%q", c.model, c.fastModel, defaultModel, defaultFastModel)
	}
	if c.baseURL != defaultBaseURL {
		t.Errorf("baseURL = %q", c.baseURL)
	}
	if c.maxImageBytes != defaultMaxImageBytes {
		t.Errorf("maxImageBytes = %d", c.maxImageBytes)
	}
}

// ---- ErrUnconfigured ---------------------------------------------------

func TestUnconfigured_EmptyKey(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer srv.Close()

	c, err := NewClient(Config{KeyResolver: staticKey(""), BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.DailyBriefing(context.Background(), DailyBriefingRequest{Tasks: []string{"x"}})
	if !errors.Is(err, ErrUnconfigured) {
		t.Errorf("err = %v, want ErrUnconfigured", err)
	}
	if called {
		t.Error("server should not be called when key is empty")
	}
}

func TestUnconfigured_ResolverError(t *testing.T) {
	c, err := NewClient(Config{
		KeyResolver: stubKeyResolver{err: errors.New("vault down")},
		BaseURL:     "http://127.0.0.1:1",
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.IntentClassify(context.Background(), IntentClassifyRequest{Utterance: "x"})
	if !errors.Is(err, ErrUnconfigured) {
		t.Errorf("err = %v, want ErrUnconfigured", err)
	}
}

// ---- DailyBriefing -----------------------------------------------------

func TestDailyBriefing_RoundTrip(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertCommonHeaders(t, r)
		req := decodeMessagesReq(t, r)
		if req.Model != "claude-sonnet-4-5" {
			t.Errorf("model = %q, want fast model", req.Model)
		}
		if len(req.Messages) != 1 || req.Messages[0].Role != "user" {
			t.Errorf("messages = %+v", req.Messages)
		}
		writeText(w, "Lead with the framing alert; ", "then pour footings.")
	})
	defer cleanup()

	resp, err := c.DailyBriefing(context.Background(), DailyBriefingRequest{
		Tasks:    []string{"1.1 Pour footings", "1.2 Strip forms"},
		Alerts:   []string{"[critical] Framing inspection failed"},
		UserRole: "superintendent",
	})
	if err != nil {
		t.Fatalf("DailyBriefing: %v", err)
	}
	if resp.Reply != "Lead with the framing alert; then pour footings." {
		t.Errorf("reply = %q", resp.Reply)
	}
	if resp.SessionID == uuid.Nil {
		t.Error("session_id should be populated")
	}
}

func TestDailyBriefing_EchoesSessionID(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, "ok")
	})
	defer cleanup()

	sid := uuid.New()
	resp, err := c.DailyBriefing(context.Background(), DailyBriefingRequest{
		SessionID: &sid, Tasks: []string{"x"},
	})
	if err != nil {
		t.Fatalf("DailyBriefing: %v", err)
	}
	if resp.SessionID != sid {
		t.Errorf("session_id = %s, want echoed %s", resp.SessionID, sid)
	}
}

// ---- IntentClassify ----------------------------------------------------

func TestIntentClassify_ValidatesUtterance(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be called for empty utterance")
	})
	defer cleanup()
	if _, err := c.IntentClassify(context.Background(), IntentClassifyRequest{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestIntentClassify_RoundTrip(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertCommonHeaders(t, r)
		req := decodeMessagesReq(t, r)
		if req.Model != "claude-sonnet-4-5" {
			t.Errorf("model = %q, want fast model", req.Model)
		}
		if req.ToolChoice == nil || req.ToolChoice.Type != "tool" || req.ToolChoice.Name != "classify_intent" {
			t.Errorf("tool_choice = %+v", req.ToolChoice)
		}
		if len(req.Tools) != 1 || req.Tools[0].Name != "classify_intent" {
			t.Errorf("tools = %+v", req.Tools)
		}
		writeToolUse(t, w, "classify_intent", map[string]any{
			"intent":     "material_request",
			"confidence": 0.92,
			"entities":   map[string]string{"material": "OSB", "quantity": "200"},
		})
	})
	defer cleanup()

	resp, err := c.IntentClassify(context.Background(), IntentClassifyRequest{
		Utterance: "Need 200 sheets of 1/2 OSB", Channel: "sms",
	})
	if err != nil {
		t.Fatalf("IntentClassify: %v", err)
	}
	if resp.Intent != "material_request" || resp.Confidence < 0.9 {
		t.Errorf("resp = %+v", resp)
	}
	if resp.Entities["material"] != "OSB" {
		t.Errorf("entities = %+v", resp.Entities)
	}
}

// ---- InvoiceExtract ----------------------------------------------------

func TestInvoiceExtract_RequiresInput(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be called for empty payload")
	})
	defer cleanup()
	if _, err := c.InvoiceExtract(context.Background(), InvoiceExtractRequest{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestInvoiceExtract_TextRoundTrip(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertCommonHeaders(t, r)
		req := decodeMessagesReq(t, r)
		if req.Model != "claude-opus-4-6" {
			t.Errorf("model = %q, want opus", req.Model)
		}
		// text path: a single text block, no image block.
		if len(req.Messages[0].Content) != 1 || req.Messages[0].Content[0].Type != "text" {
			t.Errorf("content = %+v, want single text block", req.Messages[0].Content)
		}
		writeToolUse(t, w, "extract_invoice", map[string]any{
			"vendor_name":           "Acme Lumber",
			"invoice_no":            "INV-1042",
			"issued_date":           "2026-04-15",
			"total_cents":           int64(125000),
			"invoice_currency_code": "USD",
			"line_items": []map[string]any{
				{"description": "2x4 SPF studs", "quantity": 100, "unit_cents": int64(450), "amount_cents": int64(45000)},
				{"description": "OSB 1/2 4x8", "quantity": 200, "unit_cents": int64(400), "amount_cents": int64(80000)},
			},
		})
	})
	defer cleanup()

	resp, err := c.InvoiceExtract(context.Background(), InvoiceExtractRequest{Text: "Acme Lumber INV-1042 ..."})
	if err != nil {
		t.Fatalf("InvoiceExtract: %v", err)
	}
	if resp.VendorName != "Acme Lumber" || resp.InvoiceNo != "INV-1042" {
		t.Errorf("header mismatch: %+v", resp)
	}
	if resp.TotalCents != 125000 || resp.CurrencyCode != "USD" {
		t.Errorf("totals mismatch: total=%d ccy=%q", resp.TotalCents, resp.CurrencyCode)
	}
	if len(resp.LineItems) != 2 || resp.LineItems[0].UnitCents != 450 {
		t.Errorf("line items = %+v", resp.LineItems)
	}
}

func TestInvoiceExtract_DocumentImageRoundTrip(t *testing.T) {
	// Doc server serves a tiny PNG over TLS (the doc fetch is https-only); AI
	// server asserts an image block.
	docSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(tinyPNG())
	}))
	defer docSrv.Close()

	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeMessagesReq(t, r)
		content := req.Messages[0].Content
		var sawImage bool
		for _, blk := range content {
			if blk.Type == "image" {
				sawImage = true
				if blk.Source == nil || blk.Source.Type != "base64" || blk.Source.MediaType != "image/png" {
					t.Errorf("image source = %+v", blk.Source)
				}
				if blk.Source.Data == "" {
					t.Error("image data empty")
				}
			}
		}
		if !sawImage {
			t.Errorf("expected an image content block; content = %+v", content)
		}
		writeToolUse(t, w, "extract_invoice", map[string]any{
			"vendor_name":           "Img Vendor",
			"invoice_no":            "IMG-1",
			"total_cents":           int64(999),
			"invoice_currency_code": "CAD",
			"line_items":            []map[string]any{},
		})
	})
	defer cleanup()
	// Reach the loopback TLS doc server (the default doc client is the SSRF
	// guard, which refuses loopback).
	c.docHTTPClient = docSrv.Client()

	resp, err := c.InvoiceExtract(context.Background(), InvoiceExtractRequest{DocumentURL: docSrv.URL + "/doc.png"})
	if err != nil {
		t.Fatalf("InvoiceExtract: %v", err)
	}
	if resp.VendorName != "Img Vendor" || resp.CurrencyCode != "CAD" {
		t.Errorf("resp = %+v", resp)
	}
}

// ---- ProcurementRecommend ----------------------------------------------

func TestProcurementRecommend_RequiresMaterialRequestID(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be called for zero material_request_id")
	})
	defer cleanup()
	if _, err := c.ProcurementRecommend(context.Background(), ProcurementRecommendRequest{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestProcurementRecommend_RoundTrip(t *testing.T) {
	wantMRID := uuid.New()
	vendorID := uuid.New()
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeMessagesReq(t, r)
		if req.Model != "claude-opus-4-6" {
			t.Errorf("model = %q, want opus", req.Model)
		}
		writeToolUse(t, w, "recommend_vendors", map[string]any{
			"recommendations": []map[string]any{
				{
					"vendor_id":             vendorID,
					"vendor_name":           "Pacific Builders Supply",
					"predicted_spend_cents": int64(48000),
					"currency_code":         "USD",
					"confidence":            0.78,
					"reasoning":             "Best historical lead time",
				},
			},
		})
	})
	defer cleanup()

	resp, err := c.ProcurementRecommend(context.Background(), ProcurementRecommendRequest{
		MaterialRequestID: wantMRID, BudgetCents: 50000, CurrencyCode: "USD",
	})
	if err != nil {
		t.Fatalf("ProcurementRecommend: %v", err)
	}
	if len(resp.Recommendations) != 1 {
		t.Fatalf("recs len = %d", len(resp.Recommendations))
	}
	rec := resp.Recommendations[0]
	if rec.VendorID != vendorID || rec.PredictedSpendCents != 48000 || rec.CurrencyCode != "USD" {
		t.Errorf("rec mismatch: %+v", rec)
	}
}

// ---- TribunalReview ----------------------------------------------------

func TestTribunalReview_RequiresFacts(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be called for empty facts")
	})
	defer cleanup()
	if _, err := c.TribunalReview(context.Background(), TribunalReviewRequest{DisputeID: uuid.New()}); err == nil {
		t.Fatal("expected error")
	}
}

func TestTribunalReview_RoundTrip(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeMessagesReq(t, r)
		if req.Model != "claude-opus-4-6" {
			t.Errorf("model = %q, want opus", req.Model)
		}
		writeToolUse(t, w, "review_dispute", map[string]any{
			"recommendation": "approve",
			"confidence":     0.81,
			"rationale":      "Within scope tolerance",
		})
	})
	defer cleanup()

	resp, err := c.TribunalReview(context.Background(), TribunalReviewRequest{
		DisputeID: uuid.New(),
		Facts:     json.RawMessage(`{"change_order_id":"co-7","delta_cents":2400}`),
	})
	if err != nil {
		t.Fatalf("TribunalReview: %v", err)
	}
	if resp.Recommendation != "approve" || resp.Confidence < 0.8 {
		t.Errorf("resp = %+v", resp)
	}
}

// ---- UpdateSchedule ----------------------------------------------------

func TestUpdateSchedule_ValidatesProjectID(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be called for nil project_id")
	})
	defer cleanup()
	_, err := c.UpdateSchedule(context.Background(), UpdateScheduleRequest{
		Tasks: []ScheduleTaskSnapshot{{TaskID: uuid.New(), DurationDays: 5}},
	})
	if err == nil {
		t.Fatal("expected error for nil project_id")
	}
}

func TestUpdateSchedule_ValidatesTasksNonEmpty(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be called for empty tasks")
	})
	defer cleanup()
	if _, err := c.UpdateSchedule(context.Background(), UpdateScheduleRequest{ProjectID: uuid.New()}); err == nil {
		t.Fatal("expected error for empty tasks")
	}
}

func TestUpdateSchedule_RoundTrip(t *testing.T) {
	wantProject := uuid.New()
	wantTaskA := uuid.New()
	wantTaskB := uuid.New()

	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeMessagesReq(t, r)
		if req.Model != "claude-opus-4-6" {
			t.Errorf("model = %q, want opus", req.Model)
		}
		newDur := 9
		writeToolUse(t, w, "recommend_adjustments", map[string]any{
			"adjustments": []ScheduleAdjustment{
				{TaskID: wantTaskA, NewDurationDays: &newDur, Rationale: "weather drift"},
				{TaskID: wantTaskB, Rationale: "monitor only"},
			},
		})
	})
	defer cleanup()

	resp, err := c.UpdateSchedule(context.Background(), UpdateScheduleRequest{
		ProjectID:        wantProject,
		ProjectStartDate: "2026-05-06T00:00:00Z",
		Tasks: []ScheduleTaskSnapshot{
			{TaskID: wantTaskA, WBSCode: "1.1", Name: "Footings", DurationDays: 7, Status: "pending", IsCritical: true},
			{TaskID: wantTaskB, WBSCode: "1.2", Name: "Strip forms", DurationDays: 2, Status: "pending"},
		},
		Dependencies: []ScheduleDepSnapshot{
			{PredecessorID: wantTaskA, SuccessorID: wantTaskB, DependencyType: "FS", LagDays: 0},
		},
	})
	if err != nil {
		t.Fatalf("UpdateSchedule: %v", err)
	}
	if len(resp.Adjustments) != 2 {
		t.Fatalf("adjustments len = %d", len(resp.Adjustments))
	}
	if resp.Adjustments[0].NewDurationDays == nil || *resp.Adjustments[0].NewDurationDays != 9 {
		t.Errorf("adjustments[0].NewDurationDays = %v, want 9", resp.Adjustments[0].NewDurationDays)
	}
	if resp.Adjustments[1].NewDurationDays != nil {
		t.Errorf("adjustments[1].NewDurationDays = %v, want nil", resp.Adjustments[1].NewDurationDays)
	}
}

// ---- DelayCascadeReason ------------------------------------------------

func TestDelayCascadeReason_ValidatesSlippedTasks(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server should not be called for empty slipped tasks")
	})
	defer cleanup()
	if _, err := c.DelayCascadeReason(context.Background(), DelayCascadeReasonRequest{ProjectName: "Maple St"}); err == nil {
		t.Fatal("expected error for empty slipped tasks")
	}
}

func TestDelayCascadeReason_RoundTrip(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertCommonHeaders(t, r)
		req := decodeMessagesReq(t, r)
		if req.Model != "claude-opus-4-6" {
			t.Errorf("model = %q, want opus", req.Model)
		}
		if req.ToolChoice == nil || req.ToolChoice.Type != "tool" || req.ToolChoice.Name != "assess_delay_cascade" {
			t.Errorf("tool_choice = %+v", req.ToolChoice)
		}
		if len(req.Tools) != 1 || req.Tools[0].Name != "assess_delay_cascade" {
			t.Errorf("tools = %+v", req.Tools)
		}
		writeToolUse(t, w, "assess_delay_cascade", map[string]any{
			"impacts": []CascadeImpact{
				{
					Module:            "procurement",
					Severity:          "critical",
					Title:             "Window order at risk",
					Body:              "Framing slip pushes the must-order date past the lead-time window.",
					RecommendedAction: "Expedite the window PO today.",
				},
				{
					Module:            "crew",
					Severity:          "high",
					Title:             "Drywall crew idle risk",
					Body:              "Downstream tasks slip; the booked crew may arrive before work is ready.",
					RecommendedAction: "Reschedule the drywall crew by three days.",
				},
			},
		})
	})
	defer cleanup()

	resp, err := c.DelayCascadeReason(context.Background(), DelayCascadeReasonRequest{
		ProjectName: "Maple St Custom",
		SlippedTasks: []DelayCascadeSlippedTask{
			{WBS: "3.1", Name: "Framing", EarlyFinish: "2026-06-20", LateFinish: "2026-06-20", FloatDays: 0, IsCritical: true},
		},
		Procurement: []DelayCascadeProcurement{
			{Description: "Vinyl windows", Status: "pending", LeadTimeDays: 21, MustOrderBy: "2026-06-15"},
		},
		Budget: []DelayCascadeBudget{
			{WBS: "3.1", EstimatedCents: 1_200_000, CommittedCents: 800_000, ActualCents: 250_000, CurrencyCode: "USD"},
		},
	})
	if err != nil {
		t.Fatalf("DelayCascadeReason: %v", err)
	}
	if len(resp.Impacts) != 2 {
		t.Fatalf("impacts len = %d, want 2", len(resp.Impacts))
	}
	if resp.Impacts[0].Module != "procurement" || resp.Impacts[0].Severity != "critical" {
		t.Errorf("impacts[0] = %+v", resp.Impacts[0])
	}
	if resp.Impacts[1].Module != "crew" || resp.Impacts[1].RecommendedAction == "" {
		t.Errorf("impacts[1] = %+v", resp.Impacts[1])
	}
}

// ---- error propagation -------------------------------------------------

func TestTask_HTTPErrorPropagates(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"bad tool"}}`))
	})
	defer cleanup()

	_, err := c.IntentClassify(context.Background(), IntentClassifyRequest{Utterance: "x"})
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("err type = %T, want *HTTPError chain", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest || httpErr.Type != "invalid_request_error" {
		t.Errorf("HTTPError = %+v", httpErr)
	}
}

func TestCallTool_MissingToolUseErrors(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		// Respond with only a text block — no tool_use.
		writeText(w, "I cannot do that")
	})
	defer cleanup()

	_, err := c.IntentClassify(context.Background(), IntentClassifyRequest{Utterance: "x"})
	if err == nil {
		t.Fatal("expected error when model returns no tool_use")
	}
}

// TestContextWithOrgID confirms the org id threads through to the
// resolver.
func TestContextWithOrgID(t *testing.T) {
	var seen string
	resolver := orgCapturingResolver{seen: &seen, key: "k"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, "ok")
	}))
	defer srv.Close()

	c, err := NewClient(Config{KeyResolver: resolver, BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	ctx := ContextWithOrgID(context.Background(), "org-abc")
	if _, err := c.DailyBriefing(ctx, DailyBriefingRequest{Tasks: []string{"x"}}); err != nil {
		t.Fatalf("DailyBriefing: %v", err)
	}
	if seen != "org-abc" {
		t.Errorf("resolver saw org %q, want org-abc", seen)
	}
}

type orgCapturingResolver struct {
	seen *string
	key  string
}

func (r orgCapturingResolver) AnthropicKey(_ context.Context, orgID string) (string, error) {
	*r.seen = orgID
	return r.key, nil
}

// ---- transport-level edge legs ----------------------------------------

// recordingMetrics is a MetricsObserver that records the last observed
// call so a test can assert observe() forwarded to it.
type recordingMetrics struct {
	calls     int
	lastKind  string
	lastModel string
	lastErr   error
}

func (m *recordingMetrics) ObserveAICall(kind, model string, _ time.Duration, err error) {
	m.calls++
	m.lastKind = kind
	m.lastModel = model
	m.lastErr = err
}

// erroringBody is a response body whose Read always fails, used to drive
// the io.ReadAll failure legs in messages() and fetchDocumentImage().
type erroringBody struct{}

func (erroringBody) Read([]byte) (int, error) { return 0, errors.New("read boom") }
func (erroringBody) Close() error             { return nil }

// stubTransport is an http.RoundTripper backed by a function, so a test
// can synthesize a response (or transport error) without a live server.
type stubTransport struct {
	fn func(*http.Request) (*http.Response, error)
}

func (s stubTransport) RoundTrip(r *http.Request) (*http.Response, error) { return s.fn(r) }

// TestObserve_ForwardsToMetrics proves the optional MetricsObserver is
// invoked on a completed call (the observe() metrics!=nil branch).
func TestObserve_ForwardsToMetrics(t *testing.T) {
	rec := &recordingMetrics{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeText(w, "hello")
	}))
	defer srv.Close()

	c, err := NewClient(Config{
		KeyResolver: staticKey("k"),
		BaseURL:     srv.URL,
		Metrics:     rec,
		Retry:       RetryConfig{MaxAttempts: 2, BaseDelayMs: 1, Multiplier: 2.0},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.DailyBriefing(context.Background(), DailyBriefingRequest{Tasks: []string{"t"}}); err != nil {
		t.Fatalf("DailyBriefing: %v", err)
	}
	if rec.calls != 1 {
		t.Errorf("ObserveAICall calls=%d, want 1", rec.calls)
	}
	if rec.lastErr != nil {
		t.Errorf("observed err=%v, want nil on success", rec.lastErr)
	}
	if rec.lastKind != "daily_briefing" {
		t.Errorf("observed kind=%q, want daily_briefing", rec.lastKind)
	}
}

// TestMessages_InvalidJSONBodyMapsToDecodeError covers the 2xx decode leg:
// a 200 with a malformed body is the client's bug, not transport's, so it
// returns immediately (no retry) with a decode error.
func TestMessages_InvalidJSONBodyMapsToDecodeError(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not json`))
	})
	defer cleanup()

	_, err := c.DailyBriefing(context.Background(), DailyBriefingRequest{Tasks: []string{"t"}})
	if err == nil || !strings.Contains(err.Error(), "decode response") {
		t.Fatalf("err = %v, want a decode response error", err)
	}
}

// TestMessages_ContextCancelledReturnsCtxErr covers the per-attempt
// ctx.Err() guard at the top of the retry loop: an already-cancelled
// context returns before any HTTP attempt.
func TestMessages_ContextCancelledReturnsCtxErr(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("server must not be hit when ctx is already cancelled")
	})
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.DailyBriefing(ctx, DailyBriefingRequest{Tasks: []string{"t"}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestMessages_ResponseBodyReadErrorMapsToTransient covers the readErr
// leg: a 200 whose body fails mid-read counts as a transport failure,
// retries are exhausted, and the call maps to ErrTransient.
func TestMessages_ResponseBodyReadErrorMapsToTransient(t *testing.T) {
	rt := stubTransport{fn: func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       erroringBody{},
			Header:     make(http.Header),
		}, nil
	}}
	c, err := NewClient(Config{
		KeyResolver: staticKey("k"),
		BaseURL:     "http://anthropic.invalid",
		HTTPClient:  &http.Client{Transport: rt},
		Retry:       RetryConfig{MaxAttempts: 2, BaseDelayMs: 1, Multiplier: 2.0},
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if _, err := c.DailyBriefing(context.Background(), DailyBriefingRequest{Tasks: []string{"t"}}); !errors.Is(err, ErrTransient) {
		t.Fatalf("err = %v, want ErrTransient", err)
	}
}

// ---- task egress legs -------------------------------------------------

// TestTask_CallToolErrorPropagates covers the callTool-error leg in every
// tool-backed task method: a 4xx from /v1/messages surfaces as an
// *HTTPError through each method's error wrap.
func TestTask_CallToolErrorPropagates(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"nope"}}`))
	})
	defer cleanup()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"InvoiceExtract", func() error {
			_, err := c.InvoiceExtract(ctx, InvoiceExtractRequest{Text: "an invoice"})
			return err
		}},
		{"ProcurementRecommend", func() error {
			_, err := c.ProcurementRecommend(ctx, ProcurementRecommendRequest{MaterialRequestID: uuid.New()})
			return err
		}},
		{"TribunalReview", func() error {
			_, err := c.TribunalReview(ctx, TribunalReviewRequest{DisputeID: uuid.New(), Facts: json.RawMessage(`{"x":1}`)})
			return err
		}},
		{"UpdateSchedule", func() error {
			_, err := c.UpdateSchedule(ctx, UpdateScheduleRequest{
				ProjectID: uuid.New(),
				Tasks:     []ScheduleTaskSnapshot{{TaskID: uuid.New()}},
			})
			return err
		}},
		{"DelayCascadeReason", func() error {
			_, err := c.DelayCascadeReason(ctx, DelayCascadeReasonRequest{
				SlippedTasks: []DelayCascadeSlippedTask{{WBS: "1.1", Name: "Footings", IsCritical: true}},
			})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var httpErr *HTTPError
			if err := tc.call(); !errors.As(err, &httpErr) {
				t.Fatalf("err = %v (%T), want *HTTPError chain", err, err)
			}
		})
	}
}

// TestTask_DecodeToolOutputError covers each task method's
// json.Unmarshal-of-tool-output leg. The server echoes the requested
// tool name but returns a JSON array as the tool input, which cannot
// decode into any typed response struct.
func TestTask_DecodeToolOutputError(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		req := decodeMessagesReq(t, r)
		toolName := ""
		if len(req.Tools) > 0 {
			toolName = req.Tools[0].Name
		}
		resp := messagesResponse{
			ID: "msg_x", Type: "message", Role: "assistant", Model: "m", StopReason: "tool_use",
			Content: []contentBlock{
				{Type: "tool_use", ID: "toolu_1", Name: toolName, Input: json.RawMessage(`[1,2,3]`)},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
	defer cleanup()
	ctx := context.Background()

	cases := []struct {
		name string
		call func() error
	}{
		{"IntentClassify", func() error {
			_, err := c.IntentClassify(ctx, IntentClassifyRequest{Utterance: "u"})
			return err
		}},
		{"InvoiceExtract", func() error {
			_, err := c.InvoiceExtract(ctx, InvoiceExtractRequest{Text: "an invoice"})
			return err
		}},
		{"ProcurementRecommend", func() error {
			_, err := c.ProcurementRecommend(ctx, ProcurementRecommendRequest{MaterialRequestID: uuid.New()})
			return err
		}},
		{"TribunalReview", func() error {
			_, err := c.TribunalReview(ctx, TribunalReviewRequest{DisputeID: uuid.New(), Facts: json.RawMessage(`{"x":1}`)})
			return err
		}},
		{"UpdateSchedule", func() error {
			_, err := c.UpdateSchedule(ctx, UpdateScheduleRequest{
				ProjectID: uuid.New(),
				Tasks:     []ScheduleTaskSnapshot{{TaskID: uuid.New()}},
			})
			return err
		}},
		{"DelayCascadeReason", func() error {
			_, err := c.DelayCascadeReason(ctx, DelayCascadeReasonRequest{
				SlippedTasks: []DelayCascadeSlippedTask{{WBS: "1.1", Name: "Footings", IsCritical: true}},
			})
			return err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil || !strings.Contains(err.Error(), "decode tool output") {
				t.Fatalf("err = %v, want a decode tool output error", err)
			}
		})
	}
}

// TestInvoiceExtract_DocumentFetchError covers the fetchDocumentImage
// error wrap in InvoiceExtract: a doc URL that 404s fails before the
// model is ever called.
func TestInvoiceExtract_DocumentFetchError(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("anthropic endpoint must not be called when the doc fetch fails")
	})
	defer cleanup()

	docSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer docSrv.Close()
	c.docHTTPClient = docSrv.Client()

	if _, err := c.InvoiceExtract(context.Background(), InvoiceExtractRequest{DocumentURL: docSrv.URL}); err == nil {
		t.Fatal("expected an error when the document fetch fails")
	}
}

// TestTribunalReview_RequiresDisputeID covers the dispute_id guard: a nil
// DisputeID is rejected before any model call.
func TestTribunalReview_RequiresDisputeID(t *testing.T) {
	c, cleanup := newTaskTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("model must not be called when dispute_id is missing")
	})
	defer cleanup()

	if _, err := c.TribunalReview(context.Background(), TribunalReviewRequest{Facts: json.RawMessage(`{"x":1}`)}); err == nil {
		t.Fatal("expected an error when dispute_id is nil")
	}
}
