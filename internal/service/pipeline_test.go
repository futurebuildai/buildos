package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/models"
)

// These tests exercise the early-return validation paths in
// AdvanceProspect that fire before any DB call. The full transition
// path (project insert + prospect update + River InsertTx) needs a
// real Postgres + River setup and lives in the integration suite
// (deferred until Testcontainers infra lands).

func TestAdvanceProspect_RejectsUnknownTarget(t *testing.T) {
	svc := &PipelineService{} // pool/store/riverClient unused by this path
	_, err := svc.AdvanceProspect(context.Background(), "test-sub", AdvanceProspectInput{
		ProspectID: uuid.New(),
		OrgID:      uuid.New(),
		Target:     models.PipelineStage("BOGUS_STAGE"),
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

// PermitIssuedDate is required for the PERMIT_ISSUED transition. The
// guard runs before any DB call, so a nil pool/store/riverClient is OK
// for this test — the validation short-circuits.
func TestAdvanceProspect_PermitIssuedRequiresDate(t *testing.T) {
	svc := &PipelineService{} // pool/store/riverClient unused by this path
	_, err := svc.AdvanceProspect(context.Background(), "test-sub", AdvanceProspectInput{
		ProspectID:       uuid.New(),
		OrgID:            uuid.New(),
		Target:           models.StagePermitIssued,
		PermitIssuedDate: nil,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput (nil PermitIssuedDate should block transition)", err)
	}
}

// When PermitIssuedDate is supplied but the service was built without a
// RiverClient, the second guard in transitionToPermitIssued returns
// ErrNotImplemented. This ensures partial-wiring deployments fail loud
// rather than silently transitioning a prospect to PERMIT_ISSUED with
// no construction Project.
func TestAdvanceProspect_PermitIssuedRequiresRiverClient(t *testing.T) {
	svc := &PipelineService{} // riverClient nil; pool/store unused
	d := time.Now()
	_, err := svc.AdvanceProspect(context.Background(), "test-sub", AdvanceProspectInput{
		ProspectID:       uuid.New(),
		OrgID:            uuid.New(),
		Target:           models.StagePermitIssued,
		PermitIssuedDate: &d,
	})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("err = %v, want ErrNotImplemented (nil riverClient should block transition)", err)
	}
}

// marshalProspectPromotedPayload contract: must produce a feed-card-shaped
// envelope per ADR-001 D14. Brain consumers (LocalBlue write-back, GableLBM
// portfolio refresh, etc.) deserialize against this shape; drift here breaks
// every downstream consumer at once. Test asserts the explicit shape rather
// than round-tripping the struct so a renamed JSON key fails loud.
func TestMarshalProspectPromotedPayload_ShapeMatchesContract(t *testing.T) {
	prospectID := uuid.New()
	projectID := uuid.New()
	orgID := uuid.New()
	addr := "123 Builder Ln"
	issued := time.Date(2026, 5, 1, 14, 30, 0, 0, time.UTC)

	raw, err := marshalProspectPromotedPayload(prospectPromotedPayload{
		ProspectID:       prospectID,
		ProjectID:        projectID,
		OrgID:            orgID,
		Name:             "Acme Reno",
		Address:          &addr,
		GSF:              2400,
		PermitIssuedDate: issued,
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got["card_type"] != "pipeline.prospect_promoted" {
		t.Errorf("card_type = %q, want pipeline.prospect_promoted", got["card_type"])
	}
	if got["title"] != "Project created: Acme Reno" {
		t.Errorf("title = %q", got["title"])
	}
	if got["body"] != "Permit issued for Acme Reno · 2400 GSF" {
		t.Errorf("body = %q", got["body"])
	}
	if got["priority"] != "normal" {
		t.Errorf("priority = %q, want normal", got["priority"])
	}
	if actions, ok := got["actions"].([]any); !ok || len(actions) != 0 {
		t.Errorf("actions = %v, want []", got["actions"])
	}

	meta, ok := got["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata not an object: %T", got["metadata"])
	}
	if meta["prospect_id"] != prospectID.String() {
		t.Errorf("metadata.prospect_id = %v", meta["prospect_id"])
	}
	if meta["project_id"] != projectID.String() {
		t.Errorf("metadata.project_id = %v", meta["project_id"])
	}
	if meta["org_id"] != orgID.String() {
		t.Errorf("metadata.org_id = %v", meta["org_id"])
	}
	// JSON unmarshals numeric values as float64 by default.
	if gsf, _ := meta["gsf"].(float64); gsf != 2400 {
		t.Errorf("metadata.gsf = %v, want 2400", meta["gsf"])
	}
	if meta["permit_issued_date"] != "2026-05-01T14:30:00Z" {
		t.Errorf("metadata.permit_issued_date = %v", meta["permit_issued_date"])
	}
	if meta["address"] != "123 Builder Ln" {
		t.Errorf("metadata.address = %v", meta["address"])
	}
}

// Address is optional. When nil it must be omitted from metadata
// entirely — Brain treats absent vs empty-string as different signals
// in its dedup/ETL.
func TestMarshalProspectPromotedPayload_OmitsNilAddress(t *testing.T) {
	raw, err := marshalProspectPromotedPayload(prospectPromotedPayload{
		ProspectID:       uuid.New(),
		ProjectID:        uuid.New(),
		OrgID:            uuid.New(),
		Name:             "No-Address Reno",
		Address:          nil,
		GSF:              1800,
		PermitIssuedDate: time.Now(),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	meta, ok := got["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata not an object: %T", got["metadata"])
	}
	if _, present := meta["address"]; present {
		t.Errorf("metadata.address must be absent when input Address is nil; got %v", meta["address"])
	}
}
