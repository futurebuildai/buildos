import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:image_picker/image_picker.dart';
import 'package:intl/intl.dart';

import '../l10n/app_localizations.dart';
import '../providers/app_providers.dart';
import '../theme/app_theme.dart';

/// Daily Log capture (UX field flows): work summary + weather + safety, plus an
/// optional photo. Everything is queued to the outbox, so it submits with no
/// signal and drains later. (Crew check-in is its own screen — CheckInScreen.)
class DailyLogScreen extends ConsumerStatefulWidget {
  const DailyLogScreen({super.key});

  @override
  ConsumerState<DailyLogScreen> createState() => _DailyLogScreenState();
}

class _DailyLogScreenState extends ConsumerState<DailyLogScreen> {
  final _formKey = GlobalKey<FormState>();
  final _summary = TextEditingController();
  final _weather = TextEditingController();
  final _safety = TextEditingController();

  String? _projectId;
  final List<String> _photoPaths = [];
  bool _capturing = false;

  @override
  void dispose() {
    _summary.dispose();
    _weather.dispose();
    _safety.dispose();
    super.dispose();
  }

  Future<void> _capturePhoto() async {
    setState(() => _capturing = true);
    try {
      final picker = ImagePicker();
      final shot = await picker.pickImage(
        source: ImageSource.camera,
        maxWidth: 2048,
        imageQuality: 80,
      );
      if (shot != null) {
        setState(() => _photoPaths.add(shot.path));
      }
    } finally {
      if (mounted) setState(() => _capturing = false);
    }
  }

  Future<void> _submit(AppLocalizations l10n) async {
    final projectId = _projectId;
    if (projectId == null || !(_formKey.currentState?.validate() ?? false)) {
      return;
    }
    final sync = ref.read(syncServiceProvider);
    final today = DateFormat('yyyy-MM-dd').format(DateTime.now());

    await sync.queueDailyLog(
      projectId: projectId,
      logDate: today,
      workSummary: _summary.text.trim(),
      weatherConditions: _weather.text.trim().isEmpty
          ? null
          : _weather.text.trim(),
      safetyIncidents: _safety.text.trim().isEmpty ? null : _safety.text.trim(),
    );

    if (!mounted) return;
    _summary.clear();
    _weather.clear();
    _safety.clear();
    setState(() {
      _photoPaths.clear();
    });
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text(l10n.queuedForSync)));
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final tasks = ref.watch(tasksProvider).value ?? const [];
    final projectIds = {for (final t in tasks) t.projectId}.toList();
    // Reconcile against the live set, not a one-time `??=`: a background sync can
    // invalidate tasksProvider while this tab is open, leaving a stale selection
    // (a daily log filed against the wrong project) or tripping the dropdown's
    // value-in-items assert. See CheckInScreen for the same fix.
    if (_projectId == null || !projectIds.contains(_projectId)) {
      _projectId = projectIds.isNotEmpty ? projectIds.first : null;
    }

    return SingleChildScrollView(
      padding: const EdgeInsets.all(FbSizes.gap),
      child: Form(
        key: _formKey,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            if (projectIds.length > 1) ...[
              DropdownButtonFormField<String>(
                initialValue: _projectId,
                items: [
                  for (final id in projectIds)
                    DropdownMenuItem(value: id, child: Text(id)),
                ],
                onChanged: (v) => setState(() => _projectId = v),
                decoration: const InputDecoration(labelText: 'Project'),
              ),
              const SizedBox(height: FbSizes.gap),
            ],
            TextFormField(
              controller: _summary,
              minLines: 3,
              maxLines: 6,
              decoration: InputDecoration(labelText: l10n.workSummary),
              validator: (v) =>
                  (v == null || v.trim().isEmpty) ? l10n.workSummary : null,
            ),
            const SizedBox(height: FbSizes.gap),
            TextFormField(
              controller: _weather,
              decoration: InputDecoration(labelText: l10n.weatherConditions),
            ),
            const SizedBox(height: FbSizes.gap),
            TextFormField(
              controller: _safety,
              minLines: 2,
              maxLines: 4,
              decoration: InputDecoration(labelText: l10n.safetyIncidents),
            ),
            const SizedBox(height: FbSizes.gap),
            OutlinedButton.icon(
              onPressed: _capturing ? null : _capturePhoto,
              icon: const Icon(Icons.photo_camera_outlined),
              label: Text(l10n.capturePhoto),
            ),
            if (_photoPaths.isNotEmpty)
              Padding(
                padding: const EdgeInsets.only(top: FbSizes.gapSmall),
                child: Text(
                  '${_photoPaths.length} 📷',
                  style: const TextStyle(color: FbColors.textSecondary),
                ),
              ),
            const SizedBox(height: 24),
            FilledButton(
              onPressed: _projectId == null ? null : () => _submit(l10n),
              child: Text(l10n.submit),
            ),
          ],
        ),
      ),
    );
  }
}
