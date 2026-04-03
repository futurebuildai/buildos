import { html, css, nothing } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import {
  listEmployees, listCertifications,
  type Employee, type Certification, ApiError,
} from '../../state/api.js';
import { currentOrg } from '../../state/store.js';

@customElement('fb-hr-view')
export class FBHRView extends FBBaseElement {
  @state() private _loading = true;
  @state() private _error = '';
  @state() private _employees: Employee[] = [];
  @state() private _selectedEmployee: string | null = null;
  @state() private _certifications: Certification[] = [];
  @state() private _loadingCerts = false;

  static styles = [
    ...FBBaseElement.styles,
    css`
      :host { display: block; padding: var(--fb-space-6); }

      .page-header {
        margin-bottom: var(--fb-space-6);
      }

      .page-header h1 {
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-text-primary);
        margin: 0;
      }

      .page-header p {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        margin-top: var(--fb-space-1);
      }

      .content-grid {
        display: grid;
        grid-template-columns: 2fr 1fr;
        gap: var(--fb-space-6);
      }

      @media (max-width: 1024px) { .content-grid { grid-template-columns: 1fr; } }

      .table-container {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
        overflow-x: auto;
      }

      .section-title {
        font-size: var(--fb-text-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-4);
      }

      table { width: 100%; border-collapse: collapse; }

      th {
        text-align: left;
        padding: var(--fb-space-3) var(--fb-space-4);
        font-size: var(--fb-text-xs);
        font-weight: 600;
        color: var(--fb-text-muted);
        text-transform: uppercase;
        letter-spacing: 0.05em;
        border-bottom: 1px solid var(--fb-border);
        white-space: nowrap;
      }

      td {
        padding: var(--fb-space-3) var(--fb-space-4);
        font-size: var(--fb-text-sm);
        color: var(--fb-text-primary);
        border-bottom: 1px solid var(--fb-border);
        white-space: nowrap;
      }

      td.mono { font-family: var(--fb-font-mono); font-variant-numeric: tabular-nums; }
      tr:hover td { background: rgba(255, 255, 255, 0.02); }

      tr.selected td {
        background: rgba(0, 255, 163, 0.05);
        border-color: rgba(0, 255, 163, 0.1);
      }

      tr { cursor: pointer; }

      .status-badge {
        display: inline-block;
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 600;
      }

      .status-badge.active { background: rgba(0, 255, 163, 0.1); color: var(--fb-gable-green); }
      .status-badge.inactive { background: rgba(148, 163, 184, 0.1); color: var(--fb-text-secondary); }

      /* Certifications Panel */
      .cert-panel {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
      }

      .cert-list {
        display: flex;
        flex-direction: column;
        gap: var(--fb-space-3);
      }

      .cert-item {
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-md);
        padding: var(--fb-space-3);
      }

      .cert-item.expiring {
        border-color: rgba(245, 158, 11, 0.3);
        background: rgba(245, 158, 11, 0.05);
      }

      .cert-item.expired {
        border-color: rgba(244, 63, 94, 0.3);
        background: rgba(244, 63, 94, 0.05);
      }

      .cert-name {
        font-size: var(--fb-text-sm);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-1);
      }

      .cert-issuer {
        font-size: var(--fb-text-xs);
        color: var(--fb-text-secondary);
      }

      .cert-dates {
        display: flex;
        justify-content: space-between;
        margin-top: var(--fb-space-2);
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
      }

      .cert-expiry.expiring { color: var(--fb-amber); }
      .cert-expiry.expired { color: var(--fb-safety-red); }

      .empty-state {
        text-align: center;
        padding: var(--fb-space-6);
        color: var(--fb-text-muted);
        font-size: var(--fb-text-sm);
      }

      .error-banner {
        color: var(--fb-safety-red); padding: var(--fb-space-4);
        background: rgba(244, 63, 94, 0.1); border-radius: var(--fb-radius-md);
        border: 1px solid rgba(244, 63, 94, 0.2); margin-bottom: var(--fb-space-4);
        font-size: var(--fb-text-sm);
      }

      .loading-container { display: flex; align-items: center; justify-content: center; min-height: 300px; }
    `,
  ];

  connectedCallback() {
    super.connectedCallback();
    this._loadEmployees();
  }

  render() {
    if (this._loading) {
      return html`<div class="loading-container"><fb-spinner></fb-spinner></div>`;
    }

    return html`
      <div class="page-header">
        <h1>HR & Employees</h1>
        <p>${this._employees.length} employee${this._employees.length !== 1 ? 's' : ''} in your organization.</p>
      </div>

      ${this._error ? html`<div class="error-banner">${this._error}</div>` : nothing}

      <div class="content-grid">
        <div class="table-container">
          <h2 class="section-title">Employees</h2>
          ${this._employees.length === 0
            ? html`<div class="empty-state">No employees found.</div>`
            : html`
              <table>
                <thead>
                  <tr>
                    <th>Name</th>
                    <th>Role</th>
                    <th>Email</th>
                    <th>Status</th>
                    <th>Hire Date</th>
                  </tr>
                </thead>
                <tbody>
                  ${this._employees.map(
                    (emp) => html`
                      <tr
                        class="${this._selectedEmployee === emp.id ? 'selected' : ''}"
                        @click=${() => this._selectEmployee(emp.id)}
                      >
                        <td>${emp.name}</td>
                        <td>${emp.role}</td>
                        <td>${emp.email ?? '\u2014'}</td>
                        <td><span class="status-badge ${emp.status}">${emp.status}</span></td>
                        <td class="mono">${this._formatDate(emp.hire_date)}</td>
                      </tr>
                    `,
                  )}
                </tbody>
              </table>
            `}
        </div>

        <div class="cert-panel">
          <h2 class="section-title">Certifications</h2>
          ${!this._selectedEmployee
            ? html`<div class="empty-state">Select an employee to view certifications.</div>`
            : this._loadingCerts
              ? html`<div class="empty-state"><fb-spinner></fb-spinner></div>`
              : this._certifications.length === 0
                ? html`<div class="empty-state">No certifications on file.</div>`
                : html`
                  <div class="cert-list">
                    ${this._certifications.map((cert) => {
                      const expiryClass = this._getCertExpiryClass(cert);
                      return html`
                        <div class="cert-item ${expiryClass}">
                          <div class="cert-name">${cert.name}</div>
                          <div class="cert-issuer">${cert.issuer}</div>
                          <div class="cert-dates">
                            <span>Issued: ${this._formatDate(cert.issue_date)}</span>
                            <span class="cert-expiry ${expiryClass}">
                              ${cert.expiry_date ? `Exp: ${this._formatDate(cert.expiry_date)}` : 'No expiry'}
                            </span>
                          </div>
                        </div>
                      `;
                    })}
                  </div>
                `}
        </div>
      </div>
    `;
  }

  private _getCertExpiryClass(cert: Certification): string {
    if (!cert.expiry_date) return '';
    const expiry = new Date(cert.expiry_date);
    const now = new Date();
    const daysUntilExpiry = Math.floor((expiry.getTime() - now.getTime()) / (1000 * 60 * 60 * 24));
    if (daysUntilExpiry < 0) return 'expired';
    if (daysUntilExpiry < 30) return 'expiring';
    return '';
  }

  private _formatDate(dateStr: string): string {
    try {
      return new Date(dateStr).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
    } catch {
      return dateStr;
    }
  }

  private async _selectEmployee(employeeID: string) {
    this._selectedEmployee = employeeID;
    this._loadingCerts = true;
    this._certifications = [];
    const orgID = currentOrg.get();

    try {
      const res = await listCertifications(orgID, employeeID);
      this._certifications = res.certifications;
    } catch (err) {
      this.showToast('Failed to load certifications', 'error');
    } finally {
      this._loadingCerts = false;
    }
  }

  private async _loadEmployees() {
    this._loading = true;
    this._error = '';
    const orgID = currentOrg.get();
    if (!orgID) {
      this._loading = false;
      this._error = 'No organization selected.';
      return;
    }

    try {
      const res = await listEmployees(orgID);
      this._employees = res.employees;
    } catch (err) {
      this._error = err instanceof ApiError ? `Failed to load employees (${err.status})` : 'Failed to load employees';
    } finally {
      this._loading = false;
    }
  }
}
