package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/models"
	"github.com/futurebuildai/buildos/internal/store"
)

// Sentinel errors specific to HRService. ErrInvalidInput is reused.
var (
	// ErrEmployeeNotFound surfaces when a per-employee lookup fails
	// the org-scope check. Mirrors the store-level sentinel.
	ErrEmployeeNotFound = errors.New("hr: employee not found")
)

// HRService provides read access to employees + certifications.
// Phase A7 is read-only — the API contract specifies List endpoints
// only; CRUD on employees lands in a later sprint when frontend
// admin surfaces are built.
type HRService struct {
	pool  *pgxpool.Pool
	store *store.HRStore
}

// NewHRService creates a service bound to a pool + store.
func NewHRService(pool *pgxpool.Pool, hr *store.HRStore) *HRService {
	return &HRService{pool: pool, store: hr}
}

// ListEmployees returns every employee for the caller's org, ordered
// by last name then first name (directory order).
func (s *HRService) ListEmployees(ctx context.Context, callerOrgID uuid.UUID) ([]models.Employee, error) {
	if callerOrgID == uuid.Nil {
		return nil, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}

	var employees []models.Employee
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		got, err := s.store.ListEmployees(ctx, tx, callerOrgID)
		if err != nil {
			return err
		}
		employees = got
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list employees: %w", err)
	}
	return employees, nil
}

// ListCertifications returns every certification for an employee,
// scoped to the caller's org. Cross-org access surfaces as
// ErrEmployeeNotFound (we never leak existence across tenants).
func (s *HRService) ListCertifications(ctx context.Context, callerOrgID, employeeID uuid.UUID) ([]models.Certification, error) {
	if callerOrgID == uuid.Nil || employeeID == uuid.Nil {
		return nil, fmt.Errorf("%w: caller org_id and employee_id are required", ErrInvalidInput)
	}

	var certs []models.Certification
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if err := s.store.VerifyEmployeeInOrg(ctx, tx, employeeID, callerOrgID); err != nil {
			return err
		}
		got, err := s.store.ListCertifications(ctx, tx, employeeID)
		if err != nil {
			return err
		}
		certs = got
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrEmployeeNotFound) {
			return nil, ErrEmployeeNotFound
		}
		return nil, fmt.Errorf("list certifications: %w", err)
	}
	return certs, nil
}
