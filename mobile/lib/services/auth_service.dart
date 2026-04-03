import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

/// JWT token management service for FB-Brain OIDC authentication.
///
/// Stores and retrieves the JWT token from SharedPreferences.
/// Provides a simple integration point for the login flow.
class AuthService extends ChangeNotifier {
  static const String _tokenKey = 'fb_auth_token';

  SharedPreferences? _prefs;
  String? _cachedToken;

  /// Initialize the service by loading any stored token.
  Future<void> init() async {
    _prefs = await SharedPreferences.getInstance();
    _cachedToken = _prefs?.getString(_tokenKey);
    notifyListeners();
  }

  /// Store a JWT token after successful OIDC login.
  Future<void> storeToken(String token) async {
    _prefs ??= await SharedPreferences.getInstance();
    await _prefs!.setString(_tokenKey, token);
    _cachedToken = token;
    notifyListeners();
  }

  /// Get the current JWT token, or null if not logged in.
  String? getToken() {
    return _cachedToken;
  }

  /// Clear the stored token (logout).
  Future<void> clearToken() async {
    _prefs ??= await SharedPreferences.getInstance();
    await _prefs!.remove(_tokenKey);
    _cachedToken = null;
    notifyListeners();
  }

  /// Whether the user has a stored JWT token.
  bool get isLoggedIn => _cachedToken != null && _cachedToken!.isNotEmpty;
}
