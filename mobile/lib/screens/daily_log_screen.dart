import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:uuid/uuid.dart';

import '../database/database.dart';
import '../services/sync_service.dart';
import '../theme/app_theme.dart';

/// Daily log submission form for field workers.
///
/// Fields: project selector, weather conditions, work summary,
/// safety notes, hours worked.
/// Submit saves to Outbox for server sync.
class DailyLogScreen extends StatefulWidget {
  const DailyLogScreen({super.key});

  @override
  State<DailyLogScreen> createState() => _DailyLogScreenState();
}

class _DailyLogScreenState extends State<DailyLogScreen> {
  final _formKey = GlobalKey<FormState>();
  final _summaryController = TextEditingController();
  final _safetyNotesController = TextEditingController();
  final _hoursController = TextEditingController();

  String _selectedProject = '';
  String _weatherCondition = 'clear';
  bool _isSubmitting = false;

  List<String> _projectIds = [];

  static const List<String> _weatherOptions = [
    'clear',
    'cloudy',
    'rain',
    'snow',
    'wind',
    'extreme_heat',
    'extreme_cold',
    'fog',
  ];

  @override
  void initState() {
    super.initState();
    _loadProjects();
  }

  Future<void> _loadProjects() async {
    final db = Provider.of<AppDatabase>(context, listen: false);
    final tasks = await db.getAllTasks();
    final projects = <String>{};
    for (final task in tasks) {
      final pid = task['project_id'] as String?;
      if (pid != null && pid.isNotEmpty) {
        projects.add(pid);
      }
    }
    setState(() {
      _projectIds = projects.toList()..sort();
      if (_projectIds.isNotEmpty && _selectedProject.isEmpty) {
        _selectedProject = _projectIds.first;
      }
    });
  }

  @override
  void dispose() {
    _summaryController.dispose();
    _safetyNotesController.dispose();
    _hoursController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Daily Log'),
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Form(
          key: _formKey,
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Header.
              Text(
                'Submit Daily Report',
                style: Theme.of(context).textTheme.headlineMedium,
              ),
              const SizedBox(height: 4),
              Text(
                _formattedDate(),
                style: AppTheme.monoStyle(
                  fontSize: 14,
                  color: AppTheme.onSurfaceVariant,
                ),
              ),
              const SizedBox(height: 24),

              // Project selector.
              Text(
                'Project',
                style: Theme.of(context).textTheme.labelLarge,
              ),
              const SizedBox(height: 8),
              _buildProjectDropdown(),
              const SizedBox(height: 20),

              // Weather conditions.
              Text(
                'Weather Conditions',
                style: Theme.of(context).textTheme.labelLarge,
              ),
              const SizedBox(height: 8),
              _buildWeatherSelector(),
              const SizedBox(height: 20),

              // Work summary.
              Text(
                'Work Summary',
                style: Theme.of(context).textTheme.labelLarge,
              ),
              const SizedBox(height: 8),
              TextFormField(
                controller: _summaryController,
                maxLines: 4,
                decoration: const InputDecoration(
                  hintText: 'Describe work completed today...',
                ),
                validator: (value) {
                  if (value == null || value.trim().isEmpty) {
                    return 'Work summary is required';
                  }
                  return null;
                },
              ),
              const SizedBox(height: 20),

              // Safety notes.
              Text(
                'Safety Notes',
                style: Theme.of(context).textTheme.labelLarge,
              ),
              const SizedBox(height: 8),
              TextFormField(
                controller: _safetyNotesController,
                maxLines: 3,
                decoration: const InputDecoration(
                  hintText: 'Any safety observations or incidents...',
                ),
              ),
              const SizedBox(height: 20),

              // Hours worked.
              Text(
                'Hours Worked',
                style: Theme.of(context).textTheme.labelLarge,
              ),
              const SizedBox(height: 8),
              TextFormField(
                controller: _hoursController,
                keyboardType:
                    const TextInputType.numberWithOptions(decimal: true),
                decoration: const InputDecoration(
                  hintText: '8.0',
                  suffixText: 'hrs',
                ),
                style: AppTheme.monoStyle(fontSize: 16),
                validator: (value) {
                  if (value == null || value.trim().isEmpty) {
                    return 'Hours worked is required';
                  }
                  final hours = double.tryParse(value.trim());
                  if (hours == null || hours <= 0 || hours > 24) {
                    return 'Enter a valid number of hours (0-24)';
                  }
                  return null;
                },
              ),
              const SizedBox(height: 32),

              // Submit button.
              SizedBox(
                width: double.infinity,
                child: ElevatedButton(
                  onPressed: _isSubmitting ? null : _submit,
                  child: _isSubmitting
                      ? const SizedBox(
                          width: 20,
                          height: 20,
                          child: CircularProgressIndicator(
                            strokeWidth: 2,
                            color: AppTheme.onPrimary,
                          ),
                        )
                      : const Text('Submit Daily Log'),
                ),
              ),
              const SizedBox(height: 24),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildProjectDropdown() {
    if (_projectIds.isEmpty) {
      return Container(
        width: double.infinity,
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: AppTheme.surface2,
          borderRadius: BorderRadius.circular(10),
          border: Border.all(color: AppTheme.outlineVariant),
        ),
        child: Text(
          'No projects available. Sync to load projects.',
          style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                color: AppTheme.onSurfaceVariant,
              ),
        ),
      );
    }

    return DropdownButtonFormField<String>(
      initialValue: _selectedProject.isNotEmpty ? _selectedProject : null,
      dropdownColor: AppTheme.surface3,
      decoration: const InputDecoration(),
      items: _projectIds.map((pid) {
        return DropdownMenuItem(
          value: pid,
          child: Text(
            pid,
            style: AppTheme.monoStyle(fontSize: 14),
          ),
        );
      }).toList(),
      onChanged: (v) {
        if (v != null) {
          setState(() {
            _selectedProject = v;
          });
        }
      },
      validator: (v) {
        if (v == null || v.isEmpty) {
          return 'Select a project';
        }
        return null;
      },
    );
  }

  Widget _buildWeatherSelector() {
    return Wrap(
      spacing: 8,
      runSpacing: 8,
      children: _weatherOptions.map((weather) {
        final isSelected = _weatherCondition == weather;
        return ChoiceChip(
          label: Text(
            _weatherLabel(weather),
            style: TextStyle(
              color: isSelected ? AppTheme.onPrimary : AppTheme.onBackground,
              fontWeight: isSelected ? FontWeight.w600 : FontWeight.w400,
            ),
          ),
          selected: isSelected,
          selectedColor: AppTheme.gableGreen,
          backgroundColor: AppTheme.surface2,
          side: BorderSide(
            color: isSelected ? AppTheme.gableGreen : AppTheme.outlineVariant,
          ),
          onSelected: (selected) {
            if (selected) {
              setState(() {
                _weatherCondition = weather;
              });
            }
          },
        );
      }).toList(),
    );
  }

  String _weatherLabel(String weather) {
    switch (weather) {
      case 'clear':
        return 'Clear';
      case 'cloudy':
        return 'Cloudy';
      case 'rain':
        return 'Rain';
      case 'snow':
        return 'Snow';
      case 'wind':
        return 'Wind';
      case 'extreme_heat':
        return 'Extreme Heat';
      case 'extreme_cold':
        return 'Extreme Cold';
      case 'fog':
        return 'Fog';
      default:
        return weather;
    }
  }

  String _formattedDate() {
    final now = DateTime.now();
    final months = [
      'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
      'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
    ];
    final weekdays = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];
    return '${weekdays[now.weekday - 1]}, ${months[now.month - 1]} ${now.day}, ${now.year}';
  }

  Future<void> _submit() async {
    if (!_formKey.currentState!.validate()) return;
    if (_selectedProject.isEmpty) return;

    setState(() {
      _isSubmitting = true;
    });

    try {
      final db = Provider.of<AppDatabase>(context, listen: false);
      const uuid = Uuid();

      final payload = {
        'project_id': _selectedProject,
        'weather': _weatherCondition,
        'summary': _summaryController.text.trim(),
        'safety_notes': _safetyNotesController.text.trim(),
        'hours_worked': double.parse(_hoursController.text.trim()),
        'date': DateTime.now().toUtc().toIso8601String(),
      };

      await db.insertOutboxEntry(
        id: uuid.v4(),
        actionType: 'daily_log',
        payloadJson: jsonEncode(payload),
        idempotencyKey: uuid.v4(),
        createdAt: DateTime.now().toUtc().toIso8601String(),
      );

      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Daily log saved. Will sync when online.'),
            duration: Duration(seconds: 2),
          ),
        );

        // Clear the form.
        _summaryController.clear();
        _safetyNotesController.clear();
        _hoursController.clear();
        setState(() {
          _weatherCondition = 'clear';
        });
      }

      // Attempt sync.
      if (mounted) {
        final syncService = Provider.of<SyncService>(context, listen: false);
        syncService.fullSync();
      }
    } finally {
      if (mounted) {
        setState(() {
          _isSubmitting = false;
        });
      }
    }
  }
}
