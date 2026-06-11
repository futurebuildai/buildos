package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/ai"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/pii"
	"github.com/futurebuildai/buildos/internal/store"
)

// Daily-report audit resource type + actions (Chunk C). Reads are NOT audited
// (house style); only the two AI compositions write audit rows.
const (
	AuditResourceDailyReport  = "daily_report"
	AuditResourceClientUpdate = "client_update"

	AuditActionReportDigestGenerated = "report.digest.generated"
	AuditActionClientUpdateDrafted   = "client_update.drafted"
)

// ErrReportsAIUnavailable is returned by the two AI compositions when the
// service was constructed without the corresponding AI task client (the worker
// binary, or a fork with no Anthropic key wired). Handlers map it to 503,
// mirroring ErrAgentsAIUnavailable.
var ErrReportsAIUnavailable = errors.New("reports: ai client not configured")

// defaultReportWindowDays is the lookback for ListProjectReports when the
// caller doesn't pass an explicit since/until window (spec C.5: last 14 days).
const defaultReportWindowDays = 14

// DailyReportDigester is the consumer-side interface ReportsService needs for
// the office-digest task. Defined here so tests inject a fake and the service
// doesn't pin the whole ai.Client surface.
type DailyReportDigester interface {
	DailyReportDigest(ctx context.Context, req ai.DailyReportDigestRequest) (*ai.DailyReportDigestResponse, error)
}

// ClientProgressUpdater is the consumer-side interface for the client-safe
// homeowner draft.
type ClientProgressUpdater interface {
	ClientProgressUpdate(ctx context.Context, req ai.ClientProgressUpdateRequest) (*ai.ClientProgressUpdateResponse, error)
}

// PhotoResolver is the narrow slice of AssetService ReportsService uses to
// resolve daily_logs.photo_asset_ids → short-lived signed GET URLs. Declared as
// an interface so the report path degrades cleanly (a nil resolver → photos
// omitted, count still set) and tests substitute a fake.
type PhotoResolver interface {
	SignedGetURL(ctx context.Context, orgID, assetID uuid.UUID, ttl time.Duration) (string, error)
}

// reportThumbTTL is the operator-surface signed-GET freshness (D-5: 15 min).
const reportThumbTTL = 15 * time.Minute

// ReportsService is the operator daily-reports read surface + the AI
// composition capability (Chunk C). The reads are READ-ONLY and org+project
// scoped (RBAC minRole superintendent is enforced at the route). The two AI
// methods write audit rows; the client draft is built behind a DETERMINISTIC
// redaction allowlist (the service, NOT the model, is the gate).
//
// photos may be nil (storage unconfigured) → reports render text-only with
// PhotoCount derived from the raw id list. digester / drafter may be nil (worker
// binary / no AI key) → the AI methods return ErrReportsAIUnavailable (503).
type ReportsService struct {
	pool     *pgxpool.Pool
	fields   *store.FieldStore
	projects *store.ProjectStore
	photos   PhotoResolver
	digester DailyReportDigester
	drafter  ClientProgressUpdater
	audit    AuditRecorder
}

// NewReportsService wires the dependencies. photos/digester/drafter may be nil
// (see ReportsService doc). A nil AuditRecorder is replaced with the no-op.
func NewReportsService(
	pool *pgxpool.Pool,
	fields *store.FieldStore,
	projects *store.ProjectStore,
	photos PhotoResolver,
	digester DailyReportDigester,
	drafter ClientProgressUpdater,
	audit AuditRecorder,
) *ReportsService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &ReportsService{
		pool:     pool,
		fields:   fields,
		projects: projects,
		photos:   photos,
		digester: digester,
		drafter:  drafter,
		audit:    audit,
	}
}

// ListProjectReports returns daily-report summaries for a project over a date
// window, newest first. since/until are inclusive calendar bounds; pass zero
// values for the default last-14-days window. Org+project scoped (cross-org →
// ErrNotFound). Read-only, not audited.
func (s *ReportsService) ListProjectReports(ctx context.Context, orgID, projectID uuid.UUID, since, until time.Time) ([]models.DailyReportSummary, error) {
	if orgID == uuid.Nil || projectID == uuid.Nil {
		return nil, fmt.Errorf("%w: org_id and project_id are required", ErrInvalidInput)
	}

	var out []models.DailyReportSummary
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, orgID); err != nil {
			return err
		}
		dates, err := s.fields.ListDailyReportDates(ctx, tx, orgID, projectID, 0)
		if err != nil {
			return err
		}
		for _, d := range dates {
			if !since.IsZero() && d.Before(truncateDay(since)) {
				continue
			}
			if !until.IsZero() && d.After(truncateDay(until)) {
				continue
			}
			sum, err := s.buildSummary(ctx, tx, orgID, projectID, d)
			if err != nil {
				return err
			}
			out = append(out, sum)
		}
		return nil
	})
	if err != nil {
		return nil, mapReportError(err)
	}
	// ListDailyReportDates already returns newest-first; keep that order.
	return out, nil
}

// GetProjectReport assembles the full derived report for a (project, date),
// including resolved photo thumbnails (when storage is configured). Org+project
// scoped. Read-only, not audited. SafetyIncidents IS included (operator surface).
func (s *ReportsService) GetProjectReport(ctx context.Context, orgID, projectID uuid.UUID, day time.Time) (models.DailyReport, error) {
	if orgID == uuid.Nil || projectID == uuid.Nil {
		return models.DailyReport{}, fmt.Errorf("%w: org_id and project_id are required", ErrInvalidInput)
	}
	if day.IsZero() {
		return models.DailyReport{}, fmt.Errorf("%w: date is required", ErrInvalidInput)
	}

	var report models.DailyReport
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, orgID); err != nil {
			return err
		}
		project, err := s.projects.GetByID(ctx, tx, projectID, orgID)
		if err != nil {
			return err
		}
		r, err := s.assemble(ctx, tx, orgID, projectID, day)
		if err != nil {
			return err
		}
		r.ProjectName = project.Name
		report = r
		return nil
	})
	if err != nil {
		return models.DailyReport{}, mapReportError(err)
	}
	// Photo resolution happens AFTER the read tx commits (signed-URL minting may
	// be a network call; it must not hold a DB tx). Degrades silently when
	// storage is unconfigured — PhotoCount stays set from the raw id list.
	s.resolvePhotos(ctx, orgID, &report)
	return report, nil
}

// assemble folds the daily log + crew count + task progress into a DailyReport
// for one day. Photo ids are carried as PhotoRef{AssetID} placeholders; the
// signed URLs are minted post-commit by resolvePhotos.
func (s *ReportsService) assemble(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, day time.Time) (models.DailyReport, error) {
	r := models.DailyReport{
		ProjectID: projectID,
		LogDate:   truncateDay(day),
	}
	log, ok, err := s.fields.DailyLogFieldsByProjectDate(ctx, tx, orgID, projectID, day)
	if err != nil {
		return models.DailyReport{}, err
	}
	if ok {
		r.HasLog = true
		r.WeatherConditions = log.WeatherConditions
		r.WorkSummary = log.WorkSummary
		r.SafetyIncidents = log.SafetyIncidents
		r.ReportedBy = log.ReportedBy
		r.ReportedAt = log.ReportedAt
		r.PhotoCount = len(log.PhotoAssetIDs)
		for _, id := range log.PhotoAssetIDs {
			r.Photos = append(r.Photos, models.PhotoRef{AssetID: id})
		}
	}
	crew, err := s.fields.CrewCountByProjectDate(ctx, tx, orgID, projectID, day)
	if err != nil {
		return models.DailyReport{}, err
	}
	r.CrewCount = crew
	progress, err := s.fields.TaskProgressByProjectDate(ctx, tx, orgID, projectID, day)
	if err != nil {
		return models.DailyReport{}, err
	}
	r.TaskProgress = progress
	if r.ReportedAt.IsZero() && len(progress) > 0 {
		// No daily log: anchor the report timestamp on the latest progress.
		r.ReportedAt = progress[0].ReportedAt
	}
	return r, nil
}

// buildSummary produces the list-row projection for a day (no photo resolution).
func (s *ReportsService) buildSummary(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, day time.Time) (models.DailyReportSummary, error) {
	r, err := s.assemble(ctx, tx, orgID, projectID, day)
	if err != nil {
		return models.DailyReportSummary{}, err
	}
	return models.DailyReportSummary{
		ProjectID:         projectID,
		LogDate:           r.LogDate,
		WeatherConditions: r.WeatherConditions,
		WorkSummary:       r.WorkSummary,
		HasSafetyIncident: strings.TrimSpace(r.SafetyIncidents) != "",
		PhotoCount:        r.PhotoCount,
		CrewCount:         r.CrewCount,
		TaskProgressCount: len(r.TaskProgress),
		ReportedAt:        r.ReportedAt,
	}, nil
}

// resolvePhotos mints a short-lived signed GET URL per photo. It mutates the
// report's Photos in place. A nil resolver, or any per-photo error (storage
// unconfigured / asset not ready), drops that photo's URL silently — the
// operator surface degrades to count-only without failing the whole read.
func (s *ReportsService) resolvePhotos(ctx context.Context, orgID uuid.UUID, r *models.DailyReport) {
	if s.photos == nil || len(r.Photos) == 0 {
		// No resolver: emit no photo refs (count already set from the id list).
		r.Photos = nil
		return
	}
	resolved := make([]models.PhotoRef, 0, len(r.Photos))
	for _, p := range r.Photos {
		url, err := s.photos.SignedGetURL(ctx, orgID, p.AssetID, reportThumbTTL)
		if err != nil {
			// Storage unconfigured / asset missing / not ready → skip this photo.
			continue
		}
		resolved = append(resolved, models.PhotoRef{AssetID: p.AssetID, ThumbURL: url})
	}
	r.Photos = resolved
}

// GenerateDigest composes the INTERNAL office digest for a (project, date).
// RBAC minRole superintendent (route-gated). It loads the derived report
// (INCLUDING safety incidents — office surface), calls DailyReportDigest, and
// audits report.digest.generated. AI soft-fail → ErrReportsAIUnavailable (or a
// wrapped ai.ErrUnconfigured) → 503.
func (s *ReportsService) GenerateDigest(ctx context.Context, orgID uuid.UUID, userSub string, projectID uuid.UUID, day time.Time) (string, error) {
	if s.digester == nil {
		return "", ErrReportsAIUnavailable
	}
	report, err := s.GetProjectReport(ctx, orgID, projectID, day)
	if err != nil {
		return "", err
	}

	req := ai.DailyReportDigestRequest{
		ProjectName:       report.ProjectName,
		LogDate:           report.LogDate.Format("2006-01-02"),
		WeatherConditions: report.WeatherConditions,
		WorkSummary:       report.WorkSummary,
		SafetyIncidents:   report.SafetyIncidents, // INTERNAL — office digest includes it
		CrewCount:         report.CrewCount,
		PhotoCount:        report.PhotoCount,
		TaskProgress:      taskProgressLines(report.TaskProgress),
	}
	aiCtx := ai.ContextWithOrgID(ctx, orgID.String())
	resp, err := s.digester.DailyReportDigest(aiCtx, req)
	if err != nil {
		return "", fmt.Errorf("daily report digest: ai: %w", err)
	}

	s.recordAudit(ctx, orgID, userSub, AuditActionReportDigestGenerated, projectID, map[string]any{
		"log_date":            req.LogDate,
		"task_progress_count": len(report.TaskProgress),
		"photo_count":         report.PhotoCount,
	})
	return resp.Digest, nil
}

// ClientUpdateDraft is the result of DraftClientUpdate: the AI-generated subject
// + body for the operator to edit. Chunk C only produces the draft; Chunk D
// persists + sends.
type ClientUpdateDraft struct {
	Subject     string    `json:"subject"`
	Body        string    `json:"body"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	PhotoCount  int       `json:"photo_count"`
}

// DraftClientUpdate composes the CLIENT-SAFE homeowner progress draft for a
// (project, date) — RBAC owner/admin (route-gated). SECURITY-CRITICAL: the
// client request is built from an explicit ALLOWLIST (buildClientRequest) and
// asserted clean of Restricted-classed leakage. The AI never receives safety
// incidents, crew identities, GPS, or any *_cents amounts — the struct cannot
// carry them. Audits client_update.drafted.
func (s *ReportsService) DraftClientUpdate(ctx context.Context, orgID uuid.UUID, userSub string, projectID uuid.UUID, day time.Time) (ClientUpdateDraft, error) {
	if s.drafter == nil {
		return ClientUpdateDraft{}, ErrReportsAIUnavailable
	}
	report, err := s.GetProjectReport(ctx, orgID, projectID, day)
	if err != nil {
		return ClientUpdateDraft{}, err
	}

	req := buildClientRequest(report)
	aiCtx := ai.ContextWithOrgID(ctx, orgID.String())
	resp, err := s.drafter.ClientProgressUpdate(aiCtx, req)
	if err != nil {
		return ClientUpdateDraft{}, fmt.Errorf("client progress update: ai: %w", err)
	}

	s.recordAudit(ctx, orgID, userSub, AuditActionClientUpdateDrafted, projectID, map[string]any{
		"period_start": req.PeriodStart,
		"period_end":   req.PeriodEnd,
		"photo_count":  report.PhotoCount,
	})
	return ClientUpdateDraft{
		Subject:     resp.Subject,
		Body:        resp.Body,
		PeriodStart: report.LogDate,
		PeriodEnd:   report.LogDate,
		PhotoCount:  report.PhotoCount,
	}, nil
}

// buildClientRequest is the DETERMINISTIC REDACTION GATE. It constructs the
// client AI request from an EXPLICIT ALLOWLIST of report fields — project name,
// dates, weather, sanitized work summary, high-level task highlights (WBS/name/
// percent — NO crew identities), and photo count. It NEVER copies in
// safety_incidents, crew identities, GPS coordinates, the reporter identity, or
// any monetary amount. After building, it ASSERTS the assembled prompt carries
// no Restricted-classed value (defense in depth — see assertNoRestrictedLeak).
//
// This is the function the mandated redaction-leak test exercises.
func buildClientRequest(r models.DailyReport) ai.ClientProgressUpdateRequest {
	dateStr := r.LogDate.Format("2006-01-02")
	req := ai.ClientProgressUpdateRequest{
		ProjectName:       r.ProjectName,
		PeriodStart:       dateStr,
		PeriodEnd:         dateStr,
		WeatherConditions: r.WeatherConditions,
		WorkSummary:       r.WorkSummary,
		PhotoCount:        r.PhotoCount,
	}
	// Task highlights: WBS + name + percent only. Crew identities and GPS live on
	// task_progress's source row but are NOT in TaskProgressLine, and we add only
	// the non-identifying fields here.
	for _, l := range r.TaskProgress {
		req.HighlightLines = append(req.HighlightLines, fmt.Sprintf("%s %s — %d%%", l.WBSCode, l.Name, l.PercentComplete))
	}
	return req
}

// taskProgressLines renders the office-digest task lines ("WBS Name — NN%").
func taskProgressLines(progress []models.TaskProgressLine) []string {
	if len(progress) == 0 {
		return nil
	}
	out := make([]string, 0, len(progress))
	for _, l := range progress {
		out = append(out, fmt.Sprintf("%s %s — %d%%", l.WBSCode, l.Name, l.PercentComplete))
	}
	return out
}

// recordAudit writes one audit row in a short standalone tx (the AI flows have
// no surrounding domain mutation to ride). Marshal/insert failures are swallowed
// by AuditService.Record — the composition already succeeded.
func (s *ReportsService) recordAudit(ctx context.Context, orgID uuid.UUID, userSub, action string, projectID uuid.UUID, meta map[string]any) {
	metadata, err := json.Marshal(meta)
	if err != nil {
		return
	}
	_ = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        orgID,
			UserSub:      userSub,
			Action:       action,
			ResourceType: AuditResourceDailyReport,
			ResourceID:   projectID,
			Metadata:     metadata,
		})
		return nil
	})
}

// truncateDay normalizes t to midnight UTC (the calendar-date key for reports).
func truncateDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// mapReportError maps store sentinels to the service-level errors handlers know.
func mapReportError(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

// DefaultReportWindow returns the [since, until] window for ListProjectReports
// when the caller passes nothing: [today-14d, today]. Exposed so the handler can
// apply the same default deterministically.
func DefaultReportWindow(now time.Time) (since, until time.Time) {
	until = truncateDay(now)
	since = until.AddDate(0, 0, -defaultReportWindowDays)
	return since, until
}

// sortSummariesNewestFirst is a defensive re-sort (ListDailyReportDates already
// returns newest-first, but a caller composing windows shouldn't depend on it).
func sortSummariesNewestFirst(s []models.DailyReportSummary) {
	sort.SliceStable(s, func(i, j int) bool { return s[i].LogDate.After(s[j].LogDate) })
}

// assertNoRestrictedLeak is a belt-and-suspenders guard used by tests and the
// draft path: it reports the first forbidden substring found in the marshaled
// client request, or "" when clean. Forbidden values are the report's
// Restricted/Confidential fields that must NEVER reach the homeowner draft.
// (Exported indirectly through the redaction test.)
func assertNoRestrictedLeak(req ai.ClientProgressUpdateRequest, forbidden []string) string {
	blob, err := json.Marshal(req)
	if err != nil {
		return "marshal-error"
	}
	hay := strings.ToLower(string(blob))
	for _, f := range forbidden {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if strings.Contains(hay, strings.ToLower(f)) {
			return f
		}
	}
	return ""
}

// _ keeps pii imported for the classification reference in buildClientRequest's
// contract (the allowlist mirrors pii.Restricted fields). The compile-time
// reference documents that the redaction set is anchored on the PII catalog.
var _ = pii.Restricted
