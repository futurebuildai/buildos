import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export type SpinnerSize = 'sm' | 'md' | 'lg';

/**
 * fb-spinner — Loading spinner with Gable Green accent.
 *
 * @property size - Spinner size: sm (16px) | md (24px) | lg (40px)
 */
@customElement('fb-spinner')
export class FBSpinner extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host {
        display: inline-flex;
        align-items: center;
        justify-content: center;
      }

      .spinner {
        border-radius: 50%;
        border-style: solid;
        border-color: rgba(0, 255, 163, 0.2);
        border-top-color: var(--fb-gable-green);
        animation: spin 700ms linear infinite;
      }

      .sm { width: 16px; height: 16px; border-width: 2px; }
      .md { width: 24px; height: 24px; border-width: 3px; }
      .lg { width: 40px; height: 40px; border-width: 4px; }

      @keyframes spin {
        to { transform: rotate(360deg); }
      }

      @media (prefers-reduced-motion: reduce) {
        .spinner { animation: none; opacity: 0.6; }
      }
    `,
  ];

  @property({ type: String }) size: SpinnerSize = 'md';

  override render() {
    return html`
      <div class="spinner ${this.size}" role="status" aria-label="Loading">
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-spinner': FBSpinner;
  }
}
