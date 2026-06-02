import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-password-input.js';
import '../atoms/fb-button.js';
import '../molecules/fb-form.js';
import '../molecules/fb-field.js';
import { confirmPasswordReset } from '../../api/endpoints/auth.js';
import { ApiError, ErrorCode, userMessageForCode } from '../../api/errors.js';
import { authCardStyles } from './auth-styles.js';

/**
 * `fb-reset-password-page` — Screen P2 (UX_AUTH_ONBOARDING §5). Confirms a reset
 * by setting a new password against the `?token=` from the email link.
 *
 * Token hygiene (§5 / §8): the reset token must not leak. We strip it from the
 * address bar (history `replaceState`) as soon as it's captured so it never
 * survives in browser history, bookmarks, or a copied URL, and the email link
 * itself carries `referrerpolicy="no-referrer"`. An invalid/expired token maps
 * to uniform copy and offers a fresh request.
 */
@customElement('fb-reset-password-page')
export class FbResetPasswordPage extends FBElement {
  static override styles = [FBElement.styles, authCardStyles, css``];

  @state() private busy = false;
  @state() private error: string | null = null;
  @state() private done = false;
  @state() private tokenMissing = false;

  private token = '';

  override connectedCallback(): void {
    super.connectedCallback();
    const params = new URLSearchParams(window.location.search);
    this.token = params.get('token') ?? '';
    this.tokenMissing = this.token.length === 0;
    // Scrub the token from the URL so it never lingers in history/bookmarks.
    if (this.token && typeof window.history?.replaceState === 'function') {
      window.history.replaceState({}, '', window.location.pathname);
    }
  }

  private async onSubmit(e: CustomEvent<{ values: Record<string, string> }>): Promise<void> {
    const password = e.detail.values.password ?? '';
    const confirm = e.detail.values.confirm ?? '';
    if (!password) {
      this.error = 'Enter a new password.';
      return;
    }
    if (password !== confirm) {
      this.error = 'Those passwords don’t match.';
      return;
    }
    this.busy = true;
    this.error = null;
    try {
      await confirmPasswordReset(this.token, password);
      this.done = true;
    } catch (err) {
      this.error =
        err instanceof ApiError && err.code !== ErrorCode.INVALID_RESET_TOKEN
          ? userMessageForCode(err.code)
          : 'That reset link is invalid or expired.';
    } finally {
      this.busy = false;
    }
  }

  override render(): TemplateResult {
    if (this.done) {
      return html`
        <section class="auth-card glass-card" aria-labelledby="rp-title">
          <header class="auth-head">
            <fb-icon name="check-circle" size="28"></fb-icon>
            <h1 id="rp-title" class="auth-title">Password updated</h1>
            <p class="auth-sub">You can sign in with your new password.</p>
          </header>
          <div class="auth-links">
            <a href="/login">Go to sign in</a>
          </div>
        </section>
      `;
    }

    if (this.tokenMissing) {
      return html`
        <section class="auth-card glass-card" aria-labelledby="rp-title">
          <header class="auth-head">
            <fb-icon name="alert-triangle" size="28"></fb-icon>
            <h1 id="rp-title" class="auth-title">Reset link incomplete</h1>
            <p class="auth-sub">This link is missing its token. Request a new one.</p>
          </header>
          <div class="auth-links">
            <a href="/forgot-password">Request a new link</a>
          </div>
        </section>
      `;
    }

    return html`
      <section class="auth-card glass-card" aria-labelledby="rp-title">
        <header class="auth-head">
          <fb-icon name="lock" size="28"></fb-icon>
          <h1 id="rp-title" class="auth-title">Set a new password</h1>
        </header>

        ${this.error
          ? html`<p class="auth-error" role="alert">
              <fb-icon name="alert-circle" size="16"></fb-icon>${this.error}
              <a href="/forgot-password" style="margin-inline-start:auto">Request a new link</a>
            </p>`
          : nothing}

        <fb-form @submit=${this.onSubmit}>
          <div class="auth-fields">
            <fb-field label="New password" fieldId="rp-password">
              <fb-password-input
                name="password"
                autocomplete="new-password"
                required
              ></fb-password-input>
            </fb-field>
            <fb-field label="Confirm new password" fieldId="rp-confirm">
              <fb-password-input
                name="confirm"
                autocomplete="new-password"
                required
              ></fb-password-input>
            </fb-field>
          </div>
          <fb-button type="submit" variant="primary" full ?loading=${this.busy}>
            Update password
          </fb-button>
        </fb-form>
      </section>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-reset-password-page': FbResetPasswordPage;
  }
}
