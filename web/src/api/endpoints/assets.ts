/**
 * Asset (object-storage) endpoints — internal/api/assets.go + field.go (Chunks
 * A/B of DAILY_REPORTS_CLIENT_UPDATES). The operator console uploads a jobsite
 * photo with the presign → direct-PUT-to-R2 → confirm flow (bytes never transit
 * the Go server), then links the confirmed asset to a day's daily log so the
 * daily-report photo strip resolves it.
 *
 * All routes 503 STORAGE_UNAVAILABLE when object storage is unconfigured for the
 * fork — the UI gates the affordance on capabilities.storage_configured and the
 * upload flow surfaces a clear "object storage not configured" state on a 503.
 */
import { api, request } from '../client.js';
import type { Asset, DailyLog } from '../../types/models.js';

/** 201 payload from a presign request: where + how to PUT the bytes. */
export interface PresignResult {
  asset_id: string;
  upload_url: string;
  signed_headers: Record<string, string>;
  expires_at: string;
}

/**
 * Operator presign (superintendent+): create a pending asset row for a project
 * and get a short-lived presigned PUT URL. POST
 * /api/v1/projects/{projectID}/assets/presign-put.
 */
export function presignProjectUpload(
  projectId: string,
  contentType: string,
  byteSize: number,
): Promise<PresignResult> {
  return api.post<PresignResult>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/assets/presign-put`,
    { content_type: contentType, byte_size: byteSize },
  );
}

/**
 * PUT the raw bytes DIRECT to R2 with the signed headers. This is NOT a
 * BuildOS API call (no envelope, no Bearer token) — it goes straight to the
 * object store's presigned URL. Throws on a non-2xx response.
 */
export async function putToSignedUrl(
  uploadUrl: string,
  signedHeaders: Record<string, string>,
  body: Blob,
): Promise<void> {
  const res = await fetch(uploadUrl, { method: 'PUT', headers: signedHeaders, body });
  if (!res.ok) {
    throw new Error(`upload failed: ${res.status} ${res.statusText}`);
  }
}

/** Confirm a PUT succeeded (pending → ready). POST /api/v1/assets/{id}/confirm. */
export function confirmAsset(assetId: string): Promise<Asset> {
  return request<Asset>(`/api/v1/assets/${encodeURIComponent(assetId)}/confirm`, {
    method: 'POST',
    body: {},
  });
}

/**
 * Link confirmed assets to a day's daily log (superintendent+). POST
 * /api/v1/projects/{projectID}/daily-reports/{date}/photos. 404 when no daily
 * log exists for that day; 400 INVALID_PHOTO_ASSET on a non-ready/foreign id.
 */
export function linkPhotosToDailyLog(
  projectId: string,
  date: string,
  assetIds: string[],
): Promise<DailyLog> {
  return api.post<DailyLog>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/daily-reports/${encodeURIComponent(date)}/photos`,
    { asset_ids: assetIds },
  );
}

/**
 * Full operator upload flow for one file: presign → PUT → confirm. Returns the
 * confirmed asset id, ready to be linked to a daily log. Throws on any step
 * (the caller maps a 503 to the disabled state).
 */
export async function uploadProjectPhoto(projectId: string, file: File): Promise<string> {
  const presigned = await presignProjectUpload(projectId, file.type, file.size);
  await putToSignedUrl(presigned.upload_url, presigned.signed_headers, file);
  await confirmAsset(presigned.asset_id);
  return presigned.asset_id;
}
