import 'dart:async';
import 'dart:convert';

import 'package:drift/drift.dart';
import 'package:uuid/uuid.dart';

import '../database/app_database.dart';
import '../models/field_sync.dart';
import '../models/outbox_action.dart';
import 'api_client.dart';
import 'api_error.dart';
import 'connectivity_service.dart';

/// Owns the offline write path (Drift outbox) and the read path
/// (GET /field/sync). FRONTEND_ARCHITECTURE §8:
/// - Writes append to the outbox first, then drain FIFO with exponential
///   backoff. POSTs carry a UUID idempotency_key, so a server 409 means
///   "already accepted" and the item is marked synced (never retried forever).
/// - Reads pull `?since=<server_time>`; resolution is **server-wins** (the
///   client re-fetches after a successful drain and overwrites local rows).
/// - The drain runs through the same 401→refresh→retry interceptor; if refresh
///   fails offline, items stay queued (not dropped).
class SyncService {
  SyncService({
    required AppDatabase db,
    required ApiClient api,
    required ConnectivityService connectivity,
    Uuid? uuid,
  }) : _db = db,
       _api = api,
       _connectivity = connectivity,
       _uuid = uuid ?? const Uuid();

  final AppDatabase _db;
  final ApiClient _api;
  final ConnectivityService _connectivity;
  final Uuid _uuid;

  /// Max outbox attempts before an item is parked as failed.
  static const int maxRetries = 6;

  /// Backoff base; attempt N waits base * 2^N (capped).
  static const Duration backoffBase = Duration(seconds: 2);
  static const Duration backoffCap = Duration(minutes: 30);

  final _pending = StreamController<int>.broadcast();

  /// Live count of queued (pending) outbox items — drives [FbSyncChip].
  Stream<int> get pendingCount => _pending.stream;

  Future<void> _emitPending() async => _pending.add(await _db.pendingCount());

  // ---- Enqueue (offline-first writes) -----------------------------------

  Future<void> queueProgress({
    required String taskId,
    required int percentComplete,
    String? notes,
    double? gpsLat,
    double? gpsLng,
  }) {
    return _enqueue(OutboxAction.progress, {
      'task_id': taskId,
      'percent_complete': percentComplete,
      'notes': ?notes,
      'gps_lat': ?gpsLat,
      'gps_lng': ?gpsLng,
    });
  }

  Future<void> queueCheckin({
    required String projectId,
    List<Map<String, dynamic>> crewMembers = const [],
    double? gpsLat,
    double? gpsLng,
    String? notes,
  }) {
    return _enqueue(OutboxAction.checkin, {
      'project_id': projectId,
      'crew_members': crewMembers,
      'gps_lat': ?gpsLat,
      'gps_lng': ?gpsLng,
      'notes': ?notes,
    });
  }

  Future<void> queueDailyLog({
    required String projectId,
    required String logDate,
    required String workSummary,
    String? weatherConditions,
    String? safetyIncidents,
    List<String> photoAssetIds = const [],
  }) {
    return _enqueue(OutboxAction.dailyLog, {
      'project_id': projectId,
      'log_date': logDate,
      'work_summary': workSummary,
      'weather_conditions': ?weatherConditions,
      'safety_incidents': ?safetyIncidents,
      if (photoAssetIds.isNotEmpty) 'photo_asset_ids': photoAssetIds,
    });
  }

  Future<void> _enqueue(OutboxAction action, Map<String, dynamic> body) async {
    final key = _uuid.v4();
    body['idempotency_key'] = key;
    await _db.enqueue(
      OutboxItemsCompanion.insert(
        id: _uuid.v4(),
        action: action.wire,
        payload: jsonEncode(body),
        idempotencyKey: key,
      ),
    );
    await _emitPending();
  }

  // ---- Drain (FIFO + exponential backoff) -------------------------------

  /// Attempt to flush the outbox in FIFO order. Safe to call repeatedly.
  Future<void> drain() async {
    if (!await _connectivity.isOnline()) return;
    final now = DateTime.now().toUtc();
    final due = await _db.dueItems(now);
    for (final item in due) {
      await _attempt(item);
    }
    await _emitPending();
  }

  Future<void> _attempt(OutboxItem item) async {
    final action = OutboxAction.fromWire(item.action);
    try {
      await _api.postJson(
        action.path,
        body: jsonDecode(item.payload) as Map<String, dynamic>,
      );
      await _db.markSynced(item.id);
    } on ApiError catch (e) {
      // A replayed idempotent write is success from the outbox's view.
      if (e.isIdempotencyConflict) {
        await _db.markSynced(item.id);
        return;
      }
      // Validation rejection won't succeed on retry — park it.
      if (e.status >= 400 && e.status < 500 && !e.isSessionExpired) {
        await _db.markFailed(item.id, e.toString());
        return;
      }
      // Transient/network/session: keep queued with backoff.
      await _scheduleRetry(item, e.toString());
    } catch (e) {
      await _scheduleRetry(item, e.toString());
    }
  }

  Future<void> _scheduleRetry(OutboxItem item, String err) async {
    final next = item.retryCount + 1;
    if (next >= maxRetries) {
      await _db.markFailed(item.id, err);
      return;
    }
    await _db.markRetry(item.id, next, backoffFor(next), err);
  }

  /// Exponential backoff: base * 2^attempt, capped.
  DateTime backoffFor(int attempt) {
    final ms = backoffBase.inMilliseconds * (1 << attempt);
    final capped = ms > backoffCap.inMilliseconds
        ? backoffCap.inMilliseconds
        : ms;
    return DateTime.now().toUtc().add(Duration(milliseconds: capped));
  }

  // ---- Pull (server-wins refresh) ---------------------------------------

  /// GET `/api/v1/field/sync?since=` cursor. Overwrites the local task cache and
  /// advances the cursor to the server-authoritative `server_time`.
  Future<FieldSyncResponse> pull() async {
    final meta = await _db.loadSyncMeta();
    final json = await _api.getJson(
      '/api/v1/field/sync',
      query: {'since': ?meta?.sinceCursor},
    );
    final resp = FieldSyncResponse.fromJson(json);
    await _db.replaceTasks([
      for (final t in resp.tasks)
        CachedTasksCompanion.insert(
          id: t.id,
          projectId: t.projectId,
          wbsCode: t.wbsCode,
          name: t.name,
          durationDays: Value(t.durationDays),
          isCritical: Value(t.isCritical),
          status: Value(t.status),
          percentComplete: Value(t.percentComplete),
          totalFloat: Value(t.totalFloat),
          payload: jsonEncode({
            'id': t.id,
            'project_id': t.projectId,
            'wbs_code': t.wbsCode,
            'name': t.name,
            'duration_days': t.durationDays,
            'is_critical': t.isCritical,
            'status': t.status,
            'percent_complete': t.percentComplete,
            'total_float': t.totalFloat,
            'assigned_crew': t.assignedCrew,
          }),
        ),
    ]);
    // Equipment is a full-set collection — REPLACE (wipe-then-fill), so assets
    // that left the caller's sites disappear from the cache.
    await _db.replaceEquipment([
      for (final e in resp.equipment)
        CachedEquipmentCompanion.insert(
          id: e.id,
          name: e.name,
          assetType: e.assetType,
          status: Value(e.status),
          serialNumber: Value(e.serialNumber),
          startDate: e.startDate,
          endDate: e.endDate,
        ),
    ]);
    await _db.setSyncMeta(
      at: DateTime.now().toUtc(),
      cursor: resp.serverTime.toIso8601String(),
    );
    return resp;
  }

  /// Drain queued writes first, then re-pull so the client reflects the
  /// server's post-write truth (server-wins).
  Future<void> syncNow() async {
    await drain();
    if (await _connectivity.isOnline()) {
      await pull();
    }
  }

  void dispose() => _pending.close();
}
