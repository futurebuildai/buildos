import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export type AvatarSize = 'sm' | 'md' | 'lg';

/**
 * fb-avatar — User avatar with initials fallback.
 *
 * @property name - Full name (used to compute initials)
 * @property src - Image URL (optional)
 * @property size - Avatar size: sm (28px) | md (36px) | lg (48px)
 */
@customElement('fb-avatar')
export class FBAvatar extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: inline-flex; }

      .avatar {
        display: flex;
        align-items: center;
        justify-content: center;
        border-radius: 50%;
        overflow: hidden;
        flex-shrink: 0;
        background: rgba(0, 255, 163, 0.15);
        color: var(--fb-gable-green);
        font-family: var(--fb-font-body);
        font-weight: 600;
      }

      .sm { width: 28px; height: 28px; font-size: 11px; }
      .md { width: 36px; height: 36px; font-size: 13px; }
      .lg { width: 48px; height: 48px; font-size: 16px; }

      img {
        width: 100%;
        height: 100%;
        object-fit: cover;
      }
    `,
  ];

  @property({ type: String }) name = '';
  @property({ type: String }) src = '';
  @property({ type: String }) size: AvatarSize = 'md';

  private _getInitials(): string {
    if (!this.name) return '?';
    const parts = this.name.trim().split(/\s+/);
    if (parts.length >= 2) {
      return `${parts[0]![0] ?? ''}${parts[1]![0] ?? ''}`.toUpperCase();
    }
    return (parts[0]![0] ?? '?').toUpperCase();
  }

  override render() {
    return html`
      <div class="avatar ${this.size}" title=${this.name}>
        ${this.src
          ? html`<img src=${this.src} alt=${this.name} loading="lazy" />`
          : html`${this._getInitials()}`
        }
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-avatar': FBAvatar;
  }
}
