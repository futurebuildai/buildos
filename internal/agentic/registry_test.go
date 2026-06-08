package agentic

import "testing"

func TestRegistry_SeededWithDelayCascade(t *testing.T) {
	r := NewRegistry()
	d, ok := r.Lookup(DelayCascade)
	if !ok {
		t.Fatal("expected delay_cascade to be seeded in a new registry")
	}
	if d.Capability != DelayCascade {
		t.Fatalf("descriptor capability = %q, want %q", d.Capability, DelayCascade)
	}
	if d.Description == "" {
		t.Fatal("seeded descriptor must carry a non-empty description")
	}
}

func TestRegistry_LookupUnknown(t *testing.T) {
	r := NewRegistry()
	if _, ok := r.Lookup(Capability("nope")); ok {
		t.Fatal("Lookup of an unregistered capability should report not-found")
	}
}

func TestRegistry_RegisterAndCapabilitiesSorted(t *testing.T) {
	r := NewRegistry()
	// Register out of sorted order to prove Capabilities() sorts.
	r.Register(Descriptor{Capability: Capability("zeta_flow"), Description: "z"})
	r.Register(Descriptor{Capability: Capability("alpha_flow"), Description: "a"})

	caps := r.Capabilities()
	if len(caps) != 3 {
		t.Fatalf("Capabilities len = %d, want 3 (delay_cascade + 2)", len(caps))
	}
	// Sorted: alpha_flow, delay_cascade, zeta_flow.
	want := []Capability{Capability("alpha_flow"), DelayCascade, Capability("zeta_flow")}
	for i, w := range want {
		if caps[i].Capability != w {
			t.Fatalf("caps[%d].Capability = %q, want %q", i, caps[i].Capability, w)
		}
	}
}

func TestRegistry_RegisterOverwrites(t *testing.T) {
	r := NewRegistry()
	r.Register(Descriptor{Capability: DelayCascade, Description: "rewritten"})
	d, _ := r.Lookup(DelayCascade)
	if d.Description != "rewritten" {
		t.Fatalf("Register should overwrite; description = %q, want %q", d.Description, "rewritten")
	}
	if len(r.Capabilities()) != 1 {
		t.Fatalf("overwrite must not grow the catalog; len = %d, want 1", len(r.Capabilities()))
	}
}

func TestRegistry_RegisterEmptyPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Register with an empty capability key should panic")
		}
	}()
	r := NewRegistry()
	r.Register(Descriptor{Capability: "", Description: "x"})
}

func TestCapability_String(t *testing.T) {
	if DelayCascade.String() != "delay_cascade" {
		t.Fatalf("DelayCascade.String() = %q, want %q", DelayCascade.String(), "delay_cascade")
	}
}
