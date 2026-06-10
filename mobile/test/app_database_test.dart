import 'dart:io';

import 'package:buildos_field/database/app_database.dart';
import 'package:drift/native.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:sqlite3/sqlite3.dart';

CachedEquipmentCompanion _equip(String id, String name) =>
    CachedEquipmentCompanion.insert(
      id: id,
      name: name,
      assetType: 'excavator',
      startDate: DateTime.utc(2026, 6, 10),
      endDate: DateTime.utc(2026, 6, 20),
    );

// The v1 schema (pre-4a-ii): the three original tables + user_version = 1.
// DateTime columns are INTEGER (drift's default), but the migration test only
// seeds a DateTime-free cached_tasks row, so storage format never matters.
const _v1Ddl = '''
CREATE TABLE outbox_items (
  id TEXT NOT NULL PRIMARY KEY,
  action TEXT NOT NULL,
  payload TEXT NOT NULL,
  idempotency_key TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'pending',
  retry_count INTEGER NOT NULL DEFAULT 0,
  next_attempt_at INTEGER,
  last_error TEXT,
  created_at INTEGER NOT NULL DEFAULT 0,
  synced_at INTEGER
);
CREATE TABLE cached_tasks (
  id TEXT NOT NULL PRIMARY KEY,
  project_id TEXT NOT NULL,
  wbs_code TEXT NOT NULL,
  name TEXT NOT NULL,
  duration_days INTEGER NOT NULL DEFAULT 0,
  is_critical INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL DEFAULT 'pending',
  percent_complete INTEGER NOT NULL DEFAULT 0,
  total_float INTEGER,
  payload TEXT NOT NULL
);
CREATE TABLE sync_meta (
  id INTEGER NOT NULL PRIMARY KEY DEFAULT 1,
  last_synced_at INTEGER,
  since_cursor TEXT
);
PRAGMA user_version = 1;
''';

void main() {
  test(
    'replaceEquipment is FULL-REPLACE (delete-then-fill), not upsert',
    () async {
      final db = AppDatabase.forTesting();
      addTearDown(db.close);

      await db.replaceEquipment([
        _equip('a', 'Excavator'),
        _equip('b', 'Crane'),
      ]);
      expect((await db.allCachedEquipment()).length, 2);

      // A later sync returns only one asset — the other must DISAPPEAR (the bug
      // an upsert-only cache would produce: equipment that left my site lingers).
      await db.replaceEquipment([_equip('a', 'Excavator')]);
      final rows = await db.allCachedEquipment();
      expect(rows.length, 1);
      expect(rows.single.id, 'a');

      // An empty response clears the cache entirely.
      await db.replaceEquipment([]);
      expect(await db.allCachedEquipment(), isEmpty);
    },
  );

  test(
    'v1 → v2 migration adds the equipment cache and preserves existing data',
    () async {
      final dir = await Directory.systemTemp.createTemp('buildos_mig');
      addTearDown(() => dir.delete(recursive: true));
      final path = '${dir.path}/db.sqlite';

      // Build a real v1 database with a seeded task row, via raw sqlite3 so no v2
      // schema leaks in.
      final raw = sqlite3.open(path);
      raw.execute(_v1Ddl);
      raw.execute(
        "INSERT INTO cached_tasks (id, project_id, wbs_code, name, payload) "
        "VALUES ('t1', 'p1', '1.0', 'Frame', '{\"id\":\"t1\"}')",
      );
      raw.close();

      // Open the app DB on the same file → schemaVersion 2 > stored 1 → onUpgrade.
      final db = AppDatabase(NativeDatabase(File(path)));
      addTearDown(db.close);

      // The pre-existing task survived the upgrade.
      final tasks = await db.allCachedTasks();
      expect(tasks.length, 1);
      expect(tasks.single.id, 't1');

      // The new equipment table exists and is usable.
      await db.replaceEquipment([_equip('x', 'Loader')]);
      final equip = await db.allCachedEquipment();
      expect(equip.single.id, 'x');
    },
  );
}
