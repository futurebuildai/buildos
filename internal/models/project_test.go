package models

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestProject_ClientContactNeverSerialized is the regression guard for review
// finding M2: the homeowner contact (client_name/email/phone) is Restricted PII
// and must NEVER appear in a serialized Project — the generic project read
// endpoints carry no role gate (field_worker+ can read a project) and the AI
// assistant's project tools (superintendent+) marshal the struct into model
// context. The fields are read server-side only (Go field access, below).
func TestProject_ClientContactNeverSerialized(t *testing.T) {
	name := "Jane Homeowner"
	email := "jane@homeowner.example"
	phone := "+15551234567"
	p := Project{
		ID:          uuid.New(),
		OrgID:       uuid.New(),
		Name:        "Maple Street Residence",
		Status:      "active",
		ClientName:  &name,
		ClientEmail: &email,
		ClientPhone: &phone,
	}

	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal project: %v", err)
	}
	body := string(raw)

	// Neither the values nor the field keys may appear in the JSON.
	for _, forbidden := range []string{name, email, phone, "client_name", "client_email", "client_phone"} {
		if strings.Contains(body, forbidden) {
			t.Errorf("Project JSON leaked homeowner contact %q: %s", forbidden, body)
		}
	}
	// The benign fields still serialize (sanity: we didn't drop the whole struct).
	if !strings.Contains(body, "Maple Street Residence") {
		t.Errorf("expected project name in JSON: %s", body)
	}

	// The fields remain readable SERVER-SIDE (the send path resolves ClientEmail
	// in-tx) — json:"-" affects serialization only, not Go field access.
	if p.ClientEmail == nil || *p.ClientEmail != email {
		t.Errorf("ClientEmail not readable as a Go field: %v", p.ClientEmail)
	}
}
