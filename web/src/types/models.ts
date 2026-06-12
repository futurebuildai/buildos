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
  /** Object storage (R2) configured — gates the photo-upload affordances. */
  storage_configured: boolean;
  providers: ProviderStatus[];
}

export type ProviderName = 'anthropic' | 'resend' | 'gable' | 'localblue' | 'object_store';

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
  // Homeowner contact (client_name/email/phone) is Restricted PII and is
  // deliberately NOT serialized on the Project response (server: json:"-",
  // review finding M2). It is read server-side only for the owner/admin
  // client-update send path; no operator surface receives it over the wire.
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

/** CPM dependency relationship (internal/models/types.DependencyType). v1 arrows render FS. */
export type DependencyType = 'FS' | 'SS' | 'FF' | 'SF';

/** internal/models.TaskDependency — one edge in a project's task graph. */
export interface TaskDependency {
  id: string;
  project_id: string;
  predecessor_id: string;
  successor_id: string;
  dependency_type: DependencyType;
  lag_days: number;
}

/** GET /api/v1/projects/{projectID}/schedule/gantt (service/schedule.go GanttView). */
export interface GanttView {
  tasks: ProjectTask[];
  /** Authoritative ordered set of critical task IDs (uuid). Empty if never recalc'd. */
  critical_path: string[];
  /** RFC3339; zero-value ("0001-01-01T…" / empty) when the schedule was never computed. */
  project_end: string;
  /** Task dependency edges for drawing arrows; stable `[]` when no edges. */
  dependencies?: TaskDependency[];
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

/** One enriched AI duration proposal (service/agents.go ScheduleProposal). */
export interface ScheduleAdjustment {
  task_id: string;
  wbs_code: string;
  name: string;
  old_duration_days: number;
  /** Proposed duration; omitted on advisory / monitor-only rows. */
  new_duration_days?: number;
  rationale: string;
  /** Whether the task is currently on the critical path. */
  is_critical: boolean;
  /** True iff this row carries a real duration change to apply (vs advisory). */
  proposed_change: boolean;
  /** True only on the legacy auto-apply path; always false on a dry-run preview. */
  applied: boolean;
}

/** POST .../schedule/recommend-adjustments (service/agents.go ScheduleAdjustmentSet). */
export interface ScheduleAdjustmentSet {
  adjustments: ScheduleAdjustment[];
  /** True when this was a PREVIEW (no writes). PREVIEW-FIRST: AI proposes, human commits. */
  dry_run: boolean;
  /** Count of rows with a real duration change the user could apply. */
  proposed_changes: number;
  /** Count of monitor-only / advisory rows (no proposed change). */
  advisory_count: number;
  /** Count of durations actually changed (0 on a dry-run preview). */
  applied_deltas: number;
  /** True when CPM re-ran after a real apply. */
  critical_recomputed: boolean;
  /** Wire-compat alias of advisory_count (narrative-only suggestions). */
  skipped_rationale_only: number;
  run_id?: string;
  tokens_used?: number;
  // Cost block retained only if billing survives standalone (OQ-3); rendered when present.
  cost_cents?: string;
  currency_code?: CurrencyCode;
}

/** One row in the apply request body (POST .../schedule/adjustments/apply). */
export interface ScheduleAdjustmentApply {
  wbs_code: string;
  new_duration_days: number;
}

/** Response of POST .../schedule/adjustments/apply (service/agents.go ScheduleApplyResult). */
export interface ScheduleApplyResult {
  applied_deltas: number;
  critical_recomputed: boolean;
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

// ----------------------------- Conversational assistant (Phase 2c) -----------------------------
// POST /api/v1/agents/chat (internal/api/assistant.go). The endpoint is MULTI-TURN
// but STATELESS server-side: the client owns the running thread and resends it
// (capped) on every call — there is NO session_id in the response (unlike
// DailyBriefing). Identity (org/role/sub) comes from JWT claims server-side and is
// NEVER in the body. `tools_used` is the wire-safe transparency surface: tool name +
// error flag ONLY (args/results are Confidential and withheld by design).

/** One prior turn of the conversation, resent as `history`. */
export interface ChatTurn {
  role: 'user' | 'assistant';
  text: string;
}

/** A tool the model consulted on this turn — name + error flag only, never args/results. */
export interface ToolTrace {
  name: string;
  is_error: boolean;
}

/** POST /api/v1/agents/chat 200 body (unwrapped from the envelope `.data`). */
export interface ChatResponse {
  reply: string;
  tools_used: ToolTrace[];
  iterations: number;
  /** true (still a 200) when the loop hit a bound before end_turn — answer may be incomplete. */
  truncated: boolean;
}

// ----------------------------- Admin config (Phase 3c) -----------------------------
// Mirrors the Go admin surfaces: /api/v1/admin/agents (§2.1) and
// /api/v1/admin/connectors (§2.2). `config` serializes from json.RawMessage as an
// EMBEDDED JSON OBJECT (never a quoted string) — typed `Record<string, unknown>`
// and ALWAYS sent back to the server as an object, never JSON.stringify'd.

/** The three catalog capabilities the agentic harness exposes for config. */
export type AgentCapability = 'delay_cascade' | 'foresight' | 'experience';

/** Whether a row is the catalog default or an org-level override. */
export type ConfigSource = 'default' | 'override';

/** GET /api/v1/admin/agents → { agents: EffectiveAgentConfig[] } (effective view). */
export interface EffectiveAgentConfig {
  capability: AgentCapability;
  /** Catalog sentence (raw backend description; the UI authors friendlier copy). */
  description: string;
  enabled: boolean;
  /** Embedded object, always ≥ {}. foresight carries tuning; the others are {}. */
  config: Record<string, unknown>;
  source: ConfigSource;
  /** omitempty — present on overrides only. */
  updated_by?: string;
  /** omitempty ISO — present on overrides only. */
  updated_at?: string;
}

/** PUT /api/v1/admin/agents/{capability} → { agent: AgentConfig } (the persisted override row). */
export interface AgentConfig {
  id: string;
  org_id: string;
  capability: AgentCapability;
  enabled: boolean;
  config: Record<string, unknown>;
  updated_by: string;
  created_at: string;
  updated_at: string;
}

/**
 * foresight tuning thresholds — both POSITIVE integers (≥1); defaults {2, 80}.
 * A `type` (not `interface`) so it stays structurally assignable to the
 * `Record<string, unknown>` `config` field of the admin PUT body (interfaces get
 * no implicit index signature; object-literal type aliases do).
 */
export type ForesightConfig = {
  schedule_float_days: number;
  // A PERCENTAGE threshold (0–100 integer), NOT a monetary amount — the field
  // name is fixed by the backend wire contract (internal/agentic/foresight.go).
  // The fb/composite-currency rule matches the "budget" substring; this is a
  // genuine false positive, so the threshold stays a plain integer `number`.
  // eslint-disable-next-line fb/composite-currency
  budget_burn_percent: number;
};

export const FORESIGHT_DEFAULTS: ForesightConfig = {
  schedule_float_days: 2,
  budget_burn_percent: 80,
};

/**
 * Safely read foresight thresholds out of a `Record<string, unknown>` config,
 * coercing finite positive integers and falling back to the catalog defaults
 * {2, 80} for anything missing/NaN/non-positive. `noUncheckedIndexedAccess`
 * forces this guarded read — never index `config.schedule_float_days` directly
 * on a non-foresight card.
 */
export function readForesightConfig(config: Record<string, unknown>): ForesightConfig {
  const coerce = (raw: unknown, fallback: number): number => {
    const n = typeof raw === 'number' ? raw : Number(raw);
    return Number.isInteger(n) && n >= 1 ? n : fallback;
  };
  return {
    schedule_float_days: coerce(
      config['schedule_float_days'],
      FORESIGHT_DEFAULTS.schedule_float_days,
    ),
    budget_burn_percent: coerce(
      config['budget_burn_percent'],
      FORESIGHT_DEFAULTS.budget_burn_percent,
    ),
  };
}

/** Discriminates a built-in connector (the `reference` catalog entry) from an MCP instance. */
export type ConnectorKind = 'builtin' | 'mcp';

/** GET /api/v1/admin/connectors → { connectors: EffectiveConnector[] } (effective view). */
export interface EffectiveConnector {
  /** Connector name; the built-in catalog has exactly one today: "reference". */
  connector: string;
  /** UI branches on THIS, never the name string. */
  kind: ConnectorKind;
  description: string;
  enabled: boolean;
  /** Embedded object, ≥ {}. MCP carries `{ endpoint }`. */
  config: Record<string, unknown>;
  /** omitempty — MCP only (https URL). */
  endpoint?: string;
  /** Always present int; 0 for builtin / un-refreshed MCP. */
  tools_count: number;
  /** omitempty ISO — MCP only, present ONLY after a successful refresh. */
  tools_fetched_at?: string;
  source: ConfigSource;
  /** omitempty — override only. */
  updated_by?: string;
  /** omitempty ISO — override only. */
  updated_at?: string;
}

/** PUT /api/v1/admin/connectors/{connector} → { connector: ConnectorConfig } (the persisted row). */
export interface ConnectorConfig {
  id: string;
  org_id: string;
  connector_name: string;
  kind: ConnectorKind;
  enabled: boolean;
  config: Record<string, unknown>;
  updated_by: string;
  created_at: string;
  updated_at: string;
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

// ----------------------------- Feedback (Phase 0b) -----------------------------

export type FeedbackCategory = 'bug' | 'idea' | 'friction' | 'other';

/** Triage lifecycle owned by the backend; the widget only ever sees `new`. */
export type FeedbackStatus = 'new' | 'triaged' | 'planned' | 'shipped' | 'declined';

/**
 * Client-captured submission context (all strings). The server caps the
 * serialized object at 4096 bytes, so keep values short and flat.
 *
 * This widget always SENDS all five keys, so the submit payload keeps them
 * required. The backend guarantees none of them on READ (API_CONTRACT 13d:
 * `context?: object` — it may be `{}` or any object ≤4096B), so the read
 * model below uses `Partial<FeedbackContext>`.
 */
export interface FeedbackContext {
  route: string;
  role: string;
  app_version: string;
  user_agent: string;
  viewport: string;
}

/** POST /api/v1/feedback → 201 { feedback: Feedback }. */
export interface Feedback {
  id: string;
  org_id: string;
  user_sub: string;
  category: FeedbackCategory;
  message: string;
  /** READ model: no keys guaranteed by the backend — treat every key as optional. */
  context: Partial<FeedbackContext>;
  status: FeedbackStatus;
  triage_note: string;
  created_at: string;
  updated_at: string;
}

// ---------------------------------------------------------------------------
// Daily Reports (Chunk C — DAILY_REPORTS_CLIENT_UPDATES). A DERIVED read model
// (no daily_reports table) assembled per (project, date) from daily_logs +
// crew_checkins + task_progress. SafetyIncidents IS present on the operator
// surface; the client-safe homeowner draft is a separately-redacted projection
// built server-side from an allowlist. Mirrors internal/models/dailyreport.go.
// ---------------------------------------------------------------------------

/** An object-storage asset row (Chunk A). storage_key is never serialized. */
export interface Asset {
  id: string;
  org_id: string;
  project_id?: string;
  content_type: string;
  size_bytes: number;
  status: 'pending' | 'ready' | 'failed';
  uploaded_by: string;
  checksum_sha256?: string;
  created_at: string;
  confirmed_at?: string;
}

/** A daily_logs row (returned by the photo-link endpoint). */
export interface DailyLog {
  id: string;
  org_id: string;
  project_id: string;
  reported_by: string;
  log_date: string;
  weather_conditions?: string;
  work_summary: string;
  safety_incidents?: string;
  photo_asset_ids?: string[];
  idempotency_key: string;
  reported_at: string;
}

/** A resolved photo on a daily report: asset id + short-lived signed GET URL. */
export interface PhotoRef {
  asset_id: string;
  thumb_url: string;
  created_at?: string;
}

/** One per-task progress line folded into a daily report (no crew identity/GPS). */
export interface TaskProgressLine {
  task_id: string;
  wbs_code: string;
  name: string;
  percent_complete: number;
  notes?: string;
  reported_at: string;
}

/** GET /api/v1/projects/{id}/daily-reports/{date} — one day's full derived report. */
export interface DailyReport {
  project_id: string;
  project_name: string;
  log_date: string;
  weather_conditions?: string;
  work_summary: string;
  /** INTERNAL — present on the operator surface, never on a client update. */
  safety_incidents?: string;
  photos?: PhotoRef[];
  photo_count: number;
  reported_by: string;
  crew_count: number;
  task_progress?: TaskProgressLine[];
  reported_at: string;
  has_log: boolean;
}

/** GET /api/v1/projects/{id}/daily-reports — list-row summary projection. */
export interface DailyReportSummary {
  project_id: string;
  log_date: string;
  weather_conditions?: string;
  work_summary: string;
  has_safety_incident: boolean;
  photo_count: number;
  crew_count: number;
  task_progress_count: number;
  reported_at: string;
}

/**
 * POST /api/v1/projects/{id}/daily-reports/{date}/client-update-draft — the
 * AI-generated, client-SAFE homeowner draft. Chunk C produces the draft only;
 * the editable composer + send is Chunk D.
 */
export interface ClientUpdateDraft {
  subject: string;
  body: string;
  period_start: string;
  period_end: string;
  photo_count: number;
}

/** internal/models.ClientUpdate lifecycle status. */
export type ClientUpdateStatus = 'draft' | 'sent' | 'failed';

/**
 * internal/models.ClientUpdate — the human-in-the-loop client-update row
 * (Chunk D). The composer persists an AI draft, the operator edits subject/
 * edited_body + curates photo_asset_ids, then sends. recipient_email is NEVER
 * serialized (json:"-" server-side) — it is absent from this shape on purpose.
 */
export interface ClientUpdate {
  id: string;
  org_id: string;
  project_id: string;
  period_start: string;
  period_end: string;
  status: ClientUpdateStatus;
  ai_draft?: string;
  edited_body: string;
  subject: string;
  photo_asset_ids: string[];
  created_by: string;
  sent_by?: string;
  sent_at?: string;
  send_error?: string;
  created_at: string;
  updated_at: string;
}

/** Derived status of a public share link (server-computed, Chunk E). */
export type ShareLinkStatus = 'active' | 'revoked' | 'expired';

/**
 * internal/api.shareLinkView — the operator-side view of a public share link
 * (Chunk E). The cleartext token + its hash are NEVER in this shape; the
 * cleartext is returned exactly once (in CreateShareLinkResponse.url) at create.
 */
export interface ShareLink {
  id: string;
  client_update_id: string;
  status: ShareLinkStatus;
  expires_at: string;
  revoked_at?: string;
  last_viewed_at?: string;
  view_count: number;
  created_at: string;
}

/**
 * Response of POST .../share-links — the ONE-TIME public URL the operator
 * copies + emails to the homeowner, plus the link record. `url` is shown once
 * and never returned again (the cleartext token is not stored).
 */
export interface CreateShareLinkResponse {
  url: string;
  link: ShareLink;
}
