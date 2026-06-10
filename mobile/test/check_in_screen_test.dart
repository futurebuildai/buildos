import 'package:buildos_field/l10n/app_localizations.dart';
import 'package:buildos_field/models/project_task.dart';
import 'package:buildos_field/providers/app_providers.dart';
import 'package:buildos_field/screens/check_in_screen.dart';
import 'package:buildos_field/services/sync_service.dart';
import 'package:buildos_field/widgets/fb_dashed_border.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

/// Records queueCheckin; every other SyncService member is unused by the screen
/// and routes to noSuchMethod (throws if ever hit).
class _RecordingSync implements SyncService {
  final List<Map<String, dynamic>> checkins = [];

  @override
  Future<void> queueCheckin({
    required String projectId,
    List<Map<String, dynamic>> crewMembers = const [],
    double? gpsLat,
    double? gpsLng,
    String? notes,
  }) async {
    checkins.add({
      'projectId': projectId,
      'crewMembers': crewMembers,
      'gpsLat': gpsLat,
      'gpsLng': gpsLng,
      'notes': notes,
    });
  }

  @override
  dynamic noSuchMethod(Invocation invocation) => super.noSuchMethod(invocation);
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

const _task2 = ProjectTask(
  id: 't2',
  projectId: 'p2',
  wbsCode: '2.0',
  name: 'Roof',
  durationDays: 3,
  isCritical: false,
  status: 'not_started',
  percentComplete: 0,
);

Widget _harness(
  SyncService sync, {
  bool online = true,
  List<ProjectTask> tasks = const [_task],
}) {
  return ProviderScope(
    overrides: [
      syncServiceProvider.overrideWithValue(sync),
      onlineProvider.overrideWith((ref) => Stream.value(online)),
      pendingCountProvider.overrideWith((ref) => Stream.value(0)),
      tasksProvider.overrideWith((ref) async => tasks),
    ],
    child: const MaterialApp(
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: CheckInScreen(),
    ),
  );
}

/// Mounts the screen ON a navigator (over a sentinel first route) so the
/// post-submit `maybePop` has somewhere to pop back to.
Widget _navHarness(SyncService sync) {
  return ProviderScope(
    overrides: [
      syncServiceProvider.overrideWithValue(sync),
      onlineProvider.overrideWith((ref) => Stream.value(true)),
      pendingCountProvider.overrideWith((ref) => Stream.value(0)),
      tasksProvider.overrideWith((ref) async => const [_task]),
    ],
    child: MaterialApp(
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      home: Scaffold(
        body: Builder(
          builder: (context) => ElevatedButton(
            onPressed: () => Navigator.of(context).push(
              MaterialPageRoute<void>(builder: (_) => const CheckInScreen()),
            ),
            child: const Text('open'),
          ),
        ),
      ),
    ),
  );
}

void main() {
  testWidgets('crew roster starts with one row; add/remove changes it', (
    tester,
  ) async {
    await tester.pumpWidget(_harness(_RecordingSync()));
    await tester.pumpAndSettle();

    // One crew row = name + role; plus the notes field = 3 TextFormFields.
    expect(find.byType(TextFormField), findsNWidgets(3));
    // The sole row's remove button is disabled (always keep one row).
    expect(
      tester.widget<IconButton>(find.byType(IconButton)).onPressed,
      isNull,
    );

    await tester.tap(find.text('Add crew member'));
    await tester.pumpAndSettle();
    expect(find.byType(TextFormField), findsNWidgets(5)); // two rows + notes
    // With two rows, both removes are enabled.
    expect(
      tester.widget<IconButton>(find.byType(IconButton).first).onPressed,
      isNotNull,
    );

    // Remove the first row (enabled now that there are two).
    await tester.tap(find.byIcon(Icons.remove_circle_outline).first);
    await tester.pumpAndSettle();
    expect(find.byType(TextFormField), findsNWidgets(3));
  });

  testWidgets('submit queues a check-in with the crew payload', (tester) async {
    final sync = _RecordingSync();
    await tester.pumpWidget(_harness(sync));
    await tester.pumpAndSettle();

    // The first field is the first crew row's name (single project → no dropdown).
    await tester.enterText(find.byType(TextFormField).first, 'Sam');
    await tester.tap(find.text('Submit check-in'));
    await tester.pumpAndSettle();

    expect(sync.checkins, hasLength(1));
    expect(sync.checkins.first['projectId'], 'p1');
    expect(sync.checkins.first['crewMembers'], [
      {'name': 'Sam'},
    ]);
    // No notes entered → null (not ''); no GPS in tests → null lat/lng.
    expect(sync.checkins.first['notes'], isNull);
    expect(sync.checkins.first['gpsLat'], isNull);
    expect(sync.checkins.first['gpsLng'], isNull);
  });

  testWidgets('notes, when entered, ride along (trimmed) in the payload', (
    tester,
  ) async {
    final sync = _RecordingSync();
    await tester.pumpWidget(_harness(sync));
    await tester.pumpAndSettle();
    final fields = find.byType(TextFormField);
    await tester.enterText(fields.at(0), 'Sam'); // name
    await tester.enterText(fields.at(2), '  Footings poured  '); // notes
    await tester.tap(find.text('Submit check-in'));
    await tester.pumpAndSettle();
    expect(sync.checkins.first['notes'], 'Footings poured');
  });

  testWidgets(
    'multi-project: the dropdown selection is the submitted project',
    (tester) async {
      final sync = _RecordingSync();
      await tester.pumpWidget(_harness(sync, tasks: const [_task, _task2]));
      await tester.pumpAndSettle();

      // Two projects → a dropdown appears (absent in the single-project tests).
      expect(find.byType(DropdownButtonFormField<String>), findsOneWidget);
      await tester.tap(find.byType(DropdownButtonFormField<String>));
      await tester.pumpAndSettle();
      await tester.tap(find.text('p2').last); // the menu item
      await tester.pumpAndSettle();

      await tester.enterText(find.byType(TextFormField).first, 'Sam');
      await tester.tap(find.text('Submit check-in'));
      await tester.pumpAndSettle();
      expect(sync.checkins.first['projectId'], 'p2');
    },
  );

  testWidgets('successful submit pops back to the previous route', (
    tester,
  ) async {
    final sync = _RecordingSync();
    await tester.pumpWidget(_navHarness(sync));
    await tester.pumpAndSettle();
    await tester.tap(find.text('open'));
    await tester.pumpAndSettle();
    expect(find.byType(CheckInScreen), findsOneWidget);

    await tester.enterText(find.byType(TextFormField).first, 'Sam');
    await tester.tap(find.text('Submit check-in'));
    await tester.pumpAndSettle();

    expect(sync.checkins, hasLength(1));
    expect(find.byType(CheckInScreen), findsNothing); // popped back
  });

  testWidgets('a role, when entered, rides along in the crew payload', (
    tester,
  ) async {
    final sync = _RecordingSync();
    await tester.pumpWidget(_harness(sync));
    await tester.pumpAndSettle();
    final fields = find.byType(TextFormField);
    await tester.enterText(fields.at(0), 'Sam'); // name
    await tester.enterText(fields.at(1), 'Foreman'); // role
    await tester.tap(find.text('Submit check-in'));
    await tester.pumpAndSettle();
    expect(sync.checkins.first['crewMembers'], [
      {'name': 'Sam', 'role': 'Foreman'},
    ]);
  });

  testWidgets('submit with no named crew shows a hint and does not queue', (
    tester,
  ) async {
    final sync = _RecordingSync();
    await tester.pumpWidget(_harness(sync));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Submit check-in'));
    await tester.pump(); // surface the SnackBar

    expect(sync.checkins, isEmpty);
    expect(find.text('Add at least one crew member.'), findsOneWidget);
  });

  testWidgets('offline shows the dashed queue affordance + caption', (
    tester,
  ) async {
    await tester.pumpWidget(_harness(_RecordingSync(), online: false));
    await tester.pumpAndSettle();
    expect(find.byType(FbDashedBorder), findsOneWidget);
    expect(
      find.text('Offline — this check-in will queue and sync later.'),
      findsOneWidget,
    );
  });

  testWidgets('online hides the offline affordance', (tester) async {
    await tester.pumpWidget(_harness(_RecordingSync(), online: true));
    await tester.pumpAndSettle();
    expect(find.byType(FbDashedBorder), findsNothing);
  });
}
