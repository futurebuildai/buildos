package models

import (
	"time"

	"github.com/google/uuid"
)

// Employee mirrors an employees row. user_id is nullable — not every
// employee has a corresponding users row (a user is anyone with login
// credentials; an employee may exist as a record-only contact).
type Employee struct {
	ID        uuid.UUID  `json:"id"`
	OrgID     uuid.UUID  `json:"org_id"`
	UserID    *uuid.UUID `json:"user_id,omitempty"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Role      string     `json:"role"`
	Phone     *string    `json:"phone,omitempty"`
	HireDate  *time.Time `json:"hire_date,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Certification statuses. Schema default is 'active'.
const (
	CertificationStatusActive  = "active"
	CertificationStatusExpired = "expired"
	CertificationStatusRevoked = "revoked"
)

// IsValidCertificationStatus reports whether s is one of the allowed
// values.
func IsValidCertificationStatus(s string) bool {
	switch s {
	case CertificationStatusActive, CertificationStatusExpired, CertificationStatusRevoked:
		return true
	default:
		return false
	}
}

// Certification mirrors a certifications row. expiry_date is NOT NULL
// at the schema layer — the daily CertificationAlertsWorker scans for
// near-expiry rows. issued_date and cert_number are optional.
type Certification struct {
	ID         uuid.UUID  `json:"id"`
	EmployeeID uuid.UUID  `json:"employee_id"`
	CertType   string     `json:"cert_type"`
	CertNumber *string    `json:"cert_number,omitempty"`
	IssuedDate *time.Time `json:"issued_date,omitempty"`
	ExpiryDate time.Time  `json:"expiry_date"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
}
