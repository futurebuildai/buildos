// Load the field read hot path: GET /api/v1/field/sync. This single pull carries
// the field worker's tasks + feed cards + (Phase 4a-ii) allocated equipment, so
// it is the most-hit authenticated read on a busy job site. ROLE must be
// field_worker+ and USER_ID a user with assigned tasks for a representative
// payload.
//   k6 run -e BASE_URL=... -e ORG_ID=... -e USER_ID=... -e ROLE=field_worker scripts/k6/field_sync.js
import http from 'k6/http';
import { sleep } from 'k6';
import { BASE_URL, authHeaders, READ_THRESHOLDS, expectOk } from './lib/config.js';

export const options = {
  thresholds: READ_THRESHOLDS,
  scenarios: {
    field_sync: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 20 },
        { duration: '1m', target: 50 },
        { duration: '30s', target: 0 },
      ],
    },
  },
};

export default function () {
  const res = http.get(`${BASE_URL}/api/v1/field/sync`, {
    headers: authHeaders(),
    tags: { endpoint: 'field_sync' },
  });
  expectOk(res, 'field/sync');
  sleep(1);
}
