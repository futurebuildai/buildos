import 'dart:io';

import 'package:drift/drift.dart';
import 'package:drift/native.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

part 'app_database.g.dart';

/// The offline outbox (FRONTEND_ARCHITECTURE §8). Every field write is appended
/// here first, then drained FIFO by the [SyncService]. Each row carries a UUID
/// `idempotencyKey` so the server dedups replays — retries are safe.
class OutboxItems extends Table {
  TextColumn get id => text()();

  /// One of OutboxAction.wire — progress | checkin | daily-log.
  TextColumn get action => text()();

  /// The POST body as a JSON string.
  TextColumn get payload => text()();

  /// UNIQUE server-dedup token (a UUID). A 409 on replay → "already accepted".
  TextColumn get idempotencyKey => text().unique()();

  /// OutboxStatus.name — pending | synced | failed.
  TextColumn get status => text().withDefault(const Constant('pending'))();

  IntColumn get retryCount => integer().withDefault(const Constant(0))();

  /// Exponential-backoff gate: don't attempt before this time.
  DateTimeColumn get nextAttemptAt => dateTime().nullable()();

  TextColumn get lastError => text().nullable()();

  DateTimeColumn get createdAt => dateTime().withDefault(currentDateAndTime)();

  DateTimeColumn get syncedAt => dateTime().nullable()();

  @override
  Set<Column> get primaryKey => {id};
}

/// Server-authoritative task cache so the field app renders the task list
/// offline. Server-wins: each successful sync overwrites local rows.
class CachedTasks extends Table {
  TextColumn get id => text()();
  TextColumn get projectId => text()();
  TextColumn get wbsCode => text()();
  TextColumn get name => text()();
  IntColumn get durationDays => integer().withDefault(const Constant(0))();
  BoolColumn get isCritical => boolean().withDefault(const Constant(false))();
  TextColumn get status => text().withDefault(const Constant('pending'))();
  IntColumn get percentComplete => integer().withDefault(const Constant(0))();
  IntColumn get totalFloat => integer().nullable()();

  /// Full JSON for round-trip fidelity (fields the columns don't break out).
  TextColumn get payload => text()();

  @override
  Set<Column> get primaryKey => {id};
}

/// Single-row table holding sync metadata (the `?since=` cursor + last-sync).
class SyncMeta extends Table {
  IntColumn get id => integer().withDefault(const Constant(1))();
  DateTimeColumn get lastSyncedAt => dateTime().nullable()();

  /// RFC3339 server_time to pass back as `?since=` on the next pull.
  TextColumn get sinceCursor => text().nullable()();

  @override
  Set<Column> get primaryKey => {id};
}

@DriftDatabase(tables: [OutboxItems, CachedTasks, SyncMeta])
class AppDatabase extends _$AppDatabase {
  AppDatabase([QueryExecutor? executor]) : super(executor ?? _open());

  /// In-memory database for tests (no platform plugins needed).
  AppDatabase.forTesting() : super(NativeDatabase.memory());

  @override
  int get schemaVersion => 1;

  // ---- Outbox -----------------------------------------------------------

  Future<void> enqueue(OutboxItemsCompanion item) =>
      into(outboxItems).insert(item);

  /// FIFO, only items whose backoff gate has elapsed.
  Future<List<OutboxItem>> dueItems(DateTime now) {
    return (select(outboxItems)
          ..where(
            (t) =>
                t.status.equals('pending') &
                (t.nextAttemptAt.isSmallerOrEqualValue(now) |
                    t.nextAttemptAt.isNull()),
          )
          ..orderBy([(t) => OrderingTerm.asc(t.createdAt)]))
        .get();
  }

  Future<int> pendingCount() async {
    final rows = await (select(
      outboxItems,
    )..where((t) => t.status.equals('pending'))).get();
    return rows.length;
  }

  Future<void> markSynced(String id) {
    return (update(outboxItems)..where((t) => t.id.equals(id))).write(
      OutboxItemsCompanion(
        status: const Value('synced'),
        syncedAt: Value(DateTime.now().toUtc()),
      ),
    );
  }

  Future<void> markRetry(String id, int retryCount, DateTime next, String err) {
    return (update(outboxItems)..where((t) => t.id.equals(id))).write(
      OutboxItemsCompanion(
        retryCount: Value(retryCount),
        nextAttemptAt: Value(next),
        lastError: Value(err),
      ),
    );
  }

  Future<void> markFailed(String id, String err) {
    return (update(outboxItems)..where((t) => t.id.equals(id))).write(
      OutboxItemsCompanion(
        status: const Value('failed'),
        lastError: Value(err),
      ),
    );
  }

  // ---- Task cache (server-wins) ----------------------------------------

  Future<void> replaceTasks(List<CachedTasksCompanion> tasks) async {
    await batch((b) {
      for (final t in tasks) {
        b.insert(cachedTasks, t, onConflict: DoUpdate((_) => t));
      }
    });
  }

  Future<List<CachedTask>> allCachedTasks() => select(cachedTasks).get();

  // ---- Sync metadata ----------------------------------------------------

  Future<SyncMetaData?> loadSyncMeta() =>
      (select(syncMeta)..where((t) => t.id.equals(1))).getSingleOrNull();

  Future<void> setSyncMeta({required DateTime at, required String? cursor}) {
    return into(syncMeta).insertOnConflictUpdate(
      SyncMetaCompanion(
        id: const Value(1),
        lastSyncedAt: Value(at),
        sinceCursor: Value(cursor),
      ),
    );
  }
}

LazyDatabase _open() {
  return LazyDatabase(() async {
    final dir = await getApplicationDocumentsDirectory();
    final file = File(p.join(dir.path, 'buildos_field.sqlite'));
    return NativeDatabase.createInBackground(file);
  });
}
