class Story {
  final int id;
  final String title;
  final String text;
  final int durationMinutes;
  final String style;
  final String topic;
  final String audioStatus;
  final int? storylineId;
  final int? episodeNo;
  final String createdAt;

  Story({
    required this.id,
    required this.title,
    required this.text,
    required this.durationMinutes,
    required this.style,
    required this.topic,
    required this.audioStatus,
    this.storylineId,
    this.episodeNo,
    required this.createdAt,
  });

  factory Story.fromJson(Map<String, dynamic> j) => Story(
        id: j['id'] as int,
        title: j['title'] as String? ?? '',
        text: j['text'] as String? ?? '',
        durationMinutes: j['duration_minutes'] as int? ?? 0,
        style: j['style'] as String? ?? '',
        topic: j['topic'] as String? ?? '',
        audioStatus: j['audio_status'] as String? ?? 'pending',
        storylineId: j['storyline_id'] as int?,
        episodeNo: j['episode_no'] as int?,
        createdAt: j['created_at'] as String? ?? '',
      );
}
