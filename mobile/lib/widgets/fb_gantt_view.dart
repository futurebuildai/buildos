import 'package:flutter/material.dart';

import '../l10n/app_localizations.dart';
import '../models/project_task.dart';
import '../theme/app_theme.dart';

/// Read-only Gantt for the field (DESIGN_SYSTEM_COMPONENTS `FbGanttView`).
/// The field app never recalculates the schedule — it visualizes the
/// server-authoritative CPM result. Bars are laid out by `early_start` /
/// duration; the critical path is colored AND labeled (never color-only).
class FbGanttView extends StatelessWidget {
  const FbGanttView({super.key, required this.tasks});

  final List<ProjectTask> tasks;

  static const double _dayWidth = 28;
  static const double _rowHeight = 44;
  static const double _labelWidth = 140;

  DateTime? _parse(String? iso) => iso == null ? null : DateTime.tryParse(iso);

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final dated = [
      for (final t in tasks)
        if (_parse(t.earlyStart) != null) t,
    ];
    if (dated.isEmpty) {
      return Center(
        child: Text(
          l10n.tasksEmpty,
          style: const TextStyle(color: FbColors.textSecondary),
        ),
      );
    }

    final origin = dated
        .map((t) => _parse(t.earlyStart)!)
        .reduce((a, b) => a.isBefore(b) ? a : b);

    int offsetDays(ProjectTask t) =>
        _parse(t.earlyStart)!.difference(origin).inDays;

    final maxDay = dated
        .map((t) => offsetDays(t) + (t.durationDays < 1 ? 1 : t.durationDays))
        .reduce((a, b) => a > b ? a : b);

    return SingleChildScrollView(
      scrollDirection: Axis.horizontal,
      child: SizedBox(
        width: _labelWidth + maxDay * _dayWidth + FbSizes.gap,
        child: ListView.builder(
          itemCount: dated.length,
          itemBuilder: (_, i) {
            final t = dated[i];
            final start = offsetDays(t).toDouble();
            final span = (t.durationDays < 1 ? 1 : t.durationDays).toDouble();
            final color = t.isCritical
                ? FbColors.safetyRed
                : t.isNearCritical
                ? FbColors.amber
                : FbColors.gableGreen;
            return SizedBox(
              height: _rowHeight,
              child: Row(
                children: [
                  SizedBox(
                    width: _labelWidth,
                    child: Text(
                      t.wbsCode,
                      overflow: TextOverflow.ellipsis,
                      style: const TextStyle(
                        fontFamily: fbMonoFamily,
                        color: FbColors.textSecondary,
                      ),
                    ),
                  ),
                  SizedBox(width: start * _dayWidth),
                  Tooltip(
                    message:
                        '${t.name}${t.isCritical ? ' · ${l10n.criticalPath}' : ''}',
                    child: Container(
                      width: span * _dayWidth,
                      height: 24,
                      decoration: BoxDecoration(
                        color: color.withValues(alpha: 0.85),
                        borderRadius: BorderRadius.circular(6),
                        border: t.isCritical
                            ? Border.all(color: FbColors.safetyRed, width: 2)
                            : null,
                      ),
                      alignment: Alignment.centerLeft,
                      padding: const EdgeInsets.symmetric(horizontal: 6),
                      child: t.isCritical
                          ? const Icon(
                              Icons.priority_high,
                              size: 14,
                              color: FbColors.deepSpace,
                            )
                          : null,
                    ),
                  ),
                ],
              ),
            );
          },
        ),
      ),
    );
  }
}
