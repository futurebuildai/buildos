import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../l10n/app_localizations.dart';
import '../providers/app_providers.dart';
import '../theme/app_theme.dart';
import '../widgets/fb_sync_chip.dart';

/// Sync Status: surfaces the live connectivity/queue state, the last-sync
/// timestamp, and a manual "retry now" that drains the outbox + re-pulls.
class SyncStatusScreen extends ConsumerWidget {
  const SyncStatusScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final pending = ref.watch(pendingCountProvider).value ?? 0;
    final syncState = ref.watch(syncControllerProvider);
    final meta = syncState.lastSyncedAt;

    final lastSyncText = meta == null
        ? l10n.neverSynced
        : l10n.lastSynced(DateFormat.yMMMd().add_jm().format(meta.toLocal()));

    return Scaffold(
      appBar: AppBar(title: Text(l10n.syncStatusTitle)),
      body: ListView(
        padding: const EdgeInsets.all(FbSizes.gap),
        children: [
          Align(alignment: Alignment.centerLeft, child: const FbSyncChip()),
          const SizedBox(height: FbSizes.gap),
          Card(
            child: ListTile(
              leading: const Icon(
                Icons.access_time,
                color: FbColors.blueprintBlue,
              ),
              title: Text(lastSyncText),
            ),
          ),
          const SizedBox(height: FbSizes.gapSmall),
          Card(
            child: ListTile(
              leading: const Icon(Icons.outbox, color: FbColors.amber),
              title: Text(l10n.queuedActions),
              trailing: Text(
                '$pending',
                style: const TextStyle(
                  fontFamily: fbMonoFamily,
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ),
          if (pending == 0)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: FbSizes.gap),
              child: Text(
                l10n.nothingQueued,
                textAlign: TextAlign.center,
                style: const TextStyle(color: FbColors.textSecondary),
              ),
            ),
          if (syncState.error != null)
            Padding(
              padding: const EdgeInsets.symmetric(vertical: FbSizes.gapSmall),
              child: Text(
                syncState.error!,
                style: const TextStyle(color: FbColors.safetyRed),
              ),
            ),
          const SizedBox(height: FbSizes.gap),
          FilledButton.icon(
            onPressed: syncState.syncing
                ? null
                : () => ref.read(syncControllerProvider.notifier).syncNow(),
            icon: syncState.syncing
                ? const SizedBox(
                    width: 20,
                    height: 20,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      color: FbColors.deepSpace,
                    ),
                  )
                : const Icon(Icons.sync),
            label: Text(syncState.syncing ? l10n.syncing : l10n.retryNow),
          ),
        ],
      ),
    );
  }
}
