import 'package:flutter_test/flutter_test.dart';
import 'package:aibao_app/state/story_state.dart';

void main() {
  test('story state providers are defined', () {
    // Compile-time smoke: provider symbols exist and are referenceable.
    expect(storyGenerationProvider, isNotNull);
    expect(storyByIdProvider, isNotNull);
    expect(audioUrlPollProvider, isNotNull);
  });
}
