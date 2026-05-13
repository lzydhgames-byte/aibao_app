import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'package:aibao_app/main.dart';

void main() {
  testWidgets('Placeholder home renders', (WidgetTester tester) async {
    await tester.pumpWidget(const ProviderScope(child: AibaoApp()));
    expect(find.text('爱宝'), findsWidgets);
    expect(find.text('Plan 9-A 脚手架就绪'), findsOneWidget);
  });
}
