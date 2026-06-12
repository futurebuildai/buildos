//go:build integration

package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/mailer"
	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// ---- fakes for the integration lifecycle -------------------------------

// stubDrafter returns a fixed AI draft (the Chunk C compose is unit-tested for
// redaction; here we exercise the persist/edit/send lifecycle).
type stubDrafter struct{ subject, body string }

func (s stubDrafter) DraftClientUpdate(_ context.Context, _ uuid.UUID, _ string, _ uuid.UUID, _ time.Time) (ClientUpdateDraft, error) {
	return ClientUpdateDraft{Subject: s.subject, Body: s.body}, nil
}

// recordingMailer captures the last message + lets a test force a failure
// (transport / unconfigured) to drive the failed-send path.
type recordingMailer struct {
	mu    sync.Mutex
	last  *mailer.Message
	calls int
	err   error
}

func (m *recordingMailer) Send(_ context.Context, _ string, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return m.err
	}
	cp := msg
	m.last = &cp
	return nil
}

// cuFixture wires a ClientUpdateService over a fresh pool with a stub drafter,
// the real AssetService as the photo validator, a recording mailer, and a
// capturing audit recorder. Seeds an org + user + project (with client_email).
type cuFixture struct {
	svc     *ClientUpdateService
	asset   *AssetService
	astSt   *store.AssetStore
	mailer  *recordingMailer
	audit   *capturingAuditRecorder
	pool    *pgxpool.Pool
	orgID   uuid.UUID
	subject string
	projID  uuid.UUID
}

func newCUFixture(t *testing.T, clientEmail string) cuFixture {
	t.Helper()
	pool := testdb.NewPool(t)

	astStore := store.NewAssetStore()
	fieldStore := store.NewFieldStore()
	audit := &capturingAuditRecorder{}
	asset := NewAssetService(pool, astStore, fieldStore, nil, audit, nil, nil)
	rec := &recordingMailer{}

	svc := NewClientUpdateService(
		pool, store.NewClientUpdateStore(), store.NewProjectStore(), fieldStore,
		stubDrafter{subject: "AI subject", body: "AI body the operator will edit"},
		asset, rec, audit,
	)

	orgID := uuid.New()
	userID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projID, orgID, "Maple Duplex")
	if clientEmail != "" {
		if _, err := pool.Exec(context.Background(),
			`UPDATE projects SET client_name=$2, client_email=$3 WHERE id=$1`,
			projID, "Homeowner Jane", clientEmail); err != nil {
			t.Fatalf("set client contact: %v", err)
		}
	}

	return cuFixture{
		svc: svc, asset: asset, astSt: astStore, mailer: rec, audit: audit,
		pool: pool, orgID: orgID, subject: userID.String(), projID: projID,
	}
}

func (f cuFixture) seedReadyAsset(t *testing.T, orgID uuid.UUID, projectID *uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := pgx.BeginTxFunc(ctx, f.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		a, err := f.astSt.Create(ctx, tx, store.InsertAssetParams{
			OrgID:       orgID,
			ProjectID:   projectID,
			StorageKey:  "org/" + orgID.String() + "/" + uuid.NewString() + ".jpg",
			ContentType: "image/jpeg",
			SizeBytes:   2048,
			UploadedBy:  f.subject,
		})
		if err != nil {
			return err
		}
		id = a.ID
		_, err = f.astSt.MarkReady(ctx, tx, orgID, a.ID, nil)
		return err
	})
	if err != nil {
		t.Fatalf("seed ready asset: %v", err)
	}
	return id
}

func (f cuFixture) auditActions() []string {
	out := make([]string, 0, len(f.audit.entries))
	for _, e := range f.audit.entries {
		out = append(out, e.Action)
	}
	return out
}

// ---- the full lifecycle: create → edit → send --------------------------

func TestClientUpdate_Lifecycle_CreateEditSend(t *testing.T) {
	f := newCUFixture(t, "home@owner.example")
	ctx := context.Background()
	day := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)

	// CREATE DRAFT (from a date's AI draft).
	draft, err := f.svc.CreateDraft(ctx, f.orgID, f.subject, CreateDraftInput{ProjectID: f.projID, ReportDate: day})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if draft.Status != models.ClientUpdateStatusDraft {
		t.Errorf("status = %q, want draft", draft.Status)
	}
	if draft.EditedBody != "AI body the operator will edit" {
		t.Errorf("draft body should seed from AI: %q", draft.EditedBody)
	}
	if draft.AIDraft == nil || *draft.AIDraft != "AI body the operator will edit" {
		t.Errorf("ai_draft not preserved: %v", draft.AIDraft)
	}

	// EDIT: operator overwrites the body + subject + curates a photo.
	photo := f.seedReadyAsset(t, f.orgID, &f.projID)
	edited, err := f.svc.UpdateDraft(ctx, f.orgID, f.subject, UpdateDraftInput{
		ID:            draft.ID,
		Subject:       "Your home is coming along",
		EditedBody:    "Hi Jane,\n\nThe framing is done and it looks great.\n\n— Your builder",
		PhotoAssetIDs: []uuid.UUID{photo},
	})
	if err != nil {
		t.Fatalf("UpdateDraft: %v", err)
	}
	// The stored body is OPERATOR-controlled (not the AI text) after edit.
	if !strings.Contains(edited.EditedBody, "The framing is done") {
		t.Errorf("edited body not stored: %q", edited.EditedBody)
	}
	if len(edited.PhotoAssetIDs) != 1 || edited.PhotoAssetIDs[0] != photo {
		t.Errorf("curated photo not stored: %v", edited.PhotoAssetIDs)
	}

	// SEND.
	sent, err := f.svc.SendClientUpdate(ctx, f.orgID, f.subject, draft.ID)
	if err != nil {
		t.Fatalf("SendClientUpdate: %v", err)
	}
	if sent.Status != models.ClientUpdateStatusSent {
		t.Errorf("status = %q, want sent", sent.Status)
	}
	if sent.SentAt == nil || sent.SentBy == nil {
		t.Errorf("sent_at/sent_by not set: %+v", sent)
	}
	if sent.RecipientEmail == nil || *sent.RecipientEmail != "home@owner.example" {
		t.Errorf("recipient snapshot wrong: %v", sent.RecipientEmail)
	}

	// The mailer received the OPERATOR body + the homeowner address.
	if f.mailer.calls != 1 || f.mailer.last == nil {
		t.Fatalf("mailer called %d times, last=%v", f.mailer.calls, f.mailer.last)
	}
	if f.mailer.last.To != "home@owner.example" {
		t.Errorf("mailer To = %q", f.mailer.last.To)
	}
	if !strings.Contains(f.mailer.last.TextBody, "The framing is done") {
		t.Errorf("mailer body not the operator text: %q", f.mailer.last.TextBody)
	}

	// AUDIT: created → updated → sent, all in the recorder, none carrying the
	// recipient email.
	actions := f.auditActions()
	wantSeq := []string{AuditActionClientUpdateCreated, AuditActionClientUpdateEdited, AuditActionClientUpdateSent}
	for _, w := range wantSeq {
		found := false
		for _, a := range actions {
			if a == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing audit action %q in %v", w, actions)
		}
	}
	for _, e := range f.audit.entries {
		if strings.Contains(strings.ToLower(string(e.Metadata)), "home@owner.example") {
			t.Errorf("audit metadata leaked recipient_email: %s", e.Metadata)
		}
	}
}

// ---- MAILER_UNCONFIGURED is surfaced, row marked failed ----------------

func TestClientUpdate_Send_MailerUnconfigured(t *testing.T) {
	f := newCUFixture(t, "home@owner.example")
	f.mailer.err = mailer.ErrMailerUnconfigured
	ctx := context.Background()

	draft, err := f.svc.CreateDraft(ctx, f.orgID, f.subject, CreateDraftInput{ProjectID: f.projID, ReportDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	sent, err := f.svc.SendClientUpdate(ctx, f.orgID, f.subject, draft.ID)
	if !errors.Is(err, ErrMailerUnconfigured) {
		t.Fatalf("err = %v, want ErrMailerUnconfigured (operator MUST know)", err)
	}
	// The row is marked 'failed' with a send_error (NOT swallowed, NOT 'sent').
	if sent.Status != models.ClientUpdateStatusFailed {
		t.Errorf("status = %q, want failed", sent.Status)
	}
	if sent.SendError == nil || *sent.SendError == "" {
		t.Errorf("send_error should be set: %v", sent.SendError)
	}
	if sent.SendError != nil && strings.Contains(*sent.SendError, "home@owner.example") {
		t.Errorf("send_error leaked recipient: %q", *sent.SendError)
	}

	// A send_failed audit landed.
	found := false
	for _, a := range f.auditActions() {
		if a == AuditActionClientUpdateSendFailed {
			found = true
		}
	}
	if !found {
		t.Errorf("missing send_failed audit: %v", f.auditActions())
	}
}

// ---- a FAILED update is editable (resets to draft) + re-sendable -------
//
// Regression guard for review finding #9: a failed send was re-sendable but a
// PATCH 404'd (UpdateDraft only matched 'draft'). A failed row is now editable;
// editing resets it to 'draft' and clears send_error, so fix-then-resend flows
// through the normal lifecycle.
func TestClientUpdate_FailedUpdate_IsEditableAndResendable(t *testing.T) {
	f := newCUFixture(t, "home@owner.example")
	f.mailer.err = mailer.ErrMailerUnconfigured
	ctx := context.Background()

	draft, err := f.svc.CreateDraft(ctx, f.orgID, f.subject, CreateDraftInput{ProjectID: f.projID, ReportDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	failed, err := f.svc.SendClientUpdate(ctx, f.orgID, f.subject, draft.ID)
	if !errors.Is(err, ErrMailerUnconfigured) || failed.Status != models.ClientUpdateStatusFailed {
		t.Fatalf("setup: want failed+ErrMailerUnconfigured, got status=%q err=%v", failed.Status, err)
	}

	// Edit the FAILED row → succeeds, resets to 'draft', clears send_error.
	edited, err := f.svc.UpdateDraft(ctx, f.orgID, f.subject, UpdateDraftInput{
		ID: draft.ID, Subject: "Revised subject", EditedBody: "Revised body",
	})
	if err != nil {
		t.Fatalf("UpdateDraft on a failed row = %v, want success (finding #9)", err)
	}
	if edited.Status != models.ClientUpdateStatusDraft {
		t.Errorf("status after editing a failed row = %q, want draft", edited.Status)
	}
	if edited.SendError != nil && *edited.SendError != "" {
		t.Errorf("send_error not cleared on edit: %q", *edited.SendError)
	}
	if edited.Subject != "Revised subject" {
		t.Errorf("edit not applied: %q", edited.Subject)
	}

	// Fix the mailer and re-send → now succeeds.
	f.mailer.err = nil
	sent, err := f.svc.SendClientUpdate(ctx, f.orgID, f.subject, draft.ID)
	if err != nil {
		t.Fatalf("re-send after fix = %v, want success", err)
	}
	if sent.Status != models.ClientUpdateStatusSent {
		t.Errorf("status = %q, want sent", sent.Status)
	}
}

// ---- NO_CLIENT_CONTACT: project without client_email -------------------

func TestClientUpdate_Send_NoClientContact(t *testing.T) {
	f := newCUFixture(t, "") // no client_email
	ctx := context.Background()

	draft, err := f.svc.CreateDraft(ctx, f.orgID, f.subject, CreateDraftInput{ProjectID: f.projID, ReportDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	_, err = f.svc.SendClientUpdate(ctx, f.orgID, f.subject, draft.ID)
	if !errors.Is(err, ErrNoClientContact) {
		t.Fatalf("err = %v, want ErrNoClientContact", err)
	}
	// Mailer NOT called (no recipient).
	if f.mailer.calls != 0 {
		t.Errorf("mailer should not be called without a recipient: %d", f.mailer.calls)
	}
	// Row stays draft (the empty-contact reject rolls the send tx back).
	cur, err := f.svc.Get(ctx, f.orgID, draft.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cur.Status != models.ClientUpdateStatusDraft {
		t.Errorf("status = %q, want draft (send rolled back)", cur.Status)
	}
}

// ---- ALREADY_SENT: edit + re-send of a sent update ---------------------

func TestClientUpdate_AlreadySent(t *testing.T) {
	f := newCUFixture(t, "home@owner.example")
	ctx := context.Background()

	draft, err := f.svc.CreateDraft(ctx, f.orgID, f.subject, CreateDraftInput{ProjectID: f.projID, ReportDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := f.svc.SendClientUpdate(ctx, f.orgID, f.subject, draft.ID); err != nil {
		t.Fatalf("SendClientUpdate: %v", err)
	}

	t.Run("edit after send → ALREADY_SENT", func(t *testing.T) {
		_, err := f.svc.UpdateDraft(ctx, f.orgID, f.subject, UpdateDraftInput{ID: draft.ID, Subject: "x", EditedBody: "y"})
		if !errors.Is(err, ErrAlreadySent) {
			t.Fatalf("err = %v, want ErrAlreadySent", err)
		}
	})
	t.Run("re-send → ALREADY_SENT", func(t *testing.T) {
		_, err := f.svc.SendClientUpdate(ctx, f.orgID, f.subject, draft.ID)
		if !errors.Is(err, ErrAlreadySent) {
			t.Fatalf("err = %v, want ErrAlreadySent", err)
		}
	})
}

// ---- photo curation rejects non-ready / foreign ids --------------------

func TestClientUpdate_EditRejectsBadPhoto(t *testing.T) {
	f := newCUFixture(t, "home@owner.example")
	ctx := context.Background()

	draft, err := f.svc.CreateDraft(ctx, f.orgID, f.subject, CreateDraftInput{ProjectID: f.projID, ReportDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	// Pending (not ready) asset → must be rejected.
	var pending uuid.UUID
	if err := pgx.BeginTxFunc(ctx, f.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		a, err := f.astSt.Create(ctx, tx, store.InsertAssetParams{
			OrgID: f.orgID, ProjectID: &f.projID,
			StorageKey:  "org/" + f.orgID.String() + "/" + uuid.NewString() + ".jpg",
			ContentType: "image/jpeg", SizeBytes: 1, UploadedBy: f.subject,
		})
		pending = a.ID
		return err
	}); err != nil {
		t.Fatalf("seed pending asset: %v", err)
	}

	_, err = f.svc.UpdateDraft(ctx, f.orgID, f.subject, UpdateDraftInput{
		ID: draft.ID, Subject: "s", EditedBody: "b", PhotoAssetIDs: []uuid.UUID{pending},
	})
	if !errors.Is(err, ErrInvalidPhotoAsset) {
		t.Fatalf("err = %v, want ErrInvalidPhotoAsset", err)
	}
}

// ---- cross-org isolation -----------------------------------------------

func TestClientUpdate_CrossOrg404(t *testing.T) {
	f := newCUFixture(t, "home@owner.example")
	ctx := context.Background()
	otherOrg := uuid.New()
	testdb.SeedOrg(t, f.pool, otherOrg, "Other")

	draft, err := f.svc.CreateDraft(ctx, f.orgID, f.subject, CreateDraftInput{ProjectID: f.projID, ReportDate: time.Now()})
	if err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}

	if _, err := f.svc.Get(ctx, otherOrg, draft.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org Get = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.UpdateDraft(ctx, otherOrg, f.subject, UpdateDraftInput{ID: draft.ID, Subject: "x", EditedBody: "y"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org UpdateDraft = %v, want ErrNotFound", err)
	}
	if _, err := f.svc.SendClientUpdate(ctx, otherOrg, f.subject, draft.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-org Send = %v, want ErrNotFound", err)
	}
}

// ---- client-contact backfill on prospect → project conversion ----------

// TestClientUpdate_ContactCarriedFromProspect proves the CreateProjectFromProspect
// leak is closed: a project converted from a prospect carries the prospect's
// client_name/email/phone (so a client update has a recipient).
func TestClientUpdate_ContactCarriedFromProspect(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")

	ps := store.NewPipelineStore()
	email := "prospect@owner.example"
	phone := "555-0100"

	var projectID uuid.UUID
	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		name := "Jane Prospect"
		id, err := ps.CreateProjectFromProspect(ctx, tx, store.CreateProjectFromProspectParams{
			OrgID:            orgID,
			Name:             "Converted Project",
			GSF:              2000,
			PermitIssuedDate: time.Now(),
			ClientName:       &name,
			ClientEmail:      &email,
			ClientPhone:      &phone,
		})
		projectID = id
		return err
	})
	if err != nil {
		t.Fatalf("CreateProjectFromProspect: %v", err)
	}

	var gotName, gotEmail, gotPhone *string
	if err := pool.QueryRow(ctx,
		`SELECT client_name, client_email, client_phone FROM projects WHERE id=$1`, projectID,
	).Scan(&gotName, &gotEmail, &gotPhone); err != nil {
		t.Fatalf("read project: %v", err)
	}
	if gotEmail == nil || *gotEmail != email {
		t.Errorf("client_email = %v, want %q (leak NOT closed)", gotEmail, email)
	}
	if gotName == nil || *gotName != "Jane Prospect" {
		t.Errorf("client_name = %v", gotName)
	}
	if gotPhone == nil || *gotPhone != phone {
		t.Errorf("client_phone = %v", gotPhone)
	}
}
