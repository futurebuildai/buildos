import { html, css, nothing } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import {
  listAssets, createAsset, allocateAsset, listProjects,
  type FleetAsset, type Project, ApiError,
} from '../../state/api.js';
import { currentOrg } from '../../state/store.js';

@customElement('fb-fleet-view')
export class FBFleetView extends FBBaseElement {
  @state() private _loading = true;
  @state() private _error = '';
  @state() private _assets: FleetAsset[] = [];
  @state() private _projects: Project[] = [];
  @state() private _showCreateForm = false;
  @state() private _showAllocateForm: string | null = null;
  @state() private _formName = '';
  @state() private _formType = '';
  @state() private _formSerial = '';
  @state() private _allocProjectId = '';
  @state() private _allocStartDate = '';
  @state() private _allocEndDate = '';
  @state() private _submitting = false;

  static styles = [
    ...FBBaseElement.styles,
    css`
      :host { display: block; padding: var(--fb-space-6); }

      .page-header {
        display: flex;
        justify-content: space-between;
        align-items: center;
        margin-bottom: var(--fb-space-6);
      }

      .page-header h1 {
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-text-primary);
        margin: 0;
      }

      .add-btn {
        padding: var(--fb-space-2) var(--fb-space-4);
        background: var(--fb-gable-green);
        color: var(--fb-deep-space);
        border: none;
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-sm);
        font-weight: 600;
        cursor: pointer;
        font-family: var(--fb-font-body);
        transition: opacity var(--fb-transition-fast);
      }

      .add-btn:hover { opacity: 0.9; }

      .summary-cards {
        display: grid;
        grid-template-columns: repeat(4, 1fr);
        gap: var(--fb-space-4);
        margin-bottom: var(--fb-space-6);
      }

      @media (max-width: 768px) { .summary-cards { grid-template-columns: repeat(2, 1fr); } }

      .summary-card {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-4);
      }

      .summary-value {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-2xl);
        font-weight: 700;
        color: var(--fb-gable-green);
        font-variant-numeric: tabular-nums;
      }

      .summary-label {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        margin-top: var(--fb-space-1);
      }

      .table-container {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-5);
        overflow-x: auto;
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

      .status-badge {
        display: inline-block;
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 600;
      }

      .status-badge.available { background: rgba(0, 255, 163, 0.1); color: var(--fb-gable-green); }
      .status-badge.allocated { background: rgba(56, 189, 248, 0.1); color: var(--fb-blueprint-blue); }
      .status-badge.maintenance { background: rgba(245, 158, 11, 0.1); color: var(--fb-amber); }

      .action-btn {
        padding: var(--fb-space-1) var(--fb-space-3);
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 600;
        cursor: pointer;
        transition: all var(--fb-transition-fast);
        border: 1px solid var(--fb-border);
        background: transparent;
        color: var(--fb-text-secondary);
        font-family: var(--fb-font-body);
      }

      .action-btn:hover { border-color: var(--fb-gable-green); color: var(--fb-gable-green); }

      /* Form Modal Overlay */
      .modal-overlay {
        position: fixed;
        top: 0;
        left: 0;
        right: 0;
        bottom: 0;
        background: rgba(0, 0, 0, 0.6);
        display: flex;
        align-items: center;
        justify-content: center;
        z-index: 1000;
      }

      .modal {
        background: var(--fb-slate-steel);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-xl);
        padding: var(--fb-space-6);
        min-width: 400px;
        max-width: 500px;
      }

      .modal h2 {
        font-size: var(--fb-text-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-4);
      }

      .form-group {
        margin-bottom: var(--fb-space-4);
      }

      .form-group label {
        display: block;
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
        margin-bottom: var(--fb-space-1);
      }

      .form-group input, .form-group select {
        width: 100%;
        padding: var(--fb-space-2) var(--fb-space-3);
        background: var(--fb-surface);
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-md);
        color: var(--fb-text-primary);
        font-size: var(--fb-text-sm);
        font-family: var(--fb-font-body);
      }

      .form-group input:focus, .form-group select:focus {
        outline: none;
        border-color: var(--fb-gable-green);
      }

      .form-actions {
        display: flex;
        justify-content: flex-end;
        gap: var(--fb-space-2);
        margin-top: var(--fb-space-5);
      }

      .cancel-btn {
        padding: var(--fb-space-2) var(--fb-space-4);
        border: 1px solid var(--fb-border);
        background: transparent;
        color: var(--fb-text-secondary);
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-sm);
        cursor: pointer;
        font-family: var(--fb-font-body);
      }

      .submit-btn {
        padding: var(--fb-space-2) var(--fb-space-4);
        background: var(--fb-gable-green);
        color: var(--fb-deep-space);
        border: none;
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-sm);
        font-weight: 600;
        cursor: pointer;
        font-family: var(--fb-font-body);
      }

      .submit-btn:disabled { opacity: 0.5; cursor: not-allowed; }

      .error-banner {
        color: var(--fb-safety-red); padding: var(--fb-space-4);
        background: rgba(244, 63, 94, 0.1); border-radius: var(--fb-radius-md);
        border: 1px solid rgba(244, 63, 94, 0.2); margin-bottom: var(--fb-space-4);
        font-size: var(--fb-text-sm);
      }

      .loading-container { display: flex; align-items: center; justify-content: center; min-height: 300px; }
      .empty-state { text-align: center; padding: var(--fb-space-8); color: var(--fb-text-muted); font-size: var(--fb-text-sm); }
    `,
  ];

  connectedCallback() {
    super.connectedCallback();
    this._loadData();
  }

  render() {
    if (this._loading) {
      return html`<div class="loading-container"><fb-spinner></fb-spinner></div>`;
    }

    const total = this._assets.length;
    const available = this._assets.filter((a) => a.status === 'available').length;
    const allocated = this._assets.filter((a) => a.status === 'allocated').length;
    const maintenance = this._assets.filter((a) => a.status === 'maintenance').length;

    return html`
      <div class="page-header">
        <h1>Fleet Management</h1>
        <button class="add-btn" @click=${() => { this._showCreateForm = true; }}>+ Add Asset</button>
      </div>

      ${this._error ? html`<div class="error-banner">${this._error}</div>` : nothing}

      <div class="summary-cards">
        <div class="summary-card">
          <div class="summary-value">${total}</div>
          <div class="summary-label">Total Assets</div>
        </div>
        <div class="summary-card">
          <div class="summary-value" style="color: var(--fb-gable-green);">${available}</div>
          <div class="summary-label">Available</div>
        </div>
        <div class="summary-card">
          <div class="summary-value" style="color: var(--fb-blueprint-blue);">${allocated}</div>
          <div class="summary-label">Allocated</div>
        </div>
        <div class="summary-card">
          <div class="summary-value" style="color: var(--fb-amber);">${maintenance}</div>
          <div class="summary-label">Maintenance</div>
        </div>
      </div>

      <div class="table-container">
        ${this._assets.length === 0
          ? html`<div class="empty-state">No fleet assets. Add your first asset above.</div>`
          : html`
            <table>
              <thead>
                <tr>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Serial #</th>
                  <th>Status</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                ${this._assets.map(
                  (asset) => html`
                    <tr>
                      <td>${asset.name}</td>
                      <td>${asset.asset_type}</td>
                      <td class="mono">${asset.serial_number ?? '\u2014'}</td>
                      <td><span class="status-badge ${asset.status}">${asset.status}</span></td>
                      <td>
                        <button class="action-btn" @click=${() => { this._showAllocateForm = asset.id; }}>Allocate</button>
                      </td>
                    </tr>
                  `,
                )}
              </tbody>
            </table>
          `}
      </div>

      ${this._showCreateForm ? this._renderCreateModal() : nothing}
      ${this._showAllocateForm ? this._renderAllocateModal() : nothing}
    `;
  }

  private _renderCreateModal() {
    return html`
      <div class="modal-overlay" @click=${(e: Event) => { if (e.target === e.currentTarget) this._showCreateForm = false; }}>
        <div class="modal">
          <h2>Add Fleet Asset</h2>
          <div class="form-group">
            <label>Name</label>
            <input type="text" .value=${this._formName} @input=${(e: Event) => { this._formName = (e.target as HTMLInputElement).value; }} placeholder="e.g. CAT 320 Excavator" />
          </div>
          <div class="form-group">
            <label>Asset Type</label>
            <input type="text" .value=${this._formType} @input=${(e: Event) => { this._formType = (e.target as HTMLInputElement).value; }} placeholder="e.g. Excavator, Crane, Truck" />
          </div>
          <div class="form-group">
            <label>Serial Number (optional)</label>
            <input type="text" .value=${this._formSerial} @input=${(e: Event) => { this._formSerial = (e.target as HTMLInputElement).value; }} placeholder="e.g. CAT-320-2024-001" />
          </div>
          <div class="form-actions">
            <button class="cancel-btn" @click=${() => { this._showCreateForm = false; }}>Cancel</button>
            <button class="submit-btn" ?disabled=${this._submitting || !this._formName || !this._formType} @click=${this._handleCreate}>
              ${this._submitting ? 'Creating...' : 'Create Asset'}
            </button>
          </div>
        </div>
      </div>
    `;
  }

  private _renderAllocateModal() {
    return html`
      <div class="modal-overlay" @click=${(e: Event) => { if (e.target === e.currentTarget) this._showAllocateForm = null; }}>
        <div class="modal">
          <h2>Allocate Asset</h2>
          <div class="form-group">
            <label>Project</label>
            <select @change=${(e: Event) => { this._allocProjectId = (e.target as HTMLSelectElement).value; }}>
              <option value="">Select project...</option>
              ${this._projects.map((p) => html`<option value=${p.id}>${p.name}</option>`)}
            </select>
          </div>
          <div class="form-group">
            <label>Start Date</label>
            <input type="date" .value=${this._allocStartDate} @input=${(e: Event) => { this._allocStartDate = (e.target as HTMLInputElement).value; }} />
          </div>
          <div class="form-group">
            <label>End Date</label>
            <input type="date" .value=${this._allocEndDate} @input=${(e: Event) => { this._allocEndDate = (e.target as HTMLInputElement).value; }} />
          </div>
          <div class="form-actions">
            <button class="cancel-btn" @click=${() => { this._showAllocateForm = null; }}>Cancel</button>
            <button class="submit-btn" ?disabled=${this._submitting || !this._allocProjectId || !this._allocStartDate || !this._allocEndDate} @click=${this._handleAllocate}>
              ${this._submitting ? 'Allocating...' : 'Allocate'}
            </button>
          </div>
        </div>
      </div>
    `;
  }

  private async _handleCreate() {
    this._submitting = true;
    const orgID = currentOrg.get();
    try {
      const req: { name: string; asset_type: string; serial_number?: string } = {
        name: this._formName,
        asset_type: this._formType,
      };
      if (this._formSerial) {
        req.serial_number = this._formSerial;
      }
      await createAsset(orgID, req);
      this._showCreateForm = false;
      this._formName = '';
      this._formType = '';
      this._formSerial = '';
      this.showToast('Asset created', 'success');
      await this._loadAssets();
    } catch (err) {
      this.showToast(err instanceof ApiError ? `Failed (${err.status})` : 'Failed to create asset', 'error');
    } finally {
      this._submitting = false;
    }
  }

  private async _handleAllocate() {
    if (!this._showAllocateForm) return;
    this._submitting = true;
    const orgID = currentOrg.get();
    try {
      await allocateAsset(orgID, this._showAllocateForm, {
        project_id: this._allocProjectId,
        start_date: this._allocStartDate,
        end_date: this._allocEndDate,
      });
      this._showAllocateForm = null;
      this._allocProjectId = '';
      this._allocStartDate = '';
      this._allocEndDate = '';
      this.showToast('Asset allocated', 'success');
      await this._loadAssets();
    } catch (err) {
      this.showToast(err instanceof ApiError ? `Failed (${err.status})` : 'Allocation failed', 'error');
    } finally {
      this._submitting = false;
    }
  }

  private async _loadData() {
    this._loading = true;
    try {
      const [_, projectsRes] = await Promise.allSettled([
        this._loadAssets(),
        listProjects({ status: 'active' }),
      ]);
      if (projectsRes.status === 'fulfilled') {
        this._projects = projectsRes.value.projects;
      }
    } finally {
      this._loading = false;
    }
  }

  private async _loadAssets() {
    const orgID = currentOrg.get();
    if (!orgID) {
      this._error = 'No organization selected.';
      return;
    }
    try {
      const res = await listAssets(orgID);
      this._assets = res.assets;
    } catch (err) {
      this._error = err instanceof ApiError ? `Failed to load fleet (${err.status})` : 'Failed to load fleet data';
    }
  }
}
