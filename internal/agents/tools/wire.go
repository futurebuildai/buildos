// Package tools provides a registry of Claude-callable tools. This file provides
// convenience constructors for wiring all tools to real service implementations.
package tools

import (
	"github.com/futurebuild/futurebuild-os/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WireConfig holds all dependencies needed to wire tool implementations.
type WireConfig struct {
	Pool      *pgxpool.Pool
	BudgetSvc *service.BudgetService
	FeedSvc   *service.FeedService
}

// NewWiredRegistry creates a tool registry with all tools wired to real
// service implementations. This is the primary constructor for production use.
//
// Wiring:
//   - Project tools -> pgxpool (raw SQL queries for projects, tasks, procurement, forecast)
//   - Schedule tools -> physics engine (CPM ForwardPass + BackwardPass via pgxpool)
//   - Budget tools -> BudgetService + pgxpool (financial summary, cost estimation)
//   - Communication tools -> PgNotificationService (transactional outbox via pgxpool)
//   - Feed tools -> FeedService (feed card creation, approval cards)
//   - Market tools -> static seasonal model (no external dependency)
func NewWiredRegistry(cfg WireConfig) *Registry {
	r := NewRegistry()

	// Feed tools (wired to FeedService)
	RegisterFeedTools(r, cfg.FeedSvc)

	// Market tools (static data, no DB dependency)
	RegisterMarketTools(r)

	// Project tools wired to raw pgxpool queries
	RegisterProjectToolsWithPool(r, cfg.Pool)

	// Schedule tools wired to physics engine
	engine := NewScheduleEngine(cfg.Pool)
	RegisterScheduleToolsWithEngine(r, engine)

	// Budget tools wired to BudgetService
	RegisterBudgetToolsWithService(r, cfg.BudgetSvc, cfg.Pool)

	// Communication tools with transactional outbox persistence
	notifSvc := NewPgNotificationService(cfg.Pool, cfg.FeedSvc)
	RegisterCommunicationToolsWithService(r, notifSvc)

	return r
}

// NewWiredScheduleEngine creates a ScheduleEngine for use outside the tool registry
// (e.g., by FutureShade skills). Returns both the engine and the adapter that satisfies
// the skills.ScheduleRecalcExecutor interface.
func NewWiredScheduleEngine(pool *pgxpool.Pool) (*ScheduleEngine, *ScheduleRecalcAdapter) {
	engine := NewScheduleEngine(pool)
	adapter := NewScheduleRecalcAdapter(engine)
	return engine, adapter
}
