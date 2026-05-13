import 'package:flutter_test/flutter_test.dart';
import 'package:aibao_app/api/models/user.dart';
import 'package:aibao_app/api/models/child.dart';
import 'package:aibao_app/api/models/story.dart';
import 'package:aibao_app/api/models/audio_url.dart';

void main() {
  group('User.fromJson', () {
    test('full json', () {
      final u = User.fromJson({
        'id': 7,
        'nickname': 'alice',
        'subscription_tier': 'pro',
      });
      expect(u.id, 7);
      expect(u.nickname, 'alice');
      expect(u.subscriptionTier, 'pro');
    });

    test('minimal json (defaults)', () {
      final u = User.fromJson({'id': 1});
      expect(u.nickname, '');
      expect(u.subscriptionTier, 'free');
    });
  });

  group('Child.fromJson — profile flexibility', () {
    Map<String, dynamic> base(dynamic profile) => {
          'id': 1,
          'user_id': 2,
          'nickname': '小明',
          'gender': 'male',
          'birthday': '2020-01-01',
          'profile': profile,
        };

    test('profile as Map', () {
      final c = Child.fromJson(base({'description': '爱画画'}));
      expect(c.profile, isNotNull);
      expect(c.profile!['description'], '爱画画');
    });

    test('profile as JSON-encoded string', () {
      final c = Child.fromJson(base('{"description":"爱跑步"}'));
      expect(c.profile, isNotNull);
      expect(c.profile!['description'], '爱跑步');
    });

    test('profile as empty string -> null', () {
      final c = Child.fromJson(base(''));
      expect(c.profile, isNull);
    });

    test('profile as "{}" sentinel -> null', () {
      final c = Child.fromJson(base('{}'));
      expect(c.profile, isNull);
    });

    test('profile null', () {
      final c = Child.fromJson(base(null));
      expect(c.profile, isNull);
    });
  });

  group('Story.fromJson', () {
    Map<String, dynamic> base({int? storylineId, int? episodeNo}) => {
          'id': 10,
          'title': 'T',
          'text': 'body',
          'duration_minutes': 5,
          'style': 'warm',
          'topic': 'animals',
          'audio_status': 'ready',
          'storyline_id': storylineId,
          'episode_no': episodeNo,
          'created_at': '2026-05-15T00:00:00Z',
        };

    test('storyline_id & episode_no null', () {
      final s = Story.fromJson(base());
      expect(s.storylineId, isNull);
      expect(s.episodeNo, isNull);
    });

    test('storyline_id & episode_no filled', () {
      final s = Story.fromJson(base(storylineId: 3, episodeNo: 2));
      expect(s.storylineId, 3);
      expect(s.episodeNo, 2);
    });

    test('missing audio_status defaults to pending', () {
      final s = Story.fromJson({
        'id': 1,
        'created_at': '2026-05-15T00:00:00Z',
      });
      expect(s.audioStatus, 'pending');
    });
  });

  group('AudioUrlResponse.fromJson', () {
    test('ready', () {
      final r = AudioUrlResponse.fromJson({
        'audio_status': 'ready',
        'url': 'https://cdn/x.mp3',
        'expires_at': '2026-05-15T00:15:00Z',
      }, 200);
      expect(r, isA<AudioReady>());
      expect((r as AudioReady).url, 'https://cdn/x.mp3');
    });

    test('pending', () {
      final r = AudioUrlResponse.fromJson({
        'audio_status': 'pending',
        'retry_after': 4,
      }, 200);
      expect(r, isA<AudioPending>());
      expect((r as AudioPending).retryAfter, 4);
    });

    test('failed via 503', () {
      final r = AudioUrlResponse.fromJson({
        'code': 'audio_failed',
        'message': '生成失败',
      }, 503);
      expect(r, isA<AudioFailed>());
      expect((r as AudioFailed).message, '生成失败');
    });
  });
}
