import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../l10n/app_localizations.dart';
import '../providers/app_providers.dart';
import '../theme/app_theme.dart';

/// Passive sync indicator shown in the app bar on every field screen
/// (DESIGN_SYSTEM_COMPONENTS §8 `FbSyncChip`). Status is never color-only:
/// each state pairs a colored dot with explicit text.
///
/// - Syncing  → blueprint-blue spinner + "Syncing…"
/// - Offline / queued → amber dot + "Offline · N queued"
/// - Online & empty → green dot + "Online"
///
/// Tapping opens the Sync Status screen.
class FbSyncChip extends ConsumerWidget {
  const FbSyncChip({super.key, this.onTap});

  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final online = ref.watch(onlineProvider).value ?? true;
    final pending = ref.watch(pendingCountProvider).value ?? 0;
    final syncing = ref.watch(syncControllerProvider).syncing;

    final (Color color, String label, Widget leading) = switch ((
      syncing,
      online,
      pending,
    )) {
      (true, _, _) => (
        FbColors.blueprintBlue,
        l10n.syncing,
        const SizedBox(
          width: 12,
          height: 12,
          child: CircularProgressIndicator(
            strokeWidth: 2,
            color: FbColors.blueprintBlue,
          ),
        ),
      ),
      (false, false, _) || (false, true, > 0) => (
        FbColors.amber,
        l10n.offlineQueued(pending),
        _Dot(color: FbColors.amber),
      ),
      _ => (FbColors.gableGreen, l10n.online, _Dot(color: FbColors.gableGreen)),
    };

    return Semantics(
      button: true,
      label: label,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(FbSizes.radius),
        child: Container(
          constraints: const BoxConstraints(minHeight: 40),
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              leading,
              const SizedBox(width: 8),
              Text(
                label,
                style: TextStyle(
                  color: color,
                  fontWeight: FontWeight.w600,
                  fontSize: 13,
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _Dot extends StatelessWidget {
  const _Dot({required this.color});

  final Color color;

  @override
  Widget build(BuildContext context) => Container(
    width: 10,
    height: 10,
    decoration: BoxDecoration(color: color, shape: BoxShape.circle),
  );
}
