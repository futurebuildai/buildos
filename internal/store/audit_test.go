package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestScrubAuditPayloads_RestrictedFieldsRedacted is the contract:
// any Restricted-class field (per pii.FieldClass catalog) appearing
// in the JSONB blobs must be redacted before the row is composed.
//
// Only the JSONB payloads are touched; OrgID / UserSub / Action /
// ResourceType / RequestID columns are passed through unchanged
// (they're SQL columns, not free-form JSONB, and have their own
// retention rules).
func TestScrubAuditPayloads_RestrictedFieldsRedacted(t *testing.T) {
	cases := []struct {
		name     string
		blob     string
		mustHide []string // substrings that must NOT appear in scrubbed output
	}{
		{
			name:     "email_redacted",
			blob:     `{"email":"alice@example.com","action":"approve_quote"}`,
			mustHide: []string{"alice@example.com"},
		},
		{
			name:     "phone_redacted",
			blob:     `{"phone":"+1-415-555-0199","action":"sms_sent"}`,
			mustHide: []string{"415-555-0199", "+1-415"},
		},
		{
			name:     "ip_address_redacted",
			blob:     `{"ip_address":"203.0.113.42"}`,
			mustHide: []string{"203.0.113.42"},
		},
		{
			name:     "gps_coords_redacted",
			blob:     `{"gps_lat":"37.7749","gps_lng":"-122.4194"}`,
			mustHide: []string{"37.7749", "-122.4194"},
		},
		{
			name:     "first_last_name_redacted",
			blob:     `{"first_name":"Alice","last_name":"Liddell"}`,
			mustHide: []string{"Alice", "Liddell"},
		},
		{
			name:     "oidc_sub_redacted",
			blob:     `{"sub":"auth0|abc123","org_id":"00000000-0000-0000-0000-000000000001"}`,
			mustHide: []string{"auth0|abc123"},
		},
		{
			name: "nested_email_redacted",
			blob: `{"actor":{"email":"bob@example.com","name":"Bob"},
			        "action":"feed.card.actioned"}`,
			mustHide: []string{"bob@example.com", "\"Bob\""},
		},
		{
			name:     "list_of_phones_redacted",
			blob:     `{"phone":["+1-555-0001","+1-555-0002"]}`,
			mustHide: []string{"555-0001", "555-0002"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := InsertAuditParams{
				OrgID:        uuid.New(),
				Action:       "x",
				ResourceType: "y",
				ResourceID:   uuid.New(),
				Before:       json.RawMessage(tc.blob),
				After:        json.RawMessage(tc.blob),
				Metadata:     json.RawMessage(tc.blob),
			}
			out := scrubAuditPayloads(in)

			for _, blob := range [][]byte{out.Before, out.After, out.Metadata} {
				s := string(blob)
				for _, hidden := range tc.mustHide {
					if strings.Contains(s, hidden) {
						t.Errorf("scrubbed payload still contains %q: %s", hidden, s)
					}
				}
				// Sanity: scrubbed output must still be valid JSON.
				var v any
				if err := json.Unmarshal(blob, &v); err != nil {
					t.Errorf("scrubbed payload is not valid JSON: %v\nout=%s", err, s)
				}
			}
		})
	}
}

// TestScrubAuditPayloads_ConfidentialPreserved confirms that
// business-sensitive fields (vendor_name, *_cents amounts, project
// names) are NOT redacted at the Restricted threshold — they're
// Confidential, which is below Restricted. The audit log keeps its
// investigative value (a reviewer can see WHICH vendor and HOW MUCH
// without being able to identify WHO).
func TestScrubAuditPayloads_ConfidentialPreserved(t *testing.T) {
	in := InsertAuditParams{
		OrgID:        uuid.New(),
		Action:       "procurement.quote.approved",
		ResourceType: "quote",
		ResourceID:   uuid.New(),
		Before: json.RawMessage(
			`{"vendor_name":"Acme Lumber","amount_cents":150000,"currency_code":"USD"}`),
	}
	out := scrubAuditPayloads(in)

	s := string(out.Before)
	for _, expected := range []string{"Acme Lumber", "150000", "USD"} {
		if !strings.Contains(s, expected) {
			t.Errorf("Confidential field %q was scrubbed at Restricted threshold; should be preserved\nout=%s",
				expected, s)
		}
	}
}

// TestScrubAuditPayloads_NilAndEmptyAreSafe — calling the scrub on
// nil or empty RawMessage must not panic and must produce the same
// nil/empty value. Required because Before/After/Metadata are all
// nullable in the parameter struct.
func TestScrubAuditPayloads_NilAndEmptyAreSafe(t *testing.T) {
	in := InsertAuditParams{
		OrgID:        uuid.New(),
		Action:       "x",
		ResourceType: "y",
		ResourceID:   uuid.New(),
		// All three JSONB fields zero-value.
	}
	out := scrubAuditPayloads(in)
	if len(out.Before) != 0 || len(out.After) != 0 || len(out.Metadata) != 0 {
		t.Errorf("nil JSONB inputs must produce nil outputs; got before=%d after=%d meta=%d",
			len(out.Before), len(out.After), len(out.Metadata))
	}
}

// TestScrubAuditPayloads_MalformedJSONPassesThrough confirms the
// documented caveat: invalid JSON is returned unchanged (per the
// pii.ScrubJSON contract — better to ship a maybe-leaky audit than
// drop the row entirely). Callers are responsible for providing
// valid JSON.
func TestScrubAuditPayloads_MalformedJSONPassesThrough(t *testing.T) {
	bad := json.RawMessage(`{"email":"alice@example.com",`)
	in := InsertAuditParams{
		OrgID:        uuid.New(),
		Action:       "x",
		ResourceType: "y",
		ResourceID:   uuid.New(),
		Before:       bad,
	}
	out := scrubAuditPayloads(in)
	if string(out.Before) != string(bad) {
		t.Errorf("malformed JSON was modified; expected pass-through\nin=%s\nout=%s",
			bad, out.Before)
	}
}

// TestScrubAuditPayloads_Idempotent — scrubbing already-scrubbed
// data should produce identical bytes. Important because the same
// AuditStore could be invoked from a retry path.
func TestScrubAuditPayloads_Idempotent(t *testing.T) {
	in := InsertAuditParams{
		OrgID:        uuid.New(),
		Action:       "x",
		ResourceType: "y",
		ResourceID:   uuid.New(),
		Before:       json.RawMessage(`{"email":"alice@example.com","amount_cents":1500}`),
	}
	once := scrubAuditPayloads(in)
	twice := scrubAuditPayloads(once)

	if string(once.Before) != string(twice.Before) {
		t.Errorf("scrub is not idempotent\nonce=%s\ntwice=%s", once.Before, twice.Before)
	}
}
