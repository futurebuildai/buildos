import 'package:buildos_field/l10n/app_localizations.dart';
import 'package:buildos_field/models/equipment_asset.dart';
import 'package:buildos_field/providers/app_providers.dart';
import 'package:buildos_field/screens/equipment_screen.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

EquipmentAsset _asset({
  String id = 'a',
  String name = 'Excavator',
  String type = 'excavator',
  String status = 'available',
  String? serial,
}) => EquipmentAsset(
  id: id,
  name: name,
  assetType: type,
  status: status,
  serialNumber: serial,
  startDate: DateTime.utc(2026, 6, 10),
  endDate: DateTime.utc(2026, 6, 20),
);

Widget _harness(List<EquipmentAsset> equipment) {
  return ProviderScope(
    overrides: [
      equipmentProvider.overrideWith((ref) async => equipment),
      onlineProvider.overrideWith((ref) => Stream.value(true)),
      pendingCountProvider.overrideWith((ref) => Stream.value(0)),
    ],
    child: const MaterialApp(
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: EquipmentScreen(),
    ),
  );
}

void main() {
  testWidgets('renders an equipment card with name, type, and status label', (
    tester,
  ) async {
    await tester.pumpWidget(_harness([_asset(serial: 'SN-9')]));
    await tester.pumpAndSettle();

    expect(find.text('Excavator'), findsOneWidget); // name
    expect(find.text('excavator'), findsOneWidget); // type
    expect(find.text('Available'), findsOneWidget); // status (available)
    expect(find.text('SN SN-9'), findsOneWidget); // serial (placeholder)
    expect(find.textContaining('On site'), findsOneWidget); // allocation window
  });

  testWidgets('maps each status to its localized label (never colour-only)', (
    tester,
  ) async {
    await tester.pumpWidget(
      _harness([
        _asset(id: 'a', name: 'A', status: 'available'),
        _asset(id: 'b', name: 'B', status: 'maintenance'),
        _asset(id: 'c', name: 'C', status: 'unavailable'),
      ]),
    );
    await tester.pumpAndSettle();
    expect(find.text('Available'), findsOneWidget);
    expect(find.text('Maintenance'), findsOneWidget);
    expect(find.text('Unavailable'), findsOneWidget);
  });

  testWidgets('empty equipment shows the empty-state copy', (tester) async {
    await tester.pumpWidget(_harness(const []));
    await tester.pumpAndSettle();
    expect(find.text('No equipment on your sites yet.'), findsOneWidget);
  });
}
