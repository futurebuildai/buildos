package mailer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubResolver is a test KeyResolver returning a fixed key/error.
type stubResolver struct {
	key string
	err error
}

func (s stubResolver) ResendKey(_ context.Context, _ string) (string, error) {
	return s.key, s.err
}

func TestResendMailerHappyPath(t *testing.T) {
	const wantKey = "re_test_key_123"

	var (
		gotAuth        string
		gotContentType string
		gotPath        string
		gotMethod      string
		gotBody        resendRequest
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("Content-Type")
		gotPath = r.URL.Path
		gotMethod = r.Method
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"email_abc"}`))
	}))
	defer srv.Close()

	m := NewResendMailer(stubResolver{key: wantKey}, "noreply@buildos.dev", "BuildOS",
		WithBaseURL(srv.URL),
		WithHTTPClient(srv.Client()),
	)

	msg := Message{
		To:       "field@example.com",
		Subject:  "Daily Briefing",
		HTMLBody: "<p>hi</p>",
		TextBody: "hi",
	}
	if err := m.Send(context.Background(), "org-1", msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/emails" {
		t.Errorf("path = %q, want /emails", gotPath)
	}
	if gotAuth != "Bearer "+wantKey {
		t.Errorf("auth header = %q, want bearer with key", gotAuth)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type = %q, want application/json", gotContentType)
	}
	if gotBody.From != "BuildOS <noreply@buildos.dev>" {
		t.Errorf("from = %q, want %q", gotBody.From, "BuildOS <noreply@buildos.dev>")
	}
	if len(gotBody.To) != 1 || gotBody.To[0] != "field@example.com" {
		t.Errorf("to = %v, want [field@example.com]", gotBody.To)
	}
	if gotBody.Subject != "Daily Briefing" {
		t.Errorf("subject = %q", gotBody.Subject)
	}
	if gotBody.HTML != "<p>hi</p>" {
		t.Errorf("html = %q", gotBody.HTML)
	}
	if gotBody.Text != "hi" {
		t.Errorf("text = %q", gotBody.Text)
	}
}

func TestResendMailerAccepted202(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	m := NewResendMailer(stubResolver{key: "re_k"}, "a@b.com", "",
		WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))

	if err := m.Send(context.Background(), "org-1", Message{To: "x@y.com", Subject: "s"}); err != nil {
		t.Fatalf("Send on 202: %v", err)
	}
}

func TestResendMailerBareFromWhenNoName(t *testing.T) {
	var gotFrom string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body resendRequest
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		gotFrom = body.From
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewResendMailer(stubResolver{key: "re_k"}, "noreply@buildos.dev", "",
		WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))

	if err := m.Send(context.Background(), "org-1", Message{To: "x@y.com", Subject: "s"}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if gotFrom != "noreply@buildos.dev" {
		t.Errorf("from = %q, want bare address", gotFrom)
	}
}

func TestResendMailerUnconfigured(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	m := NewResendMailer(stubResolver{key: ""}, "a@b.com", "BuildOS",
		WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))

	err := m.Send(context.Background(), "org-1", Message{To: "x@y.com", Subject: "s"})
	if !errors.Is(err, ErrMailerUnconfigured) {
		t.Fatalf("got err=%v, want ErrMailerUnconfigured", err)
	}
	if hit {
		t.Error("server should not be hit when key is empty")
	}
}

func TestResendMailerNon2xxError(t *testing.T) {
	const secretKey = "re_super_secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"bad recipient"}`))
	}))
	defer srv.Close()

	m := NewResendMailer(stubResolver{key: secretKey}, "a@b.com", "BuildOS",
		WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))

	err := m.Send(context.Background(), "org-1", Message{To: "x@y.com", Subject: "s"})
	if err == nil {
		t.Fatal("expected error on non-2xx")
	}

	var se *SendError
	if !errors.As(err, &se) {
		t.Fatalf("got err type %T, want *SendError", err)
	}
	if se.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", se.StatusCode)
	}
	// The error must NOT leak the API key.
	if strings.Contains(err.Error(), secretKey) {
		t.Fatalf("error message leaked the API key: %q", err.Error())
	}
}

func TestResendMailerResolverError(t *testing.T) {
	m := NewResendMailer(stubResolver{err: errors.New("vault down")}, "a@b.com", "BuildOS")
	err := m.Send(context.Background(), "org-1", Message{To: "x@y.com", Subject: "s"})
	if err == nil {
		t.Fatal("expected error when resolver fails")
	}
	if errors.Is(err, ErrMailerUnconfigured) {
		t.Fatal("resolver error should not be ErrMailerUnconfigured")
	}
}

func TestNoopMailerSend(t *testing.T) {
	m := NewNoopMailer(nil)
	if err := m.Send(context.Background(), "org-1", Message{To: "x@y.com", Subject: "s"}); err != nil {
		t.Fatalf("noop Send: %v", err)
	}
}
