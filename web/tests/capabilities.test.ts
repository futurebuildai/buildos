import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock the endpoint so the store never hits the network. getCapabilities is
// re-stubbed per test to drive both the success and error (assume-on) paths.
vi.mock('../src/api/endpoints/capabilities.js', () => ({
  getCapabilities: vi.fn(),
}));

import * as capsApi from '../src/api/endpoints/capabilities.js';
import {
  refreshCapabilities,
  aiConfigured,
  emailConfigured,
  capabilities,
  clearCapabilities,
} from '../src/state/capabilityStore.js';
import type { Capabilities } from '../src/types/models.js';

const mockGet = vi.mocked(capsApi.getCapabilities);

describe('capabilityStore.refreshCapabilities', () => {
  beforeEach(() => {
    mockGet.mockReset();
    clearCapabilities(); // reset to null (assume-on) between tests
  });

  it('populates ai/email flags from a successful fetch', async () => {
    const payload: Capabilities = {
      ai_configured: true,
      email_configured: false,
      providers: [
        { provider: 'anthropic', configured: true, fingerprint: 'ab12' },
        { provider: 'resend', configured: false },
      ],
    };
    mockGet.mockResolvedValueOnce(payload);

    await refreshCapabilities();

    expect(aiConfigured.get()).toBe(true);
    expect(emailConfigured.get()).toBe(false);
    expect(capabilities.get()).toEqual(payload);
  });

  it('reflects email-on when the backend says so', async () => {
    mockGet.mockResolvedValueOnce({
      ai_configured: false,
      email_configured: true,
      providers: [],
    });

    await refreshCapabilities();

    expect(aiConfigured.get()).toBe(false);
    expect(emailConfigured.get()).toBe(true);
  });

  it('keeps assume-on (true) when the fetch throws', async () => {
    mockGet.mockRejectedValueOnce(new Error('network down'));

    await refreshCapabilities();

    // Endpoint failure must NOT flip features off — defensive assume-on.
    expect(capabilities.get()).toBeNull();
    expect(aiConfigured.get()).toBe(true);
    expect(emailConfigured.get()).toBe(true);
  });
});
