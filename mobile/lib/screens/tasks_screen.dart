import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../l10n/app_localizations.dart';
import '../models/project_task.dart';
import '../providers/app_providers.dart';
import '../theme/app_theme.dart';

/// Priority-sorted task list (UX_CORE_SCREENS field view). Each row carries a
/// slider-to-complete plus a one-tap "mark done"; both append to the offline
/// outbox so they work with no signal.
class TasksScreen extends ConsumerWidget {
  const TasksScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final tasks = ref.watch(tasksProvider);

    return RefreshIndicator(
      onRefresh: () => ref.read(syncControllerProvider.notifier).syncNow(),
      child: tasks.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (_, _) => _Centered(child: Text(l10n.genericError)),
        data: (list) {
          if (list.isEmpty) {
            return _Centered(
              child: Text(
                l10n.tasksEmpty,
                style: const TextStyle(color: FbColors.textSecondary),
              ),
            );
          }
          return ListView.separated(
            padding: const EdgeInsets.all(FbSizes.gap),
            itemCount: list.length,
            separatorBuilder: (_, _) => const SizedBox(height: FbSizes.gap),
            itemBuilder: (_, i) => _TaskCard(task: list[i]),
          );
        },
      ),
    );
  }
}

class _Centered extends StatelessWidget {
  const _Centered({required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context) => ListView(
    children: [
      const SizedBox(height: 160),
      Center(child: child),
    ],
  );
}

class _TaskCard extends ConsumerStatefulWidget {
  const _TaskCard({required this.task});
  final ProjectTask task;
  @override
  ConsumerState<_TaskCard> createState() => _TaskCardState();
}

class _TaskCardState extends ConsumerState<_TaskCard> {
  late double _pct = widget.task.percentComplete.toDouble();
  bool _dirty = false;

  Future<void> _queue(AppLocalizations l10n, int pct) async {
    await ref
        .read(syncServiceProvider)
        .queueProgress(taskId: widget.task.id, percentComplete: pct);
    setState(() {
      _pct = pct.toDouble();
      _dirty = false;
    });
    if (!mounted) return;
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text(l10n.queuedForSync)));
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final task = widget.task;
    final (Color tag, String tagText)? marker = task.isCritical
        ? (FbColors.safetyRed, l10n.criticalPath)
        : task.isNearCritical && task.totalFloat != null
        ? (FbColors.amber, l10n.floatDays(task.totalFloat!))
        : task.totalFloat != null
        ? (FbColors.blueprintBlue, l10n.floatDays(task.totalFloat!))
        : null;

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(FbSizes.gap),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Text(
                  task.wbsCode,
                  style: const TextStyle(
                    fontFamily: fbMonoFamily,
                    color: FbColors.textSecondary,
                  ),
                ),
                const Spacer(),
                if (marker != null)
                  Container(
                    padding: const EdgeInsets.symmetric(
                      horizontal: 8,
                      vertical: 4,
                    ),
                    decoration: BoxDecoration(
                      color: marker.$1.withValues(alpha: 0.16),
                      borderRadius: BorderRadius.circular(FbSizes.radius),
                    ),
                    child: Text(
                      marker.$2,
                      style: TextStyle(
                        color: marker.$1,
                        fontWeight: FontWeight.w600,
                        fontSize: 12,
                      ),
                    ),
                  ),
              ],
            ),
            const SizedBox(height: FbSizes.gapSmall),
            Text(
              task.name,
              style: const TextStyle(
                fontSize: 18,
                fontWeight: FontWeight.w600,
                color: FbColors.textPrimary,
              ),
            ),
            const SizedBox(height: FbSizes.gap),
            Text(
              l10n.percentComplete(_pct.round()),
              style: const TextStyle(
                fontFamily: fbMonoFamily,
                color: FbColors.gableGreen,
                fontWeight: FontWeight.w600,
              ),
            ),
            Slider(
              value: _pct.clamp(0, 100),
              max: 100,
              divisions: 20,
              label: '${_pct.round()}%',
              activeColor: FbColors.gableGreen,
              onChanged: task.isComplete
                  ? null
                  : (v) => setState(() {
                      _pct = v;
                      _dirty = true;
                    }),
            ),
            Row(
              children: [
                if (_dirty)
                  Expanded(
                    child: OutlinedButton(
                      onPressed: () => _queue(l10n, _pct.round()),
                      child: Text(l10n.slideToComplete),
                    ),
                  ),
                if (_dirty) const SizedBox(width: FbSizes.gapSmall),
                Expanded(
                  child: FilledButton(
                    onPressed: task.isComplete ? null : () => _queue(l10n, 100),
                    child: Text(l10n.markDone),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
