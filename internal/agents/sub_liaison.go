package agents

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SubLiaisonAgent handles SMS confirmation for subcontractor scheduling.
// P1 — Twilio integration deferred to Sprint 5.
type SubLiaisonAgent struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

// NewSubLiaisonAgent creates a new SubLiaisonAgent (stub).
func NewSubLiaisonAgent(pool *pgxpool.Pool, logger *slog.Logger) *SubLiaisonAgent {
	return &SubLiaisonAgent{pool: pool, logger: logger}
}

// ScanPending scans pending subcontractor approvals and queues SMS.
// Stub — returns nil until Twilio integration in Sprint 5.
func (a *SubLiaisonAgent) ScanPending(ctx context.Context) error {
	a.logger.Info("sub_liaison_scan: stub — Twilio integration pending (Sprint 5)")
	return nil
}
