/**
 * Typed API client — the single choke-point for all backend calls.
 *
 * Responsibilities (FRONTEND_ARCHITECTURE §4):
 *  - Unwrap the standard envelope `{ data, error, meta }` (internal/api/response.go).
 *  - Throw a structured `ApiError` (branching on machine `code`, not status).
 *  - Implement the single-flight 401 → refresh → retry interceptor (§4.2).
 *
 * The refresh callback is injected (see setRefreshHandler) to avoid an import
 * cycle with the auth store, which owns the refresh + logout side-effects.
 */
import { ApiError, ErrorCode, type FieldError } from './errors.js';
import { getAccessToken } from './tokens.js';

const API_BASE = (import.meta.env.VITE_API_BASE_URL ?? '/').replace(/\/$/, '');

interface Envelope<T> {
  data?: T;
  error?: { code: string; message: string; details?: FieldError[] };
  meta?: { request_id?: string; timestamp?: string };
}

export interface RequestOptions {
  method?: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  body?: unknown;
  /** Extra headers (e.g. X-Dev-Auth in non-prod rigs). */
  headers?: Record<string, string>;
  /** Skip the access-token header (auth endpoints that mint credentials). */
  skipAuth?: boolean;
  /**
   * Send/receive cookies on this request (`credentials: 'include'`). ONLY the
   * auth endpoints set this so the HttpOnly `buildos_refresh` cookie is set on
   * claim/login/refresh and replayed on refresh/logout. Every other API call
   * leaves it off — they authenticate with the Bearer access-token header and
   * must not ride cookies (the cookie's Path=/api/v1/auth scoping already keeps
   * it off them, but this is explicit).
   */
  withCredentials?: boolean;
  /** Internal: prevents the refresh→retry loop from recursing. */
  _isRetry?: boolean;
  signal?: AbortSignal;
}

/**
 * Refresh handler injected by the auth store. Returns true if a new access
 * token is now available, false if the session is dead (→ caller logs out).
 */
type RefreshHandler = () => Promise<boolean>;
let refreshHandler: RefreshHandler | null = null;
export function setRefreshHandler(fn: RefreshHandler): void {
  refreshHandler = fn;
}

/** Single-flight: concurrent 401s share one in-flight refresh promise (§4.2). */
let inFlightRefresh: Promise<boolean> | null = null;
function refreshOnce(): Promise<boolean> {
  if (!refreshHandler) return Promise.resolve(false);
  if (!inFlightRefresh) {
    inFlightRefresh = refreshHandler().finally(() => {
      inFlightRefresh = null;
    });
  }
  return inFlightRefresh;
}

function buildUrl(path: string): string {
  if (path.startsWith('http')) return path;
  return `${API_BASE}${path.startsWith('/') ? path : `/${path}`}`;
}

async function parseEnvelope<T>(res: Response): Promise<Envelope<T>> {
  const text = await res.text();
  if (!text) return {};
  try {
    return JSON.parse(text) as Envelope<T>;
  } catch {
    return {};
  }
}

function statusToCode(status: number): string {
  switch (status) {
    case 401:
      return ErrorCode.UNAUTHORIZED;
    case 403:
      return ErrorCode.FORBIDDEN;
    case 404:
      return ErrorCode.NOT_FOUND;
    case 409:
      return ErrorCode.CONFLICT;
    case 413:
      return ErrorCode.PAYLOAD_TOO_LARGE;
    case 429:
      return ErrorCode.RATE_LIMITED;
    case 501:
      return ErrorCode.NOT_IMPLEMENTED;
    case 503:
      return ErrorCode.SERVICE_UNAVAILABLE;
    default:
      return status >= 500 ? ErrorCode.INTERNAL_ERROR : ErrorCode.UNKNOWN;
  }
}

export async function request<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const method = opts.method ?? 'GET';
  const headers: Record<string, string> = { ...opts.headers };

  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json';
  }
  if (!opts.skipAuth) {
    const token = getAccessToken();
    if (token) headers['Authorization'] = `Bearer ${token}`;
  }

  const init: RequestInit = { method, headers };
  if (opts.body !== undefined) init.body = JSON.stringify(opts.body);
  if (opts.signal) init.signal = opts.signal;
  // Cookies ride ONLY the auth endpoints (refresh-cookie transport). Default
  // 'same-origin' is left implicit for every other call so the Bearer-header
  // API surface stays cookie-free.
  if (opts.withCredentials) init.credentials = 'include';

  let res: Response;
  try {
    res = await fetch(buildUrl(path), init);
  } catch {
    throw new ApiError({
      code: ErrorCode.NETWORK_ERROR,
      message: 'network request failed',
      status: 0,
    });
  }

  // 401 → single-flight refresh → retry once (§4.2). Auth endpoints opt out.
  if (res.status === 401 && !opts.skipAuth && !opts._isRetry) {
    const refreshed = await refreshOnce();
    if (refreshed) {
      return request<T>(path, { ...opts, _isRetry: true });
    }
    // Refresh failed: the auth store has cleared session + routed to /login.
    throw new ApiError({
      code: ErrorCode.UNAUTHORIZED,
      message: 'session expired',
      status: 401,
    });
  }

  const envelope = await parseEnvelope<T>(res);
  const requestId = envelope.meta?.request_id;

  if (!res.ok) {
    const code = envelope.error?.code ?? statusToCode(res.status);
    throw new ApiError({
      code,
      message: envelope.error?.message ?? res.statusText,
      status: res.status,
      ...(envelope.error?.details ? { details: envelope.error.details } : {}),
      ...(requestId ? { requestId } : {}),
    });
  }

  return (envelope.data ?? (undefined as T)) as T;
}

export const api = {
  get: <T>(path: string, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...opts, method: 'GET' }),
  post: <T>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'method'>) =>
    request<T>(path, { ...opts, method: 'POST', body }),
  put: <T>(path: string, body?: unknown, opts?: Omit<RequestOptions, 'method'>) =>
    request<T>(path, { ...opts, method: 'PUT', body }),
  delete: <T>(path: string, opts?: Omit<RequestOptions, 'method' | 'body'>) =>
    request<T>(path, { ...opts, method: 'DELETE' }),
};
