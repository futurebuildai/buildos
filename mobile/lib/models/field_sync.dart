import 'feed_card.dart';
import 'project_task.dart';

/// Mirrors internal/models.FieldSyncResponse — the bundle a mobile client
/// needs to refresh local state. `serverTime` is server-authoritative and is
/// passed back as `?since=` on the next sync (no client clock skew).
class FieldSyncResponse {
  const FieldSyncResponse({
    required this.tasks,
    required this.feedCards,
    required this.serverTime,
  });

  final List<ProjectTask> tasks;
  final List<FeedCard> feedCards;
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
        serverTime:
            DateTime.tryParse(json['server_time'] as String? ?? '')?.toUtc() ??
            DateTime.now().toUtc(),
      );
}
