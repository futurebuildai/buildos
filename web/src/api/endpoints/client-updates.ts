/**
 * Client Update endpoints — internal/api/client_update.go (Chunk D of
 * DAILY_REPORTS_CLIENT_UPDATES). The human-in-the-loop composer: create a draft
 * from a date's AI draft, edit the client-safe subject/body + curate photos,
 * then SEND via the existing Resend mailer. ALL routes are owner/admin
 * (router-gated — external comms trust, §9-1).
 *
 * The send path surfaces failure clearly so the operator KNOWS it did not go
 * out (NOT best-effort): 422 NO_CLIENT_CONTACT (project has no client email),
 * 422 MAILER_UNCONFIGURED (no Resend key set), 409 ALREADY_SENT, 503
 * SERVICE_UNAVAILABLE (no AI key to draft). recipient_email is never returned.
 */
import { api, request } from '../client.js';
import type { ClientUpdate } from '../../types/models.js';

/** POST — create a draft from a date's redacted AI draft (owner/admin). 503 when AI is off. */
export function createClientUpdate(projectId: string, reportDate: string): Promise<ClientUpdate> {
  return api.post<ClientUpdate>(
    `/api/v1/projects/${encodeURIComponent(projectId)}/client-updates`,
    { report_date: reportDate },
  );
}

/** GET — a project's client-update history (newest first). */
export function listClientUpdates(projectId: string): Promise<ClientUpdate[]> {
  return api
    .get<ClientUpdate[]>(`/api/v1/projects/${encodeURIComponent(projectId)}/client-updates`)
    .then((r) => r ?? []);
}

/** GET — one client update by id. */
export function getClientUpdate(id: string): Promise<ClientUpdate> {
  return api.get<ClientUpdate>(`/api/v1/client-updates/${encodeURIComponent(id)}`);
}

export interface UpdateClientUpdateBody {
  subject: string;
  edited_body: string;
  /** Operator-curated subset of the period's confirmed photos (a redaction control). */
  photo_asset_ids?: string[];
}

/** PATCH — apply the operator edit to a draft (owner/admin). 409 ALREADY_SENT on a sent row. */
export function updateClientUpdate(
  id: string,
  body: UpdateClientUpdateBody,
): Promise<ClientUpdate> {
  return request<ClientUpdate>(`/api/v1/client-updates/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    body,
  });
}

/**
 * POST — send via Resend (owner/admin). Resolves to the sent row on success.
 * Throws ApiError with code NO_CLIENT_CONTACT / MAILER_UNCONFIGURED (422),
 * ALREADY_SENT (409), or SEND_FAILED (502) so the UI tells the operator it did
 * NOT go out.
 */
export function sendClientUpdate(id: string): Promise<ClientUpdate> {
  return api.post<ClientUpdate>(`/api/v1/client-updates/${encodeURIComponent(id)}/send`);
}
