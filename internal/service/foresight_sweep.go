package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/futurebuildai/buildos/internal/agentic"
	"github.com/futurebuildai/buildos/internal/store"
)

// foresightSweepPageSize bounds memory for the cross-org keyset pagination.
const foresightSweepPageSize = 100

// ProcurementRecomputer is the narrow seam for the pre-sweep procurement status
// refresh. *ProcurementService satisfies it (its RecomputeStatuses is the same
// worker-side entrypoint procurement_check uses). The sweep recomputes first
// (R3) so the procurement dimension reads FRESH statuses regardless of River's
// inter-job ordering.
type ProcurementRecomputer interface {
	RecomputeStatuses(ctx context.Context) (int64, error)
}

// ForesightSweepService satisfies worker.ForesightRunner. It owns the cross-org
// fan-out: per project it builds a per-org orchestrator (the AI key resolves
// per-org) via the injected factory and runs RunForesight. One bad project logs
// + continues (never aborts the fleet). The cross-org READ
// (ListActiveAcrossOrgsForSweep) is the single sanctioned ADR-002 system-actor
// exception — worker-only, never behind an HTTP handler.
type ForesightSweepService struct {
	pool         *pgxpool.Pool
	projectStore *store.ProjectStore
	procChecker  ProcurementRecomputer
	newOrch      func(orgID uuid.UUID) *agentic.ForesightOrchestrator
	logger       *slog.Logger
}

// NewForesightSweepService wires the sweep. newOrch is the per-org orchestrator
// factory (mirrors cascadeOrchestratorFactory): it builds a per-org reasoner so
// the Anthropic key resolves per org. A nil logger gets slog.Default().
func NewForesightSweepService(
	pool *pgxpool.Pool,
	projectStore *store.ProjectStore,
	procChecker ProcurementRecomputer,
	newOrch func(orgID uuid.UUID) *agentic.ForesightOrchestrator,
	logger *slog.Logger,
) *ForesightSweepService {
	if logger == nil {
		logger = slog.Default()
	}
	return &ForesightSweepService{
		pool:         pool,
		projectStore: projectStore,
		procChecker:  procChecker,
		newOrch:      newOrch,
		logger:       logger,
	}
}

// RunForesightSweep is the worker.ForesightRunner entrypoint. Sequence (spec §2d):
//  1. RecomputeStatuses first — establishes happens-before so the procurement
//     dimension reads FRESH statuses (R3). Idempotent fleet sweep.
//  2. Keyset-paginate ListActiveAcrossOrgsForSweep across all orgs.
//  3. Per project: build a per-org orchestrator and RunForesight. A per-project
//     error is logged and skipped (never aborts the fleet).
//
// Returns nil unless the recompute or the listing itself errored (those
// River-retry the whole sweep).
func (s *ForesightSweepService) RunForesightSweep(ctx context.Context) error {
	log := s.logger.With(slog.String("flow", "foresight_sweep"))

	// 1. Refresh procurement statuses so the procurement dimension is fresh (R3).
	if _, err := s.procChecker.RecomputeStatuses(ctx); err != nil {
		return fmt.Errorf("foresight sweep: recompute procurement statuses: %w", err)
	}

	var (
		afterID       uuid.UUID // uuid.Nil starts; every real uuid sorts after nil
		projectsSeen  int
		cardsCreated  int
		cardsSkipped  int
		projectErrors int
	)

	for {
		var page []projectRef
		err := pgx.BeginTxFunc(ctx, s.pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
			projects, err := s.projectStore.ListActiveAcrossOrgsForSweep(ctx, tx, foresightSweepPageSize, afterID)
			if err != nil {
				return err
			}
			page = make([]projectRef, 0, len(projects))
			for _, p := range projects {
				page = append(page, projectRef{OrgID: p.OrgID, ID: p.ID})
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("foresight sweep: list active projects: %w", err)
		}
		if len(page) == 0 {
			break
		}

		for _, p := range page {
			projectsSeen++
			afterID = p.ID // advance the keyset cursor

			orch := s.newOrch(p.OrgID)
			res, runErr := orch.RunForesight(ctx, agentic.ForesightInput{
				OrgID:     p.OrgID,
				ProjectID: p.ID,
			})
			if runErr != nil {
				// Per-project isolation: log + continue, never abort the fleet.
				projectErrors++
				log.ErrorContext(ctx, "foresight sweep: project run failed",
					slog.String("org_id", p.OrgID.String()),
					slog.String("project_id", p.ID.String()),
					slog.Any("error", runErr))
				continue
			}
			cardsCreated += res.CardsCreated
			cardsSkipped += res.CardsSkipped
		}

		// A short final page means we've drained the table.
		if len(page) < foresightSweepPageSize {
			break
		}
	}

	log.InfoContext(ctx, "foresight sweep completed",
		slog.Int("projects", projectsSeen),
		slog.Int("cards_created", cardsCreated),
		slog.Int("cards_skipped", cardsSkipped),
		slog.Int("project_errors", projectErrors))
	return nil
}

// projectRef is the minimal (org, project) pair the fan-out loop needs. We copy
// it out of the read tx so each per-project orchestrator run opens its own
// tx(es) rather than holding the listing tx open across N AI calls.
type projectRef struct {
	OrgID uuid.UUID
	ID    uuid.UUID
}
