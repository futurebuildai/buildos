import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:uuid/uuid.dart';

import '../database/database.dart';
import '../services/sync_service.dart';
import '../theme/app_theme.dart';

/// GPS check-in screen for field workers.
///
/// Shows current GPS coordinates (simulated), a project selector dropdown,
/// and a "Check In" button that saves to Outbox with coordinates.
/// Displays timestamp of last check-in.
class CrewCheckinScreen extends StatefulWidget {
  const CrewCheckinScreen({super.key});

  @override
  State<CrewCheckinScreen> createState() => _CrewCheckinScreenState();
}

class _CrewCheckinScreenState extends State<CrewCheckinScreen> {
  String _selectedProject = '';
  List<String> _projectIds = [];
  bool _isCheckingIn = false;
  String? _lastCheckinTime;

  // Simulated GPS coordinates (real GPS would use geolocator package).
  double _latitude = 40.7128;
  double _longitude = -74.0060;
  bool _locationAcquired = false;

  @override
  void initState() {
    super.initState();
    _loadProjects();
    _acquireLocation();
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

  Future<void> _acquireLocation() async {
    // Simulate GPS acquisition delay.
    await Future<void>.delayed(const Duration(milliseconds: 800));
    if (mounted) {
      setState(() {
        // In production, use geolocator package for real GPS.
        _latitude = 40.7128;
        _longitude = -74.0060;
        _locationAcquired = true;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Crew Check-In'),
      ),
      body: SingleChildScrollView(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // GPS status card.
            _buildLocationCard(),
            const SizedBox(height: 24),

            // Project selector.
            Text(
              'Project',
              style: Theme.of(context).textTheme.labelLarge,
            ),
            const SizedBox(height: 8),
            _buildProjectDropdown(),
            const SizedBox(height: 32),

            // Check-in button.
            SizedBox(
              width: double.infinity,
              child: ElevatedButton.icon(
                onPressed:
                    (_isCheckingIn || !_locationAcquired) ? null : _checkIn,
                icon: _isCheckingIn
                    ? const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: AppTheme.onPrimary,
                        ),
                      )
                    : const Icon(Icons.location_on),
                label: Text(_isCheckingIn ? 'Checking In...' : 'Check In'),
              ),
            ),
            const SizedBox(height: 32),

            // Last check-in timestamp.
            if (_lastCheckinTime != null) _buildLastCheckin(),
          ],
        ),
      ),
    );
  }

  Widget _buildLocationCard() {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(
                  _locationAcquired ? Icons.gps_fixed : Icons.gps_not_fixed,
                  color: _locationAcquired
                      ? AppTheme.gableGreen
                      : AppTheme.amberWarning,
                ),
                const SizedBox(width: 12),
                Text(
                  _locationAcquired
                      ? 'Location Acquired'
                      : 'Acquiring Location...',
                  style: Theme.of(context).textTheme.titleMedium?.copyWith(
                        color: _locationAcquired
                            ? AppTheme.gableGreen
                            : AppTheme.amberWarning,
                      ),
                ),
              ],
            ),
            const SizedBox(height: 16),
            if (_locationAcquired) ...[
              _buildCoordinateRow('Latitude', _latitude),
              const SizedBox(height: 8),
              _buildCoordinateRow('Longitude', _longitude),
            ] else
              const LinearProgressIndicator(
                color: AppTheme.amberWarning,
                backgroundColor: AppTheme.surface2,
              ),
          ],
        ),
      ),
    );
  }

  Widget _buildCoordinateRow(String label, double value) {
    return Row(
      children: [
        SizedBox(
          width: 80,
          child: Text(
            label,
            style: Theme.of(context).textTheme.bodySmall,
          ),
        ),
        Text(
          value.toStringAsFixed(6),
          style: AppTheme.monoStyle(
            fontSize: 16,
            fontWeight: FontWeight.w600,
            color: AppTheme.blueprintBlue,
          ),
        ),
      ],
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
    );
  }

  Widget _buildLastCheckin() {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          children: [
            const Icon(
              Icons.check_circle_outline,
              color: AppTheme.gableGreen,
            ),
            const SizedBox(width: 12),
            Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  'Last Check-In',
                  style: Theme.of(context).textTheme.labelMedium,
                ),
                const SizedBox(height: 2),
                Text(
                  _lastCheckinTime!,
                  style: AppTheme.monoStyle(
                    fontSize: 13,
                    color: AppTheme.gableGreen,
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _checkIn() async {
    if (_selectedProject.isEmpty || !_locationAcquired) return;

    setState(() {
      _isCheckingIn = true;
    });

    try {
      final db = Provider.of<AppDatabase>(context, listen: false);
      const uuid = Uuid();
      final now = DateTime.now().toUtc();

      final payload = {
        'project_id': _selectedProject,
        'latitude': _latitude,
        'longitude': _longitude,
        'checked_in_at': now.toIso8601String(),
      };

      await db.insertOutboxEntry(
        id: uuid.v4(),
        actionType: 'crew_checkin',
        payloadJson: jsonEncode(payload),
        idempotencyKey: uuid.v4(),
        createdAt: now.toIso8601String(),
      );

      setState(() {
        _lastCheckinTime = _formatTimestamp(now);
      });

      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            content: Text('Check-in saved. Will sync when online.'),
            duration: Duration(seconds: 2),
          ),
        );
      }

      // Attempt sync.
      if (mounted) {
        final syncService = Provider.of<SyncService>(context, listen: false);
        syncService.fullSync();
      }
    } finally {
      if (mounted) {
        setState(() {
          _isCheckingIn = false;
        });
      }
    }
  }

  String _formatTimestamp(DateTime dt) {
    final local = dt.toLocal();
    final hour = local.hour.toString().padLeft(2, '0');
    final minute = local.minute.toString().padLeft(2, '0');
    final second = local.second.toString().padLeft(2, '0');
    return '${local.year}-${local.month.toString().padLeft(2, '0')}-${local.day.toString().padLeft(2, '0')} $hour:$minute:$second';
  }
}
