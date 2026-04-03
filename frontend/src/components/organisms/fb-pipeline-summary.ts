import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { formatCents, formatCentsCompact, type CurrencyCode } from '../../utils/currency.js';
import type { PipelineProspect, PipelineStage } from './fb-pipeline-kanban.js';

const STAGE_LABELS: Record<PipelineStage, string> = {
  LEAD: 'Lead',
  QUALIFIED: 'Qualified',
  ESTIMATE_SENT: 'Estimate Sent',
  VERBAL_COMMITMENT: 'Verbal Commit',
  PERMIT_APPLIED: 'Permit Applied',
  PERMIT_ISSUED: 'Permit Issued',
};

const STAGE_ORDER: PipelineStage[] = ['LEAD', 'QUALIFIED', 'ESTIMATE_SENT', 'VERBAL_COMMITMENT', 'PERMIT_APPLIED', 'PERMIT_ISSUED'];

/**
 * fb-pipeline-summary — Revenue summary per currency.
 *
 * Shows total weighted revenue and breakdown by pipeline stage.
 *
 * @property prospects - Array of PipelineProspect
 * @property currencyCode - Which currency to summarize
 */
@customElement('fb-pipeline-summary')
export class FBPipelineSummary extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .summary {
        padding: var(--fb-space-5);
      }

      .summary-header {
        display: flex;
        justify-content: space-between;
        align-items: baseline;
        margin-bottom: var(--fb-space-4);
      }

      .summary-label {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-secondary);
        text-transform: uppercase;
        letter-spacing: 0.04em;
      }

      .summary-total {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-2xl);
        font-weight: 500;
        font-variant-numeric: tabular-nums;
        color: var(--fb-gable-green);
      }

      .breakdown {
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-2);
      }

      .stage-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: var(--fb-space-1) 0;
      }

      .stage-label {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
      }

      .stage-info {
        display: flex;
        align-items: center;
        gap: var(--fb-space-3);
      }

      .stage-count {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        font-variant-numeric: tabular-nums;
      }

      .stage-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-sm);
        font-variant-numeric: tabular-nums;
        color: var(--fb-text-primary);
        min-width: 80px;
        text-align: right;
      }

      .bar-bg {
        height: 4px;
        background: rgba(255, 255, 255, 0.05);
        border-radius: 2px;
        margin-top: var(--fb-space-1);
      }
      .bar-fill {
        height: 100%;
        background: var(--fb-gable-green);
        border-radius: 2px;
        transition: width var(--fb-transition-normal);
      }
    `,
  ];

  @property({ type: Array }) prospects: PipelineProspect[] = [];
  @property({ type: String }) currencyCode: CurrencyCode = 'USD';

  private _getFiltered(): PipelineProspect[] {
    return this.prospects.filter(p => p.currencyCode === this.currencyCode);
  }

  private _getWeightedTotal(prospects: PipelineProspect[]): number {
    return prospects.reduce((sum, p) => sum + Math.round(p.estimatedCents * (p.probability / 100)), 0);
  }

  override render() {
    const filtered = this._getFiltered();
    const totalWeighted = this._getWeightedTotal(filtered);

    return html`
      <div class="summary glass-card">
        <div class="summary-header">
          <span class="summary-label">Weighted Pipeline (${this.currencyCode})</span>
          <span class="summary-total">${formatCents(totalWeighted, this.currencyCode)}</span>
        </div>

        <div class="breakdown">
          ${STAGE_ORDER.map(stageId => {
            const stageProspects = filtered.filter(p => p.stage === stageId);
            const stageWeighted = this._getWeightedTotal(stageProspects);
            const pct = totalWeighted > 0 ? (stageWeighted / totalWeighted) * 100 : 0;

            return html`
              <div>
                <div class="stage-row">
                  <span class="stage-label">${STAGE_LABELS[stageId]}</span>
                  <div class="stage-info">
                    <span class="stage-count">${stageProspects.length}</span>
                    <span class="stage-value">${formatCentsCompact(stageWeighted, this.currencyCode)}</span>
                  </div>
                </div>
                <div class="bar-bg">
                  <div class="bar-fill" style="width: ${pct}%"></div>
                </div>
              </div>
            `;
          })}
        </div>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-pipeline-summary': FBPipelineSummary;
  }
}
