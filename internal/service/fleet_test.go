package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Validation gates only. Pool/store nil — passing post-validation
// would panic, which is what proves the gates work.

func newFleetSvcForValidationTests() *FleetService {
	return NewFleetService(nil, nil)
}

func TestFleetService_ListAssets_RejectsBadInput(t *testing.T) {
	svc := newFleetSvcForValidationTests()
	if _, err := svc.ListAssets(context.Background(), uuid.Nil, nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.ListAssets(context.Background(), uuid.New(), []string{"bogus"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("bad status: err = %v, want ErrInvalidInput", err)
	}
}

func TestFleetService_CreateAsset_RejectsBadInput(t *testing.T) {
	svc := newFleetSvcForValidationTests()
	cases := []struct {
		name string
		org  uuid.UUID
		in   CreateAssetInput
	}{
		{"nil org", uuid.Nil, CreateAssetInput{Name: "x", AssetType: "y"}},
		{"empty name", uuid.New(), CreateAssetInput{Name: "  ", AssetType: "y"}},
		{"empty type", uuid.New(), CreateAssetInput{Name: "x", AssetType: ""}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.CreateAsset(context.Background(), c.org, c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestFleetService_AllocateAsset_RejectsBadInput(t *testing.T) {
	svc := newFleetSvcForValidationTests()
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	later := now.AddDate(0, 0, 7)

	cases := []struct {
		name string
		org  uuid.UUID
		in   AllocateAssetInput
	}{
		{"nil org", uuid.Nil, AllocateAssetInput{AssetID: uuid.New(), ProjectID: uuid.New(), StartDate: now, EndDate: later}},
		{"nil asset", uuid.New(), AllocateAssetInput{ProjectID: uuid.New(), StartDate: now, EndDate: later}},
		{"nil project", uuid.New(), AllocateAssetInput{AssetID: uuid.New(), StartDate: now, EndDate: later}},
		{"zero start", uuid.New(), AllocateAssetInput{AssetID: uuid.New(), ProjectID: uuid.New(), EndDate: later}},
		{"zero end", uuid.New(), AllocateAssetInput{AssetID: uuid.New(), ProjectID: uuid.New(), StartDate: now}},
		{"end == start", uuid.New(), AllocateAssetInput{AssetID: uuid.New(), ProjectID: uuid.New(), StartDate: now, EndDate: now}},
		{"end < start", uuid.New(), AllocateAssetInput{AssetID: uuid.New(), ProjectID: uuid.New(), StartDate: later, EndDate: now}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.AllocateAsset(context.Background(), c.org, c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}
