/// Mirrors internal/models.ProjectTask (CPM schedule row).
///
/// `totalFloat` is schedule slack in DAYS (physics.BackwardPass) — not money.
class ProjectTask {
  const ProjectTask({
    required this.id,
    required this.projectId,
    required this.wbsCode,
    required this.name,
    required this.durationDays,
    required this.isCritical,
    required this.status,
    required this.percentComplete,
    this.totalFloat,
    this.earlyStart,
    this.earlyFinish,
    this.assignedCrew = const [],
  });

  final String id;
  final String projectId;
  final String wbsCode;
  final String name;
  final int durationDays;
  final bool isCritical;
  final String status;
  final int percentComplete;
  final int? totalFloat;
  final String? earlyStart;
  final String? earlyFinish;
  final List<String> assignedCrew;

  factory ProjectTask.fromJson(Map<String, dynamic> json) => ProjectTask(
    id: json['id'] as String? ?? '',
    projectId: json['project_id'] as String? ?? '',
    wbsCode: json['wbs_code'] as String? ?? '',
    name: json['name'] as String? ?? '',
    durationDays: (json['duration_days'] as num?)?.toInt() ?? 0,
    isCritical: json['is_critical'] as bool? ?? false,
    status: json['status'] as String? ?? 'pending',
    percentComplete: (json['percent_complete'] as num?)?.toInt() ?? 0,
    totalFloat: (json['total_float'] as num?)?.toInt(),
    earlyStart: json['early_start'] as String?,
    earlyFinish: json['early_finish'] as String?,
    assignedCrew:
        (json['assigned_crew'] as List?)?.map((e) => e.toString()).toList() ??
        const [],
  );

  /// Near-critical when float is small (product constant; default ≤2 days).
  bool get isNearCritical =>
      !isCritical && totalFloat != null && totalFloat! <= 2;

  bool get isComplete => percentComplete >= 100 || status == 'completed';
}
