import { html, css, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-icon.js';
import type { IconName } from './icons.js';

/**
 * Status vocabulary spanning the domain-status, CPM, and BYOK key-state tokens
 * (DSC §2.1). Each maps to a color token + a default icon so the badge is never
 * color-only (WCAG 1.4.1, DSC §7.15).
 */
export type BadgeStatus =
  | 'active'
  | 'warning'
  | 'critical'
  | 'complete'
  | 'pending'
  | 'offline'
  | 'neutral'
  // key states
  | 'key-connected'
  | 'key-untested'
  | 'key-error'
  | 'key-missing';

interface StatusMeta {
  color: string;
  icon: IconName;
}

const STATUS_META: Record<BadgeStatus, StatusMeta> = {
  active: { color: 'var(--fb-status-active)', icon: 'check-circle' },
  warning: { color: 'var(--fb-status-warning)', icon: 'alert-triangle' },
  critical: { color: 'var(--fb-status-critical)', icon: 'alert-circle' },
  complete: { color: 'var(--fb-gable-green-bright)', icon: 'check' },
  pending: { color: 'var(--fb-status-pending)', icon: 'clock' },
  offline: { color: 'var(--fb-status-offline)', icon: 'wifi-off' },
  neutral: { color: 'var(--fb-status-neutral)', icon: 'info' },
  'key-connected': { color: 'var(--fb-key-connected)', icon: 'shield-check' },
  'key-untested': { color: 'var(--fb-key-untested)', icon: 'info' },
  'key-error': { color: 'var(--fb-key-error)', icon: 'alert-circle' },
  'key-missing': { color: 'var(--fb-key-missing)', icon: 'lock' },
};

/**
 * `fb-badge` — status pill (DSC §7.15). Always color + icon + text: the icon and
 * the slotted label carry the meaning; color is reinforcement only. Sizes sm/md.
 */
@customElement('fb-badge')
export class FbBadge extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: inline-flex;
      }
      .badge {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        padding: 2px var(--fb-spacing-sm);
        border-radius: var(--fb-radius-full);
        font-family: var(--fb-font-sans);
        font-size: var(--fb-text-body-sm);
        font-weight: 600;
        line-height: 1.4;
        color: var(--badge-color);
        background: color-mix(in srgb, var(--badge-color) 14%, transparent);
        border: 1px solid color-mix(in srgb, var(--badge-color) 32%, transparent);
        white-space: nowrap;
      }
      :host([size='sm']) .badge {
        font-size: var(--fb-text-label-sm);
        padding: 1px var(--fb-spacing-sm);
      }
      .dot {
        width: 6px;
        height: 6px;
        border-radius: 50%;
        background: var(--badge-color);
        flex: none;
      }
    `,
  ];

  @property({ type: String, reflect: true }) status: BadgeStatus = 'neutral';
  @property({ type: String, reflect: true }) size: 'sm' | 'md' = 'md';
  /** Use a small color dot instead of the status icon (denser tables). */
  @property({ type: Boolean }) dot = false;

  override render(): TemplateResult {
    const meta = STATUS_META[this.status] ?? STATUS_META.neutral;
    return html`
      <span class="badge" style="--badge-color:${meta.color}">
        ${this.dot
          ? html`<span class="dot" aria-hidden="true"></span>`
          : html`<fb-icon name=${meta.icon} size=${this.size === 'sm' ? 12 : 14}></fb-icon>`}
        <slot></slot>
      </span>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-badge': FbBadge;
  }
}
