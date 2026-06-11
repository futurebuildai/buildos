import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state, query } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';
import { submitFeedback } from '../../api/endpoints/feedback.js';
import { ApiError, ErrorCode, userMessageForCode } from '../../api/errors.js';
import { currentRoute } from '../../router.js';
import { currentRole } from '../../state/authStore.js';
import type { FeedbackCategory, FeedbackContext } from '../../types/models.js';

const FOCUSABLE =
  'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';

const CATEGORIES: ReadonlyArray<{ value: FeedbackCategory; label: string }> = [
  { value: 'bug', label: 'Bug' },
  { value: 'idea', label: 'Idea' },
  { value: 'friction', label: 'Friction' },
  { value: 'other', label: 'Other' },
];

/**
 * `fb-feedback-widget` — Phase 0b in-app feedback. A floating trigger button
 * (fixed bottom-right, below toasts on the z-scale) that opens a lightweight
 * anchored panel: category select + message textarea + send. On submit it
 * auto-captures context (route, role, app_version, user_agent, viewport) and
 * POSTs /api/v1/feedback; success shows a brief thank-you then closes; failure
 * shows an inline error and KEEPS the user's text.
 *
 * A11y: the panel is a non-blocking `role="dialog"` (`aria-modal="false"`)
 * with an fb-modal-style Tab trap; Esc closes and focus restores to the
 * trigger (an inner native button — host `.focus()` would no-op, the Phase 3c
 * lesson). All controls are native with real `<label for>`; validation wires
 * `aria-invalid` + `aria-describedby`; outcomes are announced via a polite
 * live region. Emits `feedback-sent` on success.
 */
@customElement('fb-feedback-widget')
export class FbFeedbackWidget extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        position: fixed;
        bottom: var(--fb-spacing-lg);
        right: var(--fb-spacing-lg);
        /* Below modals (400) and toasts (500): chrome, not an interruption. */
        z-index: var(--fb-z-dropdown);
      }
      .root {
        position: relative;
        display: flex;
        justify-content: flex-end;
      }
      .trigger {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        /* ≥44px touch-target floor (DSC §9). */
        width: 48px;
        height: 48px;
        border: 1px solid transparent;
        border-radius: var(--fb-radius-full);
        background: var(--fb-gable-green);
        color: var(--fb-deep-space);
        cursor: pointer;
        box-shadow: var(--md-sys-elevation-2);
      }
      .panel {
        position: absolute;
        bottom: calc(48px + var(--fb-spacing-sm));
        right: 0;
        width: min(320px, calc(100vw - 2 * var(--fb-spacing-lg)));
        /* Short viewports (landscape phones): cap to the visible viewport so
           the top of the form is never clipped off-screen. The subtraction is
           the panel's bottom offset (host inset + 48px trigger + anchor gap)
           plus a matching top margin; overflow scrolls inside the panel, so
           the Tab trap's focusables stay reachable. */
        max-height: calc(100dvh - 48px - var(--fb-spacing-sm) - 2 * var(--fb-spacing-lg));
        overflow-y: auto;
        display: flex;
        flex-direction: column;
        background: var(--fb-surface-1);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-lg);
        box-shadow: var(--md-sys-elevation-3);
      }
      .head {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-sm) var(--fb-spacing-md);
        border-bottom: 1px solid var(--fb-border);
      }
      .title {
        font-size: var(--fb-text-title-md);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .close {
        display: inline-flex;
        padding: 4px;
        color: var(--fb-text-secondary);
        background: transparent;
        border: none;
        border-radius: var(--fb-radius-xs);
        cursor: pointer;
      }
      .close:hover {
        color: var(--fb-text-primary);
      }
      form {
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-xs);
        padding: var(--fb-spacing-md);
      }
      .label {
        font-size: var(--fb-text-label-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      select,
      textarea {
        width: 100%;
        font-family: var(--fb-font-sans);
        font-size: var(--fb-text-body-md);
        color: var(--fb-text-primary);
        background: var(--fb-surface-2);
        border: 1px solid var(--md-sys-color-outline);
        border-radius: var(--fb-radius-sm);
        padding: var(--fb-spacing-xs) var(--fb-spacing-sm);
      }
      select {
        min-height: var(--fb-density-control);
        cursor: pointer;
      }
      textarea {
        resize: vertical;
        min-height: 88px;
      }
      select:disabled,
      textarea:disabled {
        opacity: 0.5;
        cursor: not-allowed;
      }
      select[aria-invalid='true'],
      textarea[aria-invalid='true'] {
        border-color: var(--fb-safety-red-text);
      }
      .error {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin: 0;
        font-size: var(--fb-text-body-sm);
        color: var(--fb-safety-red-text);
      }
      .actions {
        display: flex;
        justify-content: flex-end;
        margin-top: var(--fb-spacing-xs);
      }
      .send {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        gap: var(--fb-spacing-sm);
        min-height: var(--fb-density-control);
        padding: 0 var(--fb-spacing-md);
        font-family: var(--fb-font-sans);
        font-size: var(--fb-text-body-md);
        font-weight: 600;
        border: 1px solid transparent;
        border-radius: var(--fb-radius-sm);
        background: var(--fb-gable-green);
        color: var(--fb-deep-space);
        cursor: pointer;
      }
      .send:disabled {
        cursor: not-allowed;
        opacity: 0.5;
      }
      .success {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-sm);
        padding: var(--fb-spacing-md);
        color: var(--fb-text-primary);
        font-size: var(--fb-text-body-md);
      }
      .success fb-icon {
        color: var(--fb-gable-green);
        flex-shrink: 0;
      }
    `,
  ];

  @state() private open = false;
  @state() private category: FeedbackCategory = 'bug';
  @state() private message = '';
  @state() private phase: 'idle' | 'submitting' | 'success' = 'idle';
  /** Field-level validation copy (empty message / server VALIDATION_ERROR). */
  @state() private messageError: string | null = null;
  /** Non-field submit failure copy (rate limit, network, 5xx). */
  @state() private submitError: string | null = null;
  /** Polite live-region text (success/error announcements). */
  @state() private announcement = '';

  @query('.trigger') private triggerEl?: HTMLButtonElement;
  @query('.panel') private panelEl?: HTMLElement;
  @query('textarea') private textareaEl?: HTMLTextAreaElement;

  private successTimer: ReturnType<typeof setTimeout> | null = null;

  /**
   * Submission epoch — in-flight-request guard. Each submit captures the
   * current epoch; closing the panel (or starting a newer submit) bumps it.
   * When the POST settles, a stale epoch (or a panel closed mid-flight) means
   * the resolution is discarded entirely: no state change, no auto-close
   * timer, no focus moves. Closing while submitting is allowed and simply
   * abandons the in-flight request's UI effects.
   */
  private submitEpoch = 0;

  override connectedCallback(): void {
    super.connectedCallback();
    this.addEventListener('keydown', this.onKeydown);
  }

  override disconnectedCallback(): void {
    super.disconnectedCallback();
    this.removeEventListener('keydown', this.onKeydown);
    if (this.successTimer) {
      clearTimeout(this.successTimer);
      this.successTimer = null;
    }
  }

  private toggle(): void {
    if (this.open) this.closePanel();
    else this.openPanel();
  }

  private openPanel(): void {
    this.open = true;
    this.announcement = '';
    // Focus the first control; the host itself is not focusable (Phase 3c).
    void this.updateComplete.then(() => this.focusFirst());
  }

  /** Closes and restores focus to the trigger. Successful sends reset the form. */
  private closePanel(): void {
    // Invalidate any in-flight submission: its late resolution must not
    // reopen success UI, start the auto-close timer, or steal focus.
    this.submitEpoch++;
    if (this.successTimer) {
      clearTimeout(this.successTimer);
      this.successTimer = null;
    }
    if (this.phase === 'success') {
      this.message = '';
      this.category = 'bug';
    }
    this.open = false;
    this.phase = 'idle';
    this.messageError = null;
    this.submitError = null;
    void this.updateComplete.then(() => this.triggerEl?.focus());
  }

  private focusFirst(): void {
    const [first] = this.panelFocusables();
    first?.focus();
  }

  private panelFocusables(): HTMLElement[] {
    if (!this.panelEl) return [];
    return Array.from(this.panelEl.querySelectorAll<HTMLElement>(FOCUSABLE));
  }

  /** Esc closes; Tab cycles inside the panel (fb-modal's trap, panel-scoped). */
  private readonly onKeydown = (e: KeyboardEvent): void => {
    if (!this.open) return;
    if (e.key === 'Escape') {
      e.stopPropagation();
      this.closePanel();
      return;
    }
    if (e.key !== 'Tab') return;
    const items = this.panelFocusables();
    if (items.length === 0) return;
    const firstItem = items[0]!;
    const lastItem = items[items.length - 1]!;
    const active = this.shadowRoot?.activeElement;
    if (e.shiftKey && active === firstItem) {
      e.preventDefault();
      lastItem.focus();
    } else if (!e.shiftKey && active === lastItem) {
      e.preventDefault();
      firstItem.focus();
    }
  };

  /** Auto-captured submission context — all strings, kept short (≤4096B serialized). */
  private captureContext(): FeedbackContext {
    return {
      route: currentRoute.get()?.path ?? '',
      role: currentRole.get() ?? '',
      app_version: (import.meta.env.VITE_APP_VERSION as string | undefined) ?? 'dev',
      user_agent: navigator.userAgent,
      viewport: `${window.innerWidth}x${window.innerHeight}`,
    };
  }

  private async onSubmit(e: Event): Promise<void> {
    e.preventDefault();
    if (this.phase === 'submitting') return;
    const trimmed = this.message.trim();
    if (!trimmed) {
      this.messageError = 'Enter a message before sending.';
      this.announcement = this.messageError;
      void this.updateComplete.then(() => this.textareaEl?.focus());
      return;
    }
    this.messageError = null;
    this.submitError = null;
    this.phase = 'submitting';
    const epoch = ++this.submitEpoch;
    try {
      const fb = await submitFeedback({
        category: this.category,
        message: trimmed,
        context: this.captureContext(),
      });
      // Stale (closed mid-flight / superseded): discard the resolution.
      if (epoch !== this.submitEpoch || !this.open) return;
      this.phase = 'success';
      this.announcement = 'Thanks — your feedback was sent.';
      this.emit('feedback-sent', { id: fb.id });
      // Keep the trap valid: the form (and the focused Send button) just left
      // the DOM, so move focus to the remaining control (the close button).
      void this.updateComplete.then(() => this.focusFirst());
      this.successTimer = setTimeout(() => this.closePanel(), 2500);
    } catch (err) {
      // Stale (closed mid-flight / superseded): discard the rejection too.
      if (epoch !== this.submitEpoch || !this.open) return;
      // NEVER wipe the user's text on failure.
      this.phase = 'idle';
      if (err instanceof ApiError && err.code === ErrorCode.VALIDATION_ERROR) {
        this.messageError = err.details[0]?.reason ?? err.message;
        this.announcement = this.messageError;
      } else {
        this.submitError =
          err instanceof ApiError
            ? userMessageForCode(err.code)
            : userMessageForCode(ErrorCode.UNKNOWN);
        this.announcement = this.submitError;
      }
    }
  }

  private onCategoryChange(e: Event): void {
    this.category = (e.target as HTMLSelectElement).value as FeedbackCategory;
  }

  private onMessageInput(e: Event): void {
    this.message = (e.target as HTMLTextAreaElement).value;
    if (this.messageError) this.messageError = null;
  }

  override render(): TemplateResult {
    return html`
      <div class="root">
        ${this.open ? this.renderPanel() : nothing}
        <button
          class="trigger btn-primary"
          type="button"
          aria-label="Send feedback"
          aria-haspopup="dialog"
          aria-expanded=${this.open ? 'true' : 'false'}
          @click=${this.toggle}
        >
          <fb-icon name="message-circle" size="22"></fb-icon>
        </button>
        <div class="visually-hidden" role="status" aria-live="polite">${this.announcement}</div>
      </div>
    `;
  }

  private renderPanel(): TemplateResult {
    return html`
      <div class="panel" role="dialog" aria-modal="false" aria-labelledby="fb-feedback-title">
        <div class="head">
          <span class="title" id="fb-feedback-title">Send feedback</span>
          <button
            class="close"
            type="button"
            aria-label="Close feedback panel"
            @click=${this.closePanel}
          >
            <fb-icon name="x" size="16"></fb-icon>
          </button>
        </div>
        ${this.phase === 'success' ? this.renderSuccess() : this.renderForm()}
      </div>
    `;
  }

  private renderForm(): TemplateResult {
    const submitting = this.phase === 'submitting';
    return html`
      <form novalidate @submit=${this.onSubmit}>
        <label class="label" for="fb-feedback-category">Category</label>
        <select
          id="fb-feedback-category"
          .value=${this.category}
          ?disabled=${submitting}
          @change=${this.onCategoryChange}
        >
          ${CATEGORIES.map(
            (c) =>
              html`<option value=${c.value} ?selected=${c.value === this.category}>
                ${c.label}
              </option>`,
          )}
        </select>
        <label class="label" for="fb-feedback-message">Message</label>
        <textarea
          id="fb-feedback-message"
          rows="4"
          maxlength="4000"
          placeholder="What's working? What's getting in the way?"
          .value=${this.message}
          ?disabled=${submitting}
          aria-invalid=${this.messageError ? 'true' : nothing}
          aria-describedby=${this.messageError ? 'fb-feedback-message-error' : nothing}
          @input=${this.onMessageInput}
        ></textarea>
        ${this.messageError
          ? html`<p class="error" id="fb-feedback-message-error">
              <fb-icon name="alert-circle" size="14"></fb-icon>${this.messageError}
            </p>`
          : nothing}
        ${this.submitError
          ? html`<p class="error">
              <fb-icon name="alert-circle" size="14"></fb-icon>${this.submitError}
            </p>`
          : nothing}
        <div class="actions">
          <button
            class="send btn-primary"
            type="submit"
            ?disabled=${submitting}
            aria-busy=${submitting ? 'true' : nothing}
          >
            ${submitting ? 'Sending…' : 'Send feedback'}
          </button>
        </div>
      </form>
    `;
  }

  private renderSuccess(): TemplateResult {
    return html`
      <div class="success">
        <fb-icon name="check-circle" size="20"></fb-icon>
        <p>Thanks — your feedback was sent.</p>
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-feedback-widget': FbFeedbackWidget;
  }
}
