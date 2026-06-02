import 'package:buildos_field/l10n/app_localizations.dart';
import 'package:buildos_field/providers/app_providers.dart';
import 'package:buildos_field/widgets/fb_sync_chip.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

Widget _harness({required bool online, required int pending}) {
  return ProviderScope(
    overrides: [
      onlineProvider.overrideWith((ref) => Stream.value(online)),
      pendingCountProvider.overrideWith((ref) => Stream.value(pending)),
    ],
    child: const MaterialApp(
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(body: FbSyncChip()),
    ),
  );
}

void main() {
  testWidgets('shows Online when connected with an empty queue', (
    tester,
  ) async {
    await tester.pumpWidget(_harness(online: true, pending: 0));
    await tester.pumpAndSettle();
    expect(find.text('Online'), findsOneWidget);
  });

  testWidgets('shows queued count when offline', (tester) async {
    await tester.pumpWidget(_harness(online: false, pending: 3));
    await tester.pumpAndSettle();
    expect(find.text('Offline · 3 queued'), findsOneWidget);
  });

  testWidgets('shows queued count when online but writes are pending', (
    tester,
  ) async {
    await tester.pumpWidget(_harness(online: true, pending: 2));
    await tester.pumpAndSettle();
    expect(find.text('Offline · 2 queued'), findsOneWidget);
  });
}
