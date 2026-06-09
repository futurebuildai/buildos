package connectors

import (
	"strings"
	"testing"
)

func TestNamespaceToolName(t *testing.T) {
	got := NamespaceToolName("reference", "glossary")
	if got != "conn__reference__glossary" {
		t.Fatalf("NamespaceToolName = %q, want conn__reference__glossary", got)
	}
	// The namespaced name must never collide with a bare internal tool name.
	if !strings.HasPrefix(got, "conn__") {
		t.Errorf("namespaced names must carry the conn__ prefix that internal tools never use")
	}
}

func TestValidToolName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"conn__reference__glossary", true},
		{"list_projects", true},
		{"a-b_c-1", true},
		{"", false},
		{"has space", false},
		{"has:colon", false},
		{"emoji😀", false},
		{strings.Repeat("x", 128), true},
		{strings.Repeat("x", 129), false},
	}
	for _, c := range cases {
		if got := ValidToolName(c.name); got != c.ok {
			t.Errorf("ValidToolName(%q) = %v, want %v", c.name, got, c.ok)
		}
	}
}

func TestBuiltins_IncludesReference(t *testing.T) {
	bs := Builtins()
	var names []string
	for _, b := range bs {
		names = append(names, b.Name())
	}
	found := false
	for _, n := range names {
		if n == "reference" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Builtins() must include the reference connector; got %v", names)
	}
}
