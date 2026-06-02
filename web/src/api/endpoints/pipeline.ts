/**
 * Pipeline (CRM) endpoints — /api/v1/org/{orgID}/pipeline/* (internal/api/
 * pipeline.go). Superintendent+ read; owner/admin advance/lose/estimate/permit.
 * Transitions are forward-only and enforced server-side (INVALID_TRANSITION 409).
 * Money (estimate totals, permit fees) is per-currency and string-coerced.
 */
import { api } from '../client.js';
import { normalizeCents } from '../wire.js';
import type {
  Prospect,
  ProspectWithDetails,
  PipelineAnalyticsRow,
  PipelineStage,
} from '../../types/models.js';

export interface ListProspectsParams {
  stage?: PipelineStage;
  page?: number;
  per_page?: number;
}

function query(params: ListProspectsParams): string {
  const q = new URLSearchParams();
  if (params.stage) q.set('stage', params.stage);
  if (params.page) q.set('page', String(params.page));
  if (params.per_page) q.set('per_page', String(params.per_page));
  const s = q.toString();
  return s ? `?${s}` : '';
}

export function listProspects(
  orgId: string,
  params: ListProspectsParams = {},
): Promise<Prospect[]> {
  return api
    .get<{
      prospects: Prospect[];
    }>(`/api/v1/org/${encodeURIComponent(orgId)}/pipeline/prospects${query(params)}`)
    .then((r) => r.prospects ?? []);
}

export function getProspect(orgId: string, prospectId: string): Promise<ProspectWithDetails> {
  return api
    .get<ProspectWithDetails>(
      `/api/v1/org/${encodeURIComponent(orgId)}/pipeline/prospects/${encodeURIComponent(prospectId)}`,
    )
    .then((r) => normalizeCents(r));
}

export function getPipelineAnalytics(orgId: string): Promise<PipelineAnalyticsRow[]> {
  return api
    .get<{
      analytics: PipelineAnalyticsRow[];
    }>(`/api/v1/org/${encodeURIComponent(orgId)}/pipeline/analytics`)
    .then((r) => normalizeCents(r.analytics ?? []));
}

export interface AdvanceProspectInput {
  target_stage: PipelineStage;
  permit_issued_date?: string;
}
export function advanceProspect(
  orgId: string,
  prospectId: string,
  input: AdvanceProspectInput,
): Promise<Prospect> {
  return api
    .post<{
      prospect: Prospect;
    }>(
      `/api/v1/org/${encodeURIComponent(orgId)}/pipeline/prospects/${encodeURIComponent(prospectId)}/advance`,
      input,
    )
    .then((r) => r.prospect);
}

export function loseProspect(orgId: string, prospectId: string, reason: string): Promise<Prospect> {
  return api
    .post<{
      prospect: Prospect;
    }>(
      `/api/v1/org/${encodeURIComponent(orgId)}/pipeline/prospects/${encodeURIComponent(prospectId)}/lose`,
      { reason },
    )
    .then((r) => r.prospect);
}
