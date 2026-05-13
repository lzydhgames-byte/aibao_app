import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/api_exception.dart';
import '../api/models/child.dart';
import '../main.dart' show apiClientProvider;

/// Loads the user's first child (Plan 9-A assumes single-child UX).
/// Returns null if user has no child yet (handle in UI: prompt to create).
class ChildNotifier extends AsyncNotifier<Child?> {
  @override
  Future<Child?> build() async {
    final api = ref.watch(apiClientProvider);
    try {
      final children = await api.listChildren();
      return children.isEmpty ? null : children.first;
    } on ApiException catch (e) {
      throw Exception(e.userMsg);
    }
  }

  Future<Child> createChild({
    required String nickname,
    required String gender,
    required String birthday,
  }) async {
    final api = ref.read(apiClientProvider);
    final child = await api.createChild(
      nickname: nickname,
      gender: gender,
      birthday: birthday,
    );
    state = AsyncValue.data(child);
    return child;
  }
}

final childProvider =
    AsyncNotifierProvider<ChildNotifier, Child?>(ChildNotifier.new);
