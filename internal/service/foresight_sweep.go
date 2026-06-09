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
	config       agentic.ConfigResolver // per-org enabled + foresight tuning (Phase 3a)
	newOrch      func(orgID uuid.UUID) *agentic.ForesightOrchestrator
	logger       *slog.Logger
}

// NewForesightSweepService wires the sweep. newOrch is the per-org orchestrator
// factory (mirrors cascadeOrchestratorFactory): it builds a per-org reasoner so
// the Anthropic key resolves per org. config resolves the per-org enabled flag +
// foresight tuning ONCE per org at the fan-out boundary (Phase 3a). A nil config
// means enabled-with-default for every org. A nil logger gets slog.Default().
func NewForesightSweepService(
	pool *pgxpool.Pool,
	projectStore *store.ProjectStore,
	procChecker ProcurementRecomputer,
	config agentic.ConfigResolver,
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
		config:       config,
		newOrch:      newOrch,
		logger:       logger,
	}
}

// orgForesightConfig is the per-org resolved foresight config, memoized for the
// duration of one sweep so a many-project org resolves config ONCE.
type orgForesightConfig struct {
	enabled bool
	tuning  agentic.ForesightTuning
}

// resolveOrgConfig returns the org's foresight config, resolving (and parsing
// tuning) on the first sighting of an org and memoizing thereafter. A nil
// resolver yields enabled-with-default. A resolver read error is returned so the
// caller fails the whole sweep retryably (River retries) rather than swallowing
// it into the per-project skip bucket.
func (s *ForesightSweepService) resolveOrgConfig(ctx context.Context, memo map[uuid.UUID]orgForesightConfig, orgID uuid.UUID) (orgForesightConfig, error) {
	if fc, ok := memo[orgID]; ok {
		return fc, nil
	}
	var fc orgForesightConfig
	if s.config == nil {
		fc = orgForesightConfig{enabled: true, tuning: agentic.DefaultForesightTuning()}
	} else {
		cc, err := s.config.Resolve(ctx, orgID, agentic.Foresight)
		if err != nil {
			return orgForesightConfig{}, err
		}
		fc = orgForesightConfig{enabled: cc.Enabled, tuning: agentic.ParseForesightTuning(cc.Config)}
	}
	memo[orgID] = fc
	if !fc.enabled {
		// Log ONCE per org (memoization guarantees first-sighting only) so an
		// accidental org-wide disable is visible — zero cards + zero errors
		// otherwise looks identical to a quiet fleet.
		s.logger.InfoContext(ctx, "foresight sweep: capability disabled for org",
			slog.String("org_id", orgID.String()),
			slog.String("reason", "capability_disabled"))
	}
	return fc, nil
}

// RunForesightSweep is the worker.ForesightRunner entrypoint. Sequence (spec §2d):
//  1. RecomputeStatuses first — establishes happens-before so the procurement
//     dimension reads FRESH statuses (R3). Idempotent fleet sweep.
//  2. Keyset-paginate ListActiveAcrossOrgsForSweep across all orgs.
//  3. Per project: resolve the org's config ONCE (memoized; Phase 3a), skip the
//     org entirely when foresight is disabled, else build a per-org orchestrator
//     and RunForesight with the org's tuning. A per-project run error is logged
//     and skipped (never aborts the fleet); a CONFIG-resolve error fails the
//     whole sweep (retryable) — a kill-switch read failure must not be swallowed.
//
// Returns nil unless the recompute, the listing, or a config resolve errored
// (those River-retry the whole sweep).
func (s *ForesightSweepService) RunForesightSweep(ctx context.Context) error {
	log := s.logger.With(slog.String("flow", "foresight_sweep"))

	// 1. Refresh procurement statuses so the procurement dimension is fresh (R3).
	if _, err := s.procChecker.RecomputeStatuses(ctx); err != nil {
		return fmt.Errorf("foresight sweep: recompute procurement statuses: %w", err)
	}

	var (
		afterID         uuid.UUID // uuid.Nil starts; every real uuid sorts after nil
		projectsSeen    int
		risksFound      int
		cardsCreated    int
		cardsSkipped    int
		projectErrors   int
		projectsSkipped int // skipped because their org disabled foresight
	)
	cfgByOrg := make(map[uuid.UUID]orgForesightConfig) // per-sweep config memo (one resolve per org)

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

			// Resolve the org's foresight config once (memoized). A read error
			// fails the whole sweep retryably — a disable kill-switch read
			// failure must NOT fall into the per-project skip bucket below.
			fc, cErr := s.resolveOrgConfig(ctx, cfgByOrg, p.OrgID)
			if cErr != nil {
				return fmt.Errorf("foresight sweep: resolve config for org %s: %w", p.OrgID, cErr)
			}
			if !fc.enabled {
				projectsSkipped++
				continue
			}

			orch := s.newOrch(p.OrgID)
			res, runErr := orch.RunForesight(ctx, agentic.ForesightInput{
				OrgID:     p.OrgID,
				ProjectID: p.ID,
			}, fc.tuning)
			if runErr != nil {
				// Per-project isolation: log + continue, never abort the fleet.
				projectErrors++
				log.ErrorContext(ctx, "foresight sweep: project run failed",
					slog.String("org_id", p.OrgID.String()),
					slog.String("project_id", p.ID.String()),
					slog.Any("error", runErr))
				continue
			}
			risksFound += res.Risks
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
		slog.Int("projects_skipped_disabled", projectsSkipped),
		slog.Int("risks", risksFound),
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
