import 'dart:async';

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';

/// FCM wake-ups for feed/notifications (Phase G5). The field app is
/// offline-first, so push is a *hint*, not a data channel: a message simply
/// nudges the app to drain the outbox and re-pull (server-wins).
///
/// This degrades gracefully. If the fork hasn't been provisioned with Firebase
/// config (no `google-services.json` / `GoogleService-Info.plist`), every call
/// is a safe no-op — the app still works, just without push wake-ups. There is
/// no backend device-token registration endpoint yet, so the resolved token is
/// surfaced via [onToken] for a future wiring step rather than POSTed.
class PushService {
  PushService._();

  static final PushService instance = PushService._();

  bool _initialized = false;
  bool _available = false;

  /// True once Firebase initialized successfully and messaging is usable.
  bool get isAvailable => _available;

  /// Initialize Firebase + messaging. [onWake] fires on a foreground message
  /// (and on a notification tap that opens the app) so the caller can sync.
  /// [onToken] receives the FCM registration token if one is issued.
  Future<void> init({
    required FutureOr<void> Function() onWake,
    void Function(String token)? onToken,
  }) async {
    if (_initialized) return;
    _initialized = true;
    try {
      await Firebase.initializeApp();
      final messaging = FirebaseMessaging.instance;
      await messaging.requestPermission();

      final token = await messaging.getToken();
      if (token != null) onToken?.call(token);
      messaging.onTokenRefresh.listen((t) => onToken?.call(t));

      FirebaseMessaging.onMessage.listen((_) => onWake());
      FirebaseMessaging.onMessageOpenedApp.listen((_) => onWake());

      _available = true;
    } catch (e) {
      // No Firebase config (or platform unsupported): run without push.
      _available = false;
      if (kDebugMode) {
        debugPrint('PushService disabled (Firebase not configured): $e');
      }
    }
  }
}
