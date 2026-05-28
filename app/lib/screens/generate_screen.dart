import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../feature_flags.dart';
import '../state/child_state.dart';
import '../state/outline_state.dart';
import '../widgets/duration_chips.dart';
import 'legacy_generate_screen.dart';

/// Plan 11A "tell me a story today" composer — B-mode preview-confirm flow.
///
/// Behavior tree:
///   1. If `storylineId != null` (sequel mode) — delegate to LegacyGenerateScreen
///      always. Sequels MUST NOT go through outline (spec §10.1 mutex).
///   2. Else if FeatureFlags.outlineEnabled = false — delegate to LegacyGenerateScreen
///      (emergency rollback path, --dart-define=OUTLINE_ENABLED=false).
///   3. Else — render the minimal preview-input UI (prompt + duration only,
///      AI infers style + theme + educational_value on the outline page).
class GenerateScreen extends ConsumerStatefulWidget {
  /// When non-null, this screen runs in sequel mode. Forwards directly to
  /// LegacyGenerateScreen (Plan 8 + 9b sequel UX unchanged).
  final int? storylineId;
  const GenerateScreen({super.key, this.storylineId});

  @override
  ConsumerState<GenerateScreen> createState() => _GenerateScreenState();
}

class _GenerateScreenState extends ConsumerState<GenerateScreen> {
  final _promptCtrl = TextEditingController();
  int _duration = 5;
  bool _submitting = false;
  String? _error;

  @override
  void dispose() {
    _promptCtrl.dispose();
    super.dispose();
  }

  Future<void> _onLetAibaoThink() async {
    final prompt = _promptCtrl.text.trim();
    if (prompt.isEmpty) {
      setState(() => _error = '说说今晚想听什么吧');
      return;
    }
    final child = ref.read(childProvider).valueOrNull;
    if (child == null) {
      setState(() => _error = '请先创建孩子档案');
      return;
    }

    setState(() {
      _submitting = true;
      _error = null;
    });
    try {
      final result = await ref.read(outlinePreviewProvider(
        OutlinePreviewParams(
          childId: child.id,
          prompt: prompt,
          durationMin: _duration,
        ),
      ).future);
      // Stash the outline on currentOutlineProvider so the outline screen can read it.
      ref.read(currentOutlineProvider.notifier).state = result;
      if (mounted) context.go('/outline');
    } catch (e) {
      if (mounted) setState(() => _error = '让爱宝想想失败了：$e');
    } finally {
      if (mounted) setState(() => _submitting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    // Sequel mode OR outline disabled → forward to legacy.
    if (widget.storylineId != null || !FeatureFlags.outlineEnabled) {
      return LegacyGenerateScreen(storylineId: widget.storylineId);
    }

    return Scaffold(
      appBar: AppBar(title: const Text('讲什么故事？')),
      body: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            TextField(
              controller: _promptCtrl,
              maxLines: 3,
              decoration: const InputDecoration(
                hintText: '比如：跟奥特曼一起冒险',
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 24),
            const Text('故事时长', style: TextStyle(fontWeight: FontWeight.bold)),
            const SizedBox(height: 8),
            DurationChips(
              selected: _duration,
              onChanged: (v) => setState(() => _duration = v),
            ),
            const SizedBox(height: 32),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.only(bottom: 12),
                child: Text(_error!, style: const TextStyle(color: Colors.red)),
              ),
            FilledButton(
              onPressed: _submitting ? null : _onLetAibaoThink,
              child: _submitting
                  ? const SizedBox(
                      width: 20,
                      height: 20,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : const Text('让爱宝想想'),
            ),
          ],
        ),
      ),
    );
  }
}
