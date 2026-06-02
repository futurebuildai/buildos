/**
 * Feed endpoints — /api/v1/feed/* (internal/api/feed.go). All roles; the backend
 * targets cards by target_user_id / target_role, so each caller only ever sees its
 * own cards (no client-side gating). Cards arrive paginated; v1 requests a large
 * per_page since the envelope pagination block is not surfaced by the client.
 */
import { api } from '../client.js';
import type { FeedCard, FeedBackendPriority, FeedCardStatus } from '../../types/models.js';

export interface ListFeedParams {
  status?: FeedCardStatus;
  priority?: FeedBackendPriority;
  page?: number;
  per_page?: number;
}
export function listFeed(params: ListFeedParams = {}): Promise<FeedCard[]> {
  const q = new URLSearchParams();
  q.set('status', params.status ?? 'active');
  if (params.priority) q.set('priority', params.priority);
  q.set('page', String(params.page ?? 1));
  q.set('per_page', String(params.per_page ?? 100));
  return api.get<{ cards: FeedCard[] }>(`/api/v1/feed?${q.toString()}`).then((r) => r.cards ?? []);
}

export interface FeedActionInput {
  action_type: string;
  payload?: Record<string, unknown>;
}
export function actionFeedCard(cardId: string, input: FeedActionInput): Promise<void> {
  return api
    .post<unknown>(`/api/v1/feed/${encodeURIComponent(cardId)}/action`, input)
    .then(() => undefined);
}

export function dismissFeedCard(cardId: string): Promise<void> {
  return api
    .post<unknown>(`/api/v1/feed/${encodeURIComponent(cardId)}/dismiss`)
    .then(() => undefined);
}
