package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/futurebuild/futurebuild-os/pkg/ai"
)

// RegisterCommunicationTools registers tools for contacts, notifications, and messaging.
// Uses stub implementations — returns "message queued" until Twilio/SendGrid integration in Sprint 5.
func RegisterCommunicationTools(r *Registry) {
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "send_sms",
			Description: "Send an SMS message to a contact. Returns a queued confirmation. Use draft_message first to compose the message, then send after user approval.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"contact_id":{"type":"string","description":"UUID of the contact to message"},"message":{"type":"string","description":"SMS message body (keep under 160 chars for single SMS)"}},"required":["contact_id","message"]}`),
		},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var params struct {
				ContactID string `json:"contact_id"`
				Message   string `json:"message"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}
			// TODO: wire to real NotificationService (Twilio) in Sprint 5
			return fmt.Sprintf(`{"success":true,"contact_id":"%s","message_length":%d,"status":"queued","message":"SMS queued (stub — Twilio integration pending Sprint 5)"}`,
				params.ContactID, len(params.Message)), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "send_email",
			Description: "Send an email to a recipient. Returns a queued confirmation. Use draft_message first to compose the email, then send after user approval.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"to":{"type":"string","description":"Recipient email address"},"subject":{"type":"string","description":"Email subject line"},"body":{"type":"string","description":"Email body (supports markdown formatting)"}},"required":["to","subject","body"]}`),
		},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			var params struct {
				To      string `json:"to"`
				Subject string `json:"subject"`
				Body    string `json:"body"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}
			// TODO: wire to real NotificationService (SendGrid) in Sprint 5
			return fmt.Sprintf(`{"success":true,"to":"%s","subject":"%s","status":"queued","message":"Email queued (stub — SendGrid integration pending Sprint 5)"}`,
				params.To, params.Subject), nil
		},
	})

	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "draft_message",
			Description: "Draft a curated message (email or SMS) for the user to review before sending. Present the draft to the user and wait for approval before using send_email or send_sms.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"channel":{"type":"string","enum":["email","sms"],"description":"Communication channel"},"to_name":{"type":"string","description":"Recipient name"},"to_address":{"type":"string","description":"Email address or phone number"},"subject":{"type":"string","description":"Email subject (required for email, omit for SMS)"},"body":{"type":"string","description":"The drafted message body"},"context":{"type":"string","description":"Brief explanation of why this message is being sent"}},"required":["channel","to_name","to_address","body","context"]}`),
		},
		Handler: func(_ context.Context, input json.RawMessage) (string, error) {
			// draft_message is a "display" tool — it doesn't execute side effects.
			// It returns the draft back so the chat UI can render it for user review.
			var params struct {
				Channel   string `json:"channel"`
				ToName    string `json:"to_name"`
				ToAddress string `json:"to_address"`
				Subject   string `json:"subject"`
				Body      string `json:"body"`
				Context   string `json:"context"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}
			result := map[string]interface{}{
				"draft":      true,
				"channel":    params.Channel,
				"to_name":    params.ToName,
				"to_address": params.ToAddress,
				"subject":    params.Subject,
				"body":       params.Body,
				"context":    params.Context,
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		},
	})
}
