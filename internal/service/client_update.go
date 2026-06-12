package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/mailer"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Client-update audit resource + action constants. `/audit?action_prefix=
// client_update.` reconstructs a client update's compose→edit→send history.
const (
	AuditActionClientUpdateCreated    = "client_update.created"
	AuditActionClientUpdateEdited     = "client_update.updated"
	AuditActionClientUpdateSent       = "client_update.sent"
	AuditActionClientUpdateSendFailed = "client_update.send_failed"
)

// Client-update sentinel errors. Handlers map these to HTTP status codes:
//
//	ErrClientUpdateAIUnavailable → 503 SERVICE_UNAVAILABLE (no AI key / worker)
//	ErrNoClientContact           → 422 NO_CLIENT_CONTACT (project has no client_email)
//	ErrMailerUnconfigured        → 422 MAILER_UNCONFIGURED (Resend key not set — operator must KNOW)
//	ErrAlreadySent               → 409 ALREADY_SENT (edit/re-send of a sent update)
//	ErrInvalidPhotoAsset         → 400 INVALID_PHOTO_ASSET (curated photo not ready/org/project)
//	ErrNotFound                  → 404 NOT_FOUND (uniform on cross-org)
var (
	// ErrClientUpdateAIUnavailable is returned by DraftClientUpdate when the
	// service has no AI drafter wired (worker binary / no Anthropic key).
	ErrClientUpdateAIUnavailable = errors.New("client_update: ai client not configured")
	// ErrNoClientContact is returned by SendClientUpdate when the project has no
	// client_email — there is no homeowner to send to.
	ErrNoClientContact = errors.New("client_update: project has no client email")
	// ErrMailerUnconfigured re-exports the mailer sentinel at the service
	// boundary so handlers match it with errors.Is without importing internal/
	// mailer. SendClientUpdate records 'failed' + surfaces this (not best-effort).
	ErrMailerUnconfigured = mailer.ErrMailerUnconfigured
	// ErrClientUpdateSendFailed is returned when the mailer rejects the send for
	// a non-unconfigured reason (transport / non-2xx). The row is marked 'failed'
	// with the error recorded; the operator must KNOW it did not go out.
	ErrClientUpdateSendFailed = errors.New("client_update: email send failed")
	// ErrAlreadySent is returned when editing or re-sending a 'sent' update.
	ErrAlreadySent = errors.New("client_update: already sent")
)

// emailPhotoTTL is the freshness of signed GET URLs embedded in the homeowner
// email (D-3.x / §9-9 default: 7 days — long enough that the link survives in
// the recipient's inbox for a reasonable window). The operator-surface preview
// reuses the report's 15-min TTL; only the emailed copy uses the long TTL.
const emailPhotoTTL = 7 * 24 * time.Hour

// ClientUpdateDrafter is the consumer-side slice of ReportsService that the
// composer reuses VERBATIM to produce the AI draft (Chunk C — the redaction
// gate lives there). Declared as an interface so tests inject a fake and the
// composer doesn't pin the whole ReportsService surface.
type ClientUpdateDrafter interface {
	DraftClientUpdate(ctx context.Context, orgID uuid.UUID, userSub string, projectID uuid.UUID, day time.Time) (ClientUpdateDraft, error)
}

// ClientUpdatePhotoValidator validates + resolves curated photo ids. Satisfied
// by *AssetService. nil => photos not validated and not resolved (storage off):
// edits accept any ids (degrade) and the email/preview carry no photo links.
type ClientUpdatePhotoValidator interface {
	ValidatePhotoAssets(ctx context.Context, tx pgx.Tx, orgID, projectID uuid.UUID, ids []uuid.UUID) error
	SignedGetURL(ctx context.Context, orgID, assetID uuid.UUID, ttl time.Duration) (string, error)
}

// clientProjectReader is the narrow slice of ProjectStore the composer needs:
// the project's name (subject fallback) + homeowner client_email (send target).
type clientProjectReader interface {
	GetByID(ctx context.Context, tx pgx.Tx, projectID, orgID uuid.UUID) (models.Project, error)
}

// clientUserResolver resolves a JWT sub → users.id (created_by / sent_by FK).
type clientUserResolver interface {
	LookupUserIDBySubject(ctx context.Context, tx pgx.Tx, subject string, orgID uuid.UUID) (uuid.UUID, error)
}

// ClientUpdateService is the human-in-the-loop client-update composer (Chunk D).
// Flow: DraftClientUpdate (Chunk C AI draft) → persist as 'draft' → operator
// edits subject/body + curates photos → SendClientUpdate marks 'sent' + audits
// in one tx, THEN sends the email post-commit via the existing Resend mailer.
// NEVER auto-sends. A mailer-unconfigured/rejected send marks the row 'failed'
// and surfaces the reason — the operator MUST know it did not go out (the one
// place that diverges from the auth-reset best-effort posture).
//
// drafter may be nil (worker / no AI key) → DraftClientUpdate returns 503.
// photos may be nil (storage off) → edits skip photo validation and the email
// degrades to no photo links. mailer is never nil (NewNoopMailer fallback).
type ClientUpdateService struct {
	pool     *pgxpool.Pool
	store    *store.ClientUpdateStore
	projects clientProjectReader
	users    clientUserResolver
	drafter  ClientUpdateDrafter
	photos   ClientUpdatePhotoValidator
	mailer   mailer.Mailer
	audit    AuditRecorder
}

// NewClientUpdateService wires the composer. drafter/photos may be nil (see
// type doc). A nil mailer is replaced with NewNoopMailer; a nil AuditRecorder
// with the no-op.
func NewClientUpdateService(
	pool *pgxpool.Pool,
	cuStore *store.ClientUpdateStore,
	projects clientProjectReader,
	users clientUserResolver,
	drafter ClientUpdateDrafter,
	photos ClientUpdatePhotoValidator,
	m mailer.Mailer,
	audit AuditRecorder,
) *ClientUpdateService {
	if m == nil {
		m = mailer.NewNoopMailer(nil)
	}
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &ClientUpdateService{
		pool:     pool,
		store:    cuStore,
		projects: projects,
		users:    users,
		drafter:  drafter,
		photos:   photos,
		mailer:   m,
		audit:    audit,
	}
}

// CreateDraftInput is the input for CreateDraft: the project + the report date
// whose AI draft seeds this update.
type CreateDraftInput struct {
	ProjectID  uuid.UUID
	ReportDate time.Time
}

// CreateDraft produces the AI draft for the date (reusing Chunk C's redacted
// DraftClientUpdate verbatim), then persists it as a 'draft' client_update in
// ONE tx + audit client_update.created. The AI is the only thing that can
// soft-fail (503) — when it does, NO empty draft row is created. period_start =
// period_end = the report date (Chunk C drafts a single day today).
//
// SECURITY: the AI draft is built behind the Chunk C deterministic redaction
// allowlist (buildClientRequest) — safety/crew/GPS/cents never reach the model.
// The stored body is the AI text the operator then edits; it is operator-
// controlled from edit onward.
func (s *ClientUpdateService) CreateDraft(ctx context.Context, orgID uuid.UUID, userSub string, in CreateDraftInput) (models.ClientUpdate, error) {
	if orgID == uuid.Nil || in.ProjectID == uuid.Nil {
		return models.ClientUpdate{}, fmt.Errorf("%w: org_id and project_id are required", ErrInvalidInput)
	}
	if in.ReportDate.IsZero() {
		return models.ClientUpdate{}, fmt.Errorf("%w: report date is required", ErrInvalidInput)
	}
	if s.drafter == nil {
		return models.ClientUpdate{}, ErrClientUpdateAIUnavailable
	}

	// AI draft FIRST (outside the persist tx — it is a network call and must not
	// hold a DB tx). A soft-fail here returns before any row is written.
	draft, err := s.drafter.DraftClientUpdate(ctx, orgID, userSub, in.ProjectID, in.ReportDate)
	if err != nil {
		if errors.Is(err, ErrReportsAIUnavailable) {
			return models.ClientUpdate{}, ErrClientUpdateAIUnavailable
		}
		return models.ClientUpdate{}, err
	}

	period := truncateDay(in.ReportDate)
	aiBody := draft.Body

	var out models.ClientUpdate
	err = pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, in.ProjectID, orgID); err != nil {
			return err
		}
		createdBy, err := s.users.LookupUserIDBySubject(ctx, tx, userSub, orgID)
		if err != nil {
			return err
		}
		row, err := s.store.Create(ctx, tx, store.CreateClientUpdateParams{
			OrgID:         orgID,
			ProjectID:     in.ProjectID,
			PeriodStart:   period,
			PeriodEnd:     period,
			AIDraft:       &aiBody,
			EditedBody:    aiBody, // seed the editable body with the AI draft
			Subject:       draft.Subject,
			PhotoAssetIDs: []uuid.UUID{},
			CreatedBy:     createdBy,
		})
		if err != nil {
			return err
		}
		out = row
		s.recordAudit(ctx, tx, orgID, userSub, AuditActionClientUpdateCreated, row.ID, map[string]any{
			"project_id":   in.ProjectID,
			"period_start": period.Format("2006-01-02"),
			"period_end":   period.Format("2006-01-02"),
		})
		return nil
	})
	if err != nil {
		return models.ClientUpdate{}, mapStoreError(err)
	}
	return out, nil
}

// UpdateDraftInput is the operator-edit payload.
type UpdateDraftInput struct {
	ID            uuid.UUID
	Subject       string
	EditedBody    string
	PhotoAssetIDs []uuid.UUID
}

// UpdateDraft applies the operator's edit to a draft (or failed) update.
// Validates the curated photo ids are 'ready'+org+project-matched (the redaction
// control), updates subject/edited_body/photo_asset_ids in ONE tx + audit
// client_update.updated. Editing a 'failed' update resets it to 'draft' (clears
// send_error) so it re-enters the normal lifecycle. A 'sent' update is immutable
// → ErrAlreadySent (409).
func (s *ClientUpdateService) UpdateDraft(ctx context.Context, orgID uuid.UUID, userSub string, in UpdateDraftInput) (models.ClientUpdate, error) {
	if orgID == uuid.Nil || in.ID == uuid.Nil {
		return models.ClientUpdate{}, fmt.Errorf("%w: org_id and id are required", ErrInvalidInput)
	}
	ids := dedupeUUIDs(in.PhotoAssetIDs)

	var out models.ClientUpdate
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		current, err := s.store.GetByID(ctx, tx, orgID, in.ID)
		if err != nil {
			return err
		}
		if current.Status == models.ClientUpdateStatusSent {
			return ErrAlreadySent
		}
		// Validate the curated photos against the update's project (Chunk B
		// guard). Skipped when storage is off (photos nil) — degrade gracefully.
		if s.photos != nil && len(ids) > 0 {
			if err := s.photos.ValidatePhotoAssets(ctx, tx, orgID, current.ProjectID, ids); err != nil {
				return err
			}
		}
		row, err := s.store.UpdateDraft(ctx, tx, store.UpdateDraftParams{
			OrgID:         orgID,
			ID:            in.ID,
			Subject:       in.Subject,
			EditedBody:    in.EditedBody,
			PhotoAssetIDs: ids,
		})
		if err != nil {
			return err
		}
		out = row
		s.recordAudit(ctx, tx, orgID, userSub, AuditActionClientUpdateEdited, row.ID, map[string]any{
			"project_id":  row.ProjectID,
			"photo_count": len(ids),
			"subject_len": len(in.Subject),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrAlreadySent) {
			return models.ClientUpdate{}, ErrAlreadySent
		}
		if errors.Is(err, ErrInvalidPhotoAsset) {
			return models.ClientUpdate{}, ErrInvalidPhotoAsset
		}
		return models.ClientUpdate{}, mapStoreError(err)
	}
	return out, nil
}

// SendClientUpdate is the human-pressed send. It (1) loads the draft + the
// project's homeowner client_email (reject empty → ErrNoClientContact 422),
// (2) in ONE tx snapshots recipient_email + flips status='sent' + sets
// sent_by/sent_at + audits client_update.sent, then (3) AFTER COMMIT sends the
// email via the existing Resend mailer. On a mailer failure (unconfigured /
// rejected) it marks the row 'failed' with the reason and returns the sentinel
// so the operator KNOWS it did not go out — this diverges from the auth-reset
// best-effort posture on purpose. A 'sent' update is not re-sendable →
// ErrAlreadySent (409).
func (s *ClientUpdateService) SendClientUpdate(ctx context.Context, orgID uuid.UUID, userSub string, id uuid.UUID) (models.ClientUpdate, error) {
	if orgID == uuid.Nil || id == uuid.Nil {
		return models.ClientUpdate{}, fmt.Errorf("%w: org_id and id are required", ErrInvalidInput)
	}

	var (
		out       models.ClientUpdate
		recipient string
		project   models.Project
	)
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		current, err := s.store.GetByID(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		if current.Status == models.ClientUpdateStatusSent {
			return ErrAlreadySent
		}
		p, err := s.projects.GetByID(ctx, tx, current.ProjectID, orgID)
		if err != nil {
			return err
		}
		project = p
		if p.ClientEmail == nil || strings.TrimSpace(*p.ClientEmail) == "" {
			return ErrNoClientContact
		}
		recipient = strings.TrimSpace(*p.ClientEmail)

		sentBy, err := s.users.LookupUserIDBySubject(ctx, tx, userSub, orgID)
		if err != nil {
			return err
		}
		row, err := s.store.MarkSent(ctx, tx, store.MarkSentParams{
			OrgID:          orgID,
			ID:             id,
			RecipientEmail: recipient,
			SentBy:         sentBy,
		})
		if err != nil {
			return err
		}
		out = row
		// Audit references the update id + project only — NEVER recipient_email
		// (Restricted). The address is the snapshot on the row, not the log.
		s.recordAudit(ctx, tx, orgID, userSub, AuditActionClientUpdateSent, row.ID, map[string]any{
			"project_id":  row.ProjectID,
			"photo_count": len(row.PhotoAssetIDs),
		})
		return nil
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrAlreadySent):
			return models.ClientUpdate{}, ErrAlreadySent
		case errors.Is(err, ErrNoClientContact):
			return models.ClientUpdate{}, ErrNoClientContact
		default:
			return models.ClientUpdate{}, mapStoreError(err)
		}
	}

	// ---- POST-COMMIT email send -------------------------------------------
	// The status/audit are already durable; now attempt delivery. The email may
	// embed the curated photos as signed GET links (degrade to none when storage
	// is off). A failure here records 'failed' + send_error and surfaces the
	// sentinel — the operator must know.
	msg := s.composeEmail(ctx, orgID, out, project, recipient)
	if sendErr := s.mailer.Send(ctx, orgID.String(), msg); sendErr != nil {
		failed := s.markSendFailed(ctx, orgID, userSub, id, sendErr)
		if errors.Is(sendErr, mailer.ErrMailerUnconfigured) {
			return failed, ErrMailerUnconfigured
		}
		return failed, ErrClientUpdateSendFailed
	}
	return out, nil
}

// markSendFailed records the failed delivery in its own tx (the send tx already
// committed) so the row reflects 'failed'+send_error and an audit row lands.
// Returns the failed row (or the best-effort prior row on a DB hiccup). The
// send_error string never carries the recipient address.
func (s *ClientUpdateService) markSendFailed(ctx context.Context, orgID uuid.UUID, userSub string, id uuid.UUID, sendErr error) models.ClientUpdate {
	reason := "email delivery failed"
	if errors.Is(sendErr, mailer.ErrMailerUnconfigured) {
		reason = "mailer unconfigured: no Resend API key set"
	}
	var out models.ClientUpdate
	if txErr := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		row, err := s.store.MarkFailed(ctx, tx, orgID, id, reason)
		if err != nil {
			return err
		}
		out = row
		s.recordAudit(ctx, tx, orgID, userSub, AuditActionClientUpdateSendFailed, id, map[string]any{
			"project_id": row.ProjectID,
			"reason":     reason,
		})
		return nil
	}); txErr != nil {
		// The send tx already committed status='sent'; this failure-write runs in
		// its own tx. If IT fails (DB hiccup in the narrow post-commit window), the
		// row stays 'sent' while the operator is told the send failed — a real
		// consistency gap. We can't roll back the prior commit, so at minimum make
		// it LOUD (never silent) so it surfaces in logs/triage and can be
		// reconciled. The caller still receives the send-failed sentinel.
		slog.ErrorContext(ctx, "client_update.mark_send_failed.tx_error",
			"client_update_id", id, "reason", reason, "error", txErr)
	}
	return out
}

// ListByProject returns a project's client updates newest-first, org-scoped.
// Read-only, not audited. recipient_email is json:"-" so it never leaves here.
func (s *ClientUpdateService) ListByProject(ctx context.Context, orgID, projectID uuid.UUID) ([]models.ClientUpdate, error) {
	if orgID == uuid.Nil || projectID == uuid.Nil {
		return nil, fmt.Errorf("%w: org_id and project_id are required", ErrInvalidInput)
	}
	var out []models.ClientUpdate
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := store.VerifyProjectInOrg(ctx, tx, projectID, orgID); err != nil {
			return err
		}
		rows, err := s.store.ListByProject(ctx, tx, orgID, projectID)
		if err != nil {
			return err
		}
		out = rows
		return nil
	})
	if err != nil {
		return nil, mapStoreError(err)
	}
	return out, nil
}

// Get returns one client update, org-scoped. Read-only, not audited.
func (s *ClientUpdateService) Get(ctx context.Context, orgID, id uuid.UUID) (models.ClientUpdate, error) {
	if orgID == uuid.Nil || id == uuid.Nil {
		return models.ClientUpdate{}, fmt.Errorf("%w: org_id and id are required", ErrInvalidInput)
	}
	var out models.ClientUpdate
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		row, err := s.store.GetByID(ctx, tx, orgID, id)
		if err != nil {
			return err
		}
		out = row
		return nil
	})
	if err != nil {
		return models.ClientUpdate{}, mapStoreError(err)
	}
	return out, nil
}

// composeEmail renders the homeowner email: the operator-edited subject + body
// (plain text + minimal escaped HTML, mirroring auth.go sendResetEmail), plus
// the curated photos as signed GET links/images. Photos degrade to none when
// storage is off OR any per-photo URL mint fails — the text still goes out.
func (s *ClientUpdateService) composeEmail(ctx context.Context, orgID uuid.UUID, cu models.ClientUpdate, project models.Project, recipient string) mailer.Message {
	subject := strings.TrimSpace(cu.Subject)
	if subject == "" {
		subject = "Progress update on your project"
		if project.Name != "" {
			subject = "Progress update: " + project.Name
		}
	}
	body := cu.EditedBody

	photoURLs := s.resolveEmailPhotos(ctx, orgID, cu.PhotoAssetIDs)

	// Plain text: body, then a "Photos:" list of links.
	var text strings.Builder
	text.WriteString(body)
	if len(photoURLs) > 0 {
		text.WriteString("\n\nPhotos from the site:\n")
		for _, u := range photoURLs {
			text.WriteString(u)
			text.WriteString("\n")
		}
	}

	// HTML: escaped body paragraphs, then inline <img> for each photo.
	var htmlB strings.Builder
	for _, para := range strings.Split(body, "\n\n") {
		p := strings.TrimSpace(para)
		if p == "" {
			continue
		}
		htmlB.WriteString("<p>")
		htmlB.WriteString(strings.ReplaceAll(html.EscapeString(p), "\n", "<br>"))
		htmlB.WriteString("</p>")
	}
	for _, u := range photoURLs {
		htmlB.WriteString(`<p><img src="`)
		htmlB.WriteString(html.EscapeString(u))
		htmlB.WriteString(`" alt="Site photo" style="max-width:100%;height:auto;"></p>`)
	}

	return mailer.Message{
		To:       recipient,
		Subject:  subject,
		TextBody: text.String(),
		HTMLBody: htmlB.String(),
	}
}

// resolveEmailPhotos mints a 7-day signed GET URL per curated photo. A nil
// resolver (storage off) or any per-photo error drops that photo silently —
// the email degrades to text-only rather than failing the send.
func (s *ClientUpdateService) resolveEmailPhotos(ctx context.Context, orgID uuid.UUID, ids []uuid.UUID) []string {
	if s.photos == nil || len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		u, err := s.photos.SignedGetURL(ctx, orgID, id, emailPhotoTTL)
		if err != nil {
			continue
		}
		out = append(out, u)
	}
	return out
}

// recordAudit writes one audit row inside the supplied tx (rides the mutation).
func (s *ClientUpdateService) recordAudit(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, userSub, action string, id uuid.UUID, meta map[string]any) {
	metadata, err := json.Marshal(meta)
	if err != nil {
		return
	}
	s.audit.Record(ctx, tx, AuditEntry{
		OrgID:        orgID,
		UserSub:      userSub,
		Action:       action,
		ResourceType: AuditResourceClientUpdate,
		ResourceID:   id,
		Metadata:     metadata,
	})
}
