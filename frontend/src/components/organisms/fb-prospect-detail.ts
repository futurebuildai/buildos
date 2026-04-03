import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { formatCents, type CurrencyCode } from '../../utils/currency.js';

export interface ProspectContact {
  name: string;
  email: string;
  phone: string;
  role: string;
}

export interface ProspectEstimate {
  id: string;
  description: string;
  totalCents: number;
  currencyCode: CurrencyCode;
  status: string;
  createdAt: string;
}

export interface ProspectPermit {
  id: string;
  permitType: string;
  status: string;
  submittedAt: string;
  updatedAt: string;
}

export interface ProspectData {
  id: string;
  clientName: string;
  projectName: string;
  stage: string;
  probability: number;
  contact: ProspectContact;
  estimates: ProspectEstimate[];
  permits: ProspectPermit[];
  stageHistory: Array<{ stage: string; changedAt: string }>;
}

/**
 * fb-prospect-detail — Detailed prospect view.
 *
 * Shows contact info, estimates list, permits list, and stage timeline.
 *
 * @property prospect - Full ProspectData object
 */
@customElement('fb-prospect-detail')
export class FBProspectDetail extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .detail-grid {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: var(--fb-space-4);
      }

      @media (max-width: 900px) {
        .detail-grid { grid-template-columns: 1fr; }
      }

      .section {
        padding: var(--fb-space-4);
      }

      .section-title {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        font-weight: 500;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-3);
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
      }

      .contact-grid {
        display: grid;
        grid-template-columns: auto 1fr;
        gap: var(--fb-space-1) var(--fb-space-3);
        font-size: var(--fb-text-sm);
      }

      .contact-label {
        color: var(--fb-text-muted);
        font-weight: 500;
      }
      .contact-value {
        color: var(--fb-text-primary);
      }

      .estimate-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: var(--fb-space-2) 0;
        border-bottom: 1px solid var(--fb-border);
      }
      .estimate-row:last-child { border-bottom: none; }

      .estimate-desc {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        color: var(--fb-text-primary);
      }
      .estimate-date {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
      }
      .estimate-amount {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-sm);
        font-variant-numeric: tabular-nums;
        color: var(--fb-gable-green);
      }

      .timeline {
        display: flex;
        flex-direction: column;
        gap: 0;
        position: relative;
        padding-left: var(--fb-space-6);
      }

      .timeline-item {
        position: relative;
        padding-bottom: var(--fb-space-4);
      }
      .timeline-item::before {
        content: '';
        position: absolute;
        left: -21px;
        top: 6px;
        width: 10px;
        height: 10px;
        border-radius: 50%;
        background: var(--fb-gable-green);
        border: 2px solid var(--fb-deep-space);
      }
      .timeline-item::after {
        content: '';
        position: absolute;
        left: -16px;
        top: 16px;
        width: 1px;
        bottom: 0;
        background: var(--fb-border);
      }
      .timeline-item:last-child::after { display: none; }

      .timeline-stage {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-primary);
      }
      .timeline-date {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
      }

      .full-width { grid-column: 1 / -1; }

      .empty {
        color: var(--fb-text-muted);
        font-size: var(--fb-text-sm);
        font-style: italic;
      }
    `,
  ];

  @property({ type: Object }) prospect: ProspectData | null = null;

  private _formatDate(iso: string): string {
    try {
      return new Date(iso).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
      return iso;
    }
  }

  override render() {
    if (!this.prospect) {
      return html`<div class="empty">No prospect selected</div>`;
    }

    const p = this.prospect;

    return html`
      <div class="detail-grid">
        <!-- Contact Info -->
        <div class="section glass-card">
          <div class="section-title">
            <fb-icon name="people" size="sm"></fb-icon>
            Contact
          </div>
          <div class="contact-grid">
            <span class="contact-label">Name</span>
            <span class="contact-value">${p.contact.name}</span>
            <span class="contact-label">Email</span>
            <span class="contact-value">${p.contact.email}</span>
            <span class="contact-label">Phone</span>
            <span class="contact-value">${p.contact.phone}</span>
            <span class="contact-label">Role</span>
            <span class="contact-value">${p.contact.role}</span>
          </div>
        </div>

        <!-- Stage Timeline -->
        <div class="section glass-card">
          <div class="section-title">
            <fb-icon name="schedule" size="sm"></fb-icon>
            Stage History
          </div>
          ${p.stageHistory.length === 0
            ? html`<div class="empty">No history</div>`
            : html`
                <div class="timeline">
                  ${p.stageHistory.map(h => html`
                    <div class="timeline-item">
                      <span class="timeline-stage">${h.stage}</span>
                      <span class="timeline-date">${this._formatDate(h.changedAt)}</span>
                    </div>
                  `)}
                </div>
              `
          }
        </div>

        <!-- Estimates -->
        <div class="section glass-card full-width">
          <div class="section-title">
            <fb-icon name="money" size="sm"></fb-icon>
            Estimates
          </div>
          ${p.estimates.length === 0
            ? html`<div class="empty">No estimates yet</div>`
            : p.estimates.map(est => html`
                <div class="estimate-row">
                  <div>
                    <div class="estimate-desc">${est.description}</div>
                    <div class="estimate-date">${this._formatDate(est.createdAt)}</div>
                  </div>
                  <div style="display:flex; align-items:center; gap: var(--fb-space-3);">
                    <fb-badge variant=${est.status === 'approved' ? 'success' : est.status === 'rejected' ? 'error' : 'info'}>
                      ${est.status}
                    </fb-badge>
                    <span class="estimate-amount">${formatCents(est.totalCents, est.currencyCode)}</span>
                  </div>
                </div>
              `)
          }
        </div>

        <!-- Permits -->
        <div class="section glass-card full-width">
          <div class="section-title">
            <fb-icon name="check" size="sm"></fb-icon>
            Permits
          </div>
          ${p.permits.length === 0
            ? html`<div class="empty">No permits filed</div>`
            : html`
                <fb-permit-tracker .permits=${p.permits}></fb-permit-tracker>
              `
          }
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-prospect-detail': FBProspectDetail;
  }
}
