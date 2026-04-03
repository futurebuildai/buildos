import 'dart:io';

import 'package:drift/drift.dart';
import 'package:drift/native.dart';
import 'package:path_provider/path_provider.dart';
import 'package:path/path.dart' as p;

/// FutureBuild Field Portal local database.
///
/// Uses Drift with raw SQL -- no code generation required.
/// Tables: tasks, outbox, cached_briefings.
class AppDatabase extends GeneratedDatabase {
  AppDatabase() : super(_openConnection());

  /// Constructor for testing with a custom executor.
  AppDatabase.forTesting(super.e);

  @override
  int get schemaVersion => 1;

  @override
  Iterable<TableInfo<Table, dynamic>> get allTables => const [];

  @override
  MigrationStrategy get migration {
    return MigrationStrategy(
      onCreate: (Migrator m) async {
        await customStatement('''
          CREATE TABLE IF NOT EXISTS tasks (
            id TEXT PRIMARY KEY NOT NULL,
            project_id TEXT NOT NULL,
            wbs_code TEXT NOT NULL,
            name TEXT NOT NULL,
            percent_complete INTEGER NOT NULL DEFAULT 0,
            priority TEXT NOT NULL DEFAULT 'normal',
            status TEXT NOT NULL DEFAULT 'pending',
            scheduled_date TEXT,
            last_synced_at TEXT
          )
        ''');

        await customStatement('''
          CREATE TABLE IF NOT EXISTS outbox (
            id TEXT PRIMARY KEY NOT NULL,
            action_type TEXT NOT NULL,
            payload_json TEXT NOT NULL,
            idempotency_key TEXT NOT NULL UNIQUE,
            created_at TEXT NOT NULL,
            sync_status TEXT NOT NULL DEFAULT 'pending',
            retry_count INTEGER NOT NULL DEFAULT 0,
            last_attempt_at TEXT
          )
        ''');

        await customStatement('''
          CREATE TABLE IF NOT EXISTS cached_briefings (
            id TEXT PRIMARY KEY NOT NULL,
            project_id TEXT NOT NULL,
            cards_json TEXT NOT NULL,
            generated_at TEXT NOT NULL,
            cached_at TEXT NOT NULL
          )
        ''');

        // Indices for common queries.
        await customStatement(
            'CREATE INDEX IF NOT EXISTS idx_tasks_project ON tasks(project_id)');
        await customStatement(
            'CREATE INDEX IF NOT EXISTS idx_outbox_status ON outbox(sync_status)');
        await customStatement(
            'CREATE INDEX IF NOT EXISTS idx_outbox_created ON outbox(created_at)');
        await customStatement(
            'CREATE INDEX IF NOT EXISTS idx_briefings_project ON cached_briefings(project_id)');
      },
    );
  }

  // ── Task DAO ────────────────────────────────────────────────────

  /// Insert or replace a task (upsert).
  Future<void> upsertTask({
    required String id,
    required String projectId,
    required String wbsCode,
    required String name,
    int percentComplete = 0,
    String priority = 'normal',
    String status = 'pending',
    String? scheduledDate,
    String? lastSyncedAt,
  }) async {
    await customInsert(
      'INSERT OR REPLACE INTO tasks '
      '(id, project_id, wbs_code, name, percent_complete, priority, status, scheduled_date, last_synced_at) '
      'VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)',
      variables: [
        Variable.withString(id),
        Variable.withString(projectId),
        Variable.withString(wbsCode),
        Variable.withString(name),
        Variable.withInt(percentComplete),
        Variable.withString(priority),
        Variable.withString(status),
        Variable(scheduledDate),
        Variable(lastSyncedAt),
      ],
      updates: {},
    );
  }

  /// Get all tasks ordered by project and WBS code.
  Future<List<Map<String, dynamic>>> getAllTasks() async {
    final rows = await customSelect(
      'SELECT * FROM tasks ORDER BY project_id, wbs_code',
    ).get();
    return rows.map(_rowToMap).toList();
  }

  /// Get tasks for a specific project.
  Future<List<Map<String, dynamic>>> getTasksByProject(
      String projectId) async {
    final rows = await customSelect(
      'SELECT * FROM tasks WHERE project_id = ? ORDER BY wbs_code',
      variables: [Variable.withString(projectId)],
    ).get();
    return rows.map(_rowToMap).toList();
  }

  /// Update percent_complete for a task.
  Future<int> updateTaskPercent(String taskId, int percentComplete) async {
    return customUpdate(
      'UPDATE tasks SET percent_complete = ? WHERE id = ?',
      variables: [
        Variable.withInt(percentComplete),
        Variable.withString(taskId),
      ],
      updates: {},
    );
  }

  /// Watch all tasks (returns a stream for reactive UI).
  Stream<List<Map<String, dynamic>>> watchAllTasks() {
    return customSelect(
      'SELECT * FROM tasks ORDER BY project_id, wbs_code',
    ).watch().map((rows) => rows.map(_rowToMap).toList());
  }

  // ── Outbox DAO ──────────────────────────────────────────────────

  /// Insert a new outbox entry.
  Future<void> insertOutboxEntry({
    required String id,
    required String actionType,
    required String payloadJson,
    required String idempotencyKey,
    required String createdAt,
    String syncStatus = 'pending',
  }) async {
    await customInsert(
      'INSERT INTO outbox '
      '(id, action_type, payload_json, idempotency_key, created_at, sync_status, retry_count) '
      'VALUES (?, ?, ?, ?, ?, ?, 0)',
      variables: [
        Variable.withString(id),
        Variable.withString(actionType),
        Variable.withString(payloadJson),
        Variable.withString(idempotencyKey),
        Variable.withString(createdAt),
        Variable.withString(syncStatus),
      ],
      updates: {},
    );
  }

  /// Get all pending outbox entries in FIFO order.
  Future<List<Map<String, dynamic>>> getPendingOutboxEntries() async {
    final rows = await customSelect(
      "SELECT * FROM outbox WHERE sync_status = 'pending' ORDER BY created_at ASC",
    ).get();
    return rows.map(_rowToMap).toList();
  }

  /// Mark an outbox entry as synced.
  Future<int> markOutboxSynced(String id) async {
    return customUpdate(
      "UPDATE outbox SET sync_status = 'synced' WHERE id = ?",
      variables: [Variable.withString(id)],
      updates: {},
    );
  }

  /// Mark an outbox entry as failed.
  Future<int> markOutboxFailed(String id) async {
    return customUpdate(
      "UPDATE outbox SET sync_status = 'failed' WHERE id = ?",
      variables: [Variable.withString(id)],
      updates: {},
    );
  }

  /// Increment retry count and set last_attempt_at.
  Future<int> incrementOutboxRetry(String id) async {
    final now = DateTime.now().toUtc().toIso8601String();
    return customUpdate(
      'UPDATE outbox SET retry_count = retry_count + 1, '
      "last_attempt_at = ?, sync_status = 'pending' WHERE id = ?",
      variables: [
        Variable.withString(now),
        Variable.withString(id),
      ],
      updates: {},
    );
  }

  /// Get the count of pending outbox entries.
  Future<int> getPendingOutboxCount() async {
    final rows = await customSelect(
      "SELECT COUNT(*) as cnt FROM outbox WHERE sync_status = 'pending'",
    ).get();
    if (rows.isEmpty) return 0;
    return rows.first.data['cnt'] as int? ?? 0;
  }

  /// Delete an outbox entry (after successful sync).
  Future<int> deleteOutboxEntry(String id) async {
    return customUpdate(
      'DELETE FROM outbox WHERE id = ?',
      variables: [Variable.withString(id)],
      updates: {},
    );
  }

  /// Watch pending outbox count for reactive UI.
  Stream<int> watchPendingOutboxCount() {
    return customSelect(
      "SELECT COUNT(*) as cnt FROM outbox WHERE sync_status = 'pending'",
    ).watch().map((rows) {
      if (rows.isEmpty) return 0;
      return rows.first.data['cnt'] as int? ?? 0;
    });
  }

  // ── Cached Briefings DAO ────────────────────────────────────────

  /// Insert or replace a cached briefing.
  Future<void> upsertBriefing({
    required String id,
    required String projectId,
    required String cardsJson,
    required String generatedAt,
    required String cachedAt,
  }) async {
    await customInsert(
      'INSERT OR REPLACE INTO cached_briefings '
      '(id, project_id, cards_json, generated_at, cached_at) '
      'VALUES (?, ?, ?, ?, ?)',
      variables: [
        Variable.withString(id),
        Variable.withString(projectId),
        Variable.withString(cardsJson),
        Variable.withString(generatedAt),
        Variable.withString(cachedAt),
      ],
      updates: {},
    );
  }

  /// Get cached briefings, most recent first.
  Future<List<Map<String, dynamic>>> getBriefings() async {
    final rows = await customSelect(
      'SELECT * FROM cached_briefings ORDER BY cached_at DESC',
    ).get();
    return rows.map(_rowToMap).toList();
  }

  // ── Helpers ─────────────────────────────────────────────────────

  Map<String, dynamic> _rowToMap(QueryRow row) {
    return row.data;
  }
}

LazyDatabase _openConnection() {
  return LazyDatabase(() async {
    final dbFolder = await getApplicationDocumentsDirectory();
    final file = File(p.join(dbFolder.path, 'futurebuild_field.sqlite'));
    return NativeDatabase.createInBackground(file);
  });
}
