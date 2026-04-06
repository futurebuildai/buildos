package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/pkg/ai"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationService defines the interface for queuing outbound notifications.
// Implementations persist to the communication_logs table (transactional outbox).
// Actual delivery (Twilio/SendGrid) is handled by the worker process in Sprint 5.
type NotificationService interface {
	QueueSMS(ctx context.Context, orgID uuid.UUID, projectID *uuid.UUID, contactID, message string) (uuid.UUID, error)
	QueueEmail(ctx context.Context, orgID uuid.UUID, projectID *uuid.UUID, to, subject, body string) (uuid.UUID, error)
}

// PgNotificationService implements NotificationService by writing to the
// communication_logs table (transactional outbox pattern). Messages are persisted
// with status "queued" and picked up by the delivery worker for actual sending.
type PgNotificationService struct {
	pool       *pgxpool.Pool
	feedWriter FeedCardWriter
}

// NewPgNotificationService creates a notification service backed by PostgreSQL.
func NewPgNotificationService(pool *pgxpool.Pool, feedWriter FeedCardWriter) *PgNotificationService {
	return &PgNotificationService{pool: pool, feedWriter: feedWriter}
}

// QueueSMS persists an SMS message to communication_logs and creates an audit feed card.
func (s *PgNotificationService) QueueSMS(ctx context.Context, orgID uuid.UUID, projectID *uuid.UUID, contactID, message string) (uuid.UUID, error) {
	logID := uuid.New()
	idempotencyKey := uuid.New()

	_, err := s.pool.Exec(ctx, `
		INSERT INTO communication_logs (
			id, project_id, contact_name, message_type,
			message_body, status, idempotency_key
		) VALUES ($1, $2, $3, 'sms', $4, 'queued', $5)`,
		logID, projectID, contactID, message, idempotencyKey,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("queue SMS to communication_logs: %w", err)
	}

	slog.Info("SMS queued in outbox",
		"log_id", logID,
		"contact_id", contactID,
		"message_length", len(message),
	)

	// Create audit feed card so user can see the queued notification
	if s.feedWriter != nil {
		agentSource := "ChatAgent"
		card := &models.FeedCard{
			OrgID:       orgID,
			ProjectID:   projectID,
			CardType:    "notification_queued",
			Title:       fmt.Sprintf("SMS queued to %s", contactID),
			Body:        fmt.Sprintf("Message (%d chars) queued for delivery. Delivery worker will send via Twilio.", len(message)),
			Priority:    models.PriorityLow,
			AgentSource: &agentSource,
			Status:      models.FeedStatusActive,
		}
		if _, err := s.feedWriter.CreateCard(ctx, card); err != nil {
			slog.Warn("failed to create SMS audit feed card", "error", err)
			// Non-fatal: the SMS is already queued in the outbox
		}
	}

	return logID, nil
}

// QueueEmail persists an email message to communication_logs and creates an audit feed card.
func (s *PgNotificationService) QueueEmail(ctx context.Context, orgID uuid.UUID, projectID *uuid.UUID, to, subject, body string) (uuid.UUID, error) {
	logID := uuid.New()
	idempotencyKey := uuid.New()

	// Combine subject and body into message_body for the outbox record.
	messageBody := fmt.Sprintf("Subject: %s\n\n%s", subject, body)

	_, err := s.pool.Exec(ctx, `
		INSERT INTO communication_logs (
			id, project_id, contact_name, message_type,
			message_body, status, idempotency_key
		) VALUES ($1, $2, $3, 'email', $4, 'queued', $5)`,
		logID, projectID, to, messageBody, idempotencyKey,
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("queue email to communication_logs: %w", err)
	}

	slog.Info("email queued in outbox",
		"log_id", logID,
		"to", to,
		"subject", subject,
	)

	// Create audit feed card
	if s.feedWriter != nil {
		agentSource := "ChatAgent"
		card := &models.FeedCard{
			OrgID:       orgID,
			ProjectID:   projectID,
			CardType:    "notification_queued",
			Title:       fmt.Sprintf("Email queued to %s", to),
			Body:        fmt.Sprintf("Subject: %s — queued for delivery. Delivery worker will send via SendGrid.", subject),
			Priority:    models.PriorityLow,
			AgentSource: &agentSource,
			Status:      models.FeedStatusActive,
		}
		if _, err := s.feedWriter.CreateCard(ctx, card); err != nil {
			slog.Warn("failed to create email audit feed card", "error", err)
		}
	}

	return logID, nil
}

// RegisterCommunicationTools registers tools for contacts, notifications, and messaging.
// When notifSvc is nil, falls back to stub implementations.
func RegisterCommunicationTools(r *Registry) {
	RegisterCommunicationToolsWithService(r, nil)
}

// RegisterCommunicationToolsWithService registers communication tools wired to a real
// NotificationService. Pass nil notifSvc to get stub behavior (backward compatible).
func RegisterCommunicationToolsWithService(r *Registry, notifSvc NotificationService) {
	r.Register(Tool{
		Definition: ai.ToolDefinition{
			Name:        "send_sms",
			Description: "Send an SMS message to a contact. Returns a queued confirmation. Use draft_message first to compose the message, then send after user approval.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"contact_id":{"type":"string","description":"UUID of the contact to message"},"message":{"type":"string","description":"SMS message body (keep under 160 chars for single SMS)"}},"required":["contact_id","message"]}`),
		},
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var params struct {
				ContactID string `json:"contact_id"`
				Message   string `json:"message"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}

			if notifSvc != nil {
				scope := MustGetScope(ctx)
				logID, err := notifSvc.QueueSMS(ctx, scope.OrgID, &scope.ProjectID, params.ContactID, params.Message)
				if err != nil {
					return "", fmt.Errorf("queue SMS: %w", err)
				}
				result := map[string]interface{}{
					"success":        true,
					"contact_id":     params.ContactID,
					"message_length": len(params.Message),
					"status":         "queued",
					"log_id":         logID.String(),
					"message":        "SMS persisted to outbox — delivery worker will send via Twilio",
				}
				b, _ := json.Marshal(result)
				return string(b), nil
			}

			// Stub fallback
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
		Handler: func(ctx context.Context, input json.RawMessage) (string, error) {
			var params struct {
				To      string `json:"to"`
				Subject string `json:"subject"`
				Body    string `json:"body"`
			}
			if err := json.Unmarshal(input, &params); err != nil {
				return "", fmt.Errorf("parse input: %w", err)
			}

			if notifSvc != nil {
				scope := MustGetScope(ctx)
				logID, err := notifSvc.QueueEmail(ctx, scope.OrgID, &scope.ProjectID, params.To, params.Subject, params.Body)
				if err != nil {
					return "", fmt.Errorf("queue email: %w", err)
				}
				result := map[string]interface{}{
					"success": true,
					"to":      params.To,
					"subject": params.Subject,
					"status":  "queued",
					"log_id":  logID.String(),
					"message": "Email persisted to outbox — delivery worker will send via SendGrid",
				}
				b, _ := json.Marshal(result)
				return string(b), nil
			}

			// Stub fallback
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
				"created_at": time.Now().UTC().Format(time.RFC3339),
			}
			b, _ := json.Marshal(result)
			return string(b), nil
		},
	})
}
