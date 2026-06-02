import 'package:buildos_field/models/outbox_action.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('OutboxAction', () {
    test('wire values match the backend field routes', () {
      expect(OutboxAction.progress.path, '/api/v1/field/progress');
      expect(OutboxAction.checkin.path, '/api/v1/field/checkin');
      expect(OutboxAction.dailyLog.path, '/api/v1/field/daily-log');
    });

    test('fromWire round-trips every action', () {
      for (final a in OutboxAction.values) {
        expect(OutboxAction.fromWire(a.wire), a);
      }
    });
  });
}
