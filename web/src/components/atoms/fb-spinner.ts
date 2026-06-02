import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';

/**
 * `fb-spinner` — indeterminate progress indicator. Carries `role="status"` +
 * `aria-label` so screen readers announce loading. Reduced-motion slows (does not
 * stop) the rotation so the busy state remains perceivable without a fast spin.
 */
@customElement('fb-spinner')
export class FbSpinner extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: inline-flex;
      }
      .ring {
        width: var(--size, 20px);
        height: var(--size, 20px);
        border-radius: 50%;
        border: 2px solid color-mix(in srgb, var(--fb-gable-green) 25%, transparent);
        border-top-color: var(--fb-gable-green);
        animation: fb-spin 0.8s linear infinite;
      }
      @keyframes fb-spin {
        to {
          transform: rotate(360deg);
        }
      }
      @media (prefers-reduced-motion: reduce) {
        .ring {
          animation-duration: 1.6s;
        }
      }
    `,
  ];

  @property({ type: Number }) size = 20;
  @property({ type: String }) label = 'Loading';

  override render(): TemplateResult {
    return html`<div
      class="ring"
      role="status"
      aria-label=${this.label}
      style=${this.size ? `--size:${this.size}px` : nothing}
    ></div>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-spinner': FbSpinner;
  }
}
