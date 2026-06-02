/**
 * Setup wizard endpoints — /api/v1/setup/* (internal/api/setup.go
 * MountSetupRoutes). Admin-gated; exempt from SetupGate so the wizard is
 * reachable while `onboarding_complete=false`. Each mutating step persists one
 * row and returns it wrapped under a named key, matching the Go handlers'
 * `writeJSON(..., map[string]any{"<thing>": ...})` shape.
 */
import { api } from '../client.js';
import type {
  SetupState,
  CompanyProfile,
  TradeCategory,
  CostCode,
  WorkingCalendar,
  HolidayOverride,
  PermitJurisdiction,
} from '../../types/models.js';

export function getSetupState(): Promise<SetupState> {
  return api.get<SetupState>('/api/v1/setup/state');
}

export interface CompanyInfoInput {
  legal_name?: string;
  address?: string;
  ein?: string;
  company_type?: string;
  region?: string;
}
export function updateCompanyInfo(input: CompanyInfoInput): Promise<CompanyProfile> {
  return api
    .post<{ company_profile: CompanyProfile }>('/api/v1/setup/company-info', input)
    .then((r) => r.company_profile);
}

export interface TradeInput {
  code: string;
  name: string;
  description?: string;
  is_default?: boolean;
}
export function createTrade(input: TradeInput): Promise<TradeCategory> {
  return api.post<{ trade: TradeCategory }>('/api/v1/setup/trades', input).then((r) => r.trade);
}

export interface CostCodeInput {
  code: string;
  name: string;
  division: string;
  parent_code?: string;
  is_default?: boolean;
}
export function createCostCode(input: CostCodeInput): Promise<CostCode> {
  return api
    .post<{ cost_code: CostCode }>('/api/v1/setup/cost-codes', input)
    .then((r) => r.cost_code);
}

export interface CalendarInput {
  name: string;
  timezone: string;
  working_days_mask: number;
  daily_work_minutes: number;
  is_default?: boolean;
}
export function createCalendar(input: CalendarInput): Promise<WorkingCalendar> {
  return api
    .post<{ calendar: WorkingCalendar }>('/api/v1/setup/calendars', input)
    .then((r) => r.calendar);
}

export interface HolidayInput {
  /** YYYY-MM-DD or RFC3339. */
  holiday_date: string;
  name: string;
}
export function addHoliday(calendarId: string, input: HolidayInput): Promise<HolidayOverride> {
  return api
    .post<{
      holiday: HolidayOverride;
    }>(`/api/v1/setup/calendars/${encodeURIComponent(calendarId)}/holidays`, input)
    .then((r) => r.holiday);
}

export interface JurisdictionInput {
  name: string;
  region?: string;
  notes?: string;
}
export function addJurisdiction(input: JurisdictionInput): Promise<PermitJurisdiction> {
  return api
    .post<{ jurisdiction: PermitJurisdiction }>('/api/v1/setup/jurisdictions', input)
    .then((r) => r.jurisdiction);
}

export function completeSetup(): Promise<CompanyProfile> {
  return api
    .post<{ company_profile: CompanyProfile }>('/api/v1/setup/complete', {})
    .then((r) => r.company_profile);
}
