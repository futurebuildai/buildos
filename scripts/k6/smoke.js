// Smoke test: a single VU exercising the probes + the field read path. Run this
// first to confirm the target + fixtures are wired before a full load run.
//   k6 run -e BASE_URL=... -e ORG_ID=... -e USER_ID=... -e ROLE=field_worker scripts/k6/smoke.js
import http from 'k6/http';
import { sleep } from 'k6';
import { BASE_URL, authHeaders, expectOk } from './lib/config.js';

export const options = {
  vus: 1,
  iterations: 5,
  thresholds: { http_req_failed: ['rate<0.01'] },
};

export default function () {
  expectOk(http.get(`${BASE_URL}/health`), 'health');
  expectOk(http.get(`${BASE_URL}/ready`), 'ready');
  expectOk(
    http.get(`${BASE_URL}/api/v1/field/sync`, { headers: authHeaders() }),
    'field/sync',
  );
  sleep(1);
}
