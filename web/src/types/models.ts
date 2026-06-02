/**
 * Wire types mirroring internal/models/*.go (JSON tags).
 *
 * Composite Currency Pattern (CLAUDE.md / TECH_STACK §Constraints): money is
 * always an integer-cents field ending in `Cents` PLUS a sibling field ending
 * in `CurrencyCode`. `*Cents` values are typed as `string` here because BIGINT
 * can exceed 2^53 — never parse them through a JS `number` for arithmetic.
 * The composite-currency ESLint rule enforces this naming.
 */

export type CurrencyCode = 'USD' | 'CAD';
export type Role = 'owner' | 'admin' | 'superintendent' | 'field_worker';

/** internal/models.User (PasswordHash / OIDCSubject are json:"-", omitted). */
export interface User {
  id: string;
  org_id: string;
  email: string;
  display_name: string;
  role: Role;
  locale: string;
  last_login_at?: string;
  created_at: string;
  updated_at: string;
}

/** Success body of claim/login/refresh (internal/api/auth.go tokenPairResponse). */
export interface TokenPair {
  access_token: string;
  token_type: 'Bearer';
  expires_in: number;
  refresh_token: string;
  user: User;
}

/** GET /api/v1/capabilities — AI/email availability from BYOK-vault credential presence. */
export interface Capabilities {
  ai_configured: boolean;
  email_configured: boolean;
  providers: ProviderStatus[];
}

export type ProviderName = 'anthropic' | 'resend' | 'gable' | 'localblue';

export interface ProviderStatus {
  provider: ProviderName;
  configured: boolean;
  /** Optional non-reversible fingerprint/last-4 (never the key). */
  fingerprint?: string;
  created_at?: string;
  created_by?: string;
}

/** GET /api/v1/auth/state (backend gap — cold-start routing, OQ-2). */
export interface AuthState {
  needs_bootstrap: boolean;
  setup_complete: boolean;
}

// ----------------------------- Setup wizard -----------------------------
// Mirrors internal/models/setup.go + internal/api/setup.go wire DTOs.

/** Wizard step-1 company fields (stored on organizations). */
export interface CompanyProfile {
  legal_name?: string;
  address?: string;
  ein?: string;
  company_type?: string;
  region?: string;
  onboarding_complete: boolean;
  onboarding_completed_at?: string;
}

export interface TradeCategory {
  id: string;
  org_id: string;
  code: string;
  name: string;
  description?: string;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

export interface CostCode {
  id: string;
  org_id: string;
  code: string;
  name: string;
  division: string;
  parent_code?: string;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

export interface WorkingCalendar {
  id: string;
  org_id: string;
  name: string;
  timezone: string;
  /** Mon..Sun map to bits 0..6 (31 = Mon-Fri). */
  working_days_mask: number;
  daily_work_minutes: number;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

export interface HolidayOverride {
  id: string;
  calendar_id: string;
  org_id: string;
  holiday_date: string;
  name: string;
  created_at: string;
}

export interface PermitJurisdiction {
  id: string;
  org_id: string;
  name: string;
  region?: string;
  permit_types?: unknown;
  inspection_checklist?: unknown;
  notes?: string;
  created_at: string;
  updated_at: string;
}

/** GET /api/v1/setup/state (internal/api/setup.go setupStateResponse). */
export interface SetupState {
  org_id: string;
  onboarding_complete: boolean;
  company_profile: CompanyProfile;
  trades: TradeCategory[];
  cost_codes: CostCode[];
  default_calendar?: WorkingCalendar;
  default_holidays?: HolidayOverride[];
  permit_jurisdictions: PermitJurisdiction[];
}

// --------------------------- Integrations (BYOK) -------------------------
// internal/api/integrations.go integrationCredentialDTO — metadata only,
// the encrypted key bytes never cross the wire.
export interface IntegrationCredential {
  id: string;
  provider: string;
  label: string;
  /** Last 4 chars for recognition; never the full key. */
  last4: string;
  is_active: boolean;
  created_by: string;
  created_at: string;
  updated_at: string;
}

// ----------------------------- Portfolio (Phase D) -----------------------------
// Mirrors internal/models/{project,financials,fleet,hr,pipeline}.go. Per the
// Composite Currency Pattern, every `*_cents` field is a STRING here (the wire
// sends int64 numbers; api/wire.ts `normalizeCents` coerces them at the boundary).

/** internal/models.Project — GET /api/v1/projects (list), /{projectID} (single). */
export interface Project {
  id: string;
  org_id: string;
  name: string;
  address?: string;
  permit_issued_date?: string;
  project_start_date?: string;
  status: string;
  gsf?: number;
  created_at: string;
  updated_at: string;
}

/** internal/models.ProjectTask — GET /api/v1/projects/{id}/tasks (Schedule tab). */
export interface ProjectTask {
  id: string;
  project_id: string;
  wbs_code: string;
  name: string;
  duration_days: number;
  early_start?: string;
  early_finish?: string;
  late_start?: string;
  late_finish?: string;
  // CPM schedule slack in days (physics.BackwardPass), not money — the
  // monetary lint trips on the substring "total".
  // eslint-disable-next-line fb/composite-currency
  total_float?: number;
  is_critical: boolean;
  status: string;
  percent_complete: number;
  assigned_crew?: string[];
  created_at: string;
  updated_at: string;
}

/** internal/models.ProjectBudget — GET /api/v1/projects/{id}/budgets (Budget tab). */
export interface ProjectBudget {
  id: string;
  project_id: string;
  wbs_code: string;
  phase_name: string;
  estimated_cost_cents: string;
  estimated_cost_currency_code: CurrencyCode;
  committed_cost_cents: string;
  committed_cost_currency_code: CurrencyCode;
  actual_cost_cents: string;
  actual_cost_currency_code: CurrencyCode;
  created_at: string;
  updated_at: string;
}

/** internal/models.CorporateBudget — financials summary rollup (per currency). */
export interface CorporateBudget {
  id: string;
  org_id: string;
  fiscal_year: number;
  quarter: number;
  currency_code: CurrencyCode;
  total_estimated_cents: string;
  total_committed_cents: string;
  total_actual_cents: string;
  project_count: number;
  last_rollup_at: string;
  created_at: string;
  updated_at: string;
}

/** internal/models.ARAgingSnapshot — latest AR aging per currency. */
export interface ARAgingSnapshot {
  id: string;
  org_id: string;
  snapshot_date: string;
  currency_code: CurrencyCode;
  current_cents: string;
  days_30_cents: string;
  days_60_cents: string;
  days_90_plus_cents: string;
  total_receivable_cents: string;
  created_at: string;
}

/** service.FinancialsSummary — GET /financials/summary (returned directly). */
export interface FinancialsSummary {
  corporate_budgets: CorporateBudget[];
  ar_aging: ARAgingSnapshot[];
}

/** internal/models.ProjectFinancial — GET /financials/projects (per project+currency). */
export interface ProjectFinancial {
  project_id: string;
  project_name: string;
  currency_code: CurrencyCode;
  total_estimated_cents: string;
  total_committed_cents: string;
  total_actual_cents: string;
  phase_count: number;
}

/** internal/models.FleetAsset — GET /fleet. */
export type FleetAssetStatus = 'available' | 'unavailable' | 'maintenance';
export interface FleetAsset {
  id: string;
  org_id: string;
  name: string;
  asset_type: string;
  serial_number?: string;
  status: FleetAssetStatus;
  created_at: string;
}

/** internal/models.EquipmentAllocation — POST /fleet/{assetID}/allocate. */
export interface EquipmentAllocation {
  id: string;
  asset_id: string;
  project_id: string;
  start_date: string;
  end_date: string;
  created_at: string;
}

/** internal/models.Employee — GET /employees. */
export interface Employee {
  id: string;
  org_id: string;
  user_id?: string;
  first_name: string;
  last_name: string;
  role: string;
  phone?: string;
  hire_date?: string;
  created_at: string;
}

/** internal/models.Certification — GET /employees/{id}/certifications. */
export type CertificationStatus = 'active' | 'expired' | 'revoked';
export interface Certification {
  id: string;
  employee_id: string;
  cert_type: string;
  cert_number?: string;
  issued_date?: string;
  expiry_date: string;
  status: CertificationStatus;
  created_at: string;
}

// ------------------------------- Pipeline (Phase D3) -------------------------------

/** Forward-only CRM stages (internal/models.PipelineStage). */
export type PipelineStage =
  | 'LEAD'
  | 'QUALIFIED'
  | 'ESTIMATE_SENT'
  | 'VERBAL_COMMITMENT'
  | 'PERMIT_APPLIED'
  | 'PERMIT_ISSUED'
  | 'LOST';

/** internal/models.Prospect — one Kanban card. */
export interface Prospect {
  id: string;
  org_id: string;
  name: string;
  client_name: string;
  client_email?: string;
  client_phone?: string;
  address?: string;
  gsf?: number;
  pipeline_stage: PipelineStage;
  probability_pct: number;
  source?: string;
  notes?: string;
  lost_reason?: string;
  /** Set when a prospect reaches PERMIT_ISSUED (deep-links to the project). */
  project_id?: string;
  created_at: string;
  updated_at: string;
}

export interface PipelineEstimateLineItem {
  wbs_code: string;
  description: string;
  // Per-line amount; currency is governed by the parent PipelineEstimate's
  // currency_code (line items never mix currencies within one estimate).
  // eslint-disable-next-line fb/composite-currency
  estimated_cents: string;
  unit?: string;
  quantity?: number;
}

export type EstimateStatus = 'draft' | 'sent' | 'revised' | 'accepted';
export interface PipelineEstimate {
  id: string;
  prospect_id: string;
  version: number;
  total_estimated_cents: string;
  currency_code: CurrencyCode;
  line_items: PipelineEstimateLineItem[];
  margin_pct: number;
  status: EstimateStatus;
  sent_at?: string;
  created_at: string;
  updated_at: string;
}

export type PermitStatus =
  | 'not_submitted'
  | 'submitted'
  | 'under_review'
  | 'revisions_requested'
  | 'approved'
  | 'denied';
export interface Permit {
  id: string;
  prospect_id: string;
  permit_type: string;
  jurisdiction: string;
  application_number?: string;
  submitted_date?: string;
  expected_issue_date?: string;
  actual_issue_date?: string;
  fee_cents: string;
  fee_currency_code: CurrencyCode;
  status: PermitStatus;
  notes?: string;
  created_at: string;
  updated_at: string;
}

/** internal/models.ProspectWithDetails — GET /pipeline/prospects/{id} (returned directly). */
export interface ProspectWithDetails {
  prospect: Prospect;
  estimates: PipelineEstimate[];
  permits: Permit[];
}

/** internal/models.PipelineAnalyticsRow — GET /pipeline/analytics (per currency). */
export interface PipelineAnalyticsRow {
  currency_code: CurrencyCode;
  total_estimated_cents: string;
  weighted_revenue_cents: string;
  prospect_count: number;
}

// ----------------------------- Command Center (Phase E) -----------------------------
// Mirrors internal/service/{schedule,agents}.go + internal/models/{procurement,feed}.go.
// Money stays `*_cents` STRING + currency_code (normalizeCents coerces at the boundary).

/** GET /api/v1/projects/{projectID}/schedule/gantt (service/schedule.go GanttView). */
export interface GanttView {
  tasks: ProjectTask[];
  /** Authoritative ordered set of critical task IDs (uuid). Empty if never recalc'd. */
  critical_path: string[];
  /** RFC3339; zero-value ("0001-01-01T…" / empty) when the schedule was never computed. */
  project_end: string;
}

/** One task row in a recalculated CPM result (service/schedule.go TaskSchedule). */
export interface TaskSchedule {
  id: string;
  wbs_code: string;
  name: string;
  duration_days: number;
  early_start?: string;
  early_finish?: string;
  late_start?: string;
  late_finish?: string;
  // CPM slack in days, not money — the monetary lint trips on "total".
  // eslint-disable-next-line fb/composite-currency
  total_float?: number;
  is_critical: boolean;
}

/** Body of POST .../schedule/recalculate (service/schedule.go). */
export interface CPMResult {
  tasks: TaskSchedule[];
  /** Ordered WBS codes on the critical path (NB: codes here, not ids). */
  critical_path: string[];
  project_end: string;
  critical_path_changed: boolean;
}
export interface RecalcResult {
  cpm_result: CPMResult;
  /** Wall-clock physics time, surfaced as "recomputed in 142ms". */
  recalculation_ms: number;
}

/** One AI duration nudge (service/agents.go). */
export interface ScheduleAdjustment {
  task_id: string;
  wbs_code: string;
  name: string;
  old_duration_days?: number;
  new_duration_days?: number;
  rationale: string;
  /** True when the model returned a number that was applied via UpdateTask. */
  applied: boolean;
}

/** POST .../schedule/recommend-adjustments (service/agents.go ScheduleAdjustmentSet). */
export interface ScheduleAdjustmentSet {
  adjustments: ScheduleAdjustment[];
  /** Count of durations actually changed (CPM re-run server-side). */
  applied_deltas: number;
  /** Count of narrative-only suggestions (no change made). */
  skipped_rationale_only: number;
  run_id?: string;
  tokens_used?: number;
  // Cost block retained only if billing survives standalone (OQ-3); rendered when present.
  cost_cents?: string;
  currency_code?: CurrencyCode;
}

/** Triage status (models/procurement.go) — OK → WARNING → CRITICAL → ORDERED. */
export type ProcurementStatus = 'OK' | 'WARNING' | 'CRITICAL' | 'ORDERED';

/** GET /api/v1/projects/{projectID}/procurement (models/procurement.go). */
export interface ProcurementItem {
  id: string;
  project_id: string;
  name: string;
  wbs_code: string;
  estimated_cost_cents: string;
  estimated_cost_currency_code: CurrencyCode;
  lead_time_days: number;
  weather_buffer_days: number;
  need_by_date?: string;
  must_order_date?: string;
  status: ProcurementStatus;
  ordered_at?: string;
  po_number?: string;
  status_changed_at: string;
  created_at: string;
  updated_at: string;
}

/** Backend feed priority (models/feed.go) — distinct from fb-feed-card's 3-tier UI scale. */
export type FeedBackendPriority = 'critical' | 'urgent' | 'normal' | 'low';
export type FeedCardStatus = 'active' | 'dismissed' | 'actioned' | 'expired';

/** One actionable entry in a feed card's opaque JSONB actions[] (OQ-11). */
export interface FeedCardAction {
  label: string;
  action_type: string;
  payload?: Record<string, unknown>;
}

/** GET /api/v1/feed → { cards: FeedCard[], pagination } (models/feed.go). */
export interface FeedCard {
  id: string;
  project_id?: string;
  card_type: string;
  title: string;
  body: string;
  priority: FeedBackendPriority;
  target_user_id?: string;
  target_role?: Role;
  actions?: FeedCardAction[];
  status: FeedCardStatus;
  actioned_at?: string;
  expires_at?: string;
  created_at: string;
}

/** POST /api/v1/agents/daily-briefing → { briefing } (service/agents.go DailyBriefing). */
export interface DailyBriefing {
  /** The model's morning summary, rendered as markdown. */
  reply: string;
  session_id: string;
  task_count: number;
  alert_count: number;
}

/**
 * One audit-log row (migration 008_audit_log, store/audit.go). The backend has
 * already scrubbed Restricted-class fields from before/after/metadata via
 * `scrubAuditPayloads` (DSC §7.16) — the viewer renders what it's given and
 * never attempts to unmask.
 */
export interface AuditEntry {
  id: string;
  org_id: string;
  /** Subject (user id) of the actor; null/empty when the action was system-driven. */
  actor_sub?: string;
  /** Resolved display name where available, else the viewer falls back to "system". */
  actor_name?: string;
  actor_role?: Role;
  /** Dotted action key, e.g. `setup.trade.created` (humanized for display). */
  action: string;
  resource_type?: string;
  resource_id?: string;
  /** Pre-scrubbed JSONB snapshots for the expandable diff. */
  before?: Record<string, unknown>;
  after?: Record<string, unknown>;
  created_at: string;
}
