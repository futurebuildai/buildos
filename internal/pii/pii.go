// Package pii classifies BuildOS data and provides masking helpers
// for any egress channel that could leak personal data outside the
// fork's deployment boundary (third-party error trackers, support
// log dumps, customer-provided observability stacks).
//
// The four classifications match the standard enterprise data
// taxonomy used by SOC 2 / ISO 27001 / GDPR:
//
//	Public        — safely shareable: org names, UUIDs, build versions.
//	Internal      — operational metadata: request_ids, trace_ids,
//	                feature flags, status enums.
//	Confidential  — business-sensitive but not personal: invoice
//	                numbers, project names, vendor names, financial
//	                amounts. Scrubbed for external tooling but kept
//	                in BuildOS's own audit log.
//	Restricted    — personal data subject to GDPR / CCPA: names,
//	                emails, phone numbers, GPS coordinates, OIDC
//	                subjects, IP addresses. Masked at every egress
//	                point; logged at debug only inside BuildOS.
//
// The classification is encoded as a tagged enum so future code
// (e.g. a "data export" feature) can apply the same rules.
package pii

import (
	"encoding/json"
	"strings"
)

// Class enumerates the four classification levels. Higher values =
// more restrictive. Use IsAtLeast to compare.
type Class int

const (
	Public Class = iota
	Internal
	Confidential
	Restricted
)

// String returns a stable lowercase name for the class. Useful for
// log tags + audit metadata.
func (c Class) String() string {
	switch c {
	case Public:
		return "public"
	case Internal:
		return "internal"
	case Confidential:
		return "confidential"
	case Restricted:
		return "restricted"
	default:
		return "unknown"
	}
}

// IsAtLeast reports whether c is at or above the threshold. Used by
// scrubbers ("scrub anything at or above Confidential when sending
// to Sentry").
func (c Class) IsAtLeast(threshold Class) bool { return c >= threshold }

// MaskString redacts a string according to its classification. The
// goal is to leave enough information for an operator reading a
// scrubbed log to recognize "this is the same value across two
// records" while not actually exposing the value.
//
//	Public/Internal: returned unchanged.
//	Confidential:    first char + asterisks (e.g. "Acme Corp" → "A********").
//	                 Length is preserved so substring searches still
//	                 work for incident triage.
//	Restricted:      fixed [REDACTED] string. Length-preservation
//	                 would itself leak information (e.g. exact length
//	                 of an email distinguishes "alice@x" from
//	                 "alice@buildos.dev").
func MaskString(s string, c Class) string {
	switch c {
	case Public, Internal:
		return s
	case Confidential:
		if s == "" {
			return ""
		}
		// One leading char + masked body of the same total length.
		// Operator can correlate "starts with A and is 9 chars" but
		// can't reconstruct.
		if len(s) == 1 {
			return "*"
		}
		return string(s[0]) + strings.Repeat("*", len(s)-1)
	case Restricted:
		if s == "" {
			return ""
		}
		return "[REDACTED]"
	default:
		return "[REDACTED]"
	}
}

// FieldClass is the map of known JSON/struct-field names → their
// classification. Used by ScrubMap and ScrubJSON to apply the right
// masking without per-call configuration.
//
// Keys are lowercase and matched case-insensitively. Add patterns
// here as new fields appear; the goal is for ANY egress wrapper to
// route through this map rather than hand-rolling field lists.
var FieldClass = map[string]Class{
	// Restricted (PII)
	"email":          Restricted,
	"email_address":  Restricted,
	"phone":          Restricted,
	"phone_number":   Restricted,
	"first_name":     Restricted,
	"last_name":      Restricted,
	"display_name":   Restricted,
	"name":           Restricted, // contractor / employee names
	"address":        Restricted,
	"street_address": Restricted,
	"gps_lat":        Restricted,
	"gps_lng":        Restricted,
	"oidc_subject":   Restricted,
	"sub":            Restricted, // JWT sub claim
	"user_sub":       Restricted,
	"ip_address":     Restricted,
	"remote_addr":    Restricted,

	// Restricted (homeowner contact PII — Chunk D client updates). The
	// homeowner's name/email/phone live on projects; the snapshot address on a
	// client_update is recipient_email. NEVER log/audit/serialize to a
	// field_worker (handler/role-gated): a client update is owner/admin only.
	"client_name":     Restricted,
	"client_email":    Restricted,
	"client_phone":    Restricted,
	"recipient_email": Restricted,

	// Restricted (secret material) — must NEVER reach a log/audit/Sentry sink.
	"password":      Restricted,
	"password_hash": Restricted,
	"secret":        Restricted,
	"token":         Restricted,
	"access_token":  Restricted,
	"refresh_token": Restricted,
	"reset_token":   Restricted,
	"token_hash":    Restricted,
	"api_key":       Restricted,
	"apikey":        Restricted,
	"private_key":   Restricted,
	"authorization": Restricted,
	"jwt":           Restricted,
	"vault_key":     Restricted,

	// Confidential (business-sensitive)
	"message":        Confidential, // feedback free text (Phase 0b)
	"triage_note":    Confidential, // feedback triage free text (Phase 0b)
	"vendor_name":    Confidential,
	"invoice_number": Confidential,
	"po_number":      Confidential,
	"project_name":   Confidential,
	"prospect_name":  Confidential,
	"contact_name":   Confidential,
	"company":        Confidential,
	"organization":   Confidential,
	"amount_cents":   Confidential,
	"total_cents":    Confidential,
	"cost_cents":     Confidential,
	"budget_cents":   Confidential,
	"price_cents":    Confidential,

	// Internal (correlation ids — kept clear so triage works)
	"request_id":  Internal,
	"trace_id":    Internal,
	"span_id":     Internal,
	"event_type":  Internal,
	"action":      Internal,
	"resource_id": Internal,
	"org_id":      Internal,
	"id":          Internal,
}

// ClassFor returns the classification for a known field name. Unknown
// fields default to Internal — they're not Public (don't leak by
// default) but not Restricted either (don't over-mask correlation
// ids that happen to share a name pattern). Callers wanting strict
// behavior on unknowns should set their own default.
func ClassFor(field string) Class {
	if c, ok := FieldClass[strings.ToLower(field)]; ok {
		return c
	}
	return Internal
}

// ScrubMap walks a map[string]any and masks values whose key
// classification is at or above the threshold. Recurses into nested
// maps and slices of maps. Non-map / non-slice values that aren't
// strings are passed through unchanged — only string values get
// MaskString applied.
//
// The threshold parameter lets callers tune their scrubbing:
//
//	ScrubMap(m, Restricted) — only redact PII (Sentry-safe)
//	ScrubMap(m, Confidential) — also redact business-sensitive
//	                            (third-party support log dump)
//	ScrubMap(m, Internal) — almost everything (a paranoid export)
//
// Returns a new map; the input is not modified.
func ScrubMap(m map[string]any, threshold Class) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = scrubValue(k, v, threshold)
	}
	return out
}

// scrubValue is the recursive workhorse. Field name + value + threshold
// → masked value of the same shape.
func scrubValue(field string, v any, threshold Class) any {
	cls := ClassFor(field)

	switch val := v.(type) {
	case string:
		if cls.IsAtLeast(threshold) {
			return MaskString(val, cls)
		}
		return val
	case map[string]any:
		// Recurse into nested map; the parent field's class doesn't
		// dictate child classes (the nested keys carry their own).
		return ScrubMap(val, threshold)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			// Slice items don't have field names; pass the parent
			// field through so e.g. a list of phone numbers is
			// masked correctly.
			out[i] = scrubValue(field, item, threshold)
		}
		return out
	default:
		// Numeric, bool, nil — applied only when the FIELD itself
		// classifies as restricted/confidential. e.g. amount_cents
		// is an int64; we replace with a sentinel rather than mask
		// digit-by-digit.
		if cls.IsAtLeast(threshold) {
			switch cls {
			case Restricted:
				return "[REDACTED]"
			case Confidential:
				// Numeric confidential fields collapse to a marker
				// that preserves "this exists" without the value.
				return "[CONFIDENTIAL]"
			}
		}
		return v
	}
}

// ScrubJSON unmarshals JSON, scrubs the result via ScrubMap, and
// re-marshals. Used for raw payload blobs (audit_log.before_state /
// after_state) where the structure is
// dynamic. Returns the input unchanged on parse failure — better
// to ship a maybe-leaky payload than to drop the audit/event
// entirely.
func ScrubJSON(raw []byte, threshold Class) []byte {
	if len(raw) == 0 {
		return raw
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	scrubbed := scrubAny(v, threshold)
	out, err := json.Marshal(scrubbed)
	if err != nil {
		return raw
	}
	return out
}

// scrubAny dispatches on the top-level JSON shape. Top-level scalars
// can't be masked usefully (no field name); pass through. Arrays /
// objects route through scrubValue / ScrubMap.
func scrubAny(v any, threshold Class) any {
	switch val := v.(type) {
	case map[string]any:
		return ScrubMap(val, threshold)
	case []any:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = scrubAny(item, threshold)
		}
		return out
	default:
		return v
	}
}
