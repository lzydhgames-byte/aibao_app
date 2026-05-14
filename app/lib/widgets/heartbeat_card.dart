import 'package:flutter/material.dart';
import '../api/models/heartbeat.dart';

/// Top-of-home greeting card from Plan 8 backend. Renders just the greeting
/// string in a soft-colored Material 3 card. Active storylines render
/// separately via `StorylineCarousel` so this widget stays focused.
class HeartbeatCard extends StatelessWidget {
  final HeartbeatResponse data;
  const HeartbeatCard({super.key, required this.data});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Card(
      color: scheme.primaryContainer,
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Row(
          children: [
            const Text('🐼', style: TextStyle(fontSize: 40)),
            const SizedBox(width: 16),
            Expanded(
              child: Text(
                data.greeting,
                style: TextStyle(
                  fontSize: 16,
                  color: scheme.onPrimaryContainer,
                  height: 1.4,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
