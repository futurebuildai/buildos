/**
 * Capabilities endpoint (FRONTEND_ARCHITECTURE §6.2).
 *
 * GET /api/v1/capabilities is a backend gap (§3.5) — NOT yet mounted. The
 * capability store catches the thrown ApiError and applies the "assume-on"
 * fallback so a capabilities outage never hard-bricks the UI; AI affordances
 * then degrade reactively on a 503 AI_UNCONFIGURED.
 */
import { api } from '../client.js';
import type { Capabilities } from '../../types/models.js';

export function getCapabilities(): Promise<Capabilities> {
  return api.get<Capabilities>('/api/v1/capabilities');
}
