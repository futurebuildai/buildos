import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, property, queryAssignedElements } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './../atoms/fb-icon.js';

/**
 * `fb-field` — label + hint + error wrapper for a single control (DSC §7.3).
 *
 * Slot a single `fb-*` control (input/select/password/secret/money). The field
 * owns the visible `<label>`, the optional hint, and the error message, and wires
 * the relationships onto the slotted control programmatically: it sets the
 * control's `describedby` (hint/error ids) and `invalid` properties so screen
 * readers announce them. Errors are text + icon, never color-only (WCAG 1.4.1).
 */
@customElement('fb-field')
export class FbField extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: block;
      }
      .label {
        display: block;
        margin-bottom: var(--fb-spacing-xs);
        font-size: var(--fb-text-label-lg);
        font-weight: 600;
        color: var(--fb-text-primary);
      }
      .req {
        color: var(--fb-safety-red-text);
        margin-inline-start: 2px;
      }
      .hint {
        margin-top: var(--fb-spacing-xs);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-text-secondary);
      }
      .error {
        display: flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        margin-top: var(--fb-spacing-xs);
        font-size: var(--fb-text-body-sm);
        color: var(--fb-safety-red-text);
      }
    `,
  ];

  @property({ type: String }) label = '';
  @property({ type: String }) hint?: string;
  @property({ type: String }) error?: string;
  @property({ type: Boolean }) required = false;
  /** Stable id base so hint/error ids are deterministic for aria wiring. */
  @property({ type: String }) fieldId = `fld-${Math.random().toString(36).slice(2, 8)}`;

  @queryAssignedElements({ flatten: true })
  private controls!: HTMLElement[];

  private get hintId(): string {
    return `${this.fieldId}-hint`;
  }
  private get errorId(): string {
    return `${this.fieldId}-error`;
  }

  /** Push label/describedby/invalid onto the slotted control after it's assigned. */
  private wireControl(): void {
    const control = this.controls[0] as
      | (HTMLElement & { label?: string; describedby?: string; invalid?: boolean })
      | undefined;
    if (!control) return;
    // The visible `<label for>` lives in this shadow root and cannot reach the
    // control's input across the shadow boundary, so we also push the label text
    // onto the control, which renders it as `aria-label` (an accessible name).
    if (this.label) control.label = this.label;
    const ids = [this.hint ? this.hintId : undefined, this.error ? this.errorId : undefined].filter(
      (v): v is string => !!v,
    );
    if (ids.length) control.describedby = ids.join(' ');
    else delete control.describedby;
    control.invalid = !!this.error;
  }

  override updated(): void {
    this.wireControl();
  }

  override render(): TemplateResult {
    // A single root element is intentional: a top-level `<slot>` interleaved with
    // sibling dynamic parts is mis-committed by some shadow-DOM implementations,
    // so the field's pieces live inside one wrapper.
    return html`
      <div class="root">
        ${this.label
          ? html`<label class="label" for=${this.fieldId}>
              ${this.label}${this.required
                ? html`<span class="req" aria-hidden="true">*</span>`
                : nothing}
            </label>`
          : nothing}
        <slot @slotchange=${this.wireControl}></slot>
        ${this.hint && !this.error
          ? html`<p class="hint" id=${this.hintId}>${this.hint}</p>`
          : nothing}
        ${this.error
          ? html`<p class="error" id=${this.errorId} role="alert">
              <fb-icon name="alert-circle" size="14"></fb-icon>${this.error}
            </p>`
          : nothing}
      </div>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-field': FbField;
  }
}
