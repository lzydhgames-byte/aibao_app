import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../api/models/child.dart';
import '../state/auth_state.dart';
import '../state/child_state.dart';
import '../state/heartbeat_state.dart';
import '../state/story_list_state.dart';
import '../widgets/bootstrap_prompt_card.dart';
import '../widgets/heartbeat_card.dart';
import '../widgets/storyline_carousel.dart';
import '../widgets/story_history_list.dart';

/// Plan 9b home screen — 4-section scroll: heartbeat, child card,
/// bootstrap-prompt (conditional), storyline carousel, story history.
/// FAB "今天讲什么故事？" floats above content.
class HomeScreen extends ConsumerWidget {
  const HomeScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final childAsync = ref.watch(childProvider);

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
          IconButton(
            icon: const Icon(Icons.logout),
            tooltip: '退出登录',
            onPressed: () async {
              await ref.read(authProvider.notifier).logout();
              // Router redirect handles route swap.
            },
          ),
        ],
      ),
      body: childAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text(
              '孩子档案加载失败：$e',
              textAlign: TextAlign.center,
              style: const TextStyle(color: Colors.redAccent),
            ),
          ),
        ),
        data: (child) {
          if (child == null) {
            // Router redirect should send user to /onboarding/create-child;
            // if we somehow land here, show a placeholder.
            return const Center(child: Text('加载中...'));
          }
          return _HomeBody(child: child);
        },
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: () => context.go('/generate'),
        icon: const Icon(Icons.mic),
        label: const Text('今天讲什么故事？'),
      ),
    );
  }
}

class _HomeBody extends ConsumerWidget {
  final Child child;
  const _HomeBody({required this.child});

  bool _needsBootstrap(Child c) {
    final desc = c.profile?['description'];
    return desc == null || (desc is String && desc.trim().isEmpty);
  }

  String _genderLabel(String g) => switch (g) {
        'male' => '男孩',
        'female' => '女孩',
        _ => g.isEmpty ? '未填' : g,
      };

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final heartbeatAsync = ref.watch(heartbeatProvider(child.id));
    final storiesAsync = ref.watch(storyListProvider(child.id));

    return SafeArea(
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 96),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // Heartbeat
            heartbeatAsync.when(
              loading: () => const SizedBox.shrink(),
              error: (_, __) => const SizedBox.shrink(),
              data: (hb) => HeartbeatCard(data: hb),
            ),
            const SizedBox(height: 12),
            // Child card
            _childCard(context),
            const SizedBox(height: 12),
            // Bootstrap prompt (conditional)
            if (_needsBootstrap(child)) ...[
              BootstrapPromptCard(
                childNickname: child.nickname,
                onTap: () => context.push('/bootstrap'),
              ),
              const SizedBox(height: 12),
            ],
            // Storyline carousel (only if any)
            heartbeatAsync.when(
              loading: () => const SizedBox.shrink(),
              error: (_, __) => const SizedBox.shrink(),
              data: (hb) {
                if (hb.activeStorylines.isEmpty) {
                  return const SizedBox.shrink();
                }
                return Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Padding(
                      padding: const EdgeInsets.symmetric(horizontal: 4),
                      child: Text(
                        '连续剧',
                        style: Theme.of(context).textTheme.titleMedium,
                      ),
                    ),
                    const SizedBox(height: 8),
                    StorylineCarousel(
                      items: hb.activeStorylines,
                      onTap: (sl) =>
                          context.go('/generate?storyline_id=${sl.id}'),
                    ),
                    const SizedBox(height: 12),
                  ],
                );
              },
            ),
            // Story history
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 4),
              child: Text(
                '最近听过',
                style: Theme.of(context).textTheme.titleMedium,
              ),
            ),
            const SizedBox(height: 8),
            storiesAsync.when(
              loading: () => const Padding(
                padding: EdgeInsets.symmetric(vertical: 24),
                child: Center(child: CircularProgressIndicator()),
              ),
              error: (_, __) => const SizedBox.shrink(),
              data: (list) => StoryHistoryList(items: list),
            ),
          ],
        ),
      ),
    );
  }

  Widget _childCard(BuildContext context) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Row(
          children: [
            const Text('🐼', style: TextStyle(fontSize: 56)),
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
  }
}
