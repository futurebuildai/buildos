package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---- daily_report_digest ----------------------------------------------

func TestDailyReportDigest_DispatchAndText(t *testing.T) {
	c, closeSrv := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertCommonHeaders(t, r)
		req := decodeMessagesReq(t, r)
		// FastModel is selected for the digest (cheap prose).
		if req.Model != "claude-sonnet-4-5" {
			t.Errorf("model = %q, want fast model", req.Model)
		}
		// The office digest is a free-text call (no tool); the safety incident
		// IS part of the prompt (office surface).
		joined := userText(req)
		if !strings.Contains(joined, "scaffold collapse") {
			t.Errorf("office digest prompt should carry the safety incident, got %q", joined)
		}
		writeText(w, "Office digest: framing progressed; one safety incident logged.")
	})
	defer closeSrv()

	resp, err := c.DailyReportDigest(context.Background(), DailyReportDigestRequest{
		ProjectName:     "Maple Duplex",
		LogDate:         "2026-06-09",
		WorkSummary:     "Framed the second floor.",
		SafetyIncidents: "scaffold collapse near grid C",
		CrewCount:       4,
		PhotoCount:      3,
		TaskProgress:    []string{"2.0 Framing — 60%"},
	})
	if err != nil {
		t.Fatalf("DailyReportDigest: %v", err)
	}
	if !strings.Contains(resp.Digest, "Office digest") {
		t.Errorf("digest = %q", resp.Digest)
	}
}

func TestDailyReportDigest_Unconfigured(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	c, err := NewClient(Config{KeyResolver: staticKey(""), BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.DailyReportDigest(context.Background(), DailyReportDigestRequest{ProjectName: "X"})
	if !errors.Is(err, ErrUnconfigured) {
		t.Errorf("err = %v, want ErrUnconfigured", err)
	}
	if called {
		t.Error("server should not be called when key is empty")
	}
}

// ---- client_progress_update -------------------------------------------

func TestClientProgressUpdate_DispatchAndDecode(t *testing.T) {
	c, closeSrv := newTaskTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assertCommonHeaders(t, r)
		req := decodeMessagesReq(t, r)
		if req.Model != "claude-sonnet-4-5" {
			t.Errorf("model = %q, want fast model", req.Model)
		}
		// Structured tool call: compose_client_update.
		if len(req.Tools) == 0 || req.Tools[0].Name != "compose_client_update" {
			t.Errorf("expected compose_client_update tool, got %+v", req.Tools)
		}
		writeToolUse(t, w, "compose_client_update", map[string]string{
			"subject": "Your home is coming along!",
			"body":    "This week the framing went up...",
		})
	})
	defer closeSrv()

	resp, err := c.ClientProgressUpdate(context.Background(), ClientProgressUpdateRequest{
		ProjectName: "Maple Duplex",
		PeriodStart: "2026-06-09",
		PeriodEnd:   "2026-06-09",
		WorkSummary: "Framed the second floor.",
		PhotoCount:  3,
	})
	if err != nil {
		t.Fatalf("ClientProgressUpdate: %v", err)
	}
	if resp.Subject == "" || resp.Body == "" {
		t.Errorf("empty draft: %+v", resp)
	}
}

func TestClientProgressUpdate_Unconfigured(t *testing.T) {
	c, err := NewClient(Config{KeyResolver: staticKey(""), BaseURL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	_, err = c.ClientProgressUpdate(context.Background(), ClientProgressUpdateRequest{ProjectName: "X"})
	if !errors.Is(err, ErrUnconfigured) {
		t.Errorf("err = %v, want ErrUnconfigured", err)
	}
}

// userText flattens the user message content blocks into one string for prompt
// assertions.
func userText(req messagesRequest) string {
	var sb strings.Builder
	for _, m := range req.Messages {
		for _, b := range m.Content {
			sb.WriteString(b.Text)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}
