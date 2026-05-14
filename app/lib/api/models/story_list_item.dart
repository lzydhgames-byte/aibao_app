class StoryListItem {
  final int id;
  final String title;
  final int durationMinutes;
  final String style;
  final String audioStatus;
  final int? storylineId;
  final int? episodeNo;
  final String createdAt;

  StoryListItem({
    required this.id,
    required this.title,
    required this.durationMinutes,
    required this.style,
    required this.audioStatus,
    this.storylineId,
    this.episodeNo,
    required this.createdAt,
  });

  factory StoryListItem.fromJson(Map<String, dynamic> j) => StoryListItem(
        id: j['id'] as int,
        title: j['title'] as String? ?? '',
        durationMinutes: j['duration_minutes'] as int? ?? 0,
        style: j['style'] as String? ?? '',
        audioStatus: j['audio_status'] as String? ?? 'pending',
        storylineId: j['storyline_id'] as int?,
        episodeNo: j['episode_no'] as int?,
        createdAt: j['created_at'] as String? ?? '',
      );
}
