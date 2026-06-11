/**
 * Conversational assistant endpoint — POST /api/v1/agents/chat
 * (internal/api/assistant.go). A bounded Claude tool-use loop over ~8 read-only
 * ERP tools. Native AI: a missing key surfaces as 503 SERVICE_UNAVAILABLE (the
 * canonical "AI off" signal, §9); an admin kill-switch is 403 CAPABILITY_DISABLED.
 *
 * STATELESS server-side: the client owns the conversation and resends it (capped:
 * ≤10 turns, ≤24k total chars, ≤8k per message — trim BEFORE calling so the
 * server's hard 400s never trip in normal use). Identity (org/role/sub) is read
 * from JWT claims server-side — NEVER pass it in the body.
 */
import { api } from '../client.js';
import type { ChatTurn, ChatResponse } from '../../types/models.js';

export function sendChat(message: string, history: ChatTurn[]): Promise<ChatResponse> {
  return api.post<ChatResponse>('/api/v1/agents/chat', { message, history });
}
