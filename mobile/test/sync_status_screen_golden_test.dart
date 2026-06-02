@Tags(['golden'])
library;

import 'dart:io';

import 'package:buildos_field/l10n/app_localizations.dart';
import 'package:buildos_field/providers/app_providers.dart';
import 'package:buildos_field/screens/sync_status_screen.dart';
import 'package:buildos_field/theme/app_theme.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:intl/intl.dart';

/// Golden (pixel) coverage for [SyncStatusScreen] — the field app's primary ops
/// surface (last-sync time, queued-outbox count, error + manual "retry now").
/// Locks down the full-screen layout: the FbSyncChip header, the two info
/// Cards, the conditional "everything is up to date" / error copy, and the
/// primary action button. The widget tests cover the outbox *logic*; this
/// covers what the worker in the field actually sees.
///
/// **Opt-in by design** (same rationale as fb_sync_chip_golden_test.dart):
/// renderer-version sensitive, and CI's `mobile` job runs `flutter test`
/// against an unpinned `channel: stable`, so these self-skip unless
/// RUN_GOLDEN_TESTS=1 is set.
///
///   Run:     RUN_GOLDEN_TESTS=1 flutter test --tags golden
///   Refresh: append --update-goldens
final bool _skip = Platform.environment['RUN_GOLDEN_TESTS'] != '1';

/// Pins the sync controller to a fixed [SyncUiState] so the screen renders a
/// deterministic last-sync time / error without running a real sync.
class _FixedSyncController extends SyncController {
  _FixedSyncController(this._state);

  final SyncUiState _state;

  @override
  SyncUiState build() => _state;
}

/// A LOCAL (not UTC) fixed instant — the screen calls `.toLocal()` before
/// formatting, so a local DateTime keeps the rendered timestamp independent of
/// the host timezone.
final DateTime _fixedSyncedAt = DateTime(2026, 1, 15, 9, 41);

Widget _harness({
  required bool online,
  required int pending,
  required SyncUiState sync,
}) {
  return ProviderScope(
    overrides: [
      onlineProvider.overrideWith((ref) => Stream.value(online)),
      pendingCountProvider.overrideWith((ref) => Stream.value(pending)),
      syncControllerProvider.overrideWith(() => _FixedSyncController(sync)),
    ],
    child: MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: buildFieldTheme(),
      locale: const Locale('en'),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: const SyncStatusScreen(),
    ),
  );
}

Future<void> _pumpScreen(WidgetTester tester, Widget widget) async {
  // A representative field-phone viewport, fixed so layout is deterministic.
  tester.view.physicalSize = const Size(1080, 2280);
  tester.view.devicePixelRatio = 3.0;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(widget);
  await tester.pumpAndSettle();
}

void main() {
  // Lock the formatting locale (the screen's DateFormat has no explicit locale)
  // so the rendered timestamp glyph run is reproducible.
  Intl.defaultLocale = 'en_US';

  testWidgets('online · synced · empty queue → "up to date" + enabled retry', (
    tester,
  ) async {
    await _pumpScreen(
      tester,
      _harness(
        online: true,
        pending: 0,
        sync: SyncUiState(lastSyncedAt: _fixedSyncedAt),
      ),
    );
    await expectLater(
      find.byType(SyncStatusScreen),
      matchesGoldenFile('goldens/sync_status_synced_empty.png'),
    );
  }, skip: _skip);

  testWidgets('offline · queued · error → safety-red error + queued count', (
    tester,
  ) async {
    await _pumpScreen(
      tester,
      _harness(
        online: false,
        pending: 3,
        sync: SyncUiState(lastSyncedAt: _fixedSyncedAt, error: 'Sync failed'),
      ),
    );
    await expectLater(
      find.byType(SyncStatusScreen),
      matchesGoldenFile('goldens/sync_status_queued_error.png'),
    );
  }, skip: _skip);
}
