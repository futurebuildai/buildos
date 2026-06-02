import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import './../atoms/fb-button.js';
import { userMessageForCode, type ErrorCode } from '../../api/errors.js';
import type { IconName } from '../atoms/icons.js';

export type StateMode = 'loading' | 'empty' | 'error' | 'gated';
export type SkeletonShape = 'table' | 'card' | 'text';

/**
 * `fb-state` — the loading / skeleton / empty / error / feature-gated family
 * (DSC §7.10–§7.13). One component, four modes:
 *
 * - `loading` — shape-matched skeletons (table rows / card / text lines). Shimmer
 *   is suppressed under `prefers-reduced-motion` (handled in FBElement styles).
 * - `empty`   — centered icon + headline + body + optional primary action (slot).
 * - `error`   — icon + friendly copy mapped from `error.code` (DSC §11.1). Raw 5xx
 *   `message` is never shown; instead a generic line + `request_id` for support.
 *   Emits `retry` when the retry button is pressed.
 * - `gated`   — the pivot-critical "this feature requires a key" state (DSC §11.2).
 *   Names the capability in user terms; the configure link shows ONLY to owners
 *   (`can-configure`), non-owners get uniform copy with no link/oracle.
 */
@customElement('fb-state')
export class FbState extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .center {
        display: flex;
        flex-direction: column;
        align-items: center;
        justify-content: center;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-2xl) var(--fb-spacing-lg);
        text-align: center;
        color: var(--fb-text-secondary);
      }
      .glyph {
        color: var(--fb-text-muted);
      }
      .glyph.error {
        color: var(--fb-safety-red);
      }
      .glyph.gated {
        color: var(--fb-key-missing);
      }
      .headline {
        font-size: var(--fb-text-title-md);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .body {
        max-width: 42ch;
        font-size: var(--fb-text-body-md);
        color: var(--fb-text-secondary);
      }
      .ref {
        font-family: var(--fb-font-mono);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-muted);
      }
      .actions {
        margin-top: var(--fb-spacing-sm);
        display: flex;
        gap: var(--fb-spacing-sm);
      }
      /* Skeletons */
      .sk-row {
        display: flex;
        gap: var(--fb-spacing-md);
        padding: var(--fb-spacing-sm) 0;
      }
      .sk-row .skeleton-text {
        margin: 0;
      }
      .sk-card {
        padding: var(--fb-spacing-md);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-md);
      }
    `,
  ];

  @property({ type: String, reflect: true }) mode: StateMode = 'loading';
  @property({ type: String }) skeleton: SkeletonShape = 'text';
  /** Number of skeleton rows/lines to render while loading. */
  @property({ type: Number }) rows = 3;
  @property({ type: String }) icon?: IconName;
  @property({ type: String }) heading?: string;
  @property({ type: String }) message?: string;
  /** Backend machine error code — mapped to friendly copy in `error` mode. */
  @property({ type: String, attribute: 'error-code' }) errorCode?: ErrorCode | string;
  /** request_id surfaced for support on 5xx (DSC §11.1). */
  @property({ type: String, attribute: 'request-id' }) requestId?: string;
  /** Show a Retry affordance in `error` mode. */
  @property({ type: Boolean }) retryable = false;
  /** Owner-only: render the "Go to Integrations →" link in `gated` mode. */
  @property({ type: Boolean, attribute: 'can-configure' }) canConfigure = false;

  private onRetry(): void {
    this.emit('retry');
  }

  private onConfigure(): void {
    this.emit('configure');
  }

  private renderSkeleton(): TemplateResult {
    if (this.skeleton === 'card') {
      return html`<div class="sk-card" aria-hidden="true">
        <span class="skeleton skeleton-text" style="width:60%"></span>
        <span class="skeleton skeleton-box"></span>
      </div>`;
    }
    if (this.skeleton === 'table') {
      const widths = ['20%', '30%', '25%', '15%'];
      return html`<div class="sk-table">
        ${Array.from({ length: this.rows }).map(
          () =>
            html`<div class="sk-row" aria-hidden="true">
              ${widths.map(
                (w) => html`<span class="skeleton skeleton-text" style="width:${w}"></span>`,
              )}
            </div>`,
        )}
      </div>`;
    }
    return html`<div class="sk-lines">
      ${Array.from({ length: this.rows }).map(
        (_, i) =>
          html`<span
            class="skeleton skeleton-text"
            aria-hidden="true"
            style="width:${i === this.rows - 1 ? '70%' : '100%'}"
          ></span>`,
      )}
    </div>`;
  }

  override render(): TemplateResult {
    if (this.mode === 'loading') {
      // Busy region: SRs hear a single status, sighted users see shaped skeletons.
      return html`<div role="status" aria-live="polite" aria-busy="true">
        <span class="visually-hidden">Loading…</span>
        ${this.renderSkeleton()}
      </div>`;
    }

    if (this.mode === 'error') {
      const copy = this.message ?? userMessageForCode(this.errorCode ?? '');
      return html`<div class="center" role="alert">
        <fb-icon class="glyph error" name=${this.icon ?? 'alert-circle'} size="40"></fb-icon>
        <div class="headline">${this.heading ?? 'Something went wrong'}</div>
        <p class="body">${copy}</p>
        ${this.requestId ? html`<p class="ref">Reference: ${this.requestId}</p>` : nothing}
        ${this.retryable
          ? html`<div class="actions">
              <fb-button variant="secondary" icon="refresh" @click=${this.onRetry}>Retry</fb-button>
            </div>`
          : nothing}
      </div>`;
    }

    if (this.mode === 'gated') {
      return html`<div class="center">
        <fb-icon class="glyph gated" name=${this.icon ?? 'key'} size="40"></fb-icon>
        <div class="headline">${this.heading ?? 'This feature is turned off'}</div>
        <p class="body">${this.message ?? 'Add the required API key to enable this feature.'}</p>
        ${this.canConfigure
          ? html`<div class="actions">
              <fb-button variant="primary" @click=${this.onConfigure}
                >Go to Integrations →</fb-button
              >
            </div>`
          : html`<p class="body">Ask your account owner to add an API key.</p>`}
      </div>`;
    }

    // empty
    return html`<div class="center">
      <fb-icon class="glyph" name=${this.icon ?? 'inbox'} size="40"></fb-icon>
      <div class="headline">${this.heading ?? 'Nothing here yet'}</div>
      ${this.message ? html`<p class="body">${this.message}</p>` : nothing}
      <div class="actions"><slot name="action"></slot></div>
    </div>`;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-state': FbState;
  }
}
