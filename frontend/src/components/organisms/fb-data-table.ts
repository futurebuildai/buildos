import { html, css, nothing } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';

export interface ColumnDef {
  key: string;
  label: string;
  type: 'text' | 'currency' | 'number' | 'date' | 'status' | 'percent';
  sortable?: boolean;
  width?: string;
  currencyCode?: 'USD' | 'CAD';
}

export interface TableRow {
  id: string;
  [key: string]: unknown;
}

type SortDirection = 'asc' | 'desc' | 'none';

/**
 * fb-data-table — Sortable, paginated data table with JetBrains Mono for numeric columns.
 *
 * @property columns - Array of column definitions
 * @property rows - Array of row data objects
 * @property pageSize - Rows per page (default: 20)
 * @fires fb-row-click - Emitted on row click with { row }
 * @fires fb-sort - Emitted on sort change with { key, direction }
 */
@customElement('fb-data-table')
export class FBDataTable extends FBBaseElement {
  static override styles = [
    ...FBBaseElement.styles as [],
    css`
      :host { display: block; }

      .table-container {
        border-radius: var(--fb-radius-md);
        border: 1px solid var(--fb-border);
        overflow: hidden;
      }

      .table-scroll {
        overflow-x: auto;
      }

      table {
        width: 100%;
        border-collapse: collapse;
        font-size: var(--fb-text-sm);
      }

      thead {
        background: var(--fb-slate-steel);
        position: sticky;
        top: 0;
        z-index: 1;
      }

      th {
        padding: var(--fb-space-3) var(--fb-space-4);
        text-align: left;
        font-family: var(--fb-font-body);
        font-weight: 500;
        font-size: var(--fb-text-xs);
        color: var(--fb-text-secondary);
        text-transform: uppercase;
        letter-spacing: 0.04em;
        border-bottom: 1px solid var(--fb-border);
        white-space: nowrap;
        user-select: none;
      }

      th.sortable {
        cursor: pointer;
        transition: color var(--fb-transition-fast);
      }
      th.sortable:hover { color: var(--fb-text-primary); }

      th.sorted { color: var(--fb-gable-green); }

      .sort-icon {
        display: inline-flex;
        vertical-align: middle;
        margin-left: 4px;
      }

      td {
        padding: var(--fb-space-2) var(--fb-space-4);
        border-bottom: 1px solid var(--fb-border);
        vertical-align: middle;
      }

      tr {
        transition: background var(--fb-transition-fast);
      }
      tbody tr:hover {
        background: rgba(255, 255, 255, 0.02);
        cursor: pointer;
      }

      .numeric {
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
        text-align: right;
      }
      th.numeric { text-align: right; }

      .pagination {
        display: flex;
        align-items: center;
        justify-content: space-between;
        padding: var(--fb-space-3) var(--fb-space-4);
        border-top: 1px solid var(--fb-border);
        background: var(--fb-slate-steel);
      }

      .page-info {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-secondary);
      }

      .page-controls {
        display: flex;
        gap: var(--fb-space-1);
      }

      .page-btn {
        background: none;
        border: 1px solid var(--fb-border);
        color: var(--fb-text-secondary);
        padding: 4px 10px;
        border-radius: var(--fb-radius-sm);
        cursor: pointer;
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        transition: all var(--fb-transition-fast);
      }
      .page-btn:hover:not(:disabled) {
        border-color: var(--fb-border-hover);
        color: var(--fb-text-primary);
      }
      .page-btn:disabled {
        opacity: 0.3;
        cursor: not-allowed;
      }

      .empty-state {
        text-align: center;
        padding: var(--fb-space-12) var(--fb-space-6);
        color: var(--fb-text-muted);
        font-family: var(--fb-font-body);
        font-size: var(--fb-text-base);
      }
    `,
  ];

  @property({ type: Array }) columns: ColumnDef[] = [];
  @property({ type: Array }) rows: TableRow[] = [];
  @property({ type: Number }) pageSize = 20;

  @state() private _sortKey = '';
  @state() private _sortDir: SortDirection = 'none';
  @state() private _currentPage = 0;

  private _isNumericType(type: string): boolean {
    return type === 'currency' || type === 'number' || type === 'percent';
  }

  private _getSortedRows(): TableRow[] {
    if (this._sortDir === 'none' || !this._sortKey) return this.rows;

    return [...this.rows].sort((a, b) => {
      const aVal = a[this._sortKey];
      const bVal = b[this._sortKey];
      let cmp = 0;

      if (typeof aVal === 'number' && typeof bVal === 'number') {
        cmp = aVal - bVal;
      } else {
        cmp = String(aVal ?? '').localeCompare(String(bVal ?? ''));
      }

      return this._sortDir === 'desc' ? -cmp : cmp;
    });
  }

  private _getPageRows(): TableRow[] {
    const sorted = this._getSortedRows();
    const start = this._currentPage * this.pageSize;
    return sorted.slice(start, start + this.pageSize);
  }

  override render() {
    const totalPages = Math.ceil(this.rows.length / this.pageSize);
    const pageRows = this._getPageRows();
    const startRow = this._currentPage * this.pageSize + 1;
    const endRow = Math.min(startRow + this.pageSize - 1, this.rows.length);

    return html`
      <div class="table-container">
        <div class="table-scroll">
          <table>
            <thead>
              <tr>
                ${this.columns.map(col => {
                  const isNumeric = this._isNumericType(col.type);
                  const isSorted = this._sortKey === col.key;
                  return html`
                    <th
                      class="${col.sortable ? 'sortable' : ''} ${isSorted ? 'sorted' : ''} ${isNumeric ? 'numeric' : ''}"
                      style=${col.width ? `width: ${col.width}` : ''}
                      @click=${col.sortable ? () => this._onSort(col.key) : nothing}
                    >
                      ${col.label}
                      ${col.sortable && isSorted ? html`
                        <span class="sort-icon">
                          <svg width="12" height="12" viewBox="0 0 24 24" fill="currentColor">
                            ${this._sortDir === 'asc'
                              ? html`<path d="M7 14l5-5 5 5z"/>`
                              : html`<path d="M7 10l5 5 5-5z"/>`
                            }
                          </svg>
                        </span>
                      ` : ''}
                    </th>
                  `;
                })}
              </tr>
            </thead>
            <tbody>
              ${pageRows.length === 0
                ? html`<tr><td colspan=${this.columns.length} class="empty-state">No data available</td></tr>`
                : pageRows.map(row => html`
                    <tr @click=${() => this._onRowClick(row)}>
                      ${this.columns.map(col => {
                        const isNumeric = this._isNumericType(col.type);
                        return html`
                          <td class="${isNumeric ? 'numeric' : ''}">
                            <fb-data-cell
                              type=${col.type}
                              .value=${row[col.key] as string | number}
                              currencyCode=${col.currencyCode ?? 'USD'}
                            ></fb-data-cell>
                          </td>
                        `;
                      })}
                    </tr>
                  `)
              }
            </tbody>
          </table>
        </div>

        ${this.rows.length > this.pageSize ? html`
          <div class="pagination">
            <span class="page-info">${startRow}-${endRow} of ${this.rows.length}</span>
            <div class="page-controls">
              <button
                class="page-btn"
                ?disabled=${this._currentPage === 0}
                @click=${() => { this._currentPage = Math.max(0, this._currentPage - 1); }}
              >Prev</button>
              <button
                class="page-btn"
                ?disabled=${this._currentPage >= totalPages - 1}
                @click=${() => { this._currentPage = Math.min(totalPages - 1, this._currentPage + 1); }}
              >Next</button>
            </div>
          </div>
        ` : ''}
      </div>
    `;
  }

  private _onSort(key: string) {
    if (this._sortKey === key) {
      this._sortDir = this._sortDir === 'asc' ? 'desc' : this._sortDir === 'desc' ? 'none' : 'asc';
    } else {
      this._sortKey = key;
      this._sortDir = 'asc';
    }
    this._currentPage = 0;
    this.emitEvent('fb-sort', { key: this._sortKey, direction: this._sortDir });
  }

  private _onRowClick(row: TableRow) {
    this.emitEvent('fb-row-click', { row });
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-data-table': FBDataTable;
  }
}
