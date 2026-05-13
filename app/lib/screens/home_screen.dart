import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../state/auth_state.dart';
import '../state/child_state.dart';

/// Plan 9-A home screen.
///
/// Shows the parent welcome line, the (single) child profile card, and a big
/// CTA that routes to /generate. Logout is in the AppBar.
class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final auth = ref.watch(authProvider);
    final childAsync = ref.watch(childProvider);
    final nickname = switch (auth) {
      AuthAuthenticated(:final user) => user.nickname.isEmpty
          ? '家长'
          : user.nickname,
      _ => '家长',
    };

    return Scaffold(
      appBar: AppBar(
        title: const Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text('🐼 ', style: TextStyle(fontSize: 24)),
            Text('爱宝'),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () async {
              await ref.read(authProvider.notifier).logout();
              // Router redirect handles route swap; nothing else to do.
            },
            child: const Text('退出'),
          ),
        ],
      ),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(
                '欢迎回来，$nickname家长',
                style: Theme.of(context).textTheme.titleMedium,
              ),
              const SizedBox(height: 16),
              Expanded(child: _childSection(context, childAsync)),
              const SizedBox(height: 16),
              SizedBox(
                height: 64,
                child: FilledButton.icon(
                  icon: const Text('🎤', style: TextStyle(fontSize: 24)),
                  label: const Text(
                    '今天讲什么故事？',
                    style: TextStyle(fontSize: 18),
                  ),
                  onPressed: () => context.go('/generate'),
                ),
              ),
              const SizedBox(height: 8),
            ],
          ),
        ),
      ),
    );
  }

  Widget _childSection(BuildContext context, AsyncValue childAsync) {
    return childAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(
        child: Text(
          '孩子档案加载失败：$e',
          textAlign: TextAlign.center,
          style: const TextStyle(color: Colors.redAccent),
        ),
      ),
      data: (child) {
        if (child == null) {
          return const Center(
            child: Padding(
              padding: EdgeInsets.symmetric(horizontal: 24),
              child: Text(
                '暂无孩子档案，请在后台 PowerShell 调用 '
                'POST /api/v1/children 创建',
                textAlign: TextAlign.center,
                style: TextStyle(fontSize: 15, color: Colors.black54),
              ),
            ),
          );
        }
        return Card(
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Row(
              children: [
                const Text('🐼', style: TextStyle(fontSize: 64)),
                const SizedBox(width: 20),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        child.nickname,
                        style: Theme.of(context).textTheme.headlineSmall,
                      ),
                      const SizedBox(height: 6),
                      Text(
                        _genderLabel(child.gender),
                        style: const TextStyle(fontSize: 14),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        '出生日期：${child.birthday}',
                        style: const TextStyle(
                          fontSize: 13,
                          color: Colors.black54,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }

  String _genderLabel(String g) {
    return switch (g) {
      'male' => '男孩',
      'female' => '女孩',
      _ => g.isEmpty ? '未填' : g,
    };
  }
}
