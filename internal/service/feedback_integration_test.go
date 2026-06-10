//go:build integration

package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// TestFeedbackService_SubmitListTriageRoundTrip drives the full loop
// the way Kelbrook will use it: a field worker submits from the
// widget, an admin lists + triages, and every mutation leaves an audit
// entry whose metadata carries category/status but NEVER the free-text
// message (posture L-6).
func TestFeedbackService_SubmitListTriageRoundTrip(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := &capturingAuditRecorder{}
	svc := NewFeedbackService(pool, store.NewFeedbackStore(), rec)
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Kelbrook Construction")

	secret := "the framing crew said the daily-log photos drop on bad signal"
	fb, err := svc.Submit(ctx, SubmitFeedbackInput{
		OrgID:    orgID,
		UserSub:  "field-worker-sub",
		Category: "bug",
		Message:  "  " + secret + "  ", // service must trim
		Context:  []byte(`{"route":"/field/daily-log","role":"field_worker","app_version":"0.1.0"}`),
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if fb.Status != "new" || fb.Message != secret {
		t.Errorf("submitted = %+v (message must be trimmed, status new)", fb)
	}

	// Audit: feedback.submitted recorded, message NOT in metadata.
	if len(rec.entries) != 1 || rec.entries[0].Action != "feedback.submitted" {
		t.Fatalf("audit entries = %+v, want one feedback.submitted", rec.entries)
	}
	if strings.Contains(string(rec.entries[0].Metadata), "framing crew") {
		t.Error("audit metadata must not carry the Confidential message body")
	}

	// Admin list (harvest surface), paginated.
	page, err := svc.ListForAdmin(ctx, ListFeedbackInput{OrgID: orgID, Status: "new"})
	if err != nil {
		t.Fatalf("ListForAdmin: %v", err)
	}
	if len(page.Feedback) != 1 || page.Feedback[0].ID != fb.ID || page.Total != 1 {
		t.Fatalf("list = %+v", page)
	}

	// Triage to planned with a note.
	note := "filed as GH issue #42"
	triaged, err := svc.Triage(ctx, TriageFeedbackInput{
		OrgID: orgID, ID: fb.ID, Status: "planned", TriageNote: &note, UserSub: "admin-sub",
	})
	if err != nil {
		t.Fatalf("Triage: %v", err)
	}
	if triaged.Status != "planned" || triaged.TriageNote != note {
		t.Errorf("triaged = %+v", triaged)
	}
	if len(rec.entries) != 2 || rec.entries[1].Action != "feedback.triaged" {
		t.Fatalf("audit entries = %+v, want feedback.triaged second", rec.entries)
	}
	if strings.Contains(string(rec.entries[1].Metadata), "GH issue") {
		t.Error("audit metadata must not carry the Confidential triage note")
	}

	// Foreign org: list empty, triage 404s, audit untouched.
	otherOrg := uuid.New()
	testdb.SeedOrg(t, pool, otherOrg, "Other Builder")
	otherPage, err := svc.ListForAdmin(ctx, ListFeedbackInput{OrgID: otherOrg})
	if err != nil {
		t.Fatalf("ListForAdmin(other): %v", err)
	}
	if len(otherPage.Feedback) != 0 {
		t.Errorf("cross-org list leak: %+v", otherPage)
	}
	if _, err := svc.Triage(ctx, TriageFeedbackInput{
		OrgID: otherOrg, ID: fb.ID, Status: "declined", UserSub: "intruder",
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org triage err = %v, want ErrNotFound", err)
	}
	if len(rec.entries) != 2 {
		t.Errorf("failed cross-org triage must not audit: %+v", rec.entries)
	}
}

// TestFeedbackService_SubmitThrottle proves the per-(org,user) flood
// guard: the cap'th+1 submission in the window returns ErrRateLimited
// and writes neither a row nor an audit entry, while a DIFFERENT user
// in the same org is unaffected.
func TestFeedbackService_SubmitThrottle(t *testing.T) {
	pool := testdb.NewPool(t)
	rec := &capturingAuditRecorder{}
	svc := NewFeedbackService(pool, store.NewFeedbackStore(), rec)
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Flood Co")

	for i := 0; i < feedbackSubmitMaxPerHour; i++ {
		if _, err := svc.Submit(ctx, SubmitFeedbackInput{
			OrgID: orgID, UserSub: "flooder", Category: "other", Message: "spam",
		}); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	auditCount := len(rec.entries)

	if _, err := svc.Submit(ctx, SubmitFeedbackInput{
		OrgID: orgID, UserSub: "flooder", Category: "other", Message: "one too many",
	}); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("over-cap submit err = %v, want ErrRateLimited", err)
	}
	if len(rec.entries) != auditCount {
		t.Error("throttled submit must not write an audit entry")
	}

	// A different user in the same org is not collateral damage.
	if _, err := svc.Submit(ctx, SubmitFeedbackInput{
		OrgID: orgID, UserSub: "grant-sub", Category: "idea", Message: "legit report",
	}); err != nil {
		t.Fatalf("other user blocked by flooder's throttle: %v", err)
	}
}
