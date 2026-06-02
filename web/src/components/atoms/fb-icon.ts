import { html, css, svg, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { unsafeSVG } from 'lit/directives/unsafe-svg.js';
import { FBElement } from '../base/fb-element.js';
import { ICON_PATHS, type IconName } from './icons.js';

/**
 * `fb-icon` — renders a registry SVG glyph at the current text color/size.
 *
 * Decorative by default (`aria-hidden`). When an icon carries meaning on its own
 * (rare — the design system pairs icons with text), pass `label` to expose an
 * accessible name via `role="img"`.
 */
@customElement('fb-icon')
export class FbIcon extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: inline-flex;
        width: var(--fb-icon-size, 20px);
        height: var(--fb-icon-size, 20px);
        color: inherit;
        line-height: 0;
      }
      svg {
        width: 100%;
        height: 100%;
        display: block;
      }
    `,
  ];

  /** Registry icon name. */
  @property({ type: String }) name: IconName = 'info';

  /** Pixel size; also settable via the `--fb-icon-size` custom property. */
  @property({ type: Number }) size?: number;

  /** Accessible name. When set the icon is exposed as `role="img"`. */
  @property({ type: String }) label?: string;

  override render(): TemplateResult {
    const inner = ICON_PATHS[this.name] ?? ICON_PATHS.info;
    const sizeStyle = this.size ? `--fb-icon-size:${this.size}px` : nothing;
    return html`
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        style=${sizeStyle}
        role=${this.label ? 'img' : 'presentation'}
        aria-label=${this.label ?? nothing}
        aria-hidden=${this.label ? nothing : 'true'}
      >
        ${svg`${unsafeSVG(inner)}`}
      </svg>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-icon': FbIcon;
  }
}
