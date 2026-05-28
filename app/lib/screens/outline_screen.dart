import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../main.dart' show apiClientProvider;
import '../state/child_state.dart';
import '../state/outline_state.dart';
import '../state/story_list_state.dart';

/// Plan 11A — outline card screen.
///
/// User arrives here from generate_screen after the AI returns a draft outline.
/// Three actions:
///   - "开始生成" → POST /stories/generate with outline_id → /player/:id
///   - "换个角度" → POST /outlines/:id/refresh (same group, variant++)
///   - "返回修改需求" → context.go('/generate')
///
/// Live 5-minute countdown derived from outline.expiresAt; on expiry the
/// "开始生成" button disables (server would 410 anyway, spec §6.5).
class OutlineScreen extends ConsumerStatefulWidget {
  const OutlineScreen({super.key});

  @override
  ConsumerState<OutlineScreen> createState() => _OutlineScreenState();
}

class _OutlineScreenState extends ConsumerState<OutlineScreen> {
  Timer? _timer;
  Duration _remaining = const Duration(minutes: 5);
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    final o = ref.read(currentOutlineProvider);
    if (o != null) {
      _remaining = o.expiresAt.difference(DateTime.now());
      _startTimer(o.expiresAt);
    }
  }

  void _startTimer(DateTime expiresAt) {
    _timer?.cancel();
    _timer = Timer.periodic(const Duration(seconds: 1), (_) {
      final left = expiresAt.difference(DateTime.now());
      if (!mounted) return;
      setState(() => _remaining = left.isNegative ? Duration.zero : left);
      if (left.isNegative) _timer?.cancel();
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _start() async {
    final outline = ref.read(currentOutlineProvider);
    final child = ref.read(childProvider).valueOrNull;
    if (outline == null || child == null) return;
    setState(() => _busy = true);
    try {
      final api = ref.read(apiClientProvider);
      final story = await api.generateStoryFromOutline(
        childId: child.id,
        outlineId: outline.outlineId,
      );
      // Plan 9b lesson: invalidate story list so home screen sees the new entry.
      ref.invalidate(storyListProvider);
      if (mounted) context.go('/player/${story.id}');
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('生成失败：$e')),
        );
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _refresh() async {
    final old = ref.read(currentOutlineProvider);
    if (old == null) return;
    setState(() => _busy = true);
    try {
      final api = ref.read(apiClientProvider);
      final fresh = await api.refreshOutline(old.outlineId);
      ref.read(currentOutlineProvider.notifier).state = fresh;
      _remaining = fresh.expiresAt.difference(DateTime.now());
      _startTimer(fresh.expiresAt);
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('换个角度失败：$e')),
        );
      }
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  String _fmtRemain(Duration d) {
    if (d.isNegative || d == Duration.zero) return '已过期';
    final m = d.inMinutes;
    final s = d.inSeconds % 60;
    return '剩余 $m:${s.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final outline = ref.watch(currentOutlineProvider);
    if (outline == null) {
      return Scaffold(
        appBar: AppBar(title: const Text('大纲')),
        body: const Center(child: Text('请先回上一步输入需求')),
      );
    }
    final o = outline.outline;
    final expired = _remaining == Duration.zero;
    return Scaffold(
      appBar: AppBar(
        title: const Text('爱宝想到了这个…'),
        actions: [
          Center(
            child: Padding(
              padding: const EdgeInsets.only(right: 16),
              child: Text(
                _fmtRemain(_remaining),
                style: TextStyle(
                  color: expired ? Colors.red : Colors.grey[700],
                ),
              ),
            ),
          ),
        ],
      ),
      body: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Text(
              o.title,
              style: const TextStyle(fontSize: 22, fontWeight: FontWeight.bold),
            ),
            const SizedBox(height: 16),
            const Text('📖 故事梗概', style: TextStyle(fontWeight: FontWeight.bold)),
            const SizedBox(height: 4),
            Text(o.synopsis),
            const SizedBox(height: 16),
            Wrap(
              spacing: 8,
              children: [
                Chip(label: Text('🎯 ${o.themes.join(' · ')}')),
                Chip(label: Text('🎨 ${o.style}')),
                Chip(label: Text('⏱ ${o.durationMin} 分钟')),
              ],
            ),
            const SizedBox(height: 12),
            Text(
              '教育意义：${o.educationalValue}',
              style: TextStyle(color: Colors.grey[700]),
            ),
            const Spacer(),
            FilledButton(
              onPressed: (_busy || expired) ? null : _start,
              child: _busy
                  ? const SizedBox(
                      width: 20,
                      height: 20,
                      child: CircularProgressIndicator(strokeWidth: 2),
                    )
                  : Text(expired ? '已过期，请重新预览' : '开始生成'),
            ),
            const SizedBox(height: 8),
            OutlinedButton(
              onPressed: (_busy || expired) ? null : _refresh,
              child: const Text('换个角度'),
            ),
            const SizedBox(height: 8),
            TextButton(
              onPressed: () => context.go('/generate'),
              child: const Text('返回修改需求'),
            ),
          ],
        ),
      ),
    );
  }
}
