import { html, css, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-input.js';
import '../atoms/fb-button.js';
import '../molecules/fb-form.js';
import '../molecules/fb-field.js';
import { requestPasswordReset } from '../../api/endpoints/auth.js';
import { authCardStyles } from './auth-styles.js';

/**
 * `fb-forgot-password-page` — Screen P1 (UX_AUTH_ONBOARDING §5). Requests a reset
 * email. The response is **enumeration-safe**: regardless of whether the address
 * exists (or the request errors), the page shows the same neutral confirmation,
 * so the form never reveals which emails are registered.
 */
@customElement('fb-forgot-password-page')
export class FbForgotPasswordPage extends FBElement {
  static override styles = [FBElement.styles, authCardStyles, css``];

  @state() private busy = false;
  @state() private sent = false;

  private async onSubmit(e: CustomEvent<{ values: Record<string, string> }>): Promise<void> {
    const email = (e.detail.values.email ?? '').trim();
    if (!email) return;
    this.busy = true;
    try {
      await requestPasswordReset(email);
    } catch {
      // Swallow: an error here must not become an existence oracle.
    } finally {
      this.busy = false;
      this.sent = true;
    }
  }

  override render(): TemplateResult {
    if (this.sent) {
      return html`
        <section class="auth-card glass-card" aria-labelledby="fp-title">
          <header class="auth-head">
            <fb-icon name="inbox" size="28"></fb-icon>
            <h1 id="fp-title" class="auth-title">Check your email</h1>
          </header>
          <p class="auth-notice" role="status">
            <fb-icon name="info" size="16"></fb-icon>
            If an account exists for that email, we’ve sent password reset instructions.
          </p>
          <div class="auth-links">
            <a href="/login">Back to sign in</a>
          </div>
        </section>
      `;
    }

    return html`
      <section class="auth-card glass-card" aria-labelledby="fp-title">
        <header class="auth-head">
          <fb-icon name="lock" size="28"></fb-icon>
          <h1 id="fp-title" class="auth-title">Reset your password</h1>
          <p class="auth-sub">We’ll email you a link to set a new one.</p>
        </header>

        <fb-form @submit=${this.onSubmit}>
          <div class="auth-fields">
            <fb-field label="Email" fieldId="fp-email">
              <fb-input
                name="email"
                type="email"
                autocomplete="username"
                inputmode="email"
                required
              ></fb-input>
            </fb-field>
          </div>
          <fb-button type="submit" variant="primary" full ?loading=${this.busy}>
            Send reset link
          </fb-button>
        </fb-form>

        <div class="auth-links">
          <a href="/login">Back to sign in</a>
        </div>
      </section>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-forgot-password-page': FbForgotPasswordPage;
  }
}
