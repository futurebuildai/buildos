import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:intl/intl.dart';

import '../l10n/app_localizations.dart';
import '../models/equipment_asset.dart';
import '../providers/app_providers.dart';
import '../theme/app_theme.dart';
import '../widgets/fb_sync_chip.dart';
import 'sync_status_screen.dart';

/// Read-only "equipment on my projects" (Phase 4a-ii). Shows the fleet assets
/// currently allocated to a site the field worker is assigned to — name, type,
/// status, serial, and the allocation window. Data rides the field/sync pull
/// (server-wins full-replace cache); pull-to-refresh re-syncs.
class EquipmentScreen extends ConsumerWidget {
  const EquipmentScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final equipment = ref.watch(equipmentProvider);

    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.equipmentTitle),
        actions: [
          Padding(
            padding: const EdgeInsets.only(right: FbSizes.gap),
            child: Center(
              child: FbSyncChip(
                onTap: () => Navigator.of(context).push(
                  MaterialPageRoute<void>(
                    builder: (_) => const SyncStatusScreen(),
                  ),
                ),
              ),
            ),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: () => ref.read(syncControllerProvider.notifier).syncNow(),
        child: equipment.when(
          loading: () => const Center(child: CircularProgressIndicator()),
          error: (_, _) => _Centered(child: Text(l10n.genericError)),
          data: (list) {
            if (list.isEmpty) {
              return _Centered(
                child: Text(
                  l10n.equipmentEmpty,
                  textAlign: TextAlign.center,
                  style: const TextStyle(color: FbColors.textSecondary),
                ),
              );
            }
            return ListView.separated(
              padding: const EdgeInsets.all(FbSizes.gap),
              itemCount: list.length,
              separatorBuilder: (_, _) => const SizedBox(height: FbSizes.gap),
              itemBuilder: (_, i) => _EquipmentCard(asset: list[i]),
            );
          },
        ),
      ),
    );
  }
}

class _EquipmentCard extends StatelessWidget {
  const _EquipmentCard({required this.asset});

  final EquipmentAsset asset;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    // Status is never colour-only — each colour is paired with a dot + text.
    // 'unavailable' uses textSecondary (≥7:1), not the muted slate token, which
    // is sub-AA on the dark card (sunlight-readability is a field a11y rule).
    final (Color color, String label) = switch (asset.status) {
      'available' => (FbColors.gableGreen, l10n.equipmentStatusAvailable),
      'maintenance' => (FbColors.amber, l10n.equipmentStatusMaintenance),
      'unavailable' => (
        FbColors.textSecondary,
        l10n.equipmentStatusUnavailable,
      ),
      _ => (FbColors.textSecondary, asset.status),
    };
    // Locale-aware months (ES → Spanish), and format the UTC value directly: the
    // dates are calendar DATEs (midnight UTC), so .toLocal() would roll them back
    // a day in every Americas timezone.
    final df = DateFormat.MMMd(Localizations.localeOf(context).toString());

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(FbSizes.gap),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    asset.name,
                    style: const TextStyle(
                      fontSize: 18,
                      fontWeight: FontWeight.w600,
                      color: FbColors.textPrimary,
                    ),
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 10,
                    vertical: 4,
                  ),
                  decoration: BoxDecoration(
                    color: color.withValues(alpha: 0.16),
                    borderRadius: BorderRadius.circular(FbSizes.radius),
                  ),
                  child: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Container(
                        width: 8,
                        height: 8,
                        decoration: BoxDecoration(
                          color: color,
                          shape: BoxShape.circle,
                        ),
                      ),
                      const SizedBox(width: 6),
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
              ],
            ),
            const SizedBox(height: FbSizes.gapSmall),
            Text(
              asset.assetType,
              style: const TextStyle(color: FbColors.textSecondary),
            ),
            if (asset.serialNumber != null &&
                asset.serialNumber!.isNotEmpty) ...[
              const SizedBox(height: 2),
              Text(
                l10n.equipmentSerial(asset.serialNumber!),
                style: const TextStyle(
                  fontFamily: fbMonoFamily,
                  color: FbColors.textSecondary,
                  fontSize: 13,
                ),
              ),
            ],
            const SizedBox(height: FbSizes.gapSmall),
            Row(
              children: [
                const Icon(
                  Icons.event_outlined,
                  size: 16,
                  color: FbColors.textSecondary,
                ),
                const SizedBox(width: 6),
                Text(
                  // end_date is EXCLUSIVE ([start, end)) — show end-1 as the
                  // inclusive last on-site day. UTC fields, no .toLocal().
                  '${l10n.equipmentOnSite}: ${df.format(asset.startDate)} – ${df.format(asset.endDate.subtract(const Duration(days: 1)))}',
                  style: const TextStyle(color: FbColors.textSecondary),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}

class _Centered extends StatelessWidget {
  const _Centered({required this.child});

  final Widget child;

  @override
  Widget build(BuildContext context) => ListView(
    // ListView so RefreshIndicator still works on the empty/error state.
    children: [
      const SizedBox(height: 120),
      Center(child: child),
    ],
  );
}
