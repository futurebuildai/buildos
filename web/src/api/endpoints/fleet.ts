/**
 * Fleet endpoints — /api/v1/org/{orgID}/fleet/* (internal/api/fleet.go).
 * Superintendent+ may list assets and allocate; owner/admin may create assets.
 * Allocation enforces a GiST no-overlap constraint server-side — an overlapping
 * booking surfaces as 409 CONFLICT, which the page handles inline.
 */
import { api } from '../client.js';
import type { FleetAsset, EquipmentAllocation, FleetAssetStatus } from '../../types/models.js';

export function listFleetAssets(
  orgId: string,
  statuses?: FleetAssetStatus[],
): Promise<FleetAsset[]> {
  const q = statuses && statuses.length ? `?status=${statuses.join(',')}` : '';
  return api
    .get<{ assets: FleetAsset[] }>(`/api/v1/org/${encodeURIComponent(orgId)}/fleet${q}`)
    .then((r) => r.assets ?? []);
}

export interface AllocateAssetInput {
  project_id: string;
  /** RFC3339 or YYYY-MM-DD. */
  start_date: string;
  end_date: string;
}
export function allocateAsset(
  orgId: string,
  assetId: string,
  input: AllocateAssetInput,
): Promise<EquipmentAllocation> {
  return api
    .post<{
      allocation: EquipmentAllocation;
    }>(
      `/api/v1/org/${encodeURIComponent(orgId)}/fleet/${encodeURIComponent(assetId)}/allocate`,
      input,
    )
    .then((r) => r.allocation);
}
