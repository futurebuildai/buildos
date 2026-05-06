package brain

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

// writeTaskResult writes the discriminated /v1/ai/tasks response
// envelope (the cost-metadata fields plus task-specific output).
func writeTaskResult(t *testing.T, w http.ResponseWriter, runID uuid.UUID, output any) {
	t.Helper()
	rawOut, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("marshal output: %v", err)
	}
	writeEnvelope(w, http.StatusOK, taskResult{
		RunID:        runID,
		TokensUsed:   1234,
		CostCents:    42,
		CurrencyCode: "USD",
		Output:       rawOut,
	})
}

// decodeTaskEnvelope decodes the request body the typed methods
// produce and returns the parsed Task discriminator + raw input bytes.
func decodeTaskEnvelope(t *testing.T, r *http.Request) (string, json.RawMessage) {
	t.Helper()
	var env taskEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env.Task, env.Input
}

func TestMaestroTasks_DailyBriefing_RoundTrip(t *testing.T) {
	wantRunID := uuid.New()
	wantSession := uuid.New()

	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/ai/tasks" {
			t.Errorf("path = %q, want /v1/ai/tasks", r.URL.Path)
		}
		task, inputRaw := decodeTaskEnvelope(t, r)
		if task != "daily_briefing" {
			t.Errorf("task = %q, want daily_briefing", task)
		}
		var input DailyBriefingRequest
		if err := json.Unmarshal(inputRaw, &input); err != nil {
			t.Fatalf("decode input: %v", err)
		}
		if len(input.Tasks) != 2 || input.Tasks[0] != "1.1 Pour footings" {
			t.Errorf("tasks = %v", input.Tasks)
		}
		if input.UserRole != "superintendent" {
			t.Errorf("user_role = %q", input.UserRole)
		}
		writeTaskResult(t, w, wantRunID, map[string]any{
			"session_id": wantSession,
			"reply":      "Lead with the framing alert; then…",
		})
	})
	defer cleanup()

	resp, err := c.Maestro.DailyBriefing(ctxWithToken(t), DailyBriefingRequest{
		Tasks:    []string{"1.1 Pour footings", "1.2 Strip forms"},
		Alerts:   []string{"[critical] Framing inspection failed"},
		UserRole: "superintendent",
	})
	if err != nil {
		t.Fatalf("DailyBriefing: %v", err)
	}
	if resp.RunID != wantRunID {
		t.Errorf("run_id = %s, want %s", resp.RunID, wantRunID)
	}
	if resp.SessionID != wantSession {
		t.Errorf("session_id = %s, want %s", resp.SessionID, wantSession)
	}
	if resp.TokensUsed != 1234 || resp.CostCents != 42 || resp.CurrencyCode != "USD" {
		t.Errorf("cost metadata mismatch: %+v", resp.CostMetadata)
	}
	if resp.Reply == "" {
		t.Error("reply empty")
	}
}

func TestMaestroTasks_IntentClassify_ValidatesUtterance(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called; client must reject empty utterance")
	})
	defer cleanup()

	_, err := c.Maestro.IntentClassify(ctxWithToken(t), IntentClassifyRequest{Utterance: ""})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMaestroTasks_IntentClassify_RoundTrip(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		task, inputRaw := decodeTaskEnvelope(t, r)
		if task != "intent_classify" {
			t.Errorf("task = %q", task)
		}
		var input IntentClassifyRequest
		_ = json.Unmarshal(inputRaw, &input)
		if input.Utterance != "Need 200 sheets of 1/2 OSB" || input.Channel != "sms" {
			t.Errorf("input = %+v", input)
		}
		writeTaskResult(t, w, uuid.New(), map[string]any{
			"intent":     "material_request",
			"confidence": 0.92,
			"entities":   map[string]string{"material": "OSB", "quantity": "200"},
		})
	})
	defer cleanup()

	resp, err := c.Maestro.IntentClassify(ctxWithToken(t), IntentClassifyRequest{
		Utterance: "Need 200 sheets of 1/2 OSB",
		Channel:   "sms",
	})
	if err != nil {
		t.Fatalf("IntentClassify: %v", err)
	}
	if resp.Intent != "material_request" {
		t.Errorf("intent = %q", resp.Intent)
	}
	if resp.Confidence < 0.9 {
		t.Errorf("confidence = %v", resp.Confidence)
	}
	if resp.Entities["material"] != "OSB" {
		t.Errorf("entities = %+v", resp.Entities)
	}
}

func TestMaestroTasks_InvoiceExtract_RequiresInput(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called; client must reject empty payload")
	})
	defer cleanup()

	_, err := c.Maestro.InvoiceExtract(ctxWithToken(t), InvoiceExtractRequest{})
	if err == nil {
		t.Fatal("expected error when neither document_url nor text is set")
	}
}

func TestMaestroTasks_InvoiceExtract_RoundTrip(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		task, _ := decodeTaskEnvelope(t, r)
		if task != "invoice_extract" {
			t.Errorf("task = %q", task)
		}
		writeTaskResult(t, w, uuid.New(), map[string]any{
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

	resp, err := c.Maestro.InvoiceExtract(ctxWithToken(t), InvoiceExtractRequest{
		Text: "Acme Lumber INV-1042 ...",
	})
	if err != nil {
		t.Fatalf("InvoiceExtract: %v", err)
	}
	if resp.VendorName != "Acme Lumber" || resp.InvoiceNo != "INV-1042" {
		t.Errorf("invoice header mismatch: %+v", resp)
	}
	if resp.TotalCents != 125000 || resp.CurrencyCode != "USD" {
		t.Errorf("totals mismatch: total=%d ccy=%q", resp.TotalCents, resp.CurrencyCode)
	}
	if len(resp.LineItems) != 2 || resp.LineItems[0].UnitCents != 450 {
		t.Errorf("line items = %+v", resp.LineItems)
	}
}

func TestMaestroTasks_ProcurementRecommend_RequiresMaterialRequestID(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called; client must reject zero material_request_id")
	})
	defer cleanup()

	_, err := c.Maestro.ProcurementRecommend(ctxWithToken(t), ProcurementRecommendRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMaestroTasks_ProcurementRecommend_RoundTrip(t *testing.T) {
	wantMRID := uuid.New()
	vendorID := uuid.New()

	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		task, inputRaw := decodeTaskEnvelope(t, r)
		if task != "procurement_recommend" {
			t.Errorf("task = %q", task)
		}
		var input ProcurementRecommendRequest
		_ = json.Unmarshal(inputRaw, &input)
		if input.MaterialRequestID != wantMRID {
			t.Errorf("material_request_id mismatch")
		}
		writeTaskResult(t, w, uuid.New(), map[string]any{
			"recommendations": []map[string]any{
				{
					"vendor_id":             vendorID,
					"vendor_name":           "Pacific Builders Supply",
					"predicted_spend_cents": int64(48000),
					"currency_code":         "USD",
					"confidence":            0.78,
					"reasoning":             "Best historical lead time on framing lumber",
				},
			},
		})
	})
	defer cleanup()

	resp, err := c.Maestro.ProcurementRecommend(ctxWithToken(t), ProcurementRecommendRequest{
		MaterialRequestID: wantMRID,
		BudgetCents:       50000,
		CurrencyCode:      "USD",
	})
	if err != nil {
		t.Fatalf("ProcurementRecommend: %v", err)
	}
	if len(resp.Recommendations) != 1 {
		t.Fatalf("recommendations len = %d", len(resp.Recommendations))
	}
	rec := resp.Recommendations[0]
	if rec.VendorID != vendorID || rec.PredictedSpendCents != 48000 || rec.CurrencyCode != "USD" {
		t.Errorf("recommendation mismatch: %+v", rec)
	}
}

func TestMaestroTasks_TribunalReview_RequiresFacts(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called; client must reject empty facts")
	})
	defer cleanup()

	_, err := c.Maestro.TribunalReview(ctxWithToken(t), TribunalReviewRequest{
		DisputeID: uuid.New(),
		// Facts intentionally empty
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMaestroTasks_TribunalReview_RoundTrip(t *testing.T) {
	wantDisputeID := uuid.New()

	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		task, inputRaw := decodeTaskEnvelope(t, r)
		if task != "tribunal_review" {
			t.Errorf("task = %q", task)
		}
		var input TribunalReviewRequest
		_ = json.Unmarshal(inputRaw, &input)
		if input.DisputeID != wantDisputeID {
			t.Errorf("dispute_id mismatch")
		}
		writeTaskResult(t, w, uuid.New(), map[string]any{
			"recommendation": "approve",
			"confidence":     0.81,
			"rationale":      "Change order is within original scope tolerance",
		})
	})
	defer cleanup()

	resp, err := c.Maestro.TribunalReview(ctxWithToken(t), TribunalReviewRequest{
		DisputeID: wantDisputeID,
		Facts:     json.RawMessage(`{"change_order_id":"co-7","delta_cents":2400}`),
	})
	if err != nil {
		t.Fatalf("TribunalReview: %v", err)
	}
	if resp.Recommendation != "approve" {
		t.Errorf("recommendation = %q", resp.Recommendation)
	}
}

func TestMaestroTasks_UpdateSchedule_ValidatesProjectID(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called; client must reject nil project_id")
	})
	defer cleanup()

	_, err := c.Maestro.UpdateSchedule(ctxWithToken(t), UpdateScheduleRequest{
		ProjectID: uuid.Nil,
		Tasks:     []ScheduleTaskSnapshot{{TaskID: uuid.New(), DurationDays: 5}},
	})
	if err == nil {
		t.Fatal("expected error for nil project_id")
	}
}

func TestMaestroTasks_UpdateSchedule_ValidatesTasksNonEmpty(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("server should not be called; client must reject empty tasks")
	})
	defer cleanup()

	_, err := c.Maestro.UpdateSchedule(ctxWithToken(t), UpdateScheduleRequest{
		ProjectID: uuid.New(),
		Tasks:     nil,
	})
	if err == nil {
		t.Fatal("expected error for empty tasks")
	}
}

func TestMaestroTasks_UpdateSchedule_RoundTrip(t *testing.T) {
	wantRunID := uuid.New()
	wantProject := uuid.New()
	wantTaskA := uuid.New()
	wantTaskB := uuid.New()

	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		task, inputRaw := decodeTaskEnvelope(t, r)
		if task != "update_schedule" {
			t.Errorf("task = %q, want update_schedule", task)
		}
		var input UpdateScheduleRequest
		if err := json.Unmarshal(inputRaw, &input); err != nil {
			t.Fatalf("decode input: %v", err)
		}
		if input.ProjectID != wantProject {
			t.Errorf("project_id = %s, want %s", input.ProjectID, wantProject)
		}
		if len(input.Tasks) != 2 {
			t.Fatalf("tasks len = %d, want 2", len(input.Tasks))
		}
		if input.Tasks[0].TaskID != wantTaskA || input.Tasks[0].DurationDays != 7 {
			t.Errorf("tasks[0] = %+v", input.Tasks[0])
		}
		if len(input.Dependencies) != 1 || input.Dependencies[0].DependencyType != "FS" {
			t.Errorf("dependencies = %+v", input.Dependencies)
		}
		// Recommend extending task A by 2 days; leave task B unchanged
		// (no NewDurationDays set — Brain returned a rationale-only row).
		newDur := 9
		writeTaskResult(t, w, wantRunID, map[string]any{
			"adjustments": []ScheduleAdjustment{
				{TaskID: wantTaskA, NewDurationDays: &newDur, Rationale: "weather drift"},
				{TaskID: wantTaskB, Rationale: "monitor only — no change"},
			},
		})
	})
	defer cleanup()

	resp, err := c.Maestro.UpdateSchedule(ctxWithToken(t), UpdateScheduleRequest{
		ProjectID:        wantProject,
		ProjectStartDate: "2026-05-06T00:00:00Z",
		Tasks: []ScheduleTaskSnapshot{
			{TaskID: wantTaskA, WBSCode: "1.1", Name: "Footings", DurationDays: 7, Status: "pending", PercentComplete: 0, IsCritical: true},
			{TaskID: wantTaskB, WBSCode: "1.2", Name: "Strip forms", DurationDays: 2, Status: "pending", PercentComplete: 0, IsCritical: false},
		},
		Dependencies: []ScheduleDepSnapshot{
			{PredecessorID: wantTaskA, SuccessorID: wantTaskB, DependencyType: "FS", LagDays: 0},
		},
	})
	if err != nil {
		t.Fatalf("UpdateSchedule: %v", err)
	}
	if resp.RunID != wantRunID {
		t.Errorf("run_id = %s, want %s", resp.RunID, wantRunID)
	}
	if resp.TokensUsed != 1234 || resp.CostCents != 42 || resp.CurrencyCode != "USD" {
		t.Errorf("cost metadata mismatch: %+v", resp.CostMetadata)
	}
	if len(resp.Adjustments) != 2 {
		t.Fatalf("adjustments len = %d", len(resp.Adjustments))
	}
	if resp.Adjustments[0].NewDurationDays == nil || *resp.Adjustments[0].NewDurationDays != 9 {
		t.Errorf("adjustments[0].NewDurationDays = %v, want *int=9", resp.Adjustments[0].NewDurationDays)
	}
	if resp.Adjustments[1].NewDurationDays != nil {
		t.Errorf("adjustments[1].NewDurationDays = %v, want nil (rationale-only)", resp.Adjustments[1].NewDurationDays)
	}
}

func TestMaestroTasks_HTTPErrorPropagates(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"INVALID_TASK","message":"unknown task name"}}`))
	})
	defer cleanup()

	_, err := c.Maestro.IntentClassify(ctxWithToken(t), IntentClassifyRequest{Utterance: "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("err type = %T, want *HTTPError chain", err)
	}
	if httpErr.StatusCode != http.StatusBadRequest || httpErr.Code != "INVALID_TASK" {
		t.Errorf("HTTPError = %+v", httpErr)
	}
}

func TestMaestroTasks_NoTokenInContext(t *testing.T) {
	c, cleanup := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("server should not be called without token in ctx")
	})
	defer cleanup()

	_, err := c.Maestro.DailyBriefing(context.Background(), DailyBriefingRequest{Tasks: []string{"x"}})
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("err = %v, want ErrUnauthenticated", err)
	}
}
