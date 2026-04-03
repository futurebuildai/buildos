package models

import (
	"time"

	"github.com/google/uuid"
)

// Employee represents an HR employee record.
// Matches employees table from migration 003.
type Employee struct {
	ID        uuid.UUID  `json:"id"`
	OrgID     uuid.UUID  `json:"org_id"`
	UserID    *uuid.UUID `json:"user_id,omitempty"`
	FirstName string     `json:"first_name"`
	LastName  string     `json:"last_name"`
	Role      string     `json:"role"`
	Phone     string     `json:"phone,omitempty"`
	HireDate  time.Time  `json:"hire_date"`
	CreatedAt time.Time  `json:"created_at"`
}

// Certification represents an employee certification with expiry tracking.
// Matches certifications table from migration 003.
type Certification struct {
	ID         uuid.UUID  `json:"id"`
	EmployeeID uuid.UUID  `json:"employee_id"`
	CertType   string     `json:"cert_type"`
	CertNumber string     `json:"cert_number"`
	IssuedDate time.Time  `json:"issued_date"`
	ExpiryDate time.Time  `json:"expiry_date"`
	Status     string     `json:"status"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CertStatus constants.
const (
	CertStatusActive  = "active"
	CertStatusExpired = "expired"
	CertStatusRevoked = "revoked"
)
