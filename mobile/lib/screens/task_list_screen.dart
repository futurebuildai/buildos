import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:uuid/uuid.dart';

import '../database/database.dart';
import '../services/sync_service.dart';
import '../theme/app_theme.dart';

/// Main screen showing assigned tasks grouped by project.
///
/// Each tile shows: task name, WBS code, percent complete (circular indicator),
/// status chip. Tap to report progress via bottom sheet.
/// Pull-to-refresh triggers a full sync.
class TaskListScreen extends StatefulWidget {
  const TaskListScreen({super.key});

  @override
  State<TaskListScreen> createState() => _TaskListScreenState();
}

class _TaskListScreenState extends State<TaskListScreen> {
  @override
  Widget build(BuildContext context) {
    final db = Provider.of<AppDatabase>(context, listen: false);

    return Scaffold(
      appBar: AppBar(
        title: const Text('My Tasks'),
      ),
      body: StreamBuilder<List<Map<String, dynamic>>>(
        stream: db.watchAllTasks(),
        builder: (context, snapshot) {
          if (snapshot.connectionState == ConnectionState.waiting &&
              !snapshot.hasData) {
            return const Center(
              child: CircularProgressIndicator(
                color: AppTheme.gableGreen,
              ),
            );
          }

          final tasks = snapshot.data ?? [];

          if (tasks.isEmpty) {
            return _buildEmptyState();
          }

          // Group tasks by project_id.
          final grouped = <String, List<Map<String, dynamic>>>{};
          for (final task in tasks) {
            final projectId = task['project_id'] as String? ?? 'Unknown';
            grouped.putIfAbsent(projectId, () => []).add(task);
          }

          final projectIds = grouped.keys.toList()..sort();

          return RefreshIndicator(
            color: AppTheme.gableGreen,
            backgroundColor: AppTheme.surface1,
            onRefresh: () async {
              final syncService =
                  Provider.of<SyncService>(context, listen: false);
              await syncService.fullSync();
            },
            child: ListView.builder(
              padding: const EdgeInsets.only(top: 8, bottom: 80),
              itemCount: projectIds.length,
              itemBuilder: (context, index) {
                final projectId = projectIds[index];
                final projectTasks = grouped[projectId]!;
                return _buildProjectGroup(projectId, projectTasks);
              },
            ),
          );
        },
      ),
    );
  }

  Widget _buildEmptyState() {
    return Center(
      child: Column(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Icon(
            Icons.assignment_outlined,
            size: 64,
            color: AppTheme.onSurfaceVariant.withValues(alpha: 0.5),
          ),
          const SizedBox(height: 16),
          Text(
            'No tasks assigned',
            style: Theme.of(context).textTheme.titleMedium?.copyWith(
                  color: AppTheme.onSurfaceVariant,
                ),
          ),
          const SizedBox(height: 8),
          Text(
            'Pull down to sync with server',
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ],
      ),
    );
  }

  Widget _buildProjectGroup(
      String projectId, List<Map<String, dynamic>> tasks) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 16, 16, 4),
          child: Row(
            children: [
              const Icon(Icons.folder_outlined,
                  size: 16, color: AppTheme.blueprintBlue),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  'Project: $projectId',
                  style: Theme.of(context).textTheme.labelLarge?.copyWith(
                        color: AppTheme.blueprintBlue,
                      ),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
              Text(
                '${tasks.length} task${tasks.length == 1 ? '' : 's'}',
                style: Theme.of(context).textTheme.labelSmall,
              ),
            ],
          ),
        ),
        ...tasks.map((task) => _buildTaskTile(task)),
      ],
    );
  }

  Widget _buildTaskTile(Map<String, dynamic> task) {
    final name = task['name'] as String? ?? 'Unnamed Task';
    final wbsCode = task['wbs_code'] as String? ?? '';
    final percentComplete = task['percent_complete'] as int? ?? 0;
    final status = task['status'] as String? ?? 'pending';
    final priority = task['priority'] as String? ?? 'normal';

    return Card(
      child: InkWell(
        onTap: () => _showProgressSheet(task),
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              // Circular progress indicator.
              SizedBox(
                width: 48,
                height: 48,
                child: Stack(
                  alignment: Alignment.center,
                  children: [
                    CircularProgressIndicator(
                      value: percentComplete / 100.0,
                      backgroundColor: AppTheme.surface2,
                      color: _progressColor(percentComplete),
                      strokeWidth: 4,
                    ),
                    Text(
                      '$percentComplete%',
                      style: AppTheme.monoStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w600,
                        color: _progressColor(percentComplete),
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 16),
              // Task details.
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      name,
                      style: Theme.of(context).textTheme.titleMedium,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 4),
                    Row(
                      children: [
                        Text(
                          wbsCode,
                          style: AppTheme.monoStyle(
                            fontSize: 12,
                            color: AppTheme.onSurfaceVariant,
                          ),
                        ),
                        if (priority != 'normal') ...[
                          const SizedBox(width: 8),
                          _buildPriorityChip(priority),
                        ],
                      ],
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              _buildStatusChip(status),
            ],
          ),
        ),
      ),
    );
  }

  Color _progressColor(int percent) {
    if (percent >= 100) return AppTheme.gableGreen;
    if (percent >= 50) return AppTheme.blueprintBlue;
    if (percent >= 25) return AppTheme.amberWarning;
    return AppTheme.onSurfaceVariant;
  }

  Widget _buildStatusChip(String status) {
    Color chipColor;
    switch (status) {
      case 'complete':
      case 'completed':
        chipColor = AppTheme.gableGreen;
      case 'in_progress':
      case 'active':
        chipColor = AppTheme.blueprintBlue;
      case 'blocked':
        chipColor = AppTheme.safetyRed;
      default:
        chipColor = AppTheme.onSurfaceVariant;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: chipColor.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: chipColor.withValues(alpha: 0.3)),
      ),
      child: Text(
        status.replaceAll('_', ' '),
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w600,
          color: chipColor,
        ),
      ),
    );
  }

  Widget _buildPriorityChip(String priority) {
    Color chipColor;
    switch (priority) {
      case 'critical':
        chipColor = AppTheme.safetyRed;
      case 'high':
        chipColor = AppTheme.amberWarning;
      default:
        chipColor = AppTheme.onSurfaceVariant;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: chipColor.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        priority.toUpperCase(),
        style: TextStyle(fontSize: 10, fontWeight: FontWeight.w700, color: chipColor),
      ),
    );
  }

  void _showProgressSheet(Map<String, dynamic> task) {
    final taskId = task['id'] as String? ?? '';
    final name = task['name'] as String? ?? 'Unnamed Task';
    final currentPercent = task['percent_complete'] as int? ?? 0;

    double sliderValue = currentPercent.toDouble();
    final notesController = TextEditingController();

    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      builder: (ctx) {
        return StatefulBuilder(
          builder: (ctx, setSheetState) {
            return Padding(
              padding: EdgeInsets.fromLTRB(
                24,
                24,
                24,
                24 + MediaQuery.of(ctx).viewInsets.bottom,
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // Drag handle.
                  Center(
                    child: Container(
                      width: 40,
                      height: 4,
                      decoration: BoxDecoration(
                        color: AppTheme.onSurfaceVariant.withValues(alpha: 0.3),
                        borderRadius: BorderRadius.circular(2),
                      ),
                    ),
                  ),
                  const SizedBox(height: 20),
                  Text(
                    'Report Progress',
                    style: Theme.of(ctx).textTheme.titleLarge,
                  ),
                  const SizedBox(height: 4),
                  Text(
                    name,
                    style: Theme.of(ctx).textTheme.bodyMedium?.copyWith(
                          color: AppTheme.onSurfaceVariant,
                        ),
                  ),
                  const SizedBox(height: 24),
                  // Percent slider.
                  Row(
                    children: [
                      Text(
                        'Completion',
                        style: Theme.of(ctx).textTheme.labelLarge,
                      ),
                      const Spacer(),
                      Text(
                        '${sliderValue.round()}%',
                        style: AppTheme.monoStyle(
                          fontSize: 18,
                          fontWeight: FontWeight.w700,
                          color: AppTheme.gableGreen,
                        ),
                      ),
                    ],
                  ),
                  Slider(
                    value: sliderValue,
                    min: 0,
                    max: 100,
                    divisions: 20,
                    activeColor: AppTheme.gableGreen,
                    inactiveColor: AppTheme.surface2,
                    onChanged: (v) {
                      setSheetState(() {
                        sliderValue = v;
                      });
                    },
                  ),
                  const SizedBox(height: 16),
                  // Notes field.
                  TextField(
                    controller: notesController,
                    maxLines: 3,
                    decoration: const InputDecoration(
                      labelText: 'Notes (optional)',
                      hintText: 'Add notes about progress...',
                    ),
                  ),
                  const SizedBox(height: 24),
                  // Submit button.
                  SizedBox(
                    width: double.infinity,
                    child: ElevatedButton(
                      onPressed: () async {
                        await _submitProgress(
                          taskId: taskId,
                          percentComplete: sliderValue.round(),
                          notes: notesController.text,
                        );
                        if (ctx.mounted) {
                          Navigator.pop(ctx);
                        }
                      },
                      child: const Text('Submit Progress'),
                    ),
                  ),
                ],
              ),
            );
          },
        );
      },
    );
  }

  Future<void> _submitProgress({
    required String taskId,
    required int percentComplete,
    required String notes,
  }) async {
    final db = Provider.of<AppDatabase>(context, listen: false);
    const uuid = Uuid();

    // Update local task immediately for responsive UI.
    await db.updateTaskPercent(taskId, percentComplete);

    // Queue the progress report in the outbox for server sync.
    final payload = {
      'task_id': taskId,
      'percent_complete': percentComplete,
      'notes': notes,
      'reported_at': DateTime.now().toUtc().toIso8601String(),
    };

    await db.insertOutboxEntry(
      id: uuid.v4(),
      actionType: 'task_progress',
      payloadJson: jsonEncode(payload),
      idempotencyKey: uuid.v4(),
      createdAt: DateTime.now().toUtc().toIso8601String(),
    );

    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Progress saved. Will sync when online.'),
          duration: Duration(seconds: 2),
        ),
      );
    }

    // Attempt immediate sync if online.
    if (mounted) {
      final syncService = Provider.of<SyncService>(context, listen: false);
      syncService.fullSync(); // Fire and forget.
    }
  }
}
