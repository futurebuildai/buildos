//go:build integration

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// shareLinkFixture builds a fully-wired ShareLinkService + the seeded data the
// public-resolution tests need: an org, a user, a project, a SENT client update
// carrying a curated photo (and ERP-laden side data on the project/update that
// must NOT leak), plus a second, NON-curated ready asset in the same project.
type shareLinkFixture struct {
	svc            *ShareLinkService
	pool           *pgxpool.Pool
	orgID          uuid.UUID
	projID         uuid.UUID
	userID         uuid.UUID
	clientUpdateID uuid.UUID
	curatedAsset   uuid.UUID
	otherAsset     uuid.UUID // ready, same project, NOT curated → must be unreachable
}

func newShareLinkFixture(t *testing.T) shareLinkFixture {
	t.Helper()
	pool := testdb.NewPool(t)
	ctx := context.Background()

	orgID := uuid.New()
	userID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme Builders")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projID, orgID, "Maple Street Residence")

	// Pin a Restricted client contact + a full street address on the project —
	// these must NEVER reach the public page.
	if _, err := pool.Exec(ctx, `
		UPDATE projects SET client_name=$2, client_email=$3, client_phone=$4, address=$5 WHERE id=$1`,
		projID, "Jane Homeowner", "jane@homeowner.example", "+1-555-867-5309",
		"742 Evergreen Terrace, Springfield IL"); err != nil {
		t.Fatalf("set project client contact: %v", err)
	}

	assetStore := store.NewAssetStore()
	cuStore := store.NewClientUpdateStore()

	// Two ready assets in the same project: one curated into the update, one not.
	var curated, other uuid.UUID
	if err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		a1, err := assetStore.Create(ctx, tx, store.InsertAssetParams{
			OrgID: orgID, ProjectID: &projID, StorageKey: "org/" + orgID.String() + "/project/" + projID.String() + "/curated.jpg",
			ContentType: "image/jpeg", SizeBytes: 1024, UploadedBy: userID.String(),
		})
		if err != nil {
			return err
		}
		if _, err := assetStore.MarkReady(ctx, tx, orgID, a1.ID, nil); err != nil {
			return err
		}
		curated = a1.ID
		a2, err := assetStore.Create(ctx, tx, store.InsertAssetParams{
			OrgID: orgID, ProjectID: &projID, StorageKey: "org/" + orgID.String() + "/project/" + projID.String() + "/other.jpg",
			ContentType: "image/jpeg", SizeBytes: 1024, UploadedBy: userID.String(),
		})
		if err != nil {
			return err
		}
		if _, err := assetStore.MarkReady(ctx, tx, orgID, a2.ID, nil); err != nil {
			return err
		}
		other = a2.ID
		return nil
	}); err != nil {
		t.Fatalf("seed assets: %v", err)
	}

	// Create a 'sent' client update. edited_body is the client-safe prose; ai_draft
	// + recipient_email carry data that must NOT leak.
	period := time.Date(2026, 6, 9, 0, 0, 0, 0, time.UTC)
	clientSafeBody := "We framed the second floor this week and the roof goes on next."
	aiDraftWithLeak := "INTERNAL: safety_incident worker fell from scaffold; crew Bob Smith, Joe Doe; cost overrun $48250"
	var cuID uuid.UUID
	if err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		c, err := cuStore.Create(ctx, tx, store.CreateClientUpdateParams{
			OrgID: orgID, ProjectID: projID, PeriodStart: period, PeriodEnd: period,
			AIDraft: &aiDraftWithLeak, EditedBody: clientSafeBody, Subject: "Weekly progress",
			PhotoAssetIDs: []uuid.UUID{curated}, CreatedBy: userID,
		})
		if err != nil {
			return err
		}
		cuID = c.ID
		if _, err := cuStore.MarkSent(ctx, tx, store.MarkSentParams{
			OrgID: orgID, ID: cuID, RecipientEmail: "jane@homeowner.example", SentBy: userID,
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("seed client update: %v", err)
	}

	svc := NewShareLinkService(
		pool, store.NewShareLinkStore(), cuStore, store.NewProjectStore(),
		store.NewFieldStore(), nil, nil,
	)
	return shareLinkFixture{
		svc: svc, pool: pool, orgID: orgID, projID: projID, userID: userID,
		clientUpdateID: cuID, curatedAsset: curated, otherAsset: other,
	}
}

// TestShareLink_Redaction_PublicUpdateCarriesNoERP is the HEADLINE security test:
// the resolved PublicUpdate (and thus the public page) carries ONLY the curated,
// client-safe fields — none of the raw ERP (safety incidents, crew identities,
// cents/budget, recipient_email, full street address, AI draft, sibling data).
func TestShareLink_Redaction_PublicUpdateCarriesNoERP(t *testing.T) {
	f := newShareLinkFixture(t)
	ctx := context.Background()

	issued, err := f.svc.CreateShareLink(ctx, f.orgID, f.userID.String(), f.clientUpdateID, 0)
	if err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}

	pub, err := f.svc.ResolvePublicUpdate(ctx, issued.Cleartext)
	if err != nil {
		t.Fatalf("ResolvePublicUpdate: %v", err)
	}

	// The allowlisted fields are present and correct.
	if pub.ProjectName != "Maple Street Residence" {
		t.Errorf("project name = %q", pub.ProjectName)
	}
	if !strings.Contains(pub.Body, "framed the second floor") {
		t.Errorf("body missing the client-safe prose: %q", pub.Body)
	}
	if len(pub.PhotoAssetIDs) != 1 || pub.PhotoAssetIDs[0] != f.curatedAsset {
		t.Errorf("curated photos = %v, want [%s]", pub.PhotoAssetIDs, f.curatedAsset)
	}

	// Concatenate every string the projection could possibly expose, then assert
	// NONE of the forbidden raw-ERP values appear. (The struct physically has no
	// field for them, but this is the belt-and-suspenders leak grep the spec
	// mandates.)
	haystack := strings.ToLower(pub.ProjectName + " " + pub.Body)
	forbidden := []string{
		"safety_incident", "scaffold", // safety / liability
		"bob smith", "joe doe", // crew identities
		"48250", "$", "cost overrun", // money
		"jane@homeowner.example", "jane homeowner", "555-867-5309", // recipient / contact PII
		"742 evergreen", "springfield", // full street address (§9-7: city/region only, here neither)
		"internal:", // the AI draft marker
	}
	for _, bad := range forbidden {
		if strings.Contains(haystack, strings.ToLower(bad)) {
			t.Errorf("FORBIDDEN ERP value %q leaked into the public projection: %q", bad, haystack)
		}
	}
}

// TestShareLink_Photo_OnlyCuratedReachable proves a leaked token grants ONLY the
// curated photos of that one update — a real, ready asset in the SAME project that
// was NOT curated resolves to the uniform invalid-token error.
func TestShareLink_Photo_OnlyCuratedReachable(t *testing.T) {
	f := newShareLinkFixture(t)
	ctx := context.Background()

	issued, err := f.svc.CreateShareLink(ctx, f.orgID, f.userID.String(), f.clientUpdateID, 0)
	if err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}

	// Curated asset resolves.
	target, err := f.svc.ResolvePublicPhoto(ctx, issued.Cleartext, f.curatedAsset)
	if err != nil {
		t.Fatalf("curated photo resolve = %v, want ok", err)
	}
	if target.OrgID != f.orgID || target.AssetID != f.curatedAsset {
		t.Errorf("resolved target = %+v", target)
	}

	// A ready, same-project, NON-curated asset is NOT reachable via this token.
	if _, err := f.svc.ResolvePublicPhoto(ctx, issued.Cleartext, f.otherAsset); !errors.Is(err, ErrInvalidShareToken) {
		t.Errorf("non-curated photo resolve = %v, want ErrInvalidShareToken", err)
	}

	// A random asset id is not reachable.
	if _, err := f.svc.ResolvePublicPhoto(ctx, issued.Cleartext, uuid.New()); !errors.Is(err, ErrInvalidShareToken) {
		t.Errorf("random photo resolve = %v, want ErrInvalidShareToken", err)
	}
}

// TestShareLink_Token_UniformNotFound proves wrong/expired/revoked/malformed
// tokens ALL resolve to the same ErrInvalidShareToken (no enumeration signal),
// and that a valid token works.
func TestShareLink_Token_UniformNotFound(t *testing.T) {
	f := newShareLinkFixture(t)
	ctx := context.Background()

	issued, err := f.svc.CreateShareLink(ctx, f.orgID, f.userID.String(), f.clientUpdateID, 0)
	if err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}

	// Valid token works.
	if _, err := f.svc.ResolvePublicUpdate(ctx, issued.Cleartext); err != nil {
		t.Fatalf("valid token = %v, want ok", err)
	}

	// Malformed (wrong length), well-formed-but-unknown, and a flipped char all
	// return the IDENTICAL error.
	bad := []string{
		"",                      // empty
		"too-short",             // malformed length
		strings.Repeat("A", 43), // well-formed shape, unknown token
	}
	// A well-formed-but-wrong token of the right shape: take the real one and flip
	// its first char to a different base64url char.
	flipped := flipFirstChar(issued.Cleartext)
	bad = append(bad, flipped)
	for _, tok := range bad {
		if _, err := f.svc.ResolvePublicUpdate(ctx, tok); !errors.Is(err, ErrInvalidShareToken) {
			t.Errorf("token %q → %v, want ErrInvalidShareToken (uniform)", tok, err)
		}
	}

	// Revoke → the previously-valid token now also returns the uniform error.
	links, err := f.svc.ListShareLinks(ctx, f.orgID, f.clientUpdateID)
	if err != nil || len(links) != 1 {
		t.Fatalf("list links = %v (err %v)", links, err)
	}
	if _, err := f.svc.RevokeShareLink(ctx, f.orgID, f.userID.String(), links[0].ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, err := f.svc.ResolvePublicUpdate(ctx, issued.Cleartext); !errors.Is(err, ErrInvalidShareToken) {
		t.Errorf("revoked token → %v, want ErrInvalidShareToken", err)
	}
}

// TestShareLink_OnlyForSentUpdate proves a link is mintable ONLY on a sent update
// (§9-10): a draft update yields ErrShareLinkNotSent.
func TestShareLink_OnlyForSentUpdate(t *testing.T) {
	pool := testdb.NewPool(t)
	ctx := context.Background()
	orgID := uuid.New()
	userID := uuid.New()
	projID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Org")
	testdb.SeedUser(t, pool, userID, orgID)
	testdb.SeedProject(t, pool, projID, orgID, "P")

	cuStore := store.NewClientUpdateStore()
	period := time.Now().UTC()
	var draftID uuid.UUID
	if err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		c, err := cuStore.Create(ctx, tx, store.CreateClientUpdateParams{
			OrgID: orgID, ProjectID: projID, PeriodStart: period, PeriodEnd: period,
			EditedBody: "draft body", Subject: "s", CreatedBy: userID,
		})
		draftID = c.ID
		return err
	}); err != nil {
		t.Fatalf("seed draft: %v", err)
	}

	svc := NewShareLinkService(pool, store.NewShareLinkStore(), cuStore, store.NewProjectStore(), store.NewFieldStore(), nil, nil)
	if _, err := svc.CreateShareLink(ctx, orgID, userID.String(), draftID, 0); !errors.Is(err, ErrShareLinkNotSent) {
		t.Errorf("link on a draft = %v, want ErrShareLinkNotSent", err)
	}
}

// TestShareLink_CrossOrg_TokenCannotLoadOtherOrg proves a token only ever yields
// its OWN org's data: the resolved update/project are loaded org-scoped to the
// LINK's org (read from the row), never from caller input. (Belt-and-suspenders:
// the store join also enforces it.)
func TestShareLink_CrossOrg_TokenCannotLoadOtherOrg(t *testing.T) {
	f := newShareLinkFixture(t)
	ctx := context.Background()

	issued, err := f.svc.CreateShareLink(ctx, f.orgID, f.userID.String(), f.clientUpdateID, 0)
	if err != nil {
		t.Fatalf("CreateShareLink: %v", err)
	}
	pub, err := f.svc.ResolvePublicUpdate(ctx, issued.Cleartext)
	if err != nil {
		t.Fatalf("ResolvePublicUpdate: %v", err)
	}
	// The resolved project is the link's own org's project, not some sibling.
	if pub.ProjectName != "Maple Street Residence" {
		t.Errorf("resolved cross-org project: %q", pub.ProjectName)
	}
}

// flipFirstChar returns s with its first base64url char swapped to a different
// valid char, producing a well-formed token that hashes differently.
func flipFirstChar(s string) string {
	if s == "" {
		return "A"
	}
	repl := byte('A')
	if s[0] == 'A' {
		repl = 'B'
	}
	return string(repl) + s[1:]
}
