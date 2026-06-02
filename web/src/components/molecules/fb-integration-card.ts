import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-badge.js';
import './../atoms/fb-button.js';
import './../atoms/fb-switch.js';
import './../atoms/fb-secret-input.js';

export type KeyState = 'connected' | 'untested' | 'error' | 'missing';

const KEY_BADGE: Record<KeyState, { status: string; label: string }> = {
  connected: { status: 'key-connected', label: 'Connected' },
  untested: { status: 'key-untested', label: 'Untested' },
  error: { status: 'key-error', label: 'Connection error' },
  missing: { status: 'key-missing', label: 'No key set' },
};

/**
 * `fb-integration-card` — one BYOK provider card (DSC §5.4): Anthropic, Resend,
 * Gable, LocalBlue. Owner-only surface (gated by the route, not here). Composes
 * the key-state badge, the write-only `fb-secret-input`, a test-connection action,
 * the last-tested timestamp, and an enable/disable switch.
 *
 * Emits: `save` ({ value }) when a new secret is submitted, `test` to trigger the
 * provider health check, and `toggle` ({ enabled }). The card never holds the
 * decrypted secret — the input is write-only by design.
 */
@customElement('fb-integration-card')
export class FbIntegrationCard extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .card {
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
      }
      .head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
        margin-bottom: var(--fb-spacing-sm);
      }
      .name {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        font-weight: 600;
        font-size: var(--fb-text-title-md);
        color: var(--fb-text-primary);
      }
      .controls {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        margin-top: var(--fb-spacing-sm);
      }
      .meta {
        margin-top: var(--fb-spacing-sm);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
        font-family: var(--fb-font-mono);
      }
    `,
  ];

  @property({ type: String }) provider = '';
  @property({ type: String, attribute: 'key-state' }) keyState: KeyState = 'missing';
  @property({ type: Boolean, attribute: 'has-value' }) hasValue = false;
  @property({ type: String }) last4?: string;
  @property({ type: String, attribute: 'last-tested' }) lastTested?: string;
  @property({ type: Boolean }) enabled = false;
  @property({ type: Boolean, reflect: true }) disabled = false;

  private pending = '';

  private onInput(e: CustomEvent<{ value: string }>): void {
    this.pending = e.detail.value;
  }

  private onSave(): void {
    if (!this.pending) return;
    this.emit('save', { value: this.pending });
    this.pending = '';
    const secret = this.renderRoot.querySelector<HTMLElement & { submit(): void }>(
      'fb-secret-input',
    );
    secret?.submit();
  }

  private onTest(): void {
    this.emit('test');
  }

  private onToggle(e: CustomEvent<{ checked: boolean }>): void {
    this.enabled = e.detail.checked;
    this.emit('toggle', { enabled: this.enabled });
  }

  override render(): TemplateResult {
    const badge = KEY_BADGE[this.keyState];
    return html`
      <div class="card">
        <div class="head">
          <span class="name"><slot name="logo"></slot>${this.provider}</span>
          <fb-switch
            ?checked=${this.enabled}
            ?disabled=${this.disabled}
            label="Enable ${this.provider}"
            @change=${this.onToggle}
          ></fb-switch>
        </div>
        <fb-badge status=${badge.status}>${badge.label}</fb-badge>
        <div class="controls">
          <fb-secret-input
            style="flex:1"
            ?has-value=${this.hasValue}
            last4=${this.last4 ?? nothing}
            label="${this.provider} API key"
            ?disabled=${this.disabled}
            @input=${this.onInput}
          ></fb-secret-input>
          <fb-button variant="secondary" size="sm" ?disabled=${this.disabled} @click=${this.onSave}
            >Save</fb-button
          >
          <fb-button
            variant="ghost"
            size="sm"
            ?disabled=${this.disabled || !this.hasValue}
            @click=${this.onTest}
            >Test</fb-button
          >
        </div>
        ${this.lastTested ? html`<p class="meta">Last tested ${this.lastTested}</p>` : nothing}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-integration-card': FbIntegrationCard;
  }
}
