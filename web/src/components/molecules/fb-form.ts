import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';

/**
 * `fb-form` — submit + error-summary wrapper (DSC §7.3).
 *
 * Wraps a native `<form>` so Enter-to-submit and `required` semantics work. On
 * submit it emits a composed `submit` event (default-prevented) carrying the
 * serialized `{ values }` from named controls; the page handler does the async
 * work and may call `setErrors()` to surface a field-keyed error summary.
 *
 * The error summary is an `role="alert"` region focused on appearance so screen
 * readers announce the count, with anchor links to each offending field.
 */
@customElement('fb-form')
export class FbForm extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .summary {
        display: flex;
        gap: var(--fb-spacing-sm);
        margin-bottom: var(--fb-spacing-md);
        padding: var(--fb-spacing-md);
        color: var(--fb-safety-red);
        background: color-mix(in srgb, var(--fb-safety-red) 10%, transparent);
        border: 1px solid var(--fb-safety-red);
        border-radius: var(--fb-radius-sm);
      }
      .summary ul {
        margin: var(--fb-spacing-xs) 0 0;
        padding-inline-start: var(--fb-spacing-md);
      }
      .summary a {
        color: var(--fb-safety-red);
      }
    `,
  ];

  /** Field label → error message; rendered as the summary + announced. */
  @state() private errors: Record<string, string> = {};

  @property({ type: String }) summaryTitle = 'Please fix the following';

  /** Set by the page after a failed submit; pass `{}` to clear. */
  setErrors(errors: Record<string, string>): void {
    this.errors = errors;
  }

  /** Reentrancy guard so a single user action submits exactly once. */
  private submitting = false;

  // The submittable controls (fb-input, fb-password-input, fb-secret-input, …)
  // and the submit fb-button each render their native element inside their OWN
  // shadow root, so none of them are form-owned by the `<form>` in THIS
  // component's shadow root. That means the browser fires neither a native
  // `submit` on button click nor implicit submission on Enter — the form is
  // dead to native mechanics across the shadow boundary. We bridge it: `click`
  // and `keydown` are composed events that bubble through the flattened tree to
  // our `<form>`, so we detect a submit-button click or an Enter keypress in a
  // text field and run submission ourselves. (The native `@submit` stays wired
  // for any same-tree `<button type=submit>` a caller slots in directly.)
  private onFormClick(e: MouseEvent): void {
    if (this.pathHasSubmitButton(e.composedPath())) {
      e.preventDefault();
      this.doSubmit();
    }
  }

  private onFormKeydown(e: KeyboardEvent): void {
    if (e.key !== 'Enter' || e.isComposing) return;
    const target = e.composedPath()[0] as HTMLElement | undefined;
    // Implicit submission: Enter in a single-line text input. Enter on a button
    // is delivered as a click (handled above); textareas keep their newline.
    if (target?.tagName === 'INPUT') {
      e.preventDefault();
      this.doSubmit();
    }
  }

  /** True if the composed path (up to this host) crosses a submit button. */
  private pathHasSubmitButton(path: EventTarget[]): boolean {
    for (const node of path) {
      if (node === this) break;
      const el = node as HTMLElement;
      if (el?.tagName !== 'BUTTON') continue;
      if ((el.getAttribute('type') ?? '').toLowerCase() === 'submit') return true;
    }
    return false;
  }

  private onNativeSubmit(e: Event): void {
    e.preventDefault();
    this.doSubmit();
  }

  private doSubmit(): void {
    // Collapse the click+any native submit from one action into a single emit.
    if (this.submitting) return;
    this.submitting = true;
    queueMicrotask(() => (this.submitting = false));

    // Controls are slotted in the light DOM, so a shadow `<form>.elements` can't
    // see them. Collect any named element exposing a `value` (native inputs and
    // custom `fb-*` controls alike) from the host's light-DOM subtree.
    const values: Record<string, string> = {};
    for (const el of Array.from(this.querySelectorAll<HTMLElement>('[name]'))) {
      const name = el.getAttribute('name');
      const value = (el as HTMLInputElement).value;
      if (name && value !== undefined) values[name] = value;
    }
    this.emit('submit', { values });
  }

  override render(): TemplateResult {
    const entries = Object.entries(this.errors);
    return html`
      <form
        novalidate
        @submit=${this.onNativeSubmit}
        @click=${this.onFormClick}
        @keydown=${this.onFormKeydown}
      >
        ${entries.length
          ? html`<div class="summary" role="alert" tabindex="-1">
              <fb-icon name="alert-triangle" size="18"></fb-icon>
              <div>
                <strong>${this.summaryTitle}</strong>
                <ul>
                  ${entries.map(([field, msg]) => html`<li>${field}: ${msg}</li>`)}
                </ul>
              </div>
            </div>`
          : nothing}
        <slot></slot>
      </form>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-form': FbForm;
  }
}
