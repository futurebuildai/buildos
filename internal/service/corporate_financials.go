package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/futurebuild/futurebuild-os/internal/models"
	"github.com/futurebuild/futurebuild-os/internal/store"
)

// CorporateFinancialsService handles org-level financial queries and rollups.
type CorporateFinancialsService struct {
	store *store.FinancialStore
}

// NewCorporateFinancialsService creates a new CorporateFinancialsService.
func NewCorporateFinancialsService(s *store.FinancialStore) *CorporateFinancialsService {
	return &CorporateFinancialsService{store: s}
}

// FinancialSummary is the response for the corporate summary endpoint.
type FinancialSummary struct {
	CorporateBudget *models.CorporateBudget `json:"corporate_budget,omitempty"`
	LatestARAging   *models.ARAgingSnapshot  `json:"latest_ar_aging,omitempty"`
}

// Summary returns the latest corporate budget rollup + AR aging for an org.
func (svc *CorporateFinancialsService) Summary(ctx context.Context, orgID uuid.UUID, currencyCode string) (*FinancialSummary, error) {
	if currencyCode == "" {
		currencyCode = "USD"
	}
	if !SupportedCurrencies[currencyCode] {
		return nil, ErrInvalidCurrency
	}

	now := time.Now()
	year, quarter := now.Year(), quarterOf(now)

	budget, err := svc.store.GetCorporateBudget(ctx, orgID, year, quarter, currencyCode)
	if err != nil {
		budget = nil // not found is OK — might not have rolled up yet
	}

	aging, err := svc.store.LatestARAgingByOrg(ctx, orgID, currencyCode)
	if err != nil {
		aging = nil // might not have any snapshots yet
	}

	return &FinancialSummary{
		CorporateBudget: budget,
		LatestARAging:   aging,
	}, nil
}

// ProjectFinancials returns per-project financial summaries for an org.
func (svc *CorporateFinancialsService) ProjectFinancials(ctx context.Context, orgID uuid.UUID, currencyCode string) ([]models.ProjectFinancialSummary, error) {
	if currencyCode == "" {
		currencyCode = "USD"
	}
	if !SupportedCurrencies[currencyCode] {
		return nil, ErrInvalidCurrency
	}
	return svc.store.ProjectFinancialSummaries(ctx, orgID, currencyCode)
}

// RunCorporateRollup aggregates project budgets into the corporate_budgets table
// for the current quarter. Called by the River background worker.
func (svc *CorporateFinancialsService) RunCorporateRollup(ctx context.Context, orgID uuid.UUID) error {
	now := time.Now()
	year, quarter := now.Year(), quarterOf(now)

	for _, cc := range []string{"USD", "CAD"} {
		estimated, committed, actual, count, err := svc.store.RollupProjectBudgets(ctx, orgID, cc)
		if err != nil {
			return fmt.Errorf("rolling up %s: %w", cc, err)
		}
		if count == 0 {
			continue // no projects in this currency
		}

		cb := &models.CorporateBudget{
			OrgID:               orgID,
			FiscalYear:          year,
			Quarter:             quarter,
			CurrencyCode:        cc,
			TotalEstimatedCents: estimated,
			TotalCommittedCents: committed,
			TotalActualCents:    actual,
			ProjectCount:        count,
		}
		if err := svc.store.UpsertCorporateBudget(ctx, cb); err != nil {
			return fmt.Errorf("upserting corporate budget for %s: %w", cc, err)
		}
	}

	return nil
}

func quarterOf(t time.Time) int {
	return (int(t.Month())-1)/3 + 1
}
