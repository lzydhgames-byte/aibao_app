import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http_mock_adapter/http_mock_adapter.dart';

import 'package:aibao_app/api/api_client.dart';
import 'package:aibao_app/api/token_storage.dart';
import 'package:aibao_app/api/models/audio_url.dart';

void main() {
  late Dio dio;
  late DioAdapter adapter;
  late InMemoryTokenStorage storage;
  late ApiClient client;

  setUp(() {
    dio = Dio(BaseOptions(
      baseUrl: 'http://test.local/api/v1',
      validateStatus: (s) => s != null && s < 600,
    ));
    adapter = DioAdapter(dio: dio);
    storage = InMemoryTokenStorage();
    client = ApiClient(storage: storage, dio: dio);
  });

  test('sendSmsCode posts and succeeds', () async {
    adapter.onPost(
      '/auth/sms/send',
      (server) => server.reply(200, {'ok': true}),
      data: {'phone': '13800138000'},
    );
    await client.sendSmsCode('13800138000');
  });

  test('loginOrRegister persists token and returns LoginResult', () async {
    adapter.onPost(
      '/auth/login_or_register',
      (server) => server.reply(200, {
        'access_token': 'tok-abc',
        'refresh_token': 'ref-xyz',
        'user': {'id': 1, 'nickname': '宝宝', 'subscription_tier': 'free'},
      }),
      data: {'phone': '13800138000', 'code': '1234', 'nickname': ''},
    );
    final r = await client.loginOrRegister(
      phone: '13800138000',
      code: '1234',
    );
    expect(r.accessToken, 'tok-abc');
    expect(r.refreshToken, 'ref-xyz');
    expect(r.user.id, 1);
    expect(r.user.nickname, '宝宝');
    expect(await storage.read(), 'tok-abc');
  });

  test('getAudioUrl returns AudioFailed on 503', () async {
    adapter.onGet(
      '/stories/123/audio_url',
      (server) => server.reply(503, {
        'code': 'audio_failed',
        'message': 'TTS 服务异常',
      }),
    );
    final r = await client.getAudioUrl(123);
    expect(r, isA<AudioFailed>());
    expect((r as AudioFailed).message, 'TTS 服务异常');
  });
}
