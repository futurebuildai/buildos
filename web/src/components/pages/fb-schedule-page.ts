import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-button.js';
import '../atoms/fb-chip.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-select.js';
import '../atoms/fb-switch.js';
import '../organisms/fb-modal.js';
import '../organisms/fb-state.js';
import '../organisms/fb-gantt-chart.js';
import {
  getGantt,
  recalculateSchedule,
  recommendAdjustments,
} from '../../api/endpoints/schedule.js';
import { listProjects } from '../../api/endpoints/projects.js';
import type { GanttView, ProjectTask, Project, ScheduleAdjustmentSet } from '../../types/models.js';
import { ApiError, ErrorCode, userMessageForCode } from '../../api/errors.js';
import { aiConfigured, markAiUnconfigured } from '../../state/capabilityStore.js';
import { hasMinRole, hasRole } from '../../state/authStore.js';
import { navigate } from '../../router.js';
import type { SelectOption } from '../atoms/fb-select.js';

/** AI drawer state — mirrors the briefing hero gating (§9). */
type AiState = 'idle' | 'loading' | 'ok' | 'gated' | 'transient';

/** A schedule is "never computed" when the critical path is empty (zero-value end). */
function neverComputed(g: GanttView | null): boolean {
  if (!g) return true;
  if (g.critical_path.length > 0) return false;
  return !g.project_end || g.project_end.startsWith('0001-01-01');
}

/**
 * `fb-schedule-page` — the CPM schedule workspace (UX_CORE_SCREENS §2–§3). Any
 * authenticated role may read; recalc and AI adjustments are min-superintendent
 * (router + button gates). The route carries no path param, so the page leads
 * with a project picker. The Gantt (`fb-gantt-chart`) is the centerpiece; a
 * "Critical path only" toggle filters non-critical bars. Recalc runs the physics
 * engine server-side, reports `recalculation_ms`, and diffs early-finish dates to
 * pulse the tasks that slipped (cascade diff). The AI "Suggest adjustments" drawer
 * (§3) is a post-hoc transparency view, gated on native-AI availability.
 */
@customElement('fb-schedule-page')
export class FbSchedulePage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .toolbar {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-md);
        flex-wrap: wrap;
      }
      .picker {
        max-width: 28rem;
        flex: 1 1 16rem;
      }
      .toolbar-spacer {
        flex: 1 1 auto;
      }
      .recalc-meta {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .gantt-wrap {
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
        overflow: hidden;
      }
      .legend {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-md);
        margin-top: var(--fb-spacing-md);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .legend span {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
      }
      .swatch {
        width: 12px;
        height: 12px;
        border-radius: 2px;
        flex: none;
      }
      .swatch.critical {
        background: var(--fb-gable-green, #00ffa3);
      }
      .swatch.normal {
        background: var(--fb-slate-steel, #1e2029);
        border: 1px solid var(--fb-blueprint-blue, #38bdf8);
      }
      .swatch.float {
        background: transparent;
        border-top: 2px dashed var(--fb-amber, #f59e0b);
        border-radius: 0;
        height: 0;
      }
      /* AI adjustments drawer */
      .adj {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
        padding: var(--fb-spacing-sm) 0;
        border-bottom: 1px solid var(--fb-border);
      }
      .adj:last-child {
        border-bottom: none;
      }
      .adj-head {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
      }
      .adj-name {
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .adj-delta {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .adj-rationale {
        margin: 0;
        color: var(--fb-text-secondary);
        font-size: var(--fb-text-body-sm);
        line-height: 1.5;
      }
      .adj-summary {
        display: flex;
        gap: var(--fb-spacing-md);
        margin-bottom: var(--fb-spacing-md);
      }
    `,
  ];

  @state() private projects: Project[] = [];
  @state() private projectId = '';
  @state() private gantt: GanttView | null = null;
  @state() private loading = true;
  @state() private ganttLoading = false;
  @state() private errorCode: string | null = null;
  @state() private ganttError: string | null = null;

  @state() private criticalOnly = false;
  @state() private recalcBusy = false;
  @state() private recalcMs: number | null = null;
  @state() private slippedIds: string[] = [];
  @state() private cascadeNotice: string | null = null;
  @state() private recalcError: string | null = null;

  // AI adjustments drawer (E2).
  @state() private adjOpen = false;
  @state() private aiState: AiState = 'idle';
  @state() private adjustments: ScheduleAdjustmentSet | null = null;
  @state() private adjErrorRequestId: string | null = null;

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
        void this.loadGantt();
      }
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private async loadGantt(): Promise<void> {
    if (!this.projectId) return;
    this.ganttLoading = true;
    this.ganttError = null;
    try {
      this.gantt = await getGantt(this.projectId);
    } catch (err) {
      this.ganttError = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.ganttLoading = false;
    }
  }

  private onProject(e: Event): void {
    this.projectId = (e as CustomEvent<{ value: string }>).detail.value;
    // Reset per-project derived state.
    this.recalcMs = null;
    this.slippedIds = [];
    this.cascadeNotice = null;
    this.recalcError = null;
    void this.loadGantt();
  }

  private projectOptions(): SelectOption[] {
    return this.projects.map((p) => ({ value: p.id, label: p.name }));
  }

  /** Recalc: snapshot early-finish, run physics, reload, then pulse slipped tasks. */
  private async recalc(): Promise<void> {
    if (!this.projectId) return;
    this.recalcBusy = true;
    this.recalcError = null;
    this.cascadeNotice = null;
    const before = new Map<string, number>();
    for (const t of this.gantt?.tasks ?? []) {
      if (t.early_finish) before.set(t.id, new Date(t.early_finish).getTime());
    }
    try {
      const result = await recalculateSchedule(this.projectId);
      this.recalcMs = result.recalculation_ms;
      await this.loadGantt();
      const slipped: string[] = [];
      for (const t of this.gantt?.tasks ?? []) {
        const prev = before.get(t.id);
        const now = t.early_finish ? new Date(t.early_finish).getTime() : null;
        if (prev !== undefined && now !== null && now > prev) slipped.push(t.id);
      }
      this.slippedIds = slipped;
      if (result.cpm_result.critical_path_changed) {
        this.cascadeNotice =
          slipped.length > 0
            ? `Critical path changed — ${slipped.length} ${slipped.length === 1 ? 'task' : 'tasks'} slipped.`
            : 'Critical path changed.';
      } else if (slipped.length > 0) {
        this.cascadeNotice = `${slipped.length} ${slipped.length === 1 ? 'task' : 'tasks'} moved.`;
      }
    } catch (err) {
      this.recalcError =
        err instanceof ApiError ? userMessageForCode(err.code) : 'Could not recalculate.';
    } finally {
      this.recalcBusy = false;
    }
  }

  // --------------------------- AI adjustments (E2) ---------------------------
  private openAdjustments(): void {
    this.adjOpen = true;
    this.adjustments = null;
    this.adjErrorRequestId = null;
    if (!aiConfigured.get()) {
      this.aiState = 'gated';
      return;
    }
    void this.loadAdjustments();
  }

  private closeAdjustments(): void {
    this.adjOpen = false;
  }

  private async loadAdjustments(): Promise<void> {
    if (!this.projectId) return;
    this.aiState = 'loading';
    this.adjErrorRequestId = null;
    try {
      this.adjustments = await recommendAdjustments(this.projectId);
      this.aiState = 'ok';
      // Server applied deltas + re-ran CPM; refresh the board to reflect them.
      if (this.adjustments.applied_deltas > 0) await this.loadGantt();
    } catch (err) {
      if (err instanceof ApiError) {
        this.adjErrorRequestId = err.requestId ?? null;
        if (err.code === ErrorCode.SERVICE_UNAVAILABLE || err.isAiUnconfigured) {
          markAiUnconfigured();
          this.aiState = 'gated';
        } else {
          this.aiState = 'transient';
        }
      } else {
        this.aiState = 'transient';
      }
    }
  }

  // ------------------------------- Render -------------------------------
  private visibleTasks(): ProjectTask[] {
    const tasks = this.gantt?.tasks ?? [];
    return this.criticalOnly ? tasks.filter((t) => t.is_critical) : tasks;
  }

  private renderAdjBody(): TemplateResult {
    if (this.aiState === 'loading')
      return html`<fb-state mode="loading" skeleton="text" rows="4"></fb-state>`;

    if (this.aiState === 'gated')
      return html`<fb-state
        mode="gated"
        heading="AI adjustments are off"
        message="Add an Anthropic API key to enable AI schedule suggestions."
        ?can-configure=${hasRole('owner')}
        @configure=${() => navigate('/settings/integrations')}
      ></fb-state>`;

    if (this.aiState === 'transient')
      return html`<fb-state
        mode="error"
        error-code=${ErrorCode.UPSTREAM_ERROR}
        request-id=${this.adjErrorRequestId ?? nothing}
        retryable
        @retry=${() => void this.loadAdjustments()}
      ></fb-state>`;

    const set = this.adjustments;
    if (!set) return html`${nothing}`;
    if (set.adjustments.length === 0)
      return html`<fb-state
        mode="empty"
        icon="sparkles"
        heading="No adjustments suggested"
        message="The model reviewed the schedule and recommended no duration changes."
      ></fb-state>`;

    return html`<div class="adj-list">
      <div class="adj-summary">
        <fb-chip>${set.applied_deltas} applied</fb-chip>
        <fb-chip>${set.skipped_rationale_only} advisory</fb-chip>
      </div>
      ${set.adjustments.map(
        (a) =>
          html`<div class="adj">
            <div class="adj-head">
              <fb-badge size="sm" status=${a.applied ? 'complete' : 'neutral'}>
                ${a.applied ? 'Applied' : 'Advisory'}
              </fb-badge>
              <span class="adj-name">${a.wbs_code} · ${a.name}</span>
            </div>
            ${a.old_duration_days !== undefined && a.new_duration_days !== undefined
              ? html`<span class="adj-delta"
                  >${a.old_duration_days}d → ${a.new_duration_days}d</span
                >`
              : nothing}
            <p class="adj-rationale">${a.rationale}</p>
          </div>`,
      )}
    </div>`;
  }

  private renderDrawer(): TemplateResult {
    if (!this.adjOpen) return html`${nothing}`;
    return html`<fb-modal open heading="AI schedule adjustments" @close=${this.closeAdjustments}>
      ${this.renderAdjBody()}
      <fb-button slot="footer" variant="ghost" @click=${this.closeAdjustments}>Close</fb-button>
    </fb-modal>`;
  }

  private renderChart(): TemplateResult {
    if (this.ganttLoading)
      return html`<fb-state mode="loading" skeleton="card" rows="6"></fb-state>`;
    if (this.ganttError)
      return html`<fb-state
        mode="error"
        error-code=${this.ganttError}
        retryable
        @retry=${() => void this.loadGantt()}
      ></fb-state>`;
    if (neverComputed(this.gantt))
      return html`<fb-state
        mode="empty"
        icon="calendar"
        heading="Schedule not computed yet"
        message="Recalculate to run the critical-path engine and draw the timeline."
      >
        ${hasMinRole('superintendent')
          ? html`<fb-button
              slot="action"
              variant="primary"
              icon="refresh"
              ?loading=${this.recalcBusy}
              @click=${() => void this.recalc()}
              >Recalculate</fb-button
            >`
          : nothing}
      </fb-state>`;

    return html`<div class="gantt-wrap">
      <fb-gantt-chart
        .tasks=${this.visibleTasks()}
        project-end=${this.gantt?.project_end ?? ''}
        .slippedIds=${this.slippedIds}
      ></fb-gantt-chart>
      <div class="legend">
        <span><span class="swatch critical"></span>Critical path</span>
        <span><span class="swatch normal"></span>Has float</span>
        <span><span class="swatch float"></span>Near-critical float (≤2d)</span>
      </div>
    </div>`;
  }

  private renderWorkspace(): TemplateResult {
    const canRecalc = hasMinRole('superintendent');
    return html`<div class="workspace">
      <div class="toolbar">
        <div class="picker">
          <fb-select
            label="Project"
            .options=${this.projectOptions()}
            value=${this.projectId}
            @change=${this.onProject}
          ></fb-select>
        </div>
        <fb-switch
          label="Critical path only"
          .checked=${this.criticalOnly}
          @change=${(e: Event) =>
            (this.criticalOnly = (e as CustomEvent<{ checked: boolean }>).detail.checked)}
          >Critical path only</fb-switch
        >
        <div class="toolbar-spacer"></div>
        ${this.recalcMs !== null
          ? html`<span class="recalc-meta"
              ><fb-icon name="clock" size="14"></fb-icon>recomputed in ${this.recalcMs}ms</span
            >`
          : nothing}
        ${canRecalc
          ? html`<fb-button
                variant="secondary"
                icon="sparkles"
                @click=${() => this.openAdjustments()}
                >Suggest adjustments</fb-button
              >
              <fb-button
                variant="primary"
                icon="refresh"
                ?loading=${this.recalcBusy}
                @click=${() => void this.recalc()}
                >Recalculate</fb-button
              >`
          : nothing}
      </div>

      ${this.recalcError
        ? html`<p class="toast err" role="alert">
            <fb-icon name="alert-circle" size="16"></fb-icon>${this.recalcError}
          </p>`
        : nothing}
      ${this.cascadeNotice
        ? html`<p class="banner warn" role="status">
            <fb-icon name="alert-triangle" size="16"></fb-icon>${this.cascadeNotice}
          </p>`
        : nothing}
      ${this.renderChart()}
    </div>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Schedule</h1>
            <p class="page-sub">The critical-path timeline — recompute to cascade any slips.</p>
          </div>
        </div>

        ${this.loading
          ? html`<fb-state mode="loading" skeleton="card" rows="6"></fb-state>`
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
                  message="Schedules are computed per project."
                ></fb-state>`
              : this.renderWorkspace()}
        ${this.renderDrawer()}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-schedule-page': FbSchedulePage;
  }
}
