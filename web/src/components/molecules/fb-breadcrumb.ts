import { html, css, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';

export interface Crumb {
  label: string;
  href?: string;
}

/**
 * `fb-breadcrumb` — navigation trail (DSC §7, AG-05). A real `<nav aria-label>`
 * with an ordered list; the last crumb is the current page (`aria-current`) and
 * not a link. Intermediate crumbs are anchors; clicking emits a composed
 * `navigate` so the SPA router can intercept without a full reload.
 */
@customElement('fb-breadcrumb')
export class FbBreadcrumb extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      ol {
        display: flex;
        flex-wrap: wrap;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin: 0;
        padding: 0;
        list-style: none;
        font-size: var(--fb-text-body-sm);
      }
      li {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        color: var(--fb-text-secondary);
      }
      a {
        color: var(--fb-text-secondary);
        text-decoration: none;
      }
      a:hover {
        color: var(--fb-text-primary);
        text-decoration: underline;
      }
      [aria-current='page'] {
        color: var(--fb-text-primary);
        font-weight: 600;
      }
      fb-icon {
        color: var(--fb-text-muted);
      }
    `,
  ];

  @property({ type: Array }) crumbs: Crumb[] = [];

  private onNavigate(e: Event, href: string): void {
    e.preventDefault();
    this.emit('navigate', { href });
  }

  override render(): TemplateResult {
    return html`
      <nav aria-label="Breadcrumb">
        <ol>
          ${this.crumbs.map((c, i) => {
            const last = i === this.crumbs.length - 1;
            return html`<li>
              ${last || !c.href
                ? html`<span aria-current=${last ? 'page' : 'false'}>${c.label}</span>`
                : html`<a href=${c.href} @click=${(e: Event) => this.onNavigate(e, c.href!)}
                    >${c.label}</a
                  >`}
              ${last ? null : html`<fb-icon name="chevron-right" size="14"></fb-icon>`}
            </li>`;
          })}
        </ol>
      </nav>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-breadcrumb': FbBreadcrumb;
  }
}
