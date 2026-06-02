/**
 * Procurement endpoints — /api/v1/projects/{projectID}/procurement/*
 * (internal/api/procurement.go). Read is any-authenticated; create/update are
 * owner/admin; request-review is min-superintendent. Status flows
 * OK → WARNING → CRITICAL → ORDERED; humans transition to ORDERED via update.
 * In the native model `request-review` writes a local `vendor_review_requested`
 * feed card (A2A removed) — see §5.3.
 */
import { api } from '../client.js';
import { normalizeCents } from '../wire.js';
import type { ProcurementItem, ProcurementStatus, CurrencyCode } from '../../types/models.js';

export function listProcurement(
  projectId: string,
  statuses?: ProcurementStatus[],
): Promise<ProcurementItem[]> {
  const q = statuses && statuses.length ? `?status=${statuses.join(',')}` : '';
  return api
    .get<{
      items: ProcurementItem[];
    }>(`/api/v1/projects/${encodeURIComponent(projectId)}/procurement${q}`)
    .then((r) => normalizeCents(r.items ?? []));
}

export interface MarkOrderedInput {
  status: 'ORDERED';
  po_number: string;
  /** RFC3339 or YYYY-MM-DD. */
  ordered_at: string;
}
export function updateProcurementItem(
  projectId: string,
  itemId: string,
  input: MarkOrderedInput,
): Promise<ProcurementItem> {
  return api
    .put<{
      item: ProcurementItem;
    }>(
      `/api/v1/projects/${encodeURIComponent(projectId)}/procurement/${encodeURIComponent(itemId)}`,
      input,
    )
    .then((r) => normalizeCents(r.item));
}

export interface RequestReviewInput {
  vendor: string;
  total_cents: string;
  currency_code: CurrencyCode;
  rfq_id?: string;
  reasoning?: string;
}
export function requestVendorReview(
  projectId: string,
  itemId: string,
  input: RequestReviewInput,
): Promise<void> {
  return api
    .post<unknown>(
      `/api/v1/projects/${encodeURIComponent(projectId)}/procurement/${encodeURIComponent(
        itemId,
      )}/request-review`,
      input,
    )
    .then(() => undefined);
}
