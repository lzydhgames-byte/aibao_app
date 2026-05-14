import 'package:flutter/material.dart';
import '../api/models/heartbeat.dart';

/// Horizontal carousel of in-progress storylines. Tapping a card calls
/// [onTap] with the chosen storyline so the caller can navigate to the
/// generate screen with the storyline preloaded.
///
/// Renders nothing when [items] is empty — caller decides whether to hide
/// the surrounding section header.
class StorylineCarousel extends StatelessWidget {
  final List<ActiveStoryline> items;
  final void Function(ActiveStoryline) onTap;

  const StorylineCarousel({
    super.key,
    required this.items,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    if (items.isEmpty) return const SizedBox.shrink();
    return SizedBox(
      height: 160,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 4),
        itemCount: items.length,
        separatorBuilder: (_, __) => const SizedBox(width: 12),
        itemBuilder: (context, i) => _StorylineCard(
          item: items[i],
          onTap: () => onTap(items[i]),
        ),
      ),
    );
  }
}

class _StorylineCard extends StatelessWidget {
  final ActiveStoryline item;
  final VoidCallback onTap;
  const _StorylineCard({required this.item, required this.onTap});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return SizedBox(
      width: 240,
      child: Card(
        color: scheme.tertiaryContainer,
        child: InkWell(
          onTap: onTap,
          borderRadius: BorderRadius.circular(12),
          child: Padding(
            padding: const EdgeInsets.all(14),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    const Text('📖', style: TextStyle(fontSize: 22)),
                    const SizedBox(width: 8),
                    Expanded(
                      child: Text(
                        item.title,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w600,
                          color: scheme.onTertiaryContainer,
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 4),
                Text(
                  '已 ${item.episodeCount} 集',
                  style: TextStyle(
                    fontSize: 12,
                    color: scheme.onTertiaryContainer.withValues(alpha: 0.7),
                  ),
                ),
                const SizedBox(height: 6),
                Expanded(
                  child: Text(
                    item.nextHint.isEmpty ? '继续下一集...' : item.nextHint,
                    maxLines: 3,
                    overflow: TextOverflow.ellipsis,
                    style: TextStyle(
                      fontSize: 13,
                      color: scheme.onTertiaryContainer,
                      height: 1.3,
                    ),
                  ),
                ),
                Align(
                  alignment: Alignment.centerRight,
                  child: TextButton(
                    onPressed: onTap,
                    child: const Text('继续 →'),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
