import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../database/database.dart';
import '../services/connectivity_service.dart';
import '../theme/app_theme.dart';

/// Reusable connectivity banner widget.
///
/// Shows an amber banner when offline with the pending outbox count.
/// Shows a green indicator when online.
/// Animated transitions between states.
class ConnectivityBanner extends StatelessWidget {
  const ConnectivityBanner({super.key});

  @override
  Widget build(BuildContext context) {
    return Consumer<ConnectivityService>(
      builder: (context, connectivity, _) {
        return AnimatedSwitcher(
          duration: const Duration(milliseconds: 300),
          transitionBuilder: (child, animation) {
            return SizeTransition(
              sizeFactor: animation,
              axisAlignment: -1.0,
              child: child,
            );
          },
          child: connectivity.isOnline
              ? const SizedBox.shrink(key: ValueKey('online'))
              : _OfflineBanner(key: const ValueKey('offline')),
        );
      },
    );
  }
}

class _OfflineBanner extends StatelessWidget {
  const _OfflineBanner({super.key});

  @override
  Widget build(BuildContext context) {
    final db = Provider.of<AppDatabase>(context, listen: false);

    return StreamBuilder<int>(
      stream: db.watchPendingOutboxCount(),
      builder: (context, snapshot) {
        final pendingCount = snapshot.data ?? 0;

        return Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
          decoration: BoxDecoration(
            color: AppTheme.amberWarning.withValues(alpha: 0.15),
            border: Border(
              bottom: BorderSide(
                color: AppTheme.amberWarning.withValues(alpha: 0.3),
              ),
            ),
          ),
          child: SafeArea(
            bottom: false,
            child: Row(
              children: [
                Container(
                  width: 8,
                  height: 8,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: AppTheme.amberWarning,
                    boxShadow: [
                      BoxShadow(
                        color: AppTheme.amberWarning.withValues(alpha: 0.4),
                        blurRadius: 6,
                        spreadRadius: 1,
                      ),
                    ],
                  ),
                ),
                const SizedBox(width: 12),
                const Icon(
                  Icons.cloud_off,
                  size: 16,
                  color: AppTheme.amberWarning,
                ),
                const SizedBox(width: 8),
                Expanded(
                  child: Text(
                    'Offline mode',
                    style: Theme.of(context).textTheme.bodySmall?.copyWith(
                          color: AppTheme.amberWarning,
                          fontWeight: FontWeight.w600,
                        ),
                  ),
                ),
                if (pendingCount > 0)
                  Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                    decoration: BoxDecoration(
                      color: AppTheme.amberWarning.withValues(alpha: 0.2),
                      borderRadius: BorderRadius.circular(10),
                    ),
                    child: Text(
                      '$pendingCount pending',
                      style: AppTheme.monoStyle(
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
      },
    );
  }
}
