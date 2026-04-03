import { html, css, nothing } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import {
  listProspects, getPipelineAnalytics,
  type Prospect, type PipelineAnalytics, type PipelineStage, ApiError,
} from '../../state/api.js';
import { currentOrg } from '../../state/store.js';
import { navigateTo } from '../../router.js';
import { formatCentsCompact, type CurrencyCode } from '../../utils/currency.js';

const PIPELINE_STAGES: PipelineStage[] = ['LEAD', 'QUALIFIED', 'ESTIMATE_SENT', 'VERBAL_COMMITMENT', 'PERMIT_APPLIED', 'PERMIT_ISSUED'];
const STAGE_LABELS: Record<string, string> = {
  LEAD: 'Lead',
  QUALIFIED: 'Qualified',
  ESTIMATE_SENT: 'Estimate Sent',
  VERBAL_COMMITMENT: 'Verbal',
  PERMIT_APPLIED: 'Permit Applied',
  PERMIT_ISSUED: 'Permit Issued',
};
const STAGE_PROB: Record<string, number> = {
  LEAD: 10, QUALIFIED: 25, ESTIMATE_SENT: 50, VERBAL_COMMITMENT: 75, PERMIT_APPLIED: 85, PERMIT_ISSUED: 100,
};

@customElement('fb-pipeline-view')
export class FBPipelineView extends FBBaseElement {
  @state() private _loading = true;
  @state() private _error = '';
  @state() private _prospects: Prospect[] = [];
  @state() private _analytics: PipelineAnalytics | null = null;

  static styles = [
    ...FBBaseElement.styles,
    css`
      :host { display: block; padding: var(--fb-space-6); }

      .page-header {
        margin-bottom: var(--fb-space-6);
      }

      .page-header h1 {
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-text-primary);
        margin: 0;
      }

      .page-header p {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        margin-top: var(--fb-space-1);
      }

      .summary-row {
        display: flex;
        gap: var(--fb-space-4);
        margin-bottom: var(--fb-space-6);
        flex-wrap: wrap;
      }

      .revenue-card {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
        flex: 1;
        min-width: 200px;
      }

      .revenue-label {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        margin-bottom: var(--fb-space-2);
      }

      .revenue-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-gable-green);
        font-variant-numeric: tabular-nums;
      }

      .revenue-currency {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        margin-top: var(--fb-space-1);
      }

      .kanban-container {
        display: grid;
        grid-template-columns: repeat(6, 1fr);
        gap: var(--fb-space-4);
        margin-bottom: var(--fb-space-6);
        overflow-x: auto;
      }

      @media (max-width: 1200px) {
        .kanban-container {
          grid-template-columns: repeat(3, 1fr);
        }
      }

      @media (max-width: 768px) {
        .kanban-container {
          grid-template-columns: 1fr 1fr;
        }
      }

      .kanban-column {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-4);
        min-height: 200px;
      }

      .column-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: var(--fb-space-3);
        padding-bottom: var(--fb-space-3);
        border-bottom: 1px solid var(--fb-border);
      }

      .column-title {
        font-size: var(--fb-text-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
      }

      .column-count {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        background: var(--fb-surface);
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
        color: var(--fb-text-muted);
      }

      .column-prob {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
      }

      .prospect-card {
        background: var(--fb-surface);
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-md);
        padding: var(--fb-space-3);
        margin-bottom: var(--fb-space-2);
        cursor: pointer;
        transition: all var(--fb-transition-fast);
      }

      .prospect-card:hover {
        border-color: var(--fb-gable-green);
        box-shadow: var(--fb-shadow-glow);
      }

      .prospect-name {
        font-size: var(--fb-text-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-1);
      }

      .prospect-client {
        font-size: var(--fb-text-xs);
        color: var(--fb-text-secondary);
      }

      .prospect-gsf {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        margin-top: var(--fb-space-1);
      }

      .column-empty {
        text-align: center;
        padding: var(--fb-space-4);
        color: var(--fb-text-muted);
        font-size: var(--fb-text-xs);
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
    `,
  ];

  connectedCallback() {
    super.connectedCallback();
    this._loadData();
  }

  render() {
    if (this._loading) {
      return html`<div class="loading-container"><fb-spinner></fb-spinner></div>`;
    }

    return html`
      <div class="page-header">
        <h1>Pre-Construction Pipeline</h1>
        <p>Track prospects from lead capture through permit issuance.</p>
      </div>

      ${this._error ? html`<div class="error-banner">${this._error}</div>` : nothing}

      ${this._analytics ? this._renderSummary() : nothing}

      <div class="kanban-container">
        ${PIPELINE_STAGES.map((stage) => this._renderColumn(stage))}
      </div>
    `;
  }

  private _renderSummary() {
    if (!this._analytics) return nothing;
    return html`
      <div class="summary-row">
        ${this._analytics.by_currency.map(
          (curr) => html`
            <div class="revenue-card">
              <div class="revenue-label">Weighted Revenue (${curr.currency_code})</div>
              <div class="revenue-value">${formatCentsCompact(curr.total_weighted_revenue_cents, curr.currency_code as CurrencyCode)}</div>
              <div class="revenue-currency">
                ${curr.stages.length} stage${curr.stages.length !== 1 ? 's' : ''} with prospects
              </div>
            </div>
          `,
        )}
        <div class="revenue-card">
          <div class="revenue-label">Total Prospects</div>
          <div class="revenue-value" style="color: var(--fb-blueprint-blue);">${this._prospects.filter((p) => p.stage !== 'LOST').length}</div>
          <div class="revenue-currency">${this._prospects.filter((p) => p.stage === 'LOST').length} lost</div>
        </div>
      </div>
    `;
  }

  private _renderColumn(stage: PipelineStage) {
    const stageProspects = this._prospects.filter((p) => p.stage === stage);
    return html`
      <div class="kanban-column">
        <div class="column-header">
          <div>
            <div class="column-title">${STAGE_LABELS[stage] ?? stage}</div>
            <div class="column-prob">${STAGE_PROB[stage] ?? 0}% probability</div>
          </div>
          <span class="column-count">${stageProspects.length}</span>
        </div>
        ${stageProspects.length === 0
          ? html`<div class="column-empty">No prospects</div>`
          : stageProspects.map(
              (p) => html`
                <div class="prospect-card" @click=${() => navigateTo(`/pipeline/${p.id}`)}>
                  <div class="prospect-name">${p.name}</div>
                  <div class="prospect-client">${p.client_name}</div>
                  ${p.gsf ? html`<div class="prospect-gsf">${p.gsf.toLocaleString()} GSF</div>` : nothing}
                </div>
              `,
            )}
      </div>
    `;
  }

  private async _loadData() {
    this._loading = true;
    this._error = '';
    const orgID = currentOrg.get();
    if (!orgID) {
      this._loading = false;
      this._error = 'No organization selected.';
      return;
    }

    try {
      const [prospectsRes, analyticsRes] = await Promise.allSettled([
        listProspects(orgID),
        getPipelineAnalytics(orgID),
      ]);

      if (prospectsRes.status === 'fulfilled') {
        this._prospects = prospectsRes.value.prospects;
      } else {
        this._error = 'Failed to load prospects';
      }

      if (analyticsRes.status === 'fulfilled') {
        this._analytics = analyticsRes.value;
      }
    } catch (err) {
      this._error = err instanceof ApiError ? `Failed to load pipeline (${err.status})` : 'Failed to load pipeline data';
    } finally {
      this._loading = false;
    }
  }
}
