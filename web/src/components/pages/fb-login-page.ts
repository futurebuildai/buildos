import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-input.js';
import '../atoms/fb-password-input.js';
import '../atoms/fb-button.js';
import '../molecules/fb-form.js';
import '../molecules/fb-field.js';
import { login, currentRole } from '../../state/authStore.js';
import { navigate, landingPathForRole } from '../../router.js';
import { ApiError, ErrorCode, userMessageForCode } from '../../api/errors.js';
import { authCardStyles } from './auth-styles.js';

/**
 * `fb-login-page` — Screen L1 (UX_AUTH_ONBOARDING §2). Email + password sign-in.
 *
 * Error copy is security-uniform: any bad credential yields one message
 * ("Email or password is incorrect.") so the form never reveals which field was
 * wrong (§2 / DSC §11.4). A `SETUP_INCOMPLETE` response routes the admin into
 * the wizard instead of surfacing an error. On success the user lands on the
 * role-appropriate home (`landingPathForRole`).
 */
@customElement('fb-login-page')
export class FbLoginPage extends FBElement {
  static override styles = [FBElement.styles, authCardStyles, css``];

  @state() private busy = false;
  @state() private error: string | null = null;

  private async onSubmit(e: CustomEvent<{ values: Record<string, string> }>): Promise<void> {
    const email = (e.detail.values.email ?? '').trim();
    const password = e.detail.values.password ?? '';
    if (!email || !password) {
      this.error = 'Enter your email and password.';
      return;
    }
    this.busy = true;
    this.error = null;
    try {
      await login(email, password);
      navigate(landingPathForRole(currentRole.get()), { replace: true });
    } catch (err) {
      if (err instanceof ApiError && err.code === ErrorCode.SETUP_INCOMPLETE) {
        navigate('/setup', { replace: true });
        return;
      }
      // Uniform credential failure; only fall back to generic copy for non-auth errors.
      this.error =
        err instanceof ApiError && err.code !== ErrorCode.INVALID_CREDENTIALS
          ? userMessageForCode(err.code)
          : 'Email or password is incorrect.';
    } finally {
      this.busy = false;
    }
  }

  override render(): TemplateResult {
    return html`
      <section class="auth-card glass-card" aria-labelledby="login-title">
        <header class="auth-head">
          <fb-icon name="hexagon" size="28"></fb-icon>
          <h1 id="login-title" class="auth-title">Sign in to BuildOS</h1>
          <p class="auth-sub">Your construction system of execution.</p>
        </header>

        ${this.error
          ? html`<p class="auth-error" role="alert">
              <fb-icon name="alert-circle" size="16"></fb-icon>${this.error}
            </p>`
          : nothing}

        <fb-form @submit=${this.onSubmit}>
          <div class="auth-fields">
            <fb-field label="Email" fieldId="login-email">
              <fb-input
                name="email"
                type="email"
                autocomplete="username"
                inputmode="email"
                required
              ></fb-input>
            </fb-field>
            <fb-field label="Password" fieldId="login-password">
              <fb-password-input
                name="password"
                autocomplete="current-password"
                required
              ></fb-password-input>
            </fb-field>
          </div>
          <fb-button type="submit" variant="primary" full ?loading=${this.busy}>
            Sign in
          </fb-button>
        </fb-form>

        <div class="auth-links">
          <a href="/forgot-password">Forgot password?</a>
        </div>
      </section>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-login-page': FbLoginPage;
  }
}
