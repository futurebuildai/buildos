import { html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { formatCentsCompact, type CurrencyCode } from '../../utils/currency.js';

export type PipelineStage = 'LEAD' | 'QUALIFIED' | 'ESTIMATE_SENT' | 'VERBAL_COMMITMENT' | 'PERMIT_APPLIED' | 'PERMIT_ISSUED';

export interface PipelineProspect {
  id: string;
  clientName: string;
  projectName: string;
  estimatedCents: number;
  currencyCode: CurrencyCode;
  probability: number;
  stage: PipelineStage;
}

const STAGES: Array<{ id: PipelineStage; label: string }> = [
  { id: 'LEAD', label: 'Lead' },
  { id: 'QUALIFIED', label: 'Qualified' },
  { id: 'ESTIMATE_SENT', label: 'Estimate Sent' },
  { id: 'VERBAL_COMMITMENT', label: 'Verbal Commit' },
  { id: 'PERMIT_APPLIED', label: 'Permit Applied' },
  { id: 'PERMIT_ISSUED', label: 'Permit Issued' },
];

/**
 * fb-pipeline-kanban — 6-stage drag board for pre-construction pipeline.
 *
 * Cards can be dragged between columns to change prospect stage.
 * Each column shows prospect count and weighted revenue.
 *
 * @property prospects - Array of PipelineProspect
 * @fires fb-stage-change - Emitted when a prospect is moved with { prospectId, newStage }
 * @fires fb-prospect-click - Bubbled from pipeline cards
 */
@customElement('fb-pipeline-kanban')
export class FBPipelineKanban extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .kanban {
        display: flex;
        gap: var(--fb-space-3);
        overflow-x: auto;
        padding-bottom: var(--fb-space-2);
        min-height: 400px;
      }

      .column {
        flex: 1;
        min-width: 220px;
        max-width: 300px;
        background: rgba(22, 24, 33, 0.4);
        border-radius: var(--fb-radius-md);
        border: 1px solid var(--fb-border);
        display: flex;
        flex-direction: column;
        max-height: 600px;
      }

      .column-header {
        padding: var(--fb-space-3);
        border-bottom: 1px solid var(--fb-border);
        flex-shrink: 0;
      }

      .column-title {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-1);
      }

      .column-stats {
        display: flex;
        justify-content: space-between;
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        font-variant-numeric: tabular-nums;
      }

      .column-body {
        flex: 1;
        padding: var(--fb-space-2);
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-2);
        overflow-y: auto;
      }

      .column.drag-over {
        background: rgba(0, 255, 163, 0.03);
        border-color: rgba(0, 255, 163, 0.2);
      }

      .empty-col {
        display: flex;
        align-items: center;
        justify-content: center;
        flex: 1;
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        padding: var(--fb-space-4);
        text-align: center;
      }
    `,
  ];

  @property({ type: Array }) prospects: PipelineProspect[] = [];

  @state() private _dragOverStage: PipelineStage | null = null;

  private _getStageProspects(stageId: PipelineStage): PipelineProspect[] {
    return this.prospects.filter(p => p.stage === stageId);
  }

  private _getWeightedRevenue(prospects: PipelineProspect[]): number {
    return prospects.reduce((sum, p) => sum + Math.round(p.estimatedCents * (p.probability / 100)), 0);
  }

  override render() {
    return html`
      <div class="kanban">
        ${STAGES.map(stage => {
          const stageProspects = this._getStageProspects(stage.id);
          const weighted = this._getWeightedRevenue(stageProspects);

          return html`
            <div
              class="column ${this._dragOverStage === stage.id ? 'drag-over' : ''}"
              @dragover=${(e: DragEvent) => this._onDragOver(e, stage.id)}
              @dragleave=${() => this._onDragLeave()}
              @drop=${(e: DragEvent) => this._onDrop(e, stage.id)}
            >
              <div class="column-header">
                <div class="column-title">${stage.label}</div>
                <div class="column-stats">
                  <span>${stageProspects.length} prospect${stageProspects.length !== 1 ? 's' : ''}</span>
                  <span>${formatCentsCompact(weighted, 'USD')}</span>
                </div>
              </div>
              <div class="column-body">
                ${stageProspects.length === 0
                  ? html`<div class="empty-col">No prospects</div>`
                  : stageProspects.map(prospect => html`
                      <fb-pipeline-card
                        prospectId=${prospect.id}
                        clientName=${prospect.clientName}
                        projectName=${prospect.projectName}
                        .estimatedCents=${prospect.estimatedCents}
                        currencyCode=${prospect.currencyCode}
                        .probability=${prospect.probability}
                      ></fb-pipeline-card>
                    `)
                }
              </div>
            </div>
          `;
        })}
      </div>
    `;
  }

  private _onDragOver(e: DragEvent, stageId: PipelineStage) {
    e.preventDefault();
    e.dataTransfer!.dropEffect = 'move';
    this._dragOverStage = stageId;
  }

  private _onDragLeave() {
    this._dragOverStage = null;
  }

  private _onDrop(e: DragEvent, newStage: PipelineStage) {
    e.preventDefault();
    this._dragOverStage = null;
    const prospectId = e.dataTransfer?.getData('text/plain');
    if (prospectId) {
      this.emitEvent('fb-stage-change', { prospectId, newStage });
    }
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-pipeline-kanban': FBPipelineKanban;
  }
}
