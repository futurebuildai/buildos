import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-icon.js';
import './fb-button.js';

/** 43-char base64url (the bootstrap-token shape from internal/service/setup.go). */
const BOOTSTRAP_TOKEN_RE = /^[A-Za-z0-9_-]{43}$/;

/**
 * `fb-secret-input` — write-only masked entry for vault secrets (DSC §5.2): API
 * keys/tokens sealed server-side by `cryptobox`. There is deliberately **no
 * "reveal stored secret"** — the fork cannot decrypt to screen by design.
 *
 * - When a secret already exists (`has-value`), shows a fixed masked placeholder
 *   (`••••••••  ····last4` if `last4` is supplied) labelled "API key set", and a
 *   "Replace" button that reveals an empty input. It never echoes the stored value.
 * - Paste-friendly and password-manager-hostile: `autocomplete="off"`,
 *   `data-1p-ignore`, `spellcheck="false"`; pasted whitespace is trimmed.
 * - The field clears its own value on `submit()` so the plaintext never lingers in
 *   the DOM.
 *
 * `bootstrap` mode (DSC §5.3) validates the 43-char base64url owner-claim token
 * client-side and exposes `valid` for the form to gate submit; the backend still
 * returns a uniform error on any redemption failure (no probing oracle).
 *
 * Emits composed `input`/`change` with `{ value, valid }`.
 */
@customElement('fb-secret-input')
export class FbSecretInput extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .masked {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        min-height: var(--fb-density-control);
        padding: 0 var(--fb-spacing-md);
        font-family: var(--fb-font-mono);
        color: var(--fb-text-secondary);
        background: var(--fb-surface-2);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-sm);
      }
      .masked .dots {
        letter-spacing: 0.15em;
      }
      .masked .last4 {
        color: var(--fb-text-muted);
      }
      .masked .spacer {
        flex: 1;
      }
      .row {
        display: flex;
        align-items: stretch;
        gap: var(--fb-spacing-sm);
      }
      input {
        flex: 1;
        min-width: 0;
        min-height: var(--fb-density-control);
        padding: 0 var(--fb-spacing-md);
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-md);
        color: var(--fb-text-primary);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-sm);
      }
      input:focus-visible {
        border-color: var(--fb-gable-green);
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 1px;
      }
      input[aria-invalid='true'] {
        border-color: var(--fb-safety-red);
      }
    `,
  ];

  @property({ type: String }) value = '';
  @property({ type: String }) name?: string;
  @property({ type: String }) placeholder = 'Paste key';
  @property({ type: Boolean, reflect: true }) disabled = false;
  @property({ type: String }) label?: string;
  @property({ type: String }) describedby?: string;
  @property({ type: Boolean, reflect: true }) invalid = false;
  /** A secret is already stored server-side; show the masked, write-only state. */
  @property({ type: Boolean, attribute: 'has-value' }) hasValue = false;
  /** Optional last 4 chars the backend may expose for recognition. */
  @property({ type: String }) last4?: string;
  /** Owner-claim bootstrap-token variant: validate 43-char base64url client-side. */
  @property({ type: Boolean }) bootstrap = false;

  /** True once the user clicks "Replace" (or no stored value exists). */
  @state() private editing = false;

  private get showInput(): boolean {
    return this.editing || !this.hasValue;
  }

  /** Client-side validity (bootstrap charset/length). Non-bootstrap = non-empty. */
  get valid(): boolean {
    if (this.bootstrap) return BOOTSTRAP_TOKEN_RE.test(this.value);
    return this.value.length > 0;
  }

  private onInput(e: Event): void {
    // Trim whitespace from pasted keys (trailing newlines are a common footgun).
    this.value = (e.target as HTMLInputElement).value.trim();
    this.emit('input', { value: this.value, valid: this.valid });
  }

  private onChange(): void {
    this.emit('change', { value: this.value, valid: this.valid });
  }

  private startReplace(): void {
    this.editing = true;
    this.value = '';
  }

  /** Clears the plaintext so it never lingers in the DOM. Call after submit. */
  submit(): void {
    this.value = '';
    this.editing = false;
  }

  private renderMasked(): TemplateResult {
    return html`
      <div class="masked" role="group" aria-label="API key set">
        <span class="dots" aria-hidden="true">••••••••••••</span>
        ${this.last4
          ? html`<span class="last4" aria-hidden="true">····${this.last4}</span>`
          : nothing}
        <span class="spacer"></span>
        <fb-button
          variant="secondary"
          size="sm"
          ?disabled=${this.disabled}
          @click=${this.startReplace}
          >Replace</fb-button
        >
      </div>
    `;
  }

  override render(): TemplateResult {
    if (!this.showInput) return this.renderMasked();
    return html`
      <div class="row">
        <input
          type="password"
          .value=${this.value}
          name=${this.name ?? nothing}
          placeholder=${this.placeholder}
          autocomplete="off"
          data-1p-ignore
          spellcheck="false"
          inputmode="text"
          ?disabled=${this.disabled}
          aria-invalid=${this.invalid ? 'true' : nothing}
          aria-label=${this.label ?? nothing}
          aria-describedby=${this.describedby ?? nothing}
          @input=${this.onInput}
          @change=${this.onChange}
        />
        <slot name="action"></slot>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-secret-input': FbSecretInput;
  }
}
