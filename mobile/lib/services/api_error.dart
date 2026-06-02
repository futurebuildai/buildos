/// Canonical machine error codes (mirror internal/api/response.go + the web
/// client's src/api/errors.ts). The client branches on `code`, not raw status.
class ErrorCode {
  ErrorCode._();

  static const invalidCredentials = 'INVALID_CREDENTIALS';
  static const invalidRefreshToken = 'INVALID_REFRESH_TOKEN';
  static const unauthorized = 'UNAUTHORIZED';
  static const forbidden = 'FORBIDDEN';
  static const setupIncomplete = 'SETUP_INCOMPLETE';
  static const aiUnconfigured = 'AI_UNCONFIGURED';
  static const serviceUnavailable = 'SERVICE_UNAVAILABLE';
  static const validationError = 'VALIDATION_ERROR';
  static const notFound = 'NOT_FOUND';
  static const conflict = 'CONFLICT';
  static const rateLimited = 'RATE_LIMITED';
  static const internalError = 'INTERNAL_ERROR';
  static const networkError = 'NETWORK_ERROR';
  static const unknown = 'UNKNOWN';
}

/// Structured error surfaced by [ApiClient] for any non-2xx (or transport)
/// failure. The backend envelope is `{ error: { code, message, details } }`.
class ApiError implements Exception {
  ApiError({required this.code, required this.message, required this.status});

  final String code;
  final String message;
  final int status;

  /// A replayed idempotent write — the outbox treats this as "already
  /// accepted" and marks the item synced rather than retrying forever.
  bool get isIdempotencyConflict => code == ErrorCode.conflict;

  /// Session is gone — caller should clear tokens and route to login.
  bool get isSessionExpired =>
      status == 401 ||
      code == ErrorCode.unauthorized ||
      code == ErrorCode.invalidRefreshToken;

  /// Worth a retry affordance (transient upstream/server failure).
  bool get isTransient =>
      code == ErrorCode.serviceUnavailable ||
      code == ErrorCode.internalError ||
      code == ErrorCode.networkError ||
      status >= 500;

  @override
  String toString() => 'ApiError($status $code): $message';
}
