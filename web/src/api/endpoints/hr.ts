/**
 * HR endpoints — /api/v1/org/{orgID}/employees/* (internal/api/fleet.go
 * HRHandler). Owner/admin only. Employee + certification rows carry PII
 * (names, phones) — the UI never logs them (CLAUDE.md PII rules).
 */
import { api } from '../client.js';
import type { Employee, Certification } from '../../types/models.js';

export function listEmployees(orgId: string): Promise<Employee[]> {
  return api
    .get<{ employees: Employee[] }>(`/api/v1/org/${encodeURIComponent(orgId)}/employees`)
    .then((r) => r.employees ?? []);
}

export function listCertifications(orgId: string, employeeId: string): Promise<Certification[]> {
  return api
    .get<{
      certifications: Certification[];
    }>(
      `/api/v1/org/${encodeURIComponent(orgId)}/employees/${encodeURIComponent(employeeId)}/certifications`,
    )
    .then((r) => r.certifications ?? []);
}
