import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:workmanager/workmanager.dart';

import 'database/database.dart';
import 'screens/crew_checkin_screen.dart';
import 'screens/daily_log_screen.dart';
import 'screens/sync_status_screen.dart';
import 'screens/task_list_screen.dart';
import 'services/api_client.dart';
import 'services/auth_service.dart';
import 'services/connectivity_service.dart';
import 'services/sync_service.dart';
import 'theme/app_theme.dart';
import 'widgets/connectivity_banner.dart';

/// Workmanager background sync task name.
const String backgroundSyncTask = 'com.futurebuild.field.backgroundSync';

/// Workmanager callback dispatcher — runs in a separate isolate.
@pragma('vm:entry-point')
void callbackDispatcher() {
  Workmanager().executeTask((task, inputData) async {
    if (task == backgroundSyncTask) {
      try {
        final db = AppDatabase();
        final apiClient = ApiClient();
        final syncService = SyncService(db: db, apiClient: apiClient);
        await syncService.init();
        await syncService.fullSync();
        await db.close();
      } catch (_) {
        // Background sync failure is non-fatal.
      }
    }
    return true;
  });
}

void main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Initialize workmanager for background sync.
  await Workmanager().initialize(callbackDispatcher, isInDebugMode: false);

  // Register periodic background sync (minimum 15 minutes on Android).
  await Workmanager().registerPeriodicTask(
    'futurebuild-background-sync',
    backgroundSyncTask,
    frequency: const Duration(minutes: 15),
    constraints: Constraints(
      networkType: NetworkType.connected,
    ),
    existingWorkPolicy: ExistingWorkPolicy.keep,
  );

  // Create core singletons.
  final db = AppDatabase();
  final authService = AuthService();
  final apiClient = ApiClient();

  // Initialize auth and set token on API client.
  await authService.init();
  if (authService.isLoggedIn) {
    apiClient.setToken(authService.getToken());
  }
  authService.addListener(() {
    apiClient.setToken(authService.getToken());
  });

  final syncService = SyncService(db: db, apiClient: apiClient);
  await syncService.init();

  final connectivityService = ConnectivityService();
  await connectivityService.init();

  // Trigger sync when connectivity restores.
  connectivityService.onConnectivityRestored = () {
    syncService.fullSync();
  };

  runApp(
    MultiProvider(
      providers: [
        Provider<AppDatabase>.value(value: db),
        ChangeNotifierProvider<AuthService>.value(value: authService),
        ChangeNotifierProvider<SyncService>.value(value: syncService),
        ChangeNotifierProvider<ConnectivityService>.value(
            value: connectivityService),
      ],
      child: const FutureBuildFieldApp(),
    ),
  );
}

/// Root application widget.
class FutureBuildFieldApp extends StatelessWidget {
  const FutureBuildFieldApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'FutureBuild Field',
      theme: AppTheme.darkTheme,
      debugShowCheckedModeBanner: false,
      home: const FieldPortalShell(),
    );
  }
}

/// Main shell with bottom navigation and connectivity banner.
class FieldPortalShell extends StatefulWidget {
  const FieldPortalShell({super.key});

  @override
  State<FieldPortalShell> createState() => _FieldPortalShellState();
}

class _FieldPortalShellState extends State<FieldPortalShell> {
  int _currentIndex = 0;

  static const List<Widget> _screens = [
    TaskListScreen(),
    DailyLogScreen(),
    CrewCheckinScreen(),
    SyncStatusScreen(),
  ];

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Column(
        children: [
          // Connectivity banner at top.
          const ConnectivityBanner(),
          // Main screen content.
          Expanded(
            child: IndexedStack(
              index: _currentIndex,
              children: _screens,
            ),
          ),
        ],
      ),
      bottomNavigationBar: BottomNavigationBar(
        currentIndex: _currentIndex,
        onTap: (index) {
          setState(() {
            _currentIndex = index;
          });
        },
        items: [
          const BottomNavigationBarItem(
            icon: Icon(Icons.assignment),
            label: 'Tasks',
          ),
          const BottomNavigationBarItem(
            icon: Icon(Icons.description),
            label: 'Daily Log',
          ),
          const BottomNavigationBarItem(
            icon: Icon(Icons.location_on),
            label: 'Check-In',
          ),
          BottomNavigationBarItem(
            icon: Consumer<AppDatabase>(
              builder: (context, db, _) {
                return StreamBuilder<int>(
                  stream: db.watchPendingOutboxCount(),
                  builder: (context, snapshot) {
                    final count = snapshot.data ?? 0;
                    if (count > 0) {
                      return Badge(
                        label: Text(
                          '$count',
                          style: const TextStyle(fontSize: 10),
                        ),
                        backgroundColor: AppTheme.amberWarning,
                        child: const Icon(Icons.sync),
                      );
                    }
                    return const Icon(Icons.sync);
                  },
                );
              },
            ),
            label: 'Sync',
          ),
        ],
      ),
    );
  }
}
