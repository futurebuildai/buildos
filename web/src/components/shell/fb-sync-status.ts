import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import type { IconName } from '../atoms/icons.js';

export type SyncState = 'idle' | 'syncing' | 'offline' | 'error';

interface SyncMeta {
  icon: IconName;
  defaultText: string;
  cls: string;
}

const SYNC_META: Record<Exclude<SyncState, 'idle'>, SyncMeta> = {
  syncing: { icon: 'refresh', defaultText: 'Saving…', cls: 'syncing' },
  offline: { icon: 'wifi-off', defaultText: 'Offline', cls: 'offline' },
  error: { icon: 'alert-triangle', defaultText: "Couldn't save changes", cls: 'error' },
};

/**
 * `fb-sync-status` — the web console's PASSIVE variant of the field sync chip
 * (DSC §8.1). Unlike the always-on Flutter chip, the web console surfaces this
 * only when a write fails to reach the server (or a sync is in flight). In the
 * `idle` state it renders nothing, so it can live permanently in the status
 * strip without adding chrome.
 *
 * Status is never color-alone: each state pairs an icon + text. A `retry`
 * affordance is offered in the `error` state. Announced politely via
 * `role="status"` / `aria-live="polite"`.
 */
@customElement('fb-sync-status')
export class FbSyncStatus extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .chip {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-xs) var(--fb-spacing-md);
        border-radius: var(--fb-radius-full);
        font-size: var(--fb-text-body-sm);
      }
      .chip.syncing {
        color: var(--fb-status-pending);
      }
      .chip.offline {
        color: var(--fb-status-offline);
      }
      .chip.error {
        color: var(--fb-safety-red);
      }
      .syncing fb-icon {
        animation: spin 1s linear infinite;
      }
      @keyframes spin {
        to {
          transform: rotate(360deg);
        }
      }
      @media (prefers-reduced-motion: reduce) {
        .syncing fb-icon {
          animation: none;
        }
      }
      .retry {
        font: inherit;
        font-size: var(--fb-text-body-sm);
        font-weight: 600;
        color: var(--fb-blueprint-blue);
        background: none;
        border: none;
        padding: 0;
        cursor: pointer;
        text-decoration: underline;
      }
    `,
  ];

  @property({ type: String }) state: SyncState = 'idle';
  /** Number of unsaved/queued writes; appended as "· N queued" when > 0. */
  @property({ type: Number }) queued = 0;
  /** Override the default copy for the current state. */
  @property({ type: String }) message?: string;

  override render(): TemplateResult {
    if (this.state === 'idle') return html`${nothing}`;
    const meta = SYNC_META[this.state];
    const text = this.message ?? meta.defaultText;
    const queuedText = this.queued > 0 ? ` · ${this.queued} queued` : '';
    return html`<div class=${`chip ${meta.cls}`} role="status" aria-live="polite">
      <fb-icon name=${meta.icon} size="16"></fb-icon>
      <span>${text}${queuedText}</span>
      ${this.state === 'error'
        ? html`<button class="retry" type="button" @click=${() => this.emit('retry')}>
            Retry
          </button>`
        : nothing}
    </div>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-sync-status': FbSyncStatus;
  }
}
