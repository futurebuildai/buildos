package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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

// HRService provides read + write access to employees + certifications.
// Reads (List*) plus the ingress create paths (CreateEmployee /
// CreateCertification) flow through here; every mutation is one tx + an
// audit row.
type HRService struct {
	pool  *pgxpool.Pool
	store *store.HRStore
	audit AuditRecorder
}

// NewHRService creates a service bound to a pool + store + audit recorder.
// audit may be nil; nil falls back to a no-op recorder so partial-wiring
// deployments / unit tests compile without an AuditService.
func NewHRService(pool *pgxpool.Pool, hr *store.HRStore, audit AuditRecorder) *HRService {
	if audit == nil {
		audit = NewNoopAuditRecorder()
	}
	return &HRService{pool: pool, store: hr, audit: audit}
}

// CreateEmployeeInput is the validated input for CreateEmployee. OrgID is
// derived from the caller's claim, never the body. UserID is optional; when
// supplied it is org-verified before insert (a cross-org user_id is rejected
// with ErrInvalidInput, never silently linked).
type CreateEmployeeInput struct {
	OrgID         uuid.UUID
	CallerUserSub string
	FirstName     string
	LastName      string
	Role          string
	Phone         *string
	HireDate      *time.Time
	UserID        *uuid.UUID
}

// CreateEmployee inserts an employee for the caller's org. Validates the
// three required text fields and (if present) the user_id org membership.
// Mirrors FleetService.CreateAsset: one tx, insert, audit on the same tx.
func (s *HRService) CreateEmployee(ctx context.Context, in CreateEmployeeInput) (models.Employee, error) {
	if in.OrgID == uuid.Nil {
		return models.Employee{}, fmt.Errorf("%w: caller org_id is required", ErrInvalidInput)
	}
	first := strings.TrimSpace(in.FirstName)
	last := strings.TrimSpace(in.LastName)
	role := strings.TrimSpace(in.Role)
	if first == "" || last == "" || role == "" {
		return models.Employee{}, fmt.Errorf("%w: first_name, last_name, and role are required", ErrInvalidInput)
	}
	var phone *string
	if in.Phone != nil {
		if t := strings.TrimSpace(*in.Phone); t != "" {
			phone = &t
		}
	}

	var emp models.Employee
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if in.UserID != nil {
			// Org-verify the linked user — never allow a cross-org user_id.
			if err := s.store.VerifyUserInOrg(ctx, tx, *in.UserID, in.OrgID); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return fmt.Errorf("%w: user_id does not belong to this org", ErrInvalidInput)
				}
				return err
			}
		}
		got, err := s.store.CreateEmployee(ctx, tx, store.CreateEmployeeParams{
			OrgID:     in.OrgID,
			FirstName: first,
			LastName:  last,
			Role:      role,
			Phone:     phone,
			HireDate:  in.HireDate,
			UserID:    in.UserID,
		})
		if err != nil {
			return err
		}
		emp = got
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.CallerUserSub,
			Action:       "hr.employee.created",
			ResourceType: AuditResourceEmployee,
			ResourceID:   got.ID,
			After:        marshalAudit(got),
			Metadata: marshalAudit(map[string]any{
				"role": got.Role,
			}),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			return models.Employee{}, err
		}
		// Route through mapStoreError so a future UNIQUE constraint
		// (23505) surfaces as 400, not a leaked 500 — the uniform
		// ingress error contract (matches ImportSchedule / budgets).
		return models.Employee{}, fmt.Errorf("create employee: %w", mapStoreError(err))
	}
	return emp, nil
}

// CreateCertificationInput is the validated input for CreateCertification.
// EmployeeID is org-verified (certifications has no org_id; isolation is
// indirect) BEFORE insert. A cross-org employee surfaces as
// ErrEmployeeNotFound → 404 (never leak existence).
type CreateCertificationInput struct {
	OrgID         uuid.UUID
	CallerUserSub string
	EmployeeID    uuid.UUID
	CertType      string
	CertNumber    *string
	IssuedDate    *time.Time
	ExpiryDate    time.Time
	Status        string
}

// CreateCertification inserts a certification for an employee scoped to the
// caller's org. expiry_date is required (schema NOT NULL). status defaults to
// "active" and is validated against {active, expired, revoked}.
func (s *HRService) CreateCertification(ctx context.Context, in CreateCertificationInput) (models.Certification, error) {
	if in.OrgID == uuid.Nil || in.EmployeeID == uuid.Nil {
		return models.Certification{}, fmt.Errorf("%w: caller org_id and employee_id are required", ErrInvalidInput)
	}
	certType := strings.TrimSpace(in.CertType)
	if certType == "" {
		return models.Certification{}, fmt.Errorf("%w: cert_type is required", ErrInvalidInput)
	}
	if in.ExpiryDate.IsZero() {
		return models.Certification{}, fmt.Errorf("%w: expiry_date is required", ErrInvalidInput)
	}
	status := in.Status
	if status == "" {
		status = models.CertificationStatusActive
	}
	if !models.IsValidCertificationStatus(status) {
		return models.Certification{}, fmt.Errorf("%w: status %q is not one of {active, expired, revoked}", ErrInvalidInput, status)
	}
	var certNumber *string
	if in.CertNumber != nil {
		if t := strings.TrimSpace(*in.CertNumber); t != "" {
			certNumber = &t
		}
	}

	var cert models.Certification
	err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if err := s.store.VerifyEmployeeInOrg(ctx, tx, in.EmployeeID, in.OrgID); err != nil {
			return err
		}
		got, err := s.store.CreateCertification(ctx, tx, store.CreateCertificationParams{
			EmployeeID: in.EmployeeID,
			CertType:   certType,
			CertNumber: certNumber,
			IssuedDate: in.IssuedDate,
			ExpiryDate: in.ExpiryDate,
			Status:     status,
		})
		if err != nil {
			return err
		}
		cert = got
		s.audit.Record(ctx, tx, AuditEntry{
			OrgID:        in.OrgID,
			UserSub:      in.CallerUserSub,
			Action:       "hr.certification.created",
			ResourceType: AuditResourceCertification,
			ResourceID:   got.ID,
			After:        marshalAudit(got),
			Metadata: marshalAudit(map[string]any{
				"employee_id": in.EmployeeID,
				"cert_type":   got.CertType,
			}),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, store.ErrEmployeeNotFound) {
			return models.Certification{}, ErrEmployeeNotFound
		}
		if errors.Is(err, ErrInvalidInput) {
			return models.Certification{}, err
		}
		// Uniform ingress error contract (see CreateEmployee).
		return models.Certification{}, fmt.Errorf("create certification: %w", mapStoreError(err))
	}
	return cert, nil
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
