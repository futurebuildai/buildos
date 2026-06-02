import 'package:buildos_field/database/app_database.dart';
import 'package:buildos_field/services/api_client.dart';
import 'package:buildos_field/services/api_error.dart';
import 'package:buildos_field/services/connectivity_service.dart';
import 'package:buildos_field/services/sync_service.dart';
import 'package:buildos_field/services/token_store.dart';
import 'package:dio/dio.dart';
import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';

/// Drives the outbox drain with a scripted API so each branch
/// (success / 409-already-accepted / 4xx-park / 5xx-retry) is verified without
/// a live backend.
class _ScriptedApi extends ApiClient {
  _ScriptedApi() : super(tokens: TokenStore());

  /// FIFO of responses: a Map is returned, an ApiError is thrown.
  final List<Object> script = [];
  int postCount = 0;

  @override
  Future<Map<String, dynamic>> postJson(
    String path, {
    Object? body,
    Options? options,
  }) async {
    postCount++;
    final next = script.isNotEmpty ? script.removeAt(0) : <String, dynamic>{};
    if (next is ApiError) throw next;
    return (next as Map).cast<String, dynamic>();
  }
}

class _FakeConnectivity implements ConnectivityService {
  _FakeConnectivity(this.online);
  bool online;
  @override
  Future<bool> isOnline() async => online;
  @override
  Stream<bool> get onChange => const Stream.empty();
}

void main() {
  late AppDatabase db;
  late _ScriptedApi api;
  late _FakeConnectivity conn;
  late SyncService sync;

  setUp(() {
    db = AppDatabase(NativeDatabase.memory());
    api = _ScriptedApi();
    conn = _FakeConnectivity(true);
    sync = SyncService(db: db, api: api, connectivity: conn);
  });

  tearDown(() async {
    sync.dispose();
    await db.close();
  });

  test('successful drain marks the item synced and clears the queue', () async {
    await sync.queueProgress(taskId: 't1', percentComplete: 50);
    expect(await db.pendingCount(), 1);

    api.script.add({'progress': {}});
    await sync.drain();

    expect(api.postCount, 1);
    expect(await db.pendingCount(), 0);
  });

  test(
    '409 conflict is treated as already-accepted (synced, not retried)',
    () async {
      await sync.queueProgress(taskId: 't1', percentComplete: 100);
      api.script.add(
        ApiError(code: ErrorCode.conflict, message: 'dup', status: 409),
      );

      await sync.drain();

      expect(await db.pendingCount(), 0);
    },
  );

  test('4xx validation rejection parks the item as failed', () async {
    await sync.queueCheckin(projectId: 'p1');
    api.script.add(
      ApiError(code: ErrorCode.validationError, message: 'bad', status: 422),
    );

    await sync.drain();

    // No longer pending, but never retried — it's parked.
    expect(await db.pendingCount(), 0);
    final due = await db.dueItems(
      DateTime.now().toUtc().add(const Duration(days: 1)),
    );
    expect(due, isEmpty);
  });

  test('5xx schedules a backoff retry and keeps the item queued', () async {
    await sync.queueProgress(taskId: 't1', percentComplete: 25);
    api.script.add(
      ApiError(
        code: ErrorCode.serviceUnavailable,
        message: 'down',
        status: 503,
      ),
    );

    await sync.drain();

    // Still pending, but gated behind nextAttemptAt (not immediately due).
    expect(await db.pendingCount(), 1);
    final dueNow = await db.dueItems(DateTime.now().toUtc());
    expect(dueNow, isEmpty);
  });

  test('offline drain is a no-op (writes stay queued)', () async {
    conn.online = false;
    await sync.queueProgress(taskId: 't1', percentComplete: 10);

    await sync.drain();

    expect(api.postCount, 0);
    expect(await db.pendingCount(), 1);
  });

  test('backoff grows exponentially and is capped', () {
    final first = sync.backoffFor(1);
    final later = sync.backoffFor(10);
    final now = DateTime.now().toUtc();
    expect(first.isAfter(now), isTrue);
    // 2s * 2^10 would exceed the 30-min cap.
    expect(
      later.difference(now).inMinutes,
      lessThanOrEqualTo(SyncService.backoffCap.inMinutes + 1),
    );
  });
}
