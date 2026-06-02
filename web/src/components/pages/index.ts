/**
 * Page-component barrel. Importing this registers every `fb-*-page` custom
 * element so the router can render them by tag (router.ts route table). Phase C
 * ships the auth, first-run, onboarding-wizard, and BYOK-integrations pages;
 * Phase D adds the portfolio workspace (projects, financials, pipeline, fleet,
 * HR). Later phases extend this list with the command-center screens.
 */
import './fb-login-page.js';
import './fb-first-run-page.js';
import './fb-forgot-password-page.js';
import './fb-reset-password-page.js';
import './fb-use-mobile-page.js';
import './fb-setup-page.js';
import './fb-integrations-page.js';

// Phase D — portfolio workspace.
import './fb-projects-page.js';
import './fb-project-detail-page.js';
import './fb-financials-page.js';
import './fb-pipeline-page.js';
import './fb-fleet-page.js';
import './fb-hr-page.js';

// Phase E — command center workspace.
import './fb-schedule-page.js';
import './fb-procurement-page.js';
import './fb-briefing-page.js';
import './fb-assistant-page.js';
import './fb-activity-page.js';
