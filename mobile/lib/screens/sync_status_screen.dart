import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../database/database.dart';
import '../services/connectivity_service.dart';
import '../services/sync_service.dart';
import '../theme/app_theme.dart';

/// Sync diagnostics screen.
///
/// Displays: last sync time, pending outbox count, connectivity indicator,
/// manual sync button, and a list of pending outbox entries.
class SyncStatusScreen extends StatelessWidget {
  const SyncStatusScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Sync Status'),
      ),
      body: Consumer2<SyncService, ConnectivityService>(
        builder: (context, syncService, connectivity, _) {
          return SingleChildScrollView(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // Connectivity status.
                _buildConnectivityCard(context, connectivity),
                const SizedBox(height: 16),

                // Last sync info.
                _buildLastSyncCard(context, syncService),
                const SizedBox(height: 16),

                // Pending outbox count.
                _buildPendingCountCard(context),
                const SizedBox(height: 24),

                // Manual sync button.
                SizedBox(
                  width: double.infinity,
                  child: ElevatedButton.icon(
                    onPressed: syncService.isSyncing
                        ? null
                        : () => syncService.fullSync(),
                    icon: syncService.isSyncing
                        ? const SizedBox(
                            width: 20,
                            height: 20,
                            child: CircularProgressIndicator(
                              strokeWidth: 2,
                              color: AppTheme.onPrimary,
                            ),
                          )
                        : const Icon(Icons.sync),
                    label:
                        Text(syncService.isSyncing ? 'Syncing...' : 'Sync Now'),
                  ),
                ),

                if (syncService.lastError != null) ...[
                  const SizedBox(height: 16),
                  _buildErrorCard(context, syncService.lastError!),
                ],

                const SizedBox(height: 24),

                // Pending outbox entries list.
                Text(
                  'Pending Outbox Entries',
                  style: Theme.of(context).textTheme.titleMedium,
                ),
                const SizedBox(height: 12),
                _buildOutboxList(context),
              ],
            ),
          );
        },
      ),
    );
  }

  Widget _buildConnectivityCard(
      BuildContext context, ConnectivityService connectivity) {
    final isOnline = connectivity.isOnline;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          children: [
            Container(
              width: 12,
              height: 12,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: isOnline ? AppTheme.gableGreen : AppTheme.amberWarning,
                boxShadow: [
                  BoxShadow(
                    color: (isOnline
                            ? AppTheme.gableGreen
                            : AppTheme.amberWarning)
                        .withValues(alpha: 0.4),
                    blurRadius: 8,
                    spreadRadius: 2,
                  ),
                ],
              ),
            ),
            const SizedBox(width: 16),
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Network Status',
                  style: Theme.of(context).textTheme.labelMedium,
                ),
                const SizedBox(height: 2),
                Text(
                  isOnline ? 'Online' : 'Offline',
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                        color: isOnline
                            ? AppTheme.gableGreen
                            : AppTheme.amberWarning,
                        fontWeight: FontWeight.w700,
                      ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildLastSyncCard(BuildContext context, SyncService syncService) {
    final lastSync = syncService.lastSyncTimestamp;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          children: [
            const Icon(Icons.schedule, color: AppTheme.blueprintBlue),
            const SizedBox(width: 16),
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Last Sync',
                  style: Theme.of(context).textTheme.labelMedium,
                ),
                const SizedBox(height: 2),
                Text(
                  lastSync ?? 'Never',
                  style: AppTheme.monoStyle(
                    fontSize: 13,
                    color: lastSync != null
                        ? AppTheme.onBackground
                        : AppTheme.onSurfaceVariant,
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildPendingCountCard(BuildContext context) {
    final db = Provider.of<AppDatabase>(context, listen: false);
    return StreamBuilder<int>(
      stream: db.watchPendingOutboxCount(),
      builder: (context, snapshot) {
        final count = snapshot.data ?? 0;
        return Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Row(
              children: [
                Icon(
                  Icons.outbox,
                  color: count > 0
                      ? AppTheme.amberWarning
                      : AppTheme.onSurfaceVariant,
                ),
                const SizedBox(width: 16),
                Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Pending Changes',
                      style: Theme.of(context).textTheme.labelMedium,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      '$count item${count == 1 ? '' : 's'} waiting to sync',
                      style: AppTheme.monoStyle(
                        fontSize: 13,
                        color: count > 0
                            ? AppTheme.amberWarning
                            : AppTheme.onSurfaceVariant,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
        );
      },
    );
  }

  Widget _buildErrorCard(BuildContext context, String error) {
    return Card(
      color: AppTheme.safetyRed.withValues(alpha: 0.1),
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Icon(Icons.error_outline, color: AppTheme.safetyRed),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Sync Error',
                    style: Theme.of(context).textTheme.labelLarge?.copyWith(
                          color: AppTheme.safetyRed,
                        ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    error,
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                          color: AppTheme.safetyRed.withValues(alpha: 0.8),
                        ),
                    maxLines: 3,
                    overflow: TextOverflow.ellipsis,
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildOutboxList(BuildContext context) {
    final db = Provider.of<AppDatabase>(context, listen: false);
    return FutureBuilder<List<Map<String, dynamic>>>(
      future: db.getPendingOutboxEntries(),
      builder: (context, snapshot) {
        final entries = snapshot.data ?? [];

        if (entries.isEmpty) {
          return Container(
            width: double.infinity,
            padding: const EdgeInsets.all(24),
            decoration: BoxDecoration(
              color: AppTheme.surface1,
              borderRadius: BorderRadius.circular(12),
              border: Border.all(color: AppTheme.outlineVariant),
            ),
            child: Column(
              children: [
                Icon(
                  Icons.check_circle_outline,
                  size: 32,
                  color: AppTheme.gableGreen.withValues(alpha: 0.5),
                ),
                const SizedBox(height: 8),
                Text(
                  'All caught up',
                  style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                        color: AppTheme.onSurfaceVariant,
                      ),
                ),
              ],
            ),
          );
        }

        return Column(
          children: entries.map((entry) {
            final actionType = entry['action_type'] as String? ?? 'unknown';
            final retryCount = entry['retry_count'] as int? ?? 0;
            final createdAt = entry['created_at'] as String? ?? '';
            final idempotencyKey =
                entry['idempotency_key'] as String? ?? '';

            IconData icon;
            Color iconColor;
            switch (actionType) {
              case 'task_progress':
                icon = Icons.trending_up;
                iconColor = AppTheme.gableGreen;
              case 'crew_checkin':
                icon = Icons.location_on;
                iconColor = AppTheme.blueprintBlue;
              case 'daily_log':
                icon = Icons.description;
                iconColor = AppTheme.amberWarning;
              default:
                icon = Icons.pending;
                iconColor = AppTheme.onSurfaceVariant;
            }

            return Card(
              child: Padding(
                padding: const EdgeInsets.all(12),
                child: Row(
                  children: [
                    Icon(icon, color: iconColor, size: 20),
                    const SizedBox(width: 12),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            actionType.replaceAll('_', ' ').toUpperCase(),
                            style: Theme.of(context)
                                .textTheme
                                .labelLarge
                                ?.copyWith(color: iconColor),
                          ),
                          const SizedBox(height: 2),
                          Text(
                            'Key: ${idempotencyKey.length > 8 ? idempotencyKey.substring(0, 8) : idempotencyKey}...',
                            style: AppTheme.monoStyle(
                              fontSize: 11,
                              color: AppTheme.onSurfaceVariant,
                            ),
                          ),
                          Text(
                            createdAt,
                            style: AppTheme.monoStyle(
                              fontSize: 11,
                              color: AppTheme.onSurfaceVariant,
                            ),
                          ),
                        ],
                      ),
                    ),
                    if (retryCount > 0)
                      Container(
                        padding: const EdgeInsets.symmetric(
                            horizontal: 8, vertical: 4),
                        decoration: BoxDecoration(
                          color: AppTheme.amberWarning.withValues(alpha: 0.15),
                          borderRadius: BorderRadius.circular(6),
                        ),
                        child: Text(
                          'Retry $retryCount',
                          style: const TextStyle(
                            fontSize: 11,
                            fontWeight: FontWeight.w600,
                            color: AppTheme.amberWarning,
                          ),
                        ),
                      ),
                  ],
                ),
              ),
            );
          }).toList(),
        );
      },
    );
  }
}
