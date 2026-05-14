import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/models/story_list_item.dart';
import '../main.dart' show apiClientProvider;

/// FutureProvider.family keyed by childId — returns up to 5 recent stories
/// newest-first. Used by home screen "最近听过" section.
///
/// Limit is fixed at 5 here; if a separate "all history" screen lands later,
/// add another provider with larger limit rather than parameterizing.
final storyListProvider =
    FutureProvider.family<List<StoryListItem>, int>((ref, childId) async {
  final api = ref.watch(apiClientProvider);
  return api.listStories(childId, limit: 5);
});
