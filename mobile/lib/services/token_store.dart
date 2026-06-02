import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../models/user.dart';

/// Persists the token pair + cached user in the OS Keychain/Keystore
/// (FRONTEND_ARCHITECTURE §4.1). The refresh token is the durable credential:
/// the field app must survive long offline periods, so the short-lived access
/// token will expire and the refresh token is what keeps a cached session
/// usable once connectivity returns.
class TokenStore {
  TokenStore({FlutterSecureStorage? storage})
    : _storage = storage ?? const FlutterSecureStorage();

  final FlutterSecureStorage _storage;

  static const _kAccess = 'fb_access_token';
  static const _kRefresh = 'fb_refresh_token';
  static const _kUser = 'fb_user';
  static const _kAccessExpiry = 'fb_access_expiry';

  Future<void> save(TokenPair pair) async {
    final expiry = DateTime.now().toUtc().add(
      Duration(seconds: pair.expiresIn),
    );
    await Future.wait([
      _storage.write(key: _kAccess, value: pair.accessToken),
      _storage.write(key: _kRefresh, value: pair.refreshToken),
      _storage.write(key: _kUser, value: jsonEncode(pair.user.toJson())),
      _storage.write(key: _kAccessExpiry, value: expiry.toIso8601String()),
    ]);
  }

  /// Rotate-in a fresh access token after a refresh while keeping the (possibly
  /// rotated) refresh token in sync.
  Future<void> updateTokens({
    required String accessToken,
    required String refreshToken,
    required int expiresIn,
  }) async {
    final expiry = DateTime.now().toUtc().add(Duration(seconds: expiresIn));
    await Future.wait([
      _storage.write(key: _kAccess, value: accessToken),
      _storage.write(key: _kRefresh, value: refreshToken),
      _storage.write(key: _kAccessExpiry, value: expiry.toIso8601String()),
    ]);
  }

  Future<String?> get accessToken => _storage.read(key: _kAccess);
  Future<String?> get refreshToken => _storage.read(key: _kRefresh);

  Future<DateTime?> get accessExpiry async {
    final raw = await _storage.read(key: _kAccessExpiry);
    return raw == null ? null : DateTime.tryParse(raw);
  }

  Future<User?> get cachedUser async {
    final raw = await _storage.read(key: _kUser);
    if (raw == null) return null;
    try {
      return User.fromJson(jsonDecode(raw) as Map<String, dynamic>);
    } catch (_) {
      return null;
    }
  }

  /// True when a refresh token exists — the field app can launch into a cached
  /// (read-only) session offline as long as the durable credential is present.
  Future<bool> get hasSession async => (await refreshToken) != null;

  Future<void> clear() => _storage.deleteAll();
}
