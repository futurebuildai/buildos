package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// Validation gates only — pool/store nil; post-validation paths would
// panic, which proves the gates work.

func newHRSvcForValidationTests() *HRService {
	return NewHRService(nil, nil)
}

func TestHRService_ListEmployees_RejectsNilOrg(t *testing.T) {
	svc := newHRSvcForValidationTests()
	_, err := svc.ListEmployees(context.Background(), uuid.Nil)
	if !errors.Is(err, ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestHRService_ListCertifications_RejectsNilIDs(t *testing.T) {
	svc := newHRSvcForValidationTests()
	if _, err := svc.ListCertifications(context.Background(), uuid.Nil, uuid.New()); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil org: err = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.ListCertifications(context.Background(), uuid.New(), uuid.Nil); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("nil employee: err = %v, want ErrInvalidInput", err)
	}
}
