/**
 * FutureBuild OS — Web Frontend entry point.
 * Registers all Lit web components and boots the application.
 *
 * Component loading order:
 * 1. Base element (shared styles + utilities)
 * 2. Design tokens and utilities
 * 3. Atoms (fb-button, fb-icon, fb-badge, etc.)
 * 4. Molecules (fb-feed-card, fb-stat-card, fb-nav-item, etc.)
 * 5. Organisms (fb-org-shell, fb-nav-sidebar, fb-data-table, etc.)
 * 6. Pages (fb-dashboard-page, fb-financials-view, etc.)
 * 7. Router initialization
 */

// ── Base Element ──────────────────────────────────────────────────────────
import './components/base/fb-element.js';

// ── Atoms ─────────────────────────────────────────────────────────────────
import './components/atoms/fb-button.js';
import './components/atoms/fb-icon.js';
import './components/atoms/fb-badge.js';
import './components/atoms/fb-text.js';
import './components/atoms/fb-input.js';
import './components/atoms/fb-select.js';
import './components/atoms/fb-chip.js';
import './components/atoms/fb-spinner.js';
import './components/atoms/fb-avatar.js';

// ── Molecules ─────────────────────────────────────────────────────────────
import './components/molecules/fb-feed-card.js';
import './components/molecules/fb-stat-card.js';
import './components/molecules/fb-nav-item.js';
import './components/molecules/fb-data-cell.js';
import './components/molecules/fb-search-bar.js';
import './components/molecules/fb-toast.js';
import './components/molecules/fb-tab-bar.js';
import './components/molecules/fb-breadcrumb.js';

// ── Organisms ─────────────────────────────────────────────────────────────
import './components/organisms/fb-org-shell.js';
import './components/organisms/fb-nav-sidebar.js';
import './components/organisms/fb-data-table.js';
import './components/organisms/fb-gantt-chart.js';
import './components/organisms/fb-budget-summary.js';
import './components/organisms/fb-ar-aging-chart.js';
import './components/organisms/fb-feed-list.js';
import './components/organisms/fb-pipeline-kanban.js';
import './components/organisms/fb-pipeline-summary.js';
import './components/organisms/fb-pipeline-card.js';
import './components/organisms/fb-prospect-detail.js';
import './components/organisms/fb-estimate-form.js';
import './components/organisms/fb-permit-tracker.js';

// ── Pages ─────────────────────────────────────────────────────────────────
import './components/pages/fb-dashboard-page.js';
import './components/pages/fb-financials-view.js';
import './components/pages/fb-schedule-view.js';
import './components/pages/fb-briefing-view.js';
import './components/pages/fb-procurement-view.js';
import './components/pages/fb-pipeline-view.js';
import './components/pages/fb-prospect-detail-page.js';
import './components/pages/fb-fleet-view.js';
import './components/pages/fb-hr-view.js';
import './components/pages/fb-settings-view.js';

// ── Router ────────────────────────────────────────────────────────────────
import { initRouter } from './router.js';

// ── Initialize Application ────────────────────────────────────────────────
initRouter();

// Log startup
console.log(
  '%c FutureBuild OS %c v0.1.0 ',
  'background: #00FFA3; color: #0A0B10; font-weight: bold; padding: 2px 8px; border-radius: 4px 0 0 4px;',
  'background: #161821; color: #E2E8F0; padding: 2px 8px; border-radius: 0 4px 4px 0;',
);
