import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import { portfolioStyles } from './portfolio-styles.js';
import '../atoms/fb-badge.js';
import '../atoms/fb-button.js';
import '../atoms/fb-icon.js';
import '../organisms/fb-modal.js';
import '../organisms/fb-state.js';
import { listEmployees, listCertifications } from '../../api/endpoints/hr.js';
import type { Employee, Certification, CertificationStatus } from '../../types/models.js';
import { ApiError } from '../../api/errors.js';
import { authClaims } from '../../state/authStore.js';
import type { BadgeStatus } from '../atoms/fb-badge.js';

/** Days from today until `date` (YYYY-MM-DD); negative when already past. */
function daysUntil(date: string): number {
  const then = new Date(date + 'T00:00:00').getTime();
  if (Number.isNaN(then)) return Number.POSITIVE_INFINITY;
  return Math.ceil((then - Date.now()) / 86_400_000);
}

function certBadge(status: CertificationStatus): { status: BadgeStatus; label: string } {
  switch (status) {
    case 'active':
      return { status: 'active', label: 'Active' };
    case 'expired':
      return { status: 'critical', label: 'Expired' };
    case 'revoked':
    default:
      return { status: 'offline', label: 'Revoked' };
  }
}

const EXPIRY_WINDOW_DAYS = 30;

/**
 * `fb-hr-page` — employees + certifications (UX_CORE_SCREENS §8). Owner/admin
 * only (router-gated). Employee and certification rows carry PII (names,
 * phones); per CLAUDE.md PII rules this surface renders them but NEVER logs
 * them. Certifications load per employee on demand; the dialog raises an expiry
 * banner when any cert is expired or lapses within the next 30 days.
 */
@customElement('fb-hr-page')
export class FbHrPage extends FBElement {
  static override styles = [
    FBElement.styles,
    portfolioStyles,
    css`
      .emp-card {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
        padding: var(--fb-spacing-md);
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
      }
      .emp-name {
        font-size: var(--fb-text-title-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .emp-meta {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .emp-actions {
        margin-top: var(--fb-spacing-xs);
      }
      .cert-list {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-sm);
      }
      .cert-row {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
        padding-bottom: var(--fb-spacing-sm);
        border-bottom: 1px solid var(--fb-border);
      }
      .cert-row:last-child {
        border-bottom: none;
        padding-bottom: 0;
      }
      .cert-type {
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .cert-exp {
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
    `,
  ];

  @state() private employees: Employee[] = [];
  @state() private loading = true;
  @state() private errorCode: string | null = null;

  // Certification dialog state.
  @state() private viewing: Employee | null = null;
  @state() private certs: Certification[] = [];
  @state() private certsLoading = false;
  @state() private certsError: string | null = null;

  private get orgId(): string {
    return authClaims.get()?.orgId ?? '';
  }

  override connectedCallback(): void {
    super.connectedCallback();
    void this.load();
  }

  private async load(): Promise<void> {
    this.loading = true;
    this.errorCode = null;
    try {
      this.employees = await listEmployees(this.orgId);
    } catch (err) {
      this.errorCode = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.loading = false;
    }
  }

  private async openCerts(emp: Employee): Promise<void> {
    this.viewing = emp;
    this.certs = [];
    this.certsError = null;
    this.certsLoading = true;
    try {
      this.certs = await listCertifications(this.orgId, emp.id);
    } catch (err) {
      this.certsError = err instanceof ApiError ? err.code : 'UNKNOWN';
    } finally {
      this.certsLoading = false;
    }
  }

  private closeCerts(): void {
    this.viewing = null;
  }

  /** Expiry banner summary across the loaded certs (severity-ranked). */
  private expirySummary(): { kind: 'warn' | 'crit'; text: string } | null {
    const expired = this.certs.filter((c) => c.status === 'expired').length;
    const soon = this.certs.filter(
      (c) => c.status === 'active' && daysUntil(c.expiry_date) <= EXPIRY_WINDOW_DAYS,
    ).length;
    if (expired > 0)
      return {
        kind: 'crit',
        text: `${expired} certification${expired > 1 ? 's are' : ' is'} expired.`,
      };
    if (soon > 0)
      return {
        kind: 'warn',
        text: `${soon} certification${soon > 1 ? 's expire' : ' expires'} within ${EXPIRY_WINDOW_DAYS} days.`,
      };
    return null;
  }

  private renderCard(e: Employee): TemplateResult {
    return html`<div class="emp-card">
      <span class="emp-name">${e.first_name} ${e.last_name}</span>
      <span class="emp-meta">${e.role}${e.phone ? ` · ${e.phone}` : ''}</span>
      ${e.hire_date ? html`<span class="emp-meta">Hired ${e.hire_date}</span>` : nothing}
      <div class="emp-actions">
        <fb-button
          size="sm"
          variant="secondary"
          icon="shield-check"
          @click=${() => void this.openCerts(e)}
          >Certifications</fb-button
        >
      </div>
    </div>`;
  }

  private renderCertsBody(): TemplateResult {
    if (this.certsLoading)
      return html`<fb-state mode="loading" skeleton="text" rows="3"></fb-state>`;
    if (this.certsError)
      return html`<fb-state mode="error" error-code=${this.certsError}></fb-state>`;
    if (this.certs.length === 0)
      return html`<fb-state
        mode="empty"
        icon="shield"
        heading="No certifications on file"
      ></fb-state>`;
    const banner = this.expirySummary();
    return html`<div class="certs-body">
      ${banner
        ? html`<p class="banner ${banner.kind === 'crit' ? '' : 'warn'}" role="status">
            <fb-icon name="alert-triangle" size="16"></fb-icon>${banner.text}
          </p>`
        : nothing}
      <div class="cert-list">
        ${this.certs.map((c) => {
          const badge = certBadge(c.status);
          return html`<div class="cert-row">
            <div>
              <div class="cert-type">${c.cert_type}</div>
              <div class="cert-exp">
                Expires ${c.expiry_date}${c.cert_number ? ` · #${c.cert_number}` : ''}
              </div>
            </div>
            <fb-badge size="sm" status=${badge.status}>${badge.label}</fb-badge>
          </div>`;
        })}
      </div>
    </div>`;
  }

  private renderDialog(): TemplateResult {
    const e = this.viewing;
    if (!e) return html`${nothing}`;
    return html`<fb-modal
      open
      heading="${e.first_name} ${e.last_name} — Certifications"
      @close=${this.closeCerts}
    >
      ${this.renderCertsBody()}
      <fb-button slot="footer" variant="secondary" @click=${this.closeCerts}>Close</fb-button>
    </fb-modal>`;
  }

  override render(): TemplateResult {
    return html`
      <div class="page">
        <div class="page-head">
          <div>
            <h1 class="page-title">HR &amp; Certifications</h1>
            <p class="page-sub">Your crew and their certification status.</p>
          </div>
        </div>

        ${this.loading
          ? html`<fb-state mode="loading" skeleton="card" rows="6"></fb-state>`
          : this.errorCode
            ? html`<fb-state
                mode="error"
                error-code=${this.errorCode}
                retryable
                @retry=${() => void this.load()}
              ></fb-state>`
            : this.employees.length === 0
              ? html`<fb-state
                  mode="empty"
                  icon="users"
                  heading="No employees yet"
                  message="Team members will appear here once they’re added."
                ></fb-state>`
              : html`<div class="card-grid">${this.employees.map((e) => this.renderCard(e))}</div>`}
        ${this.renderDialog()}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-hr-page': FbHrPage;
  }
}
