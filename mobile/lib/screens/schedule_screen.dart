import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../l10n/app_localizations.dart';
import '../providers/app_providers.dart';
import '../theme/app_theme.dart';
import '../widgets/fb_gantt_view.dart';

/// Read-only schedule (Gantt) for the field. The CPM result is computed
/// server-side; here we only visualize it.
class ScheduleScreen extends ConsumerWidget {
  const ScheduleScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final tasks = ref.watch(tasksProvider);

    return Scaffold(
      appBar: AppBar(title: Text(l10n.scheduleTitle)),
      body: Column(
        children: [
          Container(
            width: double.infinity,
            color: FbColors.slateSteelRaised,
            padding: const EdgeInsets.all(FbSizes.gapSmall),
            child: Text(
              l10n.scheduleReadOnly,
              textAlign: TextAlign.center,
              style: const TextStyle(color: FbColors.textSecondary),
            ),
          ),
          Expanded(
            child: tasks.when(
              loading: () => const Center(child: CircularProgressIndicator()),
              error: (_, _) => Center(child: Text(l10n.genericError)),
              data: (list) => Padding(
                padding: const EdgeInsets.all(FbSizes.gapSmall),
                child: FbGanttView(tasks: list),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
