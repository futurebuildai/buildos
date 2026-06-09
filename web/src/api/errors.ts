/**
 * API error taxonomy.
 *
 * The client branches on the machine `code` (not the raw HTTP status) per
 * FRONTEND_ARCHITECTURE §4.3 and DSC §11.1. Backend error envelope shape
 * (internal/api/response.go):
 *
 *   { "error": { "code", "message", "details": [{field, reason}] }, "meta": {...} }
 */

/** Canonical machine error codes the client knows how to treat. */
export const ErrorCode = {
  // Auth (internal/api/auth.go writeAuthError)
  INVALID_CREDENTIALS: 'INVALID_CREDENTIALS',
  INVALID_REFRESH_TOKEN: 'INVALID_REFRESH_TOKEN',
  INVALID_RESET_TOKEN: 'INVALID_RESET_TOKEN',
  INVALID_BOOTSTRAP_TOKEN: 'INVALID_BOOTSTRAP_TOKEN',
  FIRST_OWNER_EXISTS: 'FIRST_OWNER_EXISTS',
  UNAUTHORIZED: 'UNAUTHORIZED',
  FORBIDDEN: 'FORBIDDEN',
  // Lifecycle / gating
  SETUP_INCOMPLETE: 'SETUP_INCOMPLETE',
  // Soft-fail capability pattern (FRONTEND_ARCHITECTURE §4.3)
  AI_UNCONFIGURED: 'AI_UNCONFIGURED',
  // An admin turned a capability off (e.g. the `experience` chat kill-switch) — gating, not a fault.
  CAPABILITY_DISABLED: 'CAPABILITY_DISABLED',
  SERVICE_UNAVAILABLE: 'SERVICE_UNAVAILABLE',
  UPSTREAM_ERROR: 'UPSTREAM_ERROR',
  // Generic
  VALIDATION_ERROR: 'VALIDATION_ERROR',
  NOT_FOUND: 'NOT_FOUND',
  RATE_LIMITED: 'RATE_LIMITED',
  PAYLOAD_TOO_LARGE: 'PAYLOAD_TOO_LARGE',
  CONFLICT: 'CONFLICT',
  NOT_IMPLEMENTED: 'NOT_IMPLEMENTED',
  INTERNAL_ERROR: 'INTERNAL_ERROR',
  NETWORK_ERROR: 'NETWORK_ERROR',
  UNKNOWN: 'UNKNOWN',
} as const;

export type ErrorCode = (typeof ErrorCode)[keyof typeof ErrorCode];

export interface FieldError {
  field: string;
  reason: string;
}

/** Structured error thrown by the API client for any non-2xx response. */
export class ApiError extends Error {
  readonly code: ErrorCode | string;
  readonly status: number;
  readonly details: FieldError[];
  readonly requestId: string | undefined;

  constructor(args: {
    code: ErrorCode | string;
    message: string;
    status: number;
    details?: FieldError[];
    requestId?: string;
  }) {
    super(args.message);
    this.name = 'ApiError';
    this.code = args.code;
    this.status = args.status;
    this.details = args.details ?? [];
    this.requestId = args.requestId;
  }

  /** True for `AI_UNCONFIGURED` — soft notice + deep link, NOT an error toast. */
  get isAiUnconfigured(): boolean {
    return this.code === ErrorCode.AI_UNCONFIGURED;
  }

  /** True when the org has not finished onboarding → redirect to /setup. */
  get isSetupIncomplete(): boolean {
    return this.code === ErrorCode.SETUP_INCOMPLETE;
  }

  /**
   * Transient upstream/server failures that warrant a Retry affordance. A 501
   * `NOT_IMPLEMENTED` is excluded: the endpoint simply doesn't exist yet, so a
   * retry is pointless — the surface degrades to an empty/unavailable state
   * instead (cf. fb-activity-page).
   */
  get isTransient(): boolean {
    if (this.code === ErrorCode.AI_UNCONFIGURED || this.code === ErrorCode.NOT_IMPLEMENTED) {
      return false;
    }
    return (
      this.code === ErrorCode.SERVICE_UNAVAILABLE ||
      this.code === ErrorCode.UPSTREAM_ERROR ||
      this.code === ErrorCode.INTERNAL_ERROR ||
      this.status >= 500
    );
  }
}

/**
 * Maps an `ErrorCode` to user-facing copy (DSC §11.1). Field-level
 * `VALIDATION_ERROR` copy comes from `details[]`, not this table. `request_id`
 * is appended to 5xx copy by the surface that renders it.
 */
export function userMessageForCode(code: ErrorCode | string): string {
  switch (code) {
    case ErrorCode.INVALID_CREDENTIALS:
      // Security-uniform: never reveal which field was wrong (DSC §11.4).
      return 'Email or password is incorrect.';
    case ErrorCode.INVALID_BOOTSTRAP_TOKEN:
      return 'That setup token is invalid or expired.';
    case ErrorCode.INVALID_RESET_TOKEN:
      return 'That reset link is invalid or expired.';
    case ErrorCode.INVALID_REFRESH_TOKEN:
    case ErrorCode.UNAUTHORIZED:
      return 'Your session expired. Sign in again.';
    case ErrorCode.FORBIDDEN:
      return "You don't have access to this. Ask an owner or admin.";
    case ErrorCode.NOT_FOUND:
      return "We couldn't find that.";
    case ErrorCode.SETUP_INCOMPLETE:
      return 'Finish setup before using BuildOS.';
    case ErrorCode.RATE_LIMITED:
      return 'Too many requests. Try again in a moment.';
    case ErrorCode.AI_UNCONFIGURED:
      return 'AI features are turned off until an Anthropic API key is added.';
    case ErrorCode.CAPABILITY_DISABLED:
      return 'The AI assistant has been turned off by an admin.';
    case ErrorCode.PAYLOAD_TOO_LARGE:
      return 'That file is too large.';
    case ErrorCode.FIRST_OWNER_EXISTS:
      return 'An owner already exists for this deployment.';
    case ErrorCode.NETWORK_ERROR:
      return "Can't reach the server. Check your connection and try again.";
    case ErrorCode.SERVICE_UNAVAILABLE:
    case ErrorCode.UPSTREAM_ERROR:
    case ErrorCode.INTERNAL_ERROR:
    default:
      return 'Something went wrong on our end.';
  }
}
