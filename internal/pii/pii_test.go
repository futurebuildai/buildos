package pii

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestClass_String(t *testing.T) {
	cases := map[Class]string{
		Public:       "public",
		Internal:     "internal",
		Confidential: "confidential",
		Restricted:   "restricted",
		Class(99):    "unknown",
	}
	for c, want := range cases {
		if got := c.String(); got != want {
			t.Errorf("Class(%d).String() = %q, want %q", c, got, want)
		}
	}
}

func TestClass_IsAtLeast(t *testing.T) {
	if !Restricted.IsAtLeast(Confidential) {
		t.Error("Restricted should rank above Confidential")
	}
	if Public.IsAtLeast(Internal) {
		t.Error("Public should not rank at-or-above Internal")
	}
	if !Confidential.IsAtLeast(Confidential) {
		t.Error("class should be at-least itself")
	}
}

func TestMaskString_PerClass(t *testing.T) {
	cases := []struct {
		in    string
		class Class
		want  string
	}{
		{"alice@buildos.dev", Public, "alice@buildos.dev"},   // public unchanged
		{"alice@buildos.dev", Internal, "alice@buildos.dev"}, // internal unchanged
		{"alice@buildos.dev", Restricted, "[REDACTED]"},       // PII fully redacted
		{"Acme Corp", Confidential, "A********"},              // first-char + length-preserving
		{"X", Confidential, "*"},                              // single-char edge case
		{"", Confidential, ""},                                // empty unchanged
		{"", Restricted, ""},                                  // empty unchanged
	}
	for _, c := range cases {
		got := MaskString(c.in, c.class)
		if got != c.want {
			t.Errorf("MaskString(%q, %s) = %q, want %q", c.in, c.class, got, c.want)
		}
	}
}

func TestMaskString_RestrictedDoesNotLeakLength(t *testing.T) {
	// Two emails of different lengths must produce identical
	// masks. Defends against the leak vector "I can guess which
	// user you're masking by counting redaction bytes."
	a := MaskString("a@x.io", Restricted)
	b := MaskString("a-very-long-email-address@long-domain-name.example.com", Restricted)
	if a != b {
		t.Errorf("restricted masks reveal length: %q vs %q", a, b)
	}
}

func TestClassFor_KnownFields(t *testing.T) {
	cases := map[string]Class{
		"email":         Restricted,
		"EMAIL":         Restricted, // case-insensitive
		"phone_number":  Restricted,
		"gps_lat":       Restricted,
		"oidc_subject":  Restricted,
		"sub":           Restricted,
		"vendor_name":   Confidential,
		"amount_cents":  Confidential,
		"request_id":    Internal,
		"trace_id":      Internal,
		"unknown_field": Internal, // default
	}
	for field, want := range cases {
		if got := ClassFor(field); got != want {
			t.Errorf("ClassFor(%q) = %s, want %s", field, got, want)
		}
	}
}

func TestScrubMap_RestrictedThresholdKeepsBusinessFields(t *testing.T) {
	in := map[string]any{
		"email":          "alice@buildos.dev",
		"phone":          "+1-555-0100",
		"gps_lat":        45.5231,
		"vendor_name":    "Acme Lumber",
		"amount_cents":   int64(450000),
		"request_id":     "req-abc-123",
		"event_type":     "feed.card.actioned",
	}
	out := ScrubMap(in, Restricted)

	// Restricted fields must be masked.
	if out["email"] != "[REDACTED]" {
		t.Errorf("email = %v, want [REDACTED]", out["email"])
	}
	if out["phone"] != "[REDACTED]" {
		t.Errorf("phone = %v, want [REDACTED]", out["phone"])
	}
	if out["gps_lat"] != "[REDACTED]" {
		t.Errorf("gps_lat = %v, want [REDACTED] (numeric Restricted)", out["gps_lat"])
	}

	// Confidential fields stay intact at the Restricted threshold —
	// the scrub is for PII only.
	if out["vendor_name"] != "Acme Lumber" {
		t.Errorf("vendor_name should pass at Restricted threshold; got %v", out["vendor_name"])
	}
	if out["amount_cents"] != int64(450000) {
		t.Errorf("amount_cents should pass at Restricted threshold; got %v", out["amount_cents"])
	}

	// Correlation ids stay clear — operators need them for triage.
	if out["request_id"] != "req-abc-123" {
		t.Errorf("request_id altered: %v", out["request_id"])
	}
	if out["event_type"] != "feed.card.actioned" {
		t.Errorf("event_type altered: %v", out["event_type"])
	}
}

func TestScrubMap_ConfidentialThresholdMasksBusinessFields(t *testing.T) {
	in := map[string]any{
		"vendor_name":  "Acme Lumber",
		"amount_cents": int64(450000),
		"request_id":   "req-abc",
	}
	out := ScrubMap(in, Confidential)
	if out["vendor_name"] != "A**********" {
		t.Errorf("vendor_name = %v, want A**********", out["vendor_name"])
	}
	if out["amount_cents"] != "[CONFIDENTIAL]" {
		t.Errorf("amount_cents = %v, want [CONFIDENTIAL] sentinel", out["amount_cents"])
	}
	if out["request_id"] != "req-abc" {
		t.Errorf("request_id should pass at Confidential threshold; got %v", out["request_id"])
	}
}

func TestScrubMap_NestedMapsAndSlices(t *testing.T) {
	in := map[string]any{
		"reporter": map[string]any{
			"email":      "bob@buildos.dev",
			"first_name": "Bob",
		},
		"crew_members": []any{
			map[string]any{"name": "Alice", "phone": "+1-555-0101"},
			map[string]any{"name": "Bob", "phone": "+1-555-0102"},
		},
		"event_type": "checkin",
	}
	out := ScrubMap(in, Restricted)

	reporter := out["reporter"].(map[string]any)
	if reporter["email"] != "[REDACTED]" || reporter["first_name"] != "[REDACTED]" {
		t.Errorf("nested map not scrubbed: %+v", reporter)
	}
	crew := out["crew_members"].([]any)
	for i, m := range crew {
		entry := m.(map[string]any)
		if entry["name"] != "[REDACTED]" || entry["phone"] != "[REDACTED]" {
			t.Errorf("crew[%d] not scrubbed: %+v", i, entry)
		}
	}
	if out["event_type"] != "checkin" {
		t.Errorf("event_type altered: %v", out["event_type"])
	}
}

func TestScrubJSON_RoundTrip(t *testing.T) {
	in := []byte(`{"email":"alice@x.io","org_id":"abc","amount_cents":1000}`)
	out := ScrubJSON(in, Restricted)

	var parsed map[string]any
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed["email"] != "[REDACTED]" {
		t.Errorf("email not scrubbed in JSON output: %v", parsed["email"])
	}
	if parsed["org_id"] != "abc" {
		t.Errorf("org_id altered: %v", parsed["org_id"])
	}
	// json.Unmarshal turns numbers into float64 by default.
	if v, ok := parsed["amount_cents"].(float64); !ok || v != 1000 {
		t.Errorf("amount_cents altered at Restricted threshold: %v", parsed["amount_cents"])
	}
}

func TestScrubJSON_ParseFailureReturnsInputUnchanged(t *testing.T) {
	// Better to ship a maybe-leaky audit row than drop it; the
	// alternative — dropping audit rows on a malformed blob — is
	// observability poison. Confirm the documented behavior.
	bad := []byte(`{not valid json`)
	out := ScrubJSON(bad, Restricted)
	if string(out) != string(bad) {
		t.Errorf("malformed input changed: %q → %q", bad, out)
	}
}

func TestScrubJSON_EmptyInputUnchanged(t *testing.T) {
	out := ScrubJSON(nil, Restricted)
	if len(out) != 0 {
		t.Errorf("nil input produced %q", out)
	}
}

func TestScrubMap_DoesNotModifyInput(t *testing.T) {
	// Defensive: the scrub functions return new maps, leaving the
	// caller's data intact. A future maintainer must not start
	// mutating in place — that would silently corrupt audit_log
	// inserts that hand a reference to the same map for both
	// persistence + Sentry.
	in := map[string]any{"email": "alice@x.io"}
	_ = ScrubMap(in, Restricted)
	if in["email"] != "alice@x.io" {
		t.Errorf("input modified: %v", in["email"])
	}
}

func TestMaskString_LengthPreservedForConfidential(t *testing.T) {
	// Length-preserved confidential masks let triage greps for
	// "value of length N starting with X" still match. Defends
	// against a future change that switches to fixed-length
	// "[REDACTED]" for Confidential too.
	in := "Acme Construction Co LLC"
	out := MaskString(in, Confidential)
	if len(out) != len(in) {
		t.Errorf("Confidential mask should preserve length; got %d vs %d", len(out), len(in))
	}
	if !strings.HasPrefix(out, "A") {
		t.Errorf("Confidential mask should keep first char; got %q", out)
	}
}
