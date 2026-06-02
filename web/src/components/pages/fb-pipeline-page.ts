import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-button.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-money.js';
import '../atoms/fb-input.js';
import '../molecules/fb-stat-card.js';
import '../organisms/fb-modal.js';
import '../organisms/fb-state.js';
import {
  listProspects,
  getProspect,
  getPipelineAnalytics,
  advanceProspect,
  loseProspect,
} from '../../api/endpoints/pipeline.js';
import type {
  Prospect,
  ProspectWithDetails,
  PipelineAnalyticsRow,
  PipelineStage,
} from '../../types/models.js';
import { ApiError, userMessageForCode } from '../../api/errors.js';
import { authClaims, hasMinRole } from '../../state/authStore.js';

/** Forward-only stage order (terminal: PERMIT_ISSUED, LOST). */
const BOARD_STAGES: PipelineStage[] = [
  'LEAD',
  'QUALIFIED',
  'ESTIMATE_SENT',
  'VERBAL_COMMITMENT',
  'PERMIT_APPLIED',
  'PERMIT_ISSUED',
];

const STAGE_LABEL: Record<PipelineStage, string> = {
  LEAD: 'Lead',
  QUALIFIED: 'Qualified',
  ESTIMATE_SENT: 'Estimate sent',
  VERBAL_COMMITMENT: 'Verbal commit',
  PERMIT_APPLIED: 'Permit applied',
  PERMIT_ISSUED: 'Permit issued',
  LOST: 'Lost',
};

/** The single legal forward transition from each stage (null = terminal). */
function nextStage(stage: PipelineStage): PipelineStage | null {
  const i = BOARD_STAGES.indexOf(stage);
  if (i < 0 || i >= BOARD_STAGES.length - 1) return null;
  return BOARD_STAGES[i + 1] ?? null;
}

function todayISO(): string {
  return new Date().toISOString().slice(0, 10);
}

/**
 * `fb-pipeline-page` — the CRM Kanban (UX_CORE_SCREENS §11). Superintendent+
 * read; owner/admin advance/lose. Stage transitions are forward-only and
 * enforced server-side (INVALID_TRANSITION 409) — the UI only ever offers the
 * single legal next stage. Analytics are weighted-revenue per currency with no
 * cross-currency total. A prospect that reaches PERMIT_ISSUED deep-links to its
 * created project.
 */
@customElement('fb-pipeline-page')
export class FbPipelinePage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .board {
        display: flex;
        gap: var(--fb-spacing-md);
        overflow-x: auto;
        padding-bottom: var(--fb-spacing-sm);
      }
      .col {
        flex: 0 0 240px;
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-sm);
      }
      .col-head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        font-size: var(--fb-text-label-lg);
        font-weight: 600;
        color: var(--fb-text-secondary);
        padding-bottom: var(--fb-spacing-xs);
        border-bottom: 2px solid var(--fb-gable-green);
      }
      .count {
        color: var(--fb-text-muted);
        font-variant-numeric: tabular-nums;
      }
      .prospect {
        display: flex;
        flex-direction: column;
        gap: 4px;
        padding: var(--fb-spacing-sm);
        text-align: left;
        width: 100%;
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-sm);
        cursor: pointer;
        color: inherit;
        font: inherit;
      }
      .prospect:hover {
        border-color: var(--fb-gable-green);
      }
      .prospect:focus-visible {
        outline: 2px solid var(--fb-gable-green);
        outline-offset: 2px;
      }
      .p-name {
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .p-client {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .p-prob {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
        font-variant-numeric: tabular-nums;
      }
      .detail-section {
        margin-bottom: var(--fb-spacing-md);
      }
      .detail-section h3 {
        margin: 0 0 var(--fb-spacing-xs);
        font-size: var(--fb-text-label-lg);
        color: var(--fb-text-secondary);
      }
      .detail-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
        padding: 4px 0;
        font-size: var(--fb-text-body-sm);
      }
      .dialog-error {
        margin-bottom: var(--fb-spacing-sm);
        color: var(--fb-safety-red);
        font-size: var(--fb-text-body-sm);
      }
      .lose-box {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
        margin-top: var(--fb-spacing-sm);
      }
    `,
  ];

  @state() private prospects: Prospect[] = [];
  @state() private analytics: PipelineAnalyticsRow[] = [];
  @state() private loading = true;
  @state() private errorCode: string | null = null;

  // Detail dialog.
  @state() private detail: ProspectWithDetails | null = null;
  @state() private detailLoading = false;
  @state() private detailError: string | null = null;
  @state() private actionBusy = false;
  @state() private losing = false;
  @state() private loseReason = '';

  private get orgId(): string {
    return authClaims.get()?.orgId ?? '';
  }

  private get canManage(): boolean {
    return hasMinRole('admin');
  }

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  private async load(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      const [prospects, analytics] = await Promise.all([
        listProspects(this.orgId),
        getPipelineAnalytics(this.orgId),
      ]);
      this.prospects = prospects;
      this.analytics = analytics;
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private byStage(stage: PipelineStage): Prospect[] {
    return this.prospects.filter((p) => p.pipeline_stage === stage);
  }

  private async openDetail(prospectId: string): Promise<void> {
    this.detail = null;
    this.detailError = null;
    this.detailLoading = true;
    this.losing = false;
    this.loseReason = '';
    try {
      this.detail = await getProspect(this.orgId, prospectId);
    } catch (err) {
      this.detailError = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.detailLoading = false;
    }
  }

  private closeDetail(): void {
    this.detail = null;
    this.detailError = null;
    this.detailLoading = false;
    this.losing = false;
  }

  private async advance(): Promise<void> {
    const p = this.detail?.prospect;
    if (!p) return;
    const target = nextStage(p.pipeline_stage);
    if (!target) return;
    this.actionBusy = true;
    this.detailError = null;
    try {
      const input =
        target === 'PERMIT_ISSUED'
          ? { target_stage: target, permit_issued_date: todayISO() }
          : { target_stage: target };
      await advanceProspect(this.orgId, p.id, input);
      this.closeDetail();
      await this.load();
    } catch (err) {
      this.detailError =
        err instanceof ApiError ? userMessageForCode(err.code) : 'Could not advance the prospect.';
    } finally {
      this.actionBusy = false;
    }
  }

  private async confirmLose(): Promise<void> {
    const p = this.detail?.prospect;
    if (!p) return;
    if (!this.loseReason.trim()) {
      this.detailError = 'Add a reason before marking this prospect lost.';
      return;
    }
    this.actionBusy = true;
    this.detailError = null;
    try {
      await loseProspect(this.orgId, p.id, this.loseReason.trim());
      this.closeDetail();
      await this.load();
    } catch (err) {
      this.detailError =
        err instanceof ApiError ? userMessageForCode(err.code) : 'Could not update the prospect.';
    } finally {
      this.actionBusy = false;
    }
  }

  private renderAnalytics(): TemplateResult {
    if (this.analytics.length === 0) return html`${nothing}`;
    return html`<div class="analytics">
      ${this.analytics.map(
        (a) => html`
          <section class="currency-group">
            <span class="currency-label"
              ><fb-icon name="trending-up" size="14"></fb-icon>${a.currency_code}</span
            >
            <div class="stat-row">
              <fb-stat-card heading="Open prospects">${a.prospect_count}</fb-stat-card>
              <fb-stat-card heading="Pipeline value"
                ><fb-money
                  cents=${a.total_estimated_cents}
                  currency-code=${a.currency_code}
                ></fb-money
              ></fb-stat-card>
              <fb-stat-card heading="Weighted revenue"
                ><fb-money
                  cents=${a.weighted_revenue_cents}
                  currency-code=${a.currency_code}
                ></fb-money
              ></fb-stat-card>
            </div>
          </section>
        `,
      )}
    </div>`;
  }

  private renderColumn(stage: PipelineStage): TemplateResult {
    const cards = this.byStage(stage);
    return html`<div class="col">
      <div class="col-head">
        <span>${STAGE_LABEL[stage]}</span><span class="count">${cards.length}</span>
      </div>
      ${cards.map(
        (p) =>
          html`<button class="prospect" type="button" @click=${() => void this.openDetail(p.id)}>
            <span class="p-name">${p.name}</span>
            <span class="p-client">${p.client_name}</span>
            <span class="p-prob">${p.probability_pct}% likely</span>
          </button>`,
      )}
    </div>`;
  }

  private renderDetailBody(): TemplateResult {
    if (this.detailLoading)
      return html`<fb-state mode="loading" skeleton="text" rows="4"></fb-state>`;
    if (this.detailError && !this.detail)
      return html`<fb-state mode="error" error-code=${this.detailError}></fb-state>`;
    const d = this.detail;
    if (!d) return html`${nothing}`;
    const p = d.prospect;
    return html`
      ${this.detailError
        ? html`<p class="dialog-error" role="alert">${this.detailError}</p>`
        : nothing}
      <div class="detail-section">
        <div class="detail-row">
          <span>Stage</span>
          <fb-badge size="sm" status="neutral">${STAGE_LABEL[p.pipeline_stage]}</fb-badge>
        </div>
        <div class="detail-row"><span>Client</span><span>${p.client_name}</span></div>
        ${p.address
          ? html`<div class="detail-row"><span>Address</span><span>${p.address}</span></div>`
          : nothing}
        ${p.project_id
          ? html`<div class="detail-row">
              <span>Project</span>
              <a href="/portfolio/projects/${p.project_id}">View project →</a>
            </div>`
          : nothing}
      </div>

      <div class="detail-section">
        <h3>Estimates</h3>
        ${d.estimates.length === 0
          ? html`<p class="p-client">No estimates yet.</p>`
          : d.estimates.map(
              (est) =>
                html`<div class="detail-row">
                  <span>v${est.version} · ${est.status}</span>
                  <fb-money
                    cents=${est.total_estimated_cents}
                    currency-code=${est.currency_code}
                    show-code
                  ></fb-money>
                </div>`,
            )}
      </div>

      <div class="detail-section">
        <h3>Permits</h3>
        ${d.permits.length === 0
          ? html`<p class="p-client">No permits yet.</p>`
          : d.permits.map(
              (pm) =>
                html`<div class="detail-row">
                  <span>${pm.permit_type} · ${pm.jurisdiction} · ${pm.status}</span>
                  <fb-money cents=${pm.fee_cents} currency-code=${pm.fee_currency_code}></fb-money>
                </div>`,
            )}
      </div>

      ${this.losing
        ? html`<div class="lose-box">
            <fb-input
              label="Reason lost"
              placeholder="Why was this prospect lost?"
              value=${this.loseReason}
              @input=${(e: Event) =>
                (this.loseReason = (e as CustomEvent<{ value: string }>).detail.value)}
            ></fb-input>
          </div>`
        : nothing}
    `;
  }

  private renderDetailFooter(): TemplateResult {
    const p = this.detail?.prospect;
    if (!p || !this.canManage) return html`${nothing}`;
    const target = nextStage(p.pipeline_stage);
    const terminal = p.pipeline_stage === 'PERMIT_ISSUED' || p.pipeline_stage === 'LOST';
    if (terminal) return html`${nothing}`;
    if (this.losing) {
      return html`
        <fb-button slot="footer" variant="ghost" @click=${() => (this.losing = false)}
          >Back</fb-button
        >
        <fb-button
          slot="footer"
          variant="destructive"
          ?loading=${this.actionBusy}
          @click=${() => void this.confirmLose()}
          >Confirm lost</fb-button
        >
      `;
    }
    return html`
      <fb-button slot="footer" variant="destructive" @click=${() => (this.losing = true)}
        >Mark lost</fb-button
      >
      ${target
        ? html`<fb-button
            slot="footer"
            variant="primary"
            icon="arrow-up"
            ?loading=${this.actionBusy}
            @click=${() => void this.advance()}
            >Advance to ${STAGE_LABEL[target]}</fb-button
          >`
        : nothing}
    `;
  }

  private renderDialog(): TemplateResult {
    const open = this.detailLoading || this.detail !== null || this.detailError !== null;
    if (!open) return html`${nothing}`;
    const heading = this.detail?.prospect.name ?? 'Prospect';
    return html`<fb-modal open heading=${heading} @close=${this.closeDetail}>
      ${this.renderDetailBody()}${this.renderDetailFooter()}
    </fb-modal>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Pipeline</h1>
            <p class="page-sub">Prospects from lead to permit — forward-only stages.</p>
          </div>
        </div>

        ${this.loading
          ? html`<fb-state mode="loading" skeleton="card" rows="3"></fb-state>`
          : this.errorCode
            ? html`<fb-state
                mode="error"
                error-code=${this.errorCode}
                retryable
                @retry=${() => void this.load()}
              ></fb-state>`
            : html`<div class="pipeline-body">
                ${this.renderAnalytics()}
                <div class="board">${BOARD_STAGES.map((s) => this.renderColumn(s))}</div>
              </div>`}
        ${this.renderDialog()}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-pipeline-page': FbPipelinePage;
  }
}
