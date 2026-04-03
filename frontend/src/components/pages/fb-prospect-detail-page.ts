import { html, css, nothing } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import {
  getProspect, advanceProspect, loseProspect,
  type Prospect, type Estimate, type Permit, type PipelineStage, ApiError,
} from '../../state/api.js';
import { currentOrg } from '../../state/store.js';
import { navigateTo, getCurrentRoute } from '../../router.js';
import { formatCents, type CurrencyCode } from '../../utils/currency.js';

const STAGE_ORDER: PipelineStage[] = ['LEAD', 'QUALIFIED', 'ESTIMATE_SENT', 'VERBAL_COMMITMENT', 'PERMIT_APPLIED', 'PERMIT_ISSUED'];
const STAGE_LABELS: Record<string, string> = {
  LEAD: 'Lead', QUALIFIED: 'Qualified', ESTIMATE_SENT: 'Estimate Sent',
  VERBAL_COMMITMENT: 'Verbal Commitment', PERMIT_APPLIED: 'Permit Applied', PERMIT_ISSUED: 'Permit Issued', LOST: 'Lost',
};

const PERMIT_STATUS_LABELS: Record<string, string> = {
  not_submitted: 'Not Submitted', submitted: 'Submitted', under_review: 'Under Review',
  revisions_requested: 'Revisions', approved: 'Approved', denied: 'Denied',
};

@customElement('fb-prospect-detail-page')
export class FBProspectDetailPage extends FBBaseElement {
  @state() private _loading = true;
  @state() private _error = '';
  @state() private _prospect: Prospect | null = null;
  @state() private _estimates: Estimate[] = [];
  @state() private _permits: Permit[] = [];
  @state() private _advancing = false;

  static styles = [
    ...FBBaseElement.styles,
    css`
      :host { display: block; padding: var(--fb-space-6); }

      .back-link {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-space-2);
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-sm);
        cursor: pointer;
        margin-bottom: var(--fb-space-4);
        transition: color var(--fb-transition-fast);
      }

      .back-link:hover { color: var(--fb-gable-green); }

      .page-header {
        display: flex;
        justify-content: space-between;
        align-items: flex-start;
        margin-bottom: var(--fb-space-6);
        flex-wrap: wrap;
        gap: var(--fb-space-4);
      }

      .page-header h1 {
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-text-primary);
        margin: 0;
      }

      .stage-badge {
        display: inline-block;
        padding: var(--fb-space-1) var(--fb-space-3);
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-sm);
        font-weight: 600;
        background: rgba(0, 255, 163, 0.1);
        color: var(--fb-gable-green);
      }

      .stage-badge.LOST {
        background: rgba(244, 63, 94, 0.1);
        color: var(--fb-safety-red);
      }

      .header-actions {
        display: flex;
        gap: var(--fb-space-2);
      }

      .advance-btn {
        padding: var(--fb-space-2) var(--fb-space-4);
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-sm);
        font-weight: 600;
        cursor: pointer;
        transition: all var(--fb-transition-fast);
        border: none;
        background: var(--fb-gable-green);
        color: var(--fb-deep-space);
        font-family: var(--fb-font-body);
      }

      .advance-btn:hover { opacity: 0.9; }
      .advance-btn:disabled { opacity: 0.5; cursor: not-allowed; }

      .lose-btn {
        padding: var(--fb-space-2) var(--fb-space-4);
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-sm);
        font-weight: 600;
        cursor: pointer;
        transition: all var(--fb-transition-fast);
        border: 1px solid var(--fb-safety-red);
        background: transparent;
        color: var(--fb-safety-red);
        font-family: var(--fb-font-body);
      }

      .lose-btn:hover { background: rgba(244, 63, 94, 0.1); }

      .info-grid {
        display: grid;
        grid-template-columns: repeat(3, 1fr);
        gap: var(--fb-space-4);
        margin-bottom: var(--fb-space-6);
      }

      @media (max-width: 768px) { .info-grid { grid-template-columns: 1fr; } }

      .info-card {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
      }

      .info-label {
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        text-transform: uppercase;
        letter-spacing: 0.05em;
        margin-bottom: var(--fb-space-1);
      }

      .info-value {
        font-size: var(--fb-text-base);
        color: var(--fb-text-primary);
      }

      .info-value.mono {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
      }

      /* Stage Progress */
      .stage-progress {
        display: flex;
        gap: var(--fb-space-1);
        margin-bottom: var(--fb-space-6);
      }

      .stage-step {
        flex: 1;
        height: 4px;
        border-radius: 2px;
        background: var(--fb-surface);
      }

      .stage-step.complete { background: var(--fb-gable-green); }
      .stage-step.current { background: var(--fb-gable-green); opacity: 0.5; }

      /* Sections */
      .section {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
        margin-bottom: var(--fb-space-6);
      }

      .section-title {
        font-size: var(--fb-text-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-4);
      }

      /* Estimate list */
      .estimate-item {
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-md);
        padding: var(--fb-space-4);
        margin-bottom: var(--fb-space-3);
      }

      .estimate-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: var(--fb-space-2);
      }

      .estimate-version {
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        font-family: var(--fb-font-mono);
      }

      .estimate-total {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-lg);
        font-weight: 600;
        color: var(--fb-gable-green);
      }

      .estimate-status {
        display: inline-block;
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 600;
      }

      .estimate-status.draft { background: rgba(148, 163, 184, 0.1); color: var(--fb-text-secondary); }
      .estimate-status.sent { background: rgba(56, 189, 248, 0.1); color: var(--fb-blueprint-blue); }
      .estimate-status.revised { background: rgba(245, 158, 11, 0.1); color: var(--fb-amber); }
      .estimate-status.accepted { background: rgba(0, 255, 163, 0.1); color: var(--fb-gable-green); }

      .estimate-detail {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
      }

      /* Permit list */
      .permit-item {
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-md);
        padding: var(--fb-space-4);
        margin-bottom: var(--fb-space-3);
      }

      .permit-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: var(--fb-space-2);
      }

      .permit-type {
        font-size: var(--fb-text-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
        text-transform: capitalize;
      }

      .permit-status {
        display: inline-block;
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 600;
      }

      .permit-status.approved { background: rgba(0, 255, 163, 0.1); color: var(--fb-gable-green); }
      .permit-status.denied { background: rgba(244, 63, 94, 0.1); color: var(--fb-safety-red); }
      .permit-status.under_review { background: rgba(56, 189, 248, 0.1); color: var(--fb-blueprint-blue); }
      .permit-status.submitted { background: rgba(245, 158, 11, 0.1); color: var(--fb-amber); }
      .permit-status.not_submitted { background: rgba(148, 163, 184, 0.1); color: var(--fb-text-secondary); }
      .permit-status.revisions_requested { background: rgba(245, 158, 11, 0.1); color: var(--fb-amber); }

      .permit-detail {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
      }

      .permit-detail span {
        display: inline-block;
        margin-right: var(--fb-space-4);
      }

      .error-banner {
        color: var(--fb-safety-red);
        padding: var(--fb-space-4);
        background: rgba(244, 63, 94, 0.1);
        border-radius: var(--fb-radius-md);
        border: 1px solid rgba(244, 63, 94, 0.2);
        margin-bottom: var(--fb-space-4);
        font-size: var(--fb-text-sm);
      }

      .loading-container { display: flex; align-items: center; justify-content: center; min-height: 300px; }
      .empty-state { text-align: center; padding: var(--fb-space-6); color: var(--fb-text-muted); font-size: var(--fb-text-sm); }
    `,
  ];

  connectedCallback() {
    super.connectedCallback();
    this._loadProspect();
  }

  render() {
    if (this._loading) {
      return html`<div class="loading-container"><fb-spinner></fb-spinner></div>`;
    }

    if (!this._prospect) {
      return html`
        <div class="back-link" @click=${() => navigateTo('/pipeline')}>\u2190 Back to Pipeline</div>
        <div class="error-banner">Prospect not found.</div>
      `;
    }

    const p = this._prospect;
    const currentStageIdx = STAGE_ORDER.indexOf(p.stage as PipelineStage);
    const nextStage = currentStageIdx >= 0 && currentStageIdx < STAGE_ORDER.length - 1 ? STAGE_ORDER[currentStageIdx + 1] : null;
    const isTerminal = p.stage === 'PERMIT_ISSUED' || p.stage === 'LOST';

    return html`
      <div class="back-link" @click=${() => navigateTo('/pipeline')}>\u2190 Back to Pipeline</div>

      ${this._error ? html`<div class="error-banner">${this._error}</div>` : nothing}

      <div class="page-header">
        <div>
          <h1>${p.name}</h1>
          <span class="stage-badge ${p.stage}">${STAGE_LABELS[p.stage] ?? p.stage}</span>
        </div>
        ${!isTerminal
          ? html`
            <div class="header-actions">
              ${nextStage
                ? html`
                  <button
                    class="advance-btn"
                    ?disabled=${this._advancing}
                    @click=${() => this._handleAdvance(nextStage)}
                  >Advance to ${STAGE_LABELS[nextStage] ?? nextStage}</button>
                `
                : nothing}
              <button class="lose-btn" @click=${this._handleLose}>Mark Lost</button>
            </div>
          `
          : nothing}
      </div>

      <div class="stage-progress">
        ${STAGE_ORDER.map((_stage, idx) => {
          const cls = idx < currentStageIdx ? 'complete' : idx === currentStageIdx ? 'current' : '';
          return html`<div class="stage-step ${cls}"></div>`;
        })}
      </div>

      <div class="info-grid">
        <div class="info-card">
          <div class="info-label">Client</div>
          <div class="info-value">${p.client_name}</div>
        </div>
        <div class="info-card">
          <div class="info-label">Contact</div>
          <div class="info-value">${p.client_email ?? p.client_phone ?? '\u2014'}</div>
        </div>
        <div class="info-card">
          <div class="info-label">Address</div>
          <div class="info-value">${p.address ?? '\u2014'}</div>
        </div>
        <div class="info-card">
          <div class="info-label">GSF</div>
          <div class="info-value mono">${p.gsf ? p.gsf.toLocaleString() : '\u2014'}</div>
        </div>
        <div class="info-card">
          <div class="info-label">Source</div>
          <div class="info-value">${p.source ?? '\u2014'}</div>
        </div>
        <div class="info-card">
          <div class="info-label">Created</div>
          <div class="info-value mono">${this._formatDate(p.created_at)}</div>
        </div>
      </div>

      <div class="section">
        <h2 class="section-title">Estimates (${this._estimates.length})</h2>
        ${this._estimates.length === 0
          ? html`<div class="empty-state">No estimates created yet.</div>`
          : this._estimates.map(
              (est) => html`
                <div class="estimate-item">
                  <div class="estimate-header">
                    <div>
                      <span class="estimate-version">v${est.version}</span>
                      <span class="estimate-status ${est.status}">${est.status}</span>
                    </div>
                    <div class="estimate-total">${formatCents(est.total_estimated_cents, est.currency_code as CurrencyCode)}</div>
                  </div>
                  <div class="estimate-detail">
                    ${est.line_items.length} line item${est.line_items.length !== 1 ? 's' : ''}
                    \u00B7 ${est.margin_pct}% margin
                    \u00B7 ${est.currency_code}
                  </div>
                </div>
              `,
            )}
      </div>

      <div class="section">
        <h2 class="section-title">Permits (${this._permits.length})</h2>
        ${this._permits.length === 0
          ? html`<div class="empty-state">No permits tracked yet.</div>`
          : this._permits.map(
              (permit) => html`
                <div class="permit-item">
                  <div class="permit-header">
                    <span class="permit-type">${permit.permit_type}</span>
                    <span class="permit-status ${permit.status}">${PERMIT_STATUS_LABELS[permit.status] ?? permit.status}</span>
                  </div>
                  <div class="permit-detail">
                    <span>${permit.jurisdiction}</span>
                    ${permit.application_number ? html`<span class="mono">#${permit.application_number}</span>` : nothing}
                    ${permit.fee_cents != null && permit.fee_currency_code
                      ? html`<span class="mono">${formatCents(permit.fee_cents, permit.fee_currency_code as CurrencyCode)}</span>`
                      : nothing}
                  </div>
                </div>
              `,
            )}
      </div>

      ${p.notes
        ? html`
          <div class="section">
            <h2 class="section-title">Notes</h2>
            <div style="font-size: var(--fb-text-sm); color: var(--fb-text-secondary); line-height: 1.6;">
              ${p.notes}
            </div>
          </div>
        `
        : nothing}
    `;
  }

  private _formatDate(isoString: string): string {
    try {
      return new Date(isoString).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
      return isoString;
    }
  }

  private async _handleAdvance(targetStage: PipelineStage) {
    if (!this._prospect) return;
    this._advancing = true;
    this._error = '';
    const orgID = currentOrg.get();

    try {
      const result = await advanceProspect(orgID, this._prospect.id, { target_stage: targetStage });
      this._prospect = result.prospect;
      if (result.project_id) {
        this.showToast(`Project created! ID: ${result.project_id}`, 'success');
      } else {
        this.showToast(`Advanced to ${STAGE_LABELS[targetStage] ?? targetStage}`, 'success');
      }
    } catch (err) {
      this._error = err instanceof ApiError ? `Advance failed (${err.status})` : 'Failed to advance prospect';
      this.showToast(this._error, 'error');
    } finally {
      this._advancing = false;
    }
  }

  private async _handleLose() {
    if (!this._prospect) return;
    const reason = prompt('Reason for losing this prospect:');
    if (!reason) return;

    this._advancing = true;
    this._error = '';
    const orgID = currentOrg.get();

    try {
      const result = await loseProspect(orgID, this._prospect.id, { reason });
      this._prospect = result.prospect;
      this.showToast('Prospect marked as lost', 'info');
    } catch (err) {
      this._error = err instanceof ApiError ? `Failed (${err.status})` : 'Failed to mark as lost';
      this.showToast(this._error, 'error');
    } finally {
      this._advancing = false;
    }
  }

  private async _loadProspect() {
    this._loading = true;
    this._error = '';
    const orgID = currentOrg.get();
    const match = getCurrentRoute();
    const prospectID = match.params['id'];

    if (!orgID || !prospectID) {
      this._loading = false;
      this._error = !orgID ? 'No organization selected.' : 'No prospect ID provided.';
      return;
    }

    try {
      const res = await getProspect(orgID, prospectID);
      this._prospect = res.prospect;
      this._estimates = res.estimates;
      this._permits = res.permits;
    } catch (err) {
      this._error = err instanceof ApiError ? `Failed to load prospect (${err.status})` : 'Failed to load prospect';
    } finally {
      this._loading = false;
    }
  }
}
