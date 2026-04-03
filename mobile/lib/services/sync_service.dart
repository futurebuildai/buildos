import 'dart:convert';
import 'dart:math';

import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';

import '../database/database.dart';
import 'api_client.dart';

/// Sync orchestrator for the offline-first field portal.
///
/// Handles:
/// - Pull sync: GET /field/sync?since={lastSync}, update local Drift tables
/// - Push outbox: drain FIFO, POST per action_type with idempotency keys
/// - Retry with exponential backoff: 1s, 2s, 4s, 8s, max 5min
class SyncService extends ChangeNotifier {
  static const String _lastSyncKey = 'fb_last_sync_timestamp';
  static const int _maxBackoffSeconds = 300; // 5 minutes

  final AppDatabase _db;
  final ApiClient _apiClient;

  bool _isSyncing = false;
  String? _lastSyncTimestamp;
  String? _lastError;

  SyncService({
    required AppDatabase db,
    required ApiClient apiClient,
  })  : _db = db,
        _apiClient = apiClient;

  bool get isSyncing => _isSyncing;
  String? get lastSyncTimestamp => _lastSyncTimestamp;
  String? get lastError => _lastError;

  /// Initialize by loading last sync timestamp from SharedPreferences.
  Future<void> init() async {
    final prefs = await SharedPreferences.getInstance();
    _lastSyncTimestamp = prefs.getString(_lastSyncKey);
    notifyListeners();
  }

  /// Full sync: pull from server, then push outbox.
  Future<void> fullSync() async {
    if (_isSyncing) return;
    _isSyncing = true;
    _lastError = null;
    notifyListeners();

    try {
      await pullSync();
      await pushOutbox();
    } catch (e) {
      _lastError = e.toString();
    } finally {
      _isSyncing = false;
      notifyListeners();
    }
  }

  /// Pull sync: GET /api/v1/field/sync?since={lastSync}
  ///
  /// Updates local Drift tasks table with server data.
  Future<void> pullSync() async {
    final response = await _apiClient.syncField(since: _lastSyncTimestamp);

    if (response.isSuccess && response.body != null) {
      final syncData = SyncResponse.fromJson(response.body!);

      // Upsert each task into the local database.
      for (final task in syncData.tasks) {
        await _db.upsertTask(
          id: task['id'] as String? ?? '',
          projectId: task['project_id'] as String? ?? '',
          wbsCode: task['wbs_code'] as String? ?? '',
          name: task['name'] as String? ?? '',
          percentComplete: task['percent_complete'] as int? ?? 0,
          priority: task['priority'] as String? ?? 'normal',
          status: task['status'] as String? ?? 'pending',
          scheduledDate: task['scheduled_date'] as String?,
          lastSyncedAt: DateTime.now().toUtc().toIso8601String(),
        );
      }

      // Update last sync timestamp.
      if (syncData.serverTimestamp.isNotEmpty) {
        _lastSyncTimestamp = syncData.serverTimestamp;
        final prefs = await SharedPreferences.getInstance();
        await prefs.setString(_lastSyncKey, _lastSyncTimestamp!);
      }
    } else if (response.error != null) {
      _lastError = 'Pull sync failed: ${response.error}';
    }
  }

  /// Push outbox: drain pending entries in FIFO order.
  ///
  /// For each entry:
  /// - 201: delete from outbox (success)
  /// - 409: mark synced (duplicate, already processed)
  /// - 5xx: increment retry_count, schedule retry with backoff
  Future<void> pushOutbox() async {
    final entries = await _db.getPendingOutboxEntries();

    for (final entry in entries) {
      final id = entry['id'] as String;
      final actionType = entry['action_type'] as String;
      final payloadStr = entry['payload_json'] as String;
      final retryCount = entry['retry_count'] as int? ?? 0;

      // Exponential backoff check: skip if too recent.
      final lastAttemptStr = entry['last_attempt_at'] as String?;
      if (lastAttemptStr != null && retryCount > 0) {
        final lastAttempt = DateTime.tryParse(lastAttemptStr);
        if (lastAttempt != null) {
          final backoffSeconds =
              min(pow(2, retryCount).toInt(), _maxBackoffSeconds);
          final nextAttempt =
              lastAttempt.add(Duration(seconds: backoffSeconds));
          if (DateTime.now().toUtc().isBefore(nextAttempt)) {
            continue; // Skip — not yet time to retry.
          }
        }
      }

      Map<String, dynamic> payload;
      try {
        payload = jsonDecode(payloadStr) as Map<String, dynamic>;
      } catch (_) {
        // Malformed payload — mark as failed and skip.
        await _db.markOutboxFailed(id);
        continue;
      }

      final ApiResponse response;
      switch (actionType) {
        case 'task_progress':
          response = await _apiClient.reportProgress(payload);
        case 'crew_checkin':
          response = await _apiClient.checkin(payload);
        case 'daily_log':
          response = await _apiClient.submitDailyLog(payload);
        default:
          // Unknown action type — mark as failed.
          await _db.markOutboxFailed(id);
          continue;
      }

      if (response.isSuccess) {
        // 201 Created — successful sync.
        await _db.deleteOutboxEntry(id);
      } else if (response.isDuplicate) {
        // 409 Conflict — already processed, treat as success.
        await _db.deleteOutboxEntry(id);
      } else if (response.isServerError) {
        // 5xx — increment retry, will be picked up next cycle.
        await _db.incrementOutboxRetry(id);
      } else if (response.statusCode == 0) {
        // Network error — increment retry.
        await _db.incrementOutboxRetry(id);
        break; // Stop pushing if network is down.
      } else {
        // 4xx (other than 409) — mark as failed, do not retry.
        await _db.markOutboxFailed(id);
      }
    }
  }
}
