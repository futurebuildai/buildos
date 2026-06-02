import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-button.js';
import '../atoms/fb-chip.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-input.js';
import '../atoms/fb-select.js';
import '../atoms/fb-money.js';
import '../organisms/fb-modal.js';
import '../organisms/fb-state.js';
import {
  listProcurement,
  updateProcurementItem,
  requestVendorReview,
} from '../../api/endpoints/procurement.js';
import { listProjects } from '../../api/endpoints/projects.js';
import type {
  ProcurementItem,
  ProcurementStatus,
  Project,
  CurrencyCode,
} from '../../types/models.js';
import { ApiError, ErrorCode, userMessageForCode } from '../../api/errors.js';
import { hasMinRole, hasRole } from '../../state/authStore.js';
import { sumByCurrency } from '../../money/money.js';
import type { BadgeStatus } from '../atoms/fb-badge.js';
import type { SelectOption } from '../atoms/fb-select.js';

/** Triage urgency order (CRITICAL first); ORDERED sinks to the bottom. */
const STATUS_ORDER: ProcurementStatus[] = ['CRITICAL', 'WARNING', 'OK', 'ORDERED'];

function statusBadge(status: ProcurementStatus): { status: BadgeStatus; label: string } {
  switch (status) {
    case 'CRITICAL':
      return { status: 'critical', label: 'Critical' };
    case 'WARNING':
      return { status: 'warning', label: 'Warning' };
    case 'ORDERED':
      return { status: 'pending', label: 'Ordered' };
    case 'OK':
    default:
      return { status: 'active', label: 'On track' };
  }
}

/** Whole days from today until `date` (YYYY-MM-DD/RFC3339); negative when past. */
function daysUntil(date: string): number | null {
  const then = new Date(date).getTime();
  if (Number.isNaN(then)) return null;
  return Math.ceil((then - Date.now()) / 86_400_000);
}

/**
 * `fb-procurement-page` — the procurement triage board (UX_CORE_SCREENS §5).
 * Read is any-authenticated; Mark Ordered is owner/admin; Request Review is
 * superintendent+. The route is project-scoped but carries no path param, so the
 * page leads with a project picker (the portfolio roll-up entry). Items group by
 * status descending urgency with a per-`currency_code` subtotal — never a
 * cross-currency total. Mark Ordered updates optimistically.
 */
@customElement('fb-procurement-page')
export class FbProcurementPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .picker {
        max-width: 28rem;
        margin-bottom: var(--fb-spacing-lg);
      }
      .group {
        margin-bottom: var(--fb-spacing-lg);
      }
      .group-head {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        margin-bottom: var(--fb-spacing-sm);
      }
      .item-card {
        display: flex;
        align-items: flex-start;
        justify-content: space-between;
        gap: var(--fb-spacing-md);
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
        margin-bottom: var(--fb-spacing-sm);
      }
      .item-main {
        min-width: 0;
      }
      .item-name {
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .item-meta {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
        align-items: center;
        margin-top: var(--fb-spacing-xs);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .countdown {
        font-family: var(--fb-font-mono);
      }
      .countdown.overdue {
        color: var(--fb-safety-red);
        font-weight: 600;
      }
      .item-side {
        display: flex;
        flex-direction: column;
        align-items: flex-end;
        gap: var(--fb-spacing-sm);
        flex-shrink: 0;
      }
      .item-actions {
        display: flex;
        gap: var(--fb-spacing-sm);
      }
      .subtotals {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-lg);
        padding-top: var(--fb-spacing-md);
        border-top: 1px solid var(--fb-border);
        margin-top: var(--fb-spacing-md);
      }
      .subtotal {
        display: flex;
        flex-direction: column;
        gap: 2px;
      }
      .subtotal .label {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .field {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
        margin-bottom: var(--fb-spacing-md);
      }
      .field label {
        font-size: var(--fb-text-label-lg);
        color: var(--fb-text-secondary);
      }
      .dialog-error {
        margin-bottom: var(--fb-spacing-md);
        color: var(--fb-safety-red);
        font-size: var(--fb-text-body-sm);
      }
    `,
  ];

  @state() private projects: Project[] = [];
  @state() private projectId = '';
  @state() private items: ProcurementItem[] = [];
  @state() private loading = true;
  @state() private itemsLoading = false;
  @state() private errorCode: string | null = null;
  @state() private itemsError: string | null = null;
  @state() private notice: { kind: 'ok' | 'err'; text: string } | null = null;

  // Mark-Ordered dialog.
  @state() private ordering: ProcurementItem | null = null;
  @state() private poNumber = '';
  @state() private orderedAt = '';
  @state() private orderError: string | null = null;
  @state() private orderBusy = false;

  // Request-review dialog.
  @state() private reviewing: ProcurementItem | null = null;
  @state() private reviewVendor = '';
  @state() private reviewReasoning = '';
  @state() private reviewError: string | null = null;
  @state() private reviewBusy = false;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.loadProjects();
  }

  private async loadProjects(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      this.projects = await listProjects();
      const first = this.projects[0];
      if (first) {
        this.projectId = first.id;
        void this.loadItems();
      }
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private async loadItems(): Promise<void> {
    if (!this.projectId) return;
    this.itemsLoading = true;
    this.itemsError = null;
    try {
      this.items = await listProcurement(this.projectId);
    } catch (err) {
      this.itemsError = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.itemsLoading = false;
    }
  }

  private onProject(e: Event): void {
    this.projectId = (e as CustomEvent<{ value: string }>).detail.value;
    void this.loadItems();
  }

  private projectOptions(): SelectOption[] {
    return this.projects.map((p) => ({ value: p.id, label: p.name }));
  }

  // --------------------------- Mark Ordered ---------------------------
  private openOrder(item: ProcurementItem): void {
    this.ordering = item;
    this.poNumber = '';
    this.orderedAt = new Date().toISOString().slice(0, 10);
    this.orderError = null;
  }
  private closeOrder(): void {
    this.ordering = null;
  }
  private async submitOrder(): Promise<void> {
    if (!this.ordering) return;
    if (!this.poNumber || !this.orderedAt) {
      this.orderError = 'Enter a PO number and order date.';
      return;
    }
    this.orderBusy = true;
    this.orderError = null;
    const target = this.ordering;
    try {
      const updated = await updateProcurementItem(this.projectId, target.id, {
        status: 'ORDERED',
        po_number: this.poNumber,
        ordered_at: this.orderedAt,
      });
      this.items = this.items.map((i) => (i.id === updated.id ? updated : i));
      this.notice = { kind: 'ok', text: `${target.name} marked ordered.` };
      this.closeOrder();
    } catch (err) {
      this.orderError =
        err instanceof ApiError ? userMessageForCode(err.code) : 'Could not update the item.';
    } finally {
      this.orderBusy = false;
    }
  }

  // ------------------------- Request review --------------------------
  private openReview(item: ProcurementItem): void {
    this.reviewing = item;
    this.reviewVendor = '';
    this.reviewReasoning = '';
    this.reviewError = null;
  }
  private closeReview(): void {
    this.reviewing = null;
  }
  private async submitReview(): Promise<void> {
    if (!this.reviewing) return;
    if (!this.reviewVendor.trim()) {
      this.reviewError = 'Enter a vendor name.';
      return;
    }
    this.reviewBusy = true;
    this.reviewError = null;
    const target = this.reviewing;
    try {
      await requestVendorReview(this.projectId, target.id, {
        vendor: this.reviewVendor.trim(),
        total_cents: target.estimated_cost_cents,
        currency_code: target.estimated_cost_currency_code,
        ...(this.reviewReasoning.trim() ? { reasoning: this.reviewReasoning.trim() } : {}),
      });
      this.notice = { kind: 'ok', text: 'Review requested — sent to the review feed.' };
      this.closeReview();
    } catch (err) {
      if (err instanceof ApiError && err.code === ErrorCode.SERVICE_UNAVAILABLE) {
        this.reviewError = "This action isn't available right now.";
      } else {
        this.reviewError =
          err instanceof ApiError ? userMessageForCode(err.code) : 'Could not request review.';
      }
    } finally {
      this.reviewBusy = false;
    }
  }

  // ------------------------------ Render -----------------------------
  private renderCountdown(item: ProcurementItem): TemplateResult {
    if (item.status === 'ORDERED') {
      return html`<span class="countdown"
        >${item.po_number ? `PO ${item.po_number}` : 'Ordered'}</span
      >`;
    }
    if (!item.must_order_date) return html`${nothing}`;
    const d = daysUntil(item.must_order_date);
    if (d === null) return html`${nothing}`;
    if (d < 0) return html`<span class="countdown overdue">OVERDUE ${-d}d</span>`;
    return html`<span class="countdown">order in ${d}d</span>`;
  }

  private renderItem(item: ProcurementItem): TemplateResult {
    const badge = statusBadge(item.status);
    const canOrder = hasRole('owner', 'admin') && item.status !== 'ORDERED';
    const canReview = hasMinRole('superintendent') && item.status !== 'ORDERED';
    return html`<div class="item-card">
      <div class="item-main">
        <div class="item-name">${item.name}</div>
        <div class="item-meta">
          <span>${item.wbs_code}</span>
          ${this.renderCountdown(item)}
          ${item.weather_buffer_days > 0
            ? html`<fb-chip>+${item.weather_buffer_days}d weather buffer</fb-chip>`
            : nothing}
          <span>${item.lead_time_days}d lead</span>
        </div>
      </div>
      <div class="item-side">
        <fb-badge size="sm" status=${badge.status}>${badge.label}</fb-badge>
        <fb-money
          cents=${item.estimated_cost_cents}
          currency-code=${item.estimated_cost_currency_code}
          show-code
        ></fb-money>
        ${canOrder || canReview
          ? html`<div class="item-actions">
              ${canReview
                ? html`<fb-button size="sm" variant="ghost" @click=${() => this.openReview(item)}
                    >Request review</fb-button
                  >`
                : nothing}
              ${canOrder
                ? html`<fb-button
                    size="sm"
                    variant="secondary"
                    icon="check"
                    @click=${() => this.openOrder(item)}
                    >Mark ordered</fb-button
                  >`
                : nothing}
            </div>`
          : nothing}
      </div>
    </div>`;
  }

  private renderBoard(): TemplateResult {
    if (this.itemsLoading)
      return html`<fb-state mode="loading" skeleton="card" rows="5"></fb-state>`;
    if (this.itemsError)
      return html`<fb-state
        mode="error"
        error-code=${this.itemsError}
        retryable
        @retry=${() => void this.loadItems()}
      ></fb-state>`;
    if (this.items.length === 0)
      return html`<fb-state
        mode="empty"
        icon="package"
        heading="No procurement items"
        message="Material and equipment orders for this project will appear here."
      ></fb-state>`;

    const subtotals = sumByCurrency(
      this.items.map((i) => ({
        cents: i.estimated_cost_cents,
        currencyCode: i.estimated_cost_currency_code,
      })),
    );
    return html`<div class="board">
      <div class="groups">
        ${STATUS_ORDER.map((status) => {
          const group = this.items.filter((i) => i.status === status);
          if (group.length === 0) return nothing;
          const badge = statusBadge(status);
          return html`<section class="group">
            <div class="group-head">
              <fb-badge size="sm" status=${badge.status}>${badge.label}</fb-badge>
              <span class="item-meta">${group.length}</span>
            </div>
            ${group.map((i) => this.renderItem(i))}
          </section>`;
        })}
      </div>
      <div class="subtotals">
        ${subtotals.map(
          (s) =>
            html`<div class="subtotal">
              <span class="label">Estimated (${s.currencyCode})</span>
              <fb-money
                cents=${s.cents}
                currency-code=${s.currencyCode as CurrencyCode}
                show-code
              ></fb-money>
            </div>`,
        )}
      </div>
    </div>`;
  }

  private renderOrderDialog(): TemplateResult {
    const item = this.ordering;
    if (!item) return html`${nothing}`;
    return html`<fb-modal open heading="Mark ${item.name} ordered" @close=${this.closeOrder}>
      ${this.orderError
        ? html`<p class="dialog-error" role="alert">${this.orderError}</p>`
        : nothing}
      <div class="field">
        <label for="po">PO number</label>
        <fb-input
          id="po"
          value=${this.poNumber}
          @change=${(e: Event) =>
            (this.poNumber = (e as CustomEvent<{ value: string }>).detail.value)}
        ></fb-input>
      </div>
      <div class="field">
        <label for="ordered-at">Order date</label>
        <fb-input
          id="ordered-at"
          type="date"
          value=${this.orderedAt}
          @change=${(e: Event) =>
            (this.orderedAt = (e as CustomEvent<{ value: string }>).detail.value)}
        ></fb-input>
      </div>
      <fb-button slot="footer" variant="ghost" @click=${this.closeOrder}>Cancel</fb-button>
      <fb-button
        slot="footer"
        variant="primary"
        ?loading=${this.orderBusy}
        @click=${() => void this.submitOrder()}
        >Mark ordered</fb-button
      >
    </fb-modal>`;
  }

  private renderReviewDialog(): TemplateResult {
    const item = this.reviewing;
    if (!item) return html`${nothing}`;
    return html`<fb-modal open heading="Request vendor review" @close=${this.closeReview}>
      ${this.reviewError
        ? html`<p class="dialog-error" role="alert">${this.reviewError}</p>`
        : nothing}
      <div class="field">
        <label for="vendor">Vendor</label>
        <fb-input
          id="vendor"
          value=${this.reviewVendor}
          @change=${(e: Event) =>
            (this.reviewVendor = (e as CustomEvent<{ value: string }>).detail.value)}
        ></fb-input>
      </div>
      <div class="field">
        <label>Quoted total</label>
        <fb-money
          cents=${item.estimated_cost_cents}
          currency-code=${item.estimated_cost_currency_code}
          show-code
        ></fb-money>
      </div>
      <div class="field">
        <label for="reasoning">Reasoning (optional)</label>
        <fb-input
          id="reasoning"
          value=${this.reviewReasoning}
          @change=${(e: Event) =>
            (this.reviewReasoning = (e as CustomEvent<{ value: string }>).detail.value)}
        ></fb-input>
      </div>
      <fb-button slot="footer" variant="ghost" @click=${this.closeReview}>Cancel</fb-button>
      <fb-button
        slot="footer"
        variant="primary"
        ?loading=${this.reviewBusy}
        @click=${() => void this.submitReview()}
        >Request review</fb-button
      >
    </fb-modal>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Procurement</h1>
            <p class="page-sub">Material lead-time triage — order before the schedule slips.</p>
          </div>
        </div>

        ${this.notice
          ? html`<p class="toast ${this.notice.kind}" role="status">
              <fb-icon
                name=${this.notice.kind === 'ok' ? 'check-circle' : 'alert-circle'}
                size="16"
              ></fb-icon>
              ${this.notice.text}
            </p>`
          : nothing}
        ${this.loading
          ? html`<fb-state mode="loading" skeleton="card" rows="5"></fb-state>`
          : this.errorCode
            ? html`<fb-state
                mode="error"
                error-code=${this.errorCode}
                retryable
                @retry=${() => void this.loadProjects()}
              ></fb-state>`
            : this.projects.length === 0
              ? html`<fb-state
                  mode="empty"
                  icon="folder"
                  heading="No projects yet"
                  message="Procurement items are tracked per project."
                ></fb-state>`
              : html`<div class="board-area">
                  <div class="picker">
                    <fb-select
                      label="Project"
                      .options=${this.projectOptions()}
                      value=${this.projectId}
                      @change=${this.onProject}
                    ></fb-select>
                  </div>
                  <div class="board-host">${this.renderBoard()}</div>
                </div>`}
        ${this.renderOrderDialog()} ${this.renderReviewDialog()}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-procurement-page': FbProcurementPage;
  }
}
