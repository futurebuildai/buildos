@Tags(['live'])
library;

import 'dart:convert';
import 'dart:io';

import 'package:buildos_field/database/app_database.dart';
import 'package:buildos_field/models/outbox_action.dart';
import 'package:buildos_field/models/user.dart';
import 'package:buildos_field/services/api_client.dart';
import 'package:buildos_field/services/api_error.dart';
import 'package:buildos_field/services/connectivity_service.dart';
import 'package:buildos_field/services/sync_service.dart';
import 'package:buildos_field/services/token_store.dart';
import 'package:dio/dio.dart';
import 'package:drift/native.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:flutter_test/flutter_test.dart';

/// Live-backend integration test for the offline outbox against a REAL
/// `cmd/server` (native auth, vault on) booted by `scripts/e2e-backend.sh
/// --seed-field`. This is the only mobile lane that exercises the actual
/// claim → /field/progress contract — the unit suite (sync_service_test.dart)
/// scripts the API, so it can't catch wire-shape or idempotency drift.
///
/// The harness contract (exported env):
///   E2E_API_URL, E2E_BOOTSTRAP_TOKEN, E2E_OWNER_EMAIL, E2E_OWNER_PASSWORD,
///   E2E_TASK_ID (the seeded project_task to report progress against).
///
/// Run it:
///   scripts/e2e-backend.sh --db-up --seed-field -- \
///     flutter --no-version-check test test/live/sync_live_test.dart
///
/// When the env isn't present (a plain `flutter test`), the test self-skips so
/// the backend-free unit suite stays green offline.

/// Connectivity we flip by hand to simulate airplane mode → reconnect.
class _ToggleConnectivity implements ConnectivityService {
  _ToggleConnectivity(this.online);
  bool online;
  @override
  Future<bool> isOnline() async => online;
  @override
  Stream<bool> get onChange => const Stream.empty();
}

/// Restores REAL networking inside the test zone. `TestWidgetsFlutterBinding`
/// (needed for flutter_secure_storage's mock) installs an HttpOverrides that
/// short-circuits every request to a 400 with no socket. An empty subclass
/// inherits the default (real) `createHttpClient`, so wrapping the body in
/// [HttpOverrides.runWithHttpOverrides] lets dio actually reach the backend.
class _RealHttpOverrides extends HttpOverrides {}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  // Back flutter_secure_storage with an in-memory map so TokenStore works in
  // the Dart VM (no Keychain/Keystore under `flutter test`).
  FlutterSecureStorage.setMockInitialValues({});

  final env = Platform.environment;
  final bootstrapToken = env['E2E_BOOTSTRAP_TOKEN'];
  final taskId = env['E2E_TASK_ID'];
  final apiUrl = env['E2E_API_URL'] ?? 'http://localhost:8080';
  final ownerEmail = env['E2E_OWNER_EMAIL'] ?? 'owner@e2e.test';
  final ownerPassword =
      env['E2E_OWNER_PASSWORD'] ?? 'correct horse battery staple';

  final skip =
      (bootstrapToken == null ||
          bootstrapToken.isEmpty ||
          taskId == null ||
          taskId.isEmpty)
      ? 'live backend env not set — run via scripts/e2e-backend.sh --seed-field'
      : false;

  test(
    'offline → queue → reconnect → drain → idempotent replay (live)',
    () => HttpOverrides.runWithHttpOverrides(
      _body(
        apiUrl: apiUrl,
        bootstrapToken: bootstrapToken,
        taskId: taskId,
        ownerEmail: ownerEmail,
        ownerPassword: ownerPassword,
      ),
      _RealHttpOverrides(),
    ),
    skip: skip,
  );
}

Future<void> Function() _body({
  required String apiUrl,
  required String? bootstrapToken,
  required String? taskId,
  required String ownerEmail,
  required String ownerPassword,
}) {
  return () async {
    // The base URL is a compile-time const in production (kApiBaseUrl); for the
    // test we point dio at the harness-exported URL explicitly.
    final tokens = TokenStore();
    final api = ApiClient(
      tokens: tokens,
      dio: Dio(BaseOptions(baseUrl: apiUrl)),
      refreshDio: Dio(BaseOptions(baseUrl: apiUrl)),
    );

    // 1. Claim the first owner against the real native-auth surface. This is
    //    the unauthenticated bootstrap-token redemption; on success the backend
    //    returns a token pair we persist exactly as the app would.
    final claim = await api.postJson(
      '/api/v1/auth/claim',
      body: {
        'token': bootstrapToken,
        'email': ownerEmail,
        'password': ownerPassword,
        'display_name': 'E2E Owner',
      },
      options: skipAuth(),
    );
    final pair = TokenPair.fromJson(claim);
    expect(
      pair.accessToken,
      isNotEmpty,
      reason: 'claim returned no access token',
    );
    expect(pair.user.role, 'owner');
    await tokens.save(pair);

    final db = AppDatabase(NativeDatabase.memory());
    final conn = _ToggleConnectivity(false); // start offline (airplane mode)
    final sync = SyncService(db: db, api: api, connectivity: conn);

    addTearDown(() async {
      sync.dispose();
      await db.close();
    });

    // 2. Offline write: the progress report queues locally and a drain is a
    //    no-op (nothing leaves the device).
    await sync.queueProgress(taskId: taskId!, percentComplete: 40);
    expect(await db.pendingCount(), 1);
    await sync.drain();
    expect(await db.pendingCount(), 1, reason: 'offline drain must not flush');

    // Capture the queued item's wire payload + idempotency key BEFORE it drains
    // so we can replay the exact same request later.
    final queued = await db.dueItems(
      DateTime.now().toUtc().add(const Duration(days: 1)),
    );
    expect(queued, hasLength(1));
    final original = queued.first;

    // 3. Reconnect and drain: the real POST /api/v1/field/progress lands a 201,
    //    so the item is marked synced and the queue empties.
    conn.online = true;
    await sync.drain();
    expect(
      await db.pendingCount(),
      0,
      reason: 'online drain should accept the progress report',
    );

    // 4. Server-side idempotency: replay the EXACT same payload (same
    //    idempotency_key) directly against the API. The backend must recognize
    //    the duplicate and return 409 CONFLICT. (The outbox's own
    //    409→mark-synced handling is covered by the unit suite; the local
    //    outbox enforces a UNIQUE idempotency_key so it can't re-enqueue the
    //    same write — this asserts the SERVER dedup the outbox relies on.)
    final replayBody = jsonDecode(original.payload) as Map<String, dynamic>;
    ApiError? conflict;
    try {
      await api.postJson(OutboxAction.progress.path, body: replayBody);
    } on ApiError catch (e) {
      conflict = e;
    }
    expect(
      conflict,
      isNotNull,
      reason: 'replaying the same idempotency_key should be rejected',
    );
    expect(
      conflict!.isIdempotencyConflict,
      isTrue,
      reason: 'duplicate write should surface as 409 CONFLICT',
    );

    // 5. Read path: pull() exercises the authenticated GET /api/v1/field/sync
    //    contract end to end (SetupGate pass, response parse, cursor advance).
    //    The seeded task isn't crew-assigned to this owner, so we assert the
    //    contract works rather than a specific task — the write path above is
    //    the integrity guarantee this lane exists for.
    final resp = await sync.pull();
    expect(resp.serverTime, isNotNull);
    final meta = await db.loadSyncMeta();
    expect(
      meta?.sinceCursor,
      isNotNull,
      reason: 'pull should advance the cursor',
    );
  };
}
