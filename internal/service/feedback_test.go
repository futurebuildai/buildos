package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// The validation legs below all reject BEFORE any transaction begins,
// so a nil pool is safe — a regression that moves validation after
// BeginTxFunc panics loudly here.

func TestFeedbackSubmit_RejectsUnknownCategory(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)
	_, err := s.Submit(context.Background(), SubmitFeedbackInput{
		OrgID: uuid.New(), Category: "rant", Message: "x",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedbackSubmit_RejectsEmptyMessage(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)
	for _, msg := range []string{"", "   ", "\n\t"} {
		_, err := s.Submit(context.Background(), SubmitFeedbackInput{
			OrgID: uuid.New(), Category: "bug", Message: msg,
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Submit(message=%q) err = %v, want ErrInvalidInput", msg, err)
		}
	}
}

func TestFeedbackSubmit_RejectsOversizedMessage(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)
	_, err := s.Submit(context.Background(), SubmitFeedbackInput{
		OrgID: uuid.New(), Category: "bug",
		Message: strings.Repeat("a", feedbackMessageMaxChars+1),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedbackSubmit_RejectsNonObjectContext(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)
	for _, ctxJSON := range []string{`[1,2]`, `"str"`, `{bad`} {
		_, err := s.Submit(context.Background(), SubmitFeedbackInput{
			OrgID: uuid.New(), Category: "bug", Message: "x",
			Context: []byte(ctxJSON),
		})
		if !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Submit(context=%s) err = %v, want ErrInvalidInput", ctxJSON, err)
		}
	}
}

func TestFeedbackSubmit_RejectsOversizedContext(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)
	big := `{"pad":"` + strings.Repeat("x", feedbackContextMaxBytes) + `"}`
	_, err := s.Submit(context.Background(), SubmitFeedbackInput{
		OrgID: uuid.New(), Category: "bug", Message: "x",
		Context: []byte(big),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedbackSubmit_RequiresOrg(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)
	_, err := s.Submit(context.Background(), SubmitFeedbackInput{Category: "bug", Message: "x"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedbackTriage_RejectsUnknownStatus(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)
	_, err := s.Triage(context.Background(), TriageFeedbackInput{
		OrgID: uuid.New(), ID: uuid.New(), Status: "someday",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestFeedbackList_RejectsUnknownStatusFilter(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)
	_, err := s.ListForAdmin(context.Background(), ListFeedbackInput{OrgID: uuid.New(), Status: "bogus"})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// Postgres cannot store U+0000 in TEXT/JSONB - the NUL legs must 400
// at validation (before any tx; nil pool proves it), never 500 at
// insert time. For context the attack vector is the JSON escape form
// (raw 0x00 inside a JSON string is invalid JSON and already rejected
// as a non-object).
func TestFeedbackSubmit_RejectsNUL(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)

	if _, err := s.Submit(context.Background(), SubmitFeedbackInput{
		OrgID: uuid.New(), Category: "bug", Message: "hi" + string(rune(0)) + "there",
	}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("NUL message err = %v, want ErrInvalidInput", err)
	}

	for _, ctxJSON := range []string{
		`{"route":"/x\u0000y"}`,       // escape in a value
		`{"ke\u0000y":"v"}`,           // escape in a key
		`{"nested":{"a":["\u0000"]}}`, // escape deep in arrays/objects
	} {
		if _, err := s.Submit(context.Background(), SubmitFeedbackInput{
			OrgID: uuid.New(), Category: "bug", Message: "ok",
			Context: []byte(ctxJSON),
		}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Submit(context=%s) err = %v, want ErrInvalidInput", ctxJSON, err)
		}
	}
}

func TestFeedbackTriage_RejectsNULNote(t *testing.T) {
	s := NewFeedbackService(nil, nil, nil)
	bad := "note\x00"
	_, err := s.Triage(context.Background(), TriageFeedbackInput{
		OrgID: uuid.New(), ID: uuid.New(), Status: "triaged", TriageNote: &bad,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}
