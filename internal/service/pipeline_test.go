package service

import (
	"context"
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
