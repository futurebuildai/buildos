/**
 * Integrations (BYOK vault) endpoints — /api/v1/integrations
 * (internal/api/integrations.go MountIntegrationRoutes). Admin-gated. Only
 * non-secret metadata ever crosses the wire — the encrypted key bytes never
 * leave the deployment, so there is no "read secret" call by design (DSC §5.2).
 */
import { api } from '../client.js';
import type { IntegrationCredential } from '../../types/models.js';

export function listIntegrations(): Promise<IntegrationCredential[]> {
  return api
    .get<{ integrations: IntegrationCredential[] }>('/api/v1/integrations')
    .then((r) => r.integrations);
}

export interface SetCredentialInput {
  label: string;
  key: string;
}
export function setCredential(
  provider: string,
  input: SetCredentialInput,
): Promise<IntegrationCredential> {
  return api
    .put<{
      integration: IntegrationCredential;
    }>(`/api/v1/integrations/${encodeURIComponent(provider)}`, input)
    .then((r) => r.integration);
}

export function deleteCredential(provider: string): Promise<void> {
  return api.delete<void>(`/api/v1/integrations/${encodeURIComponent(provider)}`);
}
