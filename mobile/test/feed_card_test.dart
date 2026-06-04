import 'package:buildos_field/models/feed_card.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('fromJson parses a full card incl. nested actions', () {
    final card = FeedCard.fromJson({
      'id': 'c1',
      'card_type': 'procurement',
      'title': 'Order steel',
      'body': 'Lead time slipping',
      'priority': 'urgent',
      'status': 'active',
      'project_id': 'p1',
      'created_at': '2026-06-04T12:00:00Z',
      'actions': [
        {'label': 'Approve', 'action_type': 'approve'},
        {'label': 'Dismiss', 'action_type': 'dismiss'},
      ],
    });

    expect(card.id, 'c1');
    expect(card.cardType, 'procurement');
    expect(card.title, 'Order steel');
    expect(card.body, 'Lead time slipping');
    expect(card.priority, 'urgent');
    expect(card.status, 'active');
    expect(card.projectId, 'p1');
    expect(card.createdAt, '2026-06-04T12:00:00Z');
    expect(card.actions, hasLength(2));
    expect(card.actions.first.label, 'Approve');
    expect(card.actions.first.actionType, 'approve');
  });

  test('fromJson applies defaults when fields are absent', () {
    final card = FeedCard.fromJson(<String, dynamic>{});

    expect(card.id, '');
    expect(card.cardType, '');
    expect(card.title, '');
    expect(card.body, '');
    expect(card.priority, 'normal'); // default
    expect(card.status, 'active'); // default
    expect(card.projectId, isNull);
    expect(card.createdAt, isNull);
    expect(card.actions, isEmpty); // null actions -> const []
  });

  test('FeedCardAction.fromJson applies empty-string defaults', () {
    final action = FeedCardAction.fromJson(<String, dynamic>{});
    expect(action.label, '');
    expect(action.actionType, '');
  });

  test('priorityRank orders critical > urgent > normal > everything else', () {
    int rank(String p) => FeedCard.fromJson({'priority': p}).priorityRank;

    expect(rank('critical'), 0);
    expect(rank('urgent'), 1);
    expect(rank('normal'), 2);
    expect(rank('low'), 3); // wildcard arm
    expect(rank('anything-unknown'), 3); // wildcard arm
  });
}
