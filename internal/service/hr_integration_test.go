//go:build integration

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// seedHREmployee inserts one employee row and returns its id.
func seedHREmployee(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID, last, first, role string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO employees (org_id, first_name, last_name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		orgID, first, last, role,
	).Scan(&id); err != nil {
		t.Fatalf("seed employee: %v", err)
	}
	return id
}

// seedHRCert inserts one certification row for an employee.
func seedHRCert(t *testing.T, pool *pgxpool.Pool, employeeID uuid.UUID, certType string, expiry time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO certifications (employee_id, cert_type, expiry_date)
		VALUES ($1, $2, $3)`,
		employeeID, certType, expiry,
	); err != nil {
		t.Fatalf("seed cert: %v", err)
	}
}

// TestHRService_ListEmployees drives the read-only-tx wrapper end-to-end:
// the happy path (directory order + strict org isolation) plus the
// nil-org validation guard. The store-level ordering/isolation is proven
// in store/hr_integration_test.go; here we cover the service tx wiring.
func TestHRService_ListEmployees(t *testing.T) {
	pool := testdb.NewPool(t)
	svc := NewHRService(pool, store.NewHRStore())
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	seedHREmployee(t, pool, orgA, "Smith", "Bob", "field_worker")
	seedHREmployee(t, pool, orgA, "Adams", "Alice", "superintendent")
	seedHREmployee(t, pool, orgB, "Anderson", "Carol", "owner")

	t.Run("returns only caller-org rows in directory order", func(t *testing.T) {
		got, err := svc.ListEmployees(ctx, orgA)
		if err != nil {
			t.Fatalf("ListEmployees: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d employees, want 2 (org isolation)", len(got))
		}
		// last_name ASC, first_name ASC → Adams then Smith.
		if got[0].LastName != "Adams" || got[1].LastName != "Smith" {
			t.Errorf("order = %q, %q; want Adams, Smith", got[0].LastName, got[1].LastName)
		}
	})

	t.Run("nil caller org is rejected", func(t *testing.T) {
		if _, err := svc.ListEmployees(ctx, uuid.Nil); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})
}

// TestHRService_ListCertifications covers the happy path, the input
// guard, and — most importantly — the cross-org leg where the store's
// VerifyEmployeeInOrg failure is translated to the service-level
// ErrEmployeeNotFound sentinel (never leaking existence across tenants).
func TestHRService_ListCertifications(t *testing.T) {
	pool := testdb.NewPool(t)
	svc := NewHRService(pool, store.NewHRStore())
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	emp := seedHREmployee(t, pool, orgA, "Smith", "Bob", "owner")
	seedHRCert(t, pool, emp, "osha_10", time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC))
	seedHRCert(t, pool, emp, "contractor_license", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))

	t.Run("returns certs soonest-expiring first", func(t *testing.T) {
		got, err := svc.ListCertifications(ctx, orgA, emp)
		if err != nil {
			t.Fatalf("ListCertifications: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d certs, want 2", len(got))
		}
		if got[0].CertType != "contractor_license" {
			t.Errorf("first cert = %q, want contractor_license", got[0].CertType)
		}
	})

	t.Run("cross-org access surfaces ErrEmployeeNotFound", func(t *testing.T) {
		// orgB asking for orgA's employee: must not leak existence.
		if _, err := svc.ListCertifications(ctx, orgB, emp); !errors.Is(err, ErrEmployeeNotFound) {
			t.Fatalf("err = %v, want ErrEmployeeNotFound", err)
		}
	})

	t.Run("nil employee id is rejected", func(t *testing.T) {
		if _, err := svc.ListCertifications(ctx, orgA, uuid.Nil); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("err = %v, want ErrInvalidInput", err)
		}
	})
}
