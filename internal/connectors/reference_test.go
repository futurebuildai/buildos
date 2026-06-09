package connectors

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestReferenceConnector_BuildTools(t *testing.T) {
	c := newReferenceConnector()
	if c.Name() != "reference" {
		t.Fatalf("Name = %q, want reference", c.Name())
	}
	if c.Description() == "" {
		t.Error("Description must be non-empty (admin-facing)")
	}
	tools, err := c.BuildTools(context.Background(), Caller{OrgID: uuid.New(), Role: "admin", Sub: "s"})
	if err != nil {
		t.Fatalf("BuildTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("got %d tools, want 2", len(tools))
	}
	names := map[string]bool{}
	for _, tl := range tools {
		names[tl.Spec.Name] = true
		if tl.MinRole != referenceMinRole {
			t.Errorf("tool %q MinRole = %q, want the connector's natural %q (service floors to admin)", tl.Spec.Name, tl.MinRole, referenceMinRole)
		}
		if tl.Executor == nil {
			t.Errorf("tool %q has a nil executor", tl.Spec.Name)
		}
		if !json.Valid(tl.Spec.InputSchema) {
			t.Errorf("tool %q InputSchema is not valid JSON", tl.Spec.Name)
		}
	}
	if !names["reference_glossary"] || !names["reference_supported_currencies"] {
		t.Errorf("missing expected tools; got %v", names)
	}
}

func TestReferenceGlossary_ListAll(t *testing.T) {
	res, err := referenceGlossaryExecute(context.Background(), nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("listing all should not be an error: %s", res.Content)
	}
	var out struct {
		Terms []map[string]string `json:"terms"`
		Count int                 `json:"count"`
	}
	if err := json.Unmarshal([]byte(res.Content), &out); err != nil {
		t.Fatalf("result not JSON: %v (%s)", err, res.Content)
	}
	if out.Count != len(referenceGlossary) || len(out.Terms) != len(referenceGlossary) {
		t.Errorf("count = %d / terms = %d, want %d", out.Count, len(out.Terms), len(referenceGlossary))
	}
	// Sorted, deterministic.
	for i := 1; i < len(out.Terms); i++ {
		if out.Terms[i-1]["term"] > out.Terms[i]["term"] {
			t.Errorf("glossary terms not sorted: %q before %q", out.Terms[i-1]["term"], out.Terms[i]["term"])
		}
	}
}

func TestReferenceGlossary_TermHit_CaseInsensitive(t *testing.T) {
	res, err := referenceGlossaryExecute(context.Background(), json.RawMessage(`{"term":"  WBS  "}`))
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if res.IsError {
		t.Fatalf("a known term must not be an error: %s", res.Content)
	}
	if !strings.Contains(res.Content, "Work Breakdown Structure") {
		t.Errorf("expected the WBS definition; got %s", res.Content)
	}
}

func TestReferenceGlossary_TermMiss_SoftError(t *testing.T) {
	res, err := referenceGlossaryExecute(context.Background(), json.RawMessage(`{"term":"nonexistent"}`))
	if err != nil {
		t.Fatalf("a miss must be a soft error (nil err), got: %v", err)
	}
	if !res.IsError {
		t.Errorf("an unknown term must be IsError so the model recovers in prose")
	}
}

func TestReferenceGlossary_BadJSON_SoftError(t *testing.T) {
	res, err := referenceGlossaryExecute(context.Background(), json.RawMessage(`{not json`))
	if err != nil {
		t.Fatalf("malformed args must be a soft error (nil err), got: %v", err)
	}
	if !res.IsError {
		t.Errorf("malformed args must be IsError, not a hard failure")
	}
}

func TestReferenceCurrencies(t *testing.T) {
	res, err := referenceCurrenciesExecute(context.Background(), nil)
	if err != nil || res.IsError {
		t.Fatalf("currencies should succeed: err=%v isErr=%v", err, res.IsError)
	}
	if !strings.Contains(res.Content, "USD") || !strings.Contains(res.Content, "CAD") {
		t.Errorf("expected USD + CAD; got %s", res.Content)
	}
}
