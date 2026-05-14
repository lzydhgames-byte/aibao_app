import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import '../api/models/story_list_item.dart';

/// Vertical list of recent stories. Lives inside the home screen's outer
/// scroll view, so it is shrink-wrapped and disables its own scrolling.
class StoryHistoryList extends StatelessWidget {
  final List<StoryListItem> items;
  const StoryHistoryList({super.key, required this.items});

  @override
  Widget build(BuildContext context) {
    if (items.isEmpty) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 24),
        child: Center(
          child: Text(
            '还没有故事，去生成第一个吧～',
            style: TextStyle(
              fontSize: 14,
              color: Theme.of(context).colorScheme.onSurface.withValues(alpha: 0.6),
            ),
          ),
        ),
      );
    }
    return ListView.separated(
      shrinkWrap: true,
      physics: const NeverScrollableScrollPhysics(),
      itemCount: items.length,
      separatorBuilder: (_, __) => const SizedBox(height: 8),
      itemBuilder: (context, i) => _StoryRow(item: items[i]),
    );
  }
}

class _StoryRow extends StatelessWidget {
  final StoryListItem item;
  const _StoryRow({required this.item});

  String _styleLabel(String s) {
    switch (s) {
      case 'gentle':
        return '温柔';
      case 'adventure':
        return '冒险';
      case 'funny':
        return '搞笑';
      case 'educational':
        return '知识';
      default:
        return s;
    }
  }

  String _relativeTime(String iso) {
    if (iso.isEmpty) return '';
    final t = DateTime.tryParse(iso);
    if (t == null) return '';
    final diff = DateTime.now().difference(t);
    if (diff.inMinutes < 1) return '刚刚';
    if (diff.inMinutes < 60) return '${diff.inMinutes} 分钟前';
    if (diff.inHours < 24) return '${diff.inHours} 小时前';
    if (diff.inDays < 30) return '${diff.inDays} 天前';
    return '${(diff.inDays / 30).floor()} 个月前';
  }

  Widget? _badge(BuildContext context) {
    switch (item.audioStatus) {
      case 'pending':
        return const Text('🐼合成中', style: TextStyle(fontSize: 12));
      case 'failed':
        return const Text('❌', style: TextStyle(fontSize: 14));
      default:
        return null;
    }
  }

  @override
  Widget build(BuildContext context) {
    final subtitle =
        '${item.durationMinutes} 分钟 · ${_styleLabel(item.style)} · ${_relativeTime(item.createdAt)}';
    final badge = _badge(context);
    return Card(
      child: ListTile(
        title: Text(
          item.title,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: const TextStyle(fontWeight: FontWeight.w600),
        ),
        subtitle: Text(subtitle),
        trailing: badge ?? const Icon(Icons.chevron_right),
        onTap: () => context.push('/player/${item.id}'),
      ),
    );
  }
}
