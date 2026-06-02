import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-icon.js';

/**
 * `fb-password-input` — password field with a keyboard-reachable show/hide toggle
 * (DSC §5.1). Used in sign-in, owner-claim set-password, reset-password, and
 * change-password.
 *
 * - The toggle is a real `<button>` announcing its state via `aria-pressed` and a
 *   "Show password"/"Hide password" label.
 * - A caps-lock warning surfaces inline (text, never color-only) and is wired to
 *   the input via `aria-describedby` so screen readers hear it.
 * - The optional `hint`/strength text is also `aria-describedby` content.
 *
 * Emits composed `input`/`change` with `{ value }`.
 */
@customElement('fb-password-input')
export class FbPasswordInput extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .wrap {
        position: relative;
        display: block;
      }
      input {
        width: 100%;
        min-height: var(--fb-density-control);
        padding: 0 calc(var(--fb-density-control) + var(--fb-spacing-xs)) 0 var(--fb-spacing-md);
        font-family: var(--fb-font-sans);
        font-size: var(--fb-text-body-md);
        color: var(--fb-text-primary);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-sm);
        letter-spacing: 0.02em;
      }
      input:focus-visible {
        border-color: var(--fb-gable-green);
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 1px;
      }
      input[aria-invalid='true'] {
        border-color: var(--fb-safety-red);
      }
      input:disabled {
        opacity: 0.5;
        cursor: not-allowed;
      }
      .toggle {
        position: absolute;
        right: var(--fb-spacing-xs);
        top: 50%;
        transform: translateY(-50%);
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: 32px;
        height: 32px;
        padding: 0;
        color: var(--fb-text-secondary);
        background: transparent;
        border: none;
        border-radius: var(--fb-radius-sm);
        cursor: pointer;
      }
      .toggle:hover {
        color: var(--fb-text-primary);
      }
      .caps {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-top: var(--fb-spacing-xs);
        color: var(--fb-amber, #f59e0b);
        font-size: var(--fb-text-body-sm);
      }
    `,
  ];

  @property({ type: String }) value = '';
  @property({ type: String }) name?: string;
  @property({ type: String }) placeholder?: string;
  /** `current-password` (sign-in) or `new-password` (claim/reset). */
  @property({ type: String }) autocomplete: 'current-password' | 'new-password' =
    'current-password';
  @property({ type: Boolean, reflect: true }) disabled = false;
  @property({ type: Boolean }) required = false;
  @property({ type: Boolean, reflect: true }) invalid = false;
  @property({ type: String }) label?: string;
  /** id(s) of external hint/error nodes (set by fb-field). */
  @property({ type: String }) describedby?: string;

  @state() private revealed = false;
  @state() private capsLock = false;

  private readonly capsHintId = `caps-${Math.random().toString(36).slice(2, 8)}`;

  private onInput(e: Event): void {
    this.value = (e.target as HTMLInputElement).value;
    this.emit('input', { value: this.value });
  }

  private onChange(): void {
    this.emit('change', { value: this.value });
  }

  private onKey(e: KeyboardEvent): void {
    // getModifierState is the standard way to detect Caps Lock without logging keys.
    if (typeof e.getModifierState === 'function') {
      this.capsLock = e.getModifierState('CapsLock');
    }
  }

  private toggleReveal(): void {
    this.revealed = !this.revealed;
  }

  private describedByValue(): string | typeof nothing {
    const ids = [this.describedby, this.capsLock ? this.capsHintId : undefined].filter(
      (v): v is string => !!v,
    );
    return ids.length ? ids.join(' ') : nothing;
  }

  override render(): TemplateResult {
    return html`
      <div class="wrap">
        <input
          .value=${this.value}
          type=${this.revealed ? 'text' : 'password'}
          name=${this.name ?? nothing}
          placeholder=${this.placeholder ?? nothing}
          autocomplete=${this.autocomplete}
          ?disabled=${this.disabled}
          ?required=${this.required}
          aria-invalid=${this.invalid ? 'true' : nothing}
          aria-label=${this.label ?? nothing}
          aria-describedby=${this.describedByValue()}
          @input=${this.onInput}
          @change=${this.onChange}
          @keyup=${this.onKey}
          @keydown=${this.onKey}
        />
        <button
          class="toggle"
          type="button"
          aria-pressed=${this.revealed ? 'true' : 'false'}
          aria-label=${this.revealed ? 'Hide password' : 'Show password'}
          ?disabled=${this.disabled}
          @click=${this.toggleReveal}
        >
          <fb-icon name=${this.revealed ? 'eye-off' : 'eye'} size="18"></fb-icon>
        </button>
      </div>
      ${this.capsLock
        ? html`<p class="caps" id=${this.capsHintId}>
            <fb-icon name="alert-triangle" size="14"></fb-icon>
            Caps Lock is on
          </p>`
        : nothing}
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-password-input': FbPasswordInput;
  }
}
