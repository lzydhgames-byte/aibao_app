import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/api_exception.dart';
import '../api/models/audio_url.dart';
import '../api/models/story.dart';
import '../main.dart' show apiClientProvider;

/// Triggers POST /stories/generate. Resulting Story has audio_status=pending;
/// UI then subscribes to [audioUrlPollProvider] for the same story id.
class StoryGenerationNotifier extends AsyncNotifier<Story?> {
  @override
  Future<Story?> build() async => null;

  Future<Story> generate({
    required int childId,
    required String prompt,
    required int duration,
    required String style,
    String topic = '',
  }) async {
    state = const AsyncValue.loading();
    final api = ref.read(apiClientProvider);
    try {
      final story = await api.generateStory(
        childId: childId,
        prompt: prompt,
        duration: duration,
        style: style,
        topic: topic,
      );
      state = AsyncValue.data(story);
      return story;
    } on ApiException catch (e) {
      state = AsyncValue.error(Exception(e.userMsg), StackTrace.current);
      rethrow;
    }
  }
}

final storyGenerationProvider =
    AsyncNotifierProvider<StoryGenerationNotifier, Story?>(
  StoryGenerationNotifier.new,
);

/// One-shot story fetch by id (no polling).
final storyByIdProvider = FutureProvider.family<Story, int>((ref, id) async {
  final api = ref.watch(apiClientProvider);
  return api.getStory(id);
});

/// Polls GET /stories/:id/audio_url until ready or failed.
///
/// Termination: emits a final AudioReady / AudioFailed event then `return`s,
/// closing the stream. No infinite loop on terminal state.
final audioUrlPollProvider =
    StreamProvider.family<AudioUrlResponse, int>((ref, storyId) async* {
  final api = ref.watch(apiClientProvider);
  while (true) {
    try {
      final resp = await api.getAudioUrl(storyId);
      yield resp;
      if (resp is AudioReady || resp is AudioFailed) {
        return; // terminal — stop polling
      }
      final delay = resp is AudioPending ? resp.retryAfter : 3;
      await Future.delayed(Duration(seconds: delay));
    } on ApiException catch (e) {
      yield AudioFailed(message: e.userMsg);
      return;
    } catch (e) {
      yield AudioFailed(message: '音频加载失败: $e');
      return;
    }
  }
});
