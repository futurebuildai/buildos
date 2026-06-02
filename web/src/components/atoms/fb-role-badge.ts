import { html, css, type TemplateResult } from 'lit';
import { customElement, property } from 'lit/decorators.js';
import { FBElement } from '../base/fb-element.js';
import './fb-icon.js';
import type { IconName } from './icons.js';
import type { Role } from '../../types/models.js';

interface RoleMeta {
  label: string;
  color: string;
  icon: IconName;
}

/** Fixed role → label/accent/icon mapping (DSC §7.17). Color is decorative; the label text is always present. */
const ROLE_META: Record<Role, RoleMeta> = {
  owner: { label: 'Owner', color: 'var(--fb-gable-green)', icon: 'shield-check' },
  admin: { label: 'Admin', color: 'var(--fb-blueprint-blue)', icon: 'shield' },
  superintendent: { label: 'Superintendent', color: 'var(--fb-amber-warning)', icon: 'hard-hat' },
  field_worker: { label: 'Field Worker', color: 'var(--fb-text-secondary)', icon: 'user' },
};

/**
 * `fb-role-badge` — compact pill for the four RBAC roles (DSC §7.17). Used in
 * Users & Roles, the audit actor column, and profile. Always color + icon +
 * label; the label text alone is sufficient for SR/colorblind users.
 */
@customElement('fb-role-badge')
export class FbRoleBadge extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host {
        display: inline-flex;
      }
      .role {
        display: inline-flex;
        align-items: center;
        gap: var(--fb-spacing-xs);
        padding: 2px var(--fb-spacing-sm);
        border-radius: var(--fb-radius-full);
        font-size: var(--fb-text-body-sm);
        font-weight: 600;
        color: var(--role-color);
        background: color-mix(in srgb, var(--role-color) 12%, transparent);
        border: 1px solid color-mix(in srgb, var(--role-color) 30%, transparent);
        white-space: nowrap;
      }
    `,
  ];

  // `role` shadows HTMLElement.role (ARIAMixin). attribute:false keeps Lit from
  // reflecting it to an invalid host `role="owner"` ARIA attribute; bind via `.role`.
  @property({ attribute: false }) override role: Role = 'field_worker';

  override render(): TemplateResult {
    const meta = ROLE_META[this.role] ?? ROLE_META.field_worker;
    return html`
      <span class="role" style="--role-color:${meta.color}">
        <fb-icon name=${meta.icon} size="14"></fb-icon>
        <span>${meta.label}</span>
      </span>
    `;
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'fb-role-badge': FbRoleBadge;
  }
}
