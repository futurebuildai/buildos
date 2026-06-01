package ai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
	// Doc server serves a tiny PNG; AI server asserts an image block.
	docSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
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
