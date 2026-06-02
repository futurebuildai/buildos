import 'package:connectivity_plus/connectivity_plus.dart';

/// Thin connectivity wrapper so the sync layer can be unit-tested with a fake.
abstract class ConnectivityService {
  Future<bool> isOnline();

  /// Emits true when connectivity is (re)gained, false when lost.
  Stream<bool> get onChange;
}

class ConnectivityPlusService implements ConnectivityService {
  ConnectivityPlusService([Connectivity? connectivity])
    : _connectivity = connectivity ?? Connectivity();

  final Connectivity _connectivity;

  static bool _online(List<ConnectivityResult> results) =>
      results.any((r) => r != ConnectivityResult.none);

  @override
  Future<bool> isOnline() async =>
      _online(await _connectivity.checkConnectivity());

  @override
  Stream<bool> get onChange => _connectivity.onConnectivityChanged.map(_online);
}
