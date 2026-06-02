import { html, literal, type StaticValue } from 'lit/static-html.js';
import { css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';

export type TextVariant =
  | 'display'
  | 'headline'
  | 'title'
  | 'title-sm'
  | 'body'
  | 'body-sm'
  | 'label'
  | 'label-sm';

/**
 * `fb-text` — typography atom mapping the DESIGN_SYSTEM §3 type scale to a
 * semantic element. Numeric/data content sets `mono` for JetBrains Mono +
 * tabular-nums (DESIGN_SYSTEM §3.2). `as` picks the rendered tag so headings
 * stay semantic for screen readers.
 */
@customElement('fb-text')
export class FbText extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      :host([inline]) {
        display: inline;
      }
      .t {
        margin: 0;
        color: var(--fb-text-primary);
        font-family: var(--fb-font-sans);
      }
      .display {
        font-size: var(--fb-text-headline-lg);
        font-weight: 700;
        letter-spacing: -0.02em;
      }
      .headline {
        font-size: var(--fb-text-headline-sm);
        font-weight: 600;
      }
      .title {
        font-size: var(--fb-text-title-lg);
        font-weight: 600;
      }
      .title-sm {
        font-size: var(--fb-text-title-md);
        font-weight: 600;
      }
      .body {
        font-size: var(--fb-text-body-lg);
      }
      .body-sm {
        font-size: var(--fb-text-body-sm);
      }
      .label {
        font-size: var(--fb-text-label-lg);
        font-weight: 500;
      }
      .label-sm {
        font-size: var(--fb-text-label-sm);
        font-weight: 500;
        letter-spacing: 0.04em;
        text-transform: uppercase;
      }
      .secondary {
        color: var(--fb-text-secondary);
      }
      .muted {
        color: var(--fb-text-muted);
      }
      .mono {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
      }
    `,
  ];

  @property({ type: String, reflect: true }) variant: TextVariant = 'body';
  /** Rendered tag (keeps heading semantics). */
  @property({ type: String }) as: 'p' | 'span' | 'h1' | 'h2' | 'h3' | 'h4' | 'div' = 'p';
  @property({ type: String }) tone: 'primary' | 'secondary' | 'muted' = 'primary';
  @property({ type: Boolean, reflect: true }) mono = false;

  private tag(): StaticValue {
    switch (this.as) {
      case 'span':
        return literal`span`;
      case 'h1':
        return literal`h1`;
      case 'h2':
        return literal`h2`;
      case 'h3':
        return literal`h3`;
      case 'h4':
        return literal`h4`;
      case 'div':
        return literal`div`;
      default:
        return literal`p`;
    }
  }

  override render(): TemplateResult {
    const tag = this.tag();
    const toneClass = this.tone === 'primary' ? '' : this.tone;
    return html`
      <${tag} class="t ${this.variant} ${toneClass} ${this.mono ? 'mono' : nothing}">
        <slot></slot>
      </${tag}>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-text': FbText;
  }
}
