package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// These tests exercise the validation gates in FeedService.ListFeed /
// DismissCard / ActionCard that run BEFORE the service touches the
// database pool. With a nil pool/store, hitting any post-validation
// path would panic — which is exactly what proves the gates are
// effective.

func newFeedSvcForValidationTests() *FeedService {
	// nil audit is fine — NewFeedService substitutes a no-op recorder
	// when audit is nil.
	return NewFeedService(nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
}

func TestFeedService_ListFeed_RejectsMissingOrg(t *testing.T) {
	svc := newFeedSvcForValidationTests()
	_, err := svc.ListFeed(context.Background(), FeedListOptions{
		CallerOIDCSubject: "sub-1",
		CallerRole:        "admin",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedService_ListFeed_RejectsMissingSubject(t *testing.T) {
	svc := newFeedSvcForValidationTests()
	_, err := svc.ListFeed(context.Background(), FeedListOptions{
		CallerOrgID: uuid.New(),
		CallerRole:  "admin",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedService_ListFeed_RejectsMissingRole(t *testing.T) {
	svc := newFeedSvcForValidationTests()
	_, err := svc.ListFeed(context.Background(), FeedListOptions{
		CallerOrgID:       uuid.New(),
		CallerOIDCSubject: "sub-1",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedService_ListFeed_RejectsBadStatus(t *testing.T) {
	svc := newFeedSvcForValidationTests()
	_, err := svc.ListFeed(context.Background(), FeedListOptions{
		CallerOrgID:       uuid.New(),
		CallerOIDCSubject: "sub-1",
		CallerRole:        "admin",
		StatusFilter:      []string{"bogus"},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedService_ListFeed_RejectsBadPriority(t *testing.T) {
	svc := newFeedSvcForValidationTests()
	_, err := svc.ListFeed(context.Background(), FeedListOptions{
		CallerOrgID:       uuid.New(),
		CallerOIDCSubject: "sub-1",
		CallerRole:        "admin",
		PriorityFilter:    []string{"super-urgent"},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedService_DismissCard_RejectsZeroIDs(t *testing.T) {
	svc := newFeedSvcForValidationTests()
	_, err := svc.DismissCard(context.Background(), uuid.Nil, "sub-1", uuid.New())
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil orgID: err = %v, want ErrInvalidInput", err)
	}
	_, err = svc.DismissCard(context.Background(), uuid.New(), "sub-1", uuid.Nil)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("nil cardID: err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedService_ActionCard_RejectsBadInput(t *testing.T) {
	svc := newFeedSvcForValidationTests()
	cases := []struct {
		name string
		org  uuid.UUID
		card uuid.UUID
		in   FeedActionInput
	}{
		{"nil org", uuid.Nil, uuid.New(), FeedActionInput{ActionType: "approve"}},
		{"nil card", uuid.New(), uuid.Nil, FeedActionInput{ActionType: "approve"}},
		{"empty action_type", uuid.New(), uuid.New(), FeedActionInput{ActionType: ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.ActionCard(context.Background(), c.org, "sub-1", c.card, c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestFeedService_ActionCard_RejectsOversizedPayload(t *testing.T) {
	svc := newFeedSvcForValidationTests()
	// 9 KiB JSON string — over the 8 KiB cap.
	big := bytes.Repeat([]byte("a"), MaxFeedActionPayloadBytes+1024)
	payload, _ := json.Marshal(string(big))
	_, err := svc.ActionCard(context.Background(), uuid.New(), "sub-1", uuid.New(), FeedActionInput{
		ActionType: "approve",
		Payload:    payload,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), "payload exceeds") {
		t.Errorf("error message should call out the size cap: %v", err)
	}
}

// TestMarshalAudit covers all three legs of the audit-payload helper: a
// nil value short-circuits to nil (no "null" bytes on the audit row), a
// marshalable value round-trips to its JSON, and an unmarshalable value
// (a channel — json.Marshal returns an UnsupportedTypeError) soft-fails
// to nil rather than panicking inside the audit write.
func TestMarshalAudit(t *testing.T) {
	if got := marshalAudit(nil); got != nil {
		t.Errorf("marshalAudit(nil) = %q, want nil", got)
	}

	got := marshalAudit(map[string]any{"k": "v"})
	if string(got) != `{"k":"v"}` {
		t.Errorf("marshalAudit(map) = %q, want {\"k\":\"v\"}", got)
	}

	// chan is not JSON-marshalable → the error leg returns nil.
	if got := marshalAudit(make(chan int)); got != nil {
		t.Errorf("marshalAudit(chan) = %q, want nil (marshal error leg)", got)
	}
}

// TestIsValidFeedStatus covers the four allowed migration-003 statuses
// (true) plus the default reject leg the ListFeed filter test never
// reaches with a known-good filter.
func TestIsValidFeedStatus(t *testing.T) {
	for _, s := range []string{"active", "dismissed", "actioned", "expired"} {
		if !isValidFeedStatus(s) {
			t.Errorf("isValidFeedStatus(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "ACTIVE", "bogus"} {
		if isValidFeedStatus(s) {
			t.Errorf("isValidFeedStatus(%q) = true, want false", s)
		}
	}
}
