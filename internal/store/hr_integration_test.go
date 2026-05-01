//go:build integration

package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/testdb"
)

// seedEmployee inserts one row and returns its id.
func seedEmployee(t *testing.T, pool *pgxpool.Pool, orgID uuid.UUID, last, first, role string) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
		INSERT INTO employees (org_id, first_name, last_name, role)
		VALUES ($1, $2, $3, $4)
		RETURNING id`,
		orgID, first, last, role,
	).Scan(&id)
	if err != nil {
		t.Fatalf("seed employee: %v", err)
	}
	return id
}

// seedCert inserts one row.
func seedCert(t *testing.T, pool *pgxpool.Pool, employeeID uuid.UUID, certType string, expiry time.Time) {
	t.Helper()
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		INSERT INTO certifications (employee_id, cert_type, expiry_date)
		VALUES ($1, $2, $3)`,
		employeeID, certType, expiry,
	); err != nil {
		t.Fatalf("seed cert: %v", err)
	}
}

func TestHRStore_ListEmployees_OrderingAndCrossOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewHRStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	// Three rows in orgA, intentionally out of order so the directory
	// ordering (last_name ASC, first_name ASC) is non-trivial.
	seedEmployee(t, pool, orgA, "Smith", "Bob", "field_worker")
	seedEmployee(t, pool, orgA, "Adams", "Alice", "superintendent")
	seedEmployee(t, pool, orgA, "Smith", "Anna", "owner")
	seedEmployee(t, pool, orgB, "Anderson", "Carol", "owner")

	t.Run("returns only org's rows in directory order", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListEmployees(ctx, tx, orgA)
			if err != nil {
				return err
			}
			if len(rows) != 3 {
				t.Fatalf("got %d rows, want 3", len(rows))
			}
			// Adams, Smith (Anna), Smith (Bob) — last_name ASC then first_name ASC.
			wantLast := []string{"Adams", "Smith", "Smith"}
			wantFirst := []string{"Alice", "Anna", "Bob"}
			for i := range wantLast {
				if rows[i].LastName != wantLast[i] || rows[i].FirstName != wantFirst[i] {
					t.Errorf("row %d: got %s %s, want %s %s",
						i, rows[i].FirstName, rows[i].LastName, wantFirst[i], wantLast[i])
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cross-org returns zero", func(t *testing.T) {
		otherOrg := uuid.New()
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			rows, err := s.ListEmployees(ctx, tx, otherOrg)
			if err != nil {
				return err
			}
			if len(rows) != 0 {
				t.Errorf("got %d rows, want 0", len(rows))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestHRStore_VerifyEmployeeInOrg(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewHRStore()
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	testdb.SeedOrg(t, pool, orgA, "Org A")
	testdb.SeedOrg(t, pool, orgB, "Org B")

	empA := seedEmployee(t, pool, orgA, "Smith", "Bob", "field_worker")

	t.Run("belongs returns nil", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			return s.VerifyEmployeeInOrg(ctx, tx, empA, orgA)
		})
		if err != nil {
			t.Errorf("err = %v, want nil", err)
		}
	})

	t.Run("cross-org returns ErrEmployeeNotFound", func(t *testing.T) {
		err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			err := s.VerifyEmployeeInOrg(ctx, tx, empA, orgB)
			if !errors.Is(err, ErrEmployeeNotFound) {
				t.Errorf("err = %v, want ErrEmployeeNotFound", err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestHRStore_ListCertifications_ExpiryOrder(t *testing.T) {
	pool := testdb.NewPool(t)
	s := NewHRStore()
	ctx := context.Background()

	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Acme")
	emp := seedEmployee(t, pool, orgID, "Smith", "Bob", "owner")

	// Three certs with distinct expiry dates.
	seedCert(t, pool, emp, "osha_10", time.Date(2027, 1, 15, 0, 0, 0, 0, time.UTC))
	seedCert(t, pool, emp, "contractor_license", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	seedCert(t, pool, emp, "insurance", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))

	err := pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		rows, err := s.ListCertifications(ctx, tx, emp)
		if err != nil {
			return err
		}
		if len(rows) != 3 {
			t.Fatalf("got %d rows, want 3", len(rows))
		}
		// Soonest-expiring first.
		wantTypes := []string{"contractor_license", "insurance", "osha_10"}
		for i, w := range wantTypes {
			if rows[i].CertType != w {
				t.Errorf("row %d: cert_type = %q, want %q", i, rows[i].CertType, w)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
