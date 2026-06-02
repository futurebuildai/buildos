import 'package:dio/dio.dart';

import 'api_error.dart';
import 'token_store.dart';

/// Base URL is injected at build time:
///   flutter run --dart-define=API_BASE_URL=https://acme.buildos.example
const String kApiBaseUrl = String.fromEnvironment(
  'API_BASE_URL',
  defaultValue: 'http://localhost:8080',
);

/// Marks a request as part of the unauthenticated surface (login / refresh /
/// password-reset). These never attach a bearer token and never trigger the
/// 401→refresh interceptor (which would recurse).
const _skipAuthKey = 'fb_skip_auth';
Options skipAuth() => Options(extra: {_skipAuthKey: true});

/// Typed HTTP client wrapping dio. Implements the single-flight
/// 401→refresh→retry interceptor shared by both surfaces
/// (FRONTEND_ARCHITECTURE §4.2): on a 401, refresh ONCE (concurrent requests
/// share one refresh future), then retry the original request a single time.
/// A second 401 → session cleared + [onSessionExpired] fired.
class ApiClient {
  ApiClient({required TokenStore tokens, Dio? dio, Dio? refreshDio})
    : _tokens = tokens,
      _dio = dio ?? Dio(BaseOptions(baseUrl: kApiBaseUrl)),
      _refreshDio = refreshDio ?? Dio(BaseOptions(baseUrl: kApiBaseUrl)) {
    _dio.interceptors.add(
      InterceptorsWrapper(onRequest: _onRequest, onError: _onError),
    );
  }

  final TokenStore _tokens;
  final Dio _dio;
  final Dio _refreshDio;

  /// Fired when refresh fails — the app routes back to /login.
  void Function()? onSessionExpired;

  /// Single-flight guard: concurrent 401s await the same refresh.
  Future<bool>? _refreshing;

  Future<void> _onRequest(
    RequestOptions options,
    RequestInterceptorHandler handler,
  ) async {
    if (options.extra[_skipAuthKey] != true) {
      final token = await _tokens.accessToken;
      if (token != null) {
        options.headers['Authorization'] = 'Bearer $token';
      }
    }
    handler.next(options);
  }

  Future<void> _onError(
    DioException err,
    ErrorInterceptorHandler handler,
  ) async {
    final req = err.requestOptions;
    final is401 = err.response?.statusCode == 401;
    final alreadyRetried = req.extra['fb_retried'] == true;
    final skipAuthReq = req.extra[_skipAuthKey] == true;

    if (!is401 || alreadyRetried || skipAuthReq) {
      handler.next(err);
      return;
    }

    final ok = await _refreshOnce();
    if (!ok) {
      await _tokens.clear();
      onSessionExpired?.call();
      handler.next(err);
      return;
    }

    // Retry the original request exactly once with the rotated access token.
    try {
      final token = await _tokens.accessToken;
      final retried = req
        ..extra['fb_retried'] = true
        ..headers['Authorization'] = 'Bearer $token';
      final response = await _dio.fetch<dynamic>(retried);
      handler.resolve(response);
    } on DioException catch (e) {
      handler.next(e);
    }
  }

  /// Refresh the access token, sharing one in-flight call across concurrent
  /// 401s. Returns true on success. Rotates the stored refresh token.
  Future<bool> _refreshOnce() {
    return _refreshing ??= _doRefresh().whenComplete(() => _refreshing = null);
  }

  Future<bool> _doRefresh() async {
    final refresh = await _tokens.refreshToken;
    if (refresh == null) return false;
    try {
      final res = await _refreshDio.post<Map<String, dynamic>>(
        '/api/v1/auth/refresh',
        data: {'refresh_token': refresh},
      );
      final body = res.data ?? const {};
      final access = body['access_token'] as String?;
      final newRefresh = body['refresh_token'] as String? ?? refresh;
      final expiresIn = (body['expires_in'] as num?)?.toInt() ?? 0;
      if (access == null) return false;
      await _tokens.updateTokens(
        accessToken: access,
        refreshToken: newRefresh,
        expiresIn: expiresIn,
      );
      return true;
    } on DioException {
      return false;
    }
  }

  Future<Map<String, dynamic>> getJson(
    String path, {
    Map<String, dynamic>? query,
    Options? options,
  }) async {
    return _unwrap(
      () => _dio.get<dynamic>(path, queryParameters: query, options: options),
    );
  }

  Future<Map<String, dynamic>> postJson(
    String path, {
    Object? body,
    Options? options,
  }) async {
    return _unwrap(
      () => _dio.post<dynamic>(path, data: body, options: options),
    );
  }

  Future<Map<String, dynamic>> _unwrap(
    Future<Response<dynamic>> Function() call,
  ) async {
    try {
      final res = await call();
      // Every backend response is the standard envelope
      // `{ data, error, meta }` (internal/api/response.go) — the payload lives
      // under `data`, mirroring the web client (src/api/client.ts). Return the
      // inner object so callers (TokenPair.fromJson, FieldSyncResponse.fromJson)
      // see the payload directly, not the envelope.
      final body = res.data;
      if (body is Map<String, dynamic>) {
        final inner = body['data'];
        if (inner is Map<String, dynamic>) return inner;
      }
      return const {};
    } on DioException catch (e) {
      throw _toApiError(e);
    }
  }

  ApiError _toApiError(DioException e) {
    final res = e.response;
    final status = res?.statusCode ?? 0;
    if (res?.data is Map) {
      final envelope = (res!.data as Map)['error'];
      if (envelope is Map) {
        return ApiError(
          code: envelope['code'] as String? ?? ErrorCode.unknown,
          message: envelope['message'] as String? ?? 'Request failed',
          status: status,
        );
      }
    }
    if (status == 0) {
      return ApiError(
        code: ErrorCode.networkError,
        message: "Can't reach the server.",
        status: 0,
      );
    }
    return ApiError(
      code: ErrorCode.unknown,
      message: 'Request failed',
      status: status,
    );
  }
}
