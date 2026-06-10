// Load the most expensive write path: POST schedule/recalculate, which runs the
// deterministic CPM physics engine (forward/backward pass over the project DAG)
// inside a DB tx and may enqueue a delay_cascade job. The bench gates cap the
// pure CPM at <=200ms (80 tasks) / <=500ms (200 tasks); this measures it end to
// end under concurrency including DB I/O. PROJECT_ID must point at a seeded
// project with a representative task graph.
//   k6 run -e BASE_URL=... -e ORG_ID=... -e USER_ID=... -e ROLE=superintendent -e PROJECT_ID=... scripts/k6/schedule_recalc.js
import http from 'k6/http';
import { sleep } from 'k6';
import {
  BASE_URL,
  ORG_ID,
  PROJECT_ID,
  authHeaders,
  RECALC_THRESHOLDS,
  expectOk,
} from './lib/config.js';

export const options = {
  thresholds: RECALC_THRESHOLDS,
  scenarios: {
    recalc: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '30s', target: 5 },
        { duration: '1m', target: 15 },
        { duration: '30s', target: 0 },
      ],
    },
  },
};

export function setup() {
  if (!PROJECT_ID) {
    throw new Error('set PROJECT_ID to a seeded project with a task graph');
  }
}

export default function () {
  const url = `${BASE_URL}/api/v1/org/${ORG_ID}/projects/${PROJECT_ID}/schedule/recalculate`;
  const res = http.post(url, '{}', {
    headers: authHeaders(),
    tags: { endpoint: 'recalculate' },
  });
  expectOk(res, 'recalculate');
  sleep(1);
}
