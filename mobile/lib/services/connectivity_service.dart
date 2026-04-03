import 'dart:async';

import 'package:connectivity_plus/connectivity_plus.dart';
import 'package:flutter/foundation.dart';

/// Network state monitoring service.
///
/// Uses connectivity_plus to detect online/offline transitions.
/// Exposes a stream and notifies listeners when connectivity changes.
/// Triggers a callback when connectivity restores (for sync).
class ConnectivityService extends ChangeNotifier {
  final Connectivity _connectivity = Connectivity();
  StreamSubscription<List<ConnectivityResult>>? _subscription;

  bool _isOnline = true;
  VoidCallback? onConnectivityRestored;

  bool get isOnline => _isOnline;

  /// Initialize and start listening for connectivity changes.
  Future<void> init() async {
    // Check initial state.
    final result = await _connectivity.checkConnectivity();
    _isOnline = _hasConnection(result);
    notifyListeners();

    // Listen for changes.
    _subscription = _connectivity.onConnectivityChanged.listen(_onChanged);
  }

  void _onChanged(List<ConnectivityResult> results) {
    final wasOnline = _isOnline;
    _isOnline = _hasConnection(results);

    if (_isOnline != wasOnline) {
      notifyListeners();

      // Trigger sync when connectivity restores.
      if (_isOnline && !wasOnline && onConnectivityRestored != null) {
        onConnectivityRestored!();
      }
    }
  }

  bool _hasConnection(List<ConnectivityResult> results) {
    return results.any((r) =>
        r == ConnectivityResult.wifi ||
        r == ConnectivityResult.mobile ||
        r == ConnectivityResult.ethernet);
  }

  /// Stream of online/offline state changes.
  Stream<bool> get onlineStream {
    return _connectivity.onConnectivityChanged
        .map((results) => _hasConnection(results));
  }

  @override
  void dispose() {
    _subscription?.cancel();
    super.dispose();
  }
}
