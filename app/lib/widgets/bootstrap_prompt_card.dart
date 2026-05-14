import 'package:flutter/material.dart';

/// Soft-colored card shown on home when the child has no profile.description
/// yet. Tapping the card invites the parent to fill the 7-question BOOTSTRAP
/// interview.
class BootstrapPromptCard extends StatelessWidget {
  final String childNickname;
  final VoidCallback onTap;

  const BootstrapPromptCard({
    super.key,
    required this.childNickname,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Card(
      color: scheme.secondaryContainer,
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(12),
        child: Padding(
          padding: const EdgeInsets.all(16),
          child: Row(
            children: [
              const Text('📝', style: TextStyle(fontSize: 32)),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      '完善$childNickname的画像',
                      style: TextStyle(
                        fontSize: 16,
                        fontWeight: FontWeight.w600,
                        color: scheme.onSecondaryContainer,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      '回答 7 个问题，让爱宝更懂他',
                      style: TextStyle(
                        fontSize: 13,
                        color: scheme.onSecondaryContainer.withValues(alpha: 0.8),
                      ),
                    ),
                  ],
                ),
              ),
              Icon(Icons.chevron_right, color: scheme.onSecondaryContainer),
            ],
          ),
        ),
      ),
    );
  }
}
