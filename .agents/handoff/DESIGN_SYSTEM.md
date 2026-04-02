# Design System — GableLBM Industrial Dark

**Document ID:** AG-05-DS
**System:** FutureBuild OS (System of Execution)
**Created:** 2026-04-02
**Pipeline Stage:** 05 - Design System
**Status:** COMPLETE
**Source of Truth:** `reference-vault/futurebuild-os/specs/GABLE_LBM_DESIGN_SYSTEM.md`

---

## 1. Design Principles

Grounded in persona research (Mike/Superintendent, Sarah/Admin, Tom/Owner, Carlos/Field Worker) and the FutureBuild vision: "Simple Interface. Powerful Engine."

| # | Principle | Rationale | Persona Link |
|---|-----------|-----------|--------------|
| 1 | **Data Density Over Decoration** | Construction professionals manage 50+ simultaneous variables. Every pixel must convey information. Bloomberg Terminal ethos — not consumer SaaS. | Tom needs one-screen financial overview; Sarah needs sortable tables with 100+ rows |
| 2 | **Agent-as-Sieve** | The system surfaces what matters. Users never hunt for data — AI-driven feed cards, morning briefings, and procurement alerts push priorities to the user. | Mike's morning briefing; Carlos's task list |
| 3 | **Offline-Resilient** | Job sites have unreliable connectivity. All field interactions must queue locally and sync when connectivity restores. Visual offline indicators are mandatory. | Carlos working in a cellular dead zone; Drift outbox pattern |
| 4 | **Progressive Disclosure** | Show summary first, detail on demand. Drill-down from portfolio → project → phase → task. Never overwhelm with all data at once. | Tom clicks project card → sees financials by phase → sees individual invoices |
| 5 | **Numerical Precision** | Construction is math-heavy. All monetary values, schedule durations, and identifiers use monospaced typography for columnar alignment and rapid scanning. | JetBrains Mono for all data; BIGINT cents formatting |

---

## 2. Color System

### 2.1 Core Palette

| Token | Hex | HSL | Role | WCAG on Deep Space |
|-------|-----|-----|------|-------------------|
| **Gable Green** | `#00FFA3` | `158° 100% 50%` | Primary accent, CTAs, success states, active indicators | AAA (12.8:1) |
| **Deep Space** | `#0A0B10` | `230° 20% 5%` | Base background, page canvas | N/A (base) |
| **Slate Steel** | `#161821` | `217° 33% 17%` | Surface containers, card backgrounds, nav panels | N/A (surface) |
| **Blueprint Blue** | `#38BDF8` | `199° 95% 60%` | Secondary accent, info states, links | AAA (8.6:1) |
| **Safety Red** | `#F43F5E` | `346° 87% 60%` | Error states, destructive actions, critical alerts | AA (5.1:1) |
| **Amber Warning** | `#F59E0B` | `38° 92% 50%` | Warning states, approaching deadlines | AA (6.3:1) |

### 2.2 Surface Elevation Scale

| Layer | Token | Hex | Usage |
|-------|-------|-----|-------|
| 0 | `--fb-surface-0` | `#0A0B10` | Page background |
| 1 | `--fb-surface-1` / `--md-sys-color-surface-container` | `#161821` | Primary cards, nav panels |
| 2 | `--fb-surface-2` | `#1E2029` | Nested containers, hover states |
| 3 | `--fb-surface-3` / `--md-sys-color-surface-container-highest` | `#252836` | Elevated modals, dropdowns |

### 2.3 Semantic Color Tokens

```css
:root {
  /* Primary System */
  --md-sys-color-primary: #00FFA3;
  --md-sys-color-on-primary: #003822;
  --md-sys-color-primary-container: #005234;
  --md-sys-color-on-primary-container: #66FFC8;

  /* Background & Surface */
  --md-sys-color-background: #0A0B10;
  --md-sys-color-on-background: #F0F0F5;
  --md-sys-color-surface: #0A0B10;
  --md-sys-color-on-surface: #F0F0F5;
  --md-sys-color-on-surface-variant: #8B8D98;

  /* Borders & Outlines */
  --md-sys-color-outline: #5A5B66;
  --md-sys-color-outline-variant: rgba(255, 255, 255, 0.05);
  --fb-border: rgba(255, 255, 255, 0.05);
  --fb-border-light: rgba(255, 255, 255, 0.03);

  /* Semantic States */
  --fb-success: #00FFA3;
  --fb-error: #F43F5E;
  --fb-warning: #F59E0B;
  --fb-info: #38BDF8;
}
```

### 2.4 Tailwind CSS 4 Extension

```css
/* tailwind.config.css (Tailwind CSS 4 — CSS-first config) */
@theme {
  /* Core Gable Palette */
  --color-gable-green: #00FFA3;
  --color-gable-green-dim: #005234;
  --color-gable-green-bright: #66FFC8;
  --color-deep-space: #0A0B10;
  --color-deep-space-soft: #0E0F15;
  --color-slate-steel: #161821;
  --color-slate-steel-raised: #1E2029;
  --color-slate-steel-elevated: #252836;

  /* Semantic Colors */
  --color-blueprint-blue: #38BDF8;
  --color-safety-red: #F43F5E;
  --color-amber-warning: #F59E0B;

  /* Text Colors */
  --color-text-primary: #F0F0F5;
  --color-text-secondary: #8B8D98;
  --color-text-muted: #5A5B66;

  /* Glass Effects */
  --color-glass-bg: rgba(22, 24, 33, 0.6);
  --color-glass-border: rgba(255, 255, 255, 0.05);
  --color-glass-panel: rgba(10, 11, 16, 0.8);

  /* Typography */
  --font-sans: 'Outfit', system-ui, -apple-system, sans-serif;
  --font-mono: 'JetBrains Mono', 'Fira Code', monospace;

  /* Spacing Scale (4px base) */
  --spacing-xs: 4px;
  --spacing-sm: 8px;
  --spacing-md: 16px;
  --spacing-lg: 24px;
  --spacing-xl: 32px;
  --spacing-2xl: 48px;

  /* Border Radius */
  --radius-xs: 6px;
  --radius-sm: 8px;
  --radius-md: 12px;
  --radius-lg: 16px;
  --radius-xl: 24px;
  --radius-full: 9999px;

  /* Elevation Shadows */
  --shadow-elevation-1: 0px 1px 3px 1px rgba(0,0,0,0.25), 0px 1px 2px 0px rgba(0,0,0,0.4);
  --shadow-elevation-2: 0px 2px 6px 2px rgba(0,0,0,0.25), 0px 1px 2px 0px rgba(0,0,0,0.4);
  --shadow-elevation-3: 0px 4px 8px 3px rgba(0,0,0,0.25), 0px 1px 3px 0px rgba(0,0,0,0.4);
  --shadow-glow: 0 0 20px rgba(0,255,163,0.15);
  --shadow-glow-strong: 0 0 30px rgba(0,255,163,0.3), 0 0 60px rgba(0,255,163,0.1);
}
```

---

## 3. Typography System

### 3.1 Font Families

| Font | Role | Weight Range | Loading Strategy |
|------|------|-------------|-----------------|
| **Outfit** | All UI labels, headings, body copy, navigation | 400, 500, 700 | Google Fonts preload; `font-display: swap` |
| **JetBrains Mono** | Monetary values, schedule durations, IDs, percentages, data tables, code blocks | 400, 500, 700 | Google Fonts preload; `font-display: swap` |

### 3.2 JetBrains Mono Mandate

JetBrains Mono MUST be used for:
- All currency displays (e.g., `$1,234,567.89`)
- Schedule durations (e.g., `14d 6h`)
- Percentages and variance (e.g., `+2.3%`, `-$4,500`)
- Project/task IDs (e.g., `WBS 7.3.2`)
- Table columns containing numerical data
- Gantt chart date labels
- CPM float values
- AR aging bucket amounts

Outfit MUST be used for:
- Navigation labels, menu items
- Button text, form labels
- Card titles, section headers
- Body text, descriptions
- Status badges (text portion)
- Toast/notification messages

### 3.3 Type Scale

| Role | Font | Size/Line-Height | Weight | Token |
|------|------|-------------------|--------|-------|
| Display Large | Outfit | 57px / 64px | 400 | `--md-sys-typescale-display-large` |
| Display Medium | Outfit | 45px / 52px | 400 | `--md-sys-typescale-display-medium` |
| Display Small | Outfit | 36px / 44px | 400 | `--md-sys-typescale-display-small` |
| Headline Large | Outfit | 32px / 40px | 400 | `--md-sys-typescale-headline-large` |
| Headline Medium | Outfit | 28px / 36px | 400 | `--md-sys-typescale-headline-medium` |
| Headline Small | Outfit | 24px / 32px | 400 | `--md-sys-typescale-headline-small` |
| Title Large | Outfit | 22px / 28px | 400 | `--md-sys-typescale-title-large` |
| Title Medium | Outfit | 16px / 24px | 500 | `--md-sys-typescale-title-medium` |
| Title Small | Outfit | 14px / 20px | 500 | `--md-sys-typescale-title-small` |
| Body Large | Outfit | 16px / 24px | 400 | `--md-sys-typescale-body-large` |
| Body Medium | Outfit | 14px / 20px | 400 | `--md-sys-typescale-body-medium` |
| Body Small | Outfit | 12px / 16px | 400 | `--md-sys-typescale-body-small` |
| Label Large | Outfit | 14px / 20px | 500 | `--md-sys-typescale-label-large` |
| Label Medium | Outfit | 12px / 16px | 500 | `--md-sys-typescale-label-medium` |
| Label Small | Outfit | 11px / 16px | 500 | `--md-sys-typescale-label-small` |
| Data Display | JetBrains Mono | 14px / 20px | 400 | `--fb-font-mono` + `--fb-text-sm` |
| Data Compact | JetBrains Mono | 12px / 16px | 400 | `--fb-font-mono` + `--fb-text-xs` |

---

## 4. Spacing System

Base unit: **4px**. All spacing tokens are multiples of 4.

| Token | Value | Usage |
|-------|-------|-------|
| `--fb-spacing-xs` | 4px | Icon gaps, inline spacing |
| `--fb-spacing-sm` | 8px | Compact padding, list item gaps |
| `--fb-spacing-md` | 16px | Standard card padding, section gaps |
| `--fb-spacing-lg` | 24px | Large card padding, panel gaps |
| `--fb-spacing-xl` | 32px | Section separators |
| `--fb-spacing-2xl` | 48px | Page-level margins, hero spacing |

---

## 5. Shape System

| Token | Value | Usage |
|-------|-------|-------|
| `--md-sys-shape-corner-extra-small` | 6px | Badges, chips |
| `--md-sys-shape-corner-small` | 8px | Buttons, small inputs |
| `--md-sys-shape-corner-medium` | 12px | Cards, modals, form groups |
| `--md-sys-shape-corner-large` | 16px | Glass cards, dialog panels |
| `--md-sys-shape-corner-extra-large` | 24px | Workspace containers, hero panels |
| `--md-sys-shape-corner-full` | 9999px | Pill buttons, avatar circles, status dots |

---

## 6. Elevation & Shadows

| Level | Token | Box-Shadow | Usage |
|-------|-------|-----------|-------|
| 1 | `--md-sys-elevation-1` | `0px 1px 3px 1px rgba(0,0,0,0.25), 0px 1px 2px 0px rgba(0,0,0,0.4)` | Cards at rest, glass cards |
| 2 | `--md-sys-elevation-2` | `0px 2px 6px 2px rgba(0,0,0,0.25), 0px 1px 2px 0px rgba(0,0,0,0.4)` | Hovered cards, dropdowns |
| 3 | `--md-sys-elevation-3` | `0px 4px 8px 3px rgba(0,0,0,0.25), 0px 1px 3px 0px rgba(0,0,0,0.4)` | Modals, dialogs, toast panels |
| Glow | `--fb-glow` | `0 0 20px rgba(0,255,163,0.15)` | Active state, selected items |
| Glow Strong | `--fb-glow-strong` | `0 0 30px rgba(0,255,163,0.3), 0 0 60px rgba(0,255,163,0.1)` | Primary CTAs on hover, alerts |

---

## 7. Glassmorphism Specification

### 7.1 Glass Card (Standard)

```css
.glass-card {
  background: rgba(22, 24, 33, 0.6);      /* Slate Steel at 60% opacity */
  backdrop-filter: blur(24px);
  -webkit-backdrop-filter: blur(24px);
  border: 1px solid rgba(255, 255, 255, 0.05);
  border-radius: 16px;
  box-shadow: var(--md-sys-elevation-1);
}
```

**Usage:** Financial summary cards, feed cards, project cards, artifact panels.

### 7.2 Glass Panel (Heavy)

```css
.glass-panel {
  background: rgba(10, 11, 16, 0.8);      /* Deep Space at 80% opacity */
  backdrop-filter: blur(48px);
  -webkit-backdrop-filter: blur(48px);
  border: 1px solid rgba(255, 255, 255, 0.05);
}
```

**Usage:** Side navigation, modal overlays, command palettes.

### 7.3 Glassmorphism Rules

- **Minimum backdrop-filter:** 24px blur for readability
- **Maximum background opacity:** 80% (maintain some transparency)
- **Border:** Always 1px `rgba(255,255,255,0.05)` for edge definition
- **Fallback:** Solid `--fb-surface-1` for browsers without `backdrop-filter` support
- **Performance:** Limit to 3 concurrent glass layers per viewport to prevent GPU strain

---

## 8. FBBaseElement Class

### 8.1 Implementation (Lit 3.0)

```typescript
import { LitElement, css, CSSResultGroup } from 'lit';

export abstract class FBElement extends LitElement {
  static override styles: CSSResultGroup = css`
    :host { box-sizing: border-box; }
    :host *, :host *::before, :host *::after { box-sizing: inherit; }

    /* === Glassmorphism === */
    .glass-card {
      background: rgba(22, 24, 33, 0.6);
      backdrop-filter: blur(24px);
      -webkit-backdrop-filter: blur(24px);
      border: 1px solid rgba(255, 255, 255, 0.05);
      border-radius: 16px;
      box-shadow: var(--md-sys-elevation-1);
    }
    .glass-panel {
      background: rgba(10, 11, 16, 0.8);
      backdrop-filter: blur(48px);
      -webkit-backdrop-filter: blur(48px);
      border: 1px solid rgba(255, 255, 255, 0.05);
    }

    /* === Interaction === */
    .hover-lift {
      transition: transform 0.3s ease-out, box-shadow 0.3s ease-out;
    }
    .hover-lift:hover {
      transform: translateY(-4px);
      box-shadow: var(--md-sys-elevation-2);
    }

    /* === Glow === */
    .glow-accent { box-shadow: 0 0 20px rgba(0, 255, 163, 0.15); }
    .glow-accent-strong {
      box-shadow: 0 0 30px rgba(0, 255, 163, 0.3), 0 0 60px rgba(0, 255, 163, 0.1);
    }

    /* === Active Indicator === */
    .active-indicator { position: relative; }
    .active-indicator::before {
      content: '';
      position: absolute;
      left: 0; top: 50%;
      transform: translateY(-50%);
      width: 3px; height: 60%;
      background: #00FFA3;
      border-radius: 0 3px 3px 0;
    }

    /* === Buttons === */
    .btn-primary {
      transition: transform 0.15s ease-out, box-shadow 0.15s ease-out;
    }
    .btn-primary:hover {
      box-shadow: 0 0 20px rgba(0, 255, 163, 0.15);
      transform: translateY(-1px);
    }
    .btn-primary:active { transform: scale(0.95); box-shadow: none; }

    .btn-destructive {
      border: 1px solid rgba(244, 63, 94, 0.3);
      transition: box-shadow 0.15s ease-out;
    }
    .btn-destructive:hover { box-shadow: 0 0 20px rgba(244, 63, 94, 0.15); }

    /* === Skeleton Loading === */
    .skeleton {
      background: linear-gradient(90deg,
        var(--md-sys-color-surface-container-high) 25%,
        var(--md-sys-color-surface-container) 50%,
        var(--md-sys-color-surface-container-high) 75%
      );
      background-size: 200% 100%;
      animation: shimmer 2s linear infinite;
      border-radius: 4px;
    }
    .skeleton-text { height: 1em; width: 100%; margin-bottom: 0.5em; display: block; }
    .skeleton-box { height: 100px; width: 100%; display: block; }
    .skeleton-bar { height: 8px; width: 100%; display: block; }

    @keyframes shimmer {
      0% { background-position: 200% 0; }
      100% { background-position: -200% 0; }
    }

    /* === Data Typography === */
    .data-mono {
      font-family: 'JetBrains Mono', 'Fira Code', monospace;
      font-variant-numeric: tabular-nums;
    }
    .data-currency { font-family: 'JetBrains Mono', monospace; font-variant-numeric: tabular-nums; }
    .data-positive { color: #00FFA3; }
    .data-negative { color: #F43F5E; }
  `;

  protected emit<T = unknown>(name: string, detail?: T): CustomEvent<T> {
    const event = new CustomEvent<T>(name, {
      bubbles: true,
      composed: true,
      detail: detail as T,
    });
    this.dispatchEvent(event);
    return event;
  }
}
```

### 8.2 Inheritance Pattern

```typescript
@customElement('fb-budget-summary')
export class FBBudgetSummary extends FBElement {
  static override styles = [
    FBElement.styles,
    css`
      :host { display: block; }
      .amount { font-family: var(--fb-font-mono); font-variant-numeric: tabular-nums; }
    `
  ];
}
```

---

## 9. Component Catalog

### 9.1 Atoms

| Component | Tag | Props | Description |
|-----------|-----|-------|-------------|
| Button | `<fb-button>` | `variant: primary\|secondary\|destructive\|ghost`, `size: sm\|md\|lg`, `disabled`, `loading` | Standard action button with glow hover |
| Icon | `<fb-icon>` | `name: string`, `size: sm\|md\|lg` | Material Symbols icon wrapper |
| Badge | `<fb-badge>` | `variant: success\|warning\|error\|info\|neutral`, `size: sm\|md` | Status indicator pill |
| Text | `<fb-text>` | `variant: display\|headline\|title\|body\|label\|data`, `color`, `mono: boolean` | Typography block (auto-applies JetBrains Mono when `mono` or `variant=data`) |
| Input | `<fb-input>` | `type: text\|number\|date\|email`, `label`, `error`, `disabled` | Form input with Industrial Dark styling |
| Select | `<fb-select>` | `options: Array`, `label`, `value` | Dropdown with glass panel overlay |
| Chip | `<fb-chip>` | `label`, `removable`, `selected` | Filter/tag chip |
| Spinner | `<fb-spinner>` | `size: sm\|md\|lg` | Loading indicator |
| Avatar | `<fb-avatar>` | `name`, `src`, `size: sm\|md\|lg` | User avatar (initials fallback) |

### 9.2 Molecules

| Component | Tag | Description |
|-----------|-----|-------------|
| Feed Card | `<fb-feed-card>` | Agent-generated action card with priority badge, actions (Approve/Dismiss), glass-card styling |
| Stat Card | `<fb-stat-card>` | KPI display: label (Outfit) + value (JetBrains Mono) + trend indicator |
| Nav Item | `<fb-nav-item>` | Sidebar navigation item with icon, label, active-indicator bar |
| Data Cell | `<fb-data-cell>` | Table cell with auto-detection: applies JetBrains Mono for numeric content |
| Search Bar | `<fb-search-bar>` | Glass-panel search input with keyboard shortcut hint |
| Toast | `<fb-toast>` | Notification toast with severity icon, auto-dismiss, glass-card |
| Tab Bar | `<fb-tab-bar>` | Horizontal tab strip with active underline |
| Breadcrumb | `<fb-breadcrumb>` | Navigation breadcrumb (Portfolio > Project > Phase) |

### 9.3 Organisms

| Component | Tag | Description |
|-----------|-----|-------------|
| Data Table | `<fb-data-table>` | Sortable, filterable table with virtual scrolling (1000+ rows). JetBrains Mono for numeric columns. Green/red variance coloring. |
| AR Aging Chart | `<fb-ar-aging-chart>` | D3.js stacked horizontal bar (Current/30/60/90+). Glass-card container. |
| Budget Summary | `<fb-budget-summary>` | Three-metric card: Estimated / Committed / Actual with variance percentages |
| Feed List | `<fb-feed-list>` | Sorted feed card list (critical > urgent > normal > low) |
| Project Card | `<fb-project-card>` | Glass-card with project name, status badge, budget bar, completion % |
| Gantt Chart | `<fb-gantt-chart>` | Canvas/SVG timeline with task bars, critical path highlighting, CPM dependencies |
| Nav Sidebar | `<fb-nav-sidebar>` | Glass-panel navigation with project tree, daily focus, agent activity |
| Org Shell | `<fb-org-shell>` | Top-level layout: sidebar + content area + responsive collapse |
| Modal | `<fb-modal>` | Glass-panel overlay for fullscreen artifact view or confirmation dialogs |

### 9.4 Page Components

| Component | Tag | Workspace | Description |
|-----------|-----|-----------|-------------|
| Financials View | `<fb-financials-view>` | Portfolio | Budget summary + AR aging + project financials table |
| Fleet View | `<fb-fleet-view>` | Portfolio | Asset grid with status badges + maintenance alerts |
| HR View | `<fb-hr-view>` | Portfolio | Employee table + certification expiration alerts |
| Briefing View | `<fb-briefing-view>` | Agent Command Center | Feed card list + weather + priority sorting |
| Procurement View | `<fb-procurement-view>` | Agent Command Center | Procurement feed + Tribunal recommendation cards |
| Schedule View | `<fb-schedule-view>` | Agent Command Center | Gantt chart + CPM data + resource conflicts |
| Task List View | `<fb-task-list-view>` | Field Portal (Flutter) | Offline-capable task cards with completion buttons |
| Daily Log View | `<fb-daily-log-view>` | Field Portal (Flutter) | Photo capture + crew check-in + progress reporting |

---

## 10. Interaction Patterns

### 10.1 States

| State | Visual Treatment |
|-------|-----------------|
| **Default** | `--fb-surface-1` background, `--md-sys-elevation-1` shadow |
| **Hover** | `translateY(-4px)`, `--md-sys-elevation-2` shadow (`.hover-lift`) |
| **Active/Pressed** | `scale(0.95)`, no shadow |
| **Focus** | 2px solid `#00FFA3` outline, 2px offset (keyboard only via `:focus-visible`) |
| **Disabled** | 40% opacity, `pointer-events: none` |
| **Loading** | `.skeleton` shimmer animation on content area |
| **Empty** | Centered icon + message in `--md-sys-color-on-surface-variant` |
| **Error** | `--fb-error` border-left 3px, error message below |
| **Success** | Brief `--fb-glow` animation (300ms), then return to default |
| **Offline** | Amber badge with pending count; dashed border on queued actions |

### 10.2 Motion & Animation

| Motion | Duration | Easing | Usage |
|--------|----------|--------|-------|
| Instant | 0ms | — | Color changes, opacity toggles |
| Fast | 150ms | `ease-out` | Button press, micro-interactions |
| Standard | 200ms | `ease-out` | Hover transitions, fade-in/out |
| Emphasized | 300ms | `cubic-bezier(0.2, 0, 0, 1)` | Card expansion, panel slide, modal enter |
| Deliberate | 500ms | `cubic-bezier(0.2, 0, 0, 1)` | Page transitions, skeleton reveal |
| Shimmer | 2000ms | `linear` | Skeleton loading animation (infinite) |

**`prefers-reduced-motion` rule:** All animated elements must check `prefers-reduced-motion: reduce` and disable transforms/animations, keeping only opacity changes.

### 10.3 Responsive Breakpoints

| Breakpoint | Name | Layout |
|------------|------|--------|
| ≥1440px | Desktop XL | 3-panel: sidebar (280px) + content (flex) + artifact panel (320px) |
| ≥1200px | Desktop | 2-panel: sidebar (280px) + content (flex). Artifact panel on demand. |
| ≥768px | Tablet | Single column with collapsible sidebar (hamburger). |
| <768px | Mobile | Full-width content. Bottom nav bar. Sidebar as overlay. |

---

## 11. Accessibility Requirements

| Requirement | Standard | Implementation |
|-------------|----------|---------------|
| Contrast (text) | WCAG AA (4.5:1 minimum) | `#F0F0F5` on `#0A0B10` = 16.1:1. `#8B8D98` on `#0A0B10` = 5.2:1 (AA). |
| Contrast (large text) | WCAG AA (3:1 minimum) | All heading colors meet 3:1+ |
| Keyboard navigation | Full keyboard access | Tab order follows visual order. `:focus-visible` with 2px `#00FFA3` outline. |
| Screen readers | ARIA landmarks | `role="navigation"`, `role="main"`, `role="complementary"` on workspace panels |
| Focus indicators | Visible on all interactive elements | 2px solid `#00FFA3`, 2px offset. Never `outline: none` without replacement. |
| Reduced motion | `prefers-reduced-motion` | Disable transforms, animations. Keep opacity transitions only. |
| Touch targets | 48px minimum | All buttons, nav items, checkboxes meet 48x48px minimum. Field Portal uses 64px. |
| Error identification | WCAG 3.3.1 | Errors identified by color AND text AND icon (not color alone) |

---

## 12. Gradient & Brand Effects

| Token | Value | Usage |
|-------|-------|-------|
| Dawn Gradient | `linear-gradient(135deg, #00FFA3 0%, #00CC82 100%)` | Primary CTA backgrounds, hero banners |
| Glow Accent | `0 0 20px rgba(0,255,163,0.15)` | Active nav items, selected states |
| Glow Strong | `0 0 30px rgba(0,255,163,0.3), 0 0 60px rgba(0,255,163,0.1)` | Primary buttons on hover |

---

## 13. Icon System

- **Provider:** Material Symbols (Outlined, weight 400, grade 0, optical size 24)
- **Custom icons:** SVG sprite for FutureBuild-specific icons (hard hat, excavator, WBS tree)
- **Sizing:** 20px (sm), 24px (md), 32px (lg)
- **Color:** Inherits `currentColor` from parent

---

## 14. Dark Mode Policy

FutureBuild OS is **dark-only**. There is no light mode. The Industrial Dark aesthetic is a core brand identity element, not a user preference.

- No `prefers-color-scheme: light` media query
- No theme toggle UI
- All design tokens assume dark background
- Print stylesheets may invert for paper output

---

## 15. Currency Formatting (Frontend)

All monetary values arrive from the backend as **BIGINT cents**. The frontend is responsible for display formatting:

```typescript
function formatCents(cents: bigint | number): string {
  const dollars = Number(cents) / 100;
  return new Intl.NumberFormat('en-US', {
    style: 'currency',
    currency: 'USD',
    minimumFractionDigits: 2,
  }).format(dollars);
}
```

**Rules:**
- Always render in `<span class="data-currency">` for JetBrains Mono
- Positive variance: `data-positive` class (Gable Green)
- Negative variance: `data-negative` class (Safety Red)
- Never store or compute with floating-point on the frontend
