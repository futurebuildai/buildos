package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/pii"
	"github.com/futurebuildai/buildos/internal/store"
)

// ---- fakes -------------------------------------------------------------

// fakeDrafter records the call and returns a canned draft (or an error).
type fakeDrafter struct {
	called bool
	resp   ClientUpdateDraft
	err    error
}

func (f *fakeDrafter) DraftClientUpdate(_ context.Context, _ uuid.UUID, _ string, _ uuid.UUID, _ time.Time) (ClientUpdateDraft, error) {
	f.called = true
	if f.err != nil {
		return ClientUpdateDraft{}, f.err
	}
	return f.resp, nil
}

// fakePhotos satisfies ClientUpdatePhotoValidator. signedErr forces every
// SignedGetURL to fail (storage-error degrade); validateErr forces validation
// to reject (non-ready/foreign photo).
type fakePhotos struct {
	signedErr   error
	validateErr error
}

func (f *fakePhotos) ValidatePhotoAssets(_ context.Context, _ pgx.Tx, _, _ uuid.UUID, _ []uuid.UUID) error {
	return f.validateErr
}
func (f *fakePhotos) SignedGetURL(_ context.Context, _, assetID uuid.UUID, _ time.Duration) (string, error) {
	if f.signedErr != nil {
		return "", f.signedErr
	}
	return "https://signed.example/" + assetID.String(), nil
}

// ---- AI soft-fail: drafter nil → 503 sentinel, no row written ----------

func TestCreateDraft_AIUnconfigured(t *testing.T) {
	// drafter nil → ErrClientUpdateAIUnavailable BEFORE any DB call (pool nil is
	// never dereferenced because the guard returns first).
	svc := NewClientUpdateService(nil, store.NewClientUpdateStore(), nil, nil, nil, nil, nil, nil)
	_, err := svc.CreateDraft(context.Background(), uuid.New(), uuid.New().String(), CreateDraftInput{
		ProjectID:  uuid.New(),
		ReportDate: time.Now(),
	})
	if !errors.Is(err, ErrClientUpdateAIUnavailable) {
		t.Fatalf("err = %v, want ErrClientUpdateAIUnavailable", err)
	}
}

// TestCreateDraft_ReportsAIUnavailable_Maps asserts the Chunk C 503 sentinel
// (ErrReportsAIUnavailable) is normalized to the client-update 503 sentinel.
func TestCreateDraft_ReportsAIUnavailable_Maps(t *testing.T) {
	drafter := &fakeDrafter{err: ErrReportsAIUnavailable}
	svc := NewClientUpdateService(nil, store.NewClientUpdateStore(), nil, nil, drafter, nil, nil, nil)
	_, err := svc.CreateDraft(context.Background(), uuid.New(), uuid.New().String(), CreateDraftInput{
		ProjectID:  uuid.New(),
		ReportDate: time.Now(),
	})
	if !errors.Is(err, ErrClientUpdateAIUnavailable) {
		t.Fatalf("err = %v, want ErrClientUpdateAIUnavailable", err)
	}
	if !drafter.called {
		t.Error("drafter should have been called")
	}
}

// ---- input validation --------------------------------------------------

func TestCreateDraft_RequiresIDs(t *testing.T) {
	drafter := &fakeDrafter{resp: ClientUpdateDraft{Subject: "s", Body: "b"}}
	svc := NewClientUpdateService(nil, store.NewClientUpdateStore(), nil, nil, drafter, nil, nil, nil)
	_, err := svc.CreateDraft(context.Background(), uuid.Nil, "sub", CreateDraftInput{ProjectID: uuid.New(), ReportDate: time.Now()})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestCreateDraft_RequiresDate(t *testing.T) {
	drafter := &fakeDrafter{resp: ClientUpdateDraft{Subject: "s", Body: "b"}}
	svc := NewClientUpdateService(nil, store.NewClientUpdateStore(), nil, nil, drafter, nil, nil, nil)
	_, err := svc.CreateDraft(context.Background(), uuid.New(), "sub", CreateDraftInput{ProjectID: uuid.New(), ReportDate: time.Time{}})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestUpdateDraft_RequiresIDs(t *testing.T) {
	svc := NewClientUpdateService(nil, store.NewClientUpdateStore(), nil, nil, nil, nil, nil, nil)
	_, err := svc.UpdateDraft(context.Background(), uuid.New(), "sub", UpdateDraftInput{ID: uuid.Nil})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

func TestSendClientUpdate_RequiresIDs(t *testing.T) {
	svc := NewClientUpdateService(nil, store.NewClientUpdateStore(), nil, nil, nil, nil, nil, nil)
	_, err := svc.SendClientUpdate(context.Background(), uuid.New(), "sub", uuid.Nil)
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
}

// ---- composeEmail: operator body + photo degrade -----------------------

// TestComposeEmail_OperatorBodyControlled asserts the email carries the
// OPERATOR-EDITED body verbatim (escaped in HTML), and renders to the snapshot
// recipient — not the AI draft. The body is operator-controlled from edit on.
func TestComposeEmail_OperatorBodyControlled(t *testing.T) {
	svc := NewClientUpdateService(nil, nil, nil, nil, nil, &fakePhotos{}, nil, nil)
	cu := models.ClientUpdate{
		Subject:    "Your kitchen is coming along",
		EditedBody: "Hi there,\n\nWe finished the cabinets today.\n\nBest,\nYour builder",
	}
	msg := svc.composeEmail(context.Background(), uuid.New(), cu, models.Project{Name: "Maple"}, "home@owner.example")

	if msg.To != "home@owner.example" {
		t.Errorf("To = %q", msg.To)
	}
	if msg.Subject != "Your kitchen is coming along" {
		t.Errorf("Subject = %q", msg.Subject)
	}
	if !strings.Contains(msg.TextBody, "We finished the cabinets today.") {
		t.Errorf("text body missing operator content: %q", msg.TextBody)
	}
	if !strings.Contains(msg.HTMLBody, "We finished the cabinets today.") {
		t.Errorf("html body missing operator content: %q", msg.HTMLBody)
	}
}

// TestComposeEmail_SubjectFallback covers the empty-subject fallback.
func TestComposeEmail_SubjectFallback(t *testing.T) {
	svc := NewClientUpdateService(nil, nil, nil, nil, nil, nil, nil, nil)
	cu := models.ClientUpdate{EditedBody: "body"}
	msg := svc.composeEmail(context.Background(), uuid.New(), cu, models.Project{Name: "Maple Duplex"}, "h@o.example")
	if !strings.Contains(msg.Subject, "Maple Duplex") {
		t.Errorf("subject fallback should include project name: %q", msg.Subject)
	}
}

// TestComposeEmail_PhotoLinksAndDegrade asserts (a) curated photos resolve to
// signed links in both bodies when storage works, and (b) the email degrades
// to text-only when a SignedGetURL fails (storage error) — the send is not
// blocked by photo resolution.
func TestComposeEmail_PhotoLinksAndDegrade(t *testing.T) {
	a1, a2 := uuid.New(), uuid.New()
	cu := models.ClientUpdate{EditedBody: "progress!", PhotoAssetIDs: []uuid.UUID{a1, a2}}

	t.Run("photos included", func(t *testing.T) {
		svc := NewClientUpdateService(nil, nil, nil, nil, nil, &fakePhotos{}, nil, nil)
		msg := svc.composeEmail(context.Background(), uuid.New(), cu, models.Project{Name: "P"}, "h@o.example")
		if !strings.Contains(msg.TextBody, a1.String()) || !strings.Contains(msg.TextBody, a2.String()) {
			t.Errorf("text body missing photo links: %q", msg.TextBody)
		}
		if !strings.Contains(msg.HTMLBody, "<img") {
			t.Errorf("html body missing <img>: %q", msg.HTMLBody)
		}
	})

	t.Run("degrade to text-only on storage error", func(t *testing.T) {
		svc := NewClientUpdateService(nil, nil, nil, nil, nil, &fakePhotos{signedErr: errors.New("storage down")}, nil, nil)
		msg := svc.composeEmail(context.Background(), uuid.New(), cu, models.Project{Name: "P"}, "h@o.example")
		if strings.Contains(msg.HTMLBody, "<img") {
			t.Errorf("html should have no <img> when storage errors: %q", msg.HTMLBody)
		}
		if !strings.Contains(msg.TextBody, "progress!") {
			t.Errorf("text body should still carry operator prose: %q", msg.TextBody)
		}
	})

	t.Run("nil resolver (storage off) → no photos", func(t *testing.T) {
		svc := NewClientUpdateService(nil, nil, nil, nil, nil, nil, nil, nil)
		msg := svc.composeEmail(context.Background(), uuid.New(), cu, models.Project{Name: "P"}, "h@o.example")
		if strings.Contains(msg.HTMLBody, "<img") {
			t.Errorf("no <img> expected with nil resolver: %q", msg.HTMLBody)
		}
	})
}

// ---- recipient_email is never serialized -------------------------------

// TestClientUpdate_RecipientEmailNotSerialized proves the model's json:"-" tag
// keeps the Restricted recipient address out of every API response.
func TestClientUpdate_RecipientEmailNotSerialized(t *testing.T) {
	addr := "home@owner.example"
	cu := models.ClientUpdate{
		ID:             uuid.New(),
		Status:         models.ClientUpdateStatusSent,
		RecipientEmail: &addr,
	}
	blob, err := json.Marshal(cu)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(blob), addr) {
		t.Fatalf("recipient_email leaked into JSON: %s", blob)
	}
	if strings.Contains(strings.ToLower(string(blob)), "recipient_email") {
		t.Fatalf("recipient_email key present in JSON: %s", blob)
	}
}

// ---- PII catalog: the new contact fields are Restricted ----------------

func TestPII_ClientContactFieldsRestricted(t *testing.T) {
	for _, f := range []string{"client_email", "client_name", "client_phone", "recipient_email"} {
		if got := pii.ClassFor(f); got != pii.Restricted {
			t.Errorf("pii.ClassFor(%q) = %v, want Restricted", f, got)
		}
	}
}

// TestPII_ScrubMap_RedactsClientUpdateMetadata asserts a nested client_update
// metadata blob has its recipient_email/client_email redacted by ScrubMap at
// the Restricted threshold.
func TestPII_ScrubMap_RedactsClientUpdateMetadata(t *testing.T) {
	m := map[string]any{
		"client_update": map[string]any{
			"id":              uuid.New().String(),
			"recipient_email": "home@owner.example",
			"client_email":    "home@owner.example",
		},
	}
	scrubbed := pii.ScrubMap(m, pii.Restricted)
	blob, _ := json.Marshal(scrubbed)
	if strings.Contains(string(blob), "home@owner.example") {
		t.Fatalf("ScrubMap left a Restricted email in place: %s", blob)
	}
}
