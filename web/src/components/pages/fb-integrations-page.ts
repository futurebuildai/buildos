import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-button.js';
import '../molecules/fb-integration-card.js';
import {
  listIntegrations,
  setCredential,
  deleteCredential,
} from '../../api/endpoints/integrations.js';
import type { IntegrationCredential } from '../../types/models.js';
import { ApiError, userMessageForCode } from '../../api/errors.js';
import { refreshCapabilities } from '../../state/capabilityStore.js';
import type { KeyState } from '../molecules/fb-integration-card.js';

/** A BYOK provider the console can configure (UX_AUTH_ONBOARDING §7). */
interface ProviderDef {
  /** Wire id (lowercased) used in the route param. */
  id: string;
  name: string;
  /** Copy shown when no key is set, naming the disabled feature + consequence. */
  offNotice: string;
}

const PROVIDERS: ProviderDef[] = [
  {
    id: 'anthropic',
    name: 'Anthropic',
    offNotice: 'AI features are off until an Anthropic API key is added.',
  },
  {
    id: 'resend',
    name: 'Resend',
    offNotice: 'Password-reset emails can’t be sent until a Resend API key is added.',
  },
];

/**
 * `fb-integrations-page` — Settings → Integrations (UX_AUTH_ONBOARDING §7). The
 * owner-only BYOK surface: set/rotate/remove the encrypted-vault API keys that
 * light up AI and transactional email. Secrets are write-only (no reveal) and
 * never round-trip — the card only ever shows metadata (last4, active state).
 *
 * Route gating (owner) is the router's job; this page renders for whoever
 * reaches it. A per-provider "feature off" banner names the disabled capability
 * and its consequence so an admin understands why something is dark.
 */
@customElement('fb-integrations-page')
export class FbIntegrationsPage extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
        max-width: 720px;
      }
      .head {
        margin-bottom: var(--fb-spacing-lg);
      }
      .title {
        margin: 0 0 var(--fb-spacing-xs);
        font-size: var(--fb-text-headline-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .sub {
        margin: 0;
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
      }
      .grid {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-lg);
      }
      .off {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-bottom: var(--fb-spacing-sm);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
        background: var(--fb-surface-2);
        border: 1px solid var(--fb-glass-border);
        border-radius: var(--fb-radius-sm);
      }
      .off fb-icon {
        color: var(--fb-amber, #f59e0b);
      }
      .toast {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-bottom: var(--fb-spacing-md);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        border-radius: var(--fb-radius-sm);
      }
      .toast.ok {
        color: var(--fb-gable-green);
        background: color-mix(in srgb, var(--fb-gable-green) 12%, transparent);
        border: 1px solid var(--fb-gable-green);
      }
      .toast.err {
        color: var(--fb-safety-red);
        background: color-mix(in srgb, var(--fb-safety-red) 10%, transparent);
        border: 1px solid var(--fb-safety-red);
      }
      .loading,
      .error-state {
        padding: var(--fb-spacing-xl);
        text-align: center;
        color: var(--fb-text-secondary);
      }
    `,
  ];

  @state() private credentials: IntegrationCredential[] = [];
  @state() private loading = true;
  @state() private loadError: string | null = null;
  @state() private notice: { kind: 'ok' | 'err'; text: string } | null = null;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  private async load(): Promise<void> {
    this.loading = true;
    this.loadError = null;
    try {
      this.credentials = await listIntegrations();
    } catch (err) {
      this.loadError =
        err instanceof ApiError ? userMessageForCode(err.code) : 'Something went wrong on our end.';
    } finally {
      this.loading = false;
    }
  }

  private credFor(providerId: string): IntegrationCredential | undefined {
    return this.credentials.find((c) => c.provider === providerId && c.is_active);
  }

  private async onSave(provider: ProviderDef, e: Event): Promise<void> {
    const value = (e as CustomEvent<{ value: string }>).detail.value;
    if (!value) return;
    this.notice = null;
    try {
      await setCredential(provider.id, { label: `${provider.name} API key`, key: value });
      this.notice = { kind: 'ok', text: `${provider.name} key saved.` };
      await this.load();
      // Re-prime global AI/email gating so other screens reflect the flip.
      void refreshCapabilities();
    } catch (err) {
      this.notice = {
        kind: 'err',
        text:
          err instanceof ApiError
            ? userMessageForCode(err.code)
            : `Couldn’t save the ${provider.name} key.`,
      };
    }
  }

  private async onToggle(provider: ProviderDef, e: Event): Promise<void> {
    const enabled = (e as CustomEvent<{ enabled: boolean }>).detail.enabled;
    // The vault only models set/delete; toggling off deactivates the key.
    if (enabled) {
      // Can't activate without a key — refetch to reset the switch to truth.
      await this.load();
      return;
    }
    this.notice = null;
    try {
      await deleteCredential(provider.id);
      this.notice = { kind: 'ok', text: `${provider.name} key removed.` };
    } catch (err) {
      this.notice = {
        kind: 'err',
        text:
          err instanceof ApiError
            ? userMessageForCode(err.code)
            : `Couldn’t remove the ${provider.name} key.`,
      };
    } finally {
      await this.load();
      // Re-prime global AI/email gating so other screens reflect the flip.
      void refreshCapabilities();
    }
  }

  private onTest(provider: ProviderDef): void {
    // No backend health-check route yet (OQ Q15) — be honest rather than fake a result.
    this.notice = {
      kind: 'ok',
      text: `Connection testing for ${provider.name} isn’t available yet.`,
    };
  }

  override render(): TemplateResult {
    if (this.loading) {
      return html`<p class="loading" role="status">Loading integrations…</p>`;
    }
    if (this.loadError) {
      return html`
        <div class="error-state" role="alert">
          <p>${this.loadError}</p>
          <fb-button variant="secondary" @click=${() => void this.load()}>Retry</fb-button>
        </div>
      `;
    }

    // NOTE: keep the whole non-loading view inside a single root element. A bare
    // root-level child expression (the notice) directly preceding another child
    // expression (the provider grid) is mis-rendered by happy-dom's part-marker
    // handling, silently dropping the grid in component tests. Wrapping in one
    // container keeps the child parts off the shadow-root top level.
    return html`
      <div class="page">
        <div class="head">
          <h1 class="title">Integrations</h1>
          <p class="sub">
            Bring your own keys. Secrets are encrypted at rest and never shown again after you save
            them.
          </p>
        </div>

        ${this.notice
          ? html`<p class="toast ${this.notice.kind}" role="status">
              <fb-icon
                name=${this.notice.kind === 'ok' ? 'check-circle' : 'alert-circle'}
                size="16"
              ></fb-icon>
              ${this.notice.text}
            </p>`
          : nothing}

        <div class="grid">
          ${PROVIDERS.map((p) => {
            const cred = this.credFor(p.id);
            const keyState: KeyState = cred ? 'connected' : 'missing';
            return html`<div>
              ${cred
                ? nothing
                : html`<p class="off">
                    <fb-icon name="alert-triangle" size="14"></fb-icon>${p.offNotice}
                  </p>`}
              <fb-integration-card
                provider=${p.name}
                key-state=${keyState}
                ?has-value=${!!cred}
                last4=${cred?.last4 ?? nothing}
                ?enabled=${!!cred}
                @save=${(e: Event) => void this.onSave(p, e)}
                @toggle=${(e: Event) => void this.onToggle(p, e)}
                @test=${() => this.onTest(p)}
              ></fb-integration-card>
            </div>`;
          })}
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-integrations-page': FbIntegrationsPage;
  }
}
