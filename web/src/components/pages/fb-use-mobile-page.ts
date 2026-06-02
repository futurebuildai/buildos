import { html, css, type TemplateResult } from 'lit';
import { customElement } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import '../atoms/fb-icon.js';
import '../atoms/fb-button.js';
import { logout } from '../../state/authStore.js';
import { navigate } from '../../router.js';
import { authCardStyles } from './auth-styles.js';

/**
 * `fb-use-mobile-page` — field_worker landing (FRONTEND_ARCHITECTURE §1.1). The
 * web console has no field surfaces; field workers are routed here instead of a
 * dead-end 403. Offers a sign-out so a shared device can hand back to an
 * admin/owner.
 */
@customElement('fb-use-mobile-page')
export class FbUseMobilePage extends FBElement {
  static override styles = [
    FBElement.styles,
    authCardStyles,
    css`
      .use-actions {
        display: flex;
        justify-content: center;
        margin-top: var(--fb-spacing-lg);
      }
    `,
  ];

  private async signOut(): Promise<void> {
    await logout();
    navigate('/login', { replace: true });
  }

  override render(): TemplateResult {
    return html`
      <section class="auth-card glass-card" aria-labelledby="um-title">
        <header class="auth-head">
          <fb-icon name="hard-hat" size="28"></fb-icon>
          <h1 id="um-title" class="auth-title">Use the BuildOS mobile app</h1>
          <p class="auth-sub">
            Field work — tasks, daily logs, and photos — lives in the mobile app. The web console is
            for office and management roles.
          </p>
        </header>
        <div class="use-actions">
          <fb-button variant="secondary" icon="logout" @click=${this.signOut}>Sign out</fb-button>
        </div>
      </section>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-use-mobile-page': FbUseMobilePage;
  }
}
