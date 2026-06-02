/// Mirrors internal/models.FeedCard (GET /api/v1/feed → { cards, pagination }).
class FeedCard {
  const FeedCard({
    required this.id,
    required this.cardType,
    required this.title,
    required this.body,
    required this.priority,
    required this.status,
    this.projectId,
    this.actions = const [],
    this.createdAt,
  });

  final String id;
  final String cardType;
  final String title;
  final String body;

  /// critical > urgent > normal > low.
  final String priority;
  final String status;
  final String? projectId;
  final List<FeedCardAction> actions;
  final String? createdAt;

  factory FeedCard.fromJson(Map<String, dynamic> json) => FeedCard(
    id: json['id'] as String? ?? '',
    cardType: json['card_type'] as String? ?? '',
    title: json['title'] as String? ?? '',
    body: json['body'] as String? ?? '',
    priority: json['priority'] as String? ?? 'normal',
    status: json['status'] as String? ?? 'active',
    projectId: json['project_id'] as String?,
    actions:
        (json['actions'] as List?)
            ?.map((e) => FeedCardAction.fromJson((e as Map).cast()))
            .toList() ??
        const [],
    createdAt: json['created_at'] as String?,
  );

  /// Sort rank: lower is higher priority.
  int get priorityRank => switch (priority) {
    'critical' => 0,
    'urgent' => 1,
    'normal' => 2,
    _ => 3,
  };
}

class FeedCardAction {
  const FeedCardAction({required this.label, required this.actionType});

  final String label;
  final String actionType;

  factory FeedCardAction.fromJson(Map<String, dynamic> json) => FeedCardAction(
    label: json['label'] as String? ?? '',
    actionType: json['action_type'] as String? ?? '',
  );
}
