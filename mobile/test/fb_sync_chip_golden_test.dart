@Tags(['golden'])
library;

import 'dart:io';

import 'package:buildos_field/l10n/app_localizations.dart';
import 'package:buildos_field/providers/app_providers.dart';
import 'package:buildos_field/theme/app_theme.dart';
import 'package:buildos_field/widgets/fb_sync_chip.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

/// Golden (pixel) coverage for [FbSyncChip]'s three discrete states — the field
/// app's only always-on connectivity affordance (DESIGN_SYSTEM_COMPONENTS §8).
/// The chip's contract is "status is never color-only": each state pairs a
/// colored token (green dot / amber dot / blue spinner) with explicit text, so
/// the visual regression surface (the right dot + the right copy + the right
/// color together) is exactly what a golden locks down — something the
/// text-only widget test in fb_sync_chip_test.dart can't catch.
///
/// **Opt-in by design.** Goldens are renderer-version sensitive and CI runs
/// `flutter test` against an unpinned `channel: stable`, so these self-skip
/// unless RUN_GOLDEN_TESTS=1 is set (mirrors the `live` tag's env-guard). That
/// keeps the default backend-free suite deterministic across Flutter bumps.
///
///   Run:     RUN_GOLDEN_TESTS=1 flutter test --tags golden
///   Refresh: RUN_GOLDEN_TESTS=1 flutter test --tags golden --update-goldens
// Skipped unless RUN_GOLDEN_TESTS=1 (see the file header + dart_test.yaml).
final bool _skip = Platform.environment['RUN_GOLDEN_TESTS'] != '1';

/// Forces the syncing branch of the chip's state machine on, independent of any
/// real sync run, so the spinner state is reproducible.
class _SyncingController extends SyncController {
  @override
  SyncUiState build() => const SyncUiState(syncing: true);
}

Widget _harness({
  required bool online,
  required int pending,
  bool syncing = false,
}) {
  return ProviderScope(
    overrides: [
      onlineProvider.overrideWith((ref) => Stream.value(online)),
      pendingCountProvider.overrideWith((ref) => Stream.value(pending)),
      if (syncing) syncControllerProvider.overrideWith(_SyncingController.new),
    ],
    child: MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: buildFieldTheme(),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: const Scaffold(body: Center(child: FbSyncChip())),
    ),
  );
}

Future<void> _settleStreams(WidgetTester tester) async {
  // Two pumps: the first lets the overridden StreamProviders emit their
  // single value on the microtask queue, the second rebuilds the chip with it.
  await tester.pump();
  await tester.pump();
}

void main() {
  testWidgets('online · empty queue → green dot + "Online"', (tester) async {
    await tester.pumpWidget(_harness(online: true, pending: 0));
    await tester.pumpAndSettle();
    await expectLater(
      find.byType(FbSyncChip),
      matchesGoldenFile('goldens/sync_chip_online.png'),
    );
  }, skip: _skip);

  testWidgets('offline · queued → amber dot + "Offline · N queued"', (
    tester,
  ) async {
    await tester.pumpWidget(_harness(online: false, pending: 3));
    await tester.pumpAndSettle();
    await expectLater(
      find.byType(FbSyncChip),
      matchesGoldenFile('goldens/sync_chip_offline_queued.png'),
    );
  }, skip: _skip);

  testWidgets('syncing → blueprint-blue spinner + "Syncing…"', (tester) async {
    await tester.pumpWidget(_harness(online: true, pending: 0, syncing: true));
    await _settleStreams(tester);
    // The spinner is an infinite animation — pumpAndSettle would never return.
    // Pump a fixed duration to land on a deterministic frame instead.
    await tester.pump(const Duration(milliseconds: 32));
    await expectLater(
      find.byType(FbSyncChip),
      matchesGoldenFile('goldens/sync_chip_syncing.png'),
    );
  }, skip: _skip);
}
