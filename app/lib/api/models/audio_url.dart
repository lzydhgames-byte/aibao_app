/// Sealed class encoding the 3-state response of GET /stories/:id/audio_url
sealed class AudioUrlResponse {
  factory AudioUrlResponse.fromJson(Map<String, dynamic> j, int statusCode) {
    if (statusCode == 503 || j['code'] == 'audio_failed') {
      return AudioFailed(message: j['message'] as String? ?? '音频生成失败');
    }
    final status = j['audio_status'] as String?;
    if (status == 'ready') {
      return AudioReady(
        url: j['url'] as String,
        expiresAt: DateTime.parse(j['expires_at'] as String),
      );
    }
    return AudioPending(retryAfter: j['retry_after'] as int? ?? 5);
  }
}

class AudioReady implements AudioUrlResponse {
  final String url;
  final DateTime expiresAt;
  AudioReady({required this.url, required this.expiresAt});
}

class AudioPending implements AudioUrlResponse {
  final int retryAfter;
  AudioPending({required this.retryAfter});
}

class AudioFailed implements AudioUrlResponse {
  final String message;
  AudioFailed({required this.message});
}
