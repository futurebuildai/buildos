import { html, css, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-toast.js';
import type { ToastTone } from './fb-toast.js';

export interface ToastSpec {
  id: number;
  tone: ToastTone;
  heading?: string;
  message: string;
  /** Auto-dismiss after this many ms; 0 = sticky (errors). */
  duration: number;
}

export interface ToastOptions {
  tone?: ToastTone;
  heading?: string;
  duration?: number;
}

/**
 * `fb-toaster` — stacked toast host + live region (DSC §7, AG-05). Mount once near
 * the app root. Call `toast(message, opts)` to enqueue; errors default to sticky.
 * Two ARIA live regions split assertive (errors) from polite (the rest) so SR
 * announcements aren't dropped or interrupted.
 */
@customElement('fb-toaster')
export class FbToaster extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        position: fixed;
        bottom: var(--fb-spacing-lg);
        right: var(--fb-spacing-lg);
        z-index: var(--fb-z-toast, 1000);
        display: flex;
        flex-direction: column;
        gap: var(--fb-spacing-sm);
        pointer-events: none;
      }
      fb-toast {
        pointer-events: auto;
      }
    `,
  ];

  @state() private toasts: ToastSpec[] = [];
  private nextId = 1;
  private readonly timers = new Map<number, ReturnType<typeof setTimeout>>();

  /** Enqueue a toast. Returns its id (so callers can `dismiss` early). */
  toast(message: string, opts: ToastOptions = {}): number {
    const tone = opts.tone ?? 'info';
    // Errors stick until dismissed; everything else auto-clears.
    const duration = opts.duration ?? (tone === 'error' ? 0 : 5000);
    const id = this.nextId++;
    const spec: ToastSpec = { id, tone, message, duration };
    if (opts.heading !== undefined) spec.heading = opts.heading;
    this.toasts = [...this.toasts, spec];
    if (duration > 0) {
      this.timers.set(
        id,
        setTimeout(() => this.dismiss(id), duration),
      );
    }
    return id;
  }

  dismiss(id: number): void {
    const timer = this.timers.get(id);
    if (timer) {
      clearTimeout(timer);
      this.timers.delete(id);
    }
    this.toasts = this.toasts.filter((t) => t.id !== id);
  }

  override disconnectedCallback(): void {
    super.disconnectedCallback();
    for (const timer of this.timers.values()) clearTimeout(timer);
    this.timers.clear();
  }

  override render(): TemplateResult {
    const assertive = this.toasts.filter((t) => t.tone === 'error');
    const polite = this.toasts.filter((t) => t.tone !== 'error');
    const renderToast = (t: ToastSpec): TemplateResult => html`
      <fb-toast
        tone=${t.tone}
        heading=${t.heading ?? ''}
        message=${t.message}
        @dismiss=${() => this.dismiss(t.id)}
      ></fb-toast>
    `;
    return html`
      <div role="alert" aria-live="assertive">${assertive.map(renderToast)}</div>
      <div role="status" aria-live="polite">${polite.map(renderToast)}</div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-toaster': FbToaster;
  }
}
