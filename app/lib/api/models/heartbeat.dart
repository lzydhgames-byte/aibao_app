/// Server response from `GET /api/v1/heartbeat?child_id=N`.
///
/// The greeting is computed server-side per the request hour (early/noon/
/// afternoon/evening/late-night). active_storylines is capped at 5.
class HeartbeatResponse {
  final String greeting;
  final List<ActiveStoryline> activeStorylines;

  HeartbeatResponse({
    required this.greeting,
    required this.activeStorylines,
  });

  factory HeartbeatResponse.fromJson(Map<String, dynamic> j) {
    final list = (j['active_storylines'] as List? ?? const []);
    return HeartbeatResponse(
      greeting: j['greeting'] as String? ?? '',
      activeStorylines: list
          .cast<Map<String, dynamic>>()
          .map(ActiveStoryline.fromJson)
          .toList(),
    );
  }
}

/// One row of `active_storylines` — a series the child is currently engaged
/// in. Plan 8 backend caps to 5 most-recent.
class ActiveStoryline {
  final int id;
  final String title;
  final int episodeCount;
  final String nextHint;
  final String? lastEpisodeAt; // ISO 8601 string, nullable when never played

  ActiveStoryline({
    required this.id,
    required this.title,
    required this.episodeCount,
    required this.nextHint,
    this.lastEpisodeAt,
  });

  factory ActiveStoryline.fromJson(Map<String, dynamic> j) => ActiveStoryline(
        id: j['id'] as int,
        title: j['title'] as String? ?? '',
        episodeCount: j['episode_count'] as int? ?? 0,
        nextHint: j['next_hint'] as String? ?? '',
        lastEpisodeAt: j['last_episode_at'] as String?,
      );
}
