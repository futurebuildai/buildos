import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-icon.js';
import type { IconName } from './icons.js';

/**
 * `fb-icon-button` — a square, icon-only button (DSC §3). A `label` is REQUIRED
 * (it becomes the accessible name) since there is no visible text. Supports a
 * pressed/toggle state via `pressed` (reflected to `aria-pressed`) for affordances
 * like the password show/hide toggle.
 */
@customElement('fb-icon-button')
export class FbIconButton extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: inline-block;
      }
      button {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        width: var(--fb-density-control);
        height: var(--fb-density-control);
        padding: 0;
        border: 1px solid transparent;
        border-radius: var(--fb-radius-sm);
        background: transparent;
        color: var(--fb-text-secondary);
        cursor: pointer;
      }
      button:hover:not(:disabled) {
        background: var(--fb-surface-2);
        color: var(--fb-text-primary);
      }
      button:disabled {
        cursor: not-allowed;
        opacity: 0.5;
      }
      button[aria-pressed='true'] {
        color: var(--fb-gable-green);
      }
      :host([size='sm']) button {
        width: 28px;
        height: 28px;
      }
    `,
  ];

  @property({ type: String }) icon: IconName = 'info';
  @property({ type: String }) label = '';
  @property({ type: String, reflect: true }) size: 'sm' | 'md' = 'md';
  @property({ type: Boolean, reflect: true }) disabled = false;
  /** Toggle state; when defined, exposes `aria-pressed`. Leave undefined for plain buttons. */
  @property({ type: Boolean }) pressed?: boolean;
  @property({ type: String }) type: 'button' | 'submit' | 'reset' = 'button';

  override render(): TemplateResult {
    return html`
      <button
        type=${this.type}
        ?disabled=${this.disabled}
        aria-label=${this.label}
        aria-pressed=${this.pressed === undefined ? nothing : this.pressed ? 'true' : 'false'}
      >
        <fb-icon name=${this.icon} size=${this.size === 'sm' ? 14 : 18}></fb-icon>
      </button>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-icon-button': FbIconButton;
  }
}
