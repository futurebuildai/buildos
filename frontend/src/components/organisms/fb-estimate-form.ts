import { html, css } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { formatCents, type CurrencyCode } from '../../utils/currency.js';

export interface EstimateLineItem {
  id: string;
  wbsCode: string;
  description: string;
  estimatedCents: number;
  unit: string;
  quantity: number;
}

/**
 * fb-estimate-form — Line-item estimate editor.
 *
 * Supports add/remove line items with WBS code, description, estimated cents,
 * unit, and quantity. Calculates subtotal, margin, and total.
 *
 * @property lineItems - Array of line items
 * @property currencyCode - Currency for display
 * @property marginPercent - Margin percentage
 * @fires fb-estimate-save - Emitted on save with { lineItems, marginPercent, totalCents }
 * @fires fb-line-item-change - Emitted on any line item change
 */
@customElement('fb-estimate-form')
export class FBEstimateForm extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .form-container {
        padding: var(--fb-space-4);
      }

      .form-title {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
        font-weight: 500;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-4);
      }

      .line-items {
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-2);
        margin-bottom: var(--fb-space-4);
      }

      .line-item-header {
        display: grid;
        grid-template-columns: 80px 1fr 120px 80px 80px 40px;
        gap: var(--fb-space-2);
        padding: var(--fb-space-1) 0;
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-xs);
        font-weight: 500;
        color: var(--fb-text-muted);
        text-transform: uppercase;
        letter-spacing: 0.04em;
      }

      .line-item-row {
        display: grid;
        grid-template-columns: 80px 1fr 120px 80px 80px 40px;
        gap: var(--fb-space-2);
        align-items: center;
        padding: var(--fb-space-2);
        border-radius: var(--fb-radius-sm);
        background: rgba(255, 255, 255, 0.02);
        border: 1px solid var(--fb-border);
      }

      .line-item-row input {
        background: var(--fb-slate-steel);
        border: 1px solid var(--fb-border);
        border-radius: 4px;
        padding: 6px 8px;
        color: var(--fb-text-primary);
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        width: 100%;
        outline: none;
      }
      .line-item-row input:focus {
        border-color: var(--fb-gable-green);
      }

      .line-item-row input[type="number"] {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
      }

      .remove-btn {
        display: flex;
        align-items: center;
        justify-content: center;
        background: none;
        border: none;
        color: var(--fb-text-muted);
        cursor: pointer;
        padding: 4px;
        border-radius: 4px;
      }
      .remove-btn:hover { color: var(--fb-safety-red); background: rgba(244, 63, 94, 0.1); }

      .add-btn {
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
        background: none;
        border: 1px dashed var(--fb-border);
        color: var(--fb-text-secondary);
        padding: var(--fb-space-2) var(--fb-space-3);
        border-radius: var(--fb-radius-sm);
        cursor: pointer;
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        width: 100%;
        transition: all var(--fb-transition-fast);
      }
      .add-btn:hover { border-color: var(--fb-gable-green); color: var(--fb-gable-green); }

      .totals {
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-2);
        padding: var(--fb-space-3);
        border-top: 1px solid var(--fb-border);
      }

      .total-row {
        display: flex;
        justify-content: space-between;
        align-items: center;
      }

      .total-label {
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
      }

      .total-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-base);
        font-variant-numeric: tabular-nums;
        color: var(--fb-text-primary);
      }

      .total-final {
        font-size: var(--fb-text-lg);
        font-weight: 500;
        color: var(--fb-gable-green);
      }

      .margin-input {
        display: flex;
        align-items: center;
        gap: var(--fb-space-2);
      }
      .margin-input input {
        width: 60px;
        background: var(--fb-slate-steel);
        border: 1px solid var(--fb-border);
        border-radius: 4px;
        padding: 4px 8px;
        color: var(--fb-text-primary);
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-sm);
        text-align: right;
        outline: none;
      }
      .margin-input input:focus { border-color: var(--fb-gable-green); }
      .margin-input span { color: var(--fb-text-muted); font-size: var(--fb-text-sm); }

      .actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--fb-space-2);
        margin-top: var(--fb-space-4);
      }

      @media (max-width: 768px) {
        .line-item-header, .line-item-row {
          grid-template-columns: 1fr;
        }
        .line-item-header { display: none; }
      }
    `,
  ];

  @property({ type: Array }) lineItems: EstimateLineItem[] = [];
  @property({ type: String }) currencyCode: CurrencyCode = 'USD';
  @property({ type: Number }) marginPercent = 15;

  @state() private _items: EstimateLineItem[] = [];
  @state() private _margin = 15;

  override willUpdate(changed: Map<string, unknown>) {
    if (changed.has('lineItems')) {
      this._items = [...this.lineItems];
    }
    if (changed.has('marginPercent')) {
      this._margin = this.marginPercent;
    }
  }

  private _getSubtotalCents(): number {
    return this._items.reduce((sum, item) => sum + (item.estimatedCents * item.quantity), 0);
  }

  private _getMarginCents(): number {
    return Math.round(this._getSubtotalCents() * (this._margin / 100));
  }

  private _getTotalCents(): number {
    return this._getSubtotalCents() + this._getMarginCents();
  }

  override render() {
    return html`
      <div class="form-container glass-card">
        <div class="form-title">Estimate Line Items</div>

        <div class="line-items">
          <div class="line-item-header">
            <span>WBS</span>
            <span>Description</span>
            <span>Unit Cost</span>
            <span>Unit</span>
            <span>Qty</span>
            <span></span>
          </div>

          ${this._items.map((item, i) => html`
            <div class="line-item-row">
              <input
                type="text"
                .value=${item.wbsCode}
                placeholder="1.0"
                @input=${(e: Event) => this._updateItem(i, 'wbsCode', (e.target as HTMLInputElement).value)}
              />
              <input
                type="text"
                .value=${item.description}
                placeholder="Description"
                @input=${(e: Event) => this._updateItem(i, 'description', (e.target as HTMLInputElement).value)}
              />
              <input
                type="number"
                .value=${String(item.estimatedCents / 100)}
                placeholder="0.00"
                step="0.01"
                @input=${(e: Event) => this._updateItem(i, 'estimatedCents', Math.round(parseFloat((e.target as HTMLInputElement).value || '0') * 100))}
              />
              <input
                type="text"
                .value=${item.unit}
                placeholder="ea"
                @input=${(e: Event) => this._updateItem(i, 'unit', (e.target as HTMLInputElement).value)}
              />
              <input
                type="number"
                .value=${String(item.quantity)}
                placeholder="1"
                min="0"
                @input=${(e: Event) => this._updateItem(i, 'quantity', parseInt((e.target as HTMLInputElement).value || '0', 10))}
              />
              <button class="remove-btn" @click=${() => this._removeItem(i)} aria-label="Remove line">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M6 19c0 1.1.9 2 2 2h8c1.1 0 2-.9 2-2V7H6v12zM19 4h-3.5l-1-1h-5l-1 1H5v2h14V4z"/>
                </svg>
              </button>
            </div>
          `)}

          <button class="add-btn" @click=${this._addItem}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
              <path d="M19 13h-6v6h-2v-6H5v-2h6V5h2v6h6v2z"/>
            </svg>
            Add Line Item
          </button>
        </div>

        <div class="totals">
          <div class="total-row">
            <span class="total-label">Subtotal</span>
            <span class="total-value">${formatCents(this._getSubtotalCents(), this.currencyCode)}</span>
          </div>
          <div class="total-row">
            <div class="margin-input">
              <span class="total-label">Margin</span>
              <input
                type="number"
                .value=${String(this._margin)}
                min="0"
                max="100"
                @input=${(e: Event) => { this._margin = parseFloat((e.target as HTMLInputElement).value || '0'); }}
              />
              <span>%</span>
            </div>
            <span class="total-value">${formatCents(this._getMarginCents(), this.currencyCode)}</span>
          </div>
          <div class="total-row">
            <span class="total-label" style="font-weight:500;">Total</span>
            <span class="total-value total-final">${formatCents(this._getTotalCents(), this.currencyCode)}</span>
          </div>
        </div>

        <div class="actions">
          <fb-button variant="secondary" @fb-click=${this._onCancel}>Cancel</fb-button>
          <fb-button variant="primary" @fb-click=${this._onSave}>Save Estimate</fb-button>
        </div>
      </div>
    `;
  }

  private _addItem() {
    const newId = `line-${Date.now()}`;
    this._items = [...this._items, {
      id: newId,
      wbsCode: '',
      description: '',
      estimatedCents: 0,
      unit: 'ea',
      quantity: 1,
    }];
  }

  private _removeItem(index: number) {
    this._items = this._items.filter((_, i) => i !== index);
  }

  private _updateItem(index: number, field: keyof EstimateLineItem, value: string | number) {
    this._items = this._items.map((item, i) => {
      if (i !== index) return item;
      return { ...item, [field]: value };
    });
    this.emitEvent('fb-line-item-change', { items: this._items });
  }

  private _onSave() {
    this.emitEvent('fb-estimate-save', {
      lineItems: this._items,
      marginPercent: this._margin,
      totalCents: this._getTotalCents(),
    });
  }

  private _onCancel() {
    this.emitEvent('fb-estimate-cancel');
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-estimate-form': FBEstimateForm;
  }
}
