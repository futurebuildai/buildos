// Adversarial load on POST /api/v1/auth/login. This INTENTIONALLY exceeds the
// per-IP rate limit (DefaultRateLimitRPS=50 / burst=100) to verify two things:
//   1. The limiter holds — past the burst the server returns 429, it does NOT
//      melt down doing unbounded argon2id work (a DoS-amplification guard, since
//      argon2id is deliberately expensive).
//   2. No 5xx under the flood (no resource-exhaustion crash).
// It also serves as a manual check that login does not leak account existence
// (a wrong password for an unknown vs known user should be indistinguishable).
//
// NOTE: run this from a SINGLE source IP so the per-IP bucket actually engages
// (RealIP/X-Forwarded-For aware). Use a throwaway/non-existent email.
//   k6 run -e BASE_URL=... scripts/k6/auth_login.js
import http from 'k6/http';
import { check } from 'k6';
import { BASE_URL } from './lib/config.js';

export const options = {
  scenarios: {
    login_flood: {
      executor: 'constant-arrival-rate',
      rate: 150, // > the 50 rps steady-state + 100 burst
      timeUnit: '1s',
      duration: '30s',
      preAllocatedVUs: 50,
      maxVUs: 200,
    },
  },
  thresholds: {
    // The server must never 5xx and the limiter must actually throttle.
    checks: ['rate>0.99'],
    'checks{check:limiter throttles past the burst}': ['rate>0.10'],
  },
};

export default function () {
  const res = http.post(
    `${BASE_URL}/api/v1/auth/login`,
    JSON.stringify({ email: 'loadtest-nonexistent@example.com', password: 'wrong-password' }),
    { headers: { 'Content-Type': 'application/json' } },
  );
  check(res, {
    'no 5xx (limiter absorbs the flood)': (r) => r.status < 500,
    'auth rejected or throttled (never accepted)': (r) =>
      r.status === 429 || r.status === 401 || r.status === 400,
    'limiter throttles past the burst': (r) => r.status === 429,
  });
}
