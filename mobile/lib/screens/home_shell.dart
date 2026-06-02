import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../l10n/app_localizations.dart';
import '../providers/app_providers.dart';
import '../services/push_service.dart';
import '../widgets/fb_sync_chip.dart';
import 'daily_log_screen.dart';
import 'more_screen.dart';
import 'photos_screen.dart';
import 'sync_status_screen.dart';
import 'tasks_screen.dart';

/// The authenticated field shell: a glove-friendly bottom-nav over an
/// IndexedStack so each tab keeps its scroll position. The [FbSyncChip] lives
/// in the app bar on every tab and deep-links into the Sync Status screen.
class HomeShell extends ConsumerStatefulWidget {
  const HomeShell({super.key});

  @override
  ConsumerState<HomeShell> createState() => _HomeShellState();
}

class _HomeShellState extends ConsumerState<HomeShell> {
  int _index = 0;

  @override
  void initState() {
    super.initState();
    // Best-effort initial sync once the first frame is up.
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(syncControllerProvider.notifier).syncNow();
    });
    // FCM wake-ups nudge a sync; no-op when Firebase isn't configured.
    PushService.instance.init(
      onWake: () => ref.read(syncControllerProvider.notifier).syncNow(),
    );
  }

  void _openSyncStatus() {
    Navigator.of(
      context,
    ).push(MaterialPageRoute<void>(builder: (_) => const SyncStatusScreen()));
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    // Auto-drain the outbox whenever connectivity is (re)gained.
    ref.listen(onlineProvider, (prev, next) {
      final wasOnline = prev?.value ?? false;
      final isOnline = next.value ?? false;
      if (!wasOnline && isOnline) {
        ref.read(syncControllerProvider.notifier).syncNow();
      }
    });

    final titles = [
      l10n.tasksTitle,
      l10n.dailyLogTitle,
      l10n.photosTitle,
      l10n.navMore,
    ];

    const pages = [
      TasksScreen(),
      DailyLogScreen(),
      PhotosScreen(),
      MoreScreen(),
    ];

    return Scaffold(
      appBar: AppBar(
        title: Text(titles[_index]),
        actions: [
          FbSyncChip(onTap: _openSyncStatus),
          const SizedBox(width: 8),
        ],
      ),
      body: IndexedStack(index: _index, children: pages),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (i) => setState(() => _index = i),
        destinations: [
          NavigationDestination(
            icon: const Icon(Icons.checklist_outlined),
            selectedIcon: const Icon(Icons.checklist),
            label: l10n.navTasks,
          ),
          NavigationDestination(
            icon: const Icon(Icons.edit_note_outlined),
            selectedIcon: const Icon(Icons.edit_note),
            label: l10n.navLog,
          ),
          NavigationDestination(
            icon: const Icon(Icons.photo_camera_outlined),
            selectedIcon: const Icon(Icons.photo_camera),
            label: l10n.navPhotos,
          ),
          NavigationDestination(
            icon: const Icon(Icons.more_horiz),
            label: l10n.navMore,
          ),
        ],
      ),
    );
  }
}
