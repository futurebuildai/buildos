/**
 * Feedback endpoint — POST /api/v1/feedback (Phase 0b).
 *
 * One write surface: any authenticated role can file feedback. The body
 * carries the user's text plus an auto-captured `context` object (route,
 * role, app_version, user_agent, viewport — all strings; the server caps
 * the serialized object at 4096 bytes). The server enforces 1..4000 chars
 * on `message` after trim → `VALIDATION_ERROR` (400); submissions are
 * rate-limited → `RATE_LIMITED` (429). Success is a 201 with the persisted
 * row under `{ feedback }`.
 */
import { api } from '../client.js';
import type { Feedback, FeedbackCategory, FeedbackContext } from '../../types/models.js';

export interface SubmitFeedbackInput {
  category: FeedbackCategory;
  /** 1..4000 chars after trim (server-enforced). */
  message: string;
  /** Auto-captured by the widget — never user-edited. */
  context: FeedbackContext;
}

export function submitFeedback(input: SubmitFeedbackInput): Promise<Feedback> {
  return api.post<{ feedback: Feedback }>('/api/v1/feedback', input).then((r) => r.feedback);
}
