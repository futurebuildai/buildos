/**
 * capabilityStore — feature availability (AI / email configured?) per
 * FRONTEND_ARCHITECTURE §4.3 / §6.2.
 *
 * Source of truth: GET /api/v1/capabilities (active-credential presence in the
 * BYOK vault). Fallback contract: ASSUME-ON. If the endpoint errors or is
 * unmounted (vault not wired), AI/email render as available and degrade
 * REACTIVELY on a 503 AI_UNCONFIGURED soft-fail. This guarantees a capabilities
 * outage never hard-bricks the UI.
 *
 * Refresh triggers: at boot, after any Integrations mutation, after any
 * AI_UNCONFIGURED soft-fail.
 */
import { signal, computed } from '@lit-labs/signals';
import { getCapabilities } from '../api/endpoints/capabilities.js';
import type { Capabilities } from '../types/models.js';

/** null = not yet loaded / endpoint unmounted → assume-on. */
const capsSignal = signal<Capabilities | null>(null);

export const capabilities = computed(() => capsSignal.get());

/** Assume-on: AI is available unless we positively know it is not. */
export const aiConfigured = computed(() => capsSignal.get()?.ai_configured ?? true);
export const emailConfigured = computed(() => capsSignal.get()?.email_configured ?? true);

export async function refreshCapabilities(): Promise<void> {
  try {
    capsSignal.set(await getCapabilities());
  } catch {
    // Endpoint missing or transient: keep assume-on (do not flip to false).
    capsSignal.set(null);
  }
}

/** Force AI to "unconfigured" after a reactive 503, pending a real refresh. */
export function markAiUnconfigured(): void {
  const current = capsSignal.get();
  capsSignal.set({
    ai_configured: false,
    email_configured: current?.email_configured ?? true,
    providers: current?.providers ?? [],
  });
}

export function clearCapabilities(): void {
  capsSignal.set(null);
}
