import 'equipment_asset.dart';
import 'feed_card.dart';
import 'project_task.dart';

/// Mirrors internal/models.FieldSyncResponse — the bundle a mobile client
/// needs to refresh local state. `serverTime` is server-authoritative and is
/// passed back as `?since=` on the next sync (no client clock skew).
///
/// `equipment` is a FULL-SET collection (the server ignores `?since` for it),
/// so the client REPLACES its equipment cache each sync rather than merging.
class FieldSyncResponse {
  const FieldSyncResponse({
    required this.tasks,
    required this.feedCards,
    required this.equipment,
    required this.serverTime,
  });

  final List<ProjectTask> tasks;
  final List<FeedCard> feedCards;
  final List<EquipmentAsset> equipment;
  final DateTime serverTime;

  factory FieldSyncResponse.fromJson(Map<String, dynamic> json) =>
      FieldSyncResponse(
        tasks:
            (json['tasks'] as List?)
                ?.map((e) => ProjectTask.fromJson((e as Map).cast()))
                .toList() ??
            const [],
        feedCards:
            (json['feed_cards'] as List?)
                ?.map((e) => FeedCard.fromJson((e as Map).cast()))
                .toList() ??
            const [],
        equipment:
            (json['equipment'] as List?)
                ?.map((e) => EquipmentAsset.fromJson((e as Map).cast()))
                .toList() ??
            const [],
        serverTime:
            DateTime.tryParse(json['server_time'] as String? ?? '')?.toUtc() ??
            DateTime.now().toUtc(),
      );
}
