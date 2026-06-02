import { html, css, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from './fb-element.js';

/**
 * Temporary stand-in for routes whose real page component lands in a later
 * phase. The router table (router.ts) is the contract; as each phase registers
 * its real `fb-*-page` element, this placeholder stops being used for that route.
 */
@customElement('fb-placeholder')
export class FbPlaceholder extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: grid;
        place-items: center;
        min-height: 50vh;
        padding: var(--fb-spacing-2xl);
        text-align: center;
      }
      .title {
        font-size: var(--fb-text-headline-sm);
        font-weight: 500;
        margin: 0 0 var(--fb-spacing-sm);
      }
      .meta {
        color: var(--fb-text-secondary);
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
      }
    `,
  ];

  @property({ type: String }) heading = 'Coming soon';
  @property({ type: String }) route = '';

  override render(): TemplateResult {
    return html`
      <div class="glass-card" style="padding: var(--fb-spacing-xl)">
        <p class="title">${this.heading}</p>
        <p class="meta">${this.route}</p>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-placeholder': FbPlaceholder;
  }
}
