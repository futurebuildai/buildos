@Tags(['golden'])
library;

import 'dart:io';

import 'package:buildos_field/l10n/app_localizations.dart';
import 'package:buildos_field/models/project_task.dart';
import 'package:buildos_field/providers/app_providers.dart';
import 'package:buildos_field/screens/check_in_screen.dart';
import 'package:buildos_field/services/sync_service.dart';
import 'package:buildos_field/theme/app_theme.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

/// Golden (pixel) coverage for [CheckInScreen] — the field crew check-in form
/// and its offline affordance (the amber dashed border on the submit CTA +
/// caption, paired with the FbSyncChip). The golden locks the field-app visual
/// contract (dark theme, touch targets, the offline treatment) that the
/// text-only widget tests can't capture.
///
/// **Opt-in by design** (same as fb_sync_chip_golden_test): self-skips unless
/// RUN_GOLDEN_TESTS=1, since goldens are renderer-version sensitive and CI runs
/// an unpinned `channel: stable`.
///
///   Run:     RUN_GOLDEN_TESTS=1 flutter test --tags golden
///   Refresh: RUN_GOLDEN_TESTS=1 flutter test --tags golden --update-goldens
final bool _skip = Platform.environment['RUN_GOLDEN_TESTS'] != '1';

/// No-op SyncService — the goldens render, they don't submit.
class _NoopSync implements SyncService {
  @override
  dynamic noSuchMethod(Invocation invocation) => null;
}

const _task = ProjectTask(
  id: 't1',
  projectId: 'p1',
  wbsCode: '1.0',
  name: 'Frame',
  durationDays: 5,
  isCritical: false,
  status: 'in_progress',
  percentComplete: 0,
);

Widget _harness({required bool online}) {
  return ProviderScope(
    overrides: [
      syncServiceProvider.overrideWithValue(_NoopSync()),
      onlineProvider.overrideWith((ref) => Stream.value(online)),
      pendingCountProvider.overrideWith((ref) => Stream.value(online ? 0 : 2)),
      tasksProvider.overrideWith((ref) async => const [_task]),
    ],
    child: MaterialApp(
      debugShowCheckedModeBanner: false,
      theme: buildFieldTheme(),
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: const CheckInScreen(),
    ),
  );
}

Future<void> _pump(WidgetTester tester, Widget widget) async {
  tester.view.physicalSize = const Size(1080, 2280);
  tester.view.devicePixelRatio = 3.0;
  addTearDown(tester.view.reset);
  await tester.pumpWidget(widget);
  await tester.pumpAndSettle();
}

void main() {
  testWidgets('online → empty crew form, plain submit', (tester) async {
    await _pump(tester, _harness(online: true));
    await expectLater(
      find.byType(CheckInScreen),
      matchesGoldenFile('goldens/check_in_online.png'),
    );
  }, skip: _skip);

  testWidgets('offline → amber dashed submit + "will queue" caption', (
    tester,
  ) async {
    await _pump(tester, _harness(online: false));
    await expectLater(
      find.byType(CheckInScreen),
      matchesGoldenFile('goldens/check_in_offline_queued.png'),
    );
  }, skip: _skip);
}
