//go:build integration

package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"

	"github.com/futurebuildai/buildos/internal/store"
	"github.com/futurebuildai/buildos/internal/testdb"
)

// newProjectService wires a ProjectService to a fresh migrated pool with a real
// audit recorder (so the in-tx project.created/updated writes hit audit_log)
// and a seeded org. Returns the service + org id; the pool is reachable via
// svc.pool for direct audit_log asserts.
func newProjectService(t *testing.T) (*ProjectService, uuid.UUID) {
	t.Helper()
	pool := testdb.NewPool(t)
	orgID := uuid.New()
	testdb.SeedOrg(t, pool, orgID, "Cedar Ridge Homes")

	audit := NewAuditService(store.NewAuditStore(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	svc := NewProjectService(pool, store.NewProjectStore(), audit)
	return svc, orgID
}

func projectAuditCount(t *testing.T, s *ProjectService, orgID uuid.UUID, action string) int {
	t.Helper()
	var n int
	if err := s.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE org_id = $1 AND action = $2`,
		orgID, action).Scan(&n); err != nil {
		t.Fatalf("count audit_log: %v", err)
	}
	return n
}

// TestProjectService_CreateGetUpdate_Lifecycle is the canonical CRUD round-trip
// against a real Postgres: create (default 'active' status + a project.created
// audit row), read it back scoped to the org, then partial-update name+status
// (a project.updated audit row, original create untouched).
func TestProjectService_CreateGetUpdate_Lifecycle(t *testing.T) {
	svc, orgID := newProjectService(t)
	ctx := context.Background()

	gsf := 3200
	created, err := svc.CreateProject(ctx, CreateProjectInput{
		OrgID:   orgID,
		UserSub: "owner-sub",
		Name:    "  Birch Lane New Build  ", // trimmed by the service
		Address: strptr("  "),               // blank → normalized to NULL
		GSF:     &gsf,
	})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if created.ID == uuid.Nil {
		t.Fatal("created project has nil id")
	}
	if created.Name != "Birch Lane New Build" {
		t.Errorf("name = %q, want trimmed", created.Name)
	}
	if created.Status != "active" {
		t.Errorf("status = %q, want default active", created.Status)
	}
	if created.Address != "" {
		t.Errorf("blank address should normalize to empty, got %q", created.Address)
	}
	if got := projectAuditCount(t, svc, orgID, AuditActionProjectCreated); got != 1 {
		t.Errorf("project.created audit rows = %d, want 1", got)
	}

	got, err := svc.GetProject(ctx, orgID, created.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.ID != created.ID || got.Name != created.Name {
		t.Errorf("GetProject = %+v, want the created row", got)
	}

	updated, err := svc.UpdateProject(ctx, UpdateProjectInput{
		OrgID:     orgID,
		ProjectID: created.ID,
		UserSub:   "owner-sub",
		Name:      strptr("Birch Lane (Phase 2)"),
		Status:    strptr("completed"),
	})
	if err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}
	if updated.Name != "Birch Lane (Phase 2)" || updated.Status != "completed" {
		t.Errorf("update = %q/%q, want renamed+completed", updated.Name, updated.Status)
	}
	if got := projectAuditCount(t, svc, orgID, AuditActionProjectUpdated); got != 1 {
		t.Errorf("project.updated audit rows = %d, want 1", got)
	}
}

// TestProjectService_CrossTenantIsolation proves the org scoping on the read
// and update paths: a project created under org A is invisible to org B —
// GetProject and UpdateProject both return ErrNotFound rather than leaking the
// row's existence.
func TestProjectService_CrossTenantIsolation(t *testing.T) {
	svc, orgA := newProjectService(t)
	ctx := context.Background()

	// Seed a second org sharing the same pool.
	orgB := uuid.New()
	testdb.SeedOrg(t, svc.pool, orgB, "Rival Builders")

	p, err := svc.CreateProject(ctx, CreateProjectInput{OrgID: orgA, UserSub: "a", Name: "A's project"})
	if err != nil {
		t.Fatalf("seed CreateProject: %v", err)
	}

	if _, err := svc.GetProject(ctx, orgB, p.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant GetProject = %v, want ErrNotFound", err)
	}
	if _, err := svc.UpdateProject(ctx, UpdateProjectInput{
		OrgID: orgB, ProjectID: p.ID, UserSub: "b", Status: strptr("archived"),
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("cross-tenant UpdateProject = %v, want ErrNotFound", err)
	}
	// No update audit row should have been written for the foreign org.
	if got := projectAuditCount(t, svc, orgB, AuditActionProjectUpdated); got != 0 {
		t.Errorf("foreign-org update wrote %d audit rows, want 0", got)
	}
}

// TestProjectService_ListProjects_FilterAndPaginate covers the list read: an
// org sees only its own projects, the status filter narrows the set, and
// per_page caps the page size (newest-first ordering from the store).
func TestProjectService_ListProjects_FilterAndPaginate(t *testing.T) {
	svc, orgID := newProjectService(t)
	ctx := context.Background()

	// Three active + one archived.
	for _, name := range []string{"P1", "P2", "P3"} {
		if _, err := svc.CreateProject(ctx, CreateProjectInput{OrgID: orgID, UserSub: "o", Name: name}); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	archived, err := svc.CreateProject(ctx, CreateProjectInput{OrgID: orgID, UserSub: "o", Name: "P4"})
	if err != nil {
		t.Fatalf("seed P4: %v", err)
	}
	if _, err := svc.UpdateProject(ctx, UpdateProjectInput{
		OrgID: orgID, ProjectID: archived.ID, UserSub: "o", Status: strptr("archived"),
	}); err != nil {
		t.Fatalf("archive P4: %v", err)
	}

	all, err := svc.ListProjects(ctx, ListProjectsInput{OrgID: orgID})
	if err != nil {
		t.Fatalf("ListProjects(all): %v", err)
	}
	if len(all) != 4 {
		t.Errorf("unfiltered list len = %d, want 4", len(all))
	}

	activeOnly, err := svc.ListProjects(ctx, ListProjectsInput{OrgID: orgID, Status: "active"})
	if err != nil {
		t.Fatalf("ListProjects(active): %v", err)
	}
	if len(activeOnly) != 3 {
		t.Errorf("active list len = %d, want 3", len(activeOnly))
	}

	firstPage, err := svc.ListProjects(ctx, ListProjectsInput{OrgID: orgID, PerPage: 2, Page: 1})
	if err != nil {
		t.Fatalf("ListProjects(page1): %v", err)
	}
	if len(firstPage) != 2 {
		t.Errorf("page-1 len = %d, want 2 (per_page cap)", len(firstPage))
	}
}

// TestProjectService_Validation covers the pre-tx input gates across the three
// mutating/read entrypoints — each rejects with ErrInvalidInput before any row
// is touched.
func TestProjectService_Validation(t *testing.T) {
	svc, orgID := newProjectService(t)
	ctx := context.Background()

	tooBig := 9000 // outside the [1500,6000] residential envelope
	if _, err := svc.CreateProject(ctx, CreateProjectInput{OrgID: orgID, Name: "x", GSF: intptr(tooBig)}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateProject(gsf 9000) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateProject(ctx, CreateProjectInput{OrgID: orgID, Name: "   "}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateProject(blank name) = %v, want ErrInvalidInput", err)
	}
	if _, err := svc.CreateProject(ctx, CreateProjectInput{OrgID: uuid.Nil, Name: "x"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("CreateProject(nil org) = %v, want ErrInvalidInput", err)
	}

	if _, err := svc.ListProjects(ctx, ListProjectsInput{OrgID: orgID, Status: "bogus"}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("ListProjects(bad status) = %v, want ErrInvalidInput", err)
	}

	// Update with no fields set → "at least one field required".
	if _, err := svc.UpdateProject(ctx, UpdateProjectInput{OrgID: orgID, ProjectID: uuid.New()}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("UpdateProject(empty patch) = %v, want ErrInvalidInput", err)
	}
	// Update with a blank name → rejected even though a field is present.
	if _, err := svc.UpdateProject(ctx, UpdateProjectInput{OrgID: orgID, ProjectID: uuid.New(), Name: strptr("  ")}); !errors.Is(err, ErrInvalidInput) {
		t.Errorf("UpdateProject(blank name) = %v, want ErrInvalidInput", err)
	}
}
