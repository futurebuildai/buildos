import 'package:buildos_field/models/outbox_action.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('OutboxAction round-trips its wire value', () {
    for (final action in OutboxAction.values) {
      expect(OutboxAction.fromWire(action.wire), action);
    }
  });
}
