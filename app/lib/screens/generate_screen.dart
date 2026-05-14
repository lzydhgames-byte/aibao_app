import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../api/models/story.dart';
import '../state/child_state.dart';
import '../state/story_list_state.dart';
import '../state/story_state.dart';
import '../widgets/duration_chips.dart';
import '../widgets/style_dropdown.dart';
import '../widgets/waiting_aibao.dart';

/// Plan 9-A "tell me a story today" composer.
class GenerateScreen extends ConsumerStatefulWidget {
  /// When non-null, this screen runs in sequel mode and posts
  /// `storyline_id` to /stories/generate. Plan 9b does NOT support
  /// starting a brand-new storyline from this screen (deferred to 9c).
  final int? storylineId;
  const GenerateScreen({super.key, this.storylineId});

  @override
  ConsumerState<GenerateScreen> createState() => _GenerateScreenState();
}

class _GenerateScreenState extends ConsumerState<GenerateScreen> {
  final _promptCtrl = TextEditingController();
  final _topicCtrl = TextEditingController();
  int _duration = 5;
  String _style = kStyleOptions.first;
  bool _submitting = false;

  @override
  void initState() {
    super.initState();
    if (widget.storylineId != null) {
      _promptCtrl.text = '继续之前的剧情';
    }
  }

  @override
  void dispose() {
    _promptCtrl.dispose();
    _topicCtrl.dispose();
    super.dispose();
  }

  void _toast(String msg) {
    ScaffoldMessenger.of(context)
      ..hideCurrentSnackBar()
      ..showSnackBar(SnackBar(content: Text(msg)));
  }

  Future<void> _submit() async {
    if (_submitting) return;
    final prompt = _promptCtrl.text.trim();
    if (prompt.isEmpty) {
      _toast('请告诉爱宝今天想听什么故事');
      return;
    }
    // Defensive: childProvider may still be loading or in error state when the
    // user taps "开始". Read the AsyncValue and bail with a snackbar instead
    // of crashing on .value!.
    final childAsync = ref.read(childProvider);
    if (childAsync.isLoading) {
      _toast('孩子档案加载中，请稍候');
      return;
    }
    final child = childAsync.valueOrNull;
    if (child == null) {
      _toast('请先创建孩子档案');
      return;
    }

    setState(() => _submitting = true);
    try {
      final future = ref.read(storyGenerationProvider.notifier).generate(
            childId: child.id,
            prompt: prompt,
            duration: _duration,
            style: _style,
            topic: _topicCtrl.text.trim(),
            storylineId: widget.storylineId,
          );
      final story = await showWaitingAibao<Story>(context, future);
      if (!mounted) return;
      if (story != null) {
        // Bust the home "最近听过" cache so the newly generated story
        // shows up when the user navigates back. storyListProvider is a
        // FutureProvider.family — without this invalidate it keeps the
        // pre-generation snapshot and silently loses the new entry.
        ref.invalidate(storyListProvider(child.id));
        context.go('/player/${story.id}');
      }
    } catch (e) {
      if (mounted) _toast(_friendly(e));
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
  }

  String _friendly(Object e) {
    final s = e.toString();
    return s.startsWith('Exception: ') ? s.substring('Exception: '.length) : s;
  }

  @override
  Widget build(BuildContext context) {
    final isSequel = widget.storylineId != null;
    return Scaffold(
      appBar: AppBar(
        title: Text(
          isSequel ? '续集 (storyline #${widget.storylineId})' : '今天的故事',
        ),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => context.canPop() ? context.pop() : context.go('/'),
        ),
      ),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 16),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Expanded(
                child: SingleChildScrollView(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      Text(
                        '讲一个什么故事？',
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                      const SizedBox(height: 8),
                      TextField(
                        controller: _promptCtrl,
                        maxLines: 4,
                        minLines: 3,
                        maxLength: 100,
                        decoration: const InputDecoration(
                          hintText: '讲一个森林小冒险...',
                          border: OutlineInputBorder(),
                        ),
                      ),
                      if (isSequel) ...[
                        const SizedBox(height: 8),
                        Container(
                          padding: const EdgeInsets.all(12),
                          decoration: BoxDecoration(
                            color: Theme.of(context)
                                .colorScheme
                                .secondaryContainer,
                            borderRadius: BorderRadius.circular(8),
                          ),
                          child: Text(
                            '📖 这是上一集的续集，故事会承接之前的角色和情节',
                            style: TextStyle(
                              fontSize: 13,
                              color: Theme.of(context)
                                  .colorScheme
                                  .onSecondaryContainer,
                            ),
                          ),
                        ),
                      ],
                      const SizedBox(height: 16),
                      const Text(
                        '时长',
                        style: TextStyle(fontWeight: FontWeight.bold),
                      ),
                      const SizedBox(height: 8),
                      DurationChips(
                        selected: _duration,
                        onChanged: (v) => setState(() => _duration = v),
                      ),
                      const SizedBox(height: 20),
                      const Text(
                        '风格',
                        style: TextStyle(fontWeight: FontWeight.bold),
                      ),
                      const SizedBox(height: 8),
                      StyleDropdown(
                        selected: _style,
                        onChanged: (v) => setState(() => _style = v),
                      ),
                      const SizedBox(height: 20),
                      const Text(
                        '主题（可选）',
                        style: TextStyle(fontWeight: FontWeight.bold),
                      ),
                      const SizedBox(height: 8),
                      TextField(
                        controller: _topicCtrl,
                        maxLength: 20,
                        inputFormatters: [
                          LengthLimitingTextInputFormatter(20),
                        ],
                        decoration: const InputDecoration(
                          hintText: '勇敢、友谊...',
                          border: OutlineInputBorder(),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 12),
              SizedBox(
                height: 56,
                child: FilledButton(
                  onPressed: _submitting ? null : _submit,
                  child: const Text(
                    '开始',
                    style: TextStyle(fontSize: 18),
                  ),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
