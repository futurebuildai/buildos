import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../database/app_database.dart';
import '../models/equipment_asset.dart';
import '../models/project_task.dart';
import '../models/user.dart';
import '../services/api_client.dart';
import '../services/api_error.dart';
import '../services/auth_service.dart';
import '../services/connectivity_service.dart';
import '../services/sync_service.dart';
import '../services/token_store.dart';

/// Riverpod wiring for the field app. Providers compose the offline-first
/// service graph (token store → api client → sync/auth) and expose the live
/// streams the UI binds to (pending-outbox count, connectivity).

// ---- Singletons / DI ----------------------------------------------------

final databaseProvider = Provider<AppDatabase>((ref) {
  final db = AppDatabase();
  ref.onDispose(db.close);
  return db;
});

final tokenStoreProvider = Provider<TokenStore>((ref) => TokenStore());

final apiClientProvider = Provider<ApiClient>((ref) {
  final client = ApiClient(tokens: ref.watch(tokenStoreProvider));
  // A failed refresh drops us back to the cached-out (logged-out) state, which
  // the router redirect turns into a trip to /login.
  client.onSessionExpired = () =>
      ref.read(authControllerProvider.notifier).onSessionExpired();
  return client;
});

final authServiceProvider = Provider<AuthService>(
  (ref) => AuthService(
    api: ref.watch(apiClientProvider),
    tokens: ref.watch(tokenStoreProvider),
  ),
);

final connectivityServiceProvider = Provider<ConnectivityService>(
  (ref) => ConnectivityPlusService(),
);

final syncServiceProvider = Provider<SyncService>((ref) {
  final sync = SyncService(
    db: ref.watch(databaseProvider),
    api: ref.watch(apiClientProvider),
    connectivity: ref.watch(connectivityServiceProvider),
  );
  ref.onDispose(sync.dispose);
  return sync;
});

// ---- Live streams -------------------------------------------------------

/// Current connectivity, seeded with a one-shot probe then kept live.
final onlineProvider = StreamProvider<bool>((ref) async* {
  final connectivity = ref.watch(connectivityServiceProvider);
  yield await connectivity.isOnline();
  yield* connectivity.onChange;
});

/// Queued (pending) outbox count — drives [FbSyncChip]. Seeded from the DB so
/// the chip is correct on cold start before any enqueue/drain fires.
final pendingCountProvider = StreamProvider<int>((ref) async* {
  final db = ref.watch(databaseProvider);
  final sync = ref.watch(syncServiceProvider);
  yield await db.pendingCount();
  yield* sync.pendingCount;
});

// ---- Auth ---------------------------------------------------------------

final authControllerProvider = AsyncNotifierProvider<AuthController, User?>(
  AuthController.new,
);

/// Owns the session lifecycle. `build` resolves the cached user so a returning
/// field worker lands straight in the app (offline-friendly).
class AuthController extends AsyncNotifier<User?> {
  @override
  Future<User?> build() => ref.read(authServiceProvider).cachedUser();

  Future<void> login(String email, String password) async {
    state = const AsyncValue.loading();
    state = await AsyncValue.guard(
      () => ref.read(authServiceProvider).login(email, password),
    );
  }

  Future<void> logout() async {
    await ref.read(authServiceProvider).logout();
    state = const AsyncValue.data(null);
  }

  /// Refresh failed (refresh token rejected/expired) — force logged-out.
  void onSessionExpired() => state = const AsyncValue.data(null);
}

// ---- Task list (server-wins cache) --------------------------------------

/// Reads the cached task list and sorts it for the field: critical path first,
/// then near-critical, then by ascending float (most urgent slack first).
final tasksProvider = FutureProvider<List<ProjectTask>>((ref) async {
  final db = ref.watch(databaseProvider);
  final rows = await db.allCachedTasks();
  final tasks = [
    for (final r in rows)
      ProjectTask.fromJson(jsonDecode(r.payload) as Map<String, dynamic>),
  ];
  tasks.sort((a, b) {
    int rank(ProjectTask t) => t.isCritical
        ? 0
        : t.isNearCritical
        ? 1
        : 2;
    final byRank = rank(a).compareTo(rank(b));
    if (byRank != 0) return byRank;
    final fa = a.totalFloat ?? 1 << 30;
    final fb = b.totalFloat ?? 1 << 30;
    final byFloat = fa.compareTo(fb);
    if (byFloat != 0) return byFloat;
    return a.wbsCode.compareTo(b.wbsCode);
  });
  return tasks;
});

// ---- Equipment list (server-wins full-replace cache) --------------------

/// Reads the cached equipment allocated to the caller's active sites
/// (Phase 4a-ii, read-only), sorted by name.
final equipmentProvider = FutureProvider<List<EquipmentAsset>>((ref) async {
  final db = ref.watch(databaseProvider);
  final rows = await db.allCachedEquipment();
  final list = [
    for (final r in rows)
      EquipmentAsset(
        id: r.id,
        name: r.name,
        assetType: r.assetType,
        status: r.status,
        serialNumber: r.serialNumber,
        startDate: r.startDate,
        endDate: r.endDate,
      ),
  ];
  list.sort((a, b) => a.name.compareTo(b.name));
  return list;
});

// ---- Sync orchestration -------------------------------------------------

class SyncUiState {
  const SyncUiState({this.syncing = false, this.error, this.lastSyncedAt});

  final bool syncing;
  final String? error;
  final DateTime? lastSyncedAt;

  SyncUiState copyWith({
    bool? syncing,
    String? error,
    DateTime? lastSyncedAt,
  }) => SyncUiState(
    syncing: syncing ?? this.syncing,
    error: error,
    lastSyncedAt: lastSyncedAt ?? this.lastSyncedAt,
  );
}

final syncControllerProvider = NotifierProvider<SyncController, SyncUiState>(
  SyncController.new,
);

class SyncController extends Notifier<SyncUiState> {
  @override
  SyncUiState build() => const SyncUiState();

  /// Drain the outbox then re-pull (server-wins). Invalidates the task cache so
  /// the UI reflects the server's post-write truth.
  Future<void> syncNow() async {
    if (state.syncing) return;
    state = state.copyWith(syncing: true, error: null);
    try {
      await ref.read(syncServiceProvider).syncNow();
      ref.invalidate(tasksProvider);
      ref.invalidate(equipmentProvider);
      final meta = await ref.read(databaseProvider).loadSyncMeta();
      state = SyncUiState(syncing: false, lastSyncedAt: meta?.lastSyncedAt);
    } on ApiError catch (e) {
      state = state.copyWith(syncing: false, error: e.message);
    } catch (_) {
      state = state.copyWith(syncing: false, error: 'Sync failed');
    }
  }
}
