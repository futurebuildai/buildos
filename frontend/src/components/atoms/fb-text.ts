import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export type TextVariant = 'display' | 'headline' | 'title' | 'body' | 'label' | 'data' | 'data-compact';

/**
 * fb-text — Typography component with variant-based styling.
 *
 * Automatically applies JetBrains Mono for data/data-compact variants.
 * Outfit for all other variants.
 *
 * @property variant - Typography variant
 * @property color - Override text color (CSS custom property or hex)
 * @property mono - Force JetBrains Mono font
 */
@customElement('fb-text')
export class FBText extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: inline; }

      .display {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-4xl);
        font-weight: 400;
        line-height: 1.12;
      }
      .headline {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-3xl);
        font-weight: 400;
        line-height: 1.25;
      }
      .title {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-lg);
        font-weight: 500;
        line-height: 1.27;
      }
      .body {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        font-weight: 400;
        line-height: 1.43;
      }
      .label {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        line-height: 1.33;
        text-transform: uppercase;
        letter-spacing: 0.04em;
      }
      .data {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-base);
        font-weight: 400;
        line-height: 1.43;
        font-variant-numeric: tabular-nums;
      }
      .data-compact {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-sm);
        font-weight: 400;
        line-height: 1.33;
        font-variant-numeric: tabular-nums;
      }
      .is-mono {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
      }
    `,
  ];

  @property({ type: String }) variant: TextVariant = 'body';
  @property({ type: String }) color = '';
  @property({ type: Boolean }) mono = false;

  override render() {
    const className = `${this.variant}${this.mono ? ' is-mono' : ''}`;
    const style = this.color ? `color: ${this.color}` : '';

    return html`
      <span class=${className} style=${style}>
        <slot></slot>
      </span>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-text': FBText;
  }
}
