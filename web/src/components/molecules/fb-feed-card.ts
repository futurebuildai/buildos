import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import './../atoms/fb-button.js';
import type { IconName } from '../atoms/icons.js';

export type FeedPriority = 'critical' | 'warning' | 'info';

/** One actionable item on a feed card. `id` is echoed back in the `action` event. */
export interface FeedAction {
  id: string;
  label: string;
  variant?: 'primary' | 'secondary' | 'ghost' | 'destructive';
}

const PRIORITY_META: Record<FeedPriority, { icon: IconName; color: string }> = {
  critical: { icon: 'alert-triangle', color: 'var(--fb-safety-red)' },
  warning: { icon: 'alert-circle', color: 'var(--fb-amber, #f59e0b)' },
  info: { icon: 'info', color: 'var(--fb-blueprint-blue, #38bdf8)' },
};

/**
 * `fb-feed-card` — a prioritized activity/notification card (DSC §7, AG-05),
 * consumed by the Feed and the Daily Briefing. The backend `card_type`/`actions[]`
 * catalog is still open (OQ-11), so this stays generic: callers pass an `actions`
 * array and listen for a composed `action` ({ actionId }) plus `dismiss`. Priority
 * maps to a color + icon + accessible label — never color alone (WCAG 1.4.1).
 */
@customElement('fb-feed-card')
export class FbFeedCard extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .card {
        display: flex;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-left: 3px solid var(--prio-color);
        border-radius: var(--fb-radius-md);
      }
      .icon {
        color: var(--prio-color);
        flex-shrink: 0;
      }
      .body {
        flex: 1;
        min-width: 0;
      }
      .top {
        display: flex;
        align-items: baseline;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
      }
      .title {
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .time {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
        font-family: var(--fb-font-mono);
        white-space: nowrap;
      }
      .message {
        margin-top: var(--fb-spacing-xs);
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-md);
      }
      .actions {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
        margin-top: var(--fb-spacing-sm);
      }
      .close {
        align-self: flex-start;
        padding: 2px;
        color: var(--fb-text-muted);
        background: transparent;
        border: none;
        cursor: pointer;
      }
      .close:hover {
        color: var(--fb-text-primary);
      }
    `,
  ];

  @property({ type: String }) priority: FeedPriority = 'info';
  @property({ type: String }) heading = '';
  @property({ type: String }) message?: string;
  /** Pre-formatted relative time (e.g. "2h ago"); formatting is a caller concern. */
  @property({ type: String }) timestamp?: string;
  @property({ type: Array }) actions: FeedAction[] = [];
  @property({ type: Boolean }) dismissible = false;

  private onAction(actionId: string): void {
    this.emit('action', { actionId });
  }

  private onDismiss(): void {
    this.emit('dismiss');
  }

  override render(): TemplateResult {
    const meta = PRIORITY_META[this.priority];
    return html`
      <div class="card" style="--prio-color:${meta.color}">
        <fb-icon class="icon" name=${meta.icon} size="20" label=${this.priority}></fb-icon>
        <div class="body">
          <div class="top">
            <span class="title">${this.heading}</span>
            ${this.timestamp ? html`<span class="time">${this.timestamp}</span>` : nothing}
          </div>
          ${this.message ? html`<p class="message">${this.message}</p>` : nothing}
          ${this.actions.length
            ? html`<div class="actions">
                ${this.actions.map(
                  (a) =>
                    html`<fb-button
                      variant=${a.variant ?? 'secondary'}
                      size="sm"
                      @click=${() => this.onAction(a.id)}
                      >${a.label}</fb-button
                    >`,
                )}
              </div>`
            : nothing}
        </div>
        ${this.dismissible
          ? html`<button class="close" type="button" aria-label="Dismiss" @click=${this.onDismiss}>
              <fb-icon name="x" size="16"></fb-icon>
            </button>`
          : nothing}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-feed-card': FbFeedCard;
  }
}
