package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Validation gates only — pool/store nil. Post-validation paths would
// panic, which is what proves the gates work.

func newFieldSvcForValidationTests() *FieldService {
	// nil audit falls back to a no-op recorder; the validation gates
	// run before any post-validation path so nil pool/store stay
	// safe.
	return NewFieldService(nil, nil, nil, nil)
}

func ptrFloat(f float64) *float64 { return &f }

func TestFieldService_Sync_RejectsBadInput(t *testing.T) {
	svc := newFieldSvcForValidationTests()
	if _, err := svc.Sync(context.Background(), SyncOptions{CallerOIDCSubject: "sub-1"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.Sync(context.Background(), SyncOptions{CallerOrgID: uuid.New()}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("empty subject: err = %v, want ErrInvalidInput", err)
	}
}

func TestFieldService_ReportProgress_RejectsBadInput(t *testing.T) {
	svc := newFieldSvcForValidationTests()
	bigNotes := strings.Repeat("a", MaxFieldNotesLength+1)
	cases := []struct {
		name string
		org  uuid.UUID
		sub  string
		in   ReportProgressInput
	}{
		{"nil org", uuid.Nil, "sub-1", ReportProgressInput{TaskID: uuid.New(), IdempotencyKey: uuid.New(), PercentComplete: 50}},
		{"empty sub", uuid.New(), "", ReportProgressInput{TaskID: uuid.New(), IdempotencyKey: uuid.New(), PercentComplete: 50}},
		{"nil task", uuid.New(), "sub-1", ReportProgressInput{IdempotencyKey: uuid.New(), PercentComplete: 50}},
		{"nil key", uuid.New(), "sub-1", ReportProgressInput{TaskID: uuid.New(), PercentComplete: 50}},
		{"pct < 0", uuid.New(), "sub-1", ReportProgressInput{TaskID: uuid.New(), IdempotencyKey: uuid.New(), PercentComplete: -1}},
		{"pct > 100", uuid.New(), "sub-1", ReportProgressInput{TaskID: uuid.New(), IdempotencyKey: uuid.New(), PercentComplete: 101}},
		{"big notes", uuid.New(), "sub-1", ReportProgressInput{TaskID: uuid.New(), IdempotencyKey: uuid.New(), PercentComplete: 50, Notes: &bigNotes}},
		{"half-fix gps", uuid.New(), "sub-1", ReportProgressInput{TaskID: uuid.New(), IdempotencyKey: uuid.New(), PercentComplete: 50, GPSLat: ptrFloat(40)}},
		{"lat oob", uuid.New(), "sub-1", ReportProgressInput{TaskID: uuid.New(), IdempotencyKey: uuid.New(), PercentComplete: 50, GPSLat: ptrFloat(91), GPSLng: ptrFloat(0)}},
		{"lng oob", uuid.New(), "sub-1", ReportProgressInput{TaskID: uuid.New(), IdempotencyKey: uuid.New(), PercentComplete: 50, GPSLat: ptrFloat(0), GPSLng: ptrFloat(181)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.ReportProgress(context.Background(), c.org, c.sub, c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestFieldService_Checkin_RejectsBadInput(t *testing.T) {
	svc := newFieldSvcForValidationTests()
	cases := []struct {
		name string
		org  uuid.UUID
		sub  string
		in   CheckinInput
	}{
		{"nil org", uuid.Nil, "sub-1", CheckinInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New()}},
		{"empty sub", uuid.New(), "", CheckinInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New()}},
		{"nil project", uuid.New(), "sub-1", CheckinInput{IdempotencyKey: uuid.New()}},
		{"nil key", uuid.New(), "sub-1", CheckinInput{ProjectID: uuid.New()}},
		{"half gps", uuid.New(), "sub-1", CheckinInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), GPSLat: ptrFloat(40)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.Checkin(context.Background(), c.org, c.sub, c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestFieldService_DailyLog_RejectsBadInput(t *testing.T) {
	svc := newFieldSvcForValidationTests()
	now := time.Now().UTC()
	bigSummary := strings.Repeat("x", MaxFieldNotesLength+1)
	cases := []struct {
		name string
		org  uuid.UUID
		sub  string
		in   DailyLogInput
	}{
		{"nil org", uuid.Nil, "sub-1", DailyLogInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), LogDate: now, WorkSummary: "ok"}},
		{"empty sub", uuid.New(), "", DailyLogInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), LogDate: now, WorkSummary: "ok"}},
		{"nil project", uuid.New(), "sub-1", DailyLogInput{IdempotencyKey: uuid.New(), LogDate: now, WorkSummary: "ok"}},
		{"nil key", uuid.New(), "sub-1", DailyLogInput{ProjectID: uuid.New(), LogDate: now, WorkSummary: "ok"}},
		{"zero date", uuid.New(), "sub-1", DailyLogInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), WorkSummary: "ok"}},
		{"empty summary", uuid.New(), "sub-1", DailyLogInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), LogDate: now, WorkSummary: "  "}},
		{"oversized summary", uuid.New(), "sub-1", DailyLogInput{ProjectID: uuid.New(), IdempotencyKey: uuid.New(), LogDate: now, WorkSummary: bigSummary}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.DailyLog(context.Background(), c.org, c.sub, c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

// Smoke test that json.RawMessage on the input type round-trips
// through CheckinInput without panicking. (No DB writes.)
func TestFieldService_Checkin_AcceptsCrewMembersJSON(t *testing.T) {
	svc := newFieldSvcForValidationTests()
	in := CheckinInput{
		ProjectID:      uuid.New(),
		CrewMembers:    json.RawMessage(`[{"worker_id":"abc"}]`),
		IdempotencyKey: uuid.New(),
	}
	// We expect ErrInvalidInput here since the org_id is nil — the
	// goal is to confirm we exit BEFORE touching the pool, not that
	// the call succeeds.
	_, err := svc.Checkin(context.Background(), uuid.Nil, "sub-1", in)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}
