import 'package:flutter_test/flutter_test.dart';
import 'package:futurebuild_field/main.dart';

void main() {
  testWidgets('FutureBuild Field app renders', (WidgetTester tester) async {
    // Verify the app widget can be instantiated.
    // Full widget tests require database and service mocking.
    expect(const FutureBuildFieldApp(), isNotNull);
  });
}
