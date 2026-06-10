// Shared config for the BuildOS k6 load harness (Phase 4c).
//
// Authenticated scenarios use the dev-auth header (the target must run with
// DEV_AUTH_MODE=header — a STAGING/load build, never prod: the prod build tag
// no-ops the header, so requests would 401). Point BASE_URL at the load target
// and set ORG_ID / USER_ID / PROJECT_ID to seeded fixtures.
//
//   k6 run -e BASE_URL=https://staging.example.com \
//          -e ORG_ID=<uuid> -e USER_ID=<uuid> -e ROLE=superintendent \
//          -e PROJECT_ID=<uuid> scripts/k6/field_sync.js
import { check } from 'k6';

export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
export const ORG_ID = __ENV.ORG_ID || '00000000-0000-0000-0000-000000000001';
export const USER_ID = __ENV.USER_ID || '00000000-0000-0000-0000-000000000002';
export const ROLE = __ENV.ROLE || 'superintendent';
export const PROJECT_ID = __ENV.PROJECT_ID || '';

// X-Dev-Auth: <sub>,<org_id>,<role> (internal/api/middleware/auth_dev.go).
export function authHeaders() {
  return {
    'X-Dev-Auth': `${USER_ID},${ORG_ID},${ROLE}`,
    'Content-Type': 'application/json',
  };
}

// SLOs aligned to the CPM bench gates (BenchmarkCPM80Tasks<=200ms,
// CPM200<=500ms) plus DB + HTTP overhead. Tune per deployment.
export const READ_THRESHOLDS = {
  http_req_failed: ['rate<0.01'],
  http_req_duration: ['p(95)<500', 'p(99)<1000'],
};
export const RECALC_THRESHOLDS = {
  http_req_failed: ['rate<0.01'],
  http_req_duration: ['p(95)<1000', 'p(99)<2000'],
};

export function expectOk(res, name) {
  check(res, {
    [`${name} is 2xx`]: (r) => r.status >= 200 && r.status < 300,
  });
}
