import { html, css, nothing, type TemplateResult } from 'lit';
import { customElement, state } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-input.js';
import '../atoms/fb-password-input.js';
import '../atoms/fb-secret-input.js';
import '../atoms/fb-button.js';
import '../molecules/fb-form.js';
import '../molecules/fb-field.js';
import { claimFirstOwner } from '../../state/authStore.js';
import { navigate } from '../../router.js';
import { ApiError, ErrorCode, userMessageForCode } from '../../api/errors.js';
import { authCardStyles } from './auth-styles.js';

/**
 * `fb-first-run-page` — Screen B1, first-owner bootstrap claim
 * (UX_AUTH_ONBOARDING §1). Redeems the one-shot bootstrap token (43-char
 * base64url, validated client-side via `fb-secret-input bootstrap`) and sets the
 * owner's email/display-name/password.
 *
 * Failures are uniform: any bad/expired/consumed token yields
 * "That setup token is invalid or expired." (no probing oracle, §1). When an
 * owner already exists the backend returns `FIRST_OWNER_EXISTS` — a terminal
 * state that points the visitor at the sign-in screen. On success the new owner
 * goes straight to the setup wizard (their org is not yet onboarded).
 */
@customElement('fb-first-run-page')
export class FbFirstRunPage extends FBElement {
  static override styles = [FBElement.styles, authCardStyles, css``];

  @state() private busy = false;
  @state() private error: string | null = null;
  @state() private terminal = false;

  private async onSubmit(e: CustomEvent<{ values: Record<string, string> }>): Promise<void> {
    const v = e.detail.values;
    const token = (v.token ?? '').trim();
    const email = (v.email ?? '').trim();
    const display_name = (v.display_name ?? '').trim();
    const password = v.password ?? '';
    const confirm = v.confirm ?? '';

    if (!token || !email || !display_name || !password) {
      this.error = 'Fill in every field to claim your deployment.';
      return;
    }
    if (password !== confirm) {
      this.error = 'Those passwords don’t match.';
      return;
    }

    this.busy = true;
    this.error = null;
    try {
      await claimFirstOwner({ token, email, password, display_name });
      // The owner's org is brand-new and not onboarded — go straight to setup.
      navigate('/setup', { replace: true });
    } catch (err) {
      if (err instanceof ApiError && err.code === ErrorCode.FIRST_OWNER_EXISTS) {
        this.terminal = true;
        this.error = null;
        return;
      }
      this.error =
        err instanceof ApiError
          ? userMessageForCode(err.code)
          : 'That setup token is invalid or expired.';
    } finally {
      this.busy = false;
    }
  }

  private renderTerminal(): TemplateResult {
    return html`
      <section class="auth-card glass-card" aria-labelledby="fr-title">
        <header class="auth-head">
          <fb-icon name="shield-check" size="28"></fb-icon>
          <h1 id="fr-title" class="auth-title">This deployment is already set up</h1>
          <p class="auth-sub">An owner has already claimed this BuildOS.</p>
        </header>
        <div class="auth-links">
          <a href="/login">Go to sign in</a>
        </div>
      </section>
    `;
  }

  override render(): TemplateResult {
    if (this.terminal) return this.renderTerminal();
    return html`
      <section class="auth-card glass-card" aria-labelledby="fr-title">
        <header class="auth-head">
          <fb-icon name="hexagon" size="28"></fb-icon>
          <h1 id="fr-title" class="auth-title">Claim your BuildOS</h1>
          <p class="auth-sub">Redeem your setup token to create the owner account.</p>
        </header>

        ${this.error
          ? html`<p class="auth-error" role="alert">
              <fb-icon name="alert-circle" size="16"></fb-icon>${this.error}
            </p>`
          : nothing}

        <fb-form @submit=${this.onSubmit}>
          <div class="auth-fields">
            <fb-field
              label="Setup token"
              fieldId="fr-token"
              hint="The one-time token from your deployment setup."
            >
              <fb-secret-input
                name="token"
                bootstrap
                placeholder="Paste setup token"
              ></fb-secret-input>
            </fb-field>
            <fb-field label="Your name" fieldId="fr-name">
              <fb-input name="display_name" type="text" autocomplete="name" required></fb-input>
            </fb-field>
            <fb-field label="Email" fieldId="fr-email">
              <fb-input
                name="email"
                type="email"
                autocomplete="username"
                inputmode="email"
                required
              ></fb-input>
            </fb-field>
            <fb-field label="Password" fieldId="fr-password">
              <fb-password-input
                name="password"
                autocomplete="new-password"
                required
              ></fb-password-input>
            </fb-field>
            <fb-field label="Confirm password" fieldId="fr-confirm">
              <fb-password-input
                name="confirm"
                autocomplete="new-password"
                required
              ></fb-password-input>
            </fb-field>
          </div>
          <fb-button type="submit" variant="primary" full ?loading=${this.busy}>
            Create owner account
          </fb-button>
        </fb-form>

        <div class="auth-links">
          <a href="/login">Already have an account? Sign in</a>
        </div>
      </section>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-first-run-page': FbFirstRunPage;
  }
}
