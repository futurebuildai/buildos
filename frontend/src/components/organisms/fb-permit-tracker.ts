import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export interface PermitEntry {
  id: string;
  permitType: string;
  status: string;
  submittedAt: string;
  updatedAt: string;
}

const STATUS_VARIANT: Record<string, string> = {
  approved: 'success',
  issued: 'success',
  pending: 'info',
  submitted: 'info',
  'in-review': 'warning',
  rejected: 'error',
  expired: 'error',
};

/**
 * fb-permit-tracker — Vertical timeline of permit status changes.
 *
 * Shows permits as a timeline with status badges and dates.
 *
 * @property permits - Array of PermitEntry objects
 */
@customElement('fb-permit-tracker')
export class FBPermitTracker extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .timeline {
        display: flex;
        flex-direction: column;
        position: relative;
        padding-left: var(--fb-space-6);
      }

      .permit-item {
        position: relative;
        padding-bottom: var(--fb-space-4);
        padding-left: var(--fb-space-3);
      }

      /* Vertical connector line */
      .permit-item::before {
        content: '';
        position: absolute;
        left: -17px;
        top: 20px;
        width: 1px;
        bottom: 0;
        background: var(--fb-border);
      }
      .permit-item:last-child::before { display: none; }

      /* Status dot */
      .permit-item::after {
        content: '';
        position: absolute;
        left: -22px;
        top: 6px;
        width: 12px;
        height: 12px;
        border-radius: 50%;
        border: 2px solid var(--fb-deep-space);
      }

      .permit-item.status-approved::after,
      .permit-item.status-issued::after { background: var(--fb-gable-green); }
      .permit-item.status-pending::after,
      .permit-item.status-submitted::after { background: var(--fb-blueprint-blue); }
      .permit-item.status-in-review::after { background: var(--fb-amber); }
      .permit-item.status-rejected::after,
      .permit-item.status-expired::after { background: var(--fb-safety-red); }

      .permit-header {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--fb-space-2);
        margin-bottom: var(--fb-space-1);
      }

      .permit-type {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-primary);
      }

      .permit-dates {
        display: flex;
        flex-direction: column;
        gap: 2px;
      }

      .permit-date {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        font-variant-numeric: tabular-nums;
      }

      .permit-date-label {
        font-family: var(--fb-font-body);
        font-size: 10px;
        color: var(--fb-text-muted);
        text-transform: uppercase;
        letter-spacing: 0.04em;
      }

      .empty {
        color: var(--fb-text-muted);
        font-size: var(--fb-text-sm);
        font-style: italic;
        padding: var(--fb-space-4);
        text-align: center;
      }
    `,
  ];

  @property({ type: Array }) permits: PermitEntry[] = [];

  private _formatDate(iso: string): string {
    try {
      return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
      return iso;
    }
  }

  override render() {
    if (this.permits.length === 0) {
      return html`<div class="empty">No permits tracked</div>`;
    }

    return html`
      <div class="timeline">
        ${this.permits.map(permit => {
          const statusKey = permit.status.toLowerCase().replace(/\s+/g, '-');
          const variant = STATUS_VARIANT[statusKey] ?? 'neutral';

          return html`
            <div class="permit-item status-${statusKey}">
              <div class="permit-header">
                <span class="permit-type">${permit.permitType}</span>
                <fb-badge variant=${variant} size="sm">${permit.status}</fb-badge>
              </div>
              <div class="permit-dates">
                <div>
                  <span class="permit-date-label">Submitted</span>
                  <span class="permit-date">${this._formatDate(permit.submittedAt)}</span>
                </div>
                <div>
                  <span class="permit-date-label">Updated</span>
                  <span class="permit-date">${this._formatDate(permit.updatedAt)}</span>
                </div>
              </div>
            </div>
          `;
        })}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-permit-tracker': FBPermitTracker;
  }
}
