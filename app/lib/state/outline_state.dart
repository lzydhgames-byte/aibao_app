import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/api_client.dart';
import '../main.dart' show apiClientProvider;

/// Plan 11A — params for outlinePreviewProvider.
///
/// Equatable so FutureProvider.family caches by (childId, prompt, durationMin)
/// identity, not by reference. If any of these change, a fresh request fires;
/// otherwise the previous outline is served from cache.
class OutlinePreviewParams {
  final int childId;
  final String prompt;
  final int durationMin;
  const OutlinePreviewParams({
    required this.childId,
    required this.prompt,
    required this.durationMin,
  });

  @override
  bool operator ==(Object other) =>
      other is OutlinePreviewParams &&
      other.childId == childId &&
      other.prompt == prompt &&
      other.durationMin == durationMin;

  @override
  int get hashCode => Object.hash(childId, prompt, durationMin);
}

/// Plan 11A — fetches the AI-drafted outline card for the given input.
/// One-shot: refresh path uses ApiClient.refreshOutline directly and writes
/// the result into currentOutlineProvider.
final outlinePreviewProvider =
    FutureProvider.family<OutlinePreviewResponse, OutlinePreviewParams>(
  (ref, params) async {
    final api = ref.watch(apiClientProvider);
    return api.previewOutline(
      childId: params.childId,
      prompt: params.prompt,
      durationMin: params.durationMin,
    );
  },
);

/// Plan 11A — holds the outline the user is currently looking at on the
/// outline screen. Mutated by the screen on:
///   - first arrival from generate page (set to preview result)
///   - "换个角度" tap (replaced with refresh response)
///   - leaving the screen (set to null)
final currentOutlineProvider = StateProvider<OutlinePreviewResponse?>(
  (ref) => null,
);
