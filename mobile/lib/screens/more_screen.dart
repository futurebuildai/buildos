import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../l10n/app_localizations.dart';
import '../theme/app_theme.dart';
import 'check_in_screen.dart';
import 'equipment_screen.dart';
import 'profile_screen.dart';
import 'schedule_screen.dart';
import 'sync_status_screen.dart';

/// "More" tab: the secondary destinations that don't earn a bottom-nav slot —
/// Crew check-in, Equipment, read-only Schedule, Sync Status, and Profile.
class MoreScreen extends ConsumerWidget {
  const MoreScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);

    void push(Widget screen) => Navigator.of(
      context,
    ).push(MaterialPageRoute<void>(builder: (_) => screen));

    Widget tile(IconData icon, String label, VoidCallback onTap) => ListTile(
      leading: Icon(icon, color: FbColors.gableGreen),
      title: Text(label, style: const TextStyle(fontSize: 17)),
      trailing: const Icon(Icons.chevron_right),
      minVerticalPadding: 18,
      onTap: onTap,
    );

    return ListView(
      children: [
        tile(
          Icons.how_to_reg_outlined,
          l10n.crewCheckIn,
          () => push(const CheckInScreen()),
        ),
        const Divider(height: 1),
        tile(
          Icons.precision_manufacturing_outlined,
          l10n.equipmentTitle,
          () => push(const EquipmentScreen()),
        ),
        const Divider(height: 1),
        tile(
          Icons.timeline,
          l10n.scheduleTitle,
          () => push(const ScheduleScreen()),
        ),
        const Divider(height: 1),
        tile(
          Icons.sync,
          l10n.syncStatusTitle,
          () => push(const SyncStatusScreen()),
        ),
        const Divider(height: 1),
        tile(
          Icons.person_outline,
          l10n.profileTitle,
          () => push(const ProfileScreen()),
        ),
      ],
    );
  }
}
