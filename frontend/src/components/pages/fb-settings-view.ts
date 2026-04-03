import { html, css } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBBaseElement } from '../base/fb-element.js';
import { currentOrg, currentCurrency } from '../../state/store.js';

@customElement('fb-settings-view')
export class FBSettingsView extends FBBaseElement {
  @state() private _orgId = '';
  @state() private _currency: 'USD' | 'CAD' | 'ALL' = 'ALL';

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

      .settings-grid {
        display: grid;
        grid-template-columns: repeat(2, 1fr);
        gap: var(--fb-space-6);
      }

      @media (max-width: 768px) { .settings-grid { grid-template-columns: 1fr; } }

      .settings-section {
        background: var(--fb-glass-bg);
        backdrop-filter: var(--fb-glass-blur);
        border: var(--fb-glass-border);
        border-radius: var(--fb-radius-lg);
        padding: var(--fb-space-6);
      }

      .section-title {
        font-size: var(--fb-text-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
        margin-bottom: var(--fb-space-4);
        padding-bottom: var(--fb-space-3);
        border-bottom: 1px solid var(--fb-border);
      }

      .setting-row {
        display: flex;
        justify-content: space-between;
        align-items: center;
        padding: var(--fb-space-3) 0;
        border-bottom: 1px solid var(--fb-border);
      }

      .setting-row:last-child { border-bottom: none; }

      .setting-label {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-secondary);
      }

      .setting-value {
        font-size: var(--fb-text-sm);
        color: var(--fb-text-primary);
        font-family: var(--fb-font-mono);
      }

      .setting-select {
        background: var(--fb-surface);
        border: 1px solid var(--fb-border);
        border-radius: var(--fb-radius-md);
        padding: var(--fb-space-1) var(--fb-space-3);
        color: var(--fb-text-primary);
        font-size: var(--fb-text-sm);
        font-family: var(--fb-font-body);
        cursor: pointer;
      }

      .setting-select:focus { outline: none; border-color: var(--fb-gable-green); }
      .setting-select option { background: var(--fb-surface); }

      .info-block {
        margin-top: var(--fb-space-4);
        padding: var(--fb-space-4);
        background: var(--fb-surface);
        border-radius: var(--fb-radius-md);
        font-size: var(--fb-text-xs);
        color: var(--fb-text-muted);
        line-height: 1.5;
      }

      .version-tag {
        display: inline-block;
        padding: 2px 8px;
        border-radius: var(--fb-radius-sm);
        font-size: var(--fb-text-xs);
        font-weight: 600;
        background: rgba(0, 255, 163, 0.1);
        color: var(--fb-gable-green);
        font-family: var(--fb-font-mono);
      }
    `,
  ];

  connectedCallback() {
    super.connectedCallback();
    this._orgId = currentOrg.get();
    this._currency = currentCurrency.get();
  }

  render() {
    return html`
      <div class="page-header">
        <h1>Settings</h1>
        <p>Configure your FutureBuild OS preferences.</p>
      </div>

      <div class="settings-grid">
        <div class="settings-section">
          <h2 class="section-title">Organization</h2>
          <div class="setting-row">
            <span class="setting-label">Organization ID</span>
            <span class="setting-value">${this._orgId || '\u2014'}</span>
          </div>
          <div class="setting-row">
            <span class="setting-label">Identity Provider</span>
            <span class="setting-value">FB-Brain OIDC</span>
          </div>
          <div class="setting-row">
            <span class="setting-label">Authentication</span>
            <span class="setting-value">RS256 JWT (JWKS)</span>
          </div>
        </div>

        <div class="settings-section">
          <h2 class="section-title">Display</h2>
          <div class="setting-row">
            <span class="setting-label">Default Currency</span>
            <select
              class="setting-select"
              .value=${this._currency}
              @change=${this._onCurrencyChange}
            >
              <option value="ALL">All Currencies</option>
              <option value="USD">USD ($)</option>
              <option value="CAD">CAD (CA$)</option>
            </select>
          </div>
          <div class="setting-row">
            <span class="setting-label">Theme</span>
            <span class="setting-value">GableLBM Industrial Dark</span>
          </div>
          <div class="setting-row">
            <span class="setting-label">Numerical Font</span>
            <span class="setting-value">JetBrains Mono</span>
          </div>
        </div>

        <div class="settings-section">
          <h2 class="section-title">API Configuration</h2>
          <div class="setting-row">
            <span class="setting-label">Base URL</span>
            <span class="setting-value">/api/v1</span>
          </div>
          <div class="setting-row">
            <span class="setting-label">API Version</span>
            <span class="version-tag">v1</span>
          </div>
          <div class="setting-row">
            <span class="setting-label">Brain JWKS Endpoint</span>
            <span class="setting-value">/.well-known/jwks.json</span>
          </div>
          <div class="info-block">
            FutureBuild OS validates JWTs issued by FB-Brain via the JWKS endpoint.
            All API requests require a valid Bearer token. Rate limits apply per plan tier.
          </div>
        </div>

        <div class="settings-section">
          <h2 class="section-title">System Info</h2>
          <div class="setting-row">
            <span class="setting-label">Version</span>
            <span class="version-tag">0.1.0</span>
          </div>
          <div class="setting-row">
            <span class="setting-label">Frontend</span>
            <span class="setting-value">Lit 3.x + Vite</span>
          </div>
          <div class="setting-row">
            <span class="setting-label">Backend</span>
            <span class="setting-value">Go 1.24 + Chi</span>
          </div>
          <div class="setting-row">
            <span class="setting-label">Database</span>
            <span class="setting-value">PostgreSQL 16+</span>
          </div>
          <div class="setting-row">
            <span class="setting-label">Physics Engine</span>
            <span class="setting-value">CPM-res1.0</span>
          </div>
        </div>
      </div>
    `;
  }

  private _onCurrencyChange(e: Event) {
    const value = (e.target as HTMLSelectElement).value as 'USD' | 'CAD' | 'ALL';
    this._currency = value;
    currentCurrency.set(value);
    this.showToast(`Default currency set to ${value}`, 'info');
  }
}
