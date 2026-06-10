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

  /// FIFO of POST responses: a Map is returned, any Exception is thrown.
  final List<Object> script = [];
  int postCount = 0;

  /// FIFO of GET responses for the pull path (server-wins refresh).
  final List<Object> getScript = [];
  int getCount = 0;

  /// The decoded body of the most recent POST — lets tests assert the
  /// optional fields the queue helpers fold into the payload.
  Object? lastPostBody;

  @override
  Future<Map<String, dynamic>> postJson(
    String path, {
    Object? body,
    Options? options,
  }) async {
    postCount++;
    lastPostBody = body;
    final next = script.isNotEmpty ? script.removeAt(0) : <String, dynamic>{};
    // ApiError implements Exception; a plain Exception drives the
    // non-ApiError catch leg in SyncService._attempt.
    if (next is Exception) throw next;
    return (next as Map).cast<String, dynamic>();
  }

  @override
  Future<Map<String, dynamic>> getJson(
    String path, {
    Map<String, dynamic>? query,
    Options? options,
  }) async {
    getCount++;
    final next = getScript.isNotEmpty
        ? getScript.removeAt(0)
        : <String, dynamic>{};
    if (next is Exception) throw next;
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

  test('pendingCount stream emits the queued total after an enqueue', () async {
    final seen = <int>[];
    final sub = sync.pendingCount.listen(seen.add);
    addTearDown(sub.cancel);

    await sync.queueProgress(taskId: 't1', percentComplete: 10);
    await sync.queueProgress(taskId: 't2', percentComplete: 20);
    // Let the broadcast controller flush its microtasks.
    await Future<void>.delayed(Duration.zero);

    expect(seen, isNotEmpty);
    expect(seen.last, 2);
  });

  test('queueProgress folds optional notes + GPS into the payload', () async {
    await sync.queueProgress(
      taskId: 't1',
      percentComplete: 75,
      notes: 'poured footings',
      gpsLat: 40.1,
      gpsLng: -74.2,
    );
    api.script.add({'progress': {}});
    await sync.drain();

    final body = api.lastPostBody as Map<String, dynamic>;
    expect(body['task_id'], 't1');
    expect(body['percent_complete'], 75);
    expect(body['notes'], 'poured footings');
    expect(body['gps_lat'], 40.1);
    expect(body['gps_lng'], -74.2);
    expect(body['idempotency_key'], isNotEmpty);
  });

  test('queueCheckin folds optional GPS + notes into the payload', () async {
    await sync.queueCheckin(
      projectId: 'p1',
      crewMembers: [
        {'name': 'Sam'},
      ],
      gpsLat: 1.5,
      gpsLng: 2.5,
      notes: 'on site',
    );
    api.script.add({'checkin': {}});
    await sync.drain();

    final body = api.lastPostBody as Map<String, dynamic>;
    expect(body['project_id'], 'p1');
    expect(body['crew_members'], isA<List<dynamic>>());
    expect(body['gps_lat'], 1.5);
    expect(body['gps_lng'], 2.5);
    expect(body['notes'], 'on site');
  });

  test('queueDailyLog enqueues with weather, safety, and photos', () async {
    await sync.queueDailyLog(
      projectId: 'p1',
      logDate: '2026-06-04',
      workSummary: 'framed level 2',
      weatherConditions: 'clear',
      safetyIncidents: 'none',
      photoAssetIds: ['a1', 'a2'],
    );
    expect(await db.pendingCount(), 1);

    api.script.add({'daily_log': {}});
    await sync.drain();

    final body = api.lastPostBody as Map<String, dynamic>;
    expect(body['project_id'], 'p1');
    expect(body['log_date'], '2026-06-04');
    expect(body['work_summary'], 'framed level 2');
    expect(body['weather_conditions'], 'clear');
    expect(body['safety_incidents'], 'none');
    expect(body['photo_asset_ids'], ['a1', 'a2']);
    expect(await db.pendingCount(), 0);
  });

  test('a non-ApiError thrown during drain schedules a retry', () async {
    await sync.queueProgress(taskId: 't1', percentComplete: 30);
    // A plain Exception (e.g. an unexpected transport failure) hits the
    // bare catch in _attempt, not the typed ApiError branch.
    api.script.add(Exception('socket closed'));

    await sync.drain();

    // Kept queued (pending), gated behind backoff — not parked.
    expect(await db.pendingCount(), 1);
    final dueNow = await db.dueItems(DateTime.now().toUtc());
    expect(dueNow, isEmpty);
  });

  test('an item at the retry ceiling is parked as failed', () async {
    await sync.queueProgress(taskId: 't1', percentComplete: 40);
    final item = (await db.dueItems(DateTime.now().toUtc())).single;
    // Drive it to one short of the ceiling, due now, so the next failure
    // crosses maxRetries and parks it.
    await db.markRetry(
      item.id,
      SyncService.maxRetries - 1,
      DateTime.now().toUtc().subtract(const Duration(minutes: 1)),
      'prior failure',
    );

    api.script.add(
      ApiError(
        code: ErrorCode.serviceUnavailable,
        message: 'down',
        status: 503,
      ),
    );
    await sync.drain();

    // Parked: no longer pending and never due again.
    expect(await db.pendingCount(), 0);
    final due = await db.dueItems(
      DateTime.now().toUtc().add(const Duration(days: 365)),
    );
    expect(due, isEmpty);
  });

  test('pull caches server tasks and advances the since cursor', () async {
    api.getScript.add({
      'tasks': [
        {
          'id': 'task-1',
          'project_id': 'proj-1',
          'wbs_code': '01-100',
          'name': 'Site prep',
          'duration_days': 5,
          'is_critical': true,
          'status': 'in_progress',
          'percent_complete': 20,
          'total_float': 0,
          'assigned_crew': ['u1'],
        },
      ],
      'feed_cards': <dynamic>[],
      'server_time': '2026-06-04T12:00:00Z',
    });

    final resp = await sync.pull();

    expect(api.getCount, 1);
    expect(resp.tasks.single.id, 'task-1');

    final cached = await db.allCachedTasks();
    expect(cached.single.id, 'task-1');
    expect(cached.single.wbsCode, '01-100');
    expect(cached.single.isCritical, isTrue);

    final meta = await db.loadSyncMeta();
    expect(meta?.sinceCursor, '2026-06-04T12:00:00.000Z');
  });

  test(
    'pull full-replaces the equipment cache (drops what left my site)',
    () async {
      Map<String, dynamic> body(List<Map<String, dynamic>> equip) => {
        'tasks': <dynamic>[],
        'feed_cards': <dynamic>[],
        'equipment': equip,
        'server_time': '2026-06-04T12:00:00Z',
      };
      final excavator = {
        'id': 'eq-1',
        'name': 'Excavator',
        'asset_type': 'excavator',
        'status': 'available',
        'serial_number': 'SN-1',
        'start_date': '2026-06-01T00:00:00Z',
        'end_date': '2026-06-30T00:00:00Z',
      };
      final crane = {
        'id': 'eq-2',
        'name': 'Crane',
        'asset_type': 'crane',
        'status': 'maintenance',
        'start_date': '2026-06-01T00:00:00Z',
        'end_date': '2026-06-30T00:00:00Z',
      };

      api.getScript.add(body([excavator, crane]));
      await sync.pull();
      expect((await db.allCachedEquipment()).length, 2);

      // Next sync returns only the excavator — the crane left, so it must be gone
      // from the cache (full-replace, not upsert).
      api.getScript.add(body([excavator]));
      await sync.pull();
      final cached = await db.allCachedEquipment();
      expect(cached.length, 1);
      expect(cached.single.id, 'eq-1');

      // A malformed/partial 200 that OMITS the equipment key must NOT wipe the
      // cache (only an explicit empty array clears it).
      api.getScript.add({
        'tasks': <dynamic>[],
        'feed_cards': <dynamic>[],
        'server_time': '2026-06-04T12:00:00Z',
      });
      await sync.pull();
      expect((await db.allCachedEquipment()).length, 1); // unchanged
    },
  );

  test('syncNow drains the outbox then pulls server truth', () async {
    await sync.queueProgress(taskId: 't1', percentComplete: 60);
    api.script.add({'progress': {}}); // drain succeeds
    api.getScript.add({
      'tasks': [
        {
          'id': 'task-9',
          'project_id': 'proj-1',
          'wbs_code': '02-200',
          'name': 'Foundation',
          'duration_days': 3,
          'is_critical': false,
          'status': 'pending',
          'percent_complete': 0,
        },
      ],
      'feed_cards': <dynamic>[],
      'server_time': '2026-06-04T13:00:00Z',
    });

    await sync.syncNow();

    // Drained...
    expect(api.postCount, 1);
    expect(await db.pendingCount(), 0);
    // ...then pulled.
    expect(api.getCount, 1);
    expect((await db.allCachedTasks()).single.id, 'task-9');
  });

  test('syncNow skips the pull when offline after draining', () async {
    conn.online = false;
    await sync.queueProgress(taskId: 't1', percentComplete: 5);

    await sync.syncNow();

    // Offline: drain is a no-op and pull is skipped entirely.
    expect(api.postCount, 0);
    expect(api.getCount, 0);
    expect(await db.pendingCount(), 1);
  });
}
