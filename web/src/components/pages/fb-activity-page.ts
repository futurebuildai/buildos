import { html, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../organisms/fb-audit-trail.js';
import '../organisms/fb-state.js';
import { listAudit } from '../../api/endpoints/audit.js';
import type { AuditEntry } from '../../types/models.js';
import { ApiError, ErrorCode } from '../../api/errors.js';

interface AuditFilter {
  actionPrefix: string;
  from: string;
  to: string;
}

/**
 * `fb-activity-page` — the org-wide audit trail (UX_CORE_SCREENS §11, DSC §7.16).
 * Owner/admin only (router-gated). Wraps `fb-audit-trail` and owns the data fetch
 * for the `action_prefix`/date filters it emits. The audit read route is a tracked
 * backend gap (not yet mounted): a NOT_FOUND / NOT_IMPLEMENTED response degrades to
 * a calm "not available yet" panel rather than a hard error, so the surface ships
 * ahead of the endpoint and lights up automatically once the route lands.
 */
@customElement('fb-activity-page')
export class FbActivityPage extends FBElement {
  static override styles = [FBElement.styles, portfolioStyles];

  @state() private entries: AuditEntry[] = [];
  @state() private loading = true;
  @state() private errorCode: string | null = null;
  /** True when the backend route is absent (degrade, don't alarm). */
  @state() private unavailable = false;
  @state() private filter: AuditFilter = { actionPrefix: '', from: '', to: '' };

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  private async load(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    this.unavailable = false;
    try {
      this.entries = await listAudit({
        ...(this.filter.actionPrefix ? { action_prefix: this.filter.actionPrefix } : {}),
        ...(this.filter.from ? { from: this.filter.from } : {}),
        ...(this.filter.to ? { to: this.filter.to } : {}),
      });
    } catch (err) {
      if (
        err instanceof ApiError &&
        (err.code === ErrorCode.NOT_FOUND || err.code === ErrorCode.NOT_IMPLEMENTED)
      ) {
        this.unavailable = true;
      } else {
        this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
      }
    } finally {
      this.loading = false;
    }
  }

  private onFilter(e: Event): void {
    this.filter = (e as CustomEvent<AuditFilter>).detail;
    void this.load();
  }

  private renderBody(): TemplateResult {
    if (this.loading) return html`<fb-state mode="loading" skeleton="text" rows="6"></fb-state>`;
    if (this.unavailable)
      return html`<fb-state
        mode="empty"
        icon="history"
        heading="Activity log isn't available yet"
        message="The audit feed turns on once the backend exposes it. Nothing to show here for now."
      ></fb-state>`;
    if (this.errorCode)
      return html`<fb-state
        mode="error"
        error-code=${this.errorCode}
        retryable
        @retry=${() => void this.load()}
      ></fb-state>`;
    return html`<fb-audit-trail
      .entries=${this.entries}
      action-prefix=${this.filter.actionPrefix}
      from=${this.filter.from}
      to=${this.filter.to}
      @filter=${this.onFilter}
    ></fb-audit-trail>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">Activity</h1>
            <p class="page-sub">Who changed what, in reverse-chronological order.</p>
          </div>
        </div>
        ${this.renderBody()}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-activity-page': FbActivityPage;
  }
}
