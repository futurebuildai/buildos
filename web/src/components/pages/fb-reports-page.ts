import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-button.js';
import '../atoms/fb-chip.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-markdown.js';
import '../atoms/fb-select.js';
import '../organisms/fb-data-table.js';
import '../organisms/fb-state.js';
import { listProjects } from '../../api/endpoints/projects.js';
import {
  listDailyReports,
  getDailyReport,
  generateDigest,
  draftClientUpdate,
} from '../../api/endpoints/reports.js';
import type {
  Project,
  DailyReport,
  DailyReportSummary,
  ClientUpdateDraft,
} from '../../types/models.js';
import type { Column } from '../organisms/fb-data-table.js';
import type { SelectOption } from '../atoms/fb-select.js';
import { ApiError, ErrorCode } from '../../api/errors.js';
import { aiConfigured, markAiUnconfigured } from '../../state/capabilityStore.js';
import { hasRole, hasMinRole } from '../../state/authStore.js';
import { navigate } from '../../router.js';

/** AI action state for the digest/draft heroes (mirrors fb-briefing-page). */
type AiState = 'idle' | 'busy' | 'ok' | 'gated' | 'error';

const TASK_COLUMNS: Column[] = [
  { key: 'wbs_code', label: 'WBS' },
  { key: 'name', label: 'Task' },
  { key: 'percent_complete', label: '% Complete', type: 'number' },
];

/**
 * `fb-reports-page` — the operator Daily Reports surface (Chunk C of
 * DAILY_REPORTS_CLIENT_UPDATES). A project + date selector renders the DERIVED
 * daily report (field notes via fb-markdown, crew check-in count, per-task
 * progress, and photo thumbnails when object storage is configured). Two AI
 * actions: "Generate office digest" (superintendent+, internal) and "Draft
 * client update" (owner/admin) — the latter shows a READ-ONLY preview of the
 * client-safe homeowner draft (the editable composer + send is Chunk D). AI is
 * key-gated: a 503 (or assume-off capability) shows a gated panel; the report
 * itself never depends on AI and always renders. Photos are additive — text
 * works with zero photos.
 */
@customElement('fb-reports-page')
export class FbReportsPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .pickers {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-md);
        max-width: 48rem;
        margin-bottom: var(--fb-spacing-lg);
      }
      .picker {
        flex: 1 1 16rem;
        min-width: 14rem;
      }
      .report {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-lg);
      }
      .card {
        padding: var(--fb-spacing-lg);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
      }
      .meta-chips {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
        margin-bottom: var(--fb-spacing-md);
      }
      .mono {
        font-family: var(--fb-font-mono);
      }
      .section-label {
        font-size: var(--fb-text-title-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin: 0 0 var(--fb-spacing-sm);
      }
      .safety {
        border-left: 3px solid var(--fb-safety-red);
        padding-left: var(--fb-spacing-md);
        color: var(--fb-text-primary);
      }
      .photo-strip {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
      }
      .photo-strip img {
        width: 8rem;
        height: 8rem;
        object-fit: cover;
        border-radius: var(--fb-radius-md);
        border: 1px solid var(--md-sys-color-outline);
        background: var(--fb-surface-2);
      }
      .actions {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
      }
      .ai-output {
        white-space: normal;
      }
      .draft-subject {
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-spacing-sm);
      }
      .draft-note {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
        margin-top: var(--fb-spacing-sm);
      }
    `,
  ];

  @state() private projects: Project[] = [];
  @state() private projectId = '';
  @state() private dates: DailyReportSummary[] = [];
  @state() private selectedDate = '';
  @state() private report: DailyReport | null = null;

  @state() private loading = true;
  @state() private errorCode: string | null = null;
  @state() private datesLoading = false;
  @state() private reportLoading = false;
  @state() private reportError: string | null = null;

  // Office digest (superintendent+).
  @state() private digest: string | null = null;
  @state() private digestState: AiState = 'idle';

  // Client-update draft (owner/admin).
  @state() private draft: ClientUpdateDraft | null = null;
  @state() private draftState: AiState = 'idle';

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
        await this.loadDates();
      }
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private async loadDates(): Promise<void> {
    if (!this.projectId) return;
    this.datesLoading = true;
    this.reportError = null;
    this.resetAi();
    try {
      this.dates = await listDailyReports(this.projectId);
      const first = this.dates[0];
      if (first) {
        this.selectedDate = first.log_date.slice(0, 10);
        await this.loadReport();
      } else {
        this.selectedDate = '';
        this.report = null;
      }
    } catch (err) {
      this.reportError = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.datesLoading = false;
    }
  }

  private async loadReport(): Promise<void> {
    if (!this.projectId || !this.selectedDate) return;
    this.reportLoading = true;
    this.reportError = null;
    this.resetAi();
    try {
      this.report = await getDailyReport(this.projectId, this.selectedDate);
    } catch (err) {
      this.reportError = err instanceof ApiError ? err.code : 'UNKNOWN';
      this.report = null;
    } finally {
      this.reportLoading = false;
    }
  }

  private resetAi(): void {
    this.digest = null;
    this.digestState = 'idle';
    this.draft = null;
    this.draftState = 'idle';
  }

  private onProject(e: Event): void {
    this.projectId = (e as CustomEvent<{ value: string }>).detail.value;
    void this.loadDates();
  }

  private onDate(e: Event): void {
    this.selectedDate = (e as CustomEvent<{ value: string }>).detail.value;
    void this.loadReport();
  }

  private projectOptions(): SelectOption[] {
    return this.projects.map((p) => ({ value: p.id, label: p.name }));
  }

  private dateOptions(): SelectOption[] {
    return this.dates.map((d) => {
      const date = d.log_date.slice(0, 10);
      const flags = [
        d.has_safety_incident ? '⚠ incident' : '',
        d.photo_count > 0 ? `${d.photo_count} photo${d.photo_count === 1 ? '' : 's'}` : '',
      ].filter(Boolean);
      return { value: date, label: flags.length ? `${date} · ${flags.join(' · ')}` : date };
    });
  }

  // ------------------------------ AI actions -----------------------------
  private async onGenerateDigest(): Promise<void> {
    if (!this.report) return;
    if (!aiConfigured.get()) {
      this.digestState = 'gated';
      return;
    }
    this.digestState = 'busy';
    try {
      this.digest = await generateDigest(this.projectId, this.selectedDate);
      this.digestState = 'ok';
    } catch (err) {
      this.digestState = this.classifyAi(err);
    }
  }

  private async onDraftClientUpdate(): Promise<void> {
    if (!this.report) return;
    if (!aiConfigured.get()) {
      this.draftState = 'gated';
      return;
    }
    this.draftState = 'busy';
    try {
      this.draft = await draftClientUpdate(this.projectId, this.selectedDate);
      this.draftState = 'ok';
    } catch (err) {
      this.draftState = this.classifyAi(err);
    }
  }

  /** Map an AI call failure to a hero state (503 → gated, else transient error). */
  private classifyAi(err: unknown): AiState {
    if (
      err instanceof ApiError &&
      (err.code === ErrorCode.SERVICE_UNAVAILABLE || err.isAiUnconfigured)
    ) {
      markAiUnconfigured();
      return 'gated';
    }
    return 'error';
  }

  // ------------------------------- Render --------------------------------
  private renderPhotos(): TemplateResult {
    const photos = this.report?.photos ?? [];
    if (photos.length === 0) return html`${nothing}`;
    return html`<section class="card">
      <p class="section-label">Photos (${photos.length})</p>
      <div class="photo-strip">
        ${photos.map(
          (p) =>
            html`<img src=${p.thumb_url} alt="Jobsite photo" loading="lazy" decoding="async" />`,
        )}
      </div>
    </section>`;
  }

  private renderDigestAction(): TemplateResult {
    if (!hasMinRole('superintendent')) return html`${nothing}`;
    return html`<section class="card">
      <div class="actions">
        <fb-button
          variant="secondary"
          icon="sparkles"
          ?loading=${this.digestState === 'busy'}
          @click=${() => void this.onGenerateDigest()}
          >Generate office digest</fb-button
        >
      </div>
      ${this.digestState === 'gated'
        ? html`<fb-state
            mode="gated"
            heading="AI digest is off"
            message="Add an Anthropic API key to generate the office digest."
            ?can-configure=${hasRole('owner')}
            @configure=${() => navigate('/settings/integrations')}
          ></fb-state>`
        : nothing}
      ${this.digestState === 'error'
        ? html`<fb-state
            mode="error"
            error-code=${ErrorCode.UPSTREAM_ERROR}
            retryable
            @retry=${() => void this.onGenerateDigest()}
          ></fb-state>`
        : nothing}
      ${this.digestState === 'ok' && this.digest
        ? html`<div class="ai-output">
            <p class="section-label">Office digest</p>
            <fb-markdown .source=${this.digest}></fb-markdown>
          </div>`
        : nothing}
    </section>`;
  }

  private renderDraftAction(): TemplateResult {
    if (!hasRole('owner', 'admin')) return html`${nothing}`;
    return html`<section class="card">
      <div class="actions">
        <fb-button
          variant="ghost"
          icon="message-circle"
          ?loading=${this.draftState === 'busy'}
          @click=${() => void this.onDraftClientUpdate()}
          >Draft client update</fb-button
        >
      </div>
      ${this.draftState === 'gated'
        ? html`<fb-state
            mode="gated"
            heading="AI drafting is off"
            message="Add an Anthropic API key to draft a client update."
            ?can-configure=${hasRole('owner')}
            @configure=${() => navigate('/settings/integrations')}
          ></fb-state>`
        : nothing}
      ${this.draftState === 'error'
        ? html`<fb-state
            mode="error"
            error-code=${ErrorCode.UPSTREAM_ERROR}
            retryable
            @retry=${() => void this.onDraftClientUpdate()}
          ></fb-state>`
        : nothing}
      ${this.draftState === 'ok' && this.draft
        ? html`<div class="ai-output">
            <p class="section-label">Client update draft (preview)</p>
            <p class="draft-subject" data-test="draft-subject">${this.draft.subject}</p>
            <fb-markdown .source=${this.draft.body}></fb-markdown>
            <p class="draft-note">
              Read-only preview. Editing and sending arrive with the client-update composer.
            </p>
          </div>`
        : nothing}
    </section>`;
  }

  private renderReport(): TemplateResult {
    if (this.reportLoading)
      return html`<fb-state mode="loading" skeleton="card" rows="4"></fb-state>`;
    if (this.reportError)
      return html`<fb-state
        mode="error"
        error-code=${this.reportError}
        retryable
        @retry=${() => void this.loadReport()}
      ></fb-state>`;
    if (this.dates.length === 0)
      return html`<fb-state
        mode="empty"
        icon="inbox"
        heading="No field activity yet"
        message="Daily logs, crew check-ins, and task progress from the field will appear here."
      ></fb-state>`;
    const r = this.report;
    if (!r) return html`${nothing}`;

    const progress = r.task_progress ?? [];
    return html`<div class="report">
      <section class="card">
        <div class="meta-chips">
          <fb-chip class="mono">${r.log_date.slice(0, 10)}</fb-chip>
          ${r.weather_conditions ? html`<fb-chip>${r.weather_conditions}</fb-chip>` : nothing}
          <fb-chip class="mono">${r.crew_count} crew</fb-chip>
          <fb-chip class="mono">${r.photo_count} photo${r.photo_count === 1 ? '' : 's'}</fb-chip>
          ${r.safety_incidents
            ? html`<fb-badge size="sm" status="critical">Safety incident</fb-badge>`
            : nothing}
        </div>
        ${r.work_summary
          ? html`<p class="section-label">Field notes</p>
              <fb-markdown .source=${r.work_summary}></fb-markdown>`
          : html`<p class="page-sub">No daily-log narrative for this date.</p>`}
        ${r.safety_incidents
          ? html`<p class="section-label" style="margin-top: var(--fb-spacing-md)">
                Safety incident (internal)
              </p>
              <div class="safety"><fb-markdown .source=${r.safety_incidents}></fb-markdown></div>`
          : nothing}
      </section>

      ${progress.length > 0
        ? html`<section class="card">
            <p class="section-label">Task progress</p>
            <fb-data-table
              .columns=${TASK_COLUMNS}
              .rows=${progress.map((p) => ({ ...p, id: p.task_id }))}
              caption="Per-task progress reported on ${r.log_date.slice(0, 10)}"
            ></fb-data-table>
          </section>`
        : nothing}
      ${this.renderPhotos()} ${this.renderDigestAction()} ${this.renderDraftAction()}
    </div>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Daily Reports</h1>
            <p class="page-sub">
              The day's field activity, recapped — generate an office digest or draft a client
              update.
            </p>
          </div>
        </div>

        ${this.loading
          ? html`<fb-state mode="loading" skeleton="card" rows="4"></fb-state>`
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
                  message="Daily reports are assembled per project from field activity."
                ></fb-state>`
              : html`<div class="pickers">
                    <div class="picker">
                      <fb-select
                        label="Project"
                        .options=${this.projectOptions()}
                        value=${this.projectId}
                        @change=${this.onProject}
                      ></fb-select>
                    </div>
                    <div class="picker">
                      <fb-select
                        label="Date"
                        .options=${this.dateOptions()}
                        value=${this.selectedDate}
                        ?disabled=${this.datesLoading || this.dates.length === 0}
                        @change=${this.onDate}
                      ></fb-select>
                    </div>
                  </div>
                  ${this.renderReport()}`}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-reports-page': FbReportsPage;
  }
}
