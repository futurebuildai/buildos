package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/futurebuildai/buildos/internal/models"
)

// HRStore manages employees + certifications.
type HRStore struct{}

// NewHRStore creates a new HRStore.
func NewHRStore() *HRStore { return &HRStore{} }

// ErrEmployeeNotFound is returned when an employee lookup misses (id
// or org_id mismatch). The service maps this to 404.
var ErrEmployeeNotFound = errors.New("employees: not found")

// VerifyEmployeeInOrg returns nil if the employee belongs to the given
// org, ErrEmployeeNotFound otherwise. Service-layer guard for any
// per-employee read so a sibling-org caller can't enumerate cert data.
func (s *HRStore) VerifyEmployeeInOrg(ctx context.Context, tx pgx.Tx, employeeID, orgID uuid.UUID) error {
	var exists bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM employees WHERE id = $1 AND org_id = $2)`,
		employeeID, orgID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("verify employee in org: %w", err)
	}
	if !exists {
		return ErrEmployeeNotFound
	}
	return nil
}

// ListEmployees returns all employees for an org, ordered by
// last_name, first_name (the directory ordering humans expect).
func (s *HRStore) ListEmployees(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) ([]models.Employee, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, org_id, user_id, first_name, last_name, role, phone, hire_date, created_at
		FROM employees
		WHERE org_id = $1
		ORDER BY last_name ASC, first_name ASC, created_at DESC`,
		orgID,
	)
	if err != nil {
		return nil, fmt.Errorf("query employees: %w", err)
	}
	defer rows.Close()

	out := make([]models.Employee, 0)
	for rows.Next() {
		var e models.Employee
		if err := rows.Scan(
			&e.ID, &e.OrgID, &e.UserID,
			&e.FirstName, &e.LastName, &e.Role,
			&e.Phone, &e.HireDate, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan employee: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListCertifications returns every cert for an employee, ordered by
// expiry_date ASC so the soonest-expiring rows surface first. The
// caller is expected to have already verified the employee belongs to
// the caller's org via VerifyEmployeeInOrg.
func (s *HRStore) ListCertifications(ctx context.Context, tx pgx.Tx, employeeID uuid.UUID) ([]models.Certification, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, employee_id, cert_type, cert_number, issued_date, expiry_date, status, created_at
		FROM certifications
		WHERE employee_id = $1
		ORDER BY expiry_date ASC, created_at DESC`,
		employeeID,
	)
	if err != nil {
		return nil, fmt.Errorf("query certifications: %w", err)
	}
	defer rows.Close()

	out := make([]models.Certification, 0)
	for rows.Next() {
		var c models.Certification
		if err := rows.Scan(
			&c.ID, &c.EmployeeID, &c.CertType, &c.CertNumber,
			&c.IssuedDate, &c.ExpiryDate, &c.Status, &c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan certification: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
