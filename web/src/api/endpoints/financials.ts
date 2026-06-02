/**
 * Financials endpoints — /api/v1/org/{orgID}/financials/* (internal/api/
 * financials.go). Superintendent+ may read the summary; AR-aging and
 * per-project rollups are owner/admin. The org id comes from the caller's JWT
 * claim (the backend verifies the URL {orgID} matches). All per-currency: the
 * backend never sums across currencies, and neither does the UI.
 */
import { api } from '../client.js';
import { normalizeCents } from '../wire.js';
import type {
  FinancialsSummary,
  ARAgingSnapshot,
  ProjectFinancial,
  CurrencyCode,
} from '../../types/models.js';

function currencyQuery(currency?: CurrencyCode): string {
  return currency ? `?currency=${encodeURIComponent(currency)}` : '';
}

export function getFinancialsSummary(
  orgId: string,
  currency?: CurrencyCode,
): Promise<FinancialsSummary> {
  return api
    .get<FinancialsSummary>(
      `/api/v1/org/${encodeURIComponent(orgId)}/financials/summary${currencyQuery(currency)}`,
    )
    .then((r) => normalizeCents(r));
}

export function getARAging(orgId: string, currency?: CurrencyCode): Promise<ARAgingSnapshot[]> {
  return api
    .get<{
      snapshots: ARAgingSnapshot[];
    }>(`/api/v1/org/${encodeURIComponent(orgId)}/financials/ar-aging${currencyQuery(currency)}`)
    .then((r) => normalizeCents(r.snapshots ?? []));
}

export function getProjectFinancials(
  orgId: string,
  currency?: CurrencyCode,
): Promise<ProjectFinancial[]> {
  return api
    .get<{
      projects: ProjectFinancial[];
    }>(`/api/v1/org/${encodeURIComponent(orgId)}/financials/projects${currencyQuery(currency)}`)
    .then((r) => normalizeCents(r.projects ?? []));
}
