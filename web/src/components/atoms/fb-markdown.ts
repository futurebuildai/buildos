import { css, html, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { markdownToTemplate } from '../../lib/markdown.js';

/**
 * `fb-markdown` — the single styled prose surface for model-authored markdown
 * (briefing hero, assistant chat replies, advisory rationale). It is a thin
 * wrapper over {@link markdownToTemplate}: the parser builds an XSS-safe Lit
 * tree (every text run is an escaped `${value}`; no `unsafeHTML`), and this atom
 * just applies the token-only prose styles.
 *
 * Set `.source=${markdownString}`. Empty/whitespace source renders nothing.
 */
@customElement('fb-markdown')
export class FbMarkdown extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .prose > :first-child {
        margin-top: 0;
      }
      .prose > :last-child {
        margin-bottom: 0;
      }
      p {
        margin: 0 0 var(--fb-spacing-sm);
        color: var(--fb-text-primary);
        line-height: 1.6;
      }
      strong {
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      em {
        font-style: italic;
      }
      code {
        font-family: var(--fb-font-mono);
        font-size: 0.9em;
        background: var(--fb-surface-2);
        padding: 0.1em 0.35em;
        border-radius: var(--fb-radius-sm);
      }
      pre {
        margin: 0 0 var(--fb-spacing-sm);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        background: var(--fb-surface-2);
        border-radius: var(--fb-radius-sm);
        overflow-x: auto;
      }
      pre code {
        background: none;
        padding: 0;
        white-space: pre;
      }
      ul,
      ol {
        margin: 0 0 var(--fb-spacing-sm);
        padding-left: var(--fb-spacing-lg);
        color: var(--fb-text-primary);
      }
      li {
        margin: 0 0 calc(var(--fb-spacing-sm) / 2);
        line-height: 1.6;
      }
      h3,
      h4 {
        margin: var(--fb-spacing-md) 0 var(--fb-spacing-sm);
        color: var(--fb-text-primary);
        font-weight: 600;
      }
      h3 {
        font-size: var(--fb-text-title-md);
      }
      h4 {
        font-size: var(--fb-text-body-lg);
      }
    `,
  ];

  /** Markdown source text (model- or user-authored). */
  @property({ type: String }) source = '';

  override render(): TemplateResult {
    const src = this.source ?? '';
    if (src.trim() === '') return html``;
    return html`<div class="prose">${markdownToTemplate(src)}</div>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-markdown': FbMarkdown;
  }
}
