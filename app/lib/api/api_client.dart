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
import 'models/story_list_item.dart';
import 'models/audio_url.dart';
import 'models/heartbeat.dart';
import 'models/bootstrap.dart';

class ApiClient {
  final Dio _dio;
  final TokenStorage _storage;
  final void Function()? _onUnauthorized;

  ApiClient({
    required TokenStorage storage,
    // Dev: real device via `adb reverse tcp:8080 tcp:8080` maps phone's
    // 127.0.0.1:8080 → host PC. AVD-only builds can override to 10.0.2.2.
    // Prod: compile-time override via
    //   flutter build apk --release --dart-define=API_BASE=https://aibao.dhgames.com
    // Plan 10 deployment uses the dart-define for release APKs distributed via QR code.
    String baseUrl = const String.fromEnvironment(
      'API_BASE',
      defaultValue: 'http://127.0.0.1:8080',
    ),
    Dio? dio,
    // Plan 9b: invoked when a non-/auth response is 401 (token rejected).
    // Wire to authProvider.logout to bounce user back to /login.
    void Function()? onUnauthorized,
  })  : _dio = dio ??
            Dio(BaseOptions(
              baseUrl: '$baseUrl/api/v1',
              connectTimeout: const Duration(seconds: 10),
              // 180s covers POST /stories/generate worst case: Plan 9c's
              // length guard can fire up to 2 rewrites (3 LLM calls total)
              // on an 8min story, each ~45-55s on Doubao under load.
              receiveTimeout: const Duration(seconds: 180),
              headers: {'Content-Type': 'application/json; charset=utf-8'},
              validateStatus: (status) => status != null && status < 600,
            )),
        _storage = storage,
        _onUnauthorized = onUnauthorized {
    // Order matters (Plan 9b 12-flutter.md entry):
    //   - request interceptors run in add order  (JWT inject first)
    //   - response interceptors run in REVERSE order (401 catcher runs first
    //     on the way back, after dio has the response)
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

    // Plan 9b: 401 catcher with /auth/* whitelist. Login itself returning 401
    // (e.g. wrong SMS code) must NOT bounce — user is already on /login.
    _dio.interceptors.add(InterceptorsWrapper(
      onResponse: (response, handler) {
        if (response.statusCode == 401) {
          final path = response.requestOptions.path;
          if (!path.startsWith('/auth/')) {
            _onUnauthorized?.call();
          }
        }
        handler.next(response);
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
    // Plan 8 storyline support. Mutually exclusive — server returns 400 if both set.
    bool startStoryline = false,
    int? storylineId,
  }) async {
    final body = <String, dynamic>{
      'child_id': childId,
      'prompt': prompt,
      'duration': duration,
      'style': style,
      'topic': topic,
    };
    if (startStoryline) body['start_storyline'] = true;
    if (storylineId != null) body['storyline_id'] = storylineId;
    final r = await _dio.post('/stories/generate', data: body);
    _ensureSuccess(r);
    return Story.fromJson(r.data as Map<String, dynamic>);
  }

  /// Plan 11A — POST /outlines/preview, returns AI-drafted outline card.
  Future<OutlinePreviewResponse> previewOutline({
    required int childId,
    required String prompt,
    required int durationMin,
  }) async {
    final r = await _dio.post('/outlines/preview', data: {
      'child_id': childId,
      'prompt': prompt,
      'duration_min': durationMin,
    });
    _ensureSuccess(r);
    return OutlinePreviewResponse.fromJson(r.data as Map<String, dynamic>);
  }

  /// Plan 11A — POST /outlines/:id/refresh, invalidates parent and regenerates
  /// a new outline in the same group with variant_index++.
  Future<OutlinePreviewResponse> refreshOutline(String outlineId) async {
    final r = await _dio.post('/outlines/$outlineId/refresh');
    _ensureSuccess(r);
    return OutlinePreviewResponse.fromJson(r.data as Map<String, dynamic>);
  }

  /// Plan 11A — generate a full story from a confirmed outline ticket.
  /// Mutually exclusive with start_storyline / storyline_id (server returns
  /// 400 conflicting_modes if both set).
  ///
  /// [overrides] may contain only `style`, `themes`, and `educational_value` —
  /// any other key the server silently drops (whitelist, spec §6.3).
  Future<Story> generateStoryFromOutline({
    required int childId,
    required String outlineId,
    Map<String, dynamic>? overrides,
  }) async {
    final body = <String, dynamic>{
      'child_id': childId,
      'outline_id': outlineId,
    };
    if (overrides != null && overrides.isNotEmpty) {
      body['outline_overrides'] = overrides;
    }
    final r = await _dio.post('/stories/generate', data: body);
    _ensureSuccess(r);
    return Story.fromJson(r.data as Map<String, dynamic>);
  }

  /// Plan 9b: list recent stories for a child, newest first (server caps to 50).
  Future<List<StoryListItem>> listStories(int childId, {int limit = 5}) async {
    final r = await _dio.get(
      '/stories',
      queryParameters: {'child_id': childId, 'limit': limit},
    );
    _ensureSuccess(r);
    final items = (r.data['items'] as List? ?? const [])
        .cast<Map<String, dynamic>>();
    return items.map(StoryListItem.fromJson).toList();
  }

  /// Plan 9b: time-aware greeting + active storylines (Plan 8 backend).
  Future<HeartbeatResponse> getHeartbeat(int childId) async {
    final r = await _dio.get(
      '/heartbeat',
      queryParameters: {'child_id': childId},
    );
    _ensureSuccess(r);
    return HeartbeatResponse.fromJson(r.data as Map<String, dynamic>);
  }

  /// Plan 9b: load the 7-question BOOTSTRAP form schema.
  Future<List<BootstrapQuestion>> getBootstrapQuestions() async {
    final r = await _dio.get('/bootstrap/questions');
    _ensureSuccess(r);
    final items = (r.data['questions'] as List? ?? const [])
        .cast<Map<String, dynamic>>();
    return items.map(BootstrapQuestion.fromJson).toList();
  }

  /// Plan 9b: submit BOOTSTRAP answers. Returns the rendered profile
  /// description (may be empty if upstream LLM was unavailable — fail-open
  /// per Plan 6 / 6b).
  Future<String> submitBootstrapAnswers({
    required int childId,
    required List<BootstrapAnswer> answers,
  }) async {
    final r = await _dio.post('/bootstrap/answers', data: {
      'child_id': childId,
      'answers': answers.map((a) => a.toJson()).toList(),
    });
    _ensureSuccess(r);
    return (r.data['description'] as String?) ?? '';
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

/// Plan 11A — outline content as returned by /outlines/preview + /outlines/:id/refresh.
class OutlineDto {
  final String title;
  final String synopsis;
  final List<String> themes;
  final String style;
  final String educationalValue;
  final int durationMin;
  final String outlineGroupId;
  final int variantIndex;
  final String outlinePromptVersion;

  OutlineDto({
    required this.title,
    required this.synopsis,
    required this.themes,
    required this.style,
    required this.educationalValue,
    required this.durationMin,
    required this.outlineGroupId,
    required this.variantIndex,
    required this.outlinePromptVersion,
  });

  factory OutlineDto.fromJson(Map<String, dynamic> json) => OutlineDto(
        title: json['title'] as String? ?? '',
        synopsis: json['synopsis'] as String? ?? '',
        themes: (json['themes'] as List?)?.cast<String>() ?? const [],
        style: json['style'] as String? ?? '',
        educationalValue: json['educational_value'] as String? ?? '',
        durationMin: (json['duration_min'] as num?)?.toInt() ?? 5,
        outlineGroupId: json['outline_group_id'] as String? ?? '',
        variantIndex: (json['variant_index'] as num?)?.toInt() ?? 0,
        outlinePromptVersion: json['outline_prompt_version'] as String? ?? '',
      );
}

/// Plan 11A — wrapper returned by /outlines/preview + /outlines/:id/refresh.
class OutlinePreviewResponse {
  final String outlineId;
  final OutlineDto outline;
  final DateTime expiresAt;

  OutlinePreviewResponse({
    required this.outlineId,
    required this.outline,
    required this.expiresAt,
  });

  factory OutlinePreviewResponse.fromJson(Map<String, dynamic> json) =>
      OutlinePreviewResponse(
        outlineId: json['outline_id'] as String,
        outline: OutlineDto.fromJson(json['outline'] as Map<String, dynamic>),
        expiresAt: DateTime.parse(json['expires_at'] as String),
      );
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
