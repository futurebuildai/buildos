/**
 * Phase 3c — admin-config route/nav/error wiring (router + nav-rail + errors).
 *
 * Guards the three contracts the §9.6/§9.7 test plan and the ESC-002 kill-switch
 * depend on:
 *  (a) the `/settings/agents` + `/settings/connectors` route defs exist, are
 *      admin+ (`gate.roles` deep-equals ['owner','admin']) and are NOT plan-gated
 *      (`gate.requiresPro !== true` — ESC-002: the experience kill-switch must
 *      reach admins on self-minted plan_tier="" tokens);
 *  (b) the nav rail surfaces both items (icons `sliders`/`command`) to owner and
 *      admin and hides them from superintendent/field_worker (no dead-end 403s);
 *  (c) `CAPABILITY_DISABLED` is a known machine code mapping to the admin
 *      turned-it-off copy.
 */
import { describe, it, expect, afterEach } from 'vitest';
import { routes, type RouteDef } from '../src/router.js';
import '../src/components/shell/index.js';
import type { FbNavRail } from '../src/components/shell/fb-nav-rail.js';
import { ErrorCode, userMessageForCode } from '../src/api/errors.js';

// --------------------------- helpers (mirror shell.test.ts) ---------------------------
async function mount<T extends HTMLElement>(html: string): Promise<T> {
  const host = document.createElement('div');
  host.innerHTML = html;
  const el = host.firstElementChild as T;
  document.body.appendChild(el);
  await (el as unknown as { updateComplete: Promise<unknown> }).updateComplete;
  return el;
}

function navItem(el: FbNavRail, label: string): Element | null {
  return (
    Array.from(el.shadowRoot!.querySelectorAll('fb-nav-item')).find(
      (n) => n.getAttribute('label') === label,
    ) ?? null
  );
}

function findRoute(path: string): RouteDef | undefined {
  return routes.find((r) => r.path === path);
}

afterEach(() => {
  document.body.innerHTML = '';
});

// ------------------------------- (a) router table -------------------------------
describe('admin-config routes (router table)', () => {
  const adminRoutes: Array<[string, string, string]> = [
    ['/settings/agents', 'fb-agents-page', 'AI Agents'],
    ['/settings/connectors', 'fb-connectors-page', 'Connectors'],
  ];

  for (const [path, tag, title] of adminRoutes) {
    it(`registers ${path} as an org-shell page`, () => {
      const def = findRoute(path);
      expect(def, `route ${path} must exist`).toBeDefined();
      expect(def!.tag).toBe(tag);
      expect(def!.shell).toBe('org');
      expect(def!.title).toBe(title);
    });

    it(`gates ${path} to owner+admin (exact role allow-list)`, () => {
      const def = findRoute(path)!;
      expect(def.gate).toBeDefined();
      expect(def.gate!.roles).toEqual(['owner', 'admin']);
    });

    it(`leaves ${path} NOT plan-gated (ESC-002 kill-switch reaches admins)`, () => {
      const def = findRoute(path)!;
      // The experience kill-switch must reach admins on self-minted plan_tier=""
      // tokens — a regression here re-walls the admin surfaces behind a 402.
      expect(def.gate!.requiresPro).not.toBe(true);
    });
  }
});

// ------------------------------- (b) nav rail -------------------------------
describe('admin-config nav items (fb-nav-rail)', () => {
  const items: Array<[string, string]> = [
    ['AI Agents', 'sliders'],
    ['Connectors', 'command'],
  ];

  it('shows both items with the right icons for an owner', async () => {
    const el = await mount<FbNavRail>('<fb-nav-rail></fb-nav-rail>');
    el.userRole = 'owner';
    await el.updateComplete;
    for (const [label, icon] of items) {
      const item = navItem(el, label);
      expect(item, `${label} should render for owner`).not.toBeNull();
      expect(item!.getAttribute('icon')).toBe(icon);
    }
  });

  it('shows both items with the right icons for an admin', async () => {
    const el = await mount<FbNavRail>('<fb-nav-rail></fb-nav-rail>');
    el.userRole = 'admin';
    await el.updateComplete;
    for (const [label, icon] of items) {
      const item = navItem(el, label);
      expect(item, `${label} should render for admin`).not.toBeNull();
      expect(item!.getAttribute('icon')).toBe(icon);
    }
  });

  it('hides both items from a superintendent', async () => {
    const el = await mount<FbNavRail>('<fb-nav-rail></fb-nav-rail>');
    el.userRole = 'superintendent';
    await el.updateComplete;
    for (const [label] of items) {
      expect(navItem(el, label), `${label} must be hidden from superintendent`).toBeNull();
    }
  });

  it('hides both items from a field worker', async () => {
    const el = await mount<FbNavRail>('<fb-nav-rail></fb-nav-rail>');
    el.userRole = 'field_worker';
    await el.updateComplete;
    for (const [label] of items) {
      expect(navItem(el, label), `${label} must be hidden from field worker`).toBeNull();
    }
  });
});

// ------------------------------- (c) error code -------------------------------
describe('CAPABILITY_DISABLED error code', () => {
  it('is a known machine code with a stable string value', () => {
    expect(ErrorCode.CAPABILITY_DISABLED).toBe('CAPABILITY_DISABLED');
  });

  it('maps to the admin-turned-it-off copy', () => {
    expect(userMessageForCode('CAPABILITY_DISABLED')).toBe(
      'The AI assistant has been turned off by an admin.',
    );
    // Symbolic and literal lookups agree.
    expect(userMessageForCode(ErrorCode.CAPABILITY_DISABLED)).toBe(
      userMessageForCode('CAPABILITY_DISABLED'),
    );
  });
});
