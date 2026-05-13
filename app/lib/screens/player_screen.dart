import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:just_audio/just_audio.dart';
import '../api/models/audio_url.dart';
import '../state/story_state.dart';

/// Plan 9-A player screen.
///
/// Watches two providers:
///  - [storyByIdProvider] for the story text (one-shot fetch).
///  - [audioUrlPollProvider] for the audio URL (streams ready/pending/failed).
///
/// Owns a [AudioPlayer] instance for its lifetime; **must** dispose it in
/// [dispose] to avoid leaking native audio resources across navigations.
class PlayerScreen extends ConsumerStatefulWidget {
  final int storyId;
  const PlayerScreen({super.key, required this.storyId});

  @override
  ConsumerState<PlayerScreen> createState() => _PlayerScreenState();
}

class _PlayerScreenState extends ConsumerState<PlayerScreen> {
  final AudioPlayer _player = AudioPlayer();
  String? _loadedUrl;

  @override
  void dispose() {
    // Free native audio resources — critical, otherwise the player keeps
    // streaming after navigation.
    _player.dispose();
    super.dispose();
  }

  Future<void> _ensureUrl(AudioReady ready) async {
    if (_loadedUrl == ready.url) return;
    _loadedUrl = ready.url;
    try {
      await _player.setUrl(ready.url);
    } catch (_) {
      // Leave _loadedUrl set so we don't retry every rebuild; the StreamBuilder
      // below will simply show a stopped state.
    }
  }

  @override
  Widget build(BuildContext context) {
    final storyAsync = ref.watch(storyByIdProvider(widget.storyId));
    final audioAsync = ref.watch(audioUrlPollProvider(widget.storyId));

    return Scaffold(
      appBar: AppBar(
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => context.canPop() ? context.pop() : context.go('/'),
        ),
        title: storyAsync.maybeWhen(
          data: (s) => Text(
            s.title.length > 18 ? '${s.title.substring(0, 18)}…' : s.title,
            overflow: TextOverflow.ellipsis,
          ),
          orElse: () => const Text('故事'),
        ),
      ),
      body: storyAsync.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (e, _) => Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text('故事加载失败：$e', textAlign: TextAlign.center),
          ),
        ),
        data: (story) => Column(
          children: [
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(20),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      story.title,
                      style: Theme.of(context).textTheme.headlineSmall,
                    ),
                    const SizedBox(height: 16),
                    Text(
                      story.text,
                      style: const TextStyle(fontSize: 16, height: 1.6),
                    ),
                  ],
                ),
              ),
            ),
            Container(
              width: double.infinity,
              padding: const EdgeInsets.all(16),
              color: Theme.of(context).colorScheme.surfaceContainerHighest,
              child: audioAsync.when(
                loading: () => const _Status('🐼 音频准备中...'),
                error: (e, _) => _Status('音频加载失败：$e'),
                data: (resp) => _audioContent(resp),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _audioContent(AudioUrlResponse resp) {
    return switch (resp) {
      AudioPending() => const _Status('🐼 爱宝在录音中...'),
      AudioFailed(message: final m) => Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            _Status('❌ $m'),
            const SizedBox(height: 8),
            FilledButton.tonal(
              onPressed: () => context.go('/generate'),
              child: const Text('重新生成'),
            ),
          ],
        ),
      AudioReady ready => FutureBuilder<void>(
          future: _ensureUrl(ready),
          builder: (_, __) => _PlayerControls(player: _player),
        ),
    };
  }
}

class _Status extends StatelessWidget {
  final String text;
  const _Status(this.text);
  @override
  Widget build(BuildContext context) => SizedBox(
        height: 64,
        child: Center(
          child: Text(text, style: const TextStyle(fontSize: 15)),
        ),
      );
}

class _PlayerControls extends StatelessWidget {
  final AudioPlayer player;
  const _PlayerControls({required this.player});

  @override
  Widget build(BuildContext context) {
    return StreamBuilder<PlayerState>(
      stream: player.playerStateStream,
      builder: (_, snap) {
        final playing = snap.data?.playing ?? false;
        return Row(
          children: [
            IconButton.filled(
              iconSize: 36,
              icon: Icon(playing ? Icons.pause : Icons.play_arrow),
              onPressed: () => playing ? player.pause() : player.play(),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: StreamBuilder<Duration>(
                stream: player.positionStream,
                builder: (_, posSnap) {
                  final pos = posSnap.data ?? Duration.zero;
                  return StreamBuilder<Duration?>(
                    stream: player.durationStream,
                    builder: (_, durSnap) {
                      final dur = durSnap.data ?? Duration.zero;
                      final maxMs = dur.inMilliseconds.toDouble();
                      final hasDur = maxMs > 0;
                      final posMs = pos.inMilliseconds
                          .toDouble()
                          .clamp(0.0, hasDur ? maxMs : 1.0);
                      return Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Slider(
                            min: 0,
                            max: hasDur ? maxMs : 1.0,
                            value: posMs,
                            onChanged: hasDur
                                ? (v) => player.seek(
                                      Duration(milliseconds: v.toInt()),
                                    )
                                : null,
                          ),
                          Text(
                            '${_fmt(pos)} / ${_fmt(dur)}',
                            style: const TextStyle(fontSize: 12),
                          ),
                        ],
                      );
                    },
                  );
                },
              ),
            ),
          ],
        );
      },
    );
  }

  static String _fmt(Duration d) {
    final m = d.inMinutes.toString().padLeft(2, '0');
    final s = (d.inSeconds % 60).toString().padLeft(2, '0');
    return '$m:$s';
  }
}
