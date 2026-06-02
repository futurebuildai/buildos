package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// These tests exercise ProjectService's early-return validation paths
// and its pure helpers — everything that runs BEFORE pgx.BeginTxFunc
// touches the pool. Because every rejection short-circuits ahead of any
// DB call, a service built with a nil pool/store is safe here; the
// round-trip (insert/update + audit) lives in the store integration
// suite. NewProjectService(nil, nil, nil) also exercises the nil-audit
// → NewNoopAuditRecorder() fallback.

func newValidationOnlyService() *ProjectService {
	return NewProjectService(nil, nil, nil)
}

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

func TestListProjects_Validation(t *testing.T) {
	svc := newValidationOnlyService()
	cases := []struct {
		name string
		in   ListProjectsInput
	}{
		{"nil org", ListProjectsInput{OrgID: uuid.Nil}},
		{"unknown status", ListProjectsInput{OrgID: uuid.New(), Status: "bogus"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.ListProjects(context.Background(), c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestGetProject_Validation(t *testing.T) {
	svc := newValidationOnlyService()
	cases := []struct {
		name      string
		orgID     uuid.UUID
		projectID uuid.UUID
	}{
		{"nil org", uuid.Nil, uuid.New()},
		{"nil project", uuid.New(), uuid.Nil},
		{"both nil", uuid.Nil, uuid.Nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.GetProject(context.Background(), c.orgID, c.projectID)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestCreateProject_Validation(t *testing.T) {
	svc := newValidationOnlyService()
	cases := []struct {
		name string
		in   CreateProjectInput
	}{
		{"nil org", CreateProjectInput{OrgID: uuid.Nil, Name: "Maple St"}},
		{"blank name", CreateProjectInput{OrgID: uuid.New(), Name: "   "}},
		{"empty name", CreateProjectInput{OrgID: uuid.New(), Name: ""}},
		{"gsf below floor", CreateProjectInput{OrgID: uuid.New(), Name: "Maple St", GSF: intptr(projectGSFMin - 1)}},
		{"gsf above ceiling", CreateProjectInput{OrgID: uuid.New(), Name: "Maple St", GSF: intptr(projectGSFMax + 1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.CreateProject(context.Background(), c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestUpdateProject_Validation(t *testing.T) {
	svc := newValidationOnlyService()
	cases := []struct {
		name string
		in   UpdateProjectInput
	}{
		{"nil org", UpdateProjectInput{OrgID: uuid.Nil, ProjectID: uuid.New(), Name: strptr("x")}},
		{"nil project", UpdateProjectInput{OrgID: uuid.New(), ProjectID: uuid.Nil, Name: strptr("x")}},
		{"no fields set", UpdateProjectInput{OrgID: uuid.New(), ProjectID: uuid.New()}},
		{"blank name", UpdateProjectInput{OrgID: uuid.New(), ProjectID: uuid.New(), Name: strptr("   ")}},
		{"unknown status", UpdateProjectInput{OrgID: uuid.New(), ProjectID: uuid.New(), Status: strptr("bogus")}},
		{"gsf below floor", UpdateProjectInput{OrgID: uuid.New(), ProjectID: uuid.New(), GSF: intptr(projectGSFMin - 1)}},
		{"gsf above ceiling", UpdateProjectInput{OrgID: uuid.New(), ProjectID: uuid.New(), GSF: intptr(projectGSFMax + 1)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := svc.UpdateProject(context.Background(), c.in)
			if !errors.Is(err, ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

func TestPaginate(t *testing.T) {
	cases := []struct {
		name             string
		page, perPage    int
		wantLim, wantOff int
	}{
		{"defaults on zero", 0, 0, defaultProjectPageSize, 0},
		{"negative page floors to 1", -5, 0, defaultProjectPageSize, 0},
		{"negative perPage falls to default", 1, -1, defaultProjectPageSize, 0},
		{"perPage clamped to max", 1, maxProjectPageSize + 100, maxProjectPageSize, 0},
		{"page 2 offset", 2, 25, 25, 25},
		{"page 3 offset", 3, 10, 10, 20},
		{"perPage exactly max", 2, maxProjectPageSize, maxProjectPageSize, maxProjectPageSize},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotLim, gotOff := paginate(c.page, c.perPage)
			if gotLim != c.wantLim || gotOff != c.wantOff {
				t.Errorf("paginate(%d, %d) = (%d, %d), want (%d, %d)",
					c.page, c.perPage, gotLim, gotOff, c.wantLim, c.wantOff)
			}
		})
	}
}

func TestValidateGSF(t *testing.T) {
	cases := []struct {
		name    string
		gsf     *int
		wantErr bool
	}{
		{"nil is valid", nil, false},
		{"one below floor rejects", intptr(projectGSFMin - 1), true},
		{"exactly floor ok", intptr(projectGSFMin), false},
		{"mid-range ok", intptr((projectGSFMin + projectGSFMax) / 2), false},
		{"exactly ceiling ok", intptr(projectGSFMax), false},
		{"one above ceiling rejects", intptr(projectGSFMax + 1), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateGSF(c.gsf)
			if c.wantErr {
				if !errors.Is(err, ErrInvalidInput) {
					t.Errorf("validateGSF = %v, want ErrInvalidInput", err)
				}
				return
			}
			if err != nil {
				t.Errorf("validateGSF = %v, want nil", err)
			}
		})
	}
}

func TestNormalizeOptionalString(t *testing.T) {
	t.Run("nil stays nil", func(t *testing.T) {
		if got := normalizeOptionalString(nil); got != nil {
			t.Errorf("got %q, want nil", *got)
		}
	})
	t.Run("blank collapses to nil", func(t *testing.T) {
		if got := normalizeOptionalString(strptr("   ")); got != nil {
			t.Errorf("got %q, want nil", *got)
		}
	})
	t.Run("empty collapses to nil", func(t *testing.T) {
		if got := normalizeOptionalString(strptr("")); got != nil {
			t.Errorf("got %q, want nil", *got)
		}
	})
	t.Run("surrounding whitespace trimmed", func(t *testing.T) {
		got := normalizeOptionalString(strptr("  123 Maple St  "))
		if got == nil || *got != "123 Maple St" {
			t.Errorf("got %v, want %q", got, "123 Maple St")
		}
	})
}
