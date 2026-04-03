package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

// HRService provides business logic for HR operations.
type HRService struct {
	store *store.HRStore
}

// NewHRService creates a new HRService.
func NewHRService(s *store.HRStore) *HRService {
	return &HRService{store: s}
}

// ListEmployees returns all employees for an org.
func (svc *HRService) ListEmployees(ctx context.Context, orgID uuid.UUID) ([]models.Employee, error) {
	return svc.store.ListEmployees(ctx, orgID)
}

// GetEmployee returns a single employee.
func (svc *HRService) GetEmployee(ctx context.Context, employeeID uuid.UUID) (*models.Employee, error) {
	return svc.store.GetEmployee(ctx, employeeID)
}

// ListCertifications returns certifications for an employee.
func (svc *HRService) ListCertifications(ctx context.Context, employeeID uuid.UUID) ([]models.Certification, error) {
	return svc.store.ListCertifications(ctx, employeeID)
}

// ListExpiringCertifications returns certs expiring within the given days.
func (svc *HRService) ListExpiringCertifications(ctx context.Context, orgID uuid.UUID, withinDays int) ([]models.Certification, error) {
	if withinDays <= 0 {
		withinDays = 30
	}
	return svc.store.ListExpiringCertifications(ctx, orgID, withinDays)
}
