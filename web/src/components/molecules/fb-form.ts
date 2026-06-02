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

  private onSubmit(e: Event): void {
    e.preventDefault();
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
      <form novalidate @submit=${this.onSubmit}>
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
