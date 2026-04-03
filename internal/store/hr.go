package store

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuild/futurebuild-os/internal/models"
)

// HRStore provides raw SQL access for HR operations.
type HRStore struct {
	pool *pgxpool.Pool
}

// NewHRStore creates a new HRStore.
func NewHRStore(pool *pgxpool.Pool) *HRStore {
	return &HRStore{pool: pool}
}

// ListEmployees returns all employees for an org.
func (s *HRStore) ListEmployees(ctx context.Context, orgID uuid.UUID) ([]models.Employee, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, org_id, user_id, first_name, last_name, role, phone, hire_date, created_at
		FROM employees WHERE org_id = $1
		ORDER BY last_name, first_name`, orgID)
	if err != nil {
		return nil, fmt.Errorf("listing employees: %w", err)
	}
	defer rows.Close()

	var employees []models.Employee
	for rows.Next() {
		var e models.Employee
		if err := rows.Scan(&e.ID, &e.OrgID, &e.UserID, &e.FirstName, &e.LastName, &e.Role, &e.Phone, &e.HireDate, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning employee: %w", err)
		}
		employees = append(employees, e)
	}
	return employees, rows.Err()
}

// GetEmployee returns a single employee by ID.
func (s *HRStore) GetEmployee(ctx context.Context, employeeID uuid.UUID) (*models.Employee, error) {
	var e models.Employee
	err := s.pool.QueryRow(ctx, `
		SELECT id, org_id, user_id, first_name, last_name, role, phone, hire_date, created_at
		FROM employees WHERE id = $1`, employeeID,
	).Scan(&e.ID, &e.OrgID, &e.UserID, &e.FirstName, &e.LastName, &e.Role, &e.Phone, &e.HireDate, &e.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting employee: %w", err)
	}
	return &e, nil
}

// ListCertifications returns certifications for an employee.
func (s *HRStore) ListCertifications(ctx context.Context, employeeID uuid.UUID) ([]models.Certification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, employee_id, cert_type, cert_number, issued_date, expiry_date, status, created_at
		FROM certifications WHERE employee_id = $1
		ORDER BY expiry_date NULLS LAST`, employeeID)
	if err != nil {
		return nil, fmt.Errorf("listing certifications: %w", err)
	}
	defer rows.Close()

	var certs []models.Certification
	for rows.Next() {
		var c models.Certification
		if err := rows.Scan(&c.ID, &c.EmployeeID, &c.CertType, &c.CertNumber, &c.IssuedDate, &c.ExpiryDate, &c.Status, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning certification: %w", err)
		}
		certs = append(certs, c)
	}
	return certs, rows.Err()
}

// ListExpiringCertifications returns certs expiring within the given days.
func (s *HRStore) ListExpiringCertifications(ctx context.Context, orgID uuid.UUID, withinDays int) ([]models.Certification, error) {
	cutoff := time.Now().AddDate(0, 0, withinDays)
	rows, err := s.pool.Query(ctx, `
		SELECT c.id, c.employee_id, c.cert_type, c.cert_number, c.issued_date, c.expiry_date, c.status, c.created_at
		FROM certifications c
		JOIN employees e ON e.id = c.employee_id
		WHERE e.org_id = $1
			AND c.expiry_date IS NOT NULL
			AND c.expiry_date <= $2
			AND c.status = 'active'
		ORDER BY c.expiry_date`, orgID, cutoff)
	if err != nil {
		return nil, fmt.Errorf("listing expiring certifications: %w", err)
	}
	defer rows.Close()

	var certs []models.Certification
	for rows.Next() {
		var c models.Certification
		if err := rows.Scan(&c.ID, &c.EmployeeID, &c.CertType, &c.CertNumber, &c.IssuedDate, &c.ExpiryDate, &c.Status, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning certification: %w", err)
		}
		certs = append(certs, c)
	}
	return certs, rows.Err()
}
