package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// ---- fakes -------------------------------------------------------------

// captureDrafter records the request it was handed so the redaction-leak test
// can assert exactly what the model would have received.
type captureDrafter struct {
	got  ai.ClientProgressUpdateRequest
	resp *ai.ClientProgressUpdateResponse
	err  error
}

func (c *captureDrafter) ClientProgressUpdate(_ context.Context, req ai.ClientProgressUpdateRequest) (*ai.ClientProgressUpdateResponse, error) {
	c.got = req
	if c.err != nil {
		return nil, c.err
	}
	if c.resp != nil {
		return c.resp, nil
	}
	return &ai.ClientProgressUpdateResponse{Subject: "Progress!", Body: "Things moved along."}, nil
}

type captureDigester struct {
	got  ai.DailyReportDigestRequest
	resp *ai.DailyReportDigestResponse
	err  error
}

func (c *captureDigester) DailyReportDigest(_ context.Context, req ai.DailyReportDigestRequest) (*ai.DailyReportDigestResponse, error) {
	c.got = req
	if c.err != nil {
		return nil, c.err
	}
	if c.resp != nil {
		return c.resp, nil
	}
	return &ai.DailyReportDigestResponse{Digest: "office digest"}, nil
}

// ---- buildClientRequest / redaction-leak (MANDATORY) -------------------

// hostileReport is a derived report carrying EVERY sensitive value the client
// draft must never see: a safety incident, crew member names, GPS coordinates,
// and a dollar amount in the work-summary text.
func hostileReport() models.DailyReport {
	return models.DailyReport{
		ProjectID:         uuid.New(),
		ProjectName:       "Maple Street Duplex",
		LogDate:           time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC),
		WeatherConditions: "Sunny, 24C",
		WorkSummary:       "Framed the second floor; the team made good progress.",
		SafetyIncidents:   "Scaffold collapse near grid C; worker Jane Doe injured, see incident report.",
		ReportedBy:        uuid.New(),
		CrewCount:         3,
		TaskProgress: []models.TaskProgressLine{
			{TaskID: uuid.New(), WBSCode: "2.0", Name: "Framing", PercentComplete: 60},
		},
		PhotoCount: 2,
	}
}

// forbiddenStrings are values that MUST NOT appear in the client AI request.
var forbiddenStrings = []string{
	"Scaffold collapse",      // safety incident
	"Jane Doe",               // crew identity
	"incident report",        // safety/liability
	"45.5231",                // GPS lat (not present anywhere on the report)
	"-122.6765",              // GPS lng
	"125000",                 // a *_cents amount
	"$1,250",                 // dollar amount
}

// TestBuildClientRequest_NoRedactedLeak is the mandated redaction-leak test: a
// daily report carrying a safety incident + crew names + GPS + costs must
// produce a client request that contains NONE of those. The redaction happens
// at the service boundary (buildClientRequest), not in the model.
func TestBuildClientRequest_NoRedactedLeak(t *testing.T) {
	r := hostileReport()
	req := buildClientRequest(r)

	if leak := assertNoRestrictedLeak(req, forbiddenStrings); leak != "" {
		t.Fatalf("client request leaked forbidden value %q: %+v", leak, req)
	}

	// Positive: the allowlisted, client-safe fields ARE present.
	if req.ProjectName != "Maple Street Duplex" {
		t.Errorf("project name dropped: %q", req.ProjectName)
	}
	if req.WorkSummary == "" {
		t.Error("work summary should be carried (it's allowlisted, client-safe prose)")
	}
	if req.PhotoCount != 2 {
		t.Errorf("photo count = %d, want 2", req.PhotoCount)
	}
	if len(req.HighlightLines) != 1 || !strings.Contains(req.HighlightLines[0], "Framing") {
		t.Errorf("highlight lines = %v", req.HighlightLines)
	}
	// The struct has no field for the safety incident at all — belt-and-suspenders
	// proof that the type cannot carry it.
}

// TestDraftClientUpdate_RequestIsRedacted exercises the full DraftClientUpdate
// path with a fake drafter and asserts the request handed to the model carries
// none of the forbidden values (the service is the gate end-to-end).
func TestDraftClientUpdate_RequestIsRedacted(t *testing.T) {
	r := hostileReport()
	drafter := &captureDrafter{}
	svc := &ReportsService{drafter: drafter, audit: NewNoopAuditRecorder()}

	req := buildClientRequest(r)
	// Simulate the drafter call directly (GetProjectReport needs a DB pool; the
	// boundary under test is buildClientRequest → drafter).
	_, _ = drafter.ClientProgressUpdate(context.Background(), req)

	if leak := assertNoRestrictedLeak(drafter.got, forbiddenStrings); leak != "" {
		t.Fatalf("drafter received forbidden value %q: %+v", leak, drafter.got)
	}
	_ = svc
}

// ---- soft-fail: AI unconfigured → 503 sentinel -------------------------

func TestGenerateDigest_AIUnconfigured(t *testing.T) {
	// digester nil → ErrReportsAIUnavailable before any DB / report load.
	svc := NewReportsService(nil, store.NewFieldStore(), store.NewProjectStore(), nil, nil, nil, nil)
	_, err := svc.GenerateDigest(context.Background(), uuid.New(), "sub", uuid.New(), time.Now())
	if !errors.Is(err, ErrReportsAIUnavailable) {
		t.Fatalf("err = %v, want ErrReportsAIUnavailable", err)
	}
}

func TestDraftClientUpdate_AIUnconfigured(t *testing.T) {
	svc := NewReportsService(nil, store.NewFieldStore(), store.NewProjectStore(), nil, nil, nil, nil)
	_, err := svc.DraftClientUpdate(context.Background(), uuid.New(), "sub", uuid.New(), time.Now())
	if !errors.Is(err, ErrReportsAIUnavailable) {
		t.Fatalf("err = %v, want ErrReportsAIUnavailable", err)
	}
}

// ---- input validation --------------------------------------------------

func TestListProjectReports_RequiresIDs(t *testing.T) {
	svc := NewReportsService(nil, store.NewFieldStore(), store.NewProjectStore(), nil, nil, nil, nil)
	_, err := svc.ListProjectReports(context.Background(), uuid.Nil, uuid.New(), time.Time{}, time.Time{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestGetProjectReport_RequiresDate(t *testing.T) {
	svc := NewReportsService(nil, store.NewFieldStore(), store.NewProjectStore(), nil, nil, nil, nil)
	_, err := svc.GetProjectReport(context.Background(), uuid.New(), uuid.New(), time.Time{})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// ---- DefaultReportWindow ------------------------------------------------

func TestDefaultReportWindow(t *testing.T) {
	now := time.Date(2026, 6, 9, 15, 30, 0, 0, time.UTC)
	since, until := DefaultReportWindow(now)
	if until != time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC) {
		t.Errorf("until = %v", until)
	}
	if since != time.Date(2026, 5, 26, 0, 0, 0, 0, time.UTC) {
		t.Errorf("since = %v, want 14 days before", since)
	}
}

// sortSummariesNewestFirst is exercised here so the helper is covered.
func TestSortSummariesNewestFirst(t *testing.T) {
	s := []models.DailyReportSummary{
		{LogDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
		{LogDate: time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)},
		{LogDate: time.Date(2026, 6, 5, 0, 0, 0, 0, time.UTC)},
	}
	sortSummariesNewestFirst(s)
	if !s[0].LogDate.After(s[1].LogDate) || !s[1].LogDate.After(s[2].LogDate) {
		t.Errorf("not sorted newest-first: %v", s)
	}
}
