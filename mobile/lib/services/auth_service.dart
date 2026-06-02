import '../models/user.dart';
import 'api_client.dart';
import 'token_store.dart';

/// Native email/password auth (no OIDC) against /api/v1/auth — mirrors the web
/// console contract (internal/api/auth.go MountAuthRoutes). Tokens land in the
/// secure store; the dio interceptor in [ApiClient] owns refresh-on-401.
class AuthService {
  AuthService({required ApiClient api, required TokenStore tokens})
    : _api = api,
      _tokens = tokens;

  final ApiClient _api;
  final TokenStore _tokens;

  /// POST /api/v1/auth/login. Persists the token pair + cached user.
  Future<User> login(String email, String password) async {
    final json = await _api.postJson(
      '/api/v1/auth/login',
      body: {'email': email, 'password': password},
      options: skipAuth(),
    );
    final pair = TokenPair.fromJson(json);
    await _tokens.save(pair);
    return pair.user;
  }

  /// POST /api/v1/auth/logout — best-effort server revoke, then clear locally.
  Future<void> logout() async {
    final refresh = await _tokens.refreshToken;
    if (refresh != null) {
      try {
        await _api.postJson(
          '/api/v1/auth/logout',
          body: {'refresh_token': refresh},
          options: skipAuth(),
        );
      } catch (_) {
        // Offline or already-revoked: clearing locally is enough.
      }
    }
    await _tokens.clear();
  }

  /// The cached user for an offline launch (read-only session).
  Future<User?> cachedUser() => _tokens.cachedUser;

  Future<bool> hasSession() => _tokens.hasSession;
}
