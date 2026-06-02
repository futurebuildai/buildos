/**
 * Daily Briefing endpoint — POST /api/v1/agents/daily-briefing
 * (internal/api/agents.go). Native AI: mounts only when AgentsService is wired and
 * the Anthropic key is configured; a missing key surfaces as 503 SERVICE_UNAVAILABLE
 * (the canonical "AI off" signal, §9). The call is a synchronous one-shot that hits
 * the model — expect 1–3s latency. Not persisted as a feed card; cache client-side.
 */
import { api } from '../client.js';
import type { DailyBriefing } from '../../types/models.js';

export function getDailyBriefing(): Promise<DailyBriefing> {
  return api
    .post<{ briefing: DailyBriefing }>('/api/v1/agents/daily-briefing')
    .then((r) => r.briefing);
}
