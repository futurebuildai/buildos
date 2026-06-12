import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-button.js';
import '../atoms/fb-chip.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-input.js';
import '../atoms/fb-markdown.js';
import '../atoms/fb-select.js';
import '../organisms/fb-state.js';
import { listProjects } from '../../api/endpoints/projects.js';
import { listDailyReports, getDailyReport } from '../../api/endpoints/reports.js';
import {
  createClientUpdate,
  listClientUpdates,
  updateClientUpdate,
  sendClientUpdate,
} from '../../api/endpoints/client-updates.js';
import {
  createShareLink,
  listShareLinks,
  revokeShareLink,
} from '../../api/endpoints/share-links.js';
import type {
  Project,
  DailyReport,
  DailyReportSummary,
  ClientUpdate,
  PhotoRef,
  ShareLink,
} from '../../types/models.js';
import type { SelectOption } from '../atoms/fb-select.js';
import { ApiError, ErrorCode } from '../../api/errors.js';
import { aiConfigured, markAiUnconfigured } from '../../state/capabilityStore.js';
import { hasRole } from '../../state/authStore.js';
import { navigate } from '../../router.js';

/** Composer step state. */
type ComposerStep = 'pick' | 'editing' | 'confirming' | 'sent';
/** AI/draft generation state (mirrors fb-briefing-page heroes). */
type AiState = 'idle' | 'busy' | 'gated' | 'error';

/**
 * `fb-client-update-page` — the human-in-the-loop client-update composer
 * (Chunk D of DAILY_REPORTS_CLIENT_UPDATES). owner/admin only (route-gated).
 *
 * Flow: pick a project + date → generate the redacted AI draft (POST creates a
 * draft row) → EDIT the client-safe subject + body (textarea) and curate which
 * of the day's photos the homeowner sees → PREVIEW (markdown + photo strip) →
 * confirm → SEND via Resend. The send NEVER fires without an explicit confirm,
 * and a failed send (NO_CLIENT_CONTACT / MAILER_UNCONFIGURED) is surfaced
 * loudly — the operator MUST know it did not go out. A sent-history list shows
 * past updates for the project.
 *
 * Deep-link: ?project=<id>&date=<YYYY-MM-DD> preselects + opens the composer
 * (the Daily Reports "Draft client update" action links here).
 */
@customElement('fb-client-update-page')
export class FbClientUpdatePage extends FBElement {
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
      .card {
        padding: var(--fb-spacing-lg);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
        margin-bottom: var(--fb-spacing-lg);
      }
      .section-label {
        font-size: var(--fb-text-title-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin: 0 0 var(--fb-spacing-sm);
      }
      .field {
        margin-bottom: var(--fb-spacing-md);
      }
      .field label {
        display: block;
        font-size: var(--fb-text-body-sm);
        font-weight: 600;
        color: var(--fb-text-secondary);
        margin-bottom: var(--fb-spacing-xs);
      }
      textarea {
        width: 100%;
        min-height: 12rem;
        box-sizing: border-box;
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        font: inherit;
        color: var(--fb-text-primary);
        background: var(--fb-surface-2);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
        resize: vertical;
      }
      textarea:focus-visible {
        outline: 2px solid var(--fb-focus-ring, var(--md-sys-color-primary));
        outline-offset: 1px;
      }
      .photo-grid {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
      }
      .photo-pick {
        position: relative;
        cursor: pointer;
        border-radius: var(--fb-radius-md);
      }
      .photo-pick img {
        width: 7rem;
        height: 7rem;
        object-fit: cover;
        border-radius: var(--fb-radius-md);
        border: 2px solid transparent;
        background: var(--fb-surface-2);
        display: block;
      }
      .photo-pick.selected img {
        border-color: var(--md-sys-color-primary);
      }
      .photo-pick input {
        position: absolute;
        top: var(--fb-spacing-xs);
        left: var(--fb-spacing-xs);
        width: 1.1rem;
        height: 1.1rem;
      }
      .actions {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
        margin-top: var(--fb-spacing-md);
      }
      .preview-subject {
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-spacing-sm);
      }
      .confirm-note {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
        margin-bottom: var(--fb-spacing-sm);
      }
      .send-error {
        border-left: 3px solid var(--fb-safety-red, var(--fb-status-critical));
        padding-left: var(--fb-spacing-md);
        color: var(--fb-text-primary);
        margin-top: var(--fb-spacing-md);
      }
      .send-error strong {
        color: var(--fb-status-critical);
      }
      .photo-strip {
        display: flex;
        flex-wrap: wrap;
        gap: var(--fb-spacing-sm);
        margin-top: var(--fb-spacing-sm);
      }
      .photo-strip img {
        width: 7rem;
        height: 7rem;
        object-fit: cover;
        border-radius: var(--fb-radius-md);
        border: 1px solid var(--md-sys-color-outline);
      }
      .history-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--fb-spacing-md);
        padding: var(--fb-spacing-sm) 0;
        border-bottom: 1px solid var(--md-sys-color-outline-variant, var(--md-sys-color-outline));
      }
      .history-row:last-child {
        border-bottom: none;
      }
      .history-meta {
        display: flex;
        flex-direction: column;
        gap: 2px;
      }
      .history-sub {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .mono {
        font-family: var(--fb-font-mono);
      }
      .history-right {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
      }
      .share-panel {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-sm) 0 var(--fb-spacing-md);
      }
      .share-actions {
        display: flex;
        gap: var(--fb-spacing-sm);
      }
      .share-sub {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .share-error {
        font-size: var(--fb-text-body-sm);
        color: var(--md-sys-color-error, var(--fb-text-secondary));
        margin: 0;
      }
      .share-note {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
        margin: 0 0 var(--fb-spacing-xs);
      }
      .share-url-row {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
      }
      .share-url {
        flex: 1;
        min-width: 0;
        padding: var(--fb-spacing-xs) var(--fb-spacing-sm);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-primary);
        background: var(--fb-surface-2, var(--md-sys-color-surface-container));
        border: 1px solid var(--md-sys-color-outline-variant, var(--md-sys-color-outline));
        border-radius: var(--fb-radius-sm, 6px);
      }
      .share-list {
        list-style: none;
        margin: 0;
        padding: 0;
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
      }
      .share-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
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

  // Composer.
  @state() private step: ComposerStep = 'pick';
  @state() private draftState: AiState = 'idle';
  @state() private current: ClientUpdate | null = null;
  @state() private subject = '';
  @state() private body = '';
  @state() private selectedPhotos = new Set<string>();
  @state() private saving = false;
  @state() private sending = false;
  @state() private sendError: string | null = null;

  // History.
  @state() private history: ClientUpdate[] = [];
  @state() private historyLoading = false;

  // Public share links (Chunk E). Keyed by client_update_id. expandedShareId is
  // the history row whose links panel is open; mintedUrls holds the one-time URL
  // returned at create (shown once, copy-to-clipboard). shareBusy/shareError
  // track the in-flight create/revoke per update.
  @state() private shareLinks = new Map<string, ShareLink[]>();
  @state() private expandedShareId: string | null = null;
  @state() private mintedUrls = new Map<string, string>();
  @state() private shareBusy: string | null = null;
  @state() private shareError: string | null = null;
  @state() private copiedUrl = false;

  override connectedCallback(): void {
    super.connectedCallback();
    void this.init();
  }

  private async init(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      this.projects = await listProjects();
      const params = new URLSearchParams(window.location.search);
      const deepProject = params.get('project');
      const deepDate = params.get('date');
      const first = this.projects[0];
      this.projectId =
        deepProject && this.projects.some((p) => p.id === deepProject)
          ? deepProject
          : (first?.id ?? '');
      if (this.projectId) {
        await this.loadDates(deepDate ?? undefined);
      }
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private async loadDates(preferDate?: string): Promise<void> {
    if (!this.projectId) return;
    this.datesLoading = true;
    this.resetComposer();
    try {
      this.dates = await listDailyReports(this.projectId);
      const want =
        preferDate && this.dates.some((d) => d.log_date.slice(0, 10) === preferDate)
          ? preferDate
          : (this.dates[0]?.log_date.slice(0, 10) ?? '');
      this.selectedDate = want;
      await Promise.all([this.loadReport(), this.loadHistory()]);
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.datesLoading = false;
    }
  }

  private async loadReport(): Promise<void> {
    if (!this.projectId || !this.selectedDate) {
      this.report = null;
      return;
    }
    try {
      this.report = await getDailyReport(this.projectId, this.selectedDate);
    } catch {
      this.report = null;
    }
  }

  private async loadHistory(): Promise<void> {
    if (!this.projectId) return;
    this.historyLoading = true;
    try {
      this.history = await listClientUpdates(this.projectId);
    } catch {
      this.history = [];
    } finally {
      this.historyLoading = false;
    }
  }

  private resetComposer(): void {
    this.step = 'pick';
    this.draftState = 'idle';
    this.current = null;
    this.subject = '';
    this.body = '';
    this.selectedPhotos = new Set();
    this.sendError = null;
  }

  private onProject(e: Event): void {
    this.projectId = (e as CustomEvent<{ value: string }>).detail.value;
    void this.loadDates();
  }

  private onDate(e: Event): void {
    this.selectedDate = (e as CustomEvent<{ value: string }>).detail.value;
    this.resetComposer();
    void this.loadReport();
  }

  private projectOptions(): SelectOption[] {
    return this.projects.map((p) => ({ value: p.id, label: p.name }));
  }

  private dateOptions(): SelectOption[] {
    return this.dates.map((d) => {
      const date = d.log_date.slice(0, 10);
      return { value: date, label: date };
    });
  }

  // --------------------------- Generate draft ----------------------------
  private async onGenerateDraft(): Promise<void> {
    if (!this.projectId || !this.selectedDate) return;
    if (!aiConfigured.get()) {
      this.draftState = 'gated';
      return;
    }
    this.draftState = 'busy';
    try {
      const cu = await createClientUpdate(this.projectId, this.selectedDate);
      this.current = cu;
      this.subject = cu.subject;
      this.body = cu.edited_body;
      this.selectedPhotos = new Set(cu.photo_asset_ids);
      this.draftState = 'idle';
      this.step = 'editing';
      void this.loadHistory();
    } catch (err) {
      if (
        err instanceof ApiError &&
        (err.code === ErrorCode.SERVICE_UNAVAILABLE || err.isAiUnconfigured)
      ) {
        markAiUnconfigured();
        this.draftState = 'gated';
        return;
      }
      this.draftState = 'error';
    }
  }

  // ------------------------------- Editing -------------------------------
  private onSubject(e: Event): void {
    this.subject = (e as CustomEvent<{ value: string }>).detail.value;
  }

  private onBody(e: Event): void {
    this.body = (e.target as HTMLTextAreaElement).value;
  }

  private togglePhoto(assetId: string): void {
    const next = new Set(this.selectedPhotos);
    if (next.has(assetId)) next.delete(assetId);
    else next.add(assetId);
    this.selectedPhotos = next;
  }

  /** Persist the operator edit (subject/body/photos), then move to preview. */
  private async onSaveAndPreview(): Promise<void> {
    if (!this.current) return;
    this.saving = true;
    this.sendError = null;
    try {
      const cu = await updateClientUpdate(this.current.id, {
        subject: this.subject,
        edited_body: this.body,
        photo_asset_ids: [...this.selectedPhotos],
      });
      this.current = cu;
      this.step = 'confirming';
    } catch (err) {
      this.sendError =
        err instanceof ApiError && err.code === 'ALREADY_SENT'
          ? 'This update has already been sent and cannot be edited.'
          : 'Could not save the draft. Please try again.';
    } finally {
      this.saving = false;
    }
  }

  // ------------------------------- Sending -------------------------------
  private backToEditing(): void {
    this.step = 'editing';
    this.sendError = null;
  }

  /** The human-pressed send (after the confirm step). Surfaces failure loudly. */
  private async onConfirmSend(): Promise<void> {
    if (!this.current) return;
    this.sending = true;
    this.sendError = null;
    try {
      const cu = await sendClientUpdate(this.current.id);
      this.current = cu;
      this.step = 'sent';
      void this.loadHistory();
    } catch (err) {
      this.sendError = this.sendErrorMessage(err);
    } finally {
      this.sending = false;
    }
  }

  /** Map a send failure to operator-facing copy. The operator MUST know it did
   *  NOT go out. */
  private sendErrorMessage(err: unknown): string {
    if (!(err instanceof ApiError)) return 'Send failed. The update was NOT sent.';
    switch (err.code) {
      case 'NO_CLIENT_CONTACT':
        return 'This project has no homeowner email. Add a client email to the project, then send. The update was NOT sent.';
      case 'MAILER_UNCONFIGURED':
        return 'Email is not configured (no Resend API key). The update was NOT sent — add a Resend key in Integrations and try again.';
      case 'ALREADY_SENT':
        return 'This update has already been sent.';
      default:
        return 'The email provider rejected the send. The update was NOT sent.';
    }
  }

  private isMailerUnconfigured(): boolean {
    return this.sendError?.includes('Resend') === true;
  }

  // ------------------------------- Render --------------------------------
  private reportPhotos(): PhotoRef[] {
    return this.report?.photos ?? [];
  }

  private renderGenerate(): TemplateResult {
    return html`<section class="card">
      <p class="section-label">Compose a client update</p>
      <p class="page-sub">
        Generate a homeowner-safe draft from this day's field activity, then edit and send it.
      </p>
      <div class="actions">
        <fb-button
          variant="primary"
          icon="sparkles"
          data-test="generate-draft"
          ?disabled=${!this.selectedDate}
          ?loading=${this.draftState === 'busy'}
          @click=${() => void this.onGenerateDraft()}
          >Generate draft</fb-button
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
            @retry=${() => void this.onGenerateDraft()}
          ></fb-state>`
        : nothing}
    </section>`;
  }

  private renderEditor(): TemplateResult {
    const photos = this.reportPhotos();
    return html`<section class="card">
      <p class="section-label">Edit the update</p>
      <div class="field">
        <label for="cu-subject">Subject</label>
        <fb-input
          id="cu-subject"
          label="Subject"
          .value=${this.subject}
          data-test="cu-subject"
          @input=${this.onSubject}
        ></fb-input>
      </div>
      <div class="field">
        <label for="cu-body">Message to the homeowner</label>
        <textarea
          id="cu-body"
          data-test="cu-body"
          aria-label="Message to the homeowner"
          .value=${this.body}
          @input=${this.onBody}
        ></textarea>
      </div>
      ${photos.length
        ? html`<div class="field">
            <label>Photos to include (${this.selectedPhotos.size} selected)</label>
            <div class="photo-grid" role="group" aria-label="Select photos to include">
              ${photos.map(
                (p) =>
                  html`<label
                    class="photo-pick ${this.selectedPhotos.has(p.asset_id) ? 'selected' : ''}"
                  >
                    <input
                      type="checkbox"
                      data-test="photo-pick"
                      .checked=${this.selectedPhotos.has(p.asset_id)}
                      aria-label="Include photo"
                      @change=${() => this.togglePhoto(p.asset_id)}
                    />
                    <img src=${p.thumb_url} alt="Jobsite photo" loading="lazy" decoding="async" />
                  </label>`,
              )}
            </div>
          </div>`
        : nothing}
      <div class="actions">
        <fb-button
          variant="primary"
          data-test="preview"
          ?loading=${this.saving}
          @click=${() => void this.onSaveAndPreview()}
          >Save &amp; preview</fb-button
        >
      </div>
      ${this.sendError
        ? html`<div class="send-error" role="alert" data-test="editor-error">
            ${this.sendError}
          </div>`
        : nothing}
    </section>`;
  }

  private renderPreview(): TemplateResult {
    const photos = this.reportPhotos().filter((p) => this.selectedPhotos.has(p.asset_id));
    return html`<section class="card">
      <p class="section-label">Preview</p>
      <p class="preview-subject" data-test="preview-subject">${this.subject}</p>
      <fb-markdown .source=${this.body}></fb-markdown>
      ${photos.length
        ? html`<div class="photo-strip">
            ${photos.map(
              (p) =>
                html`<img
                  src=${p.thumb_url}
                  alt="Jobsite photo"
                  loading="lazy"
                  decoding="async"
                />`,
            )}
          </div>`
        : nothing}
      <p class="confirm-note" style="margin-top: var(--fb-spacing-md)">
        This will email the homeowner on file for this project. Sending cannot be undone.
      </p>
      <div class="actions">
        <fb-button variant="ghost" @click=${() => this.backToEditing()}>Back to edit</fb-button>
        <fb-button
          variant="primary"
          icon="message-circle"
          data-test="confirm-send"
          ?loading=${this.sending}
          @click=${() => void this.onConfirmSend()}
          >Send to homeowner</fb-button
        >
      </div>
      ${this.sendError
        ? html`<div class="send-error" role="alert" data-test="send-error">
            <strong>Not sent.</strong> ${this.sendError}
            ${this.isMailerUnconfigured() && hasRole('owner')
              ? html` <fb-button
                  variant="ghost"
                  size="sm"
                  @click=${() => navigate('/settings/integrations')}
                  >Configure email</fb-button
                >`
              : nothing}
          </div>`
        : nothing}
    </section>`;
  }

  private renderSent(): TemplateResult {
    return html`<section class="card">
      <fb-state
        mode="empty"
        icon="check-circle"
        heading="Update sent"
        message="The homeowner has been emailed. It appears in the history below."
      ></fb-state>
      <div class="actions">
        <fb-button
          variant="secondary"
          data-test="compose-another"
          @click=${() => this.resetComposer()}
          >Compose another</fb-button
        >
      </div>
    </section>`;
  }

  private renderComposer(): TemplateResult {
    if (this.dates.length === 0) {
      return html`<fb-state
        mode="empty"
        icon="inbox"
        heading="No field activity yet"
        message="A client update is drafted from a day's field report. Once the field logs activity it will appear here."
      ></fb-state>`;
    }
    switch (this.step) {
      case 'editing':
        return this.renderEditor();
      case 'confirming':
        return this.renderPreview();
      case 'sent':
        return this.renderSent();
      case 'pick':
      default:
        return this.renderGenerate();
    }
  }

  private statusBadge(status: ClientUpdate['status']): TemplateResult {
    const map: Record<
      ClientUpdate['status'],
      { label: string; status: 'active' | 'warning' | 'critical' }
    > = {
      sent: { label: 'Sent', status: 'active' },
      draft: { label: 'Draft', status: 'warning' },
      failed: { label: 'Failed', status: 'critical' },
    };
    const m = map[status];
    return html`<fb-badge size="sm" status=${m.status}>${m.label}</fb-badge>`;
  }

  // ---- Public share links (Chunk E) -------------------------------------

  /** Expand/collapse a sent update's share-link panel, lazily loading links. */
  private async toggleShare(cu: ClientUpdate): Promise<void> {
    if (this.expandedShareId === cu.id) {
      this.expandedShareId = null;
      return;
    }
    this.expandedShareId = cu.id;
    this.shareError = null;
    if (!this.shareLinks.has(cu.id)) {
      await this.loadShareLinks(cu.id);
    }
  }

  private async loadShareLinks(updateId: string): Promise<void> {
    try {
      const links = await listShareLinks(updateId);
      const next = new Map(this.shareLinks);
      next.set(updateId, links);
      this.shareLinks = next;
    } catch {
      this.shareError = 'Could not load share links.';
    }
  }

  /** Mint a new public link. The returned URL is shown ONCE (copy it now). */
  private async onCreateShareLink(cu: ClientUpdate): Promise<void> {
    this.shareBusy = cu.id;
    this.shareError = null;
    this.copiedUrl = false;
    try {
      const res = await createShareLink(cu.id);
      const minted = new Map(this.mintedUrls);
      minted.set(cu.id, res.url);
      this.mintedUrls = minted;
      await this.loadShareLinks(cu.id);
    } catch (err) {
      if (err instanceof ApiError && err.code === 'UPDATE_NOT_SENT') {
        this.shareError = 'A public link can only be created after the update is sent.';
      } else {
        this.shareError = 'Could not create the public link. Try again.';
      }
    } finally {
      this.shareBusy = null;
    }
  }

  private async onRevokeShareLink(cu: ClientUpdate, linkId: string): Promise<void> {
    this.shareBusy = cu.id;
    this.shareError = null;
    try {
      await revokeShareLink(linkId);
      // Clear any one-time URL we may be showing for a now-revoked link.
      const minted = new Map(this.mintedUrls);
      minted.delete(cu.id);
      this.mintedUrls = minted;
      await this.loadShareLinks(cu.id);
    } catch {
      this.shareError = 'Could not revoke the link. Try again.';
    } finally {
      this.shareBusy = null;
    }
  }

  private async copyShareUrl(url: string): Promise<void> {
    try {
      await navigator.clipboard?.writeText(url);
      this.copiedUrl = true;
    } catch {
      // Clipboard blocked (no permission / insecure ctx): the URL is shown
      // selectable in the field, so the operator can copy it manually.
      this.copiedUrl = false;
    }
  }

  private renderShareLinks(cu: ClientUpdate): TemplateResult {
    const links = this.shareLinks.get(cu.id) ?? [];
    const active = links.filter((l) => l.status === 'active');
    const minted = this.mintedUrls.get(cu.id);
    const busy = this.shareBusy === cu.id;
    return html`<div class="share-panel" data-test="share-panel">
      ${this.shareError
        ? html`<p class="share-error" role="alert">${this.shareError}</p>`
        : nothing}
      ${minted
        ? html`<div class="share-minted" data-test="share-minted">
            <p class="share-note">
              Copy this link now — it is shown only once. Send it to the homeowner.
            </p>
            <div class="share-url-row">
              <input
                class="share-url mono"
                readonly
                .value=${minted}
                data-test="share-url"
                @focus=${(e: Event) => (e.target as HTMLInputElement).select()}
              />
              <fb-button
                size="sm"
                variant="secondary"
                @click=${() => void this.copyShareUrl(minted)}
              >
                ${this.copiedUrl ? 'Copied' : 'Copy'}
              </fb-button>
            </div>
          </div>`
        : nothing}
      <div class="share-actions">
        <fb-button
          size="sm"
          ?disabled=${busy}
          data-test="create-share-link"
          @click=${() => void this.onCreateShareLink(cu)}
          >${busy ? 'Creating…' : 'Create public link'}</fb-button
        >
      </div>
      ${active.length === 0
        ? html`<p class="share-sub">No active public links.</p>`
        : html`<ul class="share-list" data-test="share-list">
            ${active.map(
              (l) =>
                html`<li class="share-row">
                  <span class="share-sub mono"
                    >expires ${l.expires_at.slice(0, 10)} · ${l.view_count}
                    view${l.view_count === 1 ? '' : 's'}</span
                  >
                  <fb-button
                    size="sm"
                    variant="ghost"
                    ?disabled=${busy}
                    data-test="revoke-share-link"
                    @click=${() => void this.onRevokeShareLink(cu, l.id)}
                    >Revoke</fb-button
                  >
                </li>`,
            )}
          </ul>`}
    </div>`;
  }

  private renderHistory(): TemplateResult {
    return html`<section class="card">
      <p class="section-label">Sent history</p>
      ${this.historyLoading
        ? html`<fb-state mode="loading" skeleton="card" rows="3"></fb-state>`
        : this.history.length === 0
          ? html`<p class="page-sub" data-test="no-history">
              No client updates yet for this project.
            </p>`
          : html`<div data-test="history">
              ${this.history.map(
                (cu) =>
                  html`<div class="history-item">
                    <div class="history-row">
                      <div class="history-meta">
                        <span>${cu.subject || '(no subject)'}</span>
                        <span class="history-sub mono"
                          >${cu.period_start.slice(0, 10)}${cu.sent_at
                            ? ` · sent ${cu.sent_at.slice(0, 10)}`
                            : ''}</span
                        >
                      </div>
                      <div class="history-right">
                        ${cu.status === 'sent'
                          ? html`<fb-button
                              size="sm"
                              variant="ghost"
                              data-test="share-toggle"
                              aria-expanded=${this.expandedShareId === cu.id ? 'true' : 'false'}
                              @click=${() => void this.toggleShare(cu)}
                              >Share link</fb-button
                            >`
                          : nothing}
                        ${this.statusBadge(cu.status)}
                      </div>
                    </div>
                    ${this.expandedShareId === cu.id ? this.renderShareLinks(cu) : nothing}
                  </div>`,
              )}
            </div>`}
    </section>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Client Updates</h1>
            <p class="page-sub">
              Draft a homeowner-safe progress update, edit it, then send it. You always review and
              press send — nothing is sent automatically.
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
                @retry=${() => void this.init()}
              ></fb-state>`
            : this.projects.length === 0
              ? html`<fb-state
                  mode="empty"
                  icon="folder"
                  heading="No projects yet"
                  message="Client updates are composed per project from field activity."
                ></fb-state>`
              : html`
                  <div class="pickers">
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
                        label="Report date"
                        .options=${this.dateOptions()}
                        value=${this.selectedDate}
                        ?disabled=${this.datesLoading || this.dates.length === 0}
                        @change=${this.onDate}
                      ></fb-select>
                    </div>
                  </div>
                  ${this.renderComposer()} ${this.renderHistory()}
                `}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-client-update-page': FbClientUpdatePage;
  }
}
