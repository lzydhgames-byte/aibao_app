import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/api_exception.dart';
import '../api/models/bootstrap.dart';
import '../main.dart' show apiClientProvider;

/// Snapshot of the BOOTSTRAP form: schema (questions) + current draft answers.
class BootstrapFormState {
  final List<BootstrapQuestion> questions;

  /// Keyed by question id; value matches question's `type`.
  /// (Stored loosely as dynamic to support text/single/multi/bool uniformly.)
  final Map<String, dynamic> draft;
  final bool loading;
  final bool submitting;
  final String? error;

  const BootstrapFormState({
    this.questions = const [],
    this.draft = const {},
    this.loading = false,
    this.submitting = false,
    this.error,
  });

  BootstrapFormState copyWith({
    List<BootstrapQuestion>? questions,
    Map<String, dynamic>? draft,
    bool? loading,
    bool? submitting,
    Object? error = _sentinel, // allow explicit null
  }) {
    return BootstrapFormState(
      questions: questions ?? this.questions,
      draft: draft ?? this.draft,
      loading: loading ?? this.loading,
      submitting: submitting ?? this.submitting,
      error: identical(error, _sentinel) ? this.error : error as String?,
    );
  }

  static const _sentinel = Object();
}

class BootstrapNotifier extends StateNotifier<BootstrapFormState> {
  BootstrapNotifier(this._ref) : super(const BootstrapFormState());
  final Ref _ref;

  /// Fetch the 7 questions; idempotent — calling twice is fine (rebuilds list).
  Future<void> loadQuestions() async {
    state = state.copyWith(loading: true, error: null);
    try {
      final api = _ref.read(apiClientProvider);
      final qs = await api.getBootstrapQuestions();
      // Seed draft defaults: empty string / empty list / false.
      final draft = <String, dynamic>{};
      for (final q in qs) {
        switch (q.type) {
          case 'text':
            draft[q.id] = '';
            break;
          case 'single_select':
            draft[q.id] =
                (q.options?.isNotEmpty ?? false) ? q.options!.first : '';
            break;
          case 'multi_select':
            draft[q.id] = <String>[];
            break;
          case 'boolean':
            draft[q.id] = false;
            break;
        }
      }
      state = state.copyWith(questions: qs, draft: draft, loading: false);
    } on ApiException catch (e) {
      state = state.copyWith(loading: false, error: e.userMsg);
    } catch (e) {
      state = state.copyWith(loading: false, error: e.toString());
    }
  }

  /// Update a single answer. Caller is responsible for passing the right
  /// shape (`String` / `List<String>` / `bool`) per question type.
  void setAnswer(String qId, dynamic value) {
    final next = Map<String, dynamic>.from(state.draft);
    next[qId] = value;
    state = state.copyWith(draft: next);
  }

  /// Validate against `required` flags + non-empty for text/multi-select.
  /// Returns null on success, or a user-facing reason on failure.
  String? validate() {
    for (final q in state.questions) {
      if (!q.required) continue;
      final v = state.draft[q.id];
      if (v == null) return '请填写：${q.label}';
      if (v is String && v.trim().isEmpty) return '请填写：${q.label}';
      if (v is List && v.isEmpty) return '请至少选一项：${q.label}';
      // booleans: required usually means "must answer", not "must be true";
      // since we seed to false, accept either value as a valid answer.
    }
    return null;
  }

  /// Submit current draft. Returns the rendered description string (may be
  /// empty if upstream LLM was unavailable; caller still treats success).
  /// Throws Exception on validation/api error.
  Future<String> submit({required int childId}) async {
    final problem = validate();
    if (problem != null) throw Exception(problem);
    state = state.copyWith(submitting: true, error: null);
    try {
      final api = _ref.read(apiClientProvider);
      final answers = state.draft.entries
          .map((e) => BootstrapAnswer(qId: e.key, value: e.value))
          .toList();
      final desc = await api.submitBootstrapAnswers(
        childId: childId,
        answers: answers,
      );
      state = state.copyWith(submitting: false);
      return desc;
    } on ApiException catch (e) {
      state = state.copyWith(submitting: false, error: e.userMsg);
      rethrow;
    } catch (e) {
      state = state.copyWith(submitting: false, error: e.toString());
      rethrow;
    }
  }
}

final bootstrapProvider =
    StateNotifierProvider<BootstrapNotifier, BootstrapFormState>(
  (ref) => BootstrapNotifier(ref),
);
