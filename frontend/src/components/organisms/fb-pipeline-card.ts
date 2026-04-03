import { html, css } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { formatCentsCompact, type CurrencyCode } from '../../utils/currency.js';

/**
 * fb-pipeline-card — Prospect card for the kanban board.
 *
 * Shows client name, project name, estimated value, and probability badge.
 * Draggable between pipeline stages.
 *
 * @property prospectId - Unique prospect ID
 * @property clientName - Client/company name
 * @property projectName - Project name
 * @property estimatedCents - Estimated project value in cents
 * @property currencyCode - Currency code
 * @property probability - Win probability percentage (0-100)
 * @fires fb-prospect-click - Emitted on card click with { prospectId }
 */
@customElement('fb-pipeline-card')
export class FBPipelineCard extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .card {
        padding: var(--fb-space-3);
        cursor: grab;
        transition: transform var(--fb-transition-fast), box-shadow var(--fb-transition-fast);
      }
      .card:hover {
        transform: translateY(-2px);
        box-shadow: var(--fb-shadow-md);
      }
      .card:active { cursor: grabbing; }

      .client-name {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        text-transform: uppercase;
        letter-spacing: 0.04em;
        margin-bottom: 2px;
      }

      .project-name {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        font-weight: 500;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-2);
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
      }

      .footer {
        display: flex;
        align-items: center;
        justify-content: space-between;
      }

      .value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-sm);
        font-variant-numeric: tabular-nums;
        color: var(--fb-gable-green);
      }

      .probability {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        font-variant-numeric: tabular-nums;
        padding: 2px 6px;
        border-radius: 9999px;
        background: rgba(56, 189, 248, 0.12);
        color: var(--fb-blueprint-blue);
      }
    `,
  ];

  @property({ type: String }) prospectId = '';
  @property({ type: String }) clientName = '';
  @property({ type: String }) projectName = '';
  @property({ type: Number }) estimatedCents = 0;
  @property({ type: String }) currencyCode: CurrencyCode = 'USD';
  @property({ type: Number }) probability = 0;

  override render() {
    return html`
      <div
        class="card glass-card"
        draggable="true"
        @click=${this._onClick}
        @dragstart=${this._onDragStart}
      >
        <div class="client-name">${this.clientName}</div>
        <div class="project-name">${this.projectName}</div>
        <div class="footer">
          <span class="value">${formatCentsCompact(this.estimatedCents, this.currencyCode)}</span>
          <span class="probability">${this.probability}%</span>
        </div>
      </div>
    `;
  }

  private _onClick() {
    this.emitEvent('fb-prospect-click', { prospectId: this.prospectId });
  }

  private _onDragStart(e: DragEvent) {
    e.dataTransfer?.setData('text/plain', this.prospectId);
    e.dataTransfer!.effectAllowed = 'move';
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-pipeline-card': FBPipelineCard;
  }
}
