import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import './../atoms/fb-badge.js';
import './../atoms/fb-role-badge.js';
import './../atoms/fb-money.js';
import type { BadgeStatus } from '../atoms/fb-badge.js';
import type { Role, CurrencyCode } from '../../types/models.js';

export type ColumnType = 'text' | 'number' | 'money' | 'date' | 'status' | 'role' | 'actions';
export type SortDir = 'asc' | 'desc';

export interface Column {
  key: string;
  label: string;
  type?: ColumnType;
  sortable?: boolean;
  /** Override automatic alignment (number/money/date right-align by default). */
  align?: 'left' | 'right';
  /** For `money` columns: row field holding the ISO currency code. */
  currencyKey?: string;
}

export type Row = Record<string, unknown> & { id?: string | number };

/**
 * `fb-data-table` — the accessible data grid (DSC §7.7).
 *
 * Real `<table>` semantics: `<th scope="col">` on every header, sortable headers
 * are real `<button>`s carrying `aria-sort`, and `aria-rowcount` / `aria-rowindex`
 * reflect the *true* total (set `row-count` when server-paginating or virtualizing
 * so SR users get accurate counts despite a partial DOM). Numeric/money/date
 * columns right-align with tabular-nums. Column types map to the right atom:
 * `money`→`fb-money`, `status`→`fb-badge`, `role`→`fb-role-badge`.
 *
 * Sorting is controlled: the table emits `sort` ({ key, dir }) and reflects the
 * `sort-key`/`sort-dir` props the parent feeds back (server-side sort >200 rows).
 * Selection emits `select` ({ keys }); the optional select-all header and the
 * row checkboxes are gated by the `selectable` prop (bulk actions are RBAC-gated
 * by the consuming screen, not here).
 */
@customElement('fb-data-table')
export class FbDataTable extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .wrap {
        overflow: auto;
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
      }
      table {
        width: 100%;
        border-collapse: collapse;
        font-size: var(--fb-text-body-md);
      }
      thead th {
        position: sticky;
        top: 0;
        z-index: var(--fb-z-sticky);
        background: var(--fb-surface-2);
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-label-lg);
        font-weight: 600;
        text-align: left;
        white-space: nowrap;
        border-bottom: 1px solid var(--md-sys-color-outline);
      }
      th,
      td {
        height: var(--fb-density-row);
        padding: 0 var(--fb-spacing-md);
        border-bottom: 1px solid var(--fb-border);
        color: var(--fb-text-primary);
        vertical-align: middle;
      }
      tbody tr:last-child td {
        border-bottom: none;
      }
      tbody tr:hover {
        background: var(--fb-surface-1);
      }
      tbody tr[aria-selected='true'] {
        background: color-mix(in srgb, var(--fb-gable-green) 8%, transparent);
      }
      .num,
      .right {
        text-align: right;
        font-family: var(--fb-font-mono);
        font-variant-numeric: tabular-nums;
      }
      th.right {
        text-align: right;
      }
      .sort-btn {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        font: inherit;
        color: inherit;
        background: none;
        border: none;
        padding: 0;
        cursor: pointer;
      }
      .sort-btn:hover {
        color: var(--fb-text-primary);
      }
      .checkcell {
        width: 36px;
        text-align: center;
      }
      .empty {
        padding: var(--fb-spacing-lg);
        text-align: center;
        color: var(--fb-text-secondary);
      }
    `,
  ];

  @property({ type: Array }) columns: Column[] = [];
  @property({ type: Array }) rows: Row[] = [];
  /** Field used as the stable row key for selection + events (defaults to `id`). */
  @property({ type: String, attribute: 'row-key' }) rowKey = 'id';
  @property({ type: String, attribute: 'sort-key' }) sortKey?: string;
  @property({ type: String, attribute: 'sort-dir' }) sortDir: SortDir = 'asc';
  @property({ type: Boolean }) selectable = false;
  /** True row count across all pages/virtualized rows; defaults to the rendered count. */
  @property({ type: Number, attribute: 'row-count' }) rowCount?: number;
  @property({ type: String }) caption?: string;

  @state() private selected = new Set<string>();

  private keyOf(row: Row, index: number): string {
    const v = row[this.rowKey];
    return v === undefined || v === null ? String(index) : String(v);
  }

  private isRight(col: Column): boolean {
    if (col.align) return col.align === 'right';
    return col.type === 'number' || col.type === 'money' || col.type === 'date';
  }

  private onSort(col: Column): void {
    if (!col.sortable) return;
    const dir: SortDir = this.sortKey === col.key && this.sortDir === 'asc' ? 'desc' : 'asc';
    this.emit('sort', { key: col.key, dir });
  }

  private ariaSortFor(col: Column): 'ascending' | 'descending' | 'none' {
    if (this.sortKey !== col.key) return 'none';
    return this.sortDir === 'asc' ? 'ascending' : 'descending';
  }

  private toggleRow(key: string): void {
    const next = new Set(this.selected);
    if (next.has(key)) next.delete(key);
    else next.add(key);
    this.selected = next;
    this.emit('select', { keys: [...next] });
  }

  private toggleAll(checked: boolean): void {
    const next = checked ? new Set(this.rows.map((r, i) => this.keyOf(r, i))) : new Set<string>();
    this.selected = next;
    this.emit('select', { keys: [...next] });
  }

  private renderCell(col: Column, row: Row): TemplateResult | string {
    const raw = row[col.key];
    switch (col.type) {
      case 'money': {
        const cents = raw == null ? '0' : String(raw);
        const code = (row[col.currencyKey ?? 'currency_code'] as CurrencyCode) ?? 'USD';
        return html`<fb-money cents=${cents} currency-code=${code}></fb-money>`;
      }
      case 'status':
        return html`<fb-badge size="sm" status=${(raw as BadgeStatus) ?? 'neutral'}
          >${String(raw ?? '')}</fb-badge
        >`;
      case 'role':
        return html`<fb-role-badge .role=${(raw as Role) ?? 'field_worker'}></fb-role-badge>`;
      default:
        return raw == null ? '' : String(raw);
    }
  }

  override render(): TemplateResult {
    const total = this.rowCount ?? this.rows.length;
    const allSelected = this.rows.length > 0 && this.selected.size >= this.rows.length;
    const colCount = this.columns.length + (this.selectable ? 1 : 0);

    return html`
      <div class="wrap">
        <table aria-rowcount=${total}>
          ${this.caption
            ? html`<caption class="visually-hidden">
                ${this.caption}
              </caption>`
            : nothing}
          <thead>
            <tr>
              ${this.selectable
                ? html`<th class="checkcell" scope="col">
                    <input
                      type="checkbox"
                      aria-label="Select all rows"
                      .checked=${allSelected}
                      @change=${(e: Event) =>
                        this.toggleAll((e.target as HTMLInputElement).checked)}
                    />
                  </th>`
                : nothing}
              ${this.columns.map(
                (col) =>
                  html`<th
                    scope="col"
                    class=${this.isRight(col) ? 'right' : ''}
                    aria-sort=${col.sortable ? this.ariaSortFor(col) : nothing}
                  >
                    ${col.sortable
                      ? html`<button
                          class="sort-btn"
                          type="button"
                          @click=${() => this.onSort(col)}
                        >
                          ${col.label}
                          ${this.sortKey === col.key
                            ? html`<fb-icon
                                name=${this.sortDir === 'asc' ? 'arrow-up' : 'arrow-down'}
                                size="14"
                              ></fb-icon>`
                            : nothing}
                        </button>`
                      : col.label}
                  </th>`,
              )}
            </tr>
          </thead>
          <tbody>
            ${this.rows.length === 0
              ? html`<tr>
                  <td class="empty" colspan=${colCount}>No data.</td>
                </tr>`
              : this.rows.map((row, i) => {
                  const key = this.keyOf(row, i);
                  const isSel = this.selected.has(key);
                  return html`<tr aria-rowindex=${i + 2} aria-selected=${isSel ? 'true' : nothing}>
                    ${this.selectable
                      ? html`<td class="checkcell">
                          <input
                            type="checkbox"
                            aria-label="Select row"
                            .checked=${isSel}
                            @change=${() => this.toggleRow(key)}
                          />
                        </td>`
                      : nothing}
                    ${this.columns.map(
                      (col) =>
                        html`<td class=${this.isRight(col) ? 'right' : ''}>
                          ${this.renderCell(col, row)}
                        </td>`,
                    )}
                  </tr>`;
                })}
          </tbody>
        </table>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-data-table': FbDataTable;
  }
}
