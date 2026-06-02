/**
 * Audit-log endpoint — GET /api/v1/audit (store/audit.go, migration 008).
 * Owner/admin only (router-gated). Supports an `action_prefix` filter (e.g.
 * `setup.`) plus a `from`/`to` date window. The backend scrubs Restricted-class
 * fields from the before/after JSONB before it leaves the deployment.
 *
 * NB: this read route is a tracked backend gap (FRONTEND_ARCHITECTURE — not yet
 * mounted in router.go). The Activity page degrades gracefully on NOT_FOUND /
 * NOT_IMPLEMENTED rather than hard-failing; this contract is the target shape.
 */
import { api } from '../client.js';
import type { AuditEntry } from '../../types/models.js';

export interface ListAuditParams {
  action_prefix?: string;
  from?: string;
  to?: string;
}

export function listAudit(params: ListAuditParams = {}): Promise<AuditEntry[]> {
  const q = new URLSearchParams();
  if (params.action_prefix) q.set('action_prefix', params.action_prefix);
  if (params.from) q.set('from', params.from);
  if (params.to) q.set('to', params.to);
  q.set('per_page', '200');
  const s = q.toString();
  return api
    .get<{ entries: AuditEntry[] }>(`/api/v1/audit${s ? `?${s}` : ''}`)
    .then((r) => r.entries ?? []);
}
