import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:geolocator/geolocator.dart';

import '../l10n/app_localizations.dart';
import '../providers/app_providers.dart';
import '../theme/app_theme.dart';
import '../widgets/fb_dashed_border.dart';
import '../widgets/fb_sync_chip.dart';
import 'sync_status_screen.dart';

/// Backend cap on notes — internal/service/field.go caps at 4096 BYTES (Go
/// len()), so the screen validates utf8 byte length, not char count.
const int kMaxNotesBytes = 4096;

/// Standalone crew check-in (Phase 4a-i — extracted from the Daily Log): who is
/// on site (name + role), the site GPS, and optional notes. Everything is queued
/// to the outbox, so it submits offline and drains later. When offline, the
/// submit CTA wears the field offline affordance (an amber dashed border) paired
/// with a caption + the FbSyncChip — never colour alone.
class CheckInScreen extends ConsumerStatefulWidget {
  const CheckInScreen({super.key});

  @override
  ConsumerState<CheckInScreen> createState() => _CheckInScreenState();
}

/// One editable crew row. Disposed when removed or when the screen is.
class _CrewRow {
  final TextEditingController name = TextEditingController();
  final TextEditingController role = TextEditingController();

  void dispose() {
    name.dispose();
    role.dispose();
  }
}

class _CheckInScreenState extends ConsumerState<CheckInScreen> {
  final _formKey = GlobalKey<FormState>();
  final _notes = TextEditingController();
  final List<_CrewRow> _crew = [_CrewRow()];
  String? _projectId;
  Position? _position;
  bool _submitting = false;

  @override
  void initState() {
    super.initState();
    // Best-effort location on open; the user can refresh it.
    WidgetsBinding.instance.addPostFrameCallback((_) => _captureLocation());
  }

  @override
  void dispose() {
    _notes.dispose();
    for (final c in _crew) {
      c.dispose();
    }
    super.dispose();
  }

  Future<void> _captureLocation() async {
    try {
      var perm = await Geolocator.checkPermission();
      if (perm == LocationPermission.denied) {
        perm = await Geolocator.requestPermission();
      }
      if (perm == LocationPermission.denied ||
          perm == LocationPermission.deniedForever) {
        return;
      }
      final pos = await Geolocator.getCurrentPosition();
      if (mounted) setState(() => _position = pos);
    } catch (_) {
      // Location is best-effort; the check-in still queues without it.
    }
  }

  void _addCrew() => setState(() => _crew.add(_CrewRow()));

  void _removeCrew(int i) => setState(() => _crew.removeAt(i).dispose());

  /// The crew_members payload: rows with a non-empty name become
  /// `{name, role?}`. crew_members is stored opaquely server-side (JSONB), so
  /// free-text {name, role} is the no-backend-change shape.
  List<Map<String, dynamic>> _crewPayload() {
    final out = <Map<String, dynamic>>[];
    for (final c in _crew) {
      final name = c.name.text.trim();
      if (name.isEmpty) continue;
      final role = c.role.text.trim();
      out.add({'name': name, if (role.isNotEmpty) 'role': role});
    }
    return out;
  }

  Future<void> _submit(AppLocalizations l10n) async {
    // Synchronous double-submit guard: setState only SCHEDULES a rebuild, so a
    // second tap in the same frame would still see the button enabled.
    if (_submitting) return;
    final projectId = _projectId;
    if (projectId == null ||
        projectId.isEmpty ||
        !(_formKey.currentState?.validate() ?? false)) {
      return;
    }
    final crew = _crewPayload();
    if (crew.isEmpty) {
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(SnackBar(content: Text(l10n.checkInNeedsCrew)));
      return;
    }
    setState(() => _submitting = true);
    try {
      await ref
          .read(syncServiceProvider)
          .queueCheckin(
            projectId: projectId,
            crewMembers: crew,
            gpsLat: _position?.latitude,
            gpsLng: _position?.longitude,
            notes: _notes.text.trim().isEmpty ? null : _notes.text.trim(),
          );
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
    if (!mounted) return;
    ScaffoldMessenger.of(
      context,
    ).showSnackBar(SnackBar(content: Text(l10n.checkInQueued)));
    await Navigator.of(context).maybePop();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final online = ref.watch(onlineProvider).value ?? true;
    final tasks = ref.watch(tasksProvider).value ?? const [];
    final projectIds = {for (final t in tasks) t.projectId}.toList();
    // Reconcile against the LIVE project set every build — a background sync can
    // invalidate tasksProvider while this screen is open (HomeShell stays mounted
    // underneath and auto-syncs on reconnect/push), so a plain `??=` would leave
    // a stale or no-longer-listed selection: submitting against the wrong project
    // (silent) or tripping the dropdown's value-in-items assert. Empty ids (a
    // malformed cache row → ProjectTask.fromJson default "") are not submittable.
    if (_projectId == null || !projectIds.contains(_projectId)) {
      _projectId = projectIds.isNotEmpty ? projectIds.first : null;
    }
    final canSubmit =
        _projectId != null && _projectId!.isNotEmpty && !_submitting;

    final submit = FilledButton(
      onPressed: canSubmit ? () => _submit(l10n) : null,
      child: Text(l10n.submitCheckIn),
    );

    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.crewCheckIn),
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
      body: SingleChildScrollView(
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
                  decoration: InputDecoration(labelText: l10n.projectLabel),
                ),
                const SizedBox(height: FbSizes.gap),
              ],

              // ---- Crew roster ----
              Text(
                l10n.crewOnSite,
                style: const TextStyle(color: FbColors.textSecondary),
              ),
              const SizedBox(height: FbSizes.gapSmall),
              for (var i = 0; i < _crew.length; i++) ...[
                Row(
                  children: [
                    Expanded(
                      flex: 3,
                      child: TextFormField(
                        controller: _crew[i].name,
                        textCapitalization: TextCapitalization.words,
                        decoration: InputDecoration(
                          labelText: l10n.crewMemberName,
                        ),
                      ),
                    ),
                    const SizedBox(width: FbSizes.gapSmall),
                    Expanded(
                      flex: 2,
                      child: TextFormField(
                        controller: _crew[i].role,
                        textCapitalization: TextCapitalization.words,
                        decoration: InputDecoration(
                          labelText: l10n.crewMemberRole,
                        ),
                      ),
                    ),
                    // Glove-friendly: a full 56px tap target, spaced off the
                    // role field (DESIGN_SYSTEM_COMPONENTS §8).
                    const SizedBox(width: FbSizes.gapSmall),
                    SizedBox(
                      width: FbSizes.touchTarget,
                      height: FbSizes.touchTarget,
                      child: IconButton(
                        tooltip: l10n.removeCrewMember,
                        // Keep at least one row; disable remove on the last.
                        onPressed: _crew.length > 1
                            ? () => _removeCrew(i)
                            : null,
                        icon: const Icon(Icons.remove_circle_outline),
                        color: FbColors.safetyRed,
                        iconSize: 28,
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: FbSizes.gapSmall),
              ],
              Align(
                alignment: Alignment.centerLeft,
                child: TextButton.icon(
                  onPressed: _addCrew,
                  icon: const Icon(Icons.add_circle_outline),
                  label: Text(l10n.addCrewMember),
                ),
              ),
              const SizedBox(height: FbSizes.gap),

              // ---- Notes ----
              TextFormField(
                controller: _notes,
                minLines: 2,
                maxLines: 4,
                // Validate BYTES, not a char maxLength: the backend caps notes
                // at 4096 BYTES (Go len()), so a 4096-char maxLength would let a
                // multibyte (accented/emoji) note pass here and then 400 + park
                // silently on drain. utf8-encode to match the server exactly.
                // (A validator also avoids the maxLength "0/4096" counter chrome.)
                validator: (v) =>
                    (v != null && utf8.encode(v).length > kMaxNotesBytes)
                    ? l10n.notesTooLong
                    : null,
                decoration: InputDecoration(labelText: l10n.checkInNotes),
              ),

              // ---- Location ----
              Row(
                children: [
                  Icon(
                    _position != null
                        ? Icons.location_on
                        : Icons.location_off_outlined,
                    size: 18,
                    // textSecondary (≥7:1), not the muted slate token which is
                    // below the graphic-contrast floor.
                    color: _position != null
                        ? FbColors.gableGreen
                        : FbColors.textSecondary,
                  ),
                  const SizedBox(width: 6),
                  Expanded(
                    child: Text(
                      _position != null
                          ? l10n.locationCaptured
                          : l10n.locationUnavailable,
                      style: const TextStyle(color: FbColors.textSecondary),
                    ),
                  ),
                  TextButton(
                    onPressed: _captureLocation,
                    child: Text(l10n.updateLocation),
                  ),
                ],
              ),
              const SizedBox(height: FbSizes.gap),

              // ---- Submit (+ offline affordance) ----
              if (!online) ...[
                // liveRegion so a screen reader announces the offline state when
                // it appears (status is never colour-alone: icon + text + chip).
                Semantics(
                  liveRegion: true,
                  child: Row(
                    children: [
                      const Icon(
                        Icons.cloud_off_outlined,
                        size: 18,
                        color: FbColors.amber,
                      ),
                      const SizedBox(width: 6),
                      Expanded(
                        child: Text(
                          l10n.offlineWillQueue,
                          style: const TextStyle(color: FbColors.amber),
                        ),
                      ),
                    ],
                  ),
                ),
                const SizedBox(height: FbSizes.gapSmall),
                FbDashedBorder(child: submit),
              ] else
                submit,
            ],
          ),
        ),
      ),
    );
  }
}
