import 'package:dio/dio.dart';
import 'api_exception.dart';
import 'token_storage.dart';
// NOTE: do NOT import secure_token_storage.dart here — tests must be able to
// load this file under flutter_tester (Windows) without pulling the
// flutter_secure_storage native plugin. App entrypoint constructs
// SecureTokenStorage and injects it.
import 'models/user.dart';
import 'models/child.dart';
import 'models/story.dart';
import 'models/audio_url.dart';

class ApiClient {
  final Dio _dio;
  final TokenStorage _storage;

  ApiClient({
    required TokenStorage storage,
    // Dev: real device via `adb reverse tcp:8080 tcp:8080` maps phone's
    // 127.0.0.1:8080 → host PC. AVD-only builds can override to 10.0.2.2.
    String baseUrl = 'http://127.0.0.1:8080',
    Dio? dio,
  })  : _dio = dio ??
            Dio(BaseOptions(
              baseUrl: '$baseUrl/api/v1',
              connectTimeout: const Duration(seconds: 10),
              receiveTimeout: const Duration(seconds: 60),
              headers: {'Content-Type': 'application/json; charset=utf-8'},
              validateStatus: (status) => status != null && status < 600,
            )),
        _storage = storage {
    _dio.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) async {
        final path = options.path;
        if (!path.startsWith('/auth/')) {
          final token = await _storage.read();
          if (token != null && token.isNotEmpty) {
            options.headers['Authorization'] = 'Bearer $token';
          }
        }
        return handler.next(options);
      },
    ));
  }

  /// Exposed for tests that need to attach a mock adapter.
  Dio get dio => _dio;

  // === Auth ===

  Future<void> sendSmsCode(String phone) async {
    final r = await _dio.post('/auth/sms/send', data: {'phone': phone});
    _ensureSuccess(r);
  }

  Future<LoginResult> loginOrRegister({
    required String phone,
    required String code,
    String nickname = '',
  }) async {
    final r = await _dio.post('/auth/login_or_register', data: {
      'phone': phone,
      'code': code,
      'nickname': nickname,
    });
    _ensureSuccess(r);
    final body = r.data as Map<String, dynamic>;
    final token = body['access_token'] as String;
    await _storage.write(token);
    return LoginResult(
      accessToken: token,
      refreshToken: body['refresh_token'] as String,
      user: User.fromJson(body['user'] as Map<String, dynamic>),
    );
  }

  Future<void> logout() => _storage.delete();

  Future<String?> currentToken() => _storage.read();

  // === Me ===

  Future<User> getMe() async {
    final r = await _dio.get('/me');
    _ensureSuccess(r);
    return User.fromJson(r.data as Map<String, dynamic>);
  }

  // === Children ===

  Future<List<Child>> listChildren() async {
    final r = await _dio.get('/children');
    _ensureSuccess(r);
    final items = (r.data['items'] as List).cast<Map<String, dynamic>>();
    return items.map(Child.fromJson).toList();
  }

  Future<Child> createChild({
    required String nickname,
    required String gender,
    required String birthday,
  }) async {
    final r = await _dio.post('/children', data: {
      'nickname': nickname,
      'gender': gender,
      'birthday': birthday,
    });
    _ensureSuccess(r);
    return Child.fromJson(r.data as Map<String, dynamic>);
  }

  // === Stories ===

  Future<Story> generateStory({
    required int childId,
    required String prompt,
    required int duration,
    required String style,
    String topic = '',
  }) async {
    final r = await _dio.post('/stories/generate', data: {
      'child_id': childId,
      'prompt': prompt,
      'duration': duration,
      'style': style,
      'topic': topic,
    });
    _ensureSuccess(r);
    return Story.fromJson(r.data as Map<String, dynamic>);
  }

  Future<Story> getStory(int id) async {
    final r = await _dio.get('/stories/$id');
    _ensureSuccess(r);
    return Story.fromJson(r.data as Map<String, dynamic>);
  }

  Future<AudioUrlResponse> getAudioUrl(int storyId) async {
    final r = await _dio.get('/stories/$storyId/audio_url');
    return AudioUrlResponse.fromJson(
      r.data as Map<String, dynamic>,
      r.statusCode ?? 0,
    );
  }

  // === Helpers ===

  void _ensureSuccess(Response r) {
    final code = r.statusCode ?? 0;
    if (code >= 200 && code < 300) return;
    final body = r.data as Map<String, dynamic>? ?? {};
    throw ApiException.fromResponse(code, body);
  }
}

class LoginResult {
  final String accessToken;
  final String refreshToken;
  final User user;
  LoginResult({
    required this.accessToken,
    required this.refreshToken,
    required this.user,
  });
}
