/**
 * Public share-link endpoints — internal/api/share_link.go (Chunk E of
 * DAILY_REPORTS_CLIENT_UPDATES). The owner/admin controls on a SENT client
 * update: mint a token-gated, read-only homeowner progress page at /p/{token},
 * list the links (no cleartext), and revoke. ALL routes are owner/admin
 * (router-gated — external comms trust, §9-1).
 *
 * SECURITY: the cleartext token is shown EXACTLY ONCE in CreateShareLinkResponse.url
 * at create (it is the /p/<token> URL the operator emails). It is never stored
 * and never returned again — list/get responses carry only the link record.
 */
import { api } from '../client.js';
import type { CreateShareLinkResponse, ShareLink } from '../../types/models.js';

/**
 * POST — mint a public link for a SENT client update (owner/admin). Returns the
 * one-time URL + the link record. Throws ApiError code UPDATE_NOT_SENT (422)
 * when the update has not been sent. ttlDays is optional (server default 30d,
 * capped at 365d).
 */
export function createShareLink(
  clientUpdateId: string,
  ttlDays?: number,
): Promise<CreateShareLinkResponse> {
  const body = typeof ttlDays === 'number' && ttlDays > 0 ? { ttl_days: ttlDays } : {};
  return api.post<CreateShareLinkResponse>(
    `/api/v1/client-updates/${encodeURIComponent(clientUpdateId)}/share-links`,
    body,
  );
}

/** GET — a client update's share links (active/expired/revoked). No cleartext. */
export function listShareLinks(clientUpdateId: string): Promise<ShareLink[]> {
  return api
    .get<ShareLink[]>(`/api/v1/client-updates/${encodeURIComponent(clientUpdateId)}/share-links`)
    .then((r) => r ?? []);
}

/** DELETE — revoke a share link (owner/admin). After revoke /p/{token} 404s. */
export function revokeShareLink(linkId: string): Promise<void> {
  return api
    .delete<void>(`/api/v1/share-links/${encodeURIComponent(linkId)}`)
    .then(() => undefined);
}
