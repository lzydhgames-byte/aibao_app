import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../state/bootstrap_state.dart';
import '../state/child_state.dart';
import '../widgets/bootstrap_question_card.dart';

/// 7-question BOOTSTRAP interview. Triggered when the active child has no
/// `profile.description` yet (or the parent taps the BootstrapPromptCard).
///
/// On submit the backend runs the answers through doubao-lite to produce a
/// description, which is then persisted on the child row. We invalidate
/// [childProvider] so home re-fetches the updated profile.
class BootstrapScreen extends ConsumerStatefulWidget {
  const BootstrapScreen({super.key});

  @override
  ConsumerState<BootstrapScreen> createState() => _BootstrapScreenState();
}

class _BootstrapScreenState extends ConsumerState<BootstrapScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final st = ref.read(bootstrapProvider);
      if (st.questions.isEmpty && !st.loading) {
        ref.read(bootstrapProvider.notifier).loadQuestions();
      }
    });
  }

  void _toast(String msg) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _submit(int childId) async {
    try {
      final desc =
          await ref.read(bootstrapProvider.notifier).submit(childId: childId);
      if (!mounted) return;
      ref.invalidate(childProvider);
      context.pop();
      _toast(desc.isEmpty ? '已保存' : '画像已生成，去听个新故事吧～');
    } catch (e) {
      if (!mounted) return;
      final s = e.toString();
      _toast(s.startsWith('Exception: ') ? s.substring(11) : s);
    }
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(bootstrapProvider);
    final childAsync = ref.watch(childProvider);

    return Scaffold(
      appBar: AppBar(title: const Text('完善小宇的画像')),
      body: childAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text('加载孩子档案失败: $e'),
          ),
        ),
        data: (child) {
          if (child == null) {
            return const Center(child: Text('请先创建孩子档案'));
          }
          if (state.loading) {
            return const Center(child: CircularProgressIndicator());
          }
          if (state.error != null && state.questions.isEmpty) {
            return Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text('加载失败: ${state.error}'),
                    const SizedBox(height: 12),
                    FilledButton(
                      onPressed: () => ref
                          .read(bootstrapProvider.notifier)
                          .loadQuestions(),
                      child: const Text('重试'),
                    ),
                  ],
                ),
              ),
            );
          }
          return _buildForm(context, state, child.id);
        },
      ),
    );
  }

  Widget _buildForm(
    BuildContext context,
    BootstrapFormState state,
    int childId,
  ) {
    return Column(
      children: [
        Expanded(
          child: ListView.builder(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
            itemCount: state.questions.length,
            itemBuilder: (context, i) {
              final q = state.questions[i];
              return Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: BootstrapQuestionCard(
                  question: q,
                  value: state.draft[q.id],
                  onChanged: (v) => ref
                      .read(bootstrapProvider.notifier)
                      .setAnswer(q.id, v),
                ),
              );
            },
          ),
        ),
        SafeArea(
          top: false,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
            child: SizedBox(
              width: double.infinity,
              height: 52,
              child: FilledButton(
                onPressed:
                    state.submitting ? null : () => _submit(childId),
                child: Text(state.submitting ? '保存中...' : '保存'),
              ),
            ),
          ),
        ),
      ],
    );
  }
}
